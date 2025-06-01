import os
import time
from typing import Dict, Any, Optional, Tuple

from app.models.model import (
    Task, TaskType, DocumentParseResult, 
    TextChunkResult, VectorizeResult,
    ProcessCompletePayload, ProcessCompleteResult,VectorInfo
)

# 导入新的文档处理模块
from app.document_processing.factory import (
    create_parser, create_chunker, detect_content_type, 
    process_file, get_file_from_minio
)
from app.embedders.factory import create_embedder, get_default_embedder
from app.utils.utils import logger, count_words, count_chars, retry
from app.utils.minio_client import get_minio_client

minio_client = get_minio_client()

class DocumentProcessor:
    """文档处理器，处理文档的解析、分块和向量化等任务"""
    
    def __init__(self):
        """初始化处理器，加载嵌入模型"""
        self.embedder = None
        self._load_embedder()
        
    def _load_embedder(self):
        """加载默认的嵌入模型"""
        try:
            self.embedder = get_default_embedder()
            logger.info(f"Loaded default embedder: {self.embedder.get_model_name()}")
        except Exception as e:
            logger.error(f"Failed to load default embedder: {str(e)}")
    
    def get_embedder(self, model_name: Optional[str] = None):
        """
        获取嵌入器实例
        
        参数:
            model_name: 模型名称，如果为None则使用默认模型
            
        返回:
            BaseEmbedder: 嵌入器实例
        """
        if model_name and model_name != "default":
            try:
                logger.info(f"Creating embedder with model: {model_name}")
                return create_embedder(model_name)
            except Exception as e:
                logger.error(f"Failed to create embedder with model {model_name}: {str(e)}")
                logger.info("Falling back to default embedder")
                
        # 如果没有预加载默认嵌入器或加载失败，尝试重新加载
        if not self.embedder:
            self._load_embedder()
            
        return self.embedder
    
    @retry(max_retries=3, delay=2)
    def process_document(self, task: Task) -> Tuple[bool, Dict[str, Any]]:
        """
        处理完整的文档处理任务(解析+分块+向量化)
        
        参数:
            task: 任务对象
            
        返回:
            Tuple[bool, Dict[str, Any]]: (成功与否, 结果字典)
        """
        logger.info(f"Processing complete document task: {task.id} for document {task.document_id}")
        
        try:
            if task.type != TaskType.PROCESS_COMPLETE:
                raise ValueError(f"Invalid task type: {task.type}, expected: {TaskType.PROCESS_COMPLETE}")
                
            # 解析任务参数
            payload = ProcessCompletePayload(**task.payload)
            
            # 完整处理：解析 + 分块 + 向量化
            logger.info(f"Starting complete processing for document {task.document_id}, file: {payload.file_path}")
            
            # 使用新的process_file函数一次性处理文档
            content, chunks, doc_metadata = process_file(
                file_path=payload.file_path,
                chunk_size=payload.chunk_size,
                chunk_overlap=payload.overlap,
                split_type=payload.split_type,
                metadata=payload.metadata
            )
            
            # 检查处理结果
            if not content:
                raise ValueError(f"Failed to extract content from document: {payload.file_path}")
                
            if not chunks:
                raise ValueError(f"Failed to chunk content for document: {payload.file_path}")
            
            # 向量化所有块
            embedder = self.get_embedder(payload.model)
            logger.info(f"Vectorizing {len(chunks)} chunks using model: {embedder.get_model_name()}")
            
            # 准备要嵌入的文本列表
            texts = [chunk["text"] for chunk in chunks]
            
            # 批量嵌入
            start_time = time.time()
            embeddings = embedder.embed_batch(texts)
            embed_time = time.time() - start_time
            
            logger.info(f"Vectorization completed in {embed_time:.2f}s")
            
            # 创建向量结果
            vectors = []
            for i, embedding in enumerate(embeddings):
                vectors.append(VectorInfo(
                    chunk_index=chunks[i]["index"],
                    vector=embedding
                ))
            
            # 整合结果
            result = ProcessCompleteResult(
                document_id=task.document_id,
                chunk_count=len(chunks),
                vector_count=len(vectors),
                dimension=len(vectors[0].vector) if vectors else 0,
                parse_status="completed",
                chunk_status="completed",
                vector_status="completed",
                vectors=vectors
            )
            
            return True, result.__dict__
            
        except Exception as e:
            logger.error(f"Error processing complete document: {str(e)}")
            return False, {"error": str(e)}
    
    @retry(max_retries=3, delay=2)
    def parse_document(self, task: Task) -> Tuple[bool, Dict[str, Any]]:
        """
        解析文档内容
        
        参数:
            task: 任务对象
            
        返回:
            Tuple[bool, Dict[str, Any]]: (成功与否, 结果字典)
        """
        logger.info(f"Processing document parse task: {task.id} for document {task.document_id}")
        
        try:
            # 检查任务类型
            if task.type != TaskType.DOCUMENT_PARSE:
                raise ValueError(f"Invalid task type: {task.type}, expected: {TaskType.DOCUMENT_PARSE}")
            
            # 获取文件路径和文件类型
            file_path = task.payload.get("file_path")
            if not file_path:
                raise ValueError("Missing file_path in task payload")
                
            file_name = task.payload.get("file_name", os.path.basename(file_path))
            file_type = task.payload.get("file_type", "")
            
            # 处理文件
            local_temp_path = None
            is_temp = False
            
            # 检查文件是否存在
            if not os.path.exists(file_path):
                # 尝试从MinIO下载
                local_temp_path, is_temp = get_file_from_minio(file_path)
                if is_temp:
                    file_path = local_temp_path
                else:
                    raise ValueError(f"File not found: {file_path}")
            
            try:
                # 检测文件类型
                mime_type = detect_content_type(file_path)
                
                # 获取文件扩展名
                ext = os.path.splitext(file_name)[1][1:] if '.' in file_name else ""
                
                # 创建并使用解析器
                parser = create_parser(file_path, mime_type, ext)
                content = parser.parse()
                
                # 提取标题和元数据
                title = parser.extract_title(content, file_name)
                meta = parser.get_metadata()
                
                # 添加或更新元数据
                if isinstance(meta, dict):
                    # 添加用户提供的元数据
                    user_metadata = task.payload.get("metadata", {})
                    if user_metadata:
                        meta.update(user_metadata)
                        
                    # 确保包含基本统计信息
                    if 'words' not in meta:
                        meta['words'] = count_words(content)
                    if 'chars' not in meta:
                        meta['chars'] = count_chars(content)
                    if 'filename' not in meta:
                        meta['filename'] = file_name
                else:
                    meta = {
                        'filename': file_name,
                        'words': count_words(content),
                        'chars': count_chars(content),
                        **task.payload.get("metadata", {})
                    }
                
                # 创建解析结果
                result = DocumentParseResult(
                    content=content,
                    document_id=task.document_id,
                    title=title,
                    meta=meta,
                    pages=meta.get('page_count', 1) if isinstance(meta, dict) else 1,
                    words=meta.get('words', count_words(content)),
                    chars=meta.get('chars', count_chars(content))
                )
                
                return True, result.__dict__
            
            finally:
                # 清理临时文件
                if is_temp and local_temp_path and os.path.exists(local_temp_path):
                    try:
                        os.remove(local_temp_path)
                    except Exception as e:
                        logger.warning(f"Failed to remove temporary file {local_temp_path}: {str(e)}")
                
        except Exception as e:
            logger.error(f"Error parsing document: {str(e)}")
            return False, {"error": str(e)}
    
    @retry(max_retries=3, delay=2)
    def chunk_text(self, task: Task) -> Tuple[bool, Dict[str, Any]]:
        """
        将文本分块
        
        参数:
            task: 任务对象
            
        返回:
            Tuple[bool, Dict[str, Any]]: (成功与否, 结果字典)
        """
        logger.info(f"Processing text chunking task: {task.id} for document {task.document_id}")
        
        try:
            # 检查任务类型
            if task.type != TaskType.TEXT_CHUNK:
                raise ValueError(f"Invalid task type: {task.type}, expected: {TaskType.TEXT_CHUNK}")
            
            # 获取分块参数
            content = task.payload.get("content")
            if not content:
                raise ValueError("Missing content in task payload")
                
            document_id = task.payload.get("document_id")
            chunk_size = task.payload.get("chunk_size", 1000)
            chunk_overlap = task.payload.get("overlap", 200)
            split_type = task.payload.get("split_type", "paragraph")
            
            # 设置基本元数据
            metadata = {"document_id": document_id}
            
            # 创建分块器并执行分块
            chunker = create_chunker(chunk_size, chunk_overlap, split_type)
            chunks = chunker.chunk_text(content, metadata)
            
            # 转换为预期的返回格式
            chunk_objects = []
            for i, chunk in enumerate(chunks):
                chunk_objects.append({
                    "text": chunk["text"],
                    "index": chunk["index"] if "index" in chunk else i
                })
                
            # 创建分块结果
            result = TextChunkResult(
                document_id=task.document_id,
                chunks=chunk_objects,
                chunk_count=len(chunk_objects)
            )
            
            return True, result.__dict__
            
        except Exception as e:
            logger.error(f"Error chunking text: {str(e)}")
            return False, {"error": str(e)}
    
    @retry(max_retries=3, delay=2)
    def vectorize_text(self, task: Task) -> Tuple[bool, Dict[str, Any]]:
        """
        向量化文本块
        
        参数:
            task: 任务对象
            
        返回:
            Tuple[bool, Dict[str, Any]]: (成功与否, 结果字典)
        """
        logger.info(f"Processing vectorization task: {task.id} for document {task.document_id}")
        
        try:
            # 检查任务类型
            if task.type != TaskType.VECTORIZE:
                raise ValueError(f"Invalid task type: {task.type}, expected: {TaskType.VECTORIZE}")
            
            # 获取向量化参数
            chunks = task.payload.get("chunks")
            if not chunks:
                raise ValueError("Missing chunks in task payload")
                
            model = task.payload.get("model", "default")
            
            # 获取嵌入器
            embedder = self.get_embedder(model)
            
            # 提取文本
            texts = []
            for chunk in chunks:
                if isinstance(chunk, dict) and "text" in chunk:
                    texts.append(chunk["text"])
                else:
                    raise ValueError("Invalid chunk format, missing 'text' field")
            
            # 批量嵌入
            logger.info(f"Vectorizing {len(texts)} chunks using model: {embedder.get_model_name()}")
            start_time = time.time()
            embeddings = embedder.embed_batch(texts)
            embed_time = time.time() - start_time
            logger.info(f"Vectorization completed in {embed_time:.2f}s")
            
            # 创建向量结果
            vectors = []
            for i, embedding in enumerate(embeddings):
                chunk_index = chunks[i].get("index", i)
                if isinstance(chunk_index, dict) and "index" in chunk_index:
                    chunk_index = chunk_index["index"]
                    
                vectors.append(VectorInfo(
                    chunk_index=chunk_index,
                    vector=embedding
                ))
            
            result = VectorizeResult(
                document_id=task.document_id,
                vectors=vectors,
                vector_count=len(vectors),
                model=embedder.get_model_name(),
                dimension=len(vectors[0].vector) if vectors else 0
            )
            
            return True, result.__dict__
            
        except Exception as e:
            logger.error(f"Error vectorizing text: {str(e)}")
            return False, {"error": str(e)}

    def process_task(self, task: Task) -> Tuple[bool, Dict[str, Any]]:
        """
        处理任务，根据任务类型调用相应的处理方法

        参数:
            task: 任务对象

        返回:
            Tuple[bool, Dict[str, Any]]: (成功与否, 结果字典)
        """
        logger.info(f"Processing task {task.id} of type {task.type}")
        
        if not task.type:
            logger.error(f"Task {task.id} has no type")
            return False, {"error": "Task has no type"}
        
        # 根据任务类型调用相应的处理方法
        if task.type == TaskType.DOCUMENT_PARSE:
            return self.parse_document(task)
        elif task.type == TaskType.TEXT_CHUNK:
            return self.chunk_text(task)
        elif task.type == TaskType.VECTORIZE:
            return self.vectorize_text(task)
        elif task.type == TaskType.PROCESS_COMPLETE:
            return self.process_document(task)
        else:
            logger.error(f"Unknown task type: {task.type}")
            return False, {"error": f"Unknown task type: {task.type}"}

# 创建处理器实例
document_processor = DocumentProcessor()