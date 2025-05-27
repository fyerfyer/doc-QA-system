from fastapi import APIRouter, HTTPException, UploadFile, File, Form, Query, Path
import os
import json
import uuid
import time
import tempfile
from typing import Optional

# 导入新的文档处理模块
from app.document_processing.factory import create_parser, detect_content_type, get_file_from_minio
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
    store_result: bool = Form(True),
    original_filename: Optional[str] = Form(None)
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
        logger.info(f"Parse request received - document_id: {document_id}, file present: {file is not None}, file_path: {file_path}")
        
        # 验证参数
        if not file and not file_path:
            raise HTTPException(status_code=400, detail="Either file or file_path must be provided")
        
        start_time = time.time()
        task_id = None
        local_temp_path = None
        is_temp = False
        
        # 处理上传文件
        if file:
            # 保存上传的文件到临时目录，然后解析
            # 确保保留原始文件扩展名
            orig_filename = file.filename
            logger.info(f"Processing uploaded file: {orig_filename}, content_type: {file.content_type}")
            file_ext = os.path.splitext(orig_filename)[1] if orig_filename else ".txt"
            
            logger.info(f"Using file extension: {file_ext}")
            
            with tempfile.NamedTemporaryFile(delete=False, suffix=file_ext) as temp_file:
                local_temp_path = temp_file.name
                logger.info(f"Created temporary file: {local_temp_path}")
                content = await file.read()
                logger.info(f"Read {len(content)} bytes from uploaded file")
                temp_file.write(content)
                temp_file.flush()
                os.fsync(temp_file.fileno())
                logger.info(f"Wrote content to temporary file, size: {os.path.getsize(local_temp_path)}")
            
            file_path = local_temp_path
            is_temp = True
            filename = orig_filename
        else:
            # 使用提供的文件路径
            # 检查文件是否存在于MinIO或本地
            if not os.path.exists(file_path):
                # 尝试从MinIO下载
                local_temp_path, is_temp = get_file_from_minio(file_path)
                if is_temp:
                    file_path = local_temp_path
                    logger.info(f"Downloaded file from MinIO to {local_temp_path}")
                elif not os.path.exists(file_path):
                    raise HTTPException(status_code=404, detail=f"File not found: {file_path}")
            
            # 获取用于解析器检测的文件名
            filename = original_filename or os.path.basename(file_path)
        
        try:
            # 检测文件类型 
            mime_type = detect_content_type(file_path)
            logger.info(f"Detected MIME type: {mime_type}")
            
            # 获取文件扩展名
            ext = os.path.splitext(filename)[1][1:] if filename and '.' in filename else ""
            
            # 创建并使用LlamaIndex-based解析器
            parser = create_parser(file_path, mime_type, ext)
            logger.info(f"Created parser: {type(parser).__name__}")
            
            # 解析文档
            content = parser.parse()
            
            # 提取标题和元数据
            title = parser.extract_title(content, filename)
            meta = parser.get_metadata()
            
            # 添加一些基本统计信息（如果元数据中不存在）
            if isinstance(meta, dict):
                if 'words' not in meta:
                    meta['words'] = count_words(content)
                if 'chars' not in meta:
                    meta['chars'] = count_chars(content)
                if 'filename' not in meta:
                    meta['filename'] = filename
            else:
                meta = {
                    'filename': filename,
                    'words': count_words(content),
                    'chars': count_chars(content)
                }
            
            # 创建解析结果
            result = DocumentParseResult(
                content=content,
                document_id=document_id,
                title=title,
                meta=meta,
                pages=meta.get('page_count', 1) if isinstance(meta, dict) else 1,
                words=meta.get('words', count_words(content)),
                chars=meta.get('chars', count_chars(content))
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
        
        finally:
            # 清理临时文件
            if is_temp and local_temp_path and os.path.exists(local_temp_path):
                try:
                    os.remove(local_temp_path)
                    logger.info(f"Removed temporary file: {local_temp_path}")
                except Exception as e:
                    logger.warning(f"Failed to remove temporary file {local_temp_path}: {str(e)}")
            
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
    # 这个函数主要是从Redis获取数据，不需要太多修改
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