"""
文档处理工厂模块

提供创建文档解析器和文本分块器的工厂函数，
以及用于检测文件类型和处理文档的工具函数。
"""

import os
import mimetypes
from typing import Optional, Dict, Any, Tuple, List
import tempfile

from app.document_processing.parser import DocumentParser
from app.document_processing.chunker import DocumentChunker, ChunkOptions
from app.document_processing.adapters import DocumentParserAdapter, TextChunkerAdapter
from app.utils.utils import logger
from app.utils.minio_client import get_minio_client

# 获取MinIO客户端
minio_client = get_minio_client()

# MIME类型映射表
MIME_TYPE_MAPPING = {
    "application/pdf": "pdf",
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
    "application/msword": "doc",
    "application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
    "application/vnd.ms-powerpoint": "ppt",
    "text/markdown": "md",
    "text/html": "html",
    "text/plain": "txt",
    "application/json": "json",
    "application/rtf": "rtf"
}

# 扩展名映射表
EXTENSION_MAPPING = {
    ".pdf": "pdf",
    ".docx": "docx",
    ".doc": "doc",
    ".pptx": "pptx", 
    ".ppt": "ppt",
    ".md": "md",
    ".markdown": "md",
    ".html": "html",
    ".htm": "html",
    ".txt": "txt",
    ".json": "json",
    ".rtf": "rtf"
}


def detect_content_type(file_path: str) -> str:
    """
    检测文件的MIME类型
    
    参数:
        file_path: 文件路径
        
    返回:
        str: MIME类型
    """
    # 首先通过文件扩展名猜测MIME类型
    mime_type, _ = mimetypes.guess_type(file_path)
    
    # 如果无法通过扩展名确定，尝试根据文件扩展名硬编码
    if not mime_type:
        ext = os.path.splitext(file_path)[1].lower()
        mime_mapping = {
            '.pdf': 'application/pdf',
            '.md': 'text/markdown',
            '.markdown': 'text/markdown',
            '.txt': 'text/plain',
            '.html': 'text/html',
            '.htm': 'text/html',
            '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
            '.doc': 'application/msword',
            '.pptx': 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
            '.ppt': 'application/vnd.ms-powerpoint',
            '.json': 'application/json',
            '.rtf': 'application/rtf',
        }
        mime_type = mime_mapping.get(ext, 'application/octet-stream')
    
    logger.info(f"Detected MIME type for {file_path}: {mime_type}")
    return mime_type


def get_file_from_minio(file_path: str) -> Tuple[str, bool]:
    """
    从MinIO获取文件到本地临时路径
    
    参数:
        file_path: MinIO中的文件路径
        
    返回:
        Tuple[str, bool]: (本地文件路径, 是否为临时文件)
    """
    if not minio_client:
        logger.warning(f"MinIO client not initialized, cannot download file: {file_path}")
        return file_path, False
        
    try:
        # 获取文件扩展名
        file_ext = os.path.splitext(file_path)[1]
        
        # 创建临时文件
        with tempfile.NamedTemporaryFile(delete=False, suffix=file_ext) as temp_file:
            temp_path = temp_file.name
            
        logger.info(f"Downloading file from MinIO: {file_path} to {temp_path}")
        success = minio_client.download_file(file_path, temp_path)
        
        if success:
            logger.info(f"Successfully downloaded file to {temp_path}")
            return temp_path, True
        else:
            logger.error(f"Failed to download file from MinIO: {file_path}")
            return file_path, False
            
    except Exception as e:
        logger.error(f"Error accessing file from MinIO: {str(e)}")
        return file_path, False


def create_parser(file_path: str, mime_type: Optional[str] = None, file_extension: Optional[str] = None) -> DocumentParser:
    """
    创建适合指定文件的文档解析器
    
    参数:
        file_path: 文件路径
        mime_type: MIME类型(可选，自动检测)
        file_extension: 文件扩展名(可选，自动检测)
        
    返回:
        DocumentParser: 文档解析器实例
    """
    # 如果未提供mime_type，尝试检测
    if not mime_type:
        mime_type = detect_content_type(file_path)
    
    # 创建解析器
    parser = DocumentParser(file_path, mime_type, file_extension)
    logger.info(f"Created document parser for file: {file_path}")
    
    return parser


def create_chunker(
    chunk_size: int = 1000,
    chunk_overlap: int = 200, 
    split_type: str = "sentence"
) -> DocumentChunker:
    """
    创建文本分块器
    
    参数:
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        split_type: 分块类型 (paragraph, sentence, token, semantic, hierarchical)
        
    返回:
        DocumentChunker: 文档分块器实例
    """
    options = ChunkOptions(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
        split_type=split_type
    )
    
    chunker = DocumentChunker(options)
    logger.info(f"Created document chunker with type: {split_type}, size: {chunk_size}, overlap: {chunk_overlap}")
    
    return chunker


def process_file(
    file_path: str, 
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
    split_type: str = "sentence",
    metadata: Optional[Dict[str, Any]] = None
) -> Tuple[str, List[Dict[str, Any]], Dict[str, Any]]:
    """
    完整处理文件：解析并分块
    
    参数:
        file_path: 文件路径
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        split_type: 分块类型
        metadata: 附加元数据
        
    返回:
        Tuple[str, List[Dict[str, Any]], Dict[str, Any]]: (文档内容, 分块列表, 元数据)
    """
    is_temp_file = False
    
    try:
        # 检查文件是否需要从MinIO下载
        if minio_client and not os.path.exists(file_path) and not file_path.startswith('http'):
            local_path, is_temp_file = get_file_from_minio(file_path)
            if is_temp_file:
                file_path = local_path
        
        # 解析文档
        parser = create_parser(file_path)
        content = parser.parse()
        doc_metadata = parser.get_metadata()
        
        # 分块文本
        chunker = create_chunker(chunk_size, chunk_overlap, split_type)
        
        # 合并元数据
        if metadata:
            doc_metadata.update(metadata)
        
        chunks = chunker.chunk_text(content, doc_metadata)
        
        logger.info(f"Successfully processed file {file_path}: {len(chunks)} chunks created")
        
        return content, chunks, doc_metadata
        
    finally:
        # 清理临时文件
        if is_temp_file and os.path.exists(file_path):
            try:
                os.remove(file_path)
                logger.info(f"Removed temporary file: {file_path}")
            except Exception as e:
                logger.warning(f"Failed to remove temporary file {file_path}: {str(e)}")


def create_legacy_parser(file_path: Optional[str] = None, mime_type: Optional[str] = None) -> DocumentParserAdapter:
    """
    创建兼容旧API的文档解析器适配器
    
    参数:
        file_path: 文件路径
        mime_type: MIME类型
        
    返回:
        DocumentParserAdapter: 文档解析器适配器
    """
    return DocumentParserAdapter(file_path, mime_type)


def create_legacy_chunker() -> TextChunkerAdapter:
    """
    创建兼容旧API的文本分块器适配器
    
    返回:
        TextChunkerAdapter: 文本分块器适配器
    """
    return TextChunkerAdapter()


def detect_language(text: str, default_language: str = "en") -> str:
    """
    检测文本语言
    
    参数:
        text: 要检测的文本
        default_language: 默认语言
        
    返回:
        str: 语言代码
    """
    if not text:
        return default_language
        
    try:
        from langdetect import detect
        # 使用前1000个字符检测语言
        return detect(text[:1000])
    except Exception as e:
        logger.warning(f"Error detecting language: {str(e)}. Using default: {default_language}")
        return default_language


def estimate_tokens(text: str) -> int:
    """
    估算文本中的标记数量
    
    参数:
        text: 要分析的文本
        
    返回:
        int: 估算的标记数量
    """
    if not text:
        return 0
        
    # 一个简单的启发式方法：按空格分词，每4个字符约为1个token
    words = text.split()
    chars = len(text)
    
    # 中文和日文等语言没有空格，所以我们结合字符数和单词数
    estimated_tokens = max(len(words), chars // 4)
    
    return estimated_tokens