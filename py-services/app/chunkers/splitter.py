import logging
from enum import Enum
from typing import List, Dict, Any, Optional
import re
from dataclasses import dataclass

from app.chunkers.utils import (
    normalize_text, split_sentences, detect_language,
    find_best_split_point, calculate_overlap, get_chunk_title,
    estimate_chunk_quality, count_tokens
)

# 初始化日志
logger = logging.getLogger(__name__)

class SplitType(str, Enum):
    """分块策略类型"""
    PARAGRAPH = "paragraph"  # 按段落分块
    SENTENCE = "sentence"    # 按句子分块
    LENGTH = "length"        # 按固定长度分块
    SEMANTIC = "semantic"    # 按语义分块 (需要单独实现)


@dataclass
class SplitConfig:
    """分块器配置"""
    chunk_size: int = 1000         # 默认块大小 (字符数或token数)
    chunk_overlap: int = 200        # 块之间重叠的大小
    min_chunk_size: int = 50        # 最小块大小
    min_chunk_length_to_embed: int = 10  # 最小可嵌入块长度
    keep_separator: bool = False    # 是否保留分隔符
    strip_whitespace: bool = True   # 是否去除多余空白
    length_function: str = "character"  # 长度计算函数："character" 或 "token"
    filter_metadata: Dict[str, Any] = None  # 过滤元数据


@dataclass
class Chunk:
    """文本块"""
    text: str                      # 块内容
    index: int                     # 块索引
    metadata: Dict[str, Any] = None  # 块元数据


class TextSplitter:
    """文本分块器，实现各种文本分块策略"""

    def __init__(self, config: Optional[SplitConfig] = None):
        """
        初始化文本分块器

        参数:
            config: 分块配置，如果不提供则使用默认配置
        """
        self.config = config or SplitConfig()
        self.logger = logger

    def split(self, text: str, split_type: str = "paragraph", metadata: Dict[str, Any] = None) -> List[Chunk]:
        """
        将文本分割成内容块

        参数:
            text: 要分割的文本内容
            split_type: 分割类型 (paragraph, sentence, length)
            metadata: 额外的元数据，会添加到每个块中

        返回:
            List[Chunk]: 文本块列表
        """
        if not text:
            self.logger.warning("Empty text provided to splitter")
            return []

        # 预处理文本
        text = self._preprocess_text(text)

        # 检测语言
        lang = detect_language(text)

        # 根据分割类型选择不同的分割策略
        split_type = split_type.lower()

        chunks = []
        if split_type == SplitType.PARAGRAPH or split_type == "paragraph":
            chunks = self._split_by_paragraph(text, metadata)
        elif split_type == SplitType.SENTENCE or split_type == "sentence":
            chunks = self._split_by_sentence(text, lang, metadata)
        elif split_type == SplitType.LENGTH or split_type == "length":
            chunks = self._split_by_length(text, metadata)
        elif split_type == SplitType.SEMANTIC or split_type == "semantic":
            # 语义分块需要依赖嵌入模型，目前暂不实现
            self.logger.warning("Semantic splitting not implemented, falling back to paragraph splitting")
            chunks = self._split_by_paragraph(text, metadata)
        else:
            self.logger.warning(f"Unknown split type '{split_type}', falling back to paragraph splitting")
            chunks = self._split_by_paragraph(text, metadata)

        # 过滤和后处理块
        chunks = self._post_process_chunks(chunks)

        self.logger.info(f"Split text into {len(chunks)} chunks using '{split_type}' strategy")
        return chunks

    def _preprocess_text(self, text: str) -> str:
        """
        预处理文本，规范化格式

        参数:
            text: 原始文本

        返回:
            str: 预处理后的文本
        """
        # 使用工具函数规范化文本
        text = normalize_text(text)
        return text

    def _split_by_paragraph(self, text: str, metadata: Dict[str, Any] = None) -> List[Chunk]:
        """
        按段落分割文本

        参数:
            text: 要分割的文本
            metadata: 块元数据

        返回:
            List[Chunk]: 文本块列表
        """
        # 使用空行作为段落分隔符
        paragraphs = re.split(r'\n\s*\n', text)

        # 过滤空段落
        paragraphs = [p.strip() for p in paragraphs if p.strip()]

        if not paragraphs:
            return []

        # 直接为每个段落创建一个块，不再尝试合并
        chunks = []
        for i, para in enumerate(paragraphs):
            chunk_meta = self._create_chunk_metadata(para, i, metadata)
            chunks.append(Chunk(text=para, index=i, metadata=chunk_meta))

        return chunks

    def _split_by_sentence(self, text: str, lang: str = None, metadata: Dict[str, Any] = None) -> List[Chunk]:
        """
        按句子分割文本

        参数:
            text: 要分割的文本
            lang: 文本语言
            metadata: 块元数据

        返回:
            List[Chunk]: 文本块列表
        """
        # 先分割成句子
        sentences = split_sentences(text, lang)

        if not sentences:
            return []

        chunks = []
        current_chunk = []
        current_size = 0
        chunk_index = 0

        # 动态调整块大小和重叠
        chunk_size, chunk_overlap = self.config.chunk_size, self.config.chunk_overlap

        # 合并句子到块
        for sentence in sentences:
            if not sentence.strip():
                continue

            sentence_size = len(sentence) if self.config.length_function == "character" else count_tokens(sentence)

            # 如果当前句子本身就超过块大小，需要单独处理
            if sentence_size > chunk_size:
                # 如果当前块非空，先完成当前块
                if current_chunk:
                    chunk_text = " ".join(current_chunk)
                    chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
                    chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))
                    chunk_index += 1
                    current_chunk = []
                    current_size = 0

                # 处理长句子
                word_chunks = self._split_by_length_helper(sentence, chunk_size)
                for wc in word_chunks:
                    if len(wc) >= self.config.min_chunk_size:
                        chunk_meta = self._create_chunk_metadata(wc, chunk_index, metadata)
                        chunks.append(Chunk(text=wc, index=chunk_index, metadata=chunk_meta))
                        chunk_index += 1
                continue

            # 检查添加当前句子是否会超过块大小
            if current_size + sentence_size <= chunk_size or not current_chunk:
                current_chunk.append(sentence)
                current_size += sentence_size
            else:
                # 完成当前块并开始新块
                chunk_text = " ".join(current_chunk)
                chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
                chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))
                chunk_index += 1

                # 决定是否保留重叠内容
                if chunk_overlap > 0 and len(current_chunk) > 1:
                    # 找到足够的句子作为重叠部分
                    overlap_size = 0
                    overlap_sentences = []
                    for s in reversed(current_chunk):
                        s_size = len(s) if self.config.length_function == "character" else count_tokens(s)
                        if overlap_size + s_size <= chunk_overlap:
                            overlap_sentences.insert(0, s)
                            overlap_size += s_size
                        else:
                            break

                    current_chunk = overlap_sentences + [sentence]
                    current_size = sum(len(s) if self.config.length_function == "character" else count_tokens(s)
                                       for s in current_chunk)
                else:
                    current_chunk = [sentence]
                    current_size = sentence_size

        # 添加最后一个块
        if current_chunk:
            chunk_text = " ".join(current_chunk)
            chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
            chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))

        return chunks

    def _split_by_length(self, text: str, metadata: Dict[str, Any] = None) -> List[Chunk]:
        """
        按固定长度分割文本

        参数:
            text: 要分割的文本
            metadata: 块元数据

        返回:
            List[Chunk]: 文本块列表
        """
        if not text:
            return []

        chunks = []
        chunk_size = self.config.chunk_size
        chunk_overlap = self.config.chunk_overlap
        chunk_index = 0

        text_length = len(text)

        # 如果文本小于块大小，直接返回一个块
        if text_length <= chunk_size:
            chunk_meta = self._create_chunk_metadata(text, 0, metadata)
            return [Chunk(text=text, index=0, metadata=chunk_meta)]

        # 分块
        start = 0
        while start < text_length:
            # 计算结束位置
            end = min(start + chunk_size, text_length)

            if end < text_length:
                # 查找最佳分割点
                best_end = find_best_split_point(text, end)

                # 关键修改：确保不超过最大长度
                if best_end > start + chunk_size:
                    best_end = start + chunk_size

                end = best_end

            # 提取当前块
            chunk_text = text[start:end]
            if len(chunk_text) >= self.config.min_chunk_size:
                chunk_meta = self._create_chunk_metadata(chunk_text, chunk_index, metadata)
                chunks.append(Chunk(text=chunk_text, index=chunk_index, metadata=chunk_meta))
                chunk_index += 1

            # 更新开始位置，考虑重叠
            start = end - chunk_overlap if end < text_length and chunk_overlap < end - start else end

        return chunks

    def _split_large_paragraph(self, paragraph: str, max_size: int) -> List[str]:
        """
        拆分大段落为更小的块

        参数:
            paragraph: 大段落文本
            max_size: 最大块大小

        返回:
            List[str]: 分割后的小段落列表
        """
        # 尝试按句子分割
        lang = detect_language(paragraph)
        sentences = split_sentences(paragraph, lang)

        if not sentences:
            return [paragraph]

        # 按句子组合，尽量不拆分句子
        chunks = []
        current_chunk = []
        current_size = 0

        for sentence in sentences:
            sentence_size = len(sentence) if self.config.length_function == "character" else count_tokens(sentence)

            # 如果句子本身超过最大大小，按长度拆分
            if sentence_size > max_size:
                # 如果当前块非空，先完成当前块
                if current_chunk:
                    chunks.append(" ".join(current_chunk))
                    current_chunk = []
                    current_size = 0

                # 按长度拆分句子
                word_chunks = self._split_by_length_helper(sentence, max_size)
                chunks.extend(word_chunks)
                continue

            # 判断是否需要结束当前块
            if current_size + sentence_size <= max_size or not current_chunk:
                current_chunk.append(sentence)
                current_size += sentence_size
            else:
                chunks.append(" ".join(current_chunk))
                current_chunk = [sentence]
                current_size = sentence_size

        # 添加最后一个块
        if current_chunk:
            chunks.append(" ".join(current_chunk))

        return chunks

    def _split_by_length_helper(self, text: str, max_size: int) -> List[str]:
        """
        辅助函数：将文本按固定长度拆分

        参数:
            text: 要拆分的文本
            max_size: 最大块大小

        返回:
            List[str]: 拆分后的文本块列表
        """
        # 简单地按字符或单词拆分
        if self.config.length_function == "token":
            # 按单词拆分
            words = text.split()
            chunks = []
            current_chunk = []
            current_size = 0

            for word in words:
                word_size = count_tokens(word)

                if current_size + word_size <= max_size or not current_chunk:
                    current_chunk.append(word)
                    current_size += word_size
                else:
                    chunks.append(" ".join(current_chunk))
                    current_chunk = [word]
                    current_size = word_size

            if current_chunk:
                chunks.append(" ".join(current_chunk))

            return chunks
        else:
            # 按字符拆分
            chunks = []
            for i in range(0, len(text), max_size):
                end = i + max_size
                if end < len(text):
                    # 尝试在单词边界处分割
                    while end > i and not text[end].isspace():
                        end -= 1
                    if end == i:  # 如果没有找到合适的单词边界
                        end = i + max_size
                chunk = text[i:end].strip()
                if chunk:
                    chunks.append(chunk)
            return chunks

    def _create_chunk_metadata(self, chunk_text: str, index: int, base_metadata: Dict[str, Any] = None) -> Dict[str, Any]:
        """
        为文本块创建元数据

        参数:
            chunk_text: 块文本
            index: 块索引
            base_metadata: 基础元数据

        返回:
            Dict[str, Any]: 块元数据
        """
        metadata = {}

        # 复制基础元数据
        if base_metadata:
            metadata.update(base_metadata)

        # 添加块特有元数据
        metadata.update({
            "chunk_index": index,
            "chunk_size": len(chunk_text) if self.config.length_function == "character" else count_tokens(chunk_text),
            "chunk_type": "text"
        })

        # 评估块质量
        quality = estimate_chunk_quality(chunk_text)
        metadata["quality"] = round(quality, 2)

        return metadata

    def _post_process_chunks(self, chunks: List[Chunk]) -> List[Chunk]:
        """
        处理和过滤分块结果

        参数:
            chunks: 原始文本块列表

        返回:
            List[Chunk]: 处理后的文本块列表
        """
        # 过滤过小或质量太差的块
        filtered_chunks = []
        for chunk in chunks:
            # 跳过太小的块
            if len(chunk.text) < self.config.min_chunk_length_to_embed:
                self.logger.debug(f"Skipping chunk {chunk.index}: too small ({len(chunk.text)} chars)")
                continue

            # 跳过质量太差的块
            if chunk.metadata and chunk.metadata.get("quality", 1.0) < 0.2:
                self.logger.debug(f"Skipping chunk {chunk.index}: low quality ({chunk.metadata.get('quality', 0)})")
                continue

            # 清理块文本
            if self.config.strip_whitespace:
                chunk.text = chunk.text.strip()

            filtered_chunks.append(chunk)

        # 重新编号索引
        for i, chunk in enumerate(filtered_chunks):
            chunk.index = i
            if chunk.metadata:
                chunk.metadata["chunk_index"] = i

        return filtered_chunks


def split_text(
        text: str,
        chunk_size: int = 1000,
        chunk_overlap: int = 200,
        split_type: str = "paragraph",
        metadata: Dict[str, Any] = None
) -> List[Dict[str, Any]]:
    """
    便捷函数: 分割文本为块，并返回字典格式

    参数:
        text: 要分割的文本
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        split_type: 分割类型 (paragraph, sentence, length)
        metadata: 基础元数据

    返回:
        List[Dict[str, Any]]: 格式化后的文本块列表
    """
    config = SplitConfig(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap
    )
    splitter = TextSplitter(config)
    chunks = splitter.split(text, split_type, metadata)

    # 将Chunk对象转换为字典
    return [
        {
            "text": chunk.text,
            "index": chunk.index,
            "metadata": chunk.metadata or {}
        }
        for chunk in chunks
    ]


def get_splitter(config: Optional[Dict[str, Any]] = None) -> TextSplitter:
    """
    获取配置好的分块器实例

    参数:
        config: 分块器配置参数

    返回:
        TextSplitter: 分块器实例
    """
    split_config = None
    if config:
        # 将字典转换为SplitConfig
        split_config = SplitConfig(
            chunk_size=config.get("chunk_size", 1000),
            chunk_overlap=config.get("chunk_overlap", 200),
            min_chunk_size=config.get("min_chunk_size", 50),
            min_chunk_length_to_embed=config.get("min_chunk_length_to_embed", 10),
            keep_separator=config.get("keep_separator", False),
            strip_whitespace=config.get("strip_whitespace", True),
            length_function=config.get("length_function", "character"),
            filter_metadata=config.get("filter_metadata", None)
        )

    return TextSplitter(split_config)