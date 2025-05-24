import time
from typing import Dict, Any
from fastapi import APIRouter, HTTPException, Body
from pydantic import BaseModel, Field

from app.models.model import (
    Task, TaskType, TaskStatus,
    DocumentParsePayload, TextChunkPayload,
    VectorizePayload, ProcessCompletePayload
)
from app.utils.utils import logger, get_task_key
from app.worker.tasks import (
    parse_document, chunk_text, vectorize_text, process_document,
    get_redis_client, get_task_from_redis
)

# 创建路由器
router = APIRouter(prefix="/api/tasks", tags=["tasks"])

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
    split_type: str = "paragraph"

class VectorizeRequest(BaseModel):
    document_id: str
    chunks: list
    model: str = "default"

class ProcessCompleteRequest(BaseModel):
    document_id: str
    file_path: str
    file_name: str
    file_type: str = ""
    chunk_size: int = 1000
    overlap: int = 200
    split_type: str = "paragraph"
    model: str = "default"
    metadata: Dict[str, Any] = Field(default_factory=dict)

class TaskResponse(BaseModel):
    task_id: str
    status: str = "pending"
    task_type: str

# 文档解析任务接口
@router.post("/parse", response_model=TaskResponse)
async def create_parse_task(request: DocumentParseRequest):
    """创建文档解析任务"""
    try:
        # 创建任务负载
        payload = DocumentParsePayload(
            file_path=request.file_path,
            file_name=request.file_name,
            file_type=request.file_type,
            metadata=request.metadata
        )

        # 创建任务ID
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
        parse_document.delay(task_id)

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

# 文本分块任务接口
@router.post("/chunk", response_model=TaskResponse)
async def create_chunk_task(request: TextChunkRequest):
    """创建文本分块任务"""
    try:
        # 创建任务负载
        payload = TextChunkPayload(
            document_id=request.document_id,
            content=request.content,
            chunk_size=request.chunk_size,
            overlap=request.overlap,
            split_type=request.split_type
        )

        # 创建任务ID
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
        chunk_text.delay(task_id)

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

# 向量化任务接口
@router.post("/vectorize", response_model=TaskResponse)
async def create_vectorize_task(request: VectorizeRequest):
    """创建向量化任务"""
    try:
        # 将chunks转换为ChunkInfo对象
        chunks = []
        for chunk in request.chunks:
            if isinstance(chunk, dict):
                chunks.append(chunk)
            else:
                chunks.append({"text": chunk.text, "index": chunk.index})

        # 创建任务负载
        payload = VectorizePayload(
            document_id=request.document_id,
            chunks=chunks,
            model=request.model
        )

        # 创建任务ID
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
        vectorize_text.delay(task_id)

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

# 完整流程处理任务接口
@router.post("/process", response_model=TaskResponse)
async def create_process_task(request: ProcessCompleteRequest = Body(...)):
    """创建完整处理流程任务"""
    try:
        # 创建任务负载
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

        # 创建任务ID
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

        # 以critical优先级发送到Celery队列
        process_document.apply_async(
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

# 获取任务状态接口
@router.get("/{task_id}")
async def get_task_status(task_id: str):
    """获取任务状态"""
    try:
        task = get_task_from_redis(task_id)
        if not task:
            raise HTTPException(status_code=404, detail=f"Task {task_id} not found")

        response = {
            "task_id": task.id,
            "status": task.status,
            "type": task.type,
            "document_id": task.document_id,
            "created_at": task.created_at.isoformat() if task.created_at else None,
            "updated_at": task.updated_at.isoformat() if task.updated_at else None,
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