import re
import logging
from typing import List, Optional
from enum import Enum

class SplitType(str, Enum):
    """文本分割类型枚举"""
    PARAGRAPH = "paragraph"  # 按段落分割
    SENTENCE = "sentence"    # 按句子分割
    LENGTH = "length"        # 按固定长度分割
    SEMANTIC = "semantic"    # 按语义分割（未实现）

class TextChunk:
    """文本块类，表示分割后的文本片段"""

    def __init__(self, text: str, index: int, metadata: Optional[dict] = None):
        """
        初始化文本块

        Args:
            text: 文本内容
            index: 块索引，表示在原文中的位置
            metadata: 可选的元数据
        """
        self.text = text
        self.index = index
        self.metadata = metadata or {}

    def __repr__(self) -> str:
        """字符串表示"""
        return f"TextChunk(index={self.index}, len={len(self.text)}, text='{self.text[:30]}...')"

class TextChunker:
    """文本分块器，负责将长文本分割成小块"""

    def __init__(self,
                 chunk_size: int = 1000,
                 chunk_overlap: int = 200,
                 split_type: SplitType = SplitType.PARAGRAPH,
                 max_chunks: int = 0):
        """
        初始化分块器

        Args:
            chunk_size: 单个块的最大大小（字符数）
            chunk_overlap: 相邻块之间的重叠大小（字符数）
            split_type: 分割类型
            max_chunks: 最大分块数量（0表示不限制）
        """
        self.chunk_size = chunk_size
        self.chunk_overlap = chunk_overlap
        self.split_type = split_type
        self.max_chunks = max_chunks
        self.logger = logging.getLogger(__name__)

    def split(self, text: str) -> List[TextChunk]:
        """
        将文本分割成块

        Args:
            text: 需要分割的文本

        Returns:
            分割后的文本块列表
        """
        if not text:
            return []

        # 预处理文本（规范化换行符、删除多余空白等）
        text = self._preprocess_text(text)

        # 根据分割类型选择不同的分割策略
        if self.split_type == SplitType.PARAGRAPH:
            chunks = self._split_by_paragraph(text)
            # 处理过大的段落
            chunks = self._handle_large_chunks(chunks)
        elif self.split_type == SplitType.SENTENCE:
            chunks = self._split_by_sentence(text)
            chunks = self._handle_large_chunks(chunks)
        elif self.split_type == SplitType.LENGTH:
            chunks = self._split_by_length(text)
        else:
            self.logger.warning(f"Unsupported split type: {self.split_type}, falling back to paragraph splitting")
            chunks = self._split_by_paragraph(text)
            chunks = self._handle_large_chunks(chunks)

        # 过滤空段落和进行最终清理
        chunks = self._filter_and_clean_chunks(chunks)

        # 应用最大分块数量限制
        if self.max_chunks > 0 and len(chunks) > self.max_chunks:
            chunks = chunks[:self.max_chunks]

        # 构建TextChunk对象
        result = []
        for i, chunk in enumerate(chunks):
            result.append(TextChunk(text=chunk, index=i))

        return result

    def _preprocess_text(self, text: str) -> str:
        """
        预处理文本，规范化格式

        Args:
            text: 原始文本

        Returns:
            预处理后的文本
        """
        # 统一换行符
        text = text.replace('\r\n', '\n').replace('\r', '\n')

        # 移除连续的空白行，最多保留两个换行符
        while '\n\n\n' in text:
            text = text.replace('\n\n\n', '\n\n')

        return text.strip()

    def _split_by_paragraph(self, text: str) -> List[str]:
        """
        按段落分割文本

        Args:
            text: 预处理后的文本

        Returns:
            段落列表
        """
        # 使用正则表达式匹配段落分隔
        # 空行、标题（# 开头）和列表项（* 开头）都算作段落分隔
        paragraphs = re.split(r'(?m)^\s*$|^#{1,6}\s|^\*\s', text)

        # 过滤空段落
        paragraphs = [p.strip() for p in paragraphs if p.strip()]

        # 如果段落识别结果不理想（太少或没有），回退到简单的换行符分割
        if len(paragraphs) <= 1:
            paragraphs = [p.strip() for p in text.split('\n\n') if p.strip()]

            # 如果还是没有足够段落，则按单行拆分
            if len(paragraphs) <= 1 and len(text) > self.chunk_size:
                paragraphs = [p.strip() for p in text.split('\n') if p.strip()]

        return paragraphs

    def _split_by_sentence(self, text: str) -> List[str]:
        """
        按句子分割文本

        Args:
            text: 预处理后的文本

        Returns:
            句子列表
        """
        # 改进句子分隔符识别，增强对中文文本的支持
        # 支持更多标点符号：英文句号、问号、感叹号，以及中文句号、问号、感叹号等
        delimiters = r'(?<=[.!?。！？；;])'
        sentences = re.split(delimiters, text)

        # 处理可能存在的空白句子并去除首尾空白
        sentences = [s.strip() for s in sentences if s.strip()]

        return sentences

    def _split_by_length(self, text: str) -> List[str]:
        """
        按固定长度分割文本

        Args:
            text: 预处理后的文本

        Returns:
            固定长度的文本块列表
        """
        if not text:
            return []

        if len(text) <= self.chunk_size:
            return [text]

        chunks = []
        start = 0

        while start < len(text):
            # 确定当前块的结束位置
            end = min(start + self.chunk_size, len(text))

            # 如果不是最后一个块且不在文本末尾，找更好的断点
            if end < len(text):
                # 尝试在句子结束处断开
                sentence_end = self._find_sentence_end(text, start, end)
                if sentence_end > start:
                    end = sentence_end
                else:
                    # 尝试在段落结束处断开
                    para_end = text.rfind('\n', start, end)
                    if para_end > start:
                        end = para_end + 1  # 包含换行符
                    else:
                        # 尝试在单词边界断开
                        word_end = text.rfind(' ', start, end)
                        if word_end > start:
                            end = word_end + 1  # 包含空格

            # 添加当前块
            current_chunk = text[start:end].strip()
            if current_chunk:
                chunks.append(current_chunk)

            # 计算下一个块的起始位置，考虑重叠
            if self.chunk_overlap >= end - start:
                # 如果重叠区域大于块大小，避免原地踏步
                start = end
            else:
                # 否则正常应用重叠
                start = end - self.chunk_overlap

            # 如果没有前进，强制前进以避免无限循环
            if start == end:
                break

        return chunks

    def _find_sentence_end(self, text: str, start: int, end: int) -> int:
        """
        在指定范围内查找句子结束的位置

        Args:
            text: 完整文本
            start: 起始位置
            end: 结束位置

        Returns:
            找到的句子结束位置，如果没找到则返回-1
        """
        # 定义可能的句子结束符
        sentence_enders = ['.', '!', '?', '。', '！', '？', '；', ';']

        # 从后向前查找，优先使用较后的句子结束位置
        for i in range(end - 1, start, -1):
            if i < len(text) and text[i] in sentence_enders:
                # 找到了句子结束符，返回下一个位置（即句子结束后的位置）
                return i + 1

        return -1

    def _find_word_boundary(self, text: str, start: int, end: int) -> int:
        """
        寻找合适的单词边界位置

        Args:
            text: 完整文本
            start: 起始位置
            end: 结束位置

        Returns:
            找到的单词边界位置
        """
        # 从末尾向前查找空格或标点
        min_pos = start + max(1, self.chunk_size // 2)  # 至少要前进一半长度
        for i in range(end - 1, min_pos, -1):
            if text[i].isspace() or text[i] in ',.;:!?，。；：！？':
                return i + 1

        # 找不到合适的边界，使用原始截断点
        return end

    def _handle_large_chunks(self, chunks: List[str]) -> List[str]:
        """
        处理过长的文本块，将其进一步拆分

        Args:
            chunks: 文本块列表

        Returns:
            处理后的文本块列表
        """
        result = []

        for chunk in chunks:
            # 如果块长度超过最大值，进行进一步拆分
            if len(chunk) > self.chunk_size:
                # 根据内容选择合适的拆分方式
                if chunk.count('\n') > 3:
                    # 如果有多个换行符，按换行符拆分
                    sub_chunks = [p.strip() for p in chunk.split('\n') if p.strip()]
                    sub_chunks = self._merge_small_chunks(sub_chunks)
                    result.extend(sub_chunks)
                elif self._contains_chinese(chunk):
                    # 如果包含中文，按中文句子拆分
                    sub_chunks = self._split_chinese_sentences(chunk)
                    sub_chunks = self._merge_small_chunks(sub_chunks)
                    result.extend(sub_chunks)
                else:
                    # 其他情况按固定长度拆分
                    sub_chunks = self._split_by_length(chunk)
                    result.extend(sub_chunks)
            else:
                result.append(chunk)

        return result

    def _split_chinese_sentences(self, text: str) -> List[str]:
        """
        专门处理中文句子的分割

        Args:
            text: 需要分割的中文文本

        Returns:
            分割后的中文句子列表
        """
        # 中文句子分隔符
        pattern = r'([。！？；.!?;]+)'
        parts = re.split(pattern, text)

        # 将分隔符与前面的文本组合
        sentences = []
        for i in range(0, len(parts), 2):
            if i + 1 < len(parts):
                sentences.append(parts[i] + parts[i+1])
            else:
                sentences.append(parts[i])

        # 过滤空句子
        return [s.strip() for s in sentences if s.strip()]

    def _merge_small_chunks(self, chunks: List[str]) -> List[str]:
        """
        合并过小的文本块，避免生成太多小块

        Args:
            chunks: 文本块列表

        Returns:
            合并后的文本块列表
        """
        if len(chunks) <= 1:
            return chunks

        # 过小块的阈值，低于此值的块会被考虑合并
        small_threshold = self.chunk_size // 5

        result = []
        current = ""
        current_size = 0

        for chunk in chunks:
            chunk_size = len(chunk)

            # 如果当前块加上新块不超过chunk_size，则合并
            if current_size + chunk_size <= self.chunk_size:
                if current and not current.endswith('\n'):
                    current += ' '  # 添加空格分隔
                current += chunk
                current_size += chunk_size
            else:
                # 保存当前块并开始新块
                if current:
                    result.append(current)
                current = chunk
                current_size = chunk_size

            # 如果当前块已经接近目标大小，添加到结果中
            if current_size >= self.chunk_size * 0.8 and current_size > small_threshold:
                result.append(current)
                current = ""
                current_size = 0

        # 添加最后一个块
        if current:
            result.append(current)

        return result

    def _filter_and_clean_chunks(self, chunks: List[str]) -> List[str]:
        """
        过滤空块并清理块内容

        Args:
            chunks: 文本块列表

        Returns:
            清理后的文本块列表
        """
        return [chunk.strip() for chunk in chunks if chunk.strip()]

    def _contains_chinese(self, text: str) -> bool:
        """
        检查文本是否包含中文字符

        Args:
            text: 需要检查的文本

        Returns:
            是否包含中文字符
        """
        # Unicode汉字范围
        for char in text:
            if '\u4e00' <= char <= '\u9fff':
                return True
        return False


def create_chunker(chunk_size: int = 1000, chunk_overlap: int = 200,
                   split_type: str = "paragraph", max_chunks: int = 0) -> TextChunker:
    """
    创建文本分块器实例

    Args:
        chunk_size: 块大小
        chunk_overlap: 块重叠大小
        split_type: 分割类型
        max_chunks: 最大块数量

    Returns:
        TextChunker实例
    """
    try:
        split_enum = SplitType(split_type.lower())
    except ValueError:
        # 无效的分割类型，使用默认值
        logging.warning(f"Invalid split type: {split_type}, using paragraph instead")
        split_enum = SplitType.PARAGRAPH

    return TextChunker(chunk_size=chunk_size,
                       chunk_overlap=chunk_overlap,
                       split_type=split_enum,
                       max_chunks=max_chunks)

# 创建默认分块器
chunker = TextChunker()