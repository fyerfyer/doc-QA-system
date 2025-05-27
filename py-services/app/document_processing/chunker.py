import time
from typing import List, Dict, Any, Optional
from dataclasses import dataclass

from llama_index.core import Document as LlamaDocument
from llama_index.core.node_parser import (
    SentenceSplitter,
    TokenTextSplitter,
    SemanticSplitterNodeParser,
    HierarchicalNodeParser
)

from app.utils.utils import logger, count_words
from app.utils.llama_config import DEFAULT_CHUNK_SIZE, DEFAULT_CHUNK_OVERLAP

@dataclass
class ChunkOptions:
    """文本分块选项"""
    chunk_size: int = DEFAULT_CHUNK_SIZE
    chunk_overlap: int = DEFAULT_CHUNK_OVERLAP
    split_type: str = "sentence"
    include_metadata: bool = True
    include_stats: bool = True


class DocumentChunker:
    """
    LlamaIndex 文档分块器
    
    使用 LlamaIndex 的分块器将文档内容分割为更小的块，
    支持多种分块策略，包括段落、句子、标记和语义分块
    """
    
    def __init__(self, options: Optional[ChunkOptions] = None):
        """
        初始化文档分块器
        
        参数:
            options: 分块选项，如果未提供则使用默认值
        """
        self.options = options or ChunkOptions()
        
    def chunk_text(
        self, 
        text: str,
        metadata: Optional[Dict[str, Any]] = None,
        chunk_options: Optional[ChunkOptions] = None
    ) -> List[Dict[str, Any]]:
        """
        将文本分割为块
        
        参数:
            text: 要分块的文本
            metadata: 要添加到每个块的元数据
            chunk_options: 分块选项，覆盖实例默认选项
            
        返回:
            List[Dict[str, Any]]: 分块结果，每个块包含文本和索引
        """
        start_time = time.time()
        options = chunk_options or self.options
        
        if not text or text.strip() == "":
            logger.warning("Empty text provided for chunking")
            return []
        
        # 准备基本元数据
        base_metadata = metadata or {}
        
        # 创建LlamaIndex文档
        doc = LlamaDocument(text=text, metadata=base_metadata)
        
        # 获取适当的分块器
        splitter = self._get_splitter(options)
        logger.info(f"Using splitter: {splitter.__class__.__name__} with chunk_size={options.chunk_size}, overlap={options.chunk_overlap}")
        
        try:
            # 执行分块
            nodes = splitter.get_nodes_from_documents([doc])
            process_time = time.time() - start_time
            
            logger.info(f"Split text into {len(nodes)} chunks in {process_time:.2f}s")
            
            # 转换为标准格式
            chunks = []
            for i, node in enumerate(nodes):
                # 创建块的元数据 
                # 合并传入的元数据
                chunk_metadata = {}
                if options.include_metadata and metadata:
                    chunk_metadata.update(metadata)  # 先添加传入的元数据
                if node.metadata:
                    chunk_metadata.update(node.metadata)  # 再添加节点元数据
                
                # 添加统计信息
                if options.include_stats:
                    chunk_metadata.update({
                        "chunk_index": i,
                        "chars": len(node.text),
                        "words": count_words(node.text),
                        "chunk_type": options.split_type
                    })
                
                # 创建块对象
                chunk = {
                    "text": node.text,
                    "index": i,
                    "metadata": chunk_metadata
                }
                
                chunks.append(chunk)
            
            return chunks
            
        except Exception as e:
            logger.error(f"Error chunking text: {str(e)}")
            raise
    
    def _get_splitter(self, options: ChunkOptions):
        """
        根据分块选项选择适当的分块器
        
        参数:
            options: 分块选项
            
        返回:
            适当的LlamaIndex分块器实例
        """
        split_type = options.split_type.lower()
        
        if split_type == "sentence":
            return SentenceSplitter(
                chunk_size=options.chunk_size,
                chunk_overlap=options.chunk_overlap
            )
        elif split_type == "token":
            return TokenTextSplitter(
                chunk_size=options.chunk_size,
                chunk_overlap=options.chunk_overlap
            )
        elif split_type == "semantic":
            try:
                # 尝试加载嵌入模型
                try:
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
                    logger.warning("Semantic splitting requires embedding model. Falling back to paragraph splitting.")
                    return SentenceSplitter(
                        chunk_size=options.chunk_size,
                        chunk_overlap=options.chunk_overlap,
                        paragraph_separator="\n\n"
                    )
                    
            except Exception as e:
                logger.error(f"Failed to initialize semantic splitter: {str(e)}")
                logger.info("Falling back to paragraph splitter")
                return SentenceSplitter(
                    chunk_size=options.chunk_size,
                    chunk_overlap=options.chunk_overlap,
                    paragraph_separator="\n\n"
                )
                
        elif split_type == "hierarchical":
            # 分层分块（粗到细）
            try:
                return HierarchicalNodeParser.from_defaults(
                    chunk_sizes=[options.chunk_size, options.chunk_size // 2, options.chunk_size // 4],
                    chunk_overlap=options.chunk_overlap
                )
            except Exception as e:
                logger.error(f"Failed to initialize hierarchical splitter: {str(e)}")
                logger.info("Falling back to paragraph splitter")
                return SentenceSplitter(
                    chunk_size=options.chunk_size,
                    chunk_overlap=options.chunk_overlap,
                    paragraph_separator="\n\n"
                )
        
        else:
            # 默认为段落分块（适用于paragraph和默认情况）
            return SentenceSplitter(
                chunk_size=options.chunk_size,
                chunk_overlap=options.chunk_overlap,
                paragraph_separator="\n\n"
            )
    
    def estimate_chunks(self, text: str, options: Optional[ChunkOptions] = None) -> Dict[str, Any]:
        """
        估算文本将产生的块数
        
        参数:
            text: 要估算的文本
            options: 分块选项
            
        返回:
            Dict[str, Any]: 包含估算信息的字典
        """
        if not text:
            return {"estimated_chunks": 0, "chars": 0, "words": 0}
        
        opts = options or self.options
        
        # 简单估算（基于字符数）
        total_chars = len(text)
        words = count_words(text)
        
        # 估算块的有效大小（考虑到重叠）
        effective_chunk_size = opts.chunk_size - opts.chunk_overlap
        if effective_chunk_size <= 0:
            effective_chunk_size = 1  # 避免除零错误
            
        # 估算块数量
        estimated_chunks = max(1, total_chars // effective_chunk_size)
        
        return {
            "estimated_chunks": estimated_chunks,
            "chars": total_chars,
            "words": words,
            "chunk_size": opts.chunk_size,
            "chunk_overlap": opts.chunk_overlap,
            "split_type": opts.split_type
        }


def chunk_text(
    text: str,
    chunk_size: int = DEFAULT_CHUNK_SIZE,
    chunk_overlap: int = DEFAULT_CHUNK_OVERLAP,
    split_type: str = "sentence",
    metadata: Optional[Dict[str, Any]] = None
) -> List[Dict[str, Any]]:
    """
    将文本分割为块的便捷函数
    
    参数:
        text: 要分块的文本
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        split_type: 分块类型
        metadata: 要添加到每个块的元数据
        
    返回:
        List[Dict[str, Any]]: 分块结果，每个块包含文本和索引
    """
    options = ChunkOptions(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
        split_type=split_type
    )
    
    chunker = DocumentChunker(options)
    return chunker.chunk_text(text, metadata)


def get_chunker(
    chunk_size: int = DEFAULT_CHUNK_SIZE,
    chunk_overlap: int = DEFAULT_CHUNK_OVERLAP,
    split_type: str = "sentence"
) -> DocumentChunker:
    """
    获取文档分块器实例的便捷函数
    
    参数:
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        split_type: 分块类型
        
    返回:
        DocumentChunker: 文档分块器实例
    """
    options = ChunkOptions(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
        split_type=split_type
    )
    
    return DocumentChunker(options)