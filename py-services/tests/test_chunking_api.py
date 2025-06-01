import pytest
import uuid
from fastapi.testclient import TestClient

# 导入应用
from app.main import app

# 创建测试客户端
client = TestClient(app)

# 测试文本内容
SAMPLE_TEXT_CONTENT = """
# Sample Document

This is a sample document for testing purposes.

## Section 1
Content of section 1.

## Section 2
Content of section 2.

## Section 3
Content of section 3.

## Section 4
Content of section 4.

## Section 5
Content of section 5.
"""

@pytest.fixture
def document_id():
    """
    生成一个测试用的文档ID
    """
    return f"test-doc-{uuid.uuid4().hex[:8]}"

def test_chunk_text_basic(document_id):
    """Test basic text chunking functionality"""
    data = {
        "text": SAMPLE_TEXT_CONTENT,
        "document_id": document_id,
        "store_result": True
    }
    
    response = client.post("/api/python/documents/chunk", json=data)
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查响应内容
    json_data = response.json()
    assert json_data["success"] is True
    assert json_data["document_id"] == document_id
    assert json_data["task_id"] is not None
    assert "chunks" in json_data
    assert isinstance(json_data["chunks"], list)
    assert json_data["chunk_count"] > 0

def test_chunk_text_with_invalid_params():
    """Test text chunking with invalid parameters"""
    # 测试缺少必要参数text
    data = {
        "document_id": "test-doc",
        "store_result": True
    }
    
    response = client.post("/api/python/documents/chunk", json=data)
    assert response.status_code == 422
    
    # 测试无效的chunk_size
    data = {
        "text": "Test content",
        "document_id": "test-doc",
        "chunk_size": -10,  # 负数块大小
        "store_result": True
    }
    
    response = client.post("/api/python/documents/chunk", json=data)
    assert response.status_code == 400
    
    # 测试无效的chunk_overlap
    data = {
        "text": "Test content",
        "document_id": "test-doc",
        "chunk_overlap": -5,  # 负数重叠
        "store_result": True
    }
    
    response = client.post("/api/python/documents/chunk", json=data)
    assert response.status_code == 400

def test_chunk_text_without_storing(document_id):
    """Test text chunking without storing results"""
    data = {
        "text": SAMPLE_TEXT_CONTENT,
        "document_id": document_id,
        "store_result": False
    }
    
    response = client.post("/api/python/documents/chunk", json=data)
    
    # 检查响应状态码和内容
    assert response.status_code == 200
    json_data = response.json()
    assert json_data["success"] is True
    assert json_data["task_id"] is None  # 不存储结果时，不会返回任务ID

def test_get_document_chunks(document_id):
    """Test retrieving document chunks"""
    # 先分块文本
    data = {
        "text": SAMPLE_TEXT_CONTENT,
        "document_id": document_id,
        "store_result": True
    }
    
    chunk_response = client.post("/api/python/documents/chunk", json=data)
    assert chunk_response.status_code == 200
    task_id = chunk_response.json()["task_id"]
    
    # 测试通过文档ID获取块
    response = client.get(f"/api/python/documents/{document_id}/chunks")
    assert response.status_code == 200
    json_data = response.json()
    assert json_data["success"] is True
    assert "chunks" in json_data
    assert isinstance(json_data["chunks"], list)
    
    # 测试通过任务ID获取块
    response = client.get(f"/api/python/documents/{document_id}/chunks?task_id={task_id}")
    assert response.status_code == 200
    json_data = response.json()
    assert json_data["success"] is True
    assert json_data["task_id"] == task_id
    assert "chunks" in json_data

def test_get_chunks_not_found():
    """Test retrieving non-existent document chunks"""
    nonexistent_id = f"nonexistent-{uuid.uuid4().hex}"
    response = client.get(f"/api/python/documents/{nonexistent_id}/chunks")
    assert response.status_code == 404

def test_get_chunks_with_invalid_task():
    """Test retrieving document with invalid task ID"""
    document_id = f"test-doc-{uuid.uuid4().hex[:8]}"
    invalid_task_id = f"invalid-task-{uuid.uuid4().hex}"
    
    response = client.get(f"/api/python/documents/{document_id}/chunks?task_id={invalid_task_id}")
    assert response.status_code == 404

def test_different_split_types(document_id):
    """Test different text splitting types"""
    split_types = ["paragraph", "sentence", "length"]
    
    for split_type in split_types:
        data = {
            "text": SAMPLE_TEXT_CONTENT,
            "document_id": f"{document_id}-{split_type}",
            "split_type": split_type,
            "store_result": True
        }
        
        response = client.post("/api/python/documents/chunk", json=data)
        assert response.status_code == 200
        json_data = response.json()
        assert json_data["success"] is True
        assert "chunks" in json_data
        assert len(json_data["chunks"]) > 0

def test_end_to_end_chunking(document_id):
    """
    测试文本分块的完整流程：分块然后检索
    """
    # 步骤1：分块文本
    data = {
        "text": SAMPLE_TEXT_CONTENT,
        "document_id": document_id,
        "split_type": "paragraph",
        "store_result": True
    }
    
    chunk_response = client.post("/api/python/documents/chunk", json=data)
    assert chunk_response.status_code == 200
    task_id = chunk_response.json()["task_id"]
    original_chunks = chunk_response.json()["chunks"]
    
    # 步骤2：检索分块结果
    get_response = client.get(f"/api/python/documents/{document_id}/chunks")
    assert get_response.status_code == 200
    result = get_response.json()
    
    # 验证分块结果是否一致
    assert result["success"] is True
    assert len(result["chunks"]) == len(original_chunks)
    
    # 验证块内容 - 至少第一个块应包含标题
    if result["chunks"]:
        assert "Sample Document" in result["chunks"][0]["text"]

if __name__ == "__main__":
    # 可以直接运行该文件执行测试
    pytest.main(["-xvs", __file__])