import os
import time
import traceback
import uvicorn
from contextlib import asynccontextmanager
from fastapi import FastAPI, HTTPException, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

# 导入环境变量
from dotenv import load_dotenv
load_dotenv()

from app.utils.utils import setup_logger, logger, get_task_key
from app.worker.tasks import get_redis_client

# 导入现有API路由器
from app.api.health import router as health_router
from app.api.callback import router as callback_router

# 导入新的API路由器
from app.api.document_api import router as document_router
from app.api.chunking_api import router as chunking_router
from app.api.embedding_api import router as embedding_api_router
from app.api.llm_api import router as llm_api_router  # 新增：导入LLM API路由器

from app.utils.route_display import print_routes

# 初始化日志
setup_logger(os.getenv("LOG_LEVEL", "INFO"))

# 定义生命周期管理器函数
@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    # 启动时执行的操作
    logger.info("Document Processing API started")

    # 检查环境变量
    check_environment_variables()

    # 检查Redis连接
    try:
        client = get_redis_client()
        if client.ping():
            logger.info("Redis connection successful")
        else:
            logger.error("Redis ping failed")
    except Exception as e:
        logger.error(f"Error connecting to Redis: {str(e)}")

    # 获取并打印所有API路由
    print_routes(app, logger)

    yield

    # 关闭时执行的操作
    logger.info("Document Processing API shutting down")

# 检查环境变量
def check_environment_variables():
    """检查并记录关键环境变量"""
    # LLM相关环境变量
    dashscope_api_key = os.getenv("DASHSCOPE_API_KEY")
    if not dashscope_api_key:
        logger.warning("DASHSCOPE_API_KEY not set, LLM functionality may be limited")
    
    # 设置默认变量
    os.environ.setdefault("CHUNK_SIZE", "1000")
    os.environ.setdefault("CHUNK_OVERLAP", "200")
    os.environ.setdefault("EMBEDDING_MODEL", "text-embedding-v3")
    
    # 记录已设置的变量（不记录敏感信息）
    logger.info(f"CHUNK_SIZE: {os.getenv('CHUNK_SIZE')}")
    logger.info(f"CHUNK_OVERLAP: {os.getenv('CHUNK_OVERLAP')}")
    logger.info(f"EMBEDDING_MODEL: {os.getenv('EMBEDDING_MODEL')}")
    logger.info(f"DASHSCOPE_API_KEY: {'[SET]' if dashscope_api_key else '[NOT SET]'}")

# 创建FastAPI应用
app = FastAPI(
    title="Document Processing API",
    description="API for document parsing, chunking, embedding, and LLM generation",  # 更新描述
    version="1.0.0",
    lifespan=lifespan,
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

# 添加断开连接检测中间件
@app.middleware("http")
async def add_disconnect_handler(request: Request, call_next):
    """
    中间件：添加断开连接检测功能，用于流式响应的取消
    """
    request.state.is_disconnected = False
    
    # 添加断开连接检测方法
    async def is_disconnected():
        return request.state.is_disconnected
    
    request.is_disconnected = is_disconnected
    
    try:
        return await call_next(request)
    except Exception:
        request.state.is_disconnected = True
        raise
    finally:
        request.state.is_disconnected = True

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

# 注册现有的API路由器
app.include_router(health_router)
app.include_router(callback_router)

# 注册新的API路由器
app.include_router(document_router)
app.include_router(chunking_router)
app.include_router(embedding_api_router)
app.include_router(llm_api_router)  # 新增：注册LLM API路由器

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
    logger.error(f"Unhandled exception: {str(exc)}\n{traceback.format_exc()}")
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
        "api_endpoints": {
            "documents": "/api/python/documents",
            "chunking": "/api/python/documents/chunk",
            "embedding": "/api/python/embeddings",
            "llm": "/api/python/llm",
            "rag": "/api/python/llm/rag"  
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