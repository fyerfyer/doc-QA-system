import unittest
from unittest.mock import patch, MagicMock
import json
import os
from typing import Dict, Any

from worker.main import Worker, TASK_STATUS_COMPLETED, TASK_STATUS_FAILED
from worker.main import TASK_TYPE_DOCUMENT_PROCESS, TASK_TYPE_DOCUMENT_DELETE, TASK_TYPE_EMBEDDING_GENERATE
from dotenv import load_dotenv


class TestWorker(unittest.TestCase):
    """工作进程集成测试类"""

    def setUp(self):
        """测试前初始化"""
        # 加载环境变量 - 先尝试项目根目录
        dotenv_path = os.path.join(os.path.dirname(__file__), '..', '..', '.env.example')
        if os.path.exists(dotenv_path):
            load_dotenv(dotenv_path)
            print(f"Loaded environment from {dotenv_path}")
        else:
            # 尝试当前目录
            load_dotenv()
            print("Loaded environment from current directory")

        # 修正API密钥环境变量名称
        if 'DASHBOARD_API_KEY' in os.environ and not 'DASHSCOPE_API_KEY' in os.environ:
            os.environ['DASHSCOPE_API_KEY'] = os.environ['DASHBOARD_API_KEY']

        # 确保dashscope API也直接设置了key
        api_key = os.environ.get("DASHSCOPE_API_KEY")
        if api_key:
            import dashscope
            dashscope.api_key = api_key
            print(f"Set dashscope.api_key directly with value from environment")
        else:
            print("WARNING: No DASHSCOPE_API_KEY found in environment")

        # 模拟Redis客户端
        self.redis_patcher = patch('worker.main.redis.from_url')
        self.mock_redis = self.redis_patcher.start()

        # 创建模拟的Redis客户端
        self.mock_redis_client = MagicMock()
        self.mock_redis.return_value = self.mock_redis_client

        # 模拟存储服务
        self.storage_patcher = patch('worker.main.Storage')
        self.mock_storage_class = self.storage_patcher.start()
        self.mock_storage = MagicMock()
        self.mock_storage_class.return_value = self.mock_storage

        # 模拟文档解析器
        self.parser_patcher = patch('worker.main.DocumentParser')
        self.mock_parser_class = self.parser_patcher.start()
        self.mock_parser = MagicMock()
        self.mock_parser_class.return_value = self.mock_parser

        # 模拟文本分块器
        self.chunker_patcher = patch('worker.main.create_chunker')
        self.mock_chunker_func = self.chunker_patcher.start()
        self.mock_chunker = MagicMock()
        self.mock_chunker_func.return_value = self.mock_chunker

        # 创建Worker实例
        self.worker = Worker(redis_url="redis://mock:6379/1", poll_interval=0.1)

        # 测试任务相关数据
        self.test_task_id = "test-task-123"
        self.test_file_id = "test-file-456"

    def tearDown(self):
        """测试后清理"""
        self.redis_patcher.stop()
        self.storage_patcher.stop()
        self.parser_patcher.stop()
        self.chunker_patcher.stop()

    def _create_test_task(self, task_type: str, **kwargs) -> Dict[str, Any]:
        """创建测试任务的辅助方法"""
        task_data = {
            "id": self.test_task_id,
            "type": task_type,
            "status": "pending",
            "created_at": "2023-05-09T10:00:00",
            "updated_at": "2023-05-09T10:00:00",
        }
        task_data.update(kwargs)
        return task_data

    def test_fetch_next_task(self):
        """测试获取下一个任务功能"""
        # 模拟Redis返回任务ID
        self.mock_redis_client.rpop.return_value = self.test_task_id.encode('utf-8')

        # 模拟Redis返回任务数据
        task_data = self._create_test_task(TASK_TYPE_DOCUMENT_PROCESS, file_id=self.test_file_id, file_path="/test/path.txt")
        self.mock_redis_client.get.return_value = json.dumps(task_data).encode('utf-8')

        # 执行测试
        task = self.worker._fetch_next_task()

        # 验证结果
        self.assertIsNotNone(task)
        self.assertEqual(task["id"], self.test_task_id)
        self.assertEqual(task["type"], TASK_TYPE_DOCUMENT_PROCESS)
        self.assertEqual(task["file_id"], self.test_file_id)

    def test_fetch_next_task_no_tasks(self):
        """测试队列为空时的行为"""
        # 模拟Redis没有返回任务
        self.mock_redis_client.rpop.return_value = None

        # 执行测试
        task = self.worker._fetch_next_task()

        # 验证结果
        self.assertIsNone(task)

    @patch('worker.main.time.sleep', return_value=None)  # 避免测试中等待
    def test_process_document_task(self, mock_sleep):
        """测试处理文档任务"""
        # 模拟存储服务返回文件内容
        self.mock_storage.get_file.return_value = b"Test document content"

        # 模拟解析器返回文本
        self.mock_parser._parse_text.return_value = "Test document content"

        # 模拟分块器返回文本块
        mock_chunk = MagicMock()
        mock_chunk.text = "Test document content"
        mock_chunk.index = 0
        self.mock_chunker.split.return_value = [mock_chunk]

        # 模拟嵌入生成器
        with patch('worker.main.create_embedder') as mock_embedder_func:
            mock_embedder = MagicMock()
            mock_embedder.embed_text.return_value = [0.1, 0.2, 0.3]
            mock_embedder_func.return_value = mock_embedder

            # 重新创建Worker实例以使用模拟的嵌入生成器
            self.worker = Worker(redis_url="redis://mock:6379/1", poll_interval=0.1)

            # 创建文档处理任务
            task = self._create_test_task(TASK_TYPE_DOCUMENT_PROCESS, file_id=self.test_file_id, file_path="/test/path.txt")

            # 执行测试
            success, result = self.worker._process_task(task)

            # 验证结果
            self.assertTrue(success)
            self.assertEqual(result["segment_count"], 1)
            self.assertEqual(len(result["segments"]), 1)
            self.assertEqual(result["segments"][0]["file_id"], self.test_file_id)
            self.assertEqual(result["segments"][0]["text"], "Test document content")

            # 验证调用
            self.mock_storage.get_file.assert_called_with("/test/path.txt")
            self.mock_parser._parse_text.assert_called_once()
            self.mock_chunker.split.assert_called_once()
            mock_embedder.embed_text.assert_called_once()

    def test_document_delete_task(self):
        """测试文档删除任务"""
        # 模拟存储服务删除文件
        self.mock_storage.delete_file.return_value = True

        # 创建文档删除任务
        task = self._create_test_task(TASK_TYPE_DOCUMENT_DELETE, file_id=self.test_file_id)

        # 执行测试
        success, result = self.worker._process_task(task)

        # 验证结果
        self.assertTrue(success)
        self.assertIn("deleted successfully", result["message"])

        # 验证调用
        self.mock_storage.delete_file.assert_called_with(self.test_file_id)

    def test_embedding_generate_task(self):
        """测试嵌入向量生成任务"""
        # 准备测试数据
        test_texts = ["Text 1", "Text 2", "Text 3"]

        # 模拟嵌入生成器
        with patch('worker.main.create_embedder') as mock_embedder_func:
            mock_embedder = MagicMock()

            # 使用side_effect列表指定每次调用的返回值
            mock_embedder.embed_batch.side_effect = [
                [[0.1, 0.2], [0.3, 0.4]],  # 第一次调用返回值
                [[0.5, 0.6]]               # 第二次调用返回值
            ]
            mock_embedder_func.return_value = mock_embedder

            # 重新创建Worker实例以使用模拟的嵌入生成器
            self.worker = Worker(redis_url="redis://mock:6379/1", poll_interval=0.1)

            # 创建嵌入生成任务
            task = self._create_test_task(TASK_TYPE_EMBEDDING_GENERATE, texts=test_texts, batch_size=2)

            # 执行测试
            success, result = self.worker._process_task(task)

            # 验证结果
            self.assertTrue(success)
            self.assertEqual(result["count"], 3)
            self.assertEqual(result["embeddings"], [[0.1, 0.2], [0.3, 0.4], [0.5, 0.6]])

    def test_real_embedding_api(self):
        """测试真实API调用（需要有效的API密钥）"""
        # 检查是否有API密钥
        api_key = os.environ.get("DASHSCOPE_API_KEY")
        if not api_key:
            # 尝试再次从.env.example加载
            dotenv_path = os.path.join(os.path.dirname(__file__), '..', '..', '.env.example')
            if os.path.exists(dotenv_path):
                load_dotenv(dotenv_path)
                api_key = os.environ.get("DASHSCOPE_API_KEY")
                # 显式设置dashscope的api key
                if api_key:
                    import dashscope
                    dashscope.api_key = api_key
                    os.environ["DASHSCOPE_API_KEY"] = api_key
                    print(f"Explicitly loaded API key for test_real_embedding_api")

        # 仍然没找到API key，跳过测试
        if not api_key:
            self.skipTest("API key not found in environment variables, skipping real API test")

        # 关闭模拟
        if hasattr(self, 'embedder_patcher'):
            self.embedder_patcher.stop()

        # 实际需要使用嵌入的Worker
        self.worker = Worker(redis_url="redis://mock:6379/1", poll_interval=0.1)

        # 创建简单的嵌入任务
        test_text = "这是测试文本，用于验证API调用。This is a test text for API validation."
        task = self._create_test_task(TASK_TYPE_EMBEDDING_GENERATE, texts=[test_text], batch_size=1)

        try:
            # 执行测试
            success, result = self.worker._process_task(task)

            # 验证结果
            self.assertTrue(success)
            self.assertEqual(result["count"], 1)
            self.assertTrue(isinstance(result["embeddings"], list))
            self.assertTrue(isinstance(result["embeddings"][0], list))
            self.assertTrue(len(result["embeddings"][0]) > 0)
            print(f"Real API embedding dimension: {len(result['embeddings'][0])}")
        except Exception as e:
            self.fail(f"Real API call failed: {str(e)}")

    def test_unknown_task_type(self):
        """测试未知任务类型处理"""
        # 创建未知类型任务
        task = self._create_test_task("unknown.task.type")

        # 执行测试
        success, error = self.worker._process_task(task)

        # 验证结果
        self.assertFalse(success)
        self.assertIn("Unknown task type", error)

    def test_task_error_handling(self):
        """测试任务处理错误处理"""
        # 模拟存储服务抛出异常
        self.mock_storage.get_file.side_effect = Exception("Test storage error")

        # 创建任务
        task = self._create_test_task(TASK_TYPE_DOCUMENT_PROCESS, file_id=self.test_file_id, file_path="/test/path.txt")

        # 执行测试
        success, error = self.worker._process_task(task)

        # 验证结果
        self.assertFalse(success)
        self.assertIn("Test storage error", error)

    def test_mark_task_completed(self):
        """测试标记任务为已完成"""
        # 模拟Redis任务数据
        task_data = self._create_test_task(TASK_TYPE_DOCUMENT_PROCESS, file_id=self.test_file_id)
        self.mock_redis_client.get.return_value = json.dumps(task_data).encode('utf-8')

        # 执行测试
        test_result = {"segment_count": 3}
        self.worker._mark_task_completed(self.test_task_id, test_result)

        # 验证Redis set调用
        self.mock_redis_client.set.assert_called_once()

        # 验证更新的任务内容
        call_args = self.mock_redis_client.set.call_args[0]
        updated_task_json = call_args[1]
        updated_task = json.loads(updated_task_json)

        # 验证任务状态和结果
        self.assertEqual(updated_task["status"], TASK_STATUS_COMPLETED)
        self.assertEqual(updated_task["result"], test_result)

        # 验证添加到已完成列表
        self.mock_redis_client.lpush.assert_called_once()

    def test_mark_task_failed(self):
        """测试标记任务为失败"""
        # 模拟Redis任务数据
        task_data = self._create_test_task(TASK_TYPE_DOCUMENT_PROCESS, file_id=self.test_file_id)
        self.mock_redis_client.get.return_value = json.dumps(task_data).encode('utf-8')

        # 执行测试
        error_msg = "Test error message"
        self.worker._mark_task_failed(self.test_task_id, error_msg)

        # 验证Redis set调用
        self.mock_redis_client.set.assert_called_once()

        # 验证更新的任务内容
        call_args = self.mock_redis_client.set.call_args[0]
        updated_task_json = call_args[1]
        updated_task = json.loads(updated_task_json)

        # 验证任务状态和错误信息
        self.assertEqual(updated_task["status"], TASK_STATUS_FAILED)
        self.assertEqual(updated_task["error"], error_msg)

    @patch('worker.main.time.sleep', return_value=None)
    def test_task_workflow(self, mock_sleep):
        """测试完整的任务处理工作流程"""
        # 模拟Redis任务数据
        task_data = self._create_test_task(TASK_TYPE_DOCUMENT_PROCESS, file_id=self.test_file_id, file_path="/test/path.txt")

        # 模拟Redis操作
        self.mock_redis_client.rpop.return_value = self.test_task_id.encode('utf-8')
        self.mock_redis_client.get.return_value = json.dumps(task_data).encode('utf-8')

        # 模拟文件内容和解析
        self.mock_storage.get_file.return_value = b"Test document content"
        self.mock_parser._parse_text.return_value = "Test document content"

        # 模拟分块结果
        mock_chunk = MagicMock()
        mock_chunk.text = "Test document content"
        mock_chunk.index = 0
        self.mock_chunker.split.return_value = [mock_chunk]

        # 模拟嵌入服务
        with patch('worker.main.create_embedder') as mock_embedder_func:
            mock_embedder = MagicMock()
            mock_embedder.embed_text.return_value = [0.1, 0.2, 0.3]
            mock_embedder_func.return_value = mock_embedder

            # 重新创建Worker实例
            self.worker = Worker(redis_url="redis://mock:6379/1", poll_interval=0.1)

            # 模拟工作循环的一次执行
            self.worker.running = True

            # 手动执行工作流程各步骤
            task = self.worker._fetch_next_task()
            self.worker._mark_task_processing(task['id'])
            success, result_or_error = self.worker._process_task(task)

            if success:
                self.worker._mark_task_completed(task['id'], result_or_error)
            else:
                self.worker._mark_task_failed(task['id'], str(result_or_error))

            # 发布通知
            self.worker._publish_task_update(task, success, result_or_error)

            # 验证整个流程的调用
            self.mock_redis_client.rpop.assert_called_once()
            self.mock_redis_client.get.assert_called()
            self.mock_storage.get_file.assert_called_once()
            self.mock_parser._parse_text.assert_called_once()
            self.mock_chunker.split.assert_called_once()
            mock_embedder.embed_text.assert_called_once()
            self.mock_redis_client.set.assert_called()
            self.mock_redis_client.publish.assert_called_once()


if __name__ == '__main__':
    unittest.main()