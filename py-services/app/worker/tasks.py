import os
import time
from typing import Any, Optional
from datetime import datetime, timedelta
import traceback

import redis
from celery import shared_task
from redis.exceptions import RedisError

from app.models.model import (
    Task, TaskType, TaskStatus,
    DocumentParsePayload, DocumentParseResult,
    TextChunkPayload, TextChunkResult, ChunkInfo,
    VectorizePayload, VectorizeResult, VectorInfo,
    ProcessCompletePayload, ProcessCompleteResult
)
from app.utils.utils import (
    logger, parse_redis_url, format_task_info, get_task_key,
    get_document_tasks_key, count_words, count_chars, retry, send_callback
)
from app.worker.celery_app import app

# 配置Redis连接参数
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")
REDIS_PARAMS = parse_redis_url(REDIS_URL)
CALLBACK_URL = os.getenv("CALLBACK_URL", "http://localhost:8080/api/tasks/callback")

# 获取Redis连接
def get_redis_client():
    """获取Redis客户端连接"""
    return redis.Redis(**REDIS_PARAMS)

# 任务辅助函数
def get_task_from_redis(task_id: str) -> Optional[Task]:
    """
    从Redis获取任务信息

    参数:
        task_id: 任务ID

    返回:
        Task: 任务对象，如果不存在则返回None
    """
    try:
        client = get_redis_client()
        key = get_task_key(task_id)
        data = client.get(key)
        if not data:
            logger.error(f"Task {task_id} not found in Redis")
            return None

        return Task.from_json(data)
    except RedisError as e:
        logger.error(f"Redis error when getting task {task_id}: {str(e)}")
        return None
    except Exception as e:
        logger.error(f"Failed to get task {task_id}: {str(e)}")
        return None

def update_task_status(task: Task, status: TaskStatus, result: Any = None, error: str = "") -> bool:
    """
    更新任务状态

    参数:
        task: 任务对象
        status: 新状态
        result: 处理结果
        error: 错误信息

    返回:
        bool: 成功返回True，失败返回False
    """
    try:
        client = get_redis_client()
        task.status = status
        task.updated_at = datetime.now()

        if status == TaskStatus.PROCESSING and task.started_at is None:
            task.started_at = datetime.now()

        if status in [TaskStatus.COMPLETED, TaskStatus.FAILED]:
            task.completed_at = datetime.now()

        if result is not None:
            task.result = result

        if error:
            task.error = error

        # 保存到Redis
        key = get_task_key(task.id)
        client.set(key, task.to_json())

        # 发送回调通知
        try:
            callback_data = {
                "task_id": task.id,
                "document_id": task.document_id,
                "status": task.status,
                "type": task.type,
                "result": task.result,
                "error": task.error,
                "timestamp": datetime.now().isoformat()
            }
            send_callback(CALLBACK_URL, callback_data)
        except Exception as e:
            logger.warning(f"Failed to send callback for task {task.id}: {str(e)}")

        return True
    except Exception as e:
        logger.error(f"Failed to update task {task.id} status: {str(e)}")
        return False

# 文档解析任务
@shared_task(name="app.worker.tasks.parse_document")
def parse_document(task_id: str) -> bool:
    """
    解析文档任务

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting document parse task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        payload = DocumentParsePayload(**task.payload)
        logger.info(f"Parsing document: {payload.file_path}")

        # TODO: 实现文档解析逻辑，这需要文档解析模块完成后再实现
        # 从存储中获取文件
        # 基于文件类型选择解析器
        # 执行解析

        # 临时模拟解析结果
        start_time = time.time()
        time.sleep(2)  # 模拟处理时间

        # 模拟的解析结果
        content = f"This is simulated content for {payload.file_name}"
        title = payload.file_name.split('.')[0]

        # 创建结果
        result = DocumentParseResult(
            content=content,
            title=title,
            meta={"source": payload.file_path},
            pages=1,
            words=count_words(content),
            chars=count_chars(content)
        )

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)

        elapsed = time.time() - start_time
        logger.info(f"Document parse task {task_id} completed in {elapsed:.2f}s")
        return True

    except Exception as e:
        error_msg = f"Document parse failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False

# 文本分块任务
@shared_task(name="app.worker.tasks.chunk_text")
def chunk_text(task_id: str) -> bool:
    """
    文本分块任务

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting text chunking task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        payload = TextChunkPayload(**task.payload)
        logger.info(f"Chunking text for document: {payload.document_id}")

        # TODO: 实现文本分块逻辑，这需要文本分块模块完成后再实现
        # 基于分块类型选择分块算法
        # 执行分块

        # 临时模拟分块结果
        start_time = time.time()
        time.sleep(1)  # 模拟处理时间

        # 简单的文本分块模拟
        text = payload.content
        chunks = []
        chunk_size = payload.chunk_size or 1000
        overlap = payload.overlap or 200

        # 非常简单的分块逻辑，实际实现应该更复杂
        words = text.split()
        chunk_words = []
        chunk_index = 0

        for i, word in enumerate(words):
            chunk_words.append(word)

            # 达到块大小，创建块
            if len(chunk_words) >= chunk_size:
                chunk_text = " ".join(chunk_words)
                chunks.append(ChunkInfo(text=chunk_text, index=chunk_index))
                chunk_index += 1

                # 保留重叠部分的单词
                overlap_words = min(overlap, len(chunk_words))
                chunk_words = chunk_words[-overlap_words:] if overlap_words > 0 else []

        # 添加剩余的单词
        if chunk_words:
            chunk_text = " ".join(chunk_words)
            chunks.append(ChunkInfo(text=chunk_text, index=chunk_index))

        # 创建结果
        result = TextChunkResult(
            document_id=payload.document_id,
            chunks=chunks,
            chunk_count=len(chunks)
        )

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)

        elapsed = time.time() - start_time
        logger.info(f"Text chunking task {task_id} completed in {elapsed:.2f}s: {len(chunks)} chunks created")
        return True

    except Exception as e:
        error_msg = f"Text chunking failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False

# 向量化任务
@shared_task(name="app.worker.tasks.vectorize_text")
def vectorize_text(task_id: str) -> bool:
    """
    向量化文本任务

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting text vectorization task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        payload = VectorizePayload(**task.payload)
        logger.info(f"Vectorizing text for document: {payload.document_id}, chunks: {len(payload.chunks)}")

        # TODO: 实现向量化逻辑，这需要嵌入模块完成后再实现
        # 基于模型名称选择嵌入模型
        # 执行向量化

        # 临时模拟向量化结果
        start_time = time.time()
        time.sleep(2)  # 模拟处理时间

        # 假设的向量维度
        dimension = 1536  # 常见的向量维度
        vectors = []

        # 为每个块创建随机向量（实际应使用嵌入模型）
        for chunk in payload.chunks:
            # 创建简单的随机向量，实际应该使用嵌入模型
            import numpy as np
            vector = np.random.rand(dimension).astype(np.float32).tolist()
            vectors.append(VectorInfo(
                chunk_index=chunk.index,
                vector=vector
            ))

        # 创建结果
        result = VectorizeResult(
            document_id=payload.document_id,
            vectors=vectors,
            vector_count=len(vectors),
            model=payload.model or "default",
            dimension=dimension
        )

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)

        elapsed = time.time() - start_time
        logger.info(f"Text vectorization task {task_id} completed in {elapsed:.2f}s: {len(vectors)} vectors created")
        return True

    except Exception as e:
        error_msg = f"Text vectorization failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False

# 完整文档处理任务
@shared_task(name="app.worker.tasks.process_document")
def process_document(task_id: str) -> bool:
    """
    完整文档处理流程

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting complete document processing task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        payload = ProcessCompletePayload(**task.payload)
        logger.info(f"Processing document: {payload.document_id}, file: {payload.file_path}")

        result = ProcessCompleteResult(
            document_id=payload.document_id,
            parse_status="pending",
            chunk_status="pending",
            vector_status="pending"
        )

        # 1. 解析文档
        start_time = time.time()
        try:
            # 创建解析任务载荷
            parse_payload = DocumentParsePayload(
                file_path=payload.file_path,
                file_name=payload.file_name,
                file_type=payload.file_type,
                metadata=payload.metadata
            )

            # 创建子任务并等待完成
            # 注意：在实际实现中应该使用更异步的方式，以下为简化示例
            # 模拟解析结果
            time.sleep(1)
            content = f"This is the content of {payload.file_name}"
            title = payload.file_name.split('.')[0]

            parse_result = DocumentParseResult(
                content=content,
                title=title,
                meta={"source": payload.file_path},
                pages=1,
                words=count_words(content),
                chars=count_chars(content)
            )

            result.parse_status = "completed"
            logger.info(f"Document parse completed for {payload.document_id}")
        except Exception as e:
            result.parse_status = "failed"
            result.error = f"Parse failed: {str(e)}"
            logger.error(f"Document parse failed: {str(e)}")
            # 在解析失败时，提前完成任务
            update_task_status(task, TaskStatus.FAILED, result.__dict__, result.error)
            return False

        # 2. 文本分块
        try:
            # 创建分块任务载荷
            chunk_payload = TextChunkPayload(
                document_id=payload.document_id,
                content=parse_result.content,
                chunk_size=payload.chunk_size,
                overlap=payload.overlap,
                split_type=payload.split_type
            )

            # 模拟分块
            time.sleep(1)
            chunks = []
            words = parse_result.content.split()
            chunk_size = 100
            for i in range(0, len(words), chunk_size):
                end = min(i + chunk_size, len(words))
                chunk_text = " ".join(words[i:end])
                chunks.append(ChunkInfo(text=chunk_text, index=len(chunks)))

            chunk_result = TextChunkResult(
                document_id=payload.document_id,
                chunks=chunks,
                chunk_count=len(chunks)
            )

            result.chunk_status = "completed"
            result.chunk_count = len(chunks)
            logger.info(f"Text chunking completed for {payload.document_id}: {len(chunks)} chunks")
        except Exception as e:
            result.chunk_status = "failed"
            result.error = f"Chunking failed: {str(e)}"
            logger.error(f"Text chunking failed: {str(e)}")
            # 在分块失败时，提前完成任务
            update_task_status(task, TaskStatus.FAILED, result.__dict__, result.error)
            return False

        # 3. 向量化
        try:
            # 创建向量化任务载荷
            vector_payload = VectorizePayload(
                document_id=payload.document_id,
                chunks=chunks,
                model=payload.model
            )

            # 模拟向量化
            time.sleep(2)
            dimension = 1536
            vectors = []

            import numpy as np
            for chunk in chunks:
                vector = np.random.rand(dimension).astype(np.float32).tolist()
                vectors.append(VectorInfo(
                    chunk_index=chunk.index,
                    vector=vector
                ))

            vector_result = VectorizeResult(
                document_id=payload.document_id,
                vectors=vectors,
                vector_count=len(vectors),
                model=payload.model or "default",
                dimension=dimension
            )

            result.vector_status = "completed"
            result.vector_count = len(vectors)
            result.dimension = dimension
            # 可选是否包含向量数据
            # result.vectors = vectors  # 如果需要在结果中包含向量数据
            logger.info(f"Text vectorization completed for {payload.document_id}: {len(vectors)} vectors")
        except Exception as e:
            result.vector_status = "failed"
            result.error = f"Vectorization failed: {str(e)}"
            logger.error(f"Text vectorization failed: {str(e)}")
            # 在向量化失败时，提前完成任务
            update_task_status(task, TaskStatus.FAILED, result.__dict__, result.error)
            return False

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)

        elapsed = time.time() - start_time
        logger.info(f"Complete document processing task {task_id} completed in {elapsed:.2f}s")
        return True

    except Exception as e:
        error_msg = f"Document processing failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False