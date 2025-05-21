from fastapi import APIRouter, HTTPException, Body, Query
from typing import List, Dict, Any, Optional
import time

# 导入嵌入模型工厂和相关工具
from app.embedders.factory import create_embedder, get_default_embedder
from app.utils.utils import logger
from app.models.model import Task, TaskType, TaskStatus

# 创建路由器
router = APIRouter(prefix="/api/python", tags=["embeddings"])

@router.post("/embeddings")
async def generate_embedding(
    text: str = Body(..., embed=True, description="Text to generate embedding for"),
    model: str = Query("default", description="Embedding model name"),
    dimension: int = Query(None, description="Vector dimension (only for models that support it)")
):
    """
    生成单条文本的嵌入向量
    
    参数:
    - text: 需要生成嵌入向量的文本
    - model: 嵌入模型名称，默认为"default"
    - dimension: 向量维度(仅对支持的模型有效)
    """
    try:
        start_time = time.time()
        
        # 验证文本输入
        if not text or not isinstance(text, str):
            raise HTTPException(status_code=400, detail="Text must be a non-empty string")
        
        logger.info(f"Generating embedding for text of length {len(text)} using model '{model}'")
        
        try:
            # 创建嵌入模型实例
            kwargs = {}
            if dimension:
                kwargs["dimension"] = dimension
                
            if model == "default":
                embedder = get_default_embedder(**kwargs)
                model_used = embedder.get_model_name()
            else:
                embedder = create_embedder(model, **kwargs)
                model_used = model
                
            # 生成嵌入向量
            embedding = embedder.embed(text)
            
            # 计算处理时间
            process_time = time.time() - start_time
            
            # 返回结果
            return {
                "success": True,
                "model": model_used,
                "dimension": len(embedding),
                "embedding": embedding,
                "text_length": len(text),
                "process_time_ms": int(process_time * 1000)
            }
            
        except Exception as e:
            logger.error(f"Error generating embedding: {str(e)}")
            raise HTTPException(status_code=500, detail=f"Embedding generation failed: {str(e)}")
            
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error in embedding generation: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Internal server error: {str(e)}")

@router.post("/embeddings/batch")
async def generate_embeddings_batch(
    texts: List[str] = Body(..., embed=True, description="List of texts to generate embeddings for"),
    model: str = Query("default", description="Embedding model name"),
    dimension: int = Query(None, description="Vector dimension (only for models that support it)"),
    normalize: bool = Query(False, description="Whether to normalize vectors (L2 norm)")
):
    """
    批量生成文本嵌入向量
    
    参数:
    - texts: 需要生成嵌入向量的文本列表
    - model: 嵌入模型名称，默认为"default"
    - dimension: 向量维度(仅对支持的模型有效)
    - normalize: 是否对向量进行L2标准化
    """
    try:
        start_time = time.time()
        
        # 验证文本列表
        if not texts or not isinstance(texts, list):
            raise HTTPException(status_code=400, detail="Texts must be a non-empty list of strings")
            
        if any(not isinstance(text, str) or not text.strip() for text in texts):
            raise HTTPException(status_code=400, detail="All texts must be non-empty strings")
        
        logger.info(f"Generating embeddings for {len(texts)} texts using model '{model}'")
        
        try:
            # 创建嵌入模型实例
            kwargs = {}
            if dimension:
                kwargs["dimension"] = dimension
                
            if model == "default":
                embedder = get_default_embedder(**kwargs)
                model_used = embedder.get_model_name()
            else:
                embedder = create_embedder(model, **kwargs)
                model_used = model
                
            # 批量生成嵌入向量
            embeddings = embedder.embed_batch(texts)
            
            # 如果需要，对向量进行标准化
            if normalize:
                embeddings = embedder.normalize_vectors(embeddings)
                
            # 计算处理时间
            process_time = time.time() - start_time
            
            # 计算每个文本的长度
            text_lengths = [len(text) for text in texts]
            
            # 返回结果
            return {
                "success": True,
                "model": model_used,
                "count": len(embeddings),
                "dimension": len(embeddings[0]) if embeddings else 0,
                "embeddings": embeddings,
                "text_lengths": text_lengths,
                "normalized": normalize,
                "process_time_ms": int(process_time * 1000)
            }
            
        except Exception as e:
            logger.error(f"Error generating batch embeddings: {str(e)}")
            raise HTTPException(status_code=500, detail=f"Batch embedding generation failed: {str(e)}")
            
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error in batch embedding generation: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Internal server error: {str(e)}")

@router.get("/embeddings/models")
async def list_embedding_models():
    """
    列出所有可用的嵌入模型
    """
    try:
        from app.embedders.factory import list_available_models
        
        # 获取所有可用模型
        models = list_available_models()
        
        return {
            "success": True,
            "models": models
        }
        
    except Exception as e:
        logger.error(f"Error listing embedding models: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Failed to list embedding models: {str(e)}")

@router.post("/embeddings/similarity")
async def calculate_similarity(
    text1: str = Body(..., description="First text"),
    text2: str = Body(..., description="Second text"),
    model: str = Query("default", description="Embedding model name")
):
    """
    计算两段文本的相似度
    
    参数:
    - text1: 第一段文本
    - text2: 第二段文本
    - model: 嵌入模型名称
    """
    try:
        # 验证输入
        if not text1 or not text2:
            raise HTTPException(status_code=400, detail="Both texts must be non-empty")
            
        logger.info(f"Calculating similarity between texts of length {len(text1)} and {len(text2)}")
        
        # 创建嵌入器
        embedder = get_default_embedder() if model == "default" else create_embedder(model)
        
        # 生成两段文本的嵌入向量
        embedding1 = embedder.embed(text1)
        embedding2 = embedder.embed(text2)
        
        # 计算余弦相似度
        similarity = embedder.cosine_similarity(embedding1, embedding2)
        
        return {
            "success": True,
            "similarity": similarity,
            "model": embedder.get_model_name()
        }
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error calculating text similarity: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Similarity calculation failed: {str(e)}")