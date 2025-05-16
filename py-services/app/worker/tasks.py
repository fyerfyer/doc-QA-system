import os
import time
from typing import Any, Optional
from datetime import datetime
import traceback

import redis
from celery import shared_task
from redis.exceptions import RedisError

from app.models.model import (
    Task, TaskStatus,
    DocumentParsePayload, DocumentParseResult,
    TextChunkPayload, TextChunkResult, ChunkInfo,
    VectorizePayload, VectorizeResult, VectorInfo,
    ProcessCompletePayload, ProcessCompleteResult
)
from app.utils.utils import (
    logger, parse_redis_url, get_task_key,
    count_words, count_chars, send_callback
)
from app.worker.celery_app import app

# 配置Redis连接参数
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")
REDIS_PARAMS = parse_redis_url(REDIS_URL)
CALLBACK_URL = os.getenv("CALLBACK_URL", "http://localhost:8080/api/tasks/callback")

# 获取Redis连接
def get_redis_client():
    """获取Redis客户端连接"""
    return redis.Redis(**REDIS_PARAMS)

# 任务辅助函数
def get_task_from_redis(task_id: str) -> Optional[Task]:
    """
    从Redis获取任务信息

    参数:
        task_id: 任务ID

    返回:
        Task: 任务对象，如果不存在则返回None
    """
    try:
        client = get_redis_client()
        key = get_task_key(task_id)
        data = client.get(key)
        if not data:
            logger.error(f"Task {task_id} not found in Redis")
            return None

        return Task.from_json(data)
    except RedisError as e:
        logger.error(f"Redis error when getting task {task_id}: {str(e)}")
        return None
    except Exception as e:
        logger.error(f"Failed to get task {task_id}: {str(e)}")
        return None

def update_task_status(task: Task, status: TaskStatus, result: Any = None, error: str = "") -> bool:
    """
    更新任务状态

    参数:
        task: 任务对象
        status: 新状态
        result: 处理结果
        error: 错误信息

    返回:
        bool: 成功返回True，失败返回False
    """
    try:
        client = get_redis_client()
        task.status = status
        task.updated_at = datetime.now()

        if status == TaskStatus.PROCESSING and task.started_at is None:
            task.started_at = datetime.now()

        if status in [TaskStatus.COMPLETED, TaskStatus.FAILED]:
            task.completed_at = datetime.now()

        if result is not None:
            # 确保 VectorInfo 对象能被正确序列化
            if isinstance(result, dict) and "vectors" in result:
                vectors = result["vectors"]
                if vectors and hasattr(vectors[0], "__dict__"):
                    # Convert VectorInfo objects to dictionaries
                    serializable_vectors = []
                    for vector_info in vectors:
                        serializable_vectors.append({
                            "chunk_index": vector_info.chunk_index,
                            "vector": vector_info.vector
                        })
                    result["vectors"] = serializable_vectors

            # 确保 ChunkInfo 对象能被正确序列化
            if isinstance(result, dict) and "chunks" in result:
                chunks = result["chunks"]
                if chunks and hasattr(chunks[0], "__dict__"):
                    # Convert ChunkInfo objects to dictionaries
                    serializable_chunks = []
                    for chunk_info in chunks:
                        serializable_chunks.append({
                            "text": chunk_info.text,
                            "index": chunk_info.index
                        })
                    result["chunks"] = serializable_chunks

            task.result = result

        if error:
            task.error = error

        # 保存到Redis
        key = get_task_key(task.id)
        client.set(key, task.to_json())

        # 发送回调通知
        try:
            callback_data = {
                "task_id": task.id,
                "document_id": task.document_id,
                "status": task.status,
                "type": task.type,
                "result": task.result,
                "error": task.error,
                "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
            }
            send_callback(CALLBACK_URL, callback_data)
        except Exception as e:
            logger.warning(f"Failed to send callback for task {task.id}: {str(e)}")

        return True
    except Exception as e:
        logger.error(f"Failed to update task {task.id} status: {str(e)}")
        return False

# 文档解析任务
@shared_task(name="app.worker.tasks.parse_document")
def parse_document(task_id: str) -> bool:
    """
    解析文档任务

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting document parse task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        if not isinstance(task.payload, dict):
            raise ValueError("Task payload is not a dictionary")
        else:
            payload = DocumentParsePayload(**task.payload)

        logger.info(f"Parsing document: {payload.file_path}")

        # 实现文档解析逻辑
        from app.parsers.factory import create_parser, detect_content_type

        # 检测内容类型
        mime_type = detect_content_type(payload.file_path)

        # 创建对应的解析器
        parser = create_parser(payload.file_path, mime_type)

        # 执行解析
        start_time = time.time()
        content = parser.parse(payload.file_path)

        # 提取标题和元数据
        title = parser.extract_title(content, payload.file_name)
        meta = parser.get_metadata(payload.file_path)

        # 创建结果
        result = DocumentParseResult(
            content=content,
            title=title,
            meta=meta,
            pages=meta.get('page_count', 1) if isinstance(meta, dict) else 1,
            words=count_words(content),
            chars=count_chars(content)
        )

        elapsed = time.time() - start_time
        logger.info(f"Document parse task {task_id} completed in {elapsed:.2f}s")

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)
        return True

    except Exception as e:
        error_msg = f"Document parse failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False

# 文本分块任务
@shared_task(name="app.worker.tasks.chunk_text")
def chunk_text(task_id: str) -> bool:
    """
    文本分块任务

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting text chunking task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        if not isinstance(task.payload, dict):
            raise ValueError("Task payload is not a dictionary")
        else:
            payload = TextChunkPayload(**task.payload)

        logger.info(f"Chunking text for document: {payload.document_id}")

        # 实现文本分块逻辑
        from app.chunkers.splitter import split_text

        # 执行分块
        start_time = time.time()
        chunks_data = split_text(
            text=payload.content,
            chunk_size=payload.chunk_size or 1000,
            chunk_overlap=payload.overlap or 200,
            split_type=payload.split_type or "paragraph",
            metadata={"document_id": payload.document_id}
        )

        # 转换为ChunkInfo列表
        chunks = [ChunkInfo(text=chunk['text'], index=chunk['index']) for chunk in chunks_data]

        # 创建结果
        result = TextChunkResult(
            document_id=payload.document_id,
            chunks=chunks,
            chunk_count=len(chunks)
        )

        elapsed = time.time() - start_time
        logger.info(f"Text chunking task {task_id} completed in {elapsed:.2f}s: {len(chunks)} chunks created")

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)
        return True

    except Exception as e:
        error_msg = f"Text chunking failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False

# 向量化任务
@shared_task(name="app.worker.tasks.vectorize_text")
def vectorize_text(task_id: str) -> bool:
    """
    向量化文本任务

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting text vectorization task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        if not isinstance(task.payload, dict):
            raise ValueError("Task payload is not a dictionary")
        else:
            payload = VectorizePayload(**task.payload)

        logger.info(f"Vectorizing text for document: {payload.document_id}, chunks: {len(payload.chunks)}")

        # 实现向量化逻辑
        from app.embedders.factory import create_embedder

        # 创建嵌入模型
        embedder = create_embedder(payload.model)

        # 提取文本 - 修复：处理字典和对象两种情况
        texts = []
        for chunk in payload.chunks:
            if isinstance(chunk, dict):
                # 如果是字典，直接获取text字段
                if 'text' not in chunk:
                    raise ValueError(f"Chunk missing 'text' field: {chunk}")
                texts.append(chunk['text'])
            elif hasattr(chunk, 'text'):
                # 如果是对象，使用text属性
                texts.append(chunk.text)
            else:
                raise ValueError(f"Invalid chunk format: {chunk}")

        # 执行向量化
        start_time = time.time()
        vectors_data = embedder.embed_batch(texts)

        # 创建向量信息
        dimension = len(vectors_data[0]) if vectors_data else 0
        vectors = []
        for i, vec in enumerate(vectors_data):
            chunk_index = payload.chunks[i].get('index', i) if isinstance(payload.chunks[i], dict) else payload.chunks[i].index
            vectors.append(VectorInfo(chunk_index=chunk_index, vector=vec))

        # 创建结果
        result = VectorizeResult(
            document_id=payload.document_id,
            vectors=vectors,
            vector_count=len(vectors),
            model=payload.model or embedder.get_model_name(),
            dimension=dimension
        )

        elapsed = time.time() - start_time
        logger.info(f"Text vectorization task {task_id} completed in {elapsed:.2f}s: {len(vectors)} vectors created")

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)
        return True

    except Exception as e:
        error_msg = f"Text vectorization failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False

# 完整文档处理任务
@shared_task(name="app.worker.tasks.process_document")
def process_document(task_id: str) -> bool:
    """
    完整文档处理流程

    参数:
        task_id: 任务ID

    返回:
        bool: 成功返回True，失败返回False
    """
    logger.info(f"Starting complete document processing task: {task_id}")

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    # 更新任务状态为处理中
    update_task_status(task, TaskStatus.PROCESSING)

    try:
        # 解析任务载荷
        if not isinstance(task.payload, dict):
            raise ValueError("Task payload is not a dictionary")
        else:
            payload = ProcessCompletePayload(**task.payload)

        logger.info(f"Processing document: {payload.document_id}, file: {payload.file_path}")

        result = ProcessCompleteResult(
            document_id=payload.document_id,
            parse_status="pending",
            chunk_status="pending",
            vector_status="pending"
        )

        # 1. 解析文档
        start_time = time.time()
        try:
            from app.parsers.factory import create_parser, detect_content_type

            # 检测内容类型
            mime_type = detect_content_type(payload.file_path)

            # 创建解析器并解析
            parser = create_parser(payload.file_path, mime_type)
            content = parser.parse(payload.file_path)

            # 提取标题和元数据
            title = parser.extract_title(content, payload.file_name)
            meta = parser.get_metadata(payload.file_path)

            # 创建解析结果
            parse_result = DocumentParseResult(
                content=content,
                title=title,
                meta=meta,
                pages=meta.get('page_count', 1) if isinstance(meta, dict) else 1,
                words=count_words(content),
                chars=count_chars(content)
            )

            result.parse_status = "completed"
            logger.info(f"Document parse completed for {payload.document_id}")
        except Exception as e:
            result.parse_status = "failed"
            result.error = f"Parse failed: {str(e)}"
            logger.error(f"Document parse failed: {str(e)}")
            # 在解析失败时，提前完成任务
            update_task_status(task, TaskStatus.FAILED, result.__dict__, result.error)
            return False

        # 2. 文本分块
        try:
            from app.chunkers.splitter import split_text

            # 执行分块
            chunks_data = split_text(
                text=parse_result.content,
                chunk_size=payload.chunk_size,
                chunk_overlap=payload.overlap,
                split_type=payload.split_type,
                metadata={"document_id": payload.document_id}
            )

            # 转换为ChunkInfo
            chunks = [ChunkInfo(text=chunk['text'], index=chunk['index']) for chunk in chunks_data]

            result.chunk_status = "completed"
            result.chunk_count = len(chunks)
            logger.info(f"Text chunking completed for {payload.document_id}: {len(chunks)} chunks")
        except Exception as e:
            result.chunk_status = "failed"
            result.error = f"Chunking failed: {str(e)}"
            logger.error(f"Text chunking failed: {str(e)}")
            # 在分块失败时，提前完成任务
            update_task_status(task, TaskStatus.FAILED, result.__dict__, result.error)
            return False

        # 3. 向量化
        try:
            from app.embedders.factory import create_embedder

            # 创建嵌入模型
            try:
                embedder = create_embedder(payload.model)
                logger.info(f"Successfully created embedder of type '{payload.model}'")
            except ValueError as e:
                # 如果特定模型无法使用的话，改为使用默认模型
                logger.warning(f"Could not create embedder with model '{payload.model}': {str(e)}")
                logger.warning(f"Falling back to default embedder")
                from app.embedders.factory import get_default_embedder
                embedder = get_default_embedder()

            # 提取文本并向量化
            texts = [chunk.text for chunk in chunks]
            vector_data = embedder.embed_batch(texts)

            # 创建向量信息
            dimension = len(vector_data[0]) if vector_data else 0
            vectors = [
                VectorInfo(chunk_index=chunks[i].index, vector=vec)
                for i, vec in enumerate(vector_data)
            ]

            result.vector_status = "completed"
            result.vector_count = len(vectors)
            result.dimension = dimension
            result.vectors = vectors  # 将向量包含在结果中
            logger.info(f"Text vectorization completed for {payload.document_id}: {len(vectors)} vectors")
        except Exception as e:
            result.vector_status = "failed"
            result.error = f"Vectorization failed: {str(e)}"
            logger.error(f"Text vectorization failed: {str(e)}")
            # 不需要在此提前结束，即使向量化失败仍可返回分块结果

        # 更新任务状态为已完成
        update_task_status(task, TaskStatus.COMPLETED, result.__dict__)

        elapsed = time.time() - start_time
        logger.info(f"Complete document processing task {task_id} completed in {elapsed:.2f}s")
        return True

    except Exception as e:
        error_msg = f"Document processing failed: {str(e)}\n{traceback.format_exc()}"
        logger.error(error_msg)
        update_task_status(task, TaskStatus.FAILED, error=error_msg)
        return False