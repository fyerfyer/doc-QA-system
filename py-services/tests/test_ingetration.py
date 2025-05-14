import os
import sys
import json
import time
import uuid
import pytest
import shutil
import tempfile
from pathlib import Path

# 确保能导入应用模块
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '..')))

import redis
from minio import Minio
from dotenv import load_dotenv

from app.models.model import (
    Task, TaskType, TaskStatus,
    DocumentParsePayload, TextChunkPayload, VectorizePayload
)
from app.parsers.factory import create_parser
from app.chunkers.splitter import TextSplitter
from app.embedders.factory import get_default_embedder
from app.worker.processor import DocumentProcessor
from app.utils.utils import setup_logger

# 加载环境变量
load_dotenv()

# 设置日志记录器
logger = setup_logger("INFO")

# 定义测试常量
TEST_REDIS_URL = os.environ.get("TEST_REDIS_URL", "redis://localhost:6379/0")
TEST_MINIO_URL = os.environ.get("TEST_MINIO_URL", "localhost:9000")
TEST_MINIO_ACCESS_KEY = os.environ.get("TEST_MINIO_ACCESS_KEY", "minioadmin")
TEST_MINIO_SECRET_KEY = os.environ.get("TEST_MINIO_SECRET_KEY", "minioadmin")
TEST_MINIO_BUCKET = "docqa-test"


class TestIntegration:
    @classmethod
    def setup_class(cls):
        """设置测试类的环境"""
        # 创建临时目录
        cls.temp_dir = tempfile.mkdtemp()
        logger.info(f"Created temporary directory: {cls.temp_dir}")

        # 连接Redis
        try:
            cls.redis_client = redis.from_url(TEST_REDIS_URL)
            cls.redis_client.ping()
            logger.info("Connected to Redis successfully")
        except Exception as e:
            pytest.fail(f"Failed to connect to Redis: {str(e)}")

        # 连接MinIO并创建测试桶
        try:
            secure = TEST_MINIO_URL.startswith("https://")
            host = TEST_MINIO_URL.replace("https://", "").replace("http://", "")

            cls.minio_client = Minio(
                host,
                access_key=TEST_MINIO_ACCESS_KEY,
                secret_key=TEST_MINIO_SECRET_KEY,
                secure=secure
            )

            # 检查并创建测试桶
            if not cls.minio_client.bucket_exists(TEST_MINIO_BUCKET):
                cls.minio_client.make_bucket(TEST_MINIO_BUCKET)
                logger.info(f"Created MinIO bucket: {TEST_MINIO_BUCKET}")
            else:
                logger.info(f"MinIO bucket already exists: {TEST_MINIO_BUCKET}")

        except Exception as e:
            pytest.fail(f"Failed to connect to MinIO: {str(e)}")

        # 创建测试文件
        cls.create_test_files()

    @classmethod
    def teardown_class(cls):
        """清理测试资源"""
        # 清理临时目录
        shutil.rmtree(cls.temp_dir)
        logger.info(f"Removed temporary directory: {cls.temp_dir}")

        # 清理Redis中的测试数据
        keys = cls.redis_client.keys("task:test:*")
        if keys:
            cls.redis_client.delete(*keys)
            logger.info(f"Cleaned up {len(keys)} test tasks from Redis")

        # 清理MinIO中的测试文件
        try:
            objects = cls.minio_client.list_objects(TEST_MINIO_BUCKET, prefix="test/")
            for obj in objects:
                cls.minio_client.remove_object(TEST_MINIO_BUCKET, obj.object_name)
            logger.info("Cleaned up test files from MinIO")
        except Exception as e:
            logger.warning(f"Error cleaning MinIO files: {str(e)}")

    @classmethod
    def create_test_files(cls):
        """创建测试文档文件"""
        # 创建简单文本文件
        text_path = os.path.join(cls.temp_dir, "sample.txt")
        with open(text_path, "w", encoding="utf-8") as f:
            f.write("This is a sample text file for testing.\n\n")
            f.write("It contains multiple paragraphs.\n\n")
            f.write("This is the third paragraph with some content.\n\n")
            f.write("This is the fourth paragraph to test chunking.")
        cls.text_file_path = text_path

        # 创建简单Markdown文件
        md_path = os.path.join(cls.temp_dir, "sample.md")
        with open(md_path, "w", encoding="utf-8") as f:
            f.write("# Test Markdown Document\n\n")
            f.write("This is a paragraph in a markdown file.\n\n")
            f.write("## Section 1\n\n")
            f.write("Content in section 1.\n\n")
            f.write("## Section 2\n\n")
            f.write("Content in section 2 with some more text.")
        cls.md_file_path = md_path

        # 上传到MinIO
        cls.upload_test_file_to_minio(text_path, "test/sample.txt")
        cls.upload_test_file_to_minio(md_path, "test/sample.md")

        logger.info("Test files created and uploaded to MinIO")

    @classmethod
    def upload_test_file_to_minio(cls, file_path, object_name):
        """上传测试文件到MinIO"""
        try:
            cls.minio_client.fput_object(
                TEST_MINIO_BUCKET,
                object_name,
                file_path
            )
            logger.info(f"Uploaded {file_path} to MinIO as {object_name}")
        except Exception as e:
            pytest.fail(f"Failed to upload test file to MinIO: {str(e)}")

    def test_document_parser(self):
        """测试文档解析器"""
        # 测试文本文件解析
        parser = create_parser(self.text_file_path)
        content = parser.parse(self.text_file_path)

        assert content is not None, "Parser should return content"
        assert isinstance(content, str), "Content should be a string"
        assert len(content) > 0, "Content should not be empty"
        assert "sample text file" in content, "Content should contain expected text"

        logger.info("Document parser test passed for text file")

        # 测试Markdown文件解析
        md_parser = create_parser(self.md_file_path)
        md_content = md_parser.parse(self.md_file_path)

        assert md_content is not None, "Parser should return content for Markdown"
        assert isinstance(md_content, str), "Content should be a string"
        assert len(md_content) > 0, "Content should not be empty"
        assert "Test Markdown Document" in md_content, "Content should contain markdown heading"

        logger.info("Document parser test passed for markdown file")

    def test_text_chunking(self):
        """测试文本分块"""
        text = """This is a test document for chunking.
        
        It has multiple paragraphs that should be separated properly.
        
        This is the third paragraph with some additional content.
        
        This is the fourth paragraph that will help test the chunking logic."""

        # 创建分块器
        splitter = TextSplitter()
        chunks = splitter.split(text)

        assert chunks is not None, "Chunks should not be None"
        assert len(chunks) > 0, "Should create at least one chunk"
        assert isinstance(chunks[0], dict), "Chunk should be a dictionary"
        assert "text" in chunks[0], "Chunk should have text field"
        assert len(chunks) >= 3, "Should create at least 3 chunks for this text"

        # 检查分块内容
        all_content = " ".join([c["text"] for c in chunks])
        assert "test document" in all_content, "All content should contain original text"
        assert "third paragraph" in all_content, "All content should contain third paragraph"

        logger.info(f"Text chunking test passed, created {len(chunks)} chunks")

    def test_embeddings(self):
        """测试文本嵌入功能"""
        # 使用真实的API密钥创建embedder
        embedder = get_default_embedder()

        # 测试单个文本嵌入
        text = "This is a test sentence for embedding."
        vector = embedder.embed(text)

        assert vector is not None, "Embedding should not be None"
        assert len(vector) > 0, "Embedding should have values"
        assert isinstance(vector[0], float), "Embedding should be float values"

        logger.info(f"Single text embedding test passed, vector dimension: {len(vector)}")

        # 测试批量文本嵌入
        texts = [
            "This is the first test sentence.",
            "This is the second test sentence.",
            "This is the third test sentence."
        ]

        vectors = embedder.embed_batch(texts)

        assert vectors is not None, "Batch embeddings should not be None"
        assert len(vectors) == len(texts), "Should return same number of vectors as input texts"
        assert all(len(v) == len(vectors[0]) for v in vectors), "All vectors should have same dimension"

        logger.info(f"Batch embedding test passed, created {len(vectors)} vectors")

    def test_document_processor(self):
        """测试文档处理器"""
        # 创建文档处理器
        processor = DocumentProcessor()

        # 创建解析任务
        doc_id = f"test-{uuid.uuid4()}"
        task_id = f"task:test:{uuid.uuid4()}"

        task = Task(
            id=task_id,
            type=TaskType.DOCUMENT_PARSE,
            document_id=doc_id,
            status=TaskStatus.PENDING,
            payload=DocumentParsePayload(
                file_path=self.text_file_path,
                file_name="sample.txt",
                file_type="txt"
            ).__dict__
        )

        # 保存任务到Redis
        self.redis_client.set(task_id, task.to_json())

        # 处理解析任务
        result = processor.process_task(task)
        assert result is True, "Task processing should succeed"

        # 从Redis获取更新后的任务
        updated_task_data = self.redis_client.get(task_id)
        updated_task = Task.from_json(updated_task_data)

        assert updated_task.status == TaskStatus.COMPLETED, "Task status should be COMPLETED"
        assert "content" in updated_task.result, "Result should contain content field"

        # 使用解析结果创建分块任务
        chunk_task_id = f"task:test:{uuid.uuid4()}"
        chunk_task = Task(
            id=chunk_task_id,
            type=TaskType.TEXT_CHUNK,
            document_id=doc_id,
            status=TaskStatus.PENDING,
            payload=TextChunkPayload(
                document_id=doc_id,
                content=updated_task.result["content"],
                chunk_size=500,
                overlap=100,
                split_type="paragraph"
            ).__dict__
        )

        # 保存分块任务到Redis
        self.redis_client.set(chunk_task_id, chunk_task.to_json())

        # 处理分块任务
        result = processor.process_task(chunk_task)
        assert result is True, "Chunk task processing should succeed"

        # 从Redis获取更新后的分块任务
        updated_chunk_task_data = self.redis_client.get(chunk_task_id)
        updated_chunk_task = Task.from_json(updated_chunk_task_data)

        assert updated_chunk_task.status == TaskStatus.COMPLETED, "Chunk task status should be COMPLETED"
        assert "chunks" in updated_chunk_task.result, "Result should contain chunks field"
        assert len(updated_chunk_task.result["chunks"]) > 0, "Should have created at least one chunk"

        # 使用分块结果创建向量化任务
        vector_task_id = f"task:test:{uuid.uuid4()}"
        vector_task = Task(
            id=vector_task_id,
            type=TaskType.VECTORIZE,
            document_id=doc_id,
            status=TaskStatus.PENDING,
            payload=VectorizePayload(
                document_id=doc_id,
                chunks=updated_chunk_task.result["chunks"],
                model="text-embedding-v3"
            ).__dict__
        )

        # 保存向量化任务到Redis
        self.redis_client.set(vector_task_id, vector_task.to_json())

        # 处理向量化任务
        result = processor.process_task(vector_task)
        assert result is True, "Vector task processing should succeed"

        # 从Redis获取更新后的向量化任务
        updated_vector_task_data = self.redis_client.get(vector_task_id)
        updated_vector_task = Task.from_json(updated_vector_task_data)

        assert updated_vector_task.status == TaskStatus.COMPLETED, "Vector task status should be COMPLETED"
        assert "vectors" in updated_vector_task.result, "Result should contain vectors field"
        assert len(updated_vector_task.result["vectors"]) > 0, "Should have created vectors"

        logger.info("Document processor end-to-end test passed successfully")

    def test_end_to_end_document_processing(self):
        """端到端测试文档处理流程"""
        # 创建文档处理器
        processor = DocumentProcessor()

        # 创建完整处理任务
        doc_id = f"test-{uuid.uuid4()}"
        task_id = f"task:test:{uuid.uuid4()}"

        task = Task(
            id=task_id,
            type=TaskType.PROCESS_COMPLETE,
            document_id=doc_id,
            status=TaskStatus.PENDING,
            payload={
                "document_id": doc_id,
                "file_path": self.md_file_path,
                "file_name": "sample.md",
                "file_type": "md",
                "chunk_size": 500,
                "overlap": 50,
                "split_type": "paragraph",
                "model": "text-embedding-v3"
            }
        )

        # 保存任务到Redis
        self.redis_client.set(task_id, task.to_json())

        # 处理任务
        result = processor.process_task(task)
        assert result is True, "Complete processing task should succeed"

        # 从Redis获取更新后的任务
        updated_task_data = self.redis_client.get(task_id)
        updated_task = Task.from_json(updated_task_data)

        assert updated_task.status == TaskStatus.COMPLETED, "Task status should be COMPLETED"
        assert "parse_status" in updated_task.result, "Result should contain parse_status"
        assert "chunk_status" in updated_task.result, "Result should contain chunk_status"
        assert "vector_status" in updated_task.result, "Result should contain vector_status"
        assert updated_task.result["parse_status"] == "completed", "Parse status should be completed"
        assert updated_task.result["chunk_status"] == "completed", "Chunk status should be completed"
        assert updated_task.result["vector_status"] == "completed", "Vector status should be completed"
        assert "vectors" in updated_task.result, "Should include vectors in result"

        logger.info("End-to-end document processing test passed successfully")