import os
import pytest
from unittest.mock import patch, MagicMock
from app.worker.processor import DocumentProcessor
from app.models.model import Task, TaskType, TaskStatus
from app.document_processing.parser import DocumentParser
from app.document_processing.chunker import DocumentChunker, ChunkOptions
from app.document_processing.factory import create_parser, create_chunker


@pytest.fixture
def mock_embedder():
    """模拟嵌入模型"""
    embedder = MagicMock()
    embedder.embed.return_value = [0.1, 0.2, 0.3, 0.4]
    embedder.embed_batch.return_value = [[0.1, 0.2, 0.3, 0.4], [0.5, 0.6, 0.7, 0.8]]
    embedder.get_model_name.return_value = "test-model"
    return embedder


@pytest.fixture
def mock_parser():
    """模拟文档解析器"""
    parser = MagicMock(spec=DocumentParser)
    parser.parse.return_value = "This is test document content"
    parser.extract_title.return_value = "Test Document"
    parser.get_metadata.return_value = {"pages": 1}
    return parser


@pytest.fixture
def mock_chunker():
    """模拟文本分块器"""
    chunker = MagicMock(spec=DocumentChunker)
    chunker.chunk_text.return_value = [
        {"text": "Chunk 1", "index": 0, "metadata": {}},
        {"text": "Chunk 2", "index": 1, "metadata": {}}
    ]
    return chunker


@pytest.fixture
def processor(mock_embedder):
    """创建处理器实例"""
    with patch('app.worker.processor.get_default_embedder', return_value=mock_embedder):
        processor = DocumentProcessor()
        processor.embedder = mock_embedder
        return processor


def test_processor_initialization():
    """测试处理器初始化"""
    with patch('app.worker.processor.get_default_embedder') as mock_get_default:
        mock_embedder = MagicMock()
        mock_get_default.return_value = mock_embedder

        # 设置环境变量
        os.environ["DASHSCOPE_API_KEY"] = "test_key"
        os.environ["EMBEDDING_MODEL"] = "test-model"

        processor = DocumentProcessor()

        # 验证嵌入器
        assert processor.embedder == mock_embedder
        mock_get_default.assert_called_once()

        # 清理环境变量
        del os.environ["DASHSCOPE_API_KEY"]
        del os.environ["EMBEDDING_MODEL"]
        

@pytest.mark.parametrize("task_type", [
    TaskType.DOCUMENT_PARSE,
    TaskType.TEXT_CHUNK,
    TaskType.VECTORIZE,
    TaskType.PROCESS_COMPLETE
])
def test_process_task_types(processor, task_type):
    """测试处理不同类型的任务"""
    task = Task(
        id="test-task-id",
        type=task_type,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={"test": "data"}
    )

    # 为每种任务类型模拟相应的方法
    with patch.object(processor, 'parse_document', return_value=(True, {})) as mock_parse, \
            patch.object(processor, 'chunk_text', return_value=(True, {})) as mock_chunk, \
            patch.object(processor, 'vectorize_text', return_value=(True, {})) as mock_vectorize, \
            patch.object(processor, 'process_document', return_value=(True, {})) as mock_complete:

        success, result = processor.process_task(task) 
        assert success is True 

        # 验证调用了正确的处理方法
        if task_type == TaskType.DOCUMENT_PARSE:
            mock_parse.assert_called_once_with(task)
        elif task_type == TaskType.TEXT_CHUNK:
            mock_chunk.assert_called_once_with(task)
        elif task_type == TaskType.VECTORIZE:
            mock_vectorize.assert_called_once_with(task)
        elif task_type == TaskType.PROCESS_COMPLETE:
            mock_complete.assert_called_once_with(task)


def test_process_invalid_task(processor):
    """测试处理无效任务"""
    # 无类型的任务
    invalid_task = Task(
        id="invalid-task",
        type=None,
        document_id="test-doc-id",
        status=TaskStatus.PENDING
    )

    success, _ = processor.process_task(invalid_task) 
    assert success is False


def test_parse_document(processor, mock_parser):
    """测试文档解析处理功能"""
    task = Task(
        id="parse-task",
        type=TaskType.DOCUMENT_PARSE,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={
            "file_path": "/path/to/test.pdf",
            "file_name": "test.pdf"
        }
    )

    mock_parser.parse.return_value = "Test document content"
    mock_parser.extract_title.return_value = "Test Title"
    mock_parser.get_metadata.return_value = {"words": 100, "chars": 500}

    with patch('app.worker.processor.create_parser', return_value=mock_parser), \
            patch('app.worker.processor.get_file_from_minio', return_value=("/path/to/test.pdf", True)), \
            patch('os.path.exists', return_value=True):

        success, result = processor.parse_document(task)
        assert success is True
        mock_parser.parse.assert_called_once()
        mock_parser.extract_title.assert_called_once()
        
        # 验证结果格式
        assert "content" in result
        assert "document_id" in result
        assert "title" in result
        assert "meta" in result


def test_chunk_text(processor, mock_chunker):
    """测试文本分块处理功能"""
    task = Task(
        id="chunk-task",
        type=TaskType.TEXT_CHUNK,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-id",
            "content": "This is test content for chunking.",
            "chunk_size": 100,
            "overlap": 20,
            "split_type": "paragraph"
        }
    )

    with patch('app.worker.processor.create_chunker', return_value=mock_chunker):
        success, result = processor.chunk_text(task)

        assert success is True
        mock_chunker.chunk_text.assert_called_once_with(
            task.payload["content"], 
            {"document_id": "test-doc-id"}
        )
        
        # 验证结果格式
        assert "document_id" in result
        assert "chunks" in result
        assert "chunk_count" in result


def test_vectorize_text(processor, mock_embedder):
    """测试文本向量化处理功能"""
    chunks = [
        {"text": "Chunk 1", "index": 0},
        {"text": "Chunk 2", "index": 1}
    ]

    task = Task(
        id="vectorize-task",
        type=TaskType.VECTORIZE,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-id",
            "chunks": chunks,
            "model": "test-model"
        }
    )

    success, result = processor.vectorize_text(task)

    assert success is True
    mock_embedder.embed_batch.assert_called_once()
    
    # 验证结果格式
    assert "document_id" in result
    assert "vectors" in result
    assert "vector_count" in result
    assert "dimension" in result


def test_process_document(processor, mock_parser, mock_chunker):
    """测试完整文档处理流程"""
    task = Task(
        id="complete-task",
        type=TaskType.PROCESS_COMPLETE,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-id",
            "file_path": "/path/to/test.pdf",
            "file_name": "test.pdf",
            "file_type": "pdf",
            "chunk_size": 100,
            "overlap": 20,
            "split_type": "paragraph",
            "model": "test-model"
        }
    )

    with patch('app.worker.processor.process_file') as mock_process_file, \
            patch('app.worker.processor.get_file_from_minio', return_value=("/path/to/test.pdf", False)):

        # 模拟process_file的返回值
        mock_process_file.return_value = (
            "This is test document content",
            [{"text": "Chunk 1", "index": 0}, {"text": "Chunk 2", "index": 1}],
            {"title": "Test Document", "pages": 1}
        )

        # 使处理器的嵌入模型返回向量
        processor.embedder.embed_batch.return_value = [[0.1, 0.2], [0.3, 0.4]]

        success, result = processor.process_document(task)

        assert success is True
        mock_process_file.assert_called_once()
        processor.embedder.embed_batch.assert_called_once()
        
        # 验证结果格式
        assert "document_id" in result
        assert "chunk_count" in result
        assert "vector_count" in result
        assert "dimension" in result
        assert "parse_status" in result
        assert "chunk_status" in result
        assert "vector_status" in result
        assert "vectors" in result


def test_parse_document_error(processor):
    """测试文档解析错误处理"""
    task = Task(
        id="error-parse-task",
        type=TaskType.DOCUMENT_PARSE,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={
            "file_path": "/path/to/nonexistent.pdf",
            "file_name": "nonexistent.pdf"
        }
    )

    with patch('app.worker.processor.get_file_from_minio', side_effect=Exception("File not found")):
        success, result = processor.parse_document(task)

        assert success is False
        assert "error" in result
        assert "File not found" in result["error"]


def test_embedding_fallback_strategy():
    """测试嵌入回退策略"""
    # 测试场景1: 有通义千问API密钥 - 应使用通义千问
    with patch.dict(os.environ, {"DASHSCOPE_API_KEY": "fake-api-key"}):
        with patch('app.worker.processor.get_default_embedder') as mock_get_default:
            # 模拟默认嵌入器
            mock_embedder = MagicMock()
            mock_embedder.get_model_name.return_value = "text-embedding-v3"
            mock_get_default.return_value = mock_embedder

            processor = DocumentProcessor()

            # 验证调用了默认嵌入器
            mock_get_default.assert_called_once()
            assert processor.embedder == mock_embedder

    # 测试场景2: 无通义千问API密钥 - 应使用默认嵌入器
    with patch.dict(os.environ, {"DASHSCOPE_API_KEY": ""}):
        with patch('app.worker.processor.get_default_embedder') as mock_get_default:
            # 模拟默认嵌入器
            mock_embedder = MagicMock()
            mock_embedder.get_model_name.return_value = "all-MiniLM-L6-v2"
            mock_get_default.return_value = mock_embedder

            processor = DocumentProcessor()

            # 验证调用了默认嵌入器
            mock_get_default.assert_called_once()
            assert processor.embedder == mock_embedder


def test_embedder_error_handling(processor):
    """测试嵌入器错误处理"""
    chunks = [
        {"text": "Chunk 1", "index": 0},
        {"text": "Chunk 2", "index": 1}
    ]

    task = Task(
        id="vectorize-error-task",
        type=TaskType.VECTORIZE,
        document_id="test-doc-id",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-id",
            "chunks": chunks,
            "model": "test-model"
        }
    )

    # 模拟嵌入器错误
    processor.embedder.embed_batch.side_effect = Exception("Embedding error")

    success, result = processor.vectorize_text(task)

    assert success is False
    assert "error" in result
    assert "Embedding error" in result["error"]