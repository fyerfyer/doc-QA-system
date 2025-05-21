from fastapi import APIRouter, HTTPException, UploadFile, File, Form, Query, Path
import os
import json
import uuid
import time
import tempfile
from typing import Optional

from app.parsers.factory import create_parser, detect_content_type
from app.utils.minio_client import get_minio_client
from app.utils.utils import logger, count_words, count_chars
from app.models.model import Task, TaskType, TaskStatus, DocumentParseResult
from app.worker.tasks import get_redis_client, get_task_from_redis

# 创建路由器
router = APIRouter(prefix="/api/python/documents", tags=["documents"])

# 获取MinIO客户端
minio_client = get_minio_client()

@router.post("/parse")
async def parse_document(
    file: Optional[UploadFile] = File(None),
    file_path: Optional[str] = Form(None),
    document_id: str = Form(...),
    store_result: bool = Form(True)
):
    """
    解析文档内容
    
    可以通过上传文件或指定文件路径来解析文档
    文件路径可以是MinIO中的路径或服务器本地路径
    
    参数:
    - file: 上传的文件
    - file_path: 文件路径
    - document_id: 文档ID
    - store_result: 是否存储解析结果
    """
    try:
        # 验证参数
        if not file and not file_path:
            raise HTTPException(status_code=400, detail="Either file or file_path must be provided")
        
        start_time = time.time()
        task_id = None
        
        # 处理上传文件
        if file:
            # 保存上传的文件到临时目录，然后解析
            # 确保保留原始文件扩展名
            orig_filename = file.filename
            file_ext = os.path.splitext(orig_filename)[1] if orig_filename else ".txt"
            
            with tempfile.NamedTemporaryFile(delete=False, suffix=file_ext) as temp_file:
                temp_file_path = temp_file.name
                content = await file.read()
                temp_file.write(content)
            
            try:
                # 检测文件类型
                mime_type = detect_content_type(temp_file_path)
                
                # 创建并使用解析器
                parser = create_parser(temp_file_path, mime_type)
                content = parser.parse(temp_file_path)
                
                # 提取标题和元数据
                title = parser.extract_title(content, file.filename)
                meta = parser.get_metadata(temp_file_path)
                
            finally:
                # 删除临时文件
                if os.path.exists(temp_file_path):
                    os.remove(temp_file_path)
        else:
            # 使用提供的文件路径解析
            try:
                # 检查文件是否存在
                file_exists = False
                
                # 如果是Windows风格的绝对路径，只检查本地文件系统
                is_windows_path = file_path and (file_path.startswith('C:') or file_path.startswith('D:') or ':\\' in file_path)
                
                if not is_windows_path:
                    # 尝试在MinIO中查找
                    try:
                        if minio_client.file_exists(file_path):
                            file_exists = True
                    except Exception as e:
                        logger.warning(f"Failed to check file in MinIO: {str(e)}")
                
                # 尝试在本地查找
                if not file_exists and os.path.exists(file_path):
                    file_exists = True
                
                if not file_exists:
                    raise HTTPException(status_code=404, detail=f"File not found: {file_path}")
                
                # 检测文件类型
                mime_type = detect_content_type(file_path)
                
                # 创建并使用解析器
                parser = create_parser(file_path, mime_type)
                content = parser.parse(file_path)
                
                # 提取标题和元数据
                filename = os.path.basename(file_path)
                title = parser.extract_title(content, filename)
                meta = parser.get_metadata(file_path)
                
            except FileNotFoundError:
                raise HTTPException(status_code=404, detail=f"File not found: {file_path}")
        
        # 创建解析结果
        result = DocumentParseResult(
            content=content,
            title=title,
            meta=meta,
            pages=meta.get('page_count', 1) if isinstance(meta, dict) else 1,
            words=count_words(content),
            chars=count_chars(content)
        )
        
        # 存储解析结果（如果需要）
        if store_result:
            # 创建一个任务对象
            task_id = f"parse_{document_id}_{uuid.uuid4().hex[:8]}"
            task = Task(
                id=task_id,
                type=TaskType.DOCUMENT_PARSE,
                document_id=document_id,
                status=TaskStatus.COMPLETED,
                result=result.__dict__
            )
            
            # 保存到Redis
            redis_client = get_redis_client()
            redis_client.set(f"task:{task_id}", task.to_json())
            redis_client.set(f"parse_result:{document_id}", json.dumps(result.__dict__))
            
            # 添加到文档任务集合
            redis_client.sadd(f"document_tasks:{document_id}", task_id)
            logger.info(f"Stored parsing result for document {document_id} with task {task_id}")
        
        # 计算处理时间
        process_time = time.time() - start_time
        
        # 返回结果
        return {
            "success": True,
            "document_id": document_id,
            "task_id": task_id,
            "result": result.__dict__,
            "process_time_ms": int(process_time * 1000)
        }
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error parsing document: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Document parsing failed: {str(e)}")

@router.get("/{document_id}")
async def get_document_parse_result(
    document_id: str = Path(..., description="The ID of the document"),
    task_id: Optional[str] = Query(None, description="Optional task ID")
):
    """
    获取文档解析结果
    
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
                
            return {
                "success": True,
                "document_id": document_id,
                "task_id": task_id,
                "result": task.result
            }
        
        # 否则，从存储的解析结果中获取
        result_json = redis_client.get(f"parse_result:{document_id}")
        if not result_json:
            # 查找与文档相关的任务
            task_ids = redis_client.smembers(f"document_tasks:{document_id}")
            for tid in task_ids:
                tid = tid.decode('utf-8') if isinstance(tid, bytes) else tid
                task = get_task_from_redis(tid)
                if task and task.type == TaskType.DOCUMENT_PARSE and task.status == TaskStatus.COMPLETED:
                    return {
                        "success": True,
                        "document_id": document_id,
                        "task_id": tid,
                        "result": task.result
                    }
            
            # 如果没有找到结果
            raise HTTPException(status_code=404, detail=f"No parse result found for document {document_id}")
        
        result = json.loads(result_json)
        return {
            "success": True,
            "document_id": document_id,
            "result": result
        }
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting document parse result: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Failed to get parse result: {str(e)}")