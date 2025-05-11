import logging
import numpy as np
from typing import List, Dict, Any, Optional, Callable, Tuple
from dataclasses import dataclass, field

# 导入基本分块器和工具函数
from app.chunkers.splitter import TextSplitter, SplitConfig, Chunk, split_text
from app.chunkers.utils import (
    normalize_text, split_sentences, detect_language,
    find_best_split_point, calculate_overlap, get_chunk_title,
    estimate_chunk_quality, count_tokens
)

# 初始化日志
logger = logging.getLogger(__name__)

@dataclass
class SemanticSplitConfig(SplitConfig):
    """语义分块配置，扩展基本分块配置"""
    similarity_threshold: float = 0.75      # 相似度阈值，低于此值的句子会被分到新块
    min_sentence_length: int = 5            # 最短有效句子长度
    buffer_size: int = 5                    # 缓冲区大小（分析窗口中的句子数）
    embedding_batch_size: int = 32          # 嵌入计算的批处理大小
    embedding_model: str = "default"        # 嵌入模型名称
    use_cached_embeddings: bool = True      # 是否使用缓存的嵌入
    embedding_cache_ttl: int = 3600         # 嵌入缓存的有效期（秒）
    embeddings_cache: Dict[str, List[float]] = field(default_factory=dict)  # 嵌入缓存


class SemanticSplitter(TextSplitter):
    """
    语义文本分块器

    基于文本的语义连贯性进行智能分块，而不是简单地按固定长度或段落分割。
    """

    def __init__(self, config: Optional[SemanticSplitConfig] = None,
                 embedding_func: Optional[Callable[[List[str]], List[List[float]]]] = None):
        """
        初始化语义分块器

        参数:
            config: 语义分块配置，如果为None则使用默认配置
            embedding_func: 嵌入计算函数，如果提供则使用它计算嵌入向量
        """
        # 使用SemanticSplitConfig而非基础SplitConfig
        super().__init__(config or SemanticSplitConfig())

        # 将config转换为正确的类型以便访问额外属性
        self.semantic_config = self.config if isinstance(self.config, SemanticSplitConfig) else SemanticSplitConfig()

        # 嵌入函数，用于计算语义相似度
        self._embedding_func = embedding_func

        # 缓存已计算的嵌入
        self._embeddings_cache = self.semantic_config.embeddings_cache

        self.logger.info(f"Initialized semantic splitter with threshold: {self.semantic_config.similarity_threshold}")

    def split(self, text: str, split_type: str = "semantic", metadata: Dict[str, Any] = None) -> List[Chunk]:
        """
        根据语义相似度将文本分割成块

        参数:
            text: 要分割的文本内容
            split_type: 分割类型 (semantic 或使用基类的方法)
            metadata: 额外的元数据，会添加到每个块中

        返回:
            List[Chunk]: 文本块列表
        """
        if not text:
            self.logger.warning("Empty text provided to semantic splitter")
            return []

        # 如果不是语义分块，则使用基类的方法
        if split_type.lower() != "semantic":
            return super().split(text, split_type, metadata)

        # 预处理文本
        text = self._preprocess_text(text)

        # 检测语言
        lang = detect_language(text)

        # 执行语义分块
        chunks = self._split_by_semantic_similarity(text, lang, metadata)

        # 过滤和后处理
        chunks = self._post_process_chunks(chunks)

        self.logger.info(f"Split text into {len(chunks)} chunks using semantic similarity")
        return chunks

    def _split_by_semantic_similarity(self, text: str, lang: str, metadata: Dict[str, Any] = None) -> List[Chunk]:
        """
        基于语义相似度分块

        参数:
            text: 要分割的文本
            lang: 文本语言
            metadata: 块元数据

        返回:
            List[Chunk]: 文本块列表
        """
        # 检查是否有嵌入函数
        if not self._embedding_func:
            self.logger.warning("No embedding function provided, falling back to paragraph splitting")
            return self._split_by_paragraph(text, metadata)

        # 将文本拆分为句子
        sentences = split_sentences(text, lang)
        if not sentences or len(sentences) <= 1:
            # 如果没有足够的句子，直接返回一个块
            if text:
                chunk_meta = self._create_chunk_metadata(text, 0, metadata)
                return [Chunk(text=text, index=0, metadata=chunk_meta)]
            return []

        # 过滤太短的句子
        sentences = [s for s in sentences if len(s.strip()) >= self.semantic_config.min_sentence_length]

        # 创建分块
        chunks = []
        current_chunk_sentences = []
        current_size = 0
        chunk_index = 0

        # 计算所有句子的嵌入（批处理）
        try:
            sentence_embeddings = self._get_embeddings_batch(sentences)
        except Exception as e:
            self.logger.error(f"Failed to compute embeddings: {str(e)}. Falling back to paragraph splitting.")
            return self._split_by_paragraph(text, metadata)

        # 动态分块
        for i, sentence in enumerate(sentences):
            sentence_size = len(sentence) if self.semantic_config.length_function == "character" else count_tokens(sentence)

            # 初始化分块
            if not current_chunk_sentences:
                current_chunk_sentences.append(sentence)
                current_size = sentence_size
                continue

            # 检查是否超过最大块大小
            if current_size + sentence_size > self.semantic_config.chunk_size:
                # 创建新块
                chunk_text = " ".join(current_chunk_sentences)
                chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
                chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))
                chunk_index += 1
                current_chunk_sentences = [sentence]
                current_size = sentence_size
                continue

            # 如果有足够的句子，检查语义相似度
            if len(current_chunk_sentences) >= 1 and i > 0:
                # 计算当前句子与前一个句子的相似度
                current_embedding = sentence_embeddings[i]
                prev_embedding = sentence_embeddings[i-1]

                similarity = self._compute_similarity(current_embedding, prev_embedding)

                # 如果相似度低于阈值，考虑创建新块
                if similarity < self.semantic_config.similarity_threshold:
                    # 但仅当当前块足够大且不会超过最大块大小时才切分
                    if (len(current_chunk_sentences) >= self.semantic_config.buffer_size and
                            current_size >= self.semantic_config.min_chunk_size):
                        chunk_text = " ".join(current_chunk_sentences)
                        chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
                        chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))
                        chunk_index += 1
                        current_chunk_sentences = [sentence]
                        current_size = sentence_size
                        continue

            # 继续当前块
            current_chunk_sentences.append(sentence)
            current_size += sentence_size

        # 添加最后一个块
        if current_chunk_sentences:
            chunk_text = " ".join(current_chunk_sentences)
            chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
            chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))

        return chunks

    def _get_embeddings_batch(self, texts: List[str]) -> List[List[float]]:
        """
        批量获取文本嵌入向量

        参数:
            texts: 文本列表

        返回:
            List[List[float]]: 嵌入向量列表
        """
        # 检查缓存
        if self.semantic_config.use_cached_embeddings:
            # 查找哪些文本需要计算嵌入
            texts_to_embed = []
            indices = []
            embeddings = [None] * len(texts)

            for i, text in enumerate(texts):
                if text in self._embeddings_cache:
                    embeddings[i] = self._embeddings_cache[text]
                else:
                    texts_to_embed.append(text)
                    indices.append(i)

            # 如果有未缓存的文本，计算它们的嵌入
            if texts_to_embed:
                computed_embeddings = self._embedding_func(texts_to_embed)
                for j, embedding in enumerate(computed_embeddings):
                    idx = indices[j]
                    embeddings[idx] = embedding
                    # 更新缓存
                    self._embeddings_cache[texts_to_embed[j]] = embedding

            return embeddings
        else:
            # 不使用缓存，直接计算全部嵌入
            return self._embedding_func(texts)

    def _compute_similarity(self, vec1: List[float], vec2: List[float]) -> float:
        """
        计算两个向量之间的余弦相似度

        参数:
            vec1: 第一个向量
            vec2: 第二个向量

        返回:
            float: 余弦相似度 (0-1)
        """
        # 转换为numpy数组以便计算
        vec1 = np.array(vec1)
        vec2 = np.array(vec2)

        # 计算余弦相似度
        dot_product = np.dot(vec1, vec2)
        norm_vec1 = np.linalg.norm(vec1)
        norm_vec2 = np.linalg.norm(vec2)

        # 避免除零错误
        if norm_vec1 == 0 or norm_vec2 == 0:
            return 0.0

        similarity = dot_product / (norm_vec1 * norm_vec2)

        # 确保结果在0-1范围内
        return max(0.0, min(1.0, similarity))

    def get_optimal_splits(self, text: str, max_chunks: int = 10) -> List[Chunk]:
        """
        获取文本的最优分块方案

        参数:
            text: 要分割的文本
            max_chunks: 最大块数

        返回:
            List[Chunk]: 最优的文本块列表
        """
        # 首先尝试语义分块
        semantic_chunks = self.split(text, "semantic")

        # 如果块数在可接受范围内，直接返回语义分块结果
        if len(semantic_chunks) <= max_chunks:
            return semantic_chunks

        # 否则尝试合并相似的块
        return self._merge_similar_chunks(semantic_chunks, max_chunks)

    def _merge_similar_chunks(self, chunks: List[Chunk], target_count: int) -> List[Chunk]:
        """
        合并相似的块，减少总块数

        参数:
            chunks: 原始块列表
            target_count: 目标块数

        返回:
            List[Chunk]: 合并后的块列表
        """
        if not chunks or len(chunks) <= target_count:
            return chunks

        # 获取所有块文本的嵌入
        try:
            chunk_texts = [chunk.text for chunk in chunks]
            chunk_embeddings = self._get_embeddings_batch(chunk_texts)

            # 计算块之间的相似度矩阵
            similarity_matrix = np.zeros((len(chunks), len(chunks)))
            for i in range(len(chunks)):
                for j in range(i+1, len(chunks)):
                    similarity = self._compute_similarity(
                        chunk_embeddings[i],
                        chunk_embeddings[j]
                    )
                    similarity_matrix[i][j] = similarity
                    similarity_matrix[j][i] = similarity

            # 合并相似度最高的块，直到达到目标数量
            while len(chunks) > target_count:
                # 找到相似度最高的一对块
                max_similarity = -1
                merge_i, merge_j = 0, 0

                for i in range(len(chunks)):
                    for j in range(i+1, len(chunks)):
                        if similarity_matrix[i][j] > max_similarity:
                            max_similarity = similarity_matrix[i][j]
                            merge_i, merge_j = i, j

                # 合并这两个块
                merged_text = chunks[merge_i].text + " " + chunks[merge_j].text
                merged_metadata = chunks[merge_i].metadata.copy() if chunks[merge_i].metadata else {}
                merged_chunk = Chunk(
                    text=merged_text,
                    index=chunks[merge_i].index,
                    metadata=merged_metadata
                )

                # 更新块列表和相似度矩阵
                new_chunks = [c for i, c in enumerate(chunks) if i != merge_i and i != merge_j]
                new_chunks.append(merged_chunk)

                # 计算新合并块与其他块的相似度
                merged_embedding = self._embedding_func([merged_text])[0]
                new_similarity_matrix = np.zeros((len(new_chunks), len(new_chunks)))

                # 复制剩余块之间的相似度
                remaining_indices = [i for i in range(len(chunks)) if i != merge_i and i != merge_j]
                for i, old_i in enumerate(remaining_indices):
                    for j, old_j in enumerate(remaining_indices[i+1:], i+1):
                        new_similarity_matrix[i][j] = similarity_matrix[old_i][old_j]
                        new_similarity_matrix[j][i] = similarity_matrix[old_j][old_i]

                # 计算合并块与其他块的相似度
                for i in range(len(new_chunks) - 1):
                    other_text = new_chunks[i].text
                    other_embedding = self._get_embeddings_batch([other_text])[0]
                    sim = self._compute_similarity(merged_embedding, other_embedding)
                    new_similarity_matrix[i][-1] = sim
                    new_similarity_matrix[-1][i] = sim

                # 更新变量
                chunks = new_chunks
                similarity_matrix = new_similarity_matrix

            # 重新编号
            for i, chunk in enumerate(chunks):
                chunk.index = i
                if chunk.metadata:
                    chunk.metadata["chunk_index"] = i

            return chunks

        except Exception as e:
            self.logger.warning(f"Error during chunk merging: {str(e)}. Returning original chunks.")
            # 如果合并失败，则简单地返回前target_count个块
            return chunks[:target_count]


# 便捷函数：使用语义分块
def split_text_semantic(
        text: str,
        embedding_func: Callable[[List[str]], List[List[float]]],
        chunk_size: int = 1000,
        chunk_overlap: int = 200,
        similarity_threshold: float = 0.75,
        metadata: Dict[str, Any] = None
) -> List[Dict[str, Any]]:
    """
    便捷函数: 使用语义分块方法分割文本

    参数:
        text: 要分割的文本
        embedding_func: 计算嵌入向量的函数
        chunk_size: 最大块大小
        chunk_overlap: 块重叠大小
        similarity_threshold: 语义相似度阈值
        metadata: 基础元数据

    返回:
        List[Dict[str, Any]]: 格式化后的文本块列表
    """
    config = SemanticSplitConfig(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
        similarity_threshold=similarity_threshold
    )

    splitter = SemanticSplitter(config, embedding_func)
    chunks = splitter.split(text, "semantic", metadata)

    # 将Chunk对象转换为字典
    return [
        {
            "text": chunk.text,
            "index": chunk.index,
            "metadata": chunk.metadata or {}
        }
        for chunk in chunks
    ]


# 示例嵌入函数 (使用随机向量，实际应用中替换为真实的嵌入函数)
def _dummy_embedding_func(texts: List[str]) -> List[List[float]]:
    """
    生成随机嵌入向量 (仅用于测试)

    参数:
        texts: 文本列表

    返回:
        List[List[float]]: 嵌入向量列表
    """
    dim = 384  # 假设的嵌入维度
    return [np.random.rand(dim).tolist() for _ in texts]