import os
import json
import time
import pytest
from datetime import datetime
from unittest.mock import patch, MagicMock, ANY

from celery import Task as CeleryTask
from app.models.model import Task, TaskType, TaskStatus
from app.worker.tasks import (
    get_redis_client, get_task_from_redis, update_task_status,
    parse_document, chunk_text, vectorize_text, process_document
)
from app.utils.utils import get_task_key, get_document_tasks_key


@pytest.fixture
def mock_redis():
    """Redis客户端的模拟对象"""
    with patch('app.worker.tasks.get_redis_client') as mock_get_redis:
        mock_client = MagicMock()
        mock_get_redis.return_value = mock_client
        yield mock_client


@pytest.fixture
def sample_task():
    """测试任务示例"""
    return Task(
        id="test-task-123",
        type=TaskType.DOCUMENT_PARSE,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={"file_path": "test_file.pdf", "file_name": "test_file.pdf", "file_type": "pdf"},
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )


def test_get_task_from_redis(mock_redis, sample_task):
    """测试从Redis获取任务"""
    # 设置模拟返回值
    mock_redis.get.return_value = sample_task.to_json()

    # 调用函数
    task = get_task_from_redis(sample_task.id)

    # 验证结果
    assert task is not None
    assert task.id == sample_task.id
    assert task.type == sample_task.type
    assert task.document_id == sample_task.document_id
    assert task.status == sample_task.status

    # 验证Redis调用
    mock_redis.get.assert_called_once_with(get_task_key(sample_task.id))


def test_get_task_from_redis_not_found(mock_redis):
    """测试获取不存在的任务"""
    # 设置模拟返回值
    mock_redis.get.return_value = None

    # 调用函数
    task = get_task_from_redis("non-existent-task")

    # 验证结果
    assert task is None


def test_update_task_status(mock_redis, sample_task):
    """测试更新任务状态"""
    # 设置模拟
    with patch('app.worker.tasks.send_callback') as mock_send_callback:
        mock_send_callback.return_value = True

        # 调用函数
        result = update_task_status(sample_task, TaskStatus.PROCESSING)

        # 验证结果
        assert result is True
        assert sample_task.status == TaskStatus.PROCESSING
        assert sample_task.started_at is not None

        # 验证Redis调用
        mock_redis.set.assert_called_once_with(get_task_key(sample_task.id), ANY)

        # 验证回调调用
        mock_send_callback.assert_called_once()


def test_update_task_status_completed(mock_redis, sample_task):
    """测试将任务状态更新为已完成"""
    # 设置模拟
    with patch('app.worker.tasks.send_callback') as mock_send_callback:
        # 准备测试数据
        result_data = {"content": "Test content", "title": "Test Document"}

        # 调用函数
        result = update_task_status(sample_task, TaskStatus.COMPLETED, result=result_data)

        # 验证结果
        assert result is True
        assert sample_task.status == TaskStatus.COMPLETED
        assert sample_task.result == result_data
        assert sample_task.completed_at is not None

        # 验证Redis调用
        mock_redis.set.assert_called_once_with(get_task_key(sample_task.id), ANY)

        # 验证回调调用
        mock_send_callback.assert_called_once()


def test_update_task_status_failed(mock_redis, sample_task):
    """测试将任务状态更新为失败"""
    # 设置模拟
    with patch('app.worker.tasks.send_callback') as mock_send_callback:
        # 准备测试数据
        error_message = "Test error message"

        # 调用函数
        result = update_task_status(sample_task, TaskStatus.FAILED, error=error_message)

        # 验证结果
        assert result is True
        assert sample_task.status == TaskStatus.FAILED
        assert sample_task.error == error_message
        assert sample_task.completed_at is not None

        # 验证Redis调用
        mock_redis.set.assert_called_once_with(get_task_key(sample_task.id), ANY)

        # 验证回调数据包含错误信息
        call_args = mock_send_callback.call_args[0][1]
        assert call_args["error"] == error_message


@patch('app.worker.tasks.update_task_status')
def test_parse_document_task(mock_update_status, mock_redis, sample_task):
    """测试文档解析任务"""
    # 设置模拟返回值
    mock_redis.get.return_value = sample_task.to_json()

    with patch('app.worker.tasks.get_task_from_redis', return_value=sample_task):
        # 模拟文件操作，避免文件不存在错误
        with patch('app.parsers.base.BaseParser.validate_file', return_value=None):
            with patch('app.parsers.factory.create_parser') as mock_create_parser:
                # 模拟解析器
                mock_parser = MagicMock()
                mock_parser.parse.return_value = "Sample content"
                mock_parser.extract_title.return_value = "Sample Title"
                mock_parser.get_metadata.return_value = {"page_count": 1}
                mock_create_parser.return_value = mock_parser
                
                with patch('app.parsers.factory.detect_content_type', return_value="application/pdf"):
                    # 调用任务函数
                    result = parse_document(sample_task.id)

                    # 验证结果
                    assert result is True


@patch('app.worker.tasks.update_task_status')
@patch('app.parsers.factory.create_parser')
def test_parse_document_task_implementation(mock_create_parser, mock_update_status, mock_redis, sample_task):
    """测试文档解析任务实现"""
    # 设置模拟返回值
    mock_redis.get.return_value = sample_task.to_json()

    # 创建解析器模拟
    mock_parser = MagicMock()
    mock_parser.parse.return_value = "Sample document content"
    mock_parser.extract_title.return_value = "Sample Title"
    mock_parser.get_metadata.return_value = {"page_count": 5, "author": "Test Author"}
    mock_create_parser.return_value = mock_parser

    with patch('app.worker.tasks.get_task_from_redis', return_value=sample_task):
        with patch('app.parsers.factory.detect_content_type', return_value="application/pdf"):
            # 调用任务函数
            result = parse_document(sample_task.id)

            # 验证结果
            assert result is True

            # 验证create_parser被调用
            mock_create_parser.assert_called_once()

            # 验证parser.parse被调用
            mock_parser.parse.assert_called_once_with(sample_task.payload["file_path"])
            
            # 验证extract_title被调用
            mock_parser.extract_title.assert_called_once()
            
            # 验证get_metadata被调用
            mock_parser.get_metadata.assert_called_once()

            # 验证任务状态更新
            assert mock_update_status.call_count >= 2
            # 第一次调用应更新为处理中
            assert mock_update_status.call_args_list[0][0][1] == TaskStatus.PROCESSING
            # 最后一次调用应更新为已完成
            assert mock_update_status.call_args_list[-1][0][1] == TaskStatus.COMPLETED
            
            # 验证结果格式
            result_dict = mock_update_status.call_args_list[-1][0][2]
            assert 'content' in result_dict
            assert 'title' in result_dict
            assert 'meta' in result_dict
            assert 'words' in result_dict
            assert 'chars' in result_dict


@patch('app.worker.tasks.update_task_status')
def test_chunk_text_task(mock_update_status, mock_redis):
    """测试文本分块任务"""
    # 创建测试任务
    task = Task(
        id="chunk-task-123",
        type=TaskType.TEXT_CHUNK,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-123",
            "content": "This is a test document content. It should be split into chunks.",
            "chunk_size": 50,
            "overlap": 10,
            "split_type": "length"
        },
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 设置模拟返回值
    mock_redis.get.return_value = task.to_json()

    with patch('app.worker.tasks.get_task_from_redis', return_value=task):
        # 调用任务函数
        result = chunk_text(task.id)

        # 验证结果
        assert result is True

        # 验证任务状态更新
        assert mock_update_status.call_count >= 2
        # 第一次调用应更新为处理中
        assert mock_update_status.call_args_list[0][0][1] == TaskStatus.PROCESSING
        # 最后一次调用应更新为已完成
        assert mock_update_status.call_args_list[-1][0][1] == TaskStatus.COMPLETED

        # 验证结果包含块信息
        result_dict = mock_update_status.call_args_list[-1][0][2]
        assert 'chunks' in result_dict
        assert isinstance(result_dict['chunks'], list)
        assert 'chunk_count' in result_dict
        assert result_dict['chunk_count'] == len(result_dict['chunks'])


@patch('app.worker.tasks.update_task_status')
@patch('app.chunkers.splitter.split_text')
def test_chunk_text_task_implementation(mock_split_text, mock_update_status, mock_redis):
    """测试文本分块任务的实际实现"""
    # 创建测试任务
    task = Task(
        id="chunk-task-123",
        type=TaskType.TEXT_CHUNK,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-123",
            "content": "This is a test document content. It should be split into chunks.",
            "chunk_size": 50,
            "overlap": 10,
            "split_type": "length"
        },
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 设置模拟返回值
    mock_redis.get.return_value = task.to_json()
    
    # 模拟split_text函数的返回值
    mock_chunks = [
        {"text": "This is chunk 1", "index": 0, "metadata": {"document_id": "test-doc-123"}},
        {"text": "This is chunk 2", "index": 1, "metadata": {"document_id": "test-doc-123"}}
    ]
    mock_split_text.return_value = mock_chunks

    with patch('app.worker.tasks.get_task_from_redis', return_value=task):
        # 调用任务函数
        result = chunk_text(task.id)

        # 验证结果
        assert result is True

        # 验证split_text被正确调用
        mock_split_text.assert_called_once_with(
            text=task.payload["content"],
            chunk_size=task.payload["chunk_size"],
            chunk_overlap=task.payload["overlap"],
            split_type=task.payload["split_type"],
            metadata={"document_id": task.payload["document_id"]}
        )

        # 验证任务状态更新
        assert mock_update_status.call_count >= 2
        assert mock_update_status.call_args_list[0][0][1] == TaskStatus.PROCESSING
        assert mock_update_status.call_args_list[-1][0][1] == TaskStatus.COMPLETED

        # 验证结果数据
        result_dict = mock_update_status.call_args_list[-1][0][2]
        assert 'chunks' in result_dict
        assert 'chunk_count' in result_dict
        assert result_dict['chunk_count'] == 2


@patch('app.worker.tasks.update_task_status')
def test_vectorize_text_task(mock_update_status, mock_redis):
    """测试文本向量化任务"""
    # 创建测试任务
    task = Task(
        id="vector-task-123",
        type=TaskType.VECTORIZE,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-123",
            "chunks": [
                {"text": "Chunk 1", "index": 0},
                {"text": "Chunk 2", "index": 1}
            ],
            "model": "default"
        },
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 设置模拟返回值
    mock_redis.get.return_value = task.to_json()

    with patch('app.worker.tasks.get_task_from_redis', return_value=task):
        with patch('app.embedders.factory.create_embedder') as mock_create_embedder:
            # 创建嵌入模型模拟
            mock_embedder = MagicMock()
            mock_embedder.get_model_name.return_value = "default"
            mock_embedder.embed_batch.return_value = [
                [0.1, 0.2, 0.3, 0.4],
                [0.5, 0.6, 0.7, 0.8]
            ]
            mock_create_embedder.return_value = mock_embedder
            
            # 调用任务函数
            result = vectorize_text(task.id)

            # 验证结果
            assert result is True
            
            # 验证模型调用
            mock_embedder.embed_batch.assert_called_once()
            # 验证调用参数 - 确保正确提取了文本
            texts = ["Chunk 1", "Chunk 2"]
            mock_embedder.embed_batch.assert_called_with(texts)


@patch('app.worker.tasks.update_task_status')
@patch('app.embedders.factory.create_embedder')
def test_vectorize_text_task_implementation(mock_create_embedder, mock_update_status, mock_redis):
    """测试文本向量化任务的实际实现"""
    # 创建测试任务
    task = Task(
        id="vector-task-123",
        type=TaskType.VECTORIZE,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-123",
            "chunks": [
                {"text": "Chunk 1", "index": 0},
                {"text": "Chunk 2", "index": 1}
            ],
            "model": "default"  # 使用默认模型名称避免模型不存在问题
        },
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 设置模拟返回值
    mock_redis.get.return_value = task.to_json()
    
    # 创建嵌入模型模拟
    mock_embedder = MagicMock()
    mock_embedder.get_model_name.return_value = "default-model"
    mock_embedder.embed_batch.return_value = [
        [0.1, 0.2, 0.3, 0.4],
        [0.5, 0.6, 0.7, 0.8]
    ]
    mock_create_embedder.return_value = mock_embedder

    with patch('app.worker.tasks.get_task_from_redis', return_value=task):
        # 调用任务函数
        result = vectorize_text(task.id)

        # 验证结果
        assert result is True
        
        # 验证update_task_status调用
        assert mock_update_status.call_count == 2
        # 确认第一次调用是设置为处理中
        assert mock_update_status.call_args_list[0][0][1] == TaskStatus.PROCESSING
        # 确认最后一次调用是设置为已完成
        assert mock_update_status.call_args_list[1][0][1] == TaskStatus.COMPLETED
        
        # 验证嵌入模型被正确调用
        mock_create_embedder.assert_called_once_with("default")


@patch('app.worker.tasks.update_task_status')
@patch('app.parsers.factory.create_parser')
@patch('app.chunkers.splitter.split_text')
@patch('app.embedders.factory.create_embedder')
def test_process_document_task_implementation(mock_create_embedder, mock_split_text, mock_create_parser, mock_update_status, mock_redis):
    """测试完整文档处理流程的实际实现"""
    # 创建测试任务
    task = Task(
        id="process-task-123",
        type=TaskType.PROCESS_COMPLETE,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-123",
            "file_path": "/path/to/test.pdf",
            "file_name": "test.pdf",
            "file_type": "pdf",
            "chunk_size": 500,
            "overlap": 100,
            "split_type": "paragraph",
            "model": "default"
        },
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 设置模拟返回值
    mock_redis.get.return_value = task.to_json()
    
    # 创建解析器模拟
    mock_parser = MagicMock()
    mock_parser.parse.return_value = "Sample document content for complete process test"
    mock_parser.extract_title.return_value = "Test Document"
    mock_parser.get_metadata.return_value = {"page_count": 3}
    mock_create_parser.return_value = mock_parser
    
    # 模拟split_text函数的返回值
    mock_chunks = [
        {"text": "This is chunk 1", "index": 0, "metadata": {"document_id": "test-doc-123"}},
        {"text": "This is chunk 2", "index": 1, "metadata": {"document_id": "test-doc-123"}}
    ]
    mock_split_text.return_value = mock_chunks
    
    # 创建嵌入模型模拟
    mock_embedder = MagicMock()
    mock_embedder.get_model_name.return_value = "default-model"
    mock_embedder.embed_batch.return_value = [
        [0.1, 0.2, 0.3, 0.4],
        [0.5, 0.6, 0.7, 0.8]
    ]
    mock_create_embedder.return_value = mock_embedder

    with patch('app.worker.tasks.get_task_from_redis', return_value=task):
        with patch('app.parsers.factory.detect_content_type', return_value="application/pdf"):
            # 模拟文件验证以避免文件不存在错误
            with patch('app.parsers.base.BaseParser.validate_file', return_value=None):
                # 调用任务函数
                result = process_document(task.id)

                # 验证结果
                assert result is True

                # 验证各个组件是否被调用
                mock_create_parser.assert_called_once()
                mock_parser.parse.assert_called_once()
                mock_split_text.assert_called_once()
                mock_create_embedder.assert_called_once()
                mock_embedder.embed_batch.assert_called_once()


@patch('app.worker.tasks.update_task_status')
def test_process_document_task(mock_update_status, mock_redis):
    """测试完整文档处理任务"""
    # 创建测试任务
    task = Task(
        id="process-task-123",
        type=TaskType.PROCESS_COMPLETE,
        document_id="test-doc-123",
        status=TaskStatus.PENDING,
        payload={
            "document_id": "test-doc-123",
            "file_path": "/path/to/test.pdf",
            "file_name": "test.pdf",
            "file_type": "pdf",
            "chunk_size": 500,
            "overlap": 100,
            "split_type": "paragraph",
            "model": "default"
        },
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 设置模拟返回值
    mock_redis.get.return_value = task.to_json()

    with patch('app.worker.tasks.get_task_from_redis', return_value=task):
        with patch('app.parsers.factory.create_parser') as mock_create_parser:
            mock_parser = MagicMock()
            mock_parser.parse.return_value = "Sample content"
            mock_parser.extract_title.return_value = "Test Title"
            mock_parser.get_metadata.return_value = {"page_count": 1}
            mock_create_parser.return_value = mock_parser

            with patch('app.parsers.factory.detect_content_type', return_value="application/pdf"):
                with patch('app.parsers.base.BaseParser.validate_file', return_value=None):
                    with patch('app.chunkers.splitter.split_text') as mock_split_text:
                        mock_split_text.return_value = [
                            {"text": "This is chunk 1", "index": 0, "metadata": {"document_id": "test-doc-123"}}
                        ]
                        
                        with patch('app.embedders.factory.create_embedder') as mock_create_embedder:
                            mock_embedder = MagicMock()
                            mock_embedder.get_model_name.return_value = "default-model"
                            mock_embedder.embed_batch.return_value = [[0.1, 0.2, 0.3, 0.4]]
                            mock_create_embedder.return_value = mock_embedder
                            
                            # 调用任务函数
                            result = process_document(task.id)

                            # 验证结果
                            assert result is True


def test_task_not_found_handling(mock_redis):
    """测试处理任务不存在的情况"""
    # 设置模拟返回值
    mock_redis.get.return_value = None

    # 调用任务函数
    result = parse_document("non-existent-task")

    # 验证结果
    assert result is False


@patch('app.worker.tasks.update_task_status')
def test_task_failure_handling(mock_update_status, mock_redis, sample_task):
    """测试任务失败处理"""
    # 设置模拟
    mock_redis.get.return_value = sample_task.to_json()

    # 设置正确的副作用，使第一次调用成功，第二次调用正常返回False
    mock_update_status.side_effect = [True, False] 

    with patch('app.worker.tasks.get_task_from_redis', return_value=sample_task):
        with patch('app.parsers.factory.create_parser') as mock_create_parser:
            # 设置模拟解析器抛出异常
            mock_parser = MagicMock()
            mock_parser.parse.side_effect = ValueError("Test parse error")
            mock_create_parser.return_value = mock_parser
            
            with patch('app.parsers.factory.detect_content_type', return_value="application/pdf"):
                with patch('app.parsers.base.BaseParser.validate_file', return_value=None):
                    # 调用任务函数
                    result = parse_document(sample_task.id)
                    
                    # 验证结果
                    assert result is False
                    
                    # 验证update_task_status被调用了两次：一次设置处理中，一次设置失败
                    assert mock_update_status.call_count == 2
                    assert mock_update_status.call_args_list[0][0][1] == TaskStatus.PROCESSING
                    assert mock_update_status.call_args_list[1][0][1] == TaskStatus.FAILED
                    # 验证error参数包含了错误信息
                    assert "Test parse error" in mock_update_status.call_args_list[1][1].get('error', '')


def test_task_serialization():
    """测试任务序列化和反序列化"""
    # 创建测试任务
    original_task = Task(
        id="serialize-test",
        type=TaskType.DOCUMENT_PARSE,
        document_id="doc-123",
        status=TaskStatus.PENDING,
        payload={"file_path": "test.pdf"},
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 序列化
    json_data = original_task.to_json()

    # 验证是有效的JSON
    parsed_json = json.loads(json_data)
    assert parsed_json['id'] == original_task.id
    assert parsed_json['type'] == original_task.type.value
    assert parsed_json['document_id'] == original_task.document_id

    # 反序列化
    recreated_task = Task.from_json(json_data)

    # 验证任务被正确还原
    assert recreated_task.id == original_task.id
    assert recreated_task.type == original_task.type
    assert recreated_task.document_id == original_task.document_id
    assert recreated_task.status == original_task.status
    assert recreated_task.payload == original_task.payload


@patch('celery.app.task.Task.delay')
@patch('app.worker.tasks.get_redis_client')
def test_celery_integration(mock_get_redis, mock_delay, sample_task):
    """测试Celery任务集成"""
    # 设置模拟
    mock_client = MagicMock()
    mock_get_redis.return_value = mock_client

    # 模拟任务已存储在Redis中
    with patch('app.worker.tasks.get_task_from_redis', return_value=sample_task):
        # 调用函数 - 在真实环境会调用delay()
        parse_document(sample_task.id)

        # 在生产环境中，这应该会调用celery任务的delay方法
        # 但在测试中我们没有真正调用，而是将其模拟为直接执行函数

        # 验证Redis操作
        mock_client.set.assert_called()