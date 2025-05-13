import pytest
from datetime import datetime
import json
import os
import time
import traceback
from unittest.mock import patch, MagicMock, ANY

import numpy as np
from celery import Task as CeleryTask
from app.models.model import Task, TaskType, TaskStatus, ChunkInfo, VectorInfo
from app.worker.tasks import (
    get_redis_client, get_task_from_redis, update_task_status,
    parse_document, chunk_text, vectorize_text, process_document
)
from app.utils.utils import get_task_key, get_document_tasks_key

# 测试客户端
app.include_router(router)
client = TestClient(app)

# 测试数据
sample_task_id = "test-task-123"
sample_document_id = "test-doc-123"

@pytest.fixture
def mock_redis():
    """Redis客户端的模拟对象"""
    with patch('app.worker.tasks.get_redis_client') as mock_get_redis:
        mock_client = MagicMock()
        mock_get_redis.return_value = mock_client
        yield mock_client

@pytest.fixture
def mock_get_task_from_redis():
    """模拟get_task_from_redis函数"""
    with patch("app.api.callback.get_task_from_redis") as mock:
        yield mock


@pytest.fixture
def mock_update_task_status():
    """模拟update_task_status函数"""
    with patch("app.api.callback.update_task_status") as mock:
        yield mock


@pytest.fixture
def sample_task():
    """测试任务示例"""
    return Task(
        id=sample_task_id,
        type=TaskType.DOCUMENT_PARSE,
        document_id=sample_document_id,
        status=TaskStatus.PROCESSING,
        payload={"file_path": "test_file.pdf"},
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )


def test_handle_callback_success(mock_get_task_from_redis, mock_update_task_status, sample_task):
    """测试成功处理回调请求"""
    # 模拟任务存在
    mock_get_task_from_redis.return_value = sample_task
    
    # 模拟更新成功
    mock_update_task_status.return_value = True

    # 构建有效的回调请求
    callback_data = {
        "task_id": sample_task_id,
        "document_id": sample_document_id,
        "status": "completed",
        "type": "document_parse",
        "result": {"content": "Test document content"},
        "error": "",
        "timestamp": datetime.now().isoformat()
    }

    # 发送回调请求
    response = client.post("/api/callback/", json=callback_data)

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["task_id"] == sample_task_id

    # 验证函数调用
    mock_get_task_from_redis.assert_called_with(sample_task_id)
    mock_update_task_status.assert_called_once()


def test_handle_callback_missing_fields():
    """测试缺少必填字段的回调请求"""
    # 构建缺少必填字段的回调请求
    callback_data = {
        "task_id": sample_task_id,
        # 缺少document_id
        "status": "completed",
        # 缺少type
    }

    # 发送回调请求
    response = client.post("/api/callback/", json=callback_data)

    # 验证响应
    assert response.status_code == 200  # 仍然返回200，但success=False
    assert response.json()["success"] is False
    assert "Missing required fields" in response.json()["message"]


def test_handle_callback_task_not_found(mock_get_task_from_redis):
    """测试处理不存在任务的回调请求"""
    # 模拟Redis中找不到任务
    mock_get_task_from_redis.return_value = None

    # 构建有效的回调请求
    callback_data = {
        "task_id": "non-existent-task",
        "document_id": sample_document_id,
        "status": "completed",
        "type": "document_parse",
        "result": {},
        "error": "",
        "timestamp": datetime.now().isoformat()
    }

    # 发送回调请求
    response = client.post("/api/callback/", json=callback_data)

    # 验证响应
    assert response.status_code == 200  # API设计为总是返回200，只是success状态不同
    assert response.json()["success"] is False
    assert "not found" in response.json()["message"]

    # 验证函数调用
    mock_get_task_from_redis.assert_called_with("non-existent-task")


def test_handle_callback_invalid_json():
    """测试处理无效JSON的回调请求"""
    # 发送无效的JSON
    response = client.post(
        "/api/callback/",
        content="{invalid json",
        headers={"Content-Type": "application/json"}
    )

    # 验证响应
    assert response.status_code == 200  # API设计为总是返回200，但报告错误
    assert response.json()["success"] is False
    assert "Invalid JSON" in response.json()["message"]


def test_handle_callback_invalid_status(mock_get_task_from_redis, sample_task):
    """测试处理无效任务状态的回调请求"""
    # 模拟任务存在
    mock_get_task_from_redis.return_value = sample_task

    # 构建有无效状态的回调请求
    callback_data = {
        "task_id": sample_task_id,
        "document_id": sample_document_id,
        "status": "invalid_status",  # 无效状态
        "type": "document_parse",
        "result": {},
        "error": "",
        "timestamp": datetime.now().isoformat()
    }

    # 发送回调请求
    response = client.post("/api/callback/", json=callback_data)

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is False
    assert "Invalid task status" in response.json()["message"]


def test_get_task_status(mock_get_task_from_redis, sample_task):
    """测试获取任务状态"""
    # 模拟任务存在
    mock_get_task_from_redis.return_value = sample_task

    # 发送获取任务状态请求
    response = client.get(f"/api/callback/task/{sample_task_id}")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["task_id"] == sample_task_id
    assert response.json()["status"] == "processing"
    assert response.json()["document_id"] == sample_document_id


def test_get_task_status_not_found(mock_get_task_from_redis):
    """测试获取不存在任务的状态"""
    # 模拟Redis中找不到任务
    mock_get_task_from_redis.return_value = None

    # 发送获取任务状态请求
    response = client.get("/api/callback/task/non-existent-task")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is False
    assert "not found" in response.json()["message"]


@patch('app.api.callback.get_redis_client')
def test_get_document_tasks(mock_redis_client, mock_get_task_from_redis, sample_task):
    """测试获取文档任务列表"""
    # 创建一个Redis客户端的模拟
    mock_client = MagicMock()
    mock_redis_client.return_value = mock_client
    
    # 模拟Redis中的文档任务集合
    task_ids = [b"task1", b"task2", b"task3"]
    mock_client.smembers.return_value = task_ids
    
    # 模拟获取任务详情
    mock_get_task_from_redis.side_effect = lambda task_id: Task(
        id=task_id,
        type=TaskType.DOCUMENT_PARSE,
        document_id=sample_document_id,
        status=TaskStatus.COMPLETED if task_id in ["task1", "task2"] else TaskStatus.PROCESSING,
        created_at=datetime.now(),
        updated_at=datetime.now(),
    )

    # 发送获取文档任务列表请求
    response = client.get(f"/api/callback/document/{sample_document_id}/tasks")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert len(response.json()["tasks"]) == 3
    assert response.json()["document_id"] == sample_document_id


@patch('app.api.callback.get_redis_client')
def test_get_document_tasks_empty(mock_redis_client):
    """测试获取空文档任务列表"""
    # 创建一个Redis客户端的模拟
    mock_client = MagicMock()
    mock_redis_client.return_value = mock_client
    
    # 模拟Redis中没有文档任务
    mock_client.smembers.return_value = []

    # 发送获取文档任务列表请求
    response = client.get(f"/api/callback/document/{sample_document_id}/tasks")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert len(response.json()["tasks"]) == 0
    assert response.json()["document_id"] == sample_document_id


def test_update_task_status_failure(mock_get_task_from_redis, mock_update_task_status, sample_task):
    """测试更新任务状态失败的情况"""
    # 模拟任务存在
    mock_get_task_from_redis.return_value = sample_task

    # 模拟更新任务状态失败
    mock_update_task_status.return_value = False

    # 构建回调请求
    callback_data = {
        "task_id": sample_task_id,
        "document_id": sample_document_id,
        "status": "completed",
        "type": "document_parse",
        "result": {},
        "error": "",
        "timestamp": datetime.now().isoformat()
    }

    # 发送回调请求
    response = client.post("/api/callback/", json=callback_data)

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is False
    assert "Failed to update task status" in response.json()["message"]