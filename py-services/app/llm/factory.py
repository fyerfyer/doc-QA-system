import logging
import os
from typing import Dict, Any, List

from app.llm.base import BaseLLM
from app.llm.tongyi import TongyiLLM

# 初始化日志记录器
logger = logging.getLogger(__name__)

# 注册可用的LLM模型
LLM_REGISTRY = {
    # 通义千问模型
    "tongyi": TongyiLLM,
    "qwen-turbo": TongyiLLM,
    "qwen-plus": TongyiLLM,
    "qwen-max": TongyiLLM,
    "dashscope": TongyiLLM,
    
    # 添加默认入口
    "default": TongyiLLM,
}

def create_llm(model_type: str = "tongyi", **kwargs) -> BaseLLM:
    """
    创建LLM模型实例

    参数:
        model_type: LLM模型类型或模型名称
        **kwargs: 传递给LLM模型构造函数的参数

    返回:
        BaseLLM: LLM模型实例

    异常:
        ValueError: 如果LLM模型类型不受支持
    """
    # 处理 "default" 特殊情况
    if model_type and model_type.lower() == "default":
        logger.info("Using default LLM as specified by 'default' model name")
        return get_default_llm(**kwargs)
        
    # 默认使用通义千问
    if not model_type:
        model_type = "tongyi"

    # 转换为小写以进行不区分大小写的匹配
    model_type_lower = model_type.lower()

    # 查找LLM模型类
    llm_class = LLM_REGISTRY.get(model_type_lower)

    # 如果没有直接匹配，尝试进行部分匹配
    if not llm_class:
        for key, cls in LLM_REGISTRY.items():
            if key in model_type_lower or model_type_lower in key:
                llm_class = cls
                logger.info(f"Using partial match: '{model_type}' -> '{key}'")
                break

    # 如果仍然找不到匹配项
    if not llm_class:
        raise ValueError(f"Unsupported LLM model type: {model_type}")

    # 创建实例
    try:
        logger.info(f"Creating LLM of type: {model_type}")

        # 对于通义千问模型，自动添加API密钥（如果未提供）
        if llm_class == TongyiLLM and "api_key" not in kwargs:
            api_key = os.environ.get("DASHSCOPE_API_KEY")
            if api_key:
                kwargs["api_key"] = api_key
            
        # 如果指定了模型名称但没有具体的model_name参数
        if "model_name" not in kwargs and model_type_lower != "tongyi" and model_type_lower != "default":
            if model_type_lower in ["qwen-turbo", "qwen-plus", "qwen-max"]:
                kwargs["model_name"] = model_type

        return llm_class(**kwargs)
    except Exception as e:
        logger.error(f"Failed to create LLM of type {model_type}: {str(e)}")
        raise

def get_default_llm(**kwargs) -> BaseLLM:
    """
    获取默认的LLM实例

    参数:
        **kwargs: 传递给LLM构造函数的参数

    返回:
        BaseLLM: 默认LLM实例
    """
    # 如果有通义千问API密钥，优先使用通义千问
    if os.environ.get("DASHSCOPE_API_KEY") or "api_key" in kwargs:
        return create_llm("tongyi", **kwargs)

    # TODO: 如果没有API密钥，可以使用其他备选方案，例如本地模型
    # 目前我们只支持通义千问，所以还是返回通义千问
    # 未来可以添加其他模型实现，比如OpenAI、本地模型等
    logger.warning("No DashScope API key found, LLM functionality may be limited")
    return create_llm("tongyi", **kwargs)

def list_available_models() -> Dict[str, List[Dict[str, Any]]]:
    """
    列出所有可用的LLM模型

    返回:
        Dict[str, List[Dict[str, Any]]]: 按类型分组的模型列表
    """
    # 通义千问模型列表
    tongyi_models = [
        {"name": "qwen-turbo", "type": "tongyi", "description": "通义千问Turbo模型，平衡速度和性能"},
        {"name": "qwen-plus", "type": "tongyi", "description": "通义千问Plus模型，提供更好的理解和生成能力"},
        {"name": "qwen-max", "type": "tongyi", "description": "通义千问Max模型，最强大的理解和生成能力"},
        {"name": "qwen-max-longcontext", "type": "tongyi", "description": "通义千问Max-LongContext模型，支持更长的上下文"}
    ]

    # 未来可以添加其他模型类型
    return {
        "tongyi": tongyi_models,
    }

def get_model_details(model_name: str) -> Dict[str, Any]:
    """
    获取特定模型的详细信息

    参数:
        model_name: 模型名称

    返回:
        Dict[str, Any]: 模型详情
        
    异常:
        ValueError: 如果模型不存在
    """
    models_by_type = list_available_models()
    
    # 搜索所有模型类型
    for model_type, models in models_by_type.items():
        for model in models:
            if model["name"].lower() == model_name.lower():
                return {
                    **model,
                    "model_type": model_type,
                }
    
    raise ValueError(f"Model '{model_name}' not found")