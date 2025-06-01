import os
import pytest
import numpy as np
from dotenv import load_dotenv, find_dotenv

# 添加代理设置
HTTP_PROXY = os.getenv("HTTP_PROXY", "http://127.0.0.1:7897")
HTTPS_PROXY = os.getenv("HTTPS_PROXY", "http://127.0.0.1:7897")
PROXIES = {
    "http": HTTP_PROXY,
    "https": HTTPS_PROXY
}

from app.embedders.factory import create_embedder, get_default_embedder, list_available_models
from app.embedders.tongyi import TongyiEmbedder
from app.embedders.huggingface import HuggingFaceEmbedder
from app.embedders.base import BaseEmbedder

def get_api_key():
    _ = load_dotenv(find_dotenv())
    return os.environ['DASHSCOPE_API_KEY']

# 加载环境变量
load_dotenv()

# 获取API密钥
API_KEY = get_api_key()

# 测试文本
TEST_TEXTS = [
    "中国是一个拥有悠久历史和文化的国家。",  # 中文主题完全不同
    "The weather today is sunny and warm.",  # 英文主题完全不同
    "中国有很多传统节日和习俗。",  # 与第一句相关的中文
    "English is widely spoken around the world." # 英文
]


# 测试工厂函数
def test_embedder_factory():
    """测试嵌入器工厂函数能否正确创建不同类型的嵌入器"""

    # 测试创建通义千问嵌入器
    tongyi_embedder = create_embedder("tongyi", api_key=API_KEY)
    assert isinstance(tongyi_embedder, TongyiEmbedder)

    # 测试创建Hugging Face嵌入器
    hf_embedder = create_embedder("huggingface")
    assert isinstance(hf_embedder, HuggingFaceEmbedder)

    # 测试获取默认嵌入器
    default_embedder = get_default_embedder(api_key=API_KEY)
    assert isinstance(default_embedder, BaseEmbedder)

    # 测试列出可用模型
    models = list_available_models()
    assert isinstance(models, dict)
    assert "tongyi" in models
    assert "huggingface" in models


# 测试通义千问嵌入器
@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_tongyi_embedder_basic():
    """测试通义千问嵌入器的基本功能"""

    embedder = TongyiEmbedder(api_key=API_KEY)

    # 测试单个文本嵌入
    vector = embedder.embed(TEST_TEXTS[0])
    assert isinstance(vector, list)
    assert len(vector) == embedder.dimension

    # 确保所有值都是浮点数
    assert all(isinstance(x, float) for x in vector)

    # 检查向量质量（不应该全是0或相同的值）
    assert len(set([round(x, 5) for x in vector])) > 10  # 至少有10个不同的值


@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_tongyi_embedder_batch():
    """测试通义千问嵌入器的批处理功能"""

    embedder = TongyiEmbedder(api_key=API_KEY)

    # 测试批量文本嵌入
    vectors = embedder.embed_batch(TEST_TEXTS)
    assert isinstance(vectors, list)
    assert len(vectors) == len(TEST_TEXTS)

    # 检查每个向量
    for vector in vectors:
        assert isinstance(vector, list)
        assert len(vector) == embedder.dimension
        assert all(isinstance(x, float) for x in vector)

    # 计算相似度矩阵（应该相似文本之间的相似度更高）
    similarity_matrix = []
    for i in range(len(vectors)):
        row = []
        for j in range(len(vectors)):
            sim = embedder.cosine_similarity(vectors[i], vectors[j])
            row.append(sim)
        similarity_matrix.append(row)

    # 检查相似度矩阵（自身相似度应该是1）
    for i in range(len(vectors)):
        assert abs(similarity_matrix[i][i] - 1.0) < 1e-6

    # 检查中文句子之间的相似度应该高于中文和英文之间
    assert similarity_matrix[0][2] > similarity_matrix[0][1]
    assert similarity_matrix[0][2] > similarity_matrix[0][3]


@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_tongyi_embedder_with_metadata():
    """测试通义千问嵌入器带元数据功能"""

    embedder = TongyiEmbedder(api_key=API_KEY)

    # 测试带元数据的嵌入
    metadata = {"source": "test", "category": "general"}
    result = embedder.embed_with_metadata(TEST_TEXTS[0], metadata)

    assert isinstance(result, dict)
    assert "embedding" in result
    assert "dimension" in result
    assert "model" in result
    assert "metadata" in result
    assert result["metadata"] == metadata


@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_tongyi_embedder_different_dimensions():
    """测试通义千问嵌入器不同维度的支持"""

    # 测试不同的维度配置
    dimensions = [512, 256]
    for dim in dimensions:
        embedder = TongyiEmbedder(api_key=API_KEY, dimension=dim)
        vector = embedder.embed(TEST_TEXTS[0])
        assert len(vector) == dim


# 测试Hugging Face嵌入器
def test_huggingface_embedder_basic():
    """测试Hugging Face嵌入器的基本功能"""
    
    try:
        # 设置代理并优先使用本地缓存
        embedder = HuggingFaceEmbedder(
            model_name="sentence-transformers/all-MiniLM-L6-v2",
            proxies=PROXIES,  # 添加代理
            local_files_only=True  # 优先使用本地缓存
        )
        
        # 尝试获取嵌入向量
        try:
            vector = embedder.embed(TEST_TEXTS[0])
            assert isinstance(vector, list)
            assert len(vector) == embedder.dimension
            
            # 确保所有值都是浮点数
            assert all(isinstance(x, float) for x in vector)
            
            # 检查向量质量
            assert len(set([round(x, 5) for x in vector])) > 10
        except Exception as e:
            if "connection" in str(e).lower() or "timeout" in str(e).lower() or "offline" in str(e).lower():
                pytest.skip(f"Network issues when accessing HuggingFace: {str(e)}")
            else:
                raise
    except ImportError:
        pytest.skip("sentence-transformers package not installed")


def test_huggingface_embedder_batch():
    """测试Hugging Face嵌入器的批处理功能"""
    
    try:
        # 使用小模型进行批处理测试，并优先使用本地缓存
        embedder = HuggingFaceEmbedder(
            model_name="sentence-transformers/all-MiniLM-L6-v2",
            proxies=PROXIES,
            local_files_only=True  # 优先使用本地缓存
        )
        
        try:
            # 测试批量文本嵌入
            vectors = embedder.embed_batch(TEST_TEXTS)
            assert isinstance(vectors, list)
            assert len(vectors) == len(TEST_TEXTS)
            
            # 检查每个向量
            for vector in vectors:
                assert isinstance(vector, list)
                assert len(vector) == embedder.dimension
                assert all(isinstance(x, float) for x in vector)
            
            # 卸载模型释放内存
            embedder.unload_model()
        except Exception as e:
            if "connection" in str(e).lower() or "timeout" in str(e).lower() or "offline" in str(e).lower():
                pytest.skip(f"Network issues when accessing HuggingFace: {str(e)}")
            else:
                raise
    except ImportError:
        pytest.skip("sentence-transformers package not installed")


# 边缘情况测试
@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_edge_cases():
    """测试各种边缘情况"""

    embedder = TongyiEmbedder(api_key=API_KEY)

    # 测试空字符串
    with pytest.raises(ValueError):
        embedder.embed("")

    # 测试空列表
    with pytest.raises(ValueError):
        embedder.embed_batch([])

    # 测试包含无效值的列表
    with pytest.raises(ValueError):
        embedder.embed_batch(["有效文本", "", None])


# 向量标准化测试
@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_vector_normalization():
    """测试向量标准化功能"""

    embedder = TongyiEmbedder(api_key=API_KEY)

    # 获取原始向量
    vectors = embedder.embed_batch(TEST_TEXTS[:2])

    # 标准化向量
    normalized = embedder.normalize_vectors(vectors)

    # 检查标准化后的向量长度应该接近1
    for vector in normalized:
        norm = np.linalg.norm(vector)
        assert abs(norm - 1.0) < 1e-6


@pytest.mark.skipif(not API_KEY, reason="DASHSCOPE_API_KEY is required")
def test_tongyi_openai_compatible():
    """测试通义千问OpenAI兼容接口"""

    try:
        import openai
        embedder = TongyiEmbedder(api_key=API_KEY)

        # 测试单个文本
        vector = embedder.embed_with_openai_compatible(TEST_TEXTS[0])
        assert isinstance(vector, list)
        assert len(vector) == embedder.dimension

        # 测试多个文本
        vectors = embedder.embed_with_openai_compatible(TEST_TEXTS)
        assert isinstance(vectors, list)
        assert len(vectors) == len(TEST_TEXTS)
    except ImportError:
        pytest.skip("openai package not installed")


if __name__ == "__main__":
    # 手动运行测试时，确保加载了环境变量
    if not API_KEY:
        print("警告: 未找到DASHSCOPE_API_KEY环境变量，部分测试将被跳过")

    # 运行所有测试
    pytest.main(["-xvs", __file__])