import os
import time
import traceback

from app.models.model import (
    Task, TaskType, TaskStatus,
    DocumentParsePayload, DocumentParseResult,
    TextChunkPayload, TextChunkResult, ChunkInfo,
    VectorizePayload, VectorizeResult, VectorInfo,
    ProcessCompletePayload, ProcessCompleteResult
)

from app.parsers.factory import create_parser, detect_content_type
from app.chunkers.splitter import split_text, TextSplitter, SplitConfig
from app.embedders.factory import create_embedder, get_default_embedder
from app.utils.utils import logger, send_callback, retry, count_words, count_chars
from app.worker.tasks import get_redis_client, update_task_status
from app.embedders.base import BaseEmbedder
from app.worker.tasks import get_task_from_redis

class DocumentProcessor:
    """文档处理器，负责执行文档的解析、分块和向量化任务"""

    def __init__(self):
        """初始化文档处理器"""
        self.logger = logger
        
        # 确保环境变量已配置
        self.dashscope_api_key = os.getenv("DASHSCOPE_API_KEY", "")
        self.embedding_model = os.getenv("EMBEDDING_MODEL", "text-embedding-v3")
        self.callback_url = os.getenv("CALLBACK_URL", "http://localhost:8080/api/tasks/callback")
        
        # 设置代理配置（用于HuggingFace）
        self.proxies = {
            "http": "http://127.0.0.1:7897",
            "https": "http://127.0.0.1:7897"
        }

        # 创建嵌入模型实例
        self._initialize_embedding_client()

        self.logger.info("Document processor initialized")

    def _initialize_embedding_client(self):
        """初始化嵌入模型客户端"""
        # 尝试使用通义千问API（首选）
        if self.dashscope_api_key:
            try:
                self.embedder = create_embedder(
                    "tongyi",
                    api_key=self.dashscope_api_key,
                    model_name=self.embedding_model
                )
                self.logger.info(f"Successfully initialized Tongyi embedder: {self.embedder.get_model_name()}")
                return
            except Exception as e:
                self.logger.warning(f"Failed to initialize Tongyi embedder: {str(e)}, falling back to default embedder")
        else:
            self.logger.warning("No DashScope API key found, falling back to default embedder")

        # 使用默认嵌入器（回退）
        try:
            self.embedder = get_default_embedder()
            self.logger.info(f"Successfully initialized default embedder: {self.embedder.get_model_name()}")
            return
        except Exception as e:
            self.logger.warning(f"Failed to initialize default embedder: {str(e)}, using fallback embedder")

        # 零向量嵌入器（最后回退）
        self.logger.warning("Using zero-vector fallback embedder (for testing/development only)")
        self.embedder = self._create_fallback_embedder()
        self.logger.info("Fallback embedder initialized")

    def _create_fallback_embedder(self):
        """创建简单的回退嵌入器（用于测试或错误情况）"""

        class FallbackEmbedder(BaseEmbedder):
            def __init__(self):
                super().__init__(model_name="fallback-embedder", dimension=1536)
                self.logger = logger

            def embed(self, text):
                # 生成固定维度的零向量
                self.logger.debug(f"Using fallback embedder to embed text: {text[:50]}...")
                return [0.0] * 1536

            def embed_batch(self, texts):
                # 为每个文本生成零向量
                self.logger.debug(f"Using fallback embedder to embed batch of {len(texts)} texts")
                return [[0.0] * 1536 for _ in texts]

            def get_model_name(self):
                return "fallback-embedder"

        return FallbackEmbedder()

    def process_task(self, task: Task) -> bool:
        """
        处理任务入口点

        参数:
            task: 任务对象

        返回:
            bool: 处理成功返回True，失败返回False
        """
        if not task or not task.type:
            self.logger.error("Invalid task object received")
            return False

        self.logger.info(f"Processing task: {task.id}, type: {task.type}, document_id: {task.document_id}")

        # 根据任务类型调用适当的处理函数
        try:
            if task.type == TaskType.DOCUMENT_PARSE:
                return self.process_parse_document(task)
            elif task.type == TaskType.TEXT_CHUNK:
                return self.process_chunk_text(task)
            elif task.type == TaskType.VECTORIZE:
                return self.process_vectorize_text(task)
            elif task.type == TaskType.PROCESS_COMPLETE:
                return self.process_complete(task)
            else:
                self.logger.error(f"Unsupported task type: {task.type}")
                update_task_status(task, TaskStatus.FAILED, error=f"Unsupported task type: {task.type}")
                return False
        except Exception as e:
            error_msg = f"Error processing task {task.id}: {str(e)}\n{traceback.format_exc()}"
            self.logger.error(error_msg)
            update_task_status(task, TaskStatus.FAILED, error=error_msg)
            return False

    def process_parse_document(self, task: Task) -> bool:
        """
        处理文档解析任务

        参数:
            task: 文档解析任务

        返回:
            bool: 处理成功返回True，失败返回False
        """
        self.logger.info(f"Parsing document for task: {task.id}")

        # 更新任务状态为处理中
        update_task_status(task, TaskStatus.PROCESSING)

        try:
            # 解析任务载荷
            if not isinstance(task.payload, dict):
                raise ValueError("Task payload is not a dictionary")
            else:
                payload = task.payload

            # 验证必要字段
            if 'file_path' not in payload:
                raise ValueError("Missing required field 'file_path' in payload")

            file_path = payload.get('file_path')
            file_name = payload.get('file_name', os.path.basename(file_path))

            self.logger.info(f"Processing document: {file_path}")

            # 检测文件类型
            mime_type = detect_content_type(file_path)

            # 创建适当的解析器
            parser = create_parser(file_path, mime_type)

            # 解析文档
            start_time = time.time()
            content = parser.parse(file_path)

            # 提取文档标题
            title = parser.extract_title(content, file_name) if hasattr(parser, 'extract_title') else file_name

            # 获取文档元数据
            meta = parser.get_metadata(file_path) if hasattr(parser, 'get_metadata') else {}

            # 构建结果
            result = DocumentParseResult(
                content=content,
                title=title,
                meta=meta,
                pages=meta.get('page_count', 1) if isinstance(meta, dict) else 1,
                words=count_words(content),
                chars=count_chars(content)
            )

            elapsed = time.time() - start_time
            self.logger.info(f"Document parsing completed in {elapsed:.2f}s. Text length: {len(content)} chars")

            # 更新任务状态为已完成，包括结果
            update_task_status(task, TaskStatus.COMPLETED, result=result.__dict__)
            return True

        except Exception as e:
            error_msg = f"Document parse failed: {str(e)}\n{traceback.format_exc()}"
            self.logger.error(error_msg)
            update_task_status(task, TaskStatus.FAILED, error=error_msg)
            return False

    def process_chunk_text(self, task: Task) -> bool:
        """
        处理文本分块任务

        参数:
            task: 文本分块任务

        返回:
            bool: 处理成功返回True，失败返回False
        """
        self.logger.info(f"Chunking text for task: {task.id}")

        # 更新任务状态为处理中
        update_task_status(task, TaskStatus.PROCESSING)

        try:
            # 解析任务载荷
            if not isinstance(task.payload, dict):
                raise ValueError("Task payload is not a dictionary")
            else:
                payload = task.payload

            # 验证必要字段
            if 'content' not in payload:
                raise ValueError("Missing required field 'content' in payload")
            if 'document_id' not in payload:
                raise ValueError("Missing required field 'document_id' in payload")

            document_id = payload.get('document_id')
            content = payload.get('content')
            chunk_size = int(payload.get('chunk_size', 1000))
            overlap = int(payload.get('overlap', 200))
            split_type = payload.get('split_type', 'paragraph')

            self.logger.info(f"Chunking text for document {document_id}: {len(content)} chars, type: {split_type}")

            # 执行文本分块
            start_time = time.time()
            chunks = split_text(
                text=content,
                chunk_size=chunk_size,
                chunk_overlap=overlap,
                split_type=split_type
            )

            # 构建块信息列表
            chunk_infos = []
            for i, chunk in enumerate(chunks):
                chunk_infos.append(ChunkInfo(
                    text=chunk["text"],
                    index=chunk["index"]
                ))

            # 构建结果
            result = TextChunkResult(
                document_id=document_id,
                chunks=chunk_infos,
                chunk_count=len(chunk_infos)
            )

            elapsed = time.time() - start_time
            self.logger.info(f"Text chunking completed in {elapsed:.2f}s. Generated {len(chunk_infos)} chunks")

            # 更新任务状态为已完成，包括结果
            update_task_status(task, TaskStatus.COMPLETED, result=result.__dict__)
            return True

        except Exception as e:
            error_msg = f"Text chunking failed: {str(e)}\n{traceback.format_exc()}"
            self.logger.error(error_msg)
            update_task_status(task, TaskStatus.FAILED, error=error_msg)
            return False

    def process_vectorize_text(self, task: Task) -> bool:
        """
        处理文本向量化任务

        参数:
            task: 向量化任务

        返回:
            bool: 处理成功返回True，失败返回False
        """
        self.logger.info(f"Vectorizing text for task: {task.id}")

        # 更新任务状态为处理中
        update_task_status(task, TaskStatus.PROCESSING)

        try:
            # 解析任务载荷
            if not isinstance(task.payload, dict):
                raise ValueError("Task payload is not a dictionary")
            else:
                payload = task.payload

            # 验证必要字段
            if 'chunks' not in payload:
                raise ValueError("Missing required field 'chunks' in payload")
            if 'document_id' not in payload:
                raise ValueError("Missing required field 'document_id' in payload")

            document_id = payload.get('document_id')
            chunks = payload.get('chunks', [])
            model = payload.get('model', self.embedding_model)

            if not chunks:
                raise ValueError("Empty chunks list in payload")

            self.logger.info(f"Vectorizing {len(chunks)} chunks for document {document_id}")

            # 提取文本
            texts = []
            for chunk in chunks:
                if isinstance(chunk, dict):
                    texts.append(chunk.get("text", ""))
                else:
                    texts.append(chunk.text)

            # 批量生成向量
            start_time = time.time()
            vectors = self.embedder.embed_batch(texts)

            # 构建向量信息
            vector_infos = []
            dimension = len(vectors[0]) if vectors and vectors[0] else 0

            for i, vector in enumerate(vectors):
                chunk_idx = chunks[i].get('index', i) if isinstance(chunks[i], dict) else chunks[i].index
                vector_infos.append(VectorInfo(
                    chunk_index=chunk_idx,
                    vector=vector
                ))

            # 构建结果
            result = VectorizeResult(
                document_id=document_id,
                vectors=vector_infos,
                vector_count=len(vector_infos),
                model=model,
                dimension=dimension
            )

            elapsed = time.time() - start_time
            self.logger.info(f"Vectorization completed in {elapsed:.2f}s. Generated {len(vector_infos)} vectors with dimension {dimension}")

            # 更新任务状态为已完成，包括结果
            update_task_status(task, TaskStatus.COMPLETED, result=result.__dict__)
            return True

        except Exception as e:
            error_msg = f"Text vectorization failed: {str(e)}\n{traceback.format_exc()}"
            self.logger.error(error_msg)
            update_task_status(task, TaskStatus.FAILED, error=error_msg)
            return False

    def process_complete(self, task: Task) -> bool:
        """
        处理完整文档处理流程

        参数:
            task: 完整处理任务

        返回:
            bool: 处理成功返回True，失败返回False
        """
        self.logger.info(f"Processing complete document flow for task: {task.id}")

        # 更新任务状态为处理中
        update_task_status(task, TaskStatus.PROCESSING)

        try:
            # 解析任务载荷
            if not isinstance(task.payload, dict):
                raise ValueError("Task payload is not a dictionary")
            else:
                payload = task.payload

            # 验证必要字段
            if 'document_id' not in payload:
                raise ValueError("Missing required field 'document_id' in payload")
            if 'file_path' not in payload:
                raise ValueError("Missing required field 'file_path' in payload")

            document_id = payload.get('document_id')
            file_path = payload.get('file_path')
            file_name = payload.get('file_name', os.path.basename(file_path))
            chunk_size = int(payload.get('chunk_size', 1000))
            overlap = int(payload.get('overlap', 200))
            split_type = payload.get('split_type', 'paragraph')
            model = payload.get('model', self.embedding_model)

            self.logger.info(f"Starting complete processing for document: {document_id}, file: {file_path}")

            # 1. 解析文档
            start_time = time.time()
            self.logger.info("Step 1: Parsing document")
            mime_type = detect_content_type(file_path)
            parser = create_parser(file_path, mime_type)
            content = parser.parse(file_path)

            title = parser.extract_title(content, file_name) if hasattr(parser, 'extract_title') else file_name
            meta = parser.get_metadata(file_path) if hasattr(parser, 'get_metadata') else {}

            parse_elapsed = time.time() - start_time
            self.logger.info(f"Document parsing completed in {parse_elapsed:.2f}s. Text length: {len(content)} chars")
            parse_status = "success"

            # 2. 分块文本
            chunk_start_time = time.time()
            self.logger.info("Step 2: Chunking text")
            chunks = split_text(
                text=content,
                chunk_size=chunk_size,
                chunk_overlap=overlap,
                split_type=split_type
            )

            chunk_infos = []
            for i, chunk in enumerate(chunks):
                chunk_infos.append(ChunkInfo(
                    text=chunk["text"],
                    index=chunk["index"]
                ))

            chunk_elapsed = time.time() - chunk_start_time
            self.logger.info(f"Text chunking completed in {chunk_elapsed:.2f}s. Generated {len(chunk_infos)} chunks")
            chunk_status = "success"

            # 3. 向量化文本
            if not chunk_infos:
                vector_status = "skipped"
                vector_infos = []
                dimension = 0
                self.logger.warning("No chunks generated, skipping vectorization")
            else:
                vector_status = "success"
                vector_start_time = time.time()
                self.logger.info("Step 3: Vectorizing text")

                # 提取文本并向量化
                texts = [chunk.text for chunk in chunk_infos]
                vectors = self.embedder.embed_batch(texts)

                # 构建向量信息
                vector_infos = []
                dimension = len(vectors[0]) if vectors and vectors[0] else 0

                for i, vector in enumerate(vectors):
                    vector_infos.append(VectorInfo(
                        chunk_index=chunk_infos[i].index,
                        vector=vector
                    ))

                vector_elapsed = time.time() - vector_start_time
                self.logger.info(f"Vectorization completed in {vector_elapsed:.2f}s. Generated {len(vector_infos)} vectors")

            # 构建结果
            total_elapsed = time.time() - start_time
            result = ProcessCompleteResult(
                document_id=document_id,
                chunk_count=len(chunk_infos),
                vector_count=len(vector_infos),
                dimension=dimension,
                parse_status=parse_status,
                chunk_status=chunk_status,
                vector_status=vector_status,
                vectors=vector_infos
            )

            self.logger.info(f"Complete document processing finished in {total_elapsed:.2f}s")

            # 更新任务状态为已完成，包括结果
            update_task_status(task, TaskStatus.COMPLETED, result=result.__dict__)
            return True

        except Exception as e:
            error_msg = f"Complete document processing failed: {str(e)}\n{traceback.format_exc()}"
            self.logger.error(error_msg)
            update_task_status(task, TaskStatus.FAILED, error=error_msg)
            return False

# 创建处理器单例
document_processor = DocumentProcessor()

def process_task(task_id: str) -> bool:
    """
    处理任务的入口函数

    参数:
        task_id: 任务ID

    返回:
        bool: 处理成功返回True，失败返回False
    """

    # 获取任务信息
    task = get_task_from_redis(task_id)
    if not task:
        logger.error(f"Task {task_id} not found")
        return False

    return document_processor.process_task(task)