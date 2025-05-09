import os
import sys
import time
import json
import signal
import logging
import argparse
import traceback
from typing import Dict, Any, Optional
from datetime import datetime

import redis
from redis.exceptions import RedisError

# 导入自定义服务模块
from common.storage import Storage
from parser.service import DocumentParser
from chunker.service import TextChunker, create_chunker
from embedding.service import TextEmbedder, create_embedder

# 配置日志格式
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[logging.StreamHandler(sys.stdout)]
)

# 任务状态常量
TASK_STATUS_PENDING = "pending"
TASK_STATUS_PROCESSING = "processing"
TASK_STATUS_COMPLETED = "completed"
TASK_STATUS_FAILED = "failed"

# 任务类型常量
TASK_TYPE_DOCUMENT_PROCESS = "document.process"
TASK_TYPE_DOCUMENT_DELETE = "document.delete"
TASK_TYPE_EMBEDDING_GENERATE = "embedding.generate"

class Worker:
    """工作进程类，负责处理任务队列中的任务"""

    def __init__(self, redis_url: str = None, queue_prefix: str = "taskqueue:", poll_interval: int = 5):
        """
        初始化工作进程

        Args:
            redis_url: Redis连接URL，格式为redis://[:password]@host:port/db
            queue_prefix: 队列键前缀
            poll_interval: 轮询间隔（秒）
        """
        self.logger = logging.getLogger("Worker")
        self.running = False
        self.poll_interval = poll_interval
        self.queue_prefix = queue_prefix

        # 从环境变量获取Redis配置（如果没有提供）
        if not redis_url:
            redis_host = os.environ.get("REDIS_HOST", "localhost")
            redis_port = int(os.environ.get("REDIS_PORT", 6379))
            redis_password = os.environ.get("REDIS_PASSWORD", "")
            redis_db = int(os.environ.get("REDIS_DB", 1))
            redis_url = f"redis://:{redis_password}@{redis_host}:{redis_port}/{redis_db}"

        # 初始化Redis客户端
        self.redis_client = redis.from_url(redis_url)

        # 初始化存储服务
        self.storage = Storage()

        # 初始化文档解析器
        self.parser = DocumentParser()

        # 初始化默认分块器（具体参数可从配置或环境变量中获取）
        chunk_size = int(os.environ.get("CHUNKER_SIZE", 1000))
        chunk_overlap = int(os.environ.get("CHUNKER_OVERLAP", 200))
        self.chunker = create_chunker(
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            split_type="paragraph",
            max_chunks=0
        )

        # 初始化嵌入服务
        api_key = os.environ.get("DASHSCOPE_API_KEY", "")
        if not api_key:
            self.logger.warning("DASHSCOPE_API_KEY not found in environment variables")
        self.embedder = create_embedder(
            model_name=os.environ.get("EMBEDDING_MODEL", "text-embedding-v3"),
            api_key=api_key,
            dimension=int(os.environ.get("EMBEDDING_DIMENSION", 1024))
        )

        self.logger.info("Worker initialized, waiting for tasks")

    def start(self):
        """启动工作进程，开始处理任务"""
        self.running = True
        self.logger.info("Worker started")

        # 注册信号处理器，优雅退出
        signal.signal(signal.SIGINT, self._signal_handler)
        signal.signal(signal.SIGTERM, self._signal_handler)

        try:
            self._run_loop()
        except Exception as e:
            self.logger.error(f"Unexpected error in worker: {e}")
            traceback.print_exc()
        finally:
            self.logger.info("Worker stopped")

    def stop(self):
        """停止工作进程"""
        self.logger.info("Stopping worker...")
        self.running = False

    def _signal_handler(self, sig, frame):
        """处理终止信号"""
        self.logger.info(f"Received signal {sig}, stopping worker")
        self.stop()

    def _run_loop(self):
        """主处理循环"""
        while self.running:
            try:
                # 检查任务队列中待处理的任务
                task = self._fetch_next_task()

                if task:
                    self.logger.info(f"Processing task {task['id']} of type {task['type']}")

                    # 将任务标记为处理中
                    self._mark_task_processing(task['id'])

                    # 处理任务
                    success, result_or_error = self._process_task(task)

                    # 更新任务状态
                    if success:
                        self._mark_task_completed(task['id'], result_or_error)
                    else:
                        self._mark_task_failed(task['id'], str(result_or_error))

                    # 向Redis发布通知
                    self._publish_task_update(task, success, result_or_error)
                else:
                    # 没有任务，等待下一个轮询间隔
                    time.sleep(self.poll_interval)

            except RedisError as e:
                self.logger.error(f"Redis error: {e}")
                time.sleep(self.poll_interval)  # 避免连接问题时过度重试
            except Exception as e:
                self.logger.error(f"Error in processing loop: {e}")
                traceback.print_exc()
                time.sleep(self.poll_interval)

    def _fetch_next_task(self) -> Optional[Dict[str, Any]]:
        """
        获取下一个待处理的任务

        Returns:
            任务对象或None（无任务时）
        """
        # 尝试从各个任务队列中获取任务
        task_types = [
            TASK_TYPE_DOCUMENT_PROCESS,
            TASK_TYPE_DOCUMENT_DELETE,
            TASK_TYPE_EMBEDDING_GENERATE
        ]

        for task_type in task_types:
            # 使用BRPOP非阻塞方式获取任务ID
            queue_key = f"{self.queue_prefix}queue:{task_type}"
            result = self.redis_client.rpop(queue_key)

            if result:
                # 获取到任务ID，读取任务详情
                task_id = result.decode('utf-8')
                task_key = f"{self.queue_prefix}task:{task_id}"
                task_data = self.redis_client.get(task_key)

                if task_data:
                    try:
                        return json.loads(task_data)
                    except json.JSONDecodeError:
                        self.logger.error(f"Failed to parse task data for task {task_id}")

        return None

    def _mark_task_processing(self, task_id: str):
        """将任务标记为处理中"""
        self._update_task_status(task_id, TASK_STATUS_PROCESSING)

    def _mark_task_completed(self, task_id: str, result: Dict[str, Any]):
        """将任务标记为已完成"""
        self._update_task_status(task_id, TASK_STATUS_COMPLETED, result=result)
        # 添加到已完成列表
        self.redis_client.lpush(f"{self.queue_prefix}completed_list", task_id)

    def _mark_task_failed(self, task_id: str, error_message: str):
        """将任务标记为失败"""
        self._update_task_status(task_id, TASK_STATUS_FAILED, error=error_message)
        # 添加到已完成列表（失败也视为完成）
        self.redis_client.lpush(f"{self.queue_prefix}completed_list", task_id)

    def _update_task_status(self, task_id: str, status: str, result: Dict[str, Any] = None, error: str = None):
        """
        更新任务状态

        Args:
            task_id: 任务ID
            status: 新状态
            result: 任务结果（可选）
            error: 错误信息（可选）
        """
        task_key = f"{self.queue_prefix}task:{task_id}"

        # 获取当前任务数据
        task_data = self.redis_client.get(task_key)
        if not task_data:
            self.logger.error(f"Task {task_id} not found")
            return

        try:
            task = json.loads(task_data)
            task["status"] = status
            task["updated_at"] = datetime.now().isoformat()

            if result is not None:
                task["result"] = result

            if error is not None:
                task["error"] = error

            # 保存更新后的任务数据
            self.redis_client.set(task_key, json.dumps(task))

        except json.JSONDecodeError:
            self.logger.error(f"Failed to parse task data for task {task_id}")

    def _publish_task_update(self, task: Dict[str, Any], success: bool, result_or_error: Any):
        """
        发布任务更新通知

        Args:
            task: 任务数据
            success: 是否成功
            result_or_error: 结果或错误信息
        """
        # 针对文档处理任务，发送状态更新
        if task['type'] == TASK_TYPE_DOCUMENT_PROCESS:
            document_id = task['file_id']
            channel = "document_updates"

            if success:
                # 从结果中获取段落数量
                segment_count = result_or_error.get('segment_count', 0)
                message = {
                    "document_id": document_id,
                    "status": "completed",
                    "segment_count": segment_count
                }
            else:
                message = {
                    "document_id": document_id,
                    "status": "failed",
                    "error": str(result_or_error)
                }

            # 发布消息
            self.redis_client.publish(channel, json.dumps(message))

    def _process_task(self, task: Dict[str, Any]) -> tuple[bool, Any]:
        """
        处理任务

        Args:
            task: 任务数据

        Returns:
            (success, result_or_error): 元组，表示处理是否成功及结果或错误信息
        """
        task_type = task['type']

        try:
            # 根据任务类型分发处理
            if task_type == TASK_TYPE_DOCUMENT_PROCESS:
                return self._process_document(task)
            elif task_type == TASK_TYPE_DOCUMENT_DELETE:
                return self._delete_document(task)
            elif task_type == TASK_TYPE_EMBEDDING_GENERATE:
                return self._generate_embedding(task)
            else:
                return False, f"Unknown task type: {task_type}"
        except Exception as e:
            self.logger.error(f"Error processing task {task['id']}: {e}")
            traceback.print_exc()
            return False, str(e)

    def _process_document(self, task: Dict[str, Any]) -> tuple[bool, Any]:
        """
        处理文档解析任务

        Args:
            task: 任务数据

        Returns:
            (success, result_or_error): 元组，表示处理是否成功及结果或错误信息
        """
        file_id = task['file_id']
        file_path = task['file_path']

        self.logger.info(f"Processing document: {file_id}, path: {file_path}")

        try:
            # 1. 从存储获取文件
            file_content = self.storage.get_file(file_path)

            # 2. 解析文档
            document_text = self.parser._parse_text(file_content)

            # 3. 分割文本
            chunks = self.chunker.split(document_text)
            self.logger.info(f"Document split into {len(chunks)} chunks")

            # 4. 为每个块生成嵌入向量
            segments = []
            for chunk in chunks:
                # 获取文本块的嵌入向量
                vector = self.embedder.embed_text(chunk.text)

                # 构建段落数据
                segment = {
                    "id": f"{file_id}_{chunk.index}",
                    "file_id": file_id,
                    "position": chunk.index,
                    "text": chunk.text,
                    "vector": vector,
                    "created_at": datetime.now().isoformat()
                }

                segments.append(segment)

            # 5. 构建结果
            result = {
                "segment_count": len(segments),
                "segments": segments
            }

            return True, result
        except Exception as e:
            self.logger.error(f"Document processing failed: {e}")
            traceback.print_exc()
            return False, str(e)

    def _delete_document(self, task: Dict[str, Any]) -> tuple[bool, Any]:
        """
        处理文档删除任务

        Args:
            task: 任务数据

        Returns:
            (success, result_or_error): 元组，表示处理是否成功及结果或错误信息
        """
        file_id = task['file_id']

        self.logger.info(f"Deleting document: {file_id}")

        try:
            # 从存储中删除文件
            self.storage.delete_file(file_id)

            # 构建结果
            result = {
                "message": f"Document {file_id} deleted successfully"
            }

            return True, result
        except Exception as e:
            self.logger.error(f"Document deletion failed: {e}")
            return False, str(e)

    def _generate_embedding(self, task: Dict[str, Any]) -> tuple[bool, Any]:
        """
        处理嵌入向量生成任务

        Args:
            task: 任务数据

        Returns:
            (success, result_or_error): 元组，表示处理是否成功及结果或错误信息
        """
        texts = task.get('texts', [])
        batch_size = min(task.get('batch_size', 6), 6)  # Ensure max is 6 for v3 model

        if not texts:
            return False, "No texts provided for embedding generation"

        try:
            all_embeddings = []

            # 批量处理
            for i in range(0, len(texts), batch_size):
                batch = texts[i:i+batch_size]
                batch_embeddings = self.embedder.embed_batch(batch)
                all_embeddings.extend(batch_embeddings)

            # 构建结果
            result = {
                "embeddings": all_embeddings,
                "count": len(all_embeddings)
            }

            return True, result
        except Exception as e:
            self.logger.error(f"Embedding generation failed: {e}")
            return False, str(e)


def main():
    """主函数"""
    parser = argparse.ArgumentParser(description='Python document processor worker')
    parser.add_argument('--redis-url', help='Redis connection URL')
    parser.add_argument('--queue-prefix', default='taskqueue:', help='Queue key prefix in Redis')
    parser.add_argument('--poll-interval', type=int, default=5, help='Poll interval in seconds')
    parser.add_argument('--log-level', default='INFO', choices=['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL'],
                        help='Set the logging level')

    args = parser.parse_args()

    # 设置日志级别
    logging.getLogger().setLevel(getattr(logging, args.log_level))

    worker = Worker(
        redis_url=args.redis_url,
        queue_prefix=args.queue_prefix,
        poll_interval=args.poll_interval
    )

    worker.start()

if __name__ == "__main__":
    main()