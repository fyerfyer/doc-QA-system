import os
import pytest
import dotenv
from pathlib import Path

from app.llm.factory import create_llm, get_default_llm
from app.llm.tongyi import TongyiLLM
from app.llm.model import RAGRequest, SearchResult
from app.llm.rag import RAG
from app.embedders.factory import create_embedder

# 加载环境变量
dotenv_path = Path(__file__).parent / ".env"
dotenv.load_dotenv(dotenv_path)

# 获取API密钥
API_KEY = os.getenv("DASHSCOPE_API_KEY")

# 跳过标记，当没有API密钥时跳过需要实际调用的测试
requires_api_key = pytest.mark.skipif(
    not API_KEY, reason="DashScope API key not available"
)

class TestLLMFactory:
    """LLM工厂测试类"""
    
    def test_create_llm(self):
        """测试创建LLM实例"""
        llm = create_llm("tongyi", api_key=API_KEY)
        assert llm is not None
        assert isinstance(llm, TongyiLLM)
        assert llm.get_model_name() == "qwen-turbo"
        
    def test_create_llm_with_model_name(self):
        """测试创建指定模型名称的LLM实例"""
        llm = create_llm("qwen-plus", api_key=API_KEY)
        assert llm is not None
        assert isinstance(llm, TongyiLLM)
        assert llm.get_model_name() == "qwen-plus"
        
    def test_get_default_llm(self):
        """测试获取默认LLM实例"""
        os.environ["DASHSCOPE_API_KEY"] = API_KEY or "dummy_key"
        llm = get_default_llm()
        assert llm is not None
        assert isinstance(llm, TongyiLLM)


@requires_api_key
class TestTongyiLLM:
    """通义千问LLM测试类"""
    
    @pytest.fixture
    def llm(self):
        """创建LLM实例的测试固件"""
        return TongyiLLM(api_key=API_KEY, temperature=0.1, max_tokens=100)
    
    def test_initialization(self, llm):
        """测试初始化"""
        assert llm is not None
        assert llm.get_model_name() == "qwen-turbo"
        assert llm.temperature == 0.1
        assert llm.max_tokens == 100
    
    def test_generate(self, llm):
        """测试文本生成"""
        prompt = "Give me a short greeting in English"
        response = llm.generate(prompt)
        
        assert response is not None
        assert isinstance(response, str)
        assert len(response) > 0
        print(f"Generated response: {response}")
    
    def test_chat(self, llm):
        """测试聊天功能"""
        messages = [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Say hello in three different languages"},
        ]
        
        response = llm.chat(messages)
        
        assert response is not None
        assert isinstance(response, str)
        assert len(response) > 0
        print(f"Chat response: {response}")
    
    def test_generate_stream(self, llm):
        """测试流式文本生成"""
        prompt = "List three benefits of automated testing"
        chunks = []
        
        for chunk in llm.generate_stream(prompt):
            assert chunk is not None
            chunks.append(chunk)
            
        assert len(chunks) > 0
        full_text = "".join(chunks)
        assert len(full_text) > 0
        print(f"Streamed response total chunks: {len(chunks)}")
        print(f"First chunk: {chunks[0]}")
        print(f"Last chunk: {chunks[-1]}")
    
    def test_chat_stream(self, llm):
        """测试流式聊天功能"""
        messages = [
            {"role": "user", "content": "What's the capital of France?"}
        ]
        
        chunks = []
        for chunk in llm.chat_stream(messages):
            assert chunk is not None
            chunks.append(chunk)
            
        assert len(chunks) > 0
        full_text = "".join(chunks)
        assert len(full_text) > 0
        assert "Paris" in full_text
        print(f"Streamed chat total chunks: {len(chunks)}")
        
    def test_model_parameters(self, llm):
        """测试获取模型参数"""
        params = llm.get_model_parameters()
        assert params is not None
        assert "model" in params
        assert "temperature" in params
        assert params["temperature"] == 0.1


@requires_api_key
class TestRAG:
    """RAG功能测试类"""
    
    @pytest.fixture
    def mock_search_results(self):
        """创建模拟搜索结果的测试固件"""
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
            ),
            SearchResult(
                text="The Eiffel Tower is a wrought-iron lattice tower in Paris, France.",
                score=0.85,
                metadata={"source": "landmarks.txt"},
                document_id="doc2"
            )
        ]
    
    @pytest.fixture
    def rag(self):
        """创建RAG实例的测试固件"""
        llm = create_llm("tongyi", api_key=API_KEY, temperature=0.1, max_tokens=100)
        embedder = create_embedder("text-embedding-v3", api_key=API_KEY)
        return RAG(llm=llm, embedder=embedder)
    
    def test_format_context(self, rag, mock_search_results):
        """测试上下文格式化"""
        context = rag.format_context(mock_search_results)
        assert context is not None
        assert isinstance(context, str)
        assert "Paris is the capital" in context
        assert "France is a country" in context
        
        # 测试带来源信息的格式化
        context_with_sources = rag.format_context(mock_search_results, add_source_info=True)
        assert "[SOURCE_1]" in context_with_sources or "[来源_1]" in context_with_sources
    
    def test_apply_template(self, rag):
        """测试应用提示模板"""
        context = "Paris is the capital of France."
        question = "What is the capital of France?"
        
        prompt = rag.apply_template(question, context)
        assert prompt is not None
        assert isinstance(prompt, str)
        assert context in prompt
        assert question in prompt
    
    @pytest.mark.asyncio
    async def test_generate_answer(self, rag, mock_search_results):
        """测试生成增强回答"""
        query = "What is the capital of France?"
        
        response = await rag.generate_answer(query, mock_search_results)
        
        assert response is not None
        assert response.text is not None
        assert isinstance(response.text, str)
        assert len(response.text) > 0
        print(f"RAG response: {response.text}")
    
    @pytest.mark.asyncio
    async def test_query_with_empty_results(self, rag):
        """测试当没有搜索结果时的查询行为"""
        request = RAGRequest(
            query="What is the population of Mars?",
            document_ids=["nonexistent"],
            model="tongyi"
        )
        
        # 这应该返回一个表示没有足够信息的标准响应
        response = await rag.query(request)
        assert response is not None
        assert response.text is not None
        
        assert any(phrase in response.text.lower() for phrase in [
            "没有足够的信息", 
            "not enough information", 
            "don't have enough information",
            "insufficient information",
            "cannot answer"
        ])


if __name__ == "__main__":
    pytest.main(["-xvs", __file__])