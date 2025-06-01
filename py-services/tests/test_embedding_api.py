import os
import pytest
from fastapi.testclient import TestClient
from dotenv import load_dotenv

# 导入应用
from app.main import app

# 加载环境变量
load_dotenv(os.path.join(os.path.dirname(__file__), '..', '.env'))

# 创建测试客户端
client = TestClient(app)

# 测试文本样例
SAMPLE_TEXTS = [
    "This is a sample text for testing embeddings.",
    "Another example text to use for vector generation.",
    "The embedding model should process this text and return a vector."
]

@pytest.fixture
def api_key():
    """
    获取API密钥用于测试
    """
    api_key = os.getenv("DASHSCOPE_API_KEY")
    if not api_key:
        pytest.skip("DASHSCOPE_API_KEY not found in environment variables")
    return api_key

def test_generate_embedding(api_key):
    """Test single text embedding generation"""
    # 准备请求数据
    json_data = {
        "text": SAMPLE_TEXTS[0] 
    }

    params = {
        "model": "text-embedding-v3"
    }
    
    # 发送请求
    response = client.post("/api/python/embeddings", json=json_data, params=params)
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查响应内容
    json_data = response.json()
    assert json_data["success"] is True
    assert "model" in json_data
    assert "embedding" in json_data
    assert "dimension" in json_data
    assert "text_length" in json_data
    
    # 验证嵌入向量
    embedding = json_data["embedding"]
    assert isinstance(embedding, list)
    assert len(embedding) == json_data["dimension"]
    assert all(isinstance(x, (int, float)) for x in embedding)
    
    # 验证处理时间
    assert "process_time_ms" in json_data
    assert isinstance(json_data["process_time_ms"], int)

def test_generate_embedding_batch(api_key):
    """Test batch embedding generation"""
    # 准备请求数据
    json_data = {
        "texts": SAMPLE_TEXTS
    }
    
    params = {
        "model": "text-embedding-v3",
        "normalize": True
    }
    
    # 发送请求
    response = client.post("/api/python/embeddings/batch", json=json_data, params=params)
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查响应内容
    json_data = response.json()
    assert json_data["success"] is True
    assert "model" in json_data
    assert "embeddings" in json_data
    assert "count" in json_data
    assert "dimension" in json_data
    assert json_data["count"] == len(SAMPLE_TEXTS)
    assert json_data["normalized"] is True
    
    # 验证嵌入向量
    embeddings = json_data["embeddings"]
    assert isinstance(embeddings, list)
    assert len(embeddings) == len(SAMPLE_TEXTS)
    
    # 验证每个向量的维度
    for embedding in embeddings:
        assert isinstance(embedding, list)
        assert len(embedding) == json_data["dimension"]
        assert all(isinstance(x, (int, float)) for x in embedding)
    
    # 如果向量被标准化，则验证L2范数约等于1
    if json_data["normalized"]:
        for embedding in embeddings:
            norm = sum(x*x for x in embedding) ** 0.5  # L2范数
            assert 0.99 <= norm <= 1.01  # 允许一点数值误差

def test_list_embedding_models():
    """Test listing available embedding models"""
    response = client.get("/api/python/embeddings/models")
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查响应内容
    json_data = response.json()
    assert json_data["success"] is True
    assert "models" in json_data
    
    # 验证模型列表
    models = json_data["models"]
    assert isinstance(models, dict)
    
    # 应该至少包含tongyi模型
    assert "tongyi" in models
    assert isinstance(models["tongyi"], list)
    assert len(models["tongyi"]) > 0

def test_calculate_similarity(api_key):
    """Test text similarity calculation"""
    # 准备请求数据
    json_data = {
        "text1": "This is a test sentence about artificial intelligence.",
        "text2": "AI is transforming the technology landscape."
    }
    
    params = {
        "model": "text-embedding-v3"
    }
    
    # 发送请求
    response = client.post("/api/python/embeddings/similarity", json=json_data, params=params)
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查响应内容
    json_data = response.json()
    assert json_data["success"] is True
    assert "similarity" in json_data
    assert "model" in json_data
    
    # 验证相似度分数
    similarity = json_data["similarity"]
    assert isinstance(similarity, float)
    assert 0 <= similarity <= 1  # 相似度应该在0到1之间

def test_embedding_with_invalid_input():
    """Test embedding generation with invalid input"""
    # 测试空文本
    json_data = {
        "text": ""
    }
    response = client.post("/api/python/embeddings", json=json_data)
    assert response.status_code == 400
    
    # 测试无文本
    json_data = {}
    response = client.post("/api/python/embeddings", json=json_data)
    assert response.status_code == 422
    
    # 测试批量嵌入的无效输入
    json_data = {
        "texts": []
    }
    response = client.post("/api/python/embeddings/batch", json=json_data)
    assert response.status_code == 400
    
    # 测试相似度计算的无效输入
    json_data = {
        "text1": "This is text one",
        # 缺少 text2
    }
    response = client.post("/api/python/embeddings/similarity", json=json_data)
    assert response.status_code == 422

def test_embedding_dimension_parameter(api_key):
    """Test embedding generation with dimension parameter"""
    dimension = 512  # 测试512维向量
    
    json_data = {
        "text": SAMPLE_TEXTS[0]
    }
    
    params = {
        "model": "text-embedding-v3",
        "dimension": dimension
    }
    
    response = client.post("/api/python/embeddings", json=json_data, params=params)
    
    # 检查响应状态码
    assert response.status_code == 200
    
    # 检查嵌入维度
    json_data = response.json()
    assert json_data["dimension"] == dimension
    assert len(json_data["embedding"]) == dimension

if __name__ == "__main__":
    # 可以直接运行该文件执行测试
    pytest.main(["-xvs", __file__])