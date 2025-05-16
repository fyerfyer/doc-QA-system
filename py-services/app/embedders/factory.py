import logging
import os
from typing import Dict, Any, List

from app.embedders.base import BaseEmbedder
from app.embedders.tongyi import TongyiEmbedder
from app.embedders.huggingface import HuggingFaceEmbedder

# 初始化日志记录器
logger = logging.getLogger(__name__)

# 注册可用的嵌入模型
EMBEDDER_REGISTRY = {
    # 通义千问模型
    "tongyi": TongyiEmbedder,
    "text-embedding-v1": TongyiEmbedder,
    "text-embedding-v2": TongyiEmbedder,
    "text-embedding-v3": TongyiEmbedder,
    "dashscope": TongyiEmbedder,
    
    # 添加默认入口
    "default": TongyiEmbedder,

    # HuggingFace模型
    "huggingface": HuggingFaceEmbedder,
    "sentence-transformer": HuggingFaceEmbedder,
    "all-minilm-l6-v2": HuggingFaceEmbedder,
    "all-mpnet-base-v2": HuggingFaceEmbedder,
    "paraphrase-multilingual-minilm-l12-v2": HuggingFaceEmbedder,
}

def create_embedder(embedder_type: str = "tongyi", **kwargs) -> BaseEmbedder:
    """
    创建嵌入模型实例

    参数:
        embedder_type: 嵌入模型类型或模型名称
        **kwargs: 传递给嵌入模型构造函数的参数

    返回:
        BaseEmbedder: 嵌入模型实例

    异常:
        ValueError: 如果嵌入模型类型不受支持
    """
    # 处理 "default" 特殊情况
    if embedder_type and embedder_type.lower() == "default":
        logger.info("Using default embedder as specified by 'default' model name")
        return get_default_embedder(**kwargs)
        
    # 默认使用通义千问
    if not embedder_type:
        embedder_type = "tongyi"

    # 转换为小写以进行不区分大小写的匹配
    embedder_type_lower = embedder_type.lower()

    # 查找嵌入模型类
    embedder_class = EMBEDDER_REGISTRY.get(embedder_type_lower)

    # 如果没有直接匹配，尝试进行部分匹配
    if not embedder_class:
        for key, cls in EMBEDDER_REGISTRY.items():
            if key in embedder_type_lower or embedder_type_lower in key:
                embedder_class = cls
                logger.info(f"Using partial match: '{embedder_type}' -> '{key}'")
                break

    # 如果仍然找不到匹配项
    if not embedder_class:
        raise ValueError(f"Unsupported embedder type: {embedder_type}")

    # 创建实例
    try:
        logger.info(f"Creating embedder of type: {embedder_type}")

        # 对于通义千问模型，自动添加API密钥（如果未提供）
        if embedder_class == TongyiEmbedder and "api_key" not in kwargs:
            api_key = os.environ.get("DASHSCOPE_API_KEY")
            if api_key:
                kwargs["api_key"] = api_key

        # 对于HuggingFace模型，自动处理模型名称
        if embedder_class == HuggingFaceEmbedder and embedder_type_lower != "huggingface":
            # 如果直接指定了模型名称但没有提供model_name参数
            if "model_name" not in kwargs and embedder_type_lower in EMBEDDER_REGISTRY:
                if embedder_type_lower.startswith("all-") or embedder_type_lower.startswith("paraphrase-"):
                    kwargs["model_name"] = f"sentence-transformers/{embedder_type}"

        return embedder_class(**kwargs)
    except Exception as e:
        logger.error(f"Failed to create embedder of type {embedder_type}: {str(e)}")
        raise

def get_default_embedder(**kwargs) -> BaseEmbedder:
    """
    获取默认的嵌入模型实例

    参数:
        **kwargs: 传递给嵌入模型构造函数的参数

    返回:
        BaseEmbedder: 默认的嵌入模型实例
    """
    # 如果有通义千问API密钥，优先使用通义千问
    if os.environ.get("DASHSCOPE_API_KEY") or "api_key" in kwargs:
        return create_embedder("tongyi", **kwargs)

    # 否则使用本地HuggingFace模型
    return create_embedder("huggingface", **kwargs)

def list_available_models() -> Dict[str, List[Dict[str, Any]]]:
    """
    列出所有可用的嵌入模型

    返回:
        Dict[str, List[Dict[str, Any]]]: 按类型分组的模型列表
    """
    # 创建临时实例以获取模型信息
    tongyi_models = [
        {"name": "text-embedding-v1", "type": "tongyi", "remote": True},
        {"name": "text-embedding-v2", "type": "tongyi", "remote": True},
        {"name": "text-embedding-v3", "type": "tongyi", "remote": True,
         "dimensions": [1024, 768, 512, 256, 128, 64]}
    ]

    # 尝试创建HuggingFace实例并获取支持的模型
    try:
        hf_embedder = HuggingFaceEmbedder()
        huggingface_models = hf_embedder.get_supported_models()
    except:
        huggingface_models = [
            {"name": "all-MiniLM-L6-v2", "dimension": 384, "description": "一个小型通用模型，速度快"},
            {"name": "all-mpnet-base-v2", "dimension": 768, "description": "一个效果更好的通用模型，但速度较慢"},
        ]

    return {
        "tongyi": tongyi_models,
        "huggingface": huggingface_models
    }