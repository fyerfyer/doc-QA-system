from fastapi import APIRouter, Response, status
from typing import Dict, Any
import platform
import psutil
import time
import os

from app.utils.utils import logger
from app.worker.processor import document_processor
from app.worker.tasks import get_redis_client

router = APIRouter(prefix="/api/health", tags=["health"])

@router.get("/")
async def health_check() -> Dict[str, Any]:
    """
    健康检查端点

    返回服务的健康状态和系统信息
    """
    start_time = time.time()

    # 基本信息
    health_data = {
        "status": "healthy",
        "service": "doc-qa-python-service",
        "version": os.environ.get("APP_VERSION", "1.0.0"),
        "timestamp": time.time(),
    }

    # 系统信息
    health_data["system"] = {
        "platform": platform.platform(),
        "python_version": platform.python_version(),
        "cpu_count": psutil.cpu_count(),
        "cpu_usage": psutil.cpu_percent(interval=0.1),
        "memory_usage_percent": psutil.virtual_memory().percent,
    }

    # 检查Redis连接
    redis_status = "connected"
    try:
        redis_client = get_redis_client()
        if not redis_client.ping():
            redis_status = "error"
    except Exception as e:
        logger.error(f"Redis health check failed: {str(e)}")
        redis_status = f"error: {str(e)}"

    health_data["dependencies"] = {
        "redis": redis_status,
    }

    # 检查嵌入模型
    embedding_model = "initialized"
    try:
        model_name = document_processor.embedder.get_model_name() if hasattr(document_processor.embedder, "get_model_name") else "unknown"
        embedding_model = {
            "status": "ready",
            "model": model_name
        }
    except Exception as e:
        logger.error(f"Embedding model health check failed: {str(e)}")
        embedding_model = {
            "status": "error",
            "error": str(e)
        }

    health_data["services"] = {
        "embedding": embedding_model,
    }

    # 计算响应时间
    health_data["response_time_ms"] = int((time.time() - start_time) * 1000)

    logger.info(f"Health check completed in {health_data['response_time_ms']}ms")
    return health_data

@router.get("/ping")
async def ping():
    """
    简单的ping检测端点

    返回pong响应，可用于简单的可用性检测
    """
    return {"ping": "pong"}