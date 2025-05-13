import os
import pytest
from unittest.mock import patch, MagicMock
from app.worker.processor import DocumentProcessor
from app.models.model import Task, TaskType, TaskStatus
from app.parsers.base import BaseParser
from app.chunkers.splitter import TextSplitter


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
    parser = MagicMock(spec=BaseParser)
    parser.parse.return_value = "This is test document content"
    parser.extract_title.return_value = "Test Document"
    parser.get_metadata.return_value = {"pages": 1}
    return parser


@pytest.fixture
def mock_splitter():
    """模拟文本分块器"""
    splitter = MagicMock(spec=TextSplitter)
    splitter.split.return_value = [
        {"text": "Chunk 1", "index": 0},
        {"text": "Chunk 2", "index": 1}
    ]
    return splitter


@pytest.fixture
def processor(mock_embedder):
    """创建处理器实例"""
    with patch('app.worker.processor.create_embedder', return_value=mock_embedder):
        processor = DocumentProcessor()
        processor.embedder = mock_embedder
        return processor


def test_processor_initialization():
    """测试处理器初始化"""
    with patch('app.worker.processor.create_embedder') as mock_create_embedder:
        mock_embedder = MagicMock()
        mock_create_embedder.return_value = mock_embedder

        # 设置环境变量
        os.environ["DASHSCOPE_API_KEY"] = "test_key"
        os.environ["EMBEDDING_MODEL"] = "test-model"

        processor = DocumentProcessor()

        assert processor.dashscope_api_key == "test_key"
        assert processor.embedding_model == "test-model"
        assert processor.embedder == mock_embedder

        # 清理环境变量
        del os.environ["DASHSCOPE_API_KEY"]
        del os.environ["EMBEDDING_MODEL"]


def test_processor_init_without_api_key():
    """测试没有API密钥时的初始化"""
    with patch('app.worker.processor.create_embedder') as mock_create_embedder, \
            patch('app.worker.processor.get_default_embedder') as mock_get_default:

        # 确保环境变量不存在
        if "DASHSCOPE_API_KEY" in os.environ:
            del os.environ["DASHSCOPE_API_KEY"]

        mock_default_embedder = MagicMock()
        mock_get_default.return_value = mock_default_embedder

        processor = DocumentProcessor()

        # 应该使用默认嵌入器
        mock_create_embedder.assert_not_called()
        mock_get_default.assert_called_once()
        assert processor.embedder == mock_default_embedder


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
    with patch.object(processor, 'process_parse_document', return_value=True) as mock_parse, \
            patch.object(processor, 'process_chunk_text', return_value=True) as mock_chunk, \
            patch.object(processor, 'process_vectorize_text', return_value=True) as mock_vectorize, \
            patch.object(processor, 'process_complete', return_value=True) as mock_complete:

        result = processor.process_task(task)

        assert result is True

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

    result = processor.process_task(invalid_task)
    assert result is False


def test_process_parse_document(processor, mock_parser):
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

    with patch('app.worker.processor.detect_content_type', return_value="application/pdf"), \
            patch('app.worker.processor.create_parser', return_value=mock_parser), \
            patch('app.worker.processor.update_task_status') as mock_update_status:

        result = processor.process_parse_document(task)

        assert result is True
        mock_parser.parse.assert_called_once_with("/path/to/test.pdf")
        mock_parser.extract_title.assert_called_once()

        # 验证任务状态更新
        mock_update_status.assert_called()
        # 首先更新为处理中
        assert mock_update_status.call_args_list[0].args[1] == TaskStatus.PROCESSING
        # 然后更新为已完成
        assert mock_update_status.call_args_list[1].args[1] == TaskStatus.COMPLETED


def test_process_chunk_text(processor):
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

    with patch('app.worker.processor.split_text') as mock_split_text, \
            patch('app.worker.processor.update_task_status') as mock_update_status:

        # 模拟分块结果
        mock_split_text.return_value = [
            {"text": "Chunk 1", "index": 0},
            {"text": "Chunk 2", "index": 1}
        ]

        result = processor.process_chunk_text(task)

        assert result is True
        mock_split_text.assert_called_once()

        # 验证调用参数
        call_args = mock_split_text.call_args[1]
        assert call_args["chunk_size"] == 100
        assert call_args["chunk_overlap"] == 20
        assert call_args["split_type"] == "paragraph"

        # 验证任务状态更新
        mock_update_status.assert_called()
        assert mock_update_status.call_args_list[0].args[1] == TaskStatus.PROCESSING
        assert mock_update_status.call_args_list[1].args[1] == TaskStatus.COMPLETED


def test_process_vectorize_text(processor, mock_embedder):
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

    with patch('app.worker.processor.update_task_status') as mock_update_status:
        mock_update_status.return_value = True
        result = processor.process_vectorize_text(task)

        assert result is True
        mock_embedder.embed_batch.assert_called_once()

        # 验证任务状态更新
        assert mock_update_status.call_count >= 2
        assert mock_update_status.call_args_list[0].args[1] == TaskStatus.PROCESSING
        assert mock_update_status.call_args_list[1].args[1] == TaskStatus.COMPLETED

        # 检查是否提供了结果参数，但不访问具体索引位置
        # 而是检查关键字参数
        complete_call_kwargs = mock_update_status.call_args_list[1].kwargs
        assert "result" in complete_call_kwargs


def test_process_complete(processor, mock_parser):
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

    with patch('app.worker.processor.detect_content_type', return_value="application/pdf"), \
            patch('app.worker.processor.create_parser', return_value=mock_parser), \
            patch('app.worker.processor.split_text') as mock_split_text, \
            patch('app.worker.processor.update_task_status') as mock_update_status:

        # 模拟分块结果
        mock_split_text.return_value = [
            {"text": "Chunk 1", "index": 0},
            {"text": "Chunk 2", "index": 1}
        ]

        # 使处理器的嵌入模型返回向量
        processor.embedder.embed_batch.return_value = [[0.1, 0.2], [0.3, 0.4]]

        mock_update_status.return_value = True
        result = processor.process_complete(task)

        assert result is True
        mock_parser.parse.assert_called_once()
        mock_split_text.assert_called_once()
        processor.embedder.embed_batch.assert_called_once()

        # 验证任务状态更新
        assert mock_update_status.call_count >= 2
        assert mock_update_status.call_args_list[0].args[1] == TaskStatus.PROCESSING
        assert mock_update_status.call_args_list[1].args[1] == TaskStatus.COMPLETED

        # 验证结果包含所有阶段的状态（使用关键字参数而非位置参数）
        complete_call_kwargs = mock_update_status.call_args_list[1].kwargs
        assert "result" in complete_call_kwargs


def test_process_parse_document_error(processor):
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

    with patch('app.worker.processor.detect_content_type') as mock_detect, \
            patch('app.worker.processor.update_task_status') as mock_update_status:

        # 模拟解析错误
        mock_detect.side_effect = Exception("File not found")

        mock_update_status.return_value = True
        result = processor.process_parse_document(task)

        assert result is False

        # 验证错误状态更新
        assert mock_update_status.call_count >= 2
        assert mock_update_status.call_args_list[0].args[1] == TaskStatus.PROCESSING
        assert mock_update_status.call_args_list[1].args[1] == TaskStatus.FAILED

        # 检查错误消息在关键字参数而非位置参数中
        failed_call_kwargs = mock_update_status.call_args_list[1].kwargs
        assert "error" in failed_call_kwargs
        assert "File not found" in failed_call_kwargs["error"]


def test_embedding_fallback_strategy():
    """测试嵌入回退策略"""
    # 测试场景1: 有通义千问API密钥 - 应使用通义千问
    with patch.dict(os.environ, {"DASHSCOPE_API_KEY": "fake-api-key"}):
        with patch('app.worker.processor.create_embedder') as mock_create_embedder:
            # 模拟通义千问API可用
            mock_tongyi_embedder = MagicMock()
            mock_tongyi_embedder.get_model_name.return_value = "text-embedding-v3"
            mock_create_embedder.return_value = mock_tongyi_embedder

            processor = DocumentProcessor()

            # 验证调用了通义千问嵌入器
            mock_create_embedder.assert_called_with(
                "tongyi",
                api_key="fake-api-key",
                model_name=processor.embedding_model
            )
            assert processor.embedder == mock_tongyi_embedder

    # 测试场景2: 无通义千问API密钥 - 应使用默认嵌入器
    with patch.dict(os.environ, {"DASHSCOPE_API_KEY": ""}):
        with patch('app.worker.processor.create_embedder') as mock_create_embedder, \
                patch('app.worker.processor.get_default_embedder') as mock_get_default:
            # 模拟默认嵌入器
            mock_hf_embedder = MagicMock()
            mock_hf_embedder.get_model_name.return_value = "all-MiniLM-L6-v2"
            mock_get_default.return_value = mock_hf_embedder

            processor = DocumentProcessor()

            # 验证调用了get_default_embedder而不是直接create_embedder
            mock_create_embedder.assert_not_called()
            mock_get_default.assert_called_once()
            assert processor.embedder == mock_hf_embedder

    # 测试场景3: 所有嵌入器都不可用 - 应使用fallback嵌入器
    with patch.dict(os.environ, {"DASHSCOPE_API_KEY": ""}):
        with patch('app.worker.processor.create_embedder') as mock_create_embedder, \
                patch('app.worker.processor.get_default_embedder') as mock_get_default:
            # 模拟所有嵌入器都失败
            mock_get_default.side_effect = Exception("Failed to initialize embedders")

            processor = DocumentProcessor()

            # 验证使用了fallback嵌入器
            assert processor.embedder.get_model_name() == "fallback-embedder"
            # 验证其输出是1536维的零向量
            vector = processor.embedder.embed("test")
            assert len(vector) == 1536
            assert all(v == 0.0 for v in vector)


def test_fallback_embedder():
    """测试回退嵌入器的创建与使用"""
    with patch('app.worker.processor.create_embedder'), \
            patch('app.worker.processor.get_default_embedder') as mock_get_default:
        # 模拟嵌入模型创建失败
        mock_get_default.side_effect = Exception("Failed to initialize model")

        with patch.dict('os.environ', {"DASHSCOPE_API_KEY": ""}):
            processor = DocumentProcessor()

            # 确保使用了回退嵌入器
            assert processor.embedder.get_model_name() == "fallback-embedder"

            # 测试回退嵌入器的功能
            vector = processor.embedder.embed("test text")
            assert len(vector) == 1536
            assert all(v == 0.0 for v in vector)

            # 测试批量嵌入
            vectors = processor.embedder.embed_batch(["test text 1", "test text 2"])
            assert len(vectors) == 2
            assert len(vectors[0]) == 1536
            assert len(vectors[1]) == 1536
            assert all(v == 0.0 for v in vectors[0])
