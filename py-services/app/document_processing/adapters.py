import os
from typing import Dict, List, Any, Optional
from pathlib import Path
import time

from llama_index.core import Document
from llama_index.core.node_parser import (
    SentenceSplitter,
    TokenTextSplitter,
    SemanticSplitterNodeParser
)
from llama_index.readers.file import FlatReader, PDFReader, DocxReader

# 导入应用内部的组件
from app.utils.utils import logger

class DocumentParserAdapter:
    """
    文档解析适配器 - 使用LlamaIndex的解析器但保持与原有API兼容
    """
    
    def __init__(self, file_path: str = None, mime_type: str = None):
        """
        初始化文档解析适配器
        
        参数:
            file_path: 文件路径
            mime_type: MIME类型
        """
        self.file_path = file_path
        self.mime_type = mime_type
        self.metadata = {}
        self.logger = logger
    
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
        
        try:
            # 使用合适的阅读器加载文件
            reader = self._get_reader_for_file(path)
            self.logger.info(f"Using reader: {reader.__class__.__name__} for file: {path}")
            
            # 加载文档
            start_time = time.time()
            docs = reader.load_data(Path(path))
            self.logger.info(f"Loaded document in {time.time() - start_time:.2f}s")
            
            # 如果没有文档加载，返回空字符串
            if not docs:
                self.logger.warning(f"No content was loaded from file: {path}")
                return ""
            
            # 如果加载了多个文档，合并它们
            if len(docs) > 1:
                self.logger.info(f"Merging {len(docs)} documents")
                content = "\n\n".join([doc.get_content() for doc in docs])
            else:
                content = docs[0].get_content()
            
            # 保存元数据
            if docs and hasattr(docs[0], 'metadata'):
                self.metadata = docs[0].metadata or {}
            
            return content
            
        except Exception as e:
            self.logger.error(f"Error parsing document: {str(e)}")
            raise
    
    def extract_title(self, content: str, filename: str = None) -> str:
        """
        从文档内容中提取标题
        
        参数:
            content: 文档内容
            filename: 文件名（如果无法提取标题则使用）
            
        返回:
            str: 文档标题
        """
        # 尝试从元数据中获取标题
        title = self.metadata.get('title', '')
        
        # 如果元数据中没有标题，尝试从内容中提取
        if not title and content:
            # 简单的启发式方法：使用第一行非空文本作为标题
            lines = content.split('\n')
            for line in lines:
                line = line.strip()
                if line and len(line) < 100:  # 合理的标题长度
                    title = line
                    break
        
        # 如果仍然没有标题，使用文件名
        if not title and filename:
            title = os.path.splitext(os.path.basename(filename))[0]
        
        # 如果仍然没有标题，使用默认值
        if not title:
            title = "Untitled Document"
            
        return title
    
    def get_metadata(self, file_path: Optional[str] = None) -> Dict[str, Any]:
        """
        获取文档元数据
        
        参数:
            file_path: 文件路径
            
        返回:
            Dict[str, Any]: 文档元数据
        """
        # 如果已经有元数据，直接返回
        if self.metadata:
            return self.metadata
        
        path = file_path or self.file_path
        if not path:
            return {}
            
        try:
            # 基本文件信息
            file_info = {
                'filename': os.path.basename(path),
                'extension': os.path.splitext(path)[1].lower(),
                'file_size': os.path.getsize(path) if os.path.exists(path) else 0,
            }
            
            # 合并已有的元数据和文件信息
            meta = {**self.metadata, **file_info}
            
            return meta
        except Exception as e:
            self.logger.error(f"Error getting metadata: {str(e)}")
            return {'error': str(e)}
    
    def _get_reader_for_file(self, file_path: str):
        """
        根据文件类型获取合适的阅读器
        
        参数:
            file_path: 文件路径
            
        返回:
            Reader: LlamaIndex阅读器实例
        """
        from llama_index.readers.file import FlatReader, PDFReader, DocxReader
        
        ext = os.path.splitext(file_path)[1].lower()
        
        # 根据扩展名选择阅读器
        if ext == '.pdf':
            return PDFReader()
        elif ext in ['.docx', '.doc']:
            return DocxReader()
        else:
            # 对于其他类型的文件，使用FlatReader
            return FlatReader()


class TextChunkerAdapter:
    """
    文本分块适配器 - 使用LlamaIndex的文本分块器但保持与原有API兼容
    """
    
    def __init__(self):
        """初始化文本分块适配器"""
        self.logger = logger
    
    def split_text(
        self, 
        text: str,
        chunk_size: int = 1000,
        chunk_overlap: int = 200,
        split_type: str = "sentence",
        metadata: Optional[Dict[str, Any]] = None
    ) -> List[Dict[str, Any]]:
        """
        将文本分割为块
        
        参数:
            text: 要分块的文本
            chunk_size: 块大小
            chunk_overlap: 块重叠大小
            split_type: 分块类型（paragraph, sentence, token, semantic）
            metadata: 要添加到每个块的元数据
            
        返回:
            List[Dict[str, Any]]: 分块结果，每个块包含文本和索引
        """
        if not text:
            return []
            
        try:
            # 创建LlamaIndex文档
            doc = Document(text=text, metadata=metadata or {})
            
            # 根据分块类型选择分块器
            splitter = self._get_splitter(split_type, chunk_size, chunk_overlap)
            self.logger.info(f"Using splitter: {splitter.__class__.__name__} with chunk_size={chunk_size}, overlap={chunk_overlap}")
            
            # 执行分块
            start_time = time.time()
            nodes = splitter.get_nodes_from_documents([doc])
            self.logger.info(f"Split text into {len(nodes)} chunks in {time.time() - start_time:.2f}s")
            
            # 转换为块字典列表
            chunks = []
            for i, node in enumerate(nodes):
                chunks.append({
                    "text": node.text,
                    "index": i,
                    "metadata": {**node.metadata} if node.metadata else {}
                })
                
            return chunks
            
        except Exception as e:
            self.logger.error(f"Error splitting text: {str(e)}")
            raise
    
    def _get_splitter(self, split_type: str, chunk_size: int, chunk_overlap: int):
        """
        根据分块类型获取合适的分块器
        
        参数:
            split_type: 分块类型
            chunk_size: 块大小
            chunk_overlap: 块重叠大小
            
        返回:
            Splitter: LlamaIndex分块器实例
        """
        split_type = split_type.lower()
        
        if split_type == "sentence":
            return SentenceSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap)
        elif split_type == "token":
            return TokenTextSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap)
        elif split_type == "semantic":
            try:
                # 注意：语义分块器需要嵌入模型
                from app.embedders.factory import get_default_embedder
                embedder = get_default_embedder()
                
                # 创建适配器使嵌入模型与LlamaIndex兼容
                from llama_index.core.embeddings import BaseEmbedding
                
                class EmbedderAdapter(BaseEmbedding):
                    def __init__(self, embedder):
                        super().__init__()
                        self.embedder = embedder
                        
                    def _get_text_embedding(self, text: str) -> List[float]:
                        return self.embedder.embed(text)
                        
                    def _get_text_embeddings(self, texts: List[str]) -> List[List[float]]:
                        return self.embedder.embed_batch(texts)
                
                embed_model = EmbedderAdapter(embedder)
                
                return SemanticSplitterNodeParser(
                    buffer_size=1,
                    breakpoint_percentile_threshold=95,
                    embed_model=embed_model
                )
            except ImportError:
                self.logger.warning("Semantic splitting requires embedding model. Falling back to paragraph splitting.")
                return SentenceSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap, paragraph_separator="\n\n")
        else:
            # 默认使用段落分块器（适用于paragraph和默认情况）
            return SentenceSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap, paragraph_separator="\n\n")


def create_document_parser(file_path: str = None, mime_type: str = None) -> DocumentParserAdapter:
    """
    创建文档解析适配器
    
    参数:
        file_path: 文件路径
        mime_type: MIME类型
        
    返回:
        DocumentParserAdapter: 文档解析适配器
    """
    return DocumentParserAdapter(file_path, mime_type)


def create_text_chunker() -> TextChunkerAdapter:
    """
    创建文本分块适配器
    
    返回:
        TextChunkerAdapter: 文本分块适配器
    """
    return TextChunkerAdapter()