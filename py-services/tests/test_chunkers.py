import unittest
import os
import sys

from app.chunkers.splitter import TextSplitter, SplitConfig, Chunk, split_text
from app.chunkers.semantic_splitter import SemanticSplitter, SemanticSplitConfig, split_text_semantic, _dummy_embedding_func
from app.chunkers.utils import (
    normalize_text, detect_language, split_sentences, count_tokens,
    find_best_split_point, calculate_overlap, estimate_chunk_quality
)


# 测试用的样本文本
SAMPLE_TEXT_EN = """
This is a sample English text for testing the chunking functionality.
It contains multiple sentences and paragraphs.

This is the second paragraph with some more sentences.
We need to make sure the chunker handles paragraphs correctly.

And here's a third paragraph. The chunker should separate this from the previous content.
"""

SAMPLE_TEXT_ZH = """
这是一段用于测试分块功能的中文示例文本。
它包含多个句子和段落。

这是第二段，有一些额外的句子。
我们需要确保分块器能够正确处理段落。

这是第三段。分块器应该将其与之前的内容分开。
"""

LONG_TEXT = " ".join(["This is sentence number " + str(i) + "." for i in range(100)])

class TestTextSplitter(unittest.TestCase):
    """测试基本的文本分块功能"""

    def setUp(self):
        """设置测试环境"""
        self.default_splitter = TextSplitter()
        self.custom_splitter = TextSplitter(
            SplitConfig(
                chunk_size=50,
                chunk_overlap=10,
                min_chunk_size=20
            )
        )

    def test_split_by_paragraph(self):
        """测试按段落分块"""
        chunks = self.default_splitter.split(SAMPLE_TEXT_EN, "paragraph")

        # 应该有3个段落
        self.assertEqual(len(chunks), 3)

        # 检查第一个块的内容
        self.assertIn("sample English text", chunks[0].text)

        # 检查元数据
        self.assertEqual(chunks[0].index, 0)
        self.assertIn("quality", chunks[0].metadata)

    def test_split_by_sentence(self):
        """测试按句子分块"""
        chunks = self.default_splitter.split(SAMPLE_TEXT_EN, "sentence")

        # 确保已分块
        self.assertGreater(len(chunks), 0)

        # 检查是否每个块都不超过最大块大小
        for chunk in chunks:
            self.assertLessEqual(len(chunk.text), self.default_splitter.config.chunk_size)

    def test_split_by_length(self):
        """测试按长度分块"""
        chunks = self.custom_splitter.split(LONG_TEXT, "length")

        # 确保文本已分块
        self.assertGreater(len(chunks), 1)

        # 检查块大小
        for chunk in chunks:
            self.assertLessEqual(len(chunk.text), self.custom_splitter.config.chunk_size)

    def test_chinese_text(self):
        """测试中文文本分块"""
        chunks = self.default_splitter.split(SAMPLE_TEXT_ZH, "paragraph")

        # 应该有3个段落
        self.assertEqual(len(chunks), 3)

        # 检查第一个块的内容
        self.assertIn("中文示例文本", chunks[0].text)

    def test_empty_text(self):
        """测试空文本"""
        chunks = self.default_splitter.split("", "paragraph")
        self.assertEqual(len(chunks), 0)

    def test_convenience_function(self):
        """测试便捷函数"""
        result = split_text(
            text=SAMPLE_TEXT_EN,
            chunk_size=200,
            chunk_overlap=50,
            split_type="paragraph"
        )

        # 确保结果是字典列表
        self.assertTrue(isinstance(result, list))
        self.assertTrue(all(isinstance(item, dict) for item in result))
        self.assertTrue(all("text" in item for item in result))


class TestSemanticSplitter(unittest.TestCase):
    """测试语义分块功能"""

    def setUp(self):
        """设置测试环境"""
        # 使用模拟的嵌入函数
        self.semantic_splitter = SemanticSplitter(
            SemanticSplitConfig(
                chunk_size=100,
                chunk_overlap=20,
                similarity_threshold=0.7
            ),
            embedding_func=_dummy_embedding_func
        )

    def test_semantic_split(self):
        """测试语义分块"""
        chunks = self.semantic_splitter.split(SAMPLE_TEXT_EN, "semantic")

        # 确保已分块
        self.assertGreater(len(chunks), 0)

        # 检查元数据
        for chunk in chunks:
            self.assertIn("chunk_index", chunk.metadata)

    def test_semantic_convenience_function(self):
        """测试语义分块便捷函数"""
        result = split_text_semantic(
            text=SAMPLE_TEXT_EN,
            embedding_func=_dummy_embedding_func,
            chunk_size=200,
            similarity_threshold=0.8
        )

        # 确保结果是字典列表
        self.assertTrue(isinstance(result, list))
        self.assertTrue(all(isinstance(item, dict) for item in result))

    def test_optimal_splits(self):
        """测试获取最优分块"""
        chunks = self.semantic_splitter.get_optimal_splits(LONG_TEXT, max_chunks=5)

        # 检查结果不超过最大块数
        self.assertLessEqual(len(chunks), 5)

    def test_fallback_to_paragraph(self):
        """测试在没有嵌入函数时回退到段落分块"""
        # 创建一个没有嵌入函数的分块器
        no_embedder_splitter = SemanticSplitter(SemanticSplitConfig())
        chunks = no_embedder_splitter.split(SAMPLE_TEXT_EN, "semantic")

        # 应该仍然分块成功（回退到段落分块）
        self.assertGreater(len(chunks), 0)


class TestChunkerUtils(unittest.TestCase):
    """测试分块工具函数"""

    def test_normalize_text(self):
        """测试文本规范化"""
        messy_text = "This  has  extra   spaces\r\nand\rdifferent\nnewlines"
        cleaned = normalize_text(messy_text)

        # 应该没有连续的空格和统一的换行符
        self.assertNotIn("  ", cleaned)
        self.assertNotIn("\r", cleaned)

    def test_detect_language(self):
        """测试语言检测"""
        self.assertEqual(detect_language(SAMPLE_TEXT_EN)[:2], "en")
        self.assertEqual(detect_language(SAMPLE_TEXT_ZH)[:2], "zh")

    def test_split_sentences(self):
        """测试句子分割"""
        sentences = split_sentences(SAMPLE_TEXT_EN)

        # 应该有多个句子
        self.assertGreater(len(sentences), 3)

        # 每个句子应该以句号结束或是一个完整句
        for sentence in sentences:
            if sentence.strip():  # 忽略空字符串
                self.assertTrue(
                    sentence.strip().endswith('.') or
                    sentence.strip().endswith('?') or
                    sentence.strip().endswith('!')
                )

    def test_count_tokens(self):
        """测试token计数"""
        # 英文按空格分词
        self.assertEqual(count_tokens("This is a test."), 4)

        # 中文按字符计数（乘以一个因子）
        zh_count = count_tokens("这是测试", "zh")
        self.assertGreater(zh_count, 0)

    def test_calculate_overlap(self):
        """测试重叠计算"""
        chunk_size, overlap = calculate_overlap(100, 30, 200)

        # 应该返回有效的分块参数
        self.assertEqual(chunk_size, 100)
        self.assertEqual(overlap, 30)

        # 文本比块小的情况
        chunk_size, overlap = calculate_overlap(100, 30, 50)
        self.assertEqual(chunk_size, 50)
        self.assertEqual(overlap, 0)

    def test_chunk_quality(self):
        """测试块质量评估"""
        good_text = "This is a well-formed paragraph with proper sentences and structure."
        bad_text = "!! @# $% ^^^ &&&"

        good_score = estimate_chunk_quality(good_text)
        bad_score = estimate_chunk_quality(bad_text)

        # 好的文本应该得分更高
        self.assertGreater(good_score, bad_score)

        # 分数应该在0-1范围内
        self.assertTrue(0 <= good_score <= 1)
        self.assertTrue(0 <= bad_score <= 1)

    def test_find_best_split_point(self):
        """测试寻找最佳分割点"""
        text = "First sentence. Second sentence. Third sentence."

        # 在句号附近寻找分割点
        split_point = find_best_split_point(text, 15, window=10)

        # 应该在第一个句号之后
        self.assertEqual(text[split_point-1], ".")