import json
import pytest
from datetime import datetime
from unittest.mock import patch, MagicMock

from fastapi.testclient import TestClient

from app.api.callback import router
from app.models.model import Task, TaskType, TaskStatus, TaskCallback
from app.utils.utils import get_task_key, get_document_tasks_key

# 测试客户端
client = TestClient(router)

# 测试数据
sample_task_id = "test-task-123"
sample_document_id = "test-doc-123"


@pytest.fixture
def mock_redis():
    """Redis客户端的模拟对象"""
    with patch("app.api.callback.get_redis_client") as mock_get_redis:
        mock_client = MagicMock()
        mock_get_redis.return_value = mock_client
        yield mock_client


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


def test_handle_callback_success(mock_redis, sample_task):
    """测试成功处理回调请求"""
    # 模拟Redis中的任务数据
    task_json = sample_task.to_json()
    mock_redis.get.return_value = task_json

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
    response = client.post("/", json=callback_data)

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["task_id"] == sample_task_id

    # 验证Redis操作
    mock_redis.get.assert_called_with(get_task_key(sample_task_id))
    mock_redis.set.assert_called()  # 确认调用set方法更新任务


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
    response = client.post("/", json=callback_data)

    # 验证响应
    assert response.status_code == 200  # 仍然返回200，但success=False
    assert response.json()["success"] is False
    assert "Missing required fields" in response.json()["message"]


def test_handle_callback_task_not_found(mock_redis):
    """测试处理不存在任务的回调请求"""
    # 模拟Redis中找不到任务
    mock_redis.get.return_value = None

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
    response = client.post("/", json=callback_data)

    # 验证响应
    assert response.status_code == 200  # API设计为总是返回200，只是success状态不同
    assert response.json()["success"] is False
    assert "not found" in response.json()["message"]

    # 验证Redis操作
    mock_redis.get.assert_called_with(get_task_key("non-existent-task"))
    mock_redis.set.assert_not_called()  # 不应该更新Redis


def test_handle_callback_invalid_json():
    """测试处理无效JSON的回调请求"""
    # 发送无效的JSON
    response = client.post(
        "/",
        content="{invalid json",
        headers={"Content-Type": "application/json"}
    )

    # 验证响应
    assert response.status_code == 200  # API设计为总是返回200，但报告错误
    assert response.json()["success"] is False
    assert "Invalid JSON" in response.json()["message"]


def test_handle_callback_invalid_status(mock_redis, sample_task):
    """测试处理无效任务状态的回调请求"""
    # 模拟Redis中的任务数据
    mock_redis.get.return_value = sample_task.to_json()

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
    response = client.post("/", json=callback_data)

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is False
    assert "Invalid task status" in response.json()["message"]


def test_get_task_status(mock_redis, sample_task):
    """测试获取任务状态"""
    # 模拟Redis中的任务数据
    mock_redis.get.return_value = sample_task.to_json()

    # 发送获取任务状态请求
    response = client.get(f"/task/{sample_task_id}")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["task_id"] == sample_task_id
    assert response.json()["status"] == "processing"
    assert response.json()["document_id"] == sample_document_id


def test_get_task_status_not_found(mock_redis):
    """测试获取不存在任务的状态"""
    # 模拟Redis中找不到任务
    mock_redis.get.return_value = None

    # 发送获取任务状态请求
    response = client.get("/task/non-existent-task")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is False
    assert "not found" in response.json()["message"]


def test_get_document_tasks(mock_redis):
    """测试获取文档任务列表"""
    # 模拟Redis中的文档任务集合
    task_ids = [b"task1", b"task2", b"task3"]
    mock_redis.smembers.return_value = task_ids

    # 模拟获取每个任务的数据
    tasks = []
    for i, task_id in enumerate(task_ids):
        task = Task(
            id=task_id.decode(),
            type=TaskType.DOCUMENT_PARSE,
            document_id=sample_document_id,
            status=TaskStatus.COMPLETED if i < 2 else TaskStatus.PROCESSING,
            created_at=datetime.now(),
            updated_at=datetime.now()
        )
        tasks.append(task)

    # 设置mock_redis.get的side_effect，使其根据不同的key返回不同的值
    def get_side_effect(key):
        task_key_prefix = get_task_key("").rstrip(":")
        if key.startswith(task_key_prefix):
            task_id = key[len(task_key_prefix):]
            for task in tasks:
                if task.id == task_id:
                    return task.to_json()
        return None

    mock_redis.get.side_effect = get_side_effect

    # 发送获取文档任务列表请求
    response = client.get(f"/document/{sample_document_id}/tasks")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert len(response.json()["tasks"]) == 3
    assert response.json()["document_id"] == sample_document_id


def test_get_document_tasks_empty(mock_redis):
    """测试获取空文档任务列表"""
    # 模拟Redis中没有文档任务
    mock_redis.smembers.return_value = []

    # 发送获取文档任务列表请求
    response = client.get(f"/document/{sample_document_id}/tasks")

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is True
    assert len(response.json()["tasks"]) == 0
    assert response.json()["document_id"] == sample_document_id


@patch("app.api.callback.update_task_status")
def test_update_task_status_failure(mock_update_task_status, mock_redis, sample_task):
    """测试更新任务状态失败的情况"""
    # 模拟Redis中的任务数据
    mock_redis.get.return_value = sample_task.to_json()

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
    response = client.post("/", json=callback_data)

    # 验证响应
    assert response.status_code == 200
    assert response.json()["success"] is False
    assert "Failed to update task status" in response.json()["message"]