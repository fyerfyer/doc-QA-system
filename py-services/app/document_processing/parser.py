import os
import tempfile
from pathlib import Path
from typing import Dict, Any, Optional, List, Tuple
import mimetypes
import time

from llama_index.core import Document as LlamaDocument
from llama_index.readers.file import (
    PyMuPDFReader,
    DocxReader,
    PDFReader,
    FlatReader
)
from llama_index.core.node_parser import (
    MarkdownNodeParser,
    HTMLNodeParser,
)

# 导入应用内部的组件
from app.utils.utils import logger, count_words, count_chars
from app.utils.minio_client import get_minio_client

# 获取MinIO客户端
minio_client = get_minio_client()


class DocumentParser:
    """
    LlamaIndex 文档解析器
    
    使用 LlamaIndex 的文件读取器和解析器处理各种文档类型，
    支持直接从文件路径解析或通过内存中的文件内容解析
    """
    
    def __init__(
        self, 
        file_path: Optional[str] = None, 
        mime_type: Optional[str] = None,
        file_extension: Optional[str] = None
    ):
        """
        初始化文档解析器
        
        参数:
            file_path: 文件路径（可选）
            mime_type: MIME类型（可选，自动检测）
            file_extension: 文件扩展名（可选，自动检测）
        """
        self.file_path = file_path
        self.mime_type = mime_type
        self.file_extension = file_extension
        
        # 如果未提供mime_type但提供了文件路径，尝试检测
        if not mime_type and file_path:
            self.mime_type = self._detect_mime_type(file_path)
        
        # 如果未提供file_extension但提供了文件路径，尝试提取
        if not file_extension and file_path:
            self.file_extension = os.path.splitext(file_path)[1].lower()
            
        # 解析结果
        self.content = ""
        self.metadata = {}
        self.nodes = []
        self.document = None
    
    def parse(self, file_path: Optional[str] = None) -> str:
        """
        解析文档内容
        
        参数:
            file_path: 文件路径（可选，如果在构造函数中已提供）
            
        返回:
            str: 解析后的文档文本内容
        """
        path = file_path or self.file_path
        if not path:
            raise ValueError("File path must be provided")
        
        start_time = time.time()
        logger.info(f"Starting to parse document: {path}")
        
        try:
            # 检查文件是否存在于MinIO中
            if minio_client and not os.path.exists(path) and not path.startswith('http'):
                # 可能是MinIO路径，尝试下载
                try:
                    with tempfile.NamedTemporaryFile(delete=False, suffix=self.file_extension or "") as temp_file:
                        temp_path = temp_file.name
                    
                    logger.info(f"Downloading file from MinIO: {path} to {temp_path}")
                    success = minio_client.download_file(path, temp_path)
                    
                    if success:
                        path = temp_path
                    else:
                        raise FileNotFoundError(f"Failed to download file from MinIO: {path}")
                except Exception as e:
                    logger.error(f"Error accessing file from MinIO: {str(e)}")
                    raise
            
            # 使用合适的LlamaIndex读取器加载文件
            reader = self._get_reader(path)
            logger.info(f"Using reader: {reader.__class__.__name__}")
            
            # 加载文档
            try:
                docs = reader.load_data(Path(path))
            except AttributeError:
                # 兼容某些读取器可能使用不同的方法名
                docs = reader.load(file_path=Path(path))
            
            if not docs:
                logger.warning(f"No content was loaded from file: {path}")
                return ""
            
            # 合并多文档内容
            if len(docs) > 1:
                logger.info(f"Merging {len(docs)} document sections")
                self.content = "\n\n".join([doc.get_content() for doc in docs])
            else:
                self.content = docs[0].get_content()
            
            # 提取元数据
            self._extract_metadata(docs, path)
            
            # 记录日志
            process_time = time.time() - start_time
            word_count = count_words(self.content)
            char_count = count_chars(self.content)
            
            logger.info(f"Document parsed successfully in {process_time:.2f}s: {word_count} words, {char_count} chars")
            
            # 清理临时文件
            if path.startswith(tempfile.gettempdir()) and os.path.exists(path):
                os.remove(path)
                logger.info(f"Removed temporary file: {path}")
            
            return self.content
            
        except Exception as e:
            logger.error(f"Error parsing document: {str(e)}")
            
            # 清理临时文件
            if path != file_path and path.startswith(tempfile.gettempdir()) and os.path.exists(path):
                os.remove(path)
                logger.info(f"Removed temporary file after error: {path}")
                
            raise
    
    def parse_content(self, content: str, file_name: Optional[str] = None) -> str:
        """
        直接解析文本内容
        
        参数:
            content: 文档内容
            file_name: 可选的文件名（用于元数据）
            
        返回:
            str: 解析后的文档文本
        """
        logger.info(f"Parsing content directly, length: {len(content)} chars")
        
        # 创建LlamaIndex文档对象
        doc = LlamaDocument(text=content)
        
        # 解析文件类型(如果有文件名)
        if file_name:
            extension = os.path.splitext(file_name)[1].lower()
            if extension in ['.md', '.markdown']:
                parser = MarkdownNodeParser()
                nodes = parser.get_nodes_from_documents([doc])
                self.content = "\n\n".join([node.text for node in nodes])
            elif extension in ['.html', '.htm']:
                parser = HTMLNodeParser()
                nodes = parser.get_nodes_from_documents([doc])
                self.content = "\n\n".join([node.text for node in nodes])
            else:
                self.content = content
        else:
            self.content = content
            
        # 添加基本元数据
        self.metadata = {
            "filename": file_name or "text_content",
            "extension": os.path.splitext(file_name)[1].lower() if file_name else "",
            "parsed_at": time.time(),
            "chars": len(self.content),
            "words": count_words(self.content)
        }
        
        return self.content
    
    def extract_title(self, content: Optional[str] = None, filename: Optional[str] = None) -> str:
        """
        从文档内容中提取标题
        
        参数:
            content: 文档内容(可选，如果已解析则使用解析结果)
            filename: 文件名(可选，如果无法提取标题则使用)
            
        返回:
            str: 文档标题
        """
        text = content or self.content
        
        # 尝试从元数据中获取标题
        title = self.metadata.get('title', '')
        
        # 如果元数据中没有标题，尝试从内容中提取
        if not title and text:
            # 简单的启发式方法：使用第一行非空文本作为标题
            lines = text.split('\n')
            for line in lines:
                line = line.strip()
                if line and len(line) < 100:  # 合理的标题长度
                    title = line
                    break
        
        # 如果仍然没有标题，使用文件名
        if not title:
            if filename:
                title = os.path.splitext(os.path.basename(filename))[0]
            elif self.file_path:
                title = os.path.splitext(os.path.basename(self.file_path))[0]
        
        # 如果仍然没有标题，使用默认值
        if not title:
            title = "Untitled Document"
            
        return title
    
    def get_metadata(self) -> Dict[str, Any]:
        """
        获取文档元数据
        
        返回:
            Dict[str, Any]: 文档元数据
        """
        return self.metadata
    
    def _detect_mime_type(self, file_path: str) -> str:
        """
        检测文件的MIME类型
        
        参数:
            file_path: 文件路径
            
        返回:
            str: MIME类型
        """
        # 首先通过文件扩展名猜测MIME类型
        mime_type, _ = mimetypes.guess_type(file_path)
        
        # 如果无法通过扩展名确定，尝试使用其他方法
        if not mime_type:
            extension = os.path.splitext(file_path)[1].lower()
            
            # 常见文件类型的映射
            extension_map = {
                '.pdf': 'application/pdf',
                '.md': 'text/markdown',
                '.markdown': 'text/markdown',
                '.txt': 'text/plain',
                '.html': 'text/html',
                '.htm': 'text/html',
                '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
                '.doc': 'application/msword',
                '.json': 'application/json',
            }
            
            mime_type = extension_map.get(extension, 'application/octet-stream')
        
        logger.info(f"Detected MIME type for {file_path}: {mime_type}")
        return mime_type
    
    def _get_reader(self, file_path: str):
        """
        根据文件类型获取合适的读取器
        
        参数:
            file_path: 文件路径
            
        返回:
            Reader: 适合该文件类型的LlamaIndex读取器
        """
        extension = os.path.splitext(file_path)[1].lower()
        mime_type = self.mime_type or self._detect_mime_type(file_path)
        
        # 基于MIME类型和扩展名选择合适的读取器
        if mime_type == 'application/pdf' or extension == '.pdf':
            try:
                return PyMuPDFReader()  # 优先使用PyMuPDF，性能更好
            except Exception:
                return PDFReader()
        elif mime_type in ['application/vnd.openxmlformats-officedocument.wordprocessingml.document', 
                          'application/msword'] or extension in ['.docx', '.doc']:
            return DocxReader()
        else:
            # 默认使用FlatReader
            return FlatReader()
    
    def _extract_metadata(self, docs: List[LlamaDocument], file_path: str) -> None:
        """
        从LlamaIndex文档中提取元数据
        
        参数:
            docs: LlamaIndex文档列表
            file_path: 文件路径
        """
        # 合并所有文档的元数据
        combined_metadata = {}
        for doc in docs:
            if hasattr(doc, 'metadata') and doc.metadata:
                combined_metadata.update(doc.metadata)
        
        # 基本文件信息
        file_info = {
            'filename': os.path.basename(file_path),
            'extension': os.path.splitext(file_path)[1].lower(),
            'file_size': os.path.getsize(file_path) if os.path.exists(file_path) else 0,
            'parsed_at': time.time(),
        }
        
        # 添加文档统计信息
        stats = {
            'page_count': len(docs),
            'chars': len(self.content),
            'words': count_words(self.content)
        }
        
        # 合并所有元数据
        self.metadata = {**combined_metadata, **file_info, **stats}
        
        # 添加一些PDF特有的元数据
        if self.metadata.get('extension') == '.pdf':
            # PyMuPDF可能已经提取了一些元数据
            for key in ['title', 'author', 'subject', 'creator']:
                if key not in self.metadata and key in combined_metadata:
                    self.metadata[key] = combined_metadata[key]


def parse_document(file_path: str, mime_type: Optional[str] = None) -> Tuple[str, Dict[str, Any]]:
    """
    解析文档的便捷函数
    
    参数:
        file_path: 文件路径
        mime_type: MIME类型(可选)
        
    返回:
        Tuple[str, Dict[str, Any]]: (文档内容, 元数据)
    """
    parser = DocumentParser(file_path, mime_type)
    content = parser.parse()
    metadata = parser.get_metadata()
    
    return content, metadata


def parse_content(content: str, file_name: Optional[str] = None) -> Tuple[str, Dict[str, Any]]:
    """
    直接解析文本内容的便捷函数
    
    参数:
        content: 文档内容
        file_name: 文件名(可选)
        
    返回:
        Tuple[str, Dict[str, Any]]: (解析后的文档内容, 元数据)
    """
    parser = DocumentParser()
    parsed_content = parser.parse_content(content, file_name)
    metadata = parser.get_metadata()
    
    return parsed_content, metadata


def get_parser(file_path: Optional[str] = None, mime_type: Optional[str] = None) -> DocumentParser:
    """
    获取文档解析器实例
    
    参数:
        file_path: 文件路径(可选)
        mime_type: MIME类型(可选)
        
    返回:
        DocumentParser: 文档解析器实例
    """
    return DocumentParser(file_path, mime_type)