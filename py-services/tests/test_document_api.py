import os
import tempfile
import pytest
from fastapi.testclient import TestClient
import uuid

# 导入应用
from app.main import app

# 创建测试客户端
client = TestClient(app)

# 测试文件内容
SAMPLE_TEXT_CONTENT = """
# Sample Document

This is a sample document for testing purposes.

## Section 1
Content of section 1.

## Section 2
Content of section 2.
"""

# 创建测试文件的夹具
@pytest.fixture
def sample_text_file():
    """
    创建一个示例文本文件用于测试
    """
    with tempfile.NamedTemporaryFile(suffix=".md", delete=False) as temp:
        temp.write(SAMPLE_TEXT_CONTENT.encode('utf-8'))
        temp_path = temp.name
    
    yield temp_path
    
    # 测试后清理文件
    if os.path.exists(temp_path):
        os.remove(temp_path)

@pytest.fixture
def document_id():
    """
    生成一个测试用的文档ID
    """
    return f"test-doc-{uuid.uuid4().hex[:8]}"

def test_parse_document_with_file_upload(sample_text_file, document_id):
    """Test document parsing with file upload"""
    with open(sample_text_file, "rb") as f:
        files = {"file": (os.path.basename(sample_text_file), f)}
        data = {"document_id": document_id, "store_result": "true"}
        
        response = client.post(
            "/api/python/documents/parse",
            files=files,
            data=data
        )
        
        # 检查响应状态码
        assert response.status_code == 200
        
        # 检查响应内容
        json_data = response.json()
        assert json_data["success"] is True
        assert json_data["document_id"] == document_id
        assert json_data["task_id"] is not None
        assert "result" in json_data
        assert "content" in json_data["result"]
        assert "Sample Document" in json_data["result"]["content"]

def test_parse_document_with_file_path(sample_text_file, document_id):
    """Test document parsing with file path"""
    data = {
        "file_path": sample_text_file,
        "document_id": document_id,
        "store_result": "true"
    }
    
    response = client.post(
        "/api/python/documents/parse",
        data=data
    )
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查响应内容
    json_data = response.json()
    assert json_data["success"] is True
    assert json_data["document_id"] == document_id
    assert json_data["task_id"] is not None
    assert "content" in json_data["result"]
    assert "Sample Document" in json_data["result"]["content"]


def test_parse_document_without_storing(sample_text_file):
    """Test document parsing without storing results"""
    document_id = f"test-doc-{uuid.uuid4().hex[:8]}"
    
    data = {
        "file_path": sample_text_file,
        "document_id": document_id,
        "store_result": "false"
    }
    
    response = client.post(
        "/api/python/documents/parse",
        data=data
    )
    
    # 检查响应状态码和内容
    assert response.status_code == 200
    json_data = response.json()
    assert json_data["success"] is True
    assert json_data["task_id"] is None  # 不存储结果时，不会返回任务ID

def test_parse_document_invalid_input():
    """Test document parsing with invalid input"""
    # 缺少必要参数
    response = client.post("/api/python/documents/parse", data={})
    assert response.status_code == 422  # FastAPI 使用 422 响应缺少参数
    
    # 文件不存在
    data = {
        "file_path": "/path/to/nonexistent/file.txt",
        "document_id": "test-doc"
    }
    response = client.post("/api/python/documents/parse", data=data)
    assert response.status_code == 404

def test_get_document_parse_result(sample_text_file, document_id):
    """Test retrieving document parse result"""
    # 先解析文档
    data = {
        "file_path": sample_text_file,
        "document_id": document_id,
        "store_result": "true"
    }
    
    parse_response = client.post("/api/python/documents/parse", data=data)
    assert parse_response.status_code == 200
    task_id = parse_response.json()["task_id"]
    
    # 测试通过文档ID获取结果
    response = client.get(f"/api/python/documents/{document_id}")
    assert response.status_code == 200
    json_data = response.json()
    assert json_data["success"] is True
    assert "result" in json_data
    
    # 测试通过任务ID获取结果
    response = client.get(f"/api/python/documents/{document_id}?task_id={task_id}")
    assert response.status_code == 200
    json_data = response.json()
    assert json_data["success"] is True
    assert json_data["task_id"] == task_id
    assert "result" in json_data

def test_get_document_not_found():
    """Test retrieving non-existent document"""
    nonexistent_id = f"nonexistent-{uuid.uuid4().hex}"
    response = client.get(f"/api/python/documents/{nonexistent_id}")
    assert response.status_code == 404

def test_get_document_with_invalid_task():
    """Test retrieving document with invalid task ID"""
    document_id = f"test-doc-{uuid.uuid4().hex[:8]}"
    invalid_task_id = f"invalid-task-{uuid.uuid4().hex}"
    
    response = client.get(f"/api/python/documents/{document_id}?task_id={invalid_task_id}")
    assert response.status_code == 404

# 集成测试：解析和检索文档
def test_end_to_end_document_processing(sample_text_file):
    """
    测试文档处理的完整流程：解析然后检索
    """
    document_id = f"test-doc-{uuid.uuid4().hex[:8]}"
    
    # 步骤1：解析文档
    with open(sample_text_file, "rb") as f:
        files = {"file": (os.path.basename(sample_text_file), f)}
        data = {"document_id": document_id, "store_result": "true"}
        
        parse_response = client.post(
            "/api/python/documents/parse",
            files=files,
            data=data
        )
        assert parse_response.status_code == 200
        task_id = parse_response.json()["task_id"]
    
    # 步骤2：检索文档结果
    get_response = client.get(f"/api/python/documents/{document_id}")
    assert get_response.status_code == 200
    result = get_response.json()
    
    # 验证文档内容是否正确
    assert result["success"] is True
    assert "Sample Document" in result["result"]["content"]
    assert "Section 1" in result["result"]["content"]
    assert "Section 2" in result["result"]["content"]

if __name__ == "__main__":
    # 可以直接运行该文件执行测试
    pytest.main(["-xvs", __file__])