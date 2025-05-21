import os
import time
from typing import Dict, Any, List
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI, HTTPException, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from app.models.model import (
    Task, TaskType, TaskStatus,
    DocumentParsePayload, TextChunkPayload,
    VectorizePayload, ProcessCompletePayload
)
from app.utils.utils import setup_logger, logger, get_task_key
from app.worker.tasks import (
    parse_document, chunk_text, vectorize_text, process_document,
    get_redis_client, get_task_from_redis
)

# 导入现有API路由器
from app.api.health import router as health_router
from app.api.callback import router as callback_router

# 导入新的API路由器
from app.api.document_api import router as document_router
from app.api.chunking_api import router as chunking_router
from app.api.embedding_api import router as embedding_api_router

from app.utils.route_display import print_routes

# 初始化日志
setup_logger(os.getenv("LOG_LEVEL", "INFO"))

# 定义生命周期管理器函数
@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    # 启动时执行的操作
    logger.info("Document Processing API started")

    # 检查Redis连接
    try:
        client = get_redis_client()
        if client.ping():
            logger.info("Successfully connected to Redis")
        else:
            logger.error("Failed to ping Redis")
    except Exception as e:
        logger.error(f"Error connecting to Redis: {str(e)}")

    # 获取并打印所有API路由
    print_routes(app, logger)

    yield

    # 关闭时执行的操作
    logger.info("Document Processing API shutting down")

# 创建FastAPI应用
app = FastAPI(
    title="Document Processing API",
    description="API for document parsing, chunking, and embedding",
    version="1.0.0",
    lifespan=lifespan,  # 添加 lifespan 参数
)

# 配置CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # 在生产环境中应该限制为特定域名
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# 初始化MinIO客户端
from app.utils.minio_client import get_minio_client
try:
    minio = get_minio_client()
    logger.info(f"MinIO connection initialized: {minio.endpoint}, bucket: {minio.bucket}")
except Exception as e:
    logger.error(f"Failed to initialize MinIO client: {str(e)}")

# 注册现有的API路由器
app.include_router(health_router)
app.include_router(callback_router)

# 注册新的API路由器
app.include_router(document_router)
app.include_router(chunking_router)
app.include_router(embedding_api_router)

# API请求模型
class DocumentParseRequest(BaseModel):
    file_path: str
    file_name: str
    file_type: str
    document_id: str
    metadata: Dict[str, str] = Field(default_factory=dict)

class TextChunkRequest(BaseModel):
    document_id: str
    content: str
    chunk_size: int = 1000
    overlap: int = 200
    split_type: str = "text"

class VectorizeRequest(BaseModel):
    document_id: str
    chunks: List[Dict[str, Any]]
    model: str = "default"

class ProcessCompleteRequest(BaseModel):
    document_id: str
    file_path: str
    file_name: str
    file_type: str
    chunk_size: int = 1000
    overlap: int = 200
    split_type: str = "text"
    model: str = "default"
    metadata: Dict[str, str] = Field(default_factory=dict)

class TaskResponse(BaseModel):
    task_id: str
    status: str = "pending"
    task_type: str

# 健康检查端点
@app.get("/health")
async def health_check():
    """健康检查接口"""
    return {"status": "healthy", "timestamp": time.time()}

# 文档解析任务端点
@app.post("/api/tasks/parse", response_model=TaskResponse)
async def create_parse_task(request: DocumentParseRequest):
    """创建文档解析任务"""
    try:
        # 创建任务载荷
        payload = DocumentParsePayload(
            file_path=request.file_path,
            file_name=request.file_name,
            file_type=request.file_type,
            metadata=request.metadata
        )

        # 创建任务记录
        task_id = f"parse_{int(time.time())}_{request.document_id}"

        # 创建任务对象
        task = Task(
            id=task_id,
            type=TaskType.DOCUMENT_PARSE,
            document_id=request.document_id,
            status=TaskStatus.PENDING,
            payload=payload.__dict__
        )

        # 保存任务到Redis
        client = get_redis_client()
        key = get_task_key(task_id)
        client.set(key, task.to_json())

        # 添加到文档任务集合
        doc_key = f"document_tasks:{request.document_id}"
        client.sadd(doc_key, task_id)

        # 发送到Celery队列
        celery_task = parse_document.delay(task_id)

        logger.info(f"Created document parse task: {task_id}")
        return {
            "task_id": task_id,
            "status": "pending",
            "task_type": "document_parse"
        }

    except Exception as e:
        logger.error(f"Failed to create parse task: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to create document parse task: {str(e)}"
        )

# 文本分块任务端点
@app.post("/api/tasks/chunk", response_model=TaskResponse)
async def create_chunk_task(request: TextChunkRequest):
    """创建文本分块任务"""
    try:
        # 创建任务载荷
        payload = TextChunkPayload(
            document_id=request.document_id,
            content=request.content,
            chunk_size=request.chunk_size,
            overlap=request.overlap,
            split_type=request.split_type
        )

        # 创建任务记录
        task_id = f"chunk_{int(time.time())}_{request.document_id}"

        # 创建任务对象
        task = Task(
            id=task_id,
            type=TaskType.TEXT_CHUNK,
            document_id=request.document_id,
            status=TaskStatus.PENDING,
            payload=payload.__dict__
        )

        # 保存任务到Redis
        client = get_redis_client()
        key = get_task_key(task_id)
        client.set(key, task.to_json())

        # 添加到文档任务集合
        doc_key = f"document_tasks:{request.document_id}"
        client.sadd(doc_key, task_id)

        # 发送到Celery队列
        celery_task = chunk_text.delay(task_id)

        logger.info(f"Created text chunking task: {task_id}")
        return {
            "task_id": task_id,
            "status": "pending",
            "task_type": "text_chunk"
        }

    except Exception as e:
        logger.error(f"Failed to create chunking task: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to create text chunking task: {str(e)}"
        )

# 向量化任务端点
@app.post("/api/tasks/vectorize", response_model=TaskResponse)
async def create_vectorize_task(request: VectorizeRequest):
    """创建向量化任务"""
    try:
        # 转换chunks为ChunkInfo对象
        chunks = [
            {"text": chunk["text"], "index": chunk["index"]}
            for chunk in request.chunks
        ]

        # 创建任务载荷
        payload = VectorizePayload(
            document_id=request.document_id,
            chunks=chunks,
            model=request.model
        )

        # 创建任务记录
        task_id = f"vectorize_{int(time.time())}_{request.document_id}"

        # 创建任务对象
        task = Task(
            id=task_id,
            type=TaskType.VECTORIZE,
            document_id=request.document_id,
            status=TaskStatus.PENDING,
            payload=payload.__dict__
        )

        # 保存任务到Redis
        client = get_redis_client()
        key = get_task_key(task_id)
        client.set(key, task.to_json())

        # 添加到文档任务集合
        doc_key = f"document_tasks:{request.document_id}"
        client.sadd(doc_key, task_id)

        # 发送到Celery队列
        celery_task = vectorize_text.delay(task_id)

        logger.info(f"Created vectorization task: {task_id}")
        return {
            "task_id": task_id,
            "status": "pending",
            "task_type": "vectorize"
        }

    except Exception as e:
        logger.error(f"Failed to create vectorization task: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to create vectorization task: {str(e)}"
        )

# 完整处理任务端点
@app.post("/api/tasks/process", response_model=TaskResponse)
async def create_process_task(request: ProcessCompleteRequest):
    """创建完整处理流程任务"""
    try:
        # 创建任务载荷
        payload = ProcessCompletePayload(
            document_id=request.document_id,
            file_path=request.file_path,
            file_name=request.file_name,
            file_type=request.file_type,
            chunk_size=request.chunk_size,
            overlap=request.overlap,
            split_type=request.split_type,
            model=request.model,
            metadata=request.metadata
        )

        # 创建任务记录
        task_id = f"process_{int(time.time())}_{request.document_id}"

        # 创建任务对象
        task = Task(
            id=task_id,
            type=TaskType.PROCESS_COMPLETE,
            document_id=request.document_id,
            status=TaskStatus.PENDING,
            payload=payload.__dict__
        )

        # 保存任务到Redis
        client = get_redis_client()
        key = get_task_key(task_id)
        client.set(key, task.to_json())

        # 添加到文档任务集合
        doc_key = f"document_tasks:{request.document_id}"
        client.sadd(doc_key, task_id)

        # 发送到Celery队列的critical队列
        celery_task = process_document.apply_async(
            args=[task_id],
            queue='critical'
        )

        logger.info(f"Created complete process task: {task_id}")
        return {
            "task_id": task_id,
            "status": "pending",
            "task_type": "process_complete"
        }

    except Exception as e:
        logger.error(f"Failed to create process task: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to create complete process task: {str(e)}"
        )

# 获取任务状态端点
@app.get("/api/tasks/{task_id}")
async def get_task_status(task_id: str):
    """获取任务状态"""
    try:
        task = get_task_from_redis(task_id)
        if not task:
            raise HTTPException(
                status_code=404,
                detail=f"Task {task_id} not found"
            )

        response = {
            "task_id": task.id,
            "status": task.status,
            "type": task.type,
            "document_id": task.document_id,
            "created_at": task.created_at.isoformat(),
            "updated_at": task.updated_at.isoformat(),
        }

        if task.started_at:
            response["started_at"] = task.started_at.isoformat()

        if task.completed_at:
            response["completed_at"] = task.completed_at.isoformat()

        if task.error:
            response["error"] = task.error

        if task.result:
            response["result"] = task.result

        return response

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting task status: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to get task status: {str(e)}"
        )

# 获取文档任务列表端点
@app.get("/api/documents/{document_id}/tasks")
async def get_document_tasks(document_id: str):
    """获取文档的所有任务"""
    try:
        client = get_redis_client()
        doc_key = f"document_tasks:{document_id}"
        task_ids = client.smembers(doc_key)

        if not task_ids:
            return {"tasks": []}

        tasks = []
        for task_id in task_ids:
            task_id = task_id.decode("utf-8") if isinstance(task_id, bytes) else task_id
            task = get_task_from_redis(task_id)
            if task:
                tasks.append({
                    "task_id": task.id,
                    "status": task.status,
                    "type": task.type,
                    "created_at": task.created_at.isoformat(),
                    "updated_at": task.updated_at.isoformat(),
                })

        return {"tasks": tasks}

    except Exception as e:
        logger.error(f"Error getting document tasks: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to get document tasks: {str(e)}"
        )

# 任务回调端点（用于Go服务接收任务状态更新）
@app.post("/api/callback")
async def task_callback(request: Request):
    """接收任务回调通知"""
    try:
        payload = await request.json()
        logger.info(f"Received callback for task {payload.get('task_id')}")
        # 这只是一个模拟的回调处理，实际会由Go服务调用
        return {"status": "accepted"}
    except Exception as e:
        logger.error(f"Error processing callback: {str(e)}")
        raise HTTPException(
            status_code=400,
            detail="Invalid callback payload"
        )

# 请求处理时间中间件
@app.middleware("http")
async def add_process_time_header(request: Request, call_next):
    """记录请求处理时间的中间件"""
    start_time = time.time()
    response = await call_next(request)
    process_time = time.time() - start_time
    response.headers["X-Process-Time"] = str(process_time)
    
    # 记录请求处理情况
    status_code = response.status_code
    path = request.url.path
    logger.info(f"Request {request.method} {path} completed with status {status_code} in {process_time:.4f}s")
    
    return response

# 错误处理
@app.exception_handler(HTTPException)
async def http_exception_handler(request: Request, exc: HTTPException):
    """HTTP错误处理器"""
    return JSONResponse(
        status_code=exc.status_code,
        content={"detail": exc.detail}
    )

@app.exception_handler(Exception)
async def generic_exception_handler(request: Request, exc: Exception):
    """通用错误处理器"""
    logger.error(f"Unhandled exception: {str(exc)}")
    return JSONResponse(
        status_code=500,
        content={"detail": "An unexpected error occurred"}
    )

# 根路由 - 添加版本和API列表信息
@app.get("/")
async def root():
    """根路径，返回服务基本信息"""
    return {
        "service": "Document QA Python Service",
        "version": app.version,
        "status": "active",
        "docs_url": "/docs",
        "redoc_url": "/redoc",
        "health_check": "/api/health",
        "new_endpoints": {
            "document_api": "/api/python/documents",
            "chunking_api": "/api/python/documents/chunk",
            "embedding_api": "/api/python/embeddings"
        }
    }

# 主入口
if __name__ == "__main__":
    # 启动Uvicorn服务器
    port = int(os.getenv("PORT", "8000"))
    host = os.getenv("HOST", "0.0.0.0")
    uvicorn.run(
        "app.main:app",
        host=host,
        port=port,
        reload=os.getenv("ENVIRONMENT", "development").lower() == "development",
        log_level=os.getenv("LOG_LEVEL", "info").lower()
    )