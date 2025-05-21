import os
import pytest
import dotenv
from pathlib import Path
import json
from fastapi.testclient import TestClient
from fastapi import FastAPI

from app.api.llm_api import router as llm_router
from app.main import app as main_app

# 加载环境变量
dotenv_path = Path(__file__).parent / ".env"
dotenv.load_dotenv(dotenv_path)

# 获取API密钥
API_KEY = os.getenv("DASHSCOPE_API_KEY")

# 跳过标记，当没有API密钥时跳过需要实际调用的测试
requires_api_key = pytest.mark.skipif(
    not API_KEY, reason="DashScope API key not available"
)

@pytest.fixture
def app():
    """创建测试应用的固件"""
    # 使用FastAPI创建测试应用
    app = FastAPI()
    app.include_router(llm_router)
    return app

@pytest.fixture
def client(app):
    """创建测试客户端的固件"""
    return TestClient(app)

@pytest.fixture
def main_client():
    """创建主应用测试客户端的固件"""
    return TestClient(main_app)

@requires_api_key
class TestLLMAPI:
    """LLM API测试类"""
    
    def test_generate_endpoint(self, client):
        """测试文本生成端点"""
        request_data = {
            "prompt": "What is the capital of France?",
            "model": "tongyi",
            "temperature": 0.1,
            "max_tokens": 100
        }
        
        response = client.post("/api/python/llm/generate", json=request_data)
        
        assert response.status_code == 200
        data = response.json()
        assert "text" in data
        assert len(data["text"]) > 0
        assert "Paris" in data["text"]
        assert "model" in data
        assert "prompt_tokens" in data
        assert "completion_tokens" in data
        assert "total_tokens" in data
        assert data["total_tokens"] > 0
        
        print(f"Generate response: {data['text']}")
    
    def test_chat_endpoint(self, client):
        """测试聊天端点"""
        request_data = {
            "messages": [
                {
                    "role": "system",
                    "content": "You are a helpful assistant."
                },
                {
                    "role": "user",
                    "content": "What is the capital of France?"
                }
            ],
            "model": "tongyi",
            "temperature": 0.1,
            "max_tokens": 100
        }
        
        response = client.post("/api/python/llm/chat", json=request_data)
        
        assert response.status_code == 200
        data = response.json()
        assert "text" in data
        assert len(data["text"]) > 0
        assert "Paris" in data["text"]
        assert "model" in data
        assert "total_tokens" in data
        assert data["total_tokens"] > 0
        
        print(f"Chat response: {data['text']}")
    
    def test_generate_with_invalid_params(self, client):
        """测试使用无效参数的生成端点"""
        # 空提示
        request_data = {
            "prompt": "",
            "model": "tongyi"
        }
        
        response = client.post("/api/python/llm/generate", json=request_data)
        assert response.status_code in [400, 422, 500]  # 视实现而定
        
        # 无效模型名称也应该有适当的错误处理
        request_data = {
            "prompt": "Test",
            "model": "invalid_model_name"
        }
        
        response = client.post("/api/python/llm/generate", json=request_data)
        assert response.status_code in [400, 422, 500]  # 视实现而定
    
    def test_rag_endpoint(self, client, monkeypatch):
        """测试RAG端点"""
        # 模拟向量搜索结果
        async def mock_ask_vector_db(*args, **kwargs):
            from app.llm.model import SearchResult
            return [
                SearchResult(
                    text="Paris is the capital and most populous city of France.",
                    score=0.95,
                    metadata={"source": "geography.txt"},
                    document_id="doc1"
                ),
                SearchResult(
                    text="France is a country in Western Europe with several overseas territories.",
                    score=0.9,
                    metadata={"source": "geography.txt"},
                    document_id="doc1"
                )
            ]
        
        # 使用monkeypatch替换实际的向量搜索方法
        import app.llm.rag
        monkeypatch.setattr(app.llm.rag.RAG, "ask_vector_db", mock_ask_vector_db)
        
        request_data = {
            "query": "What is the capital of France?",
            "document_ids": ["doc1"],
            "model": "tongyi",
            "temperature": 0.1,
            "max_tokens": 100
        }
        
        response = client.post("/api/python/llm/rag", json=request_data)
        
        assert response.status_code == 200
        data = response.json()
        assert "text" in data
        assert len(data["text"]) > 0
        assert "Paris" in data["text"]
        assert "sources" in data
        assert len(data["sources"]) > 0
        assert "model" in data
        assert "total_tokens" in data
        
        print(f"RAG response: {data['text']}")
        print(f"Sources: {json.dumps(data['sources'], indent=2)}")

    def test_generate_stream_partial(self, client):
        """测试流式生成端点（部分验证）"""
        request_data = {
            "prompt": "Count from 1 to 5",
            "model": "tongyi",
            "temperature": 0.1,
            "max_tokens": 100,
            "stream": True
        }
        
        with client.stream("POST", "/api/python/llm/generate", json=request_data) as response:
            # 验证响应是SSE流
            assert response.status_code == 200
            assert "text/event-stream" in response.headers["content-type"] 
            
            # 验证至少收到了一些数据
            received_data = False
            for line in response.iter_lines():
                # 处理可能的字节/字符串类型不匹配
                if isinstance(line, bytes):
                    line_str = line.decode('utf-8')
                else:
                    line_str = line
                    
                if line_str.startswith('data:'):
                    received_data = True
                    # 验证数据是有效的JSON
                    data_str = line_str[5:].strip()
                    if data_str == "[DONE]":
                        continue
                    try:
                        data = json.loads(data_str)
                        assert "type" in data
                        if data["type"] == "content":
                            assert "text" in data
                            break  # 只需验证一个有效的数据块
                    except json.JSONDecodeError:
                        pass  # 忽略无效JSON
            
            assert received_data


if __name__ == "__main__":
    pytest.main(["-xvs", __file__])