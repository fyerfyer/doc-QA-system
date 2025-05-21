from fastapi import APIRouter, HTTPException, Body, Query, Path
from typing import Optional, Dict, Any
import json
import uuid
import time

# 导入应用内部的组件
from app.chunkers.splitter import split_text
from app.utils.utils import logger
from app.models.model import Task, TaskType, TaskStatus, TextChunkResult, ChunkInfo
from app.worker.tasks import get_redis_client, get_task_from_redis

# 创建路由器
router = APIRouter(prefix="/api/python/documents", tags=["documents"])

@router.post("/chunk")
async def chunk_text(
    text: str = Body(..., description="Text content to chunk"),
    document_id: str = Body(..., description="Document ID"),
    chunk_size: int = Body(1000, description="Chunk size in characters or tokens"),
    chunk_overlap: int = Body(200, description="Overlap size between chunks"),
    split_type: str = Body("paragraph", description="Splitting strategy (paragraph, sentence, length, semantic)"),
    store_result: bool = Body(True, description="Whether to store the chunking result"),
    metadata: Optional[Dict[str, Any]] = Body(None, description="Additional metadata")
):
    """
    分割文本内容为多个块
    
    参数:
    - text: 要分割的文本内容
    - document_id: 文档ID
    - chunk_size: 块大小（字符数或标记数）
    - chunk_overlap: 块之间的重叠大小
    - split_type: 分割策略，如paragraph、sentence、length
    - store_result: 是否存储分块结果
    - metadata: 附加元数据
    """
    try:
        start_time = time.time()
        task_id = None
        
        # 验证参数
        if not text:
            raise HTTPException(status_code=400, detail="Text content cannot be empty")
            
        if chunk_size <= 0:
            raise HTTPException(status_code=400, detail="Chunk size must be positive")
            
        if chunk_overlap < 0:
            raise HTTPException(status_code=400, detail="Chunk overlap cannot be negative")
            
        logger.info(f"Chunking text for document {document_id}: {len(text)} chars, type: {split_type}")
        
        # 准备元数据
        if metadata is None:
            metadata = {}
        
        metadata["document_id"] = document_id
        
        # 执行文本分块
        chunks_data = split_text(
            text=text,
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            split_type=split_type,
            metadata=metadata
        )
        
        # 转换为ChunkInfo对象
        chunks = [ChunkInfo(text=chunk['text'], index=chunk['index']) for chunk in chunks_data]
        
        # 存储结果（如果需要）
        if store_result:
            # 创建任务对象
            task_id = f"chunk_{document_id}_{uuid.uuid4().hex[:8]}"
            result = TextChunkResult(
                document_id=document_id,
                chunks=chunks,
                chunk_count=len(chunks)
            )
            
            task = Task(
                id=task_id,
                type=TaskType.TEXT_CHUNK,
                document_id=document_id,
                status=TaskStatus.COMPLETED,
                result=result.__dict__
            )
            
            # 保存到Redis
            redis_client = get_redis_client()
            redis_client.set(f"task:{task_id}", task.to_json())
            
            # 将chunks序列化为JSON并存储
            chunks_json = json.dumps([{
                "text": chunk.text, 
                "index": chunk.index
            } for chunk in chunks])
            redis_client.set(f"chunks:{document_id}", chunks_json)
            
            # 添加到文档任务集合
            redis_client.sadd(f"document_tasks:{document_id}", task_id)
            logger.info(f"Stored chunking result for document {document_id} with task {task_id}")
        
        # 计算处理时间
        process_time = time.time() - start_time
        
        # 返回结果
        return {
            "success": True,
            "document_id": document_id,
            "task_id": task_id,
            "chunks": [{"text": chunk.text, "index": chunk.index} for chunk in chunks],
            "chunk_count": len(chunks),
            "process_time_ms": int(process_time * 1000)
        }
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error chunking text: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Text chunking failed: {str(e)}")

@router.get("/{document_id}/chunks")
async def get_document_chunks(
    document_id: str = Path(..., description="The ID of the document"),
    task_id: Optional[str] = Query(None, description="Optional task ID")
):
    """
    获取文档的分块结果
    
    参数:
    - document_id: 文档ID
    - task_id: 可选的任务ID，如果提供则从该任务中获取结果
    """
    try:
        redis_client = get_redis_client()
        
        # 如果提供了任务ID，从任务中获取结果
        if task_id:
            task = get_task_from_redis(task_id)
            if not task:
                raise HTTPException(status_code=404, detail=f"Task {task_id} not found")
                
            if task.document_id != document_id:
                raise HTTPException(status_code=400, detail=f"Task {task_id} does not belong to document {document_id}")
                
            if task.status == TaskStatus.FAILED:
                return {
                    "success": False,
                    "document_id": document_id,
                    "task_id": task_id,
                    "error": task.error
                }
                
            if task.status != TaskStatus.COMPLETED:
                return {
                    "success": False,
                    "document_id": document_id,
                    "task_id": task_id,
                    "status": task.status
                }
            
            # 从任务结果中获取块
            if isinstance(task.result, dict) and "chunks" in task.result:
                chunks_data = task.result["chunks"]
                return {
                    "success": True,
                    "document_id": document_id,
                    "task_id": task_id,
                    "chunks": chunks_data,
                    "chunk_count": len(chunks_data)
                }
            else:
                return {
                    "success": False,
                    "document_id": document_id,
                    "task_id": task_id,
                    "error": "Invalid chunk format in task result"
                }
        
        # 从最新存储的结果中获取块
        chunks_json = redis_client.get(f"chunks:{document_id}")
        if not chunks_json:
            # 查找与文档相关的分块任务
            task_ids = redis_client.smembers(f"document_tasks:{document_id}")
            chunk_tasks = []
            
            for tid in task_ids:
                tid = tid.decode('utf-8') if isinstance(tid, bytes) else tid
                task = get_task_from_redis(tid)
                if task and task.type == TaskType.TEXT_CHUNK and task.status == TaskStatus.COMPLETED:
                    chunk_tasks.append(task)
            
            # 如果没有找到分块任务
            if not chunk_tasks:
                raise HTTPException(status_code=404, detail=f"No chunks found for document {document_id}")
                
            # 找到最新的任务
            latest_task = max(chunk_tasks, key=lambda t: t.updated_at)
            
            # 从任务结果中获取块
            if isinstance(latest_task.result, dict) and "chunks" in latest_task.result:
                chunks_data = latest_task.result["chunks"]
                return {
                    "success": True,
                    "document_id": document_id,
                    "task_id": latest_task.id,
                    "chunks": chunks_data,
                    "chunk_count": len(chunks_data)
                }
            else:
                raise HTTPException(status_code=500, detail="Invalid chunk format in latest task result")
        
        # 解析存储的块
        chunks_data = json.loads(chunks_json)
        return {
            "success": True,
            "document_id": document_id,
            "chunks": chunks_data,
            "chunk_count": len(chunks_data)
        }
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting document chunks: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Failed to get chunks: {str(e)}")