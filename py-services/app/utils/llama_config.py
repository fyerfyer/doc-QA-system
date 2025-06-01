from typing import Optional, Dict, Any

from llama_index.core import Settings
from loguru import logger

# 文本分割的默认配置
DEFAULT_CHUNK_SIZE = 1000
DEFAULT_CHUNK_OVERLAP = 200
DEFAULT_SPLIT_TYPE = "sentence"  # 可选: sentence, paragraph, token


def configure_llm(llm_model: str = "gpt-3.5-turbo", 
                  temperature: float = 0.1, 
                  **kwargs) -> None:
    """
    配置LlamaIndex使用的LLM模型
    
    参数:
        llm_model: LLM模型名称
        temperature: 生成温度
        **kwargs: 传递给LLM的其他参数
    """
    try:
        # 默认尝试使用OpenAI模型
        from llama_index.llms.openai import OpenAI
        Settings.llm = OpenAI(model=llm_model, temperature=temperature, **kwargs)
        logger.info(f"Configured LLM: {llm_model}")
    except ImportError:
        logger.warning("OpenAI package not found. Please install llama-index-llms-openai")
        raise


def configure_embedding_model(model_name: str = "text-embedding-3-small",
                             embed_batch_size: int = 10,
                             **kwargs) -> None:
    """
    配置LlamaIndex使用的嵌入模型
    
    参数:
        model_name: 嵌入模型名称
        embed_batch_size: 批处理大小
        **kwargs: 传递给嵌入模型的其他参数
    """
    try:
        # 默认尝试使用OpenAI嵌入
        from llama_index.embeddings.openai import OpenAIEmbedding
        Settings.embed_model = OpenAIEmbedding(
            model=model_name, 
            embed_batch_size=embed_batch_size,
            **kwargs
        )
        logger.info(f"Configured embedding model: {model_name}, batch_size: {embed_batch_size}")
    except ImportError:
        # 如果OpenAI不可用，尝试使用HuggingFace嵌入
        try:
            from llama_index.embeddings.huggingface import HuggingFaceEmbedding
            Settings.embed_model = HuggingFaceEmbedding(
                model_name=model_name,
                **kwargs
            )
            logger.info(f"Configured HuggingFace embedding model: {model_name}")
        except ImportError:
            logger.warning("No embedding package found. Please install llama-index-embeddings-openai or llama-index-embeddings-huggingface")
            raise


def configure_text_splitter(split_type: str = DEFAULT_SPLIT_TYPE,
                          chunk_size: int = DEFAULT_CHUNK_SIZE,
                          chunk_overlap: int = DEFAULT_CHUNK_OVERLAP,
                          **kwargs) -> None:
    """
    配置LlamaIndex使用的文本分割器
    
    参数:
        split_type: 分割类型 (sentence, paragraph, token)
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        **kwargs: 传递给分割器的其他参数
    """
    # 设置全局块大小和重叠
    Settings.chunk_size = chunk_size
    Settings.chunk_overlap = chunk_overlap
    
    if split_type.lower() == "sentence":
        from llama_index.core.node_parser import SentenceSplitter
        Settings.text_splitter = SentenceSplitter(
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            **kwargs
        )
        logger.info(f"Configured SentenceSplitter with chunk_size={chunk_size}, overlap={chunk_overlap}")
    
    elif split_type.lower() == "paragraph":
        from llama_index.core.node_parser import SentenceSplitter
        Settings.text_splitter = SentenceSplitter(
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            paragraph_separator="\n\n",
            **kwargs
        )
        logger.info(f"Configured paragraph-based SentenceSplitter with chunk_size={chunk_size}, overlap={chunk_overlap}")
    
    elif split_type.lower() == "token":
        from llama_index.core.node_parser import TokenTextSplitter
        Settings.text_splitter = TokenTextSplitter(
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            **kwargs
        )
        logger.info(f"Configured TokenTextSplitter with chunk_size={chunk_size}, overlap={chunk_overlap}")
    
    else:
        raise ValueError(f"Unsupported split_type: {split_type}. Use 'sentence', 'paragraph', or 'token'")


def configure_all(llm_model: str = "gpt-3.5-turbo",
                embedding_model: str = "text-embedding-3-small",
                split_type: str = DEFAULT_SPLIT_TYPE,
                chunk_size: int = DEFAULT_CHUNK_SIZE,
                chunk_overlap: int = DEFAULT_CHUNK_OVERLAP) -> None:
    """
    一次性配置所有LlamaIndex设置
    
    参数:
        llm_model: LLM模型名称
        embedding_model: 嵌入模型名称
        split_type: 分割类型
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
    """
    configure_text_splitter(split_type, chunk_size, chunk_overlap)
    
    try:
        configure_embedding_model(embedding_model)
    except Exception as e:
        logger.error(f"Failed to configure embedding model: {str(e)}")
    
    try:
        configure_llm(llm_model)
    except Exception as e:
        logger.error(f"Failed to configure LLM: {str(e)}")
    
    logger.info("LlamaIndex configuration complete")


def get_current_config() -> Dict[str, Any]:
    """
    获取当前LlamaIndex配置
    
    返回:
        Dict[str, Any]: 当前配置信息
    """
    config = {
        "chunk_size": Settings.chunk_size,
        "chunk_overlap": Settings.chunk_overlap,
        "llm": str(Settings.llm) if Settings.llm else None,
        "embed_model": str(Settings.embed_model) if Settings.embed_model else None,
        "text_splitter": str(Settings.text_splitter) if Settings.text_splitter else None,
    }
    return config