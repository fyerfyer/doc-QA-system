import json
import unittest
from datetime import datetime, timedelta
from app.models.model import (
    Task, TaskType, TaskStatus,
    DocumentParsePayload, DocumentParseResult,
    TextChunkPayload, TextChunkResult, ChunkInfo,
    VectorizePayload, VectorizeResult, VectorInfo,
    ProcessCompletePayload, ProcessCompleteResult,
    TaskCallback
)

class TestModels(unittest.TestCase):
    """测试模型类的基本功能"""

    def test_task_creation(self):
        """测试任务创建功能"""
        task = Task(
            id="test-task-1",
            type=TaskType.DOCUMENT_PARSE,
            document_id="doc-123",
            status=TaskStatus.PENDING
        )

        # 验证默认值
        self.assertEqual(task.id, "test-task-1")
        self.assertEqual(task.type, TaskType.DOCUMENT_PARSE)
        self.assertEqual(task.document_id, "doc-123")
        self.assertEqual(task.status, TaskStatus.PENDING)
        self.assertEqual(task.error, "")
        self.assertEqual(task.attempts, 0)
        self.assertEqual(task.max_retries, 3)
        self.assertIsNone(task.started_at)
        self.assertIsNone(task.completed_at)
        self.assertIsInstance(task.created_at, datetime)
        self.assertIsInstance(task.updated_at, datetime)

    def test_task_json_serialization(self):
        """测试任务JSON序列化功能"""
        # 创建任务并设置一些字段
        now = datetime.now()
        started_at = now - timedelta(minutes=5)
        task = Task(
            id="test-task-2",
            type=TaskType.TEXT_CHUNK,
            document_id="doc-456",
            status=TaskStatus.PROCESSING,
            payload={"content": "test content"},
            error="",
            created_at=now,
            updated_at=now,
            started_at=started_at,
            attempts=1
        )

        # 序列化为JSON
        json_data = task.to_json()
        self.assertIsInstance(json_data, str)

        # 反序列化JSON数据
        parsed_data = json.loads(json_data)
        self.assertEqual(parsed_data["id"], "test-task-2")
        self.assertEqual(parsed_data["type"], "text_chunk")
        self.assertEqual(parsed_data["document_id"], "doc-456")
        self.assertEqual(parsed_data["status"], "processing")
        self.assertEqual(parsed_data["attempts"], 1)
        self.assertEqual(parsed_data["payload"], {"content": "test content"})

        # 验证日期字段已正确序列化
        self.assertIn("created_at", parsed_data)
        self.assertIn("updated_at", parsed_data)
        self.assertIn("started_at", parsed_data)

    def test_task_json_deserialization(self):
        """测试任务JSON反序列化功能"""
        # 创建JSON数据
        json_data = json.dumps({
            "id": "task-123",
            "type": "vectorize",
            "document_id": "doc-789",
            "status": "completed",
            "payload": {"chunks": [{"text": "chunk1", "index": 0}]},
            "result": {"vectors": [{"chunk_index": 0, "vector": [0.1, 0.2, 0.3]}]},
            "error": "",
            "created_at": "2023-05-15T10:30:00",
            "updated_at": "2023-05-15T10:35:00",
            "started_at": "2023-05-15T10:31:00",
            "completed_at": "2023-05-15T10:35:00",
            "attempts": 1,
            "max_retries": 3
        })

        # 反序列化为Task对象
        task = Task.from_json(json_data)

        # 验证字段
        self.assertEqual(task.id, "task-123")
        self.assertEqual(task.type, TaskType.VECTORIZE)
        self.assertEqual(task.document_id, "doc-789")
        self.assertEqual(task.status, TaskStatus.COMPLETED)
        self.assertEqual(task.attempts, 1)
        self.assertEqual(task.max_retries, 3)

        # 验证日期字段
        self.assertIsInstance(task.created_at, datetime)
        self.assertIsInstance(task.updated_at, datetime)
        self.assertIsInstance(task.started_at, datetime)
        self.assertIsInstance(task.completed_at, datetime)

        # 验证复杂字段
        self.assertEqual(task.payload, {"chunks": [{"text": "chunk1", "index": 0}]})
        self.assertEqual(task.result, {"vectors": [{"chunk_index": 0, "vector": [0.1, 0.2, 0.3]}]})

    def test_document_parse_payload(self):
        """测试DocumentParsePayload类"""
        payload = DocumentParsePayload(
            file_path="/data/files/doc1.pdf",
            file_name="doc1.pdf",
            file_type="pdf",
            metadata={"author": "Test User", "category": "Test"}
        )

        self.assertEqual(payload.file_path, "/data/files/doc1.pdf")
        self.assertEqual(payload.file_name, "doc1.pdf")
        self.assertEqual(payload.file_type, "pdf")
        self.assertEqual(payload.metadata, {"author": "Test User", "category": "Test"})

    def test_document_parse_result(self):
        """测试DocumentParseResult类"""
        result = DocumentParseResult(
            content="This is test content",
            title="Test Document",
            meta={"pages": "5"},
            pages=5,
            words=10,
            chars=50
        )

        self.assertEqual(result.content, "This is test content")
        self.assertEqual(result.title, "Test Document")
        self.assertEqual(result.meta, {"pages": "5"})
        self.assertEqual(result.pages, 5)
        self.assertEqual(result.words, 10)
        self.assertEqual(result.chars, 50)
        self.assertEqual(result.error, "")

    def test_chunk_info(self):
        """测试ChunkInfo类"""
        chunk = ChunkInfo(text="This is a chunk", index=2)

        self.assertEqual(chunk.text, "This is a chunk")
        self.assertEqual(chunk.index, 2)

    def test_text_chunk_payload(self):
        """测试TextChunkPayload类"""
        payload = TextChunkPayload(
            document_id="doc-123",
            content="This is the content to be chunked",
            chunk_size=100,
            overlap=20,
            split_type="text"
        )

        self.assertEqual(payload.document_id, "doc-123")
        self.assertEqual(payload.content, "This is the content to be chunked")
        self.assertEqual(payload.chunk_size, 100)
        self.assertEqual(payload.overlap, 20)
        self.assertEqual(payload.split_type, "text")

    def test_text_chunk_result(self):
        """测试TextChunkResult类"""
        chunks = [
            ChunkInfo(text="Chunk 1", index=0),
            ChunkInfo(text="Chunk 2", index=1)
        ]

        result = TextChunkResult(
            document_id="doc-123",
            chunks=chunks,
            chunk_count=2
        )

        self.assertEqual(result.document_id, "doc-123")
        self.assertEqual(len(result.chunks), 2)
        self.assertEqual(result.chunk_count, 2)
        self.assertEqual(result.error, "")
        self.assertEqual(result.chunks[0].text, "Chunk 1")
        self.assertEqual(result.chunks[1].text, "Chunk 2")

    def test_vectorize_payload(self):
        """测试VectorizePayload类"""
        chunks = [
            ChunkInfo(text="Chunk 1", index=0),
            ChunkInfo(text="Chunk 2", index=1)
        ]

        payload = VectorizePayload(
            document_id="doc-123",
            chunks=chunks,
            model="ada"
        )

        self.assertEqual(payload.document_id, "doc-123")
        self.assertEqual(len(payload.chunks), 2)
        self.assertEqual(payload.model, "ada")

    def test_vector_info(self):
        """测试VectorInfo类"""
        vector = VectorInfo(
            chunk_index=1,
            vector=[0.1, 0.2, 0.3, 0.4]
        )

        self.assertEqual(vector.chunk_index, 1)
        self.assertEqual(len(vector.vector), 4)
        self.assertEqual(vector.vector[0], 0.1)

    def test_vectorize_result(self):
        """测试VectorizeResult类"""
        vectors = [
            VectorInfo(chunk_index=0, vector=[0.1, 0.2]),
            VectorInfo(chunk_index=1, vector=[0.3, 0.4])
        ]

        result = VectorizeResult(
            document_id="doc-123",
            vectors=vectors,
            vector_count=2,
            model="ada",
            dimension=2
        )

        self.assertEqual(result.document_id, "doc-123")
        self.assertEqual(len(result.vectors), 2)
        self.assertEqual(result.vector_count, 2)
        self.assertEqual(result.model, "ada")
        self.assertEqual(result.dimension, 2)
        self.assertEqual(result.error, "")

    def test_process_complete_payload(self):
        """测试ProcessCompletePayload类"""
        payload = ProcessCompletePayload(
            document_id="doc-123",
            file_path="/data/files/doc1.pdf",
            file_name="doc1.pdf",
            file_type="pdf",
            chunk_size=100,
            overlap=20,
            split_type="text",
            model="ada",
            metadata={"author": "Test User"}
        )

        self.assertEqual(payload.document_id, "doc-123")
        self.assertEqual(payload.file_path, "/data/files/doc1.pdf")
        self.assertEqual(payload.file_name, "doc1.pdf")
        self.assertEqual(payload.file_type, "pdf")
        self.assertEqual(payload.chunk_size, 100)
        self.assertEqual(payload.overlap, 20)
        self.assertEqual(payload.split_type, "text")
        self.assertEqual(payload.model, "ada")
        self.assertEqual(payload.metadata, {"author": "Test User"})

    def test_process_complete_result(self):
        """测试ProcessCompleteResult类"""
        result = ProcessCompleteResult(
            document_id="doc-123",
            chunk_count=5,
            vector_count=5,
            dimension=1536,
            parse_status="completed",
            chunk_status="completed",
            vector_status="completed"
        )

        self.assertEqual(result.document_id, "doc-123")
        self.assertEqual(result.chunk_count, 5)
        self.assertEqual(result.vector_count, 5)
        self.assertEqual(result.dimension, 1536)
        self.assertEqual(result.parse_status, "completed")
        self.assertEqual(result.chunk_status, "completed")
        self.assertEqual(result.vector_status, "completed")
        self.assertEqual(result.error, "")
        self.assertEqual(len(result.vectors), 0)  # 默认为空列表

    def test_task_callback(self):
        """测试TaskCallback类"""
        now = datetime.now()
        callback = TaskCallback(
            task_id="task-123",
            document_id="doc-123",
            status=TaskStatus.COMPLETED,
            type=TaskType.DOCUMENT_PARSE,
            result={"content": "Parsed content"},
            error="",
            timestamp=now
        )

        self.assertEqual(callback.task_id, "task-123")
        self.assertEqual(callback.document_id, "doc-123")
        self.assertEqual(callback.status, TaskStatus.COMPLETED)
        self.assertEqual(callback.type, TaskType.DOCUMENT_PARSE)
        self.assertEqual(callback.result, {"content": "Parsed content"})
        self.assertEqual(callback.error, "")
        self.assertEqual(callback.timestamp, now)