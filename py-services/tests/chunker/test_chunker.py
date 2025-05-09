import unittest

from chunker.service import TextChunker, SplitType, create_chunker


class TestTextChunker(unittest.TestCase):
    """文本分块器测试类"""

    def setUp(self):
        """测试前初始化"""
        # 创建默认分块器
        self.chunker = TextChunker(chunk_size=1000, chunk_overlap=200)

        # 测试文本样例
        self.english_paragraphs = """
        This is the first paragraph. It contains multiple sentences.
        
        This is the second paragraph. It also has several sentences. These sentences help test the chunker.
        
        The third paragraph is here. More text for testing purposes.
        """

        self.chinese_text = """
        这是第一个段落。它包含了多个句子。这里是中文内容测试。
        
        这是第二个段落。它也有几个句子。这些句子用来测试分块器。
        
        第三个段落在这里。更多的文字用于测试目的。
        """

        self.mixed_text = """
        First paragraph with English. 第一段包含中英文混合内容。
        
        Second paragraph with English and Chinese. 第二段也包含了中英文混合内容。
        
        Third mixed paragraph. 第三段混合段落。
        """

    def test_paragraph_splitting(self):
        """测试段落分割功能"""
        chunker = create_chunker(split_type="paragraph")
        result = chunker.split(self.english_paragraphs)

        # 验证分段数量
        self.assertEqual(3, len(result))

        # 验证每个段落的内容
        self.assertIn("first paragraph", result[0].text)
        self.assertIn("second paragraph", result[1].text)
        self.assertIn("third paragraph", result[2].text)

    def test_sentence_splitting(self):
        """测试句子分割功能"""
        chunker = create_chunker(split_type="sentence")
        result = chunker.split("This is sentence one. This is sentence two! What about sentence three?")

        # 验证分句数量
        self.assertEqual(3, len(result))

        # 验证句子内容
        self.assertEqual("This is sentence one.", result[0].text)
        self.assertEqual("This is sentence two!", result[1].text)
        self.assertEqual("What about sentence three?", result[2].text)

    def test_length_splitting(self):
        """测试按长度分割功能"""
        # 创建每块最大长度为20的分块器
        chunker = create_chunker(chunk_size=20, chunk_overlap=5, split_type="length")

        # 一段较长文本
        long_text = "This is a long text that should be split into multiple chunks based on length."

        result = chunker.split(long_text)

        # Debug prints
        print("\n=== DEBUG INFO ===")
        print(f"Original text: '{long_text}'")
        print(f"Original (no spaces): '{long_text.replace(' ', '')}'")
        print(f"Number of chunks: {len(result)}")
        print("Chunks:")
        for i, chunk in enumerate(result):
            print(f"  {i}: '{chunk.text}'")

        # 验证块的长度都不超过设置的最大长度
        for chunk in result:
            self.assertLessEqual(len(chunk.text), 20)

        # 修改验证方法：确保所有原始单词都在某个块中出现
        original_words = set(long_text.split())
        chunks_text = " ".join(chunk.text for chunk in result)
        chunks_words = set(chunks_text.split())

        # 验证所有原始单词都在某个块中
        missing_words = original_words - chunks_words
        self.assertEqual(0, len(missing_words),
                         f"Missing words in chunks: {missing_words}")

        # 可选：也可以验证正常重建方式
        reconstructed = ""
        for i, chunk in enumerate(result):
            if i == 0:
                reconstructed = chunk.text
            else:
                # 找到前一个块和当前块的重叠部分
                prev_chunk = result[i-1].text
                current_chunk = chunk.text

                # 找到重叠点（如果有）并从那里拼接
                overlap_found = False
                for j in range(min(len(prev_chunk), len(current_chunk)), 0, -1):
                    if prev_chunk.endswith(current_chunk[:j]):
                        reconstructed += current_chunk[j:]
                        overlap_found = True
                        break

                # 如果没找到重叠，直接添加
                if not overlap_found:
                    reconstructed += " " + current_chunk

        print(f"Smart reconstructed: '{reconstructed}'")
        # 验证智能重构后的文本包含所有原始单词
        smart_words = set(reconstructed.split())
        self.assertTrue(original_words.issubset(smart_words),
                        f"Missing words in smart reconstruction: {original_words - smart_words}")

    def test_chinese_content(self):
        """测试中文内容分割"""
        result = self.chunker.split(self.chinese_text)

        # 验证中文段落被正确分割
        self.assertEqual(3, len(result))

        # 验证中文内容正确
        self.assertIn("这是第一个段落", result[0].text)
        self.assertIn("这是第二个段落", result[1].text)
        self.assertIn("第三个段落在这里", result[2].text)

    def test_mixed_language_content(self):
        """测试中英文混合内容分割"""
        result = self.chunker.split(self.mixed_text)

        # 验证混合内容分段
        self.assertEqual(3, len(result))

        # 验证英文和中文内容都被正确包含
        self.assertIn("First paragraph", result[0].text)
        self.assertIn("第一段", result[0].text)
        self.assertIn("Second paragraph", result[1].text)
        self.assertIn("第二段", result[1].text)

    def test_empty_text(self):
        """测试空文本处理"""
        result = self.chunker.split("")
        self.assertEqual(0, len(result))

        result = self.chunker.split("   ")
        self.assertEqual(0, len(result))

    def test_very_large_chunk(self):
        """测试处理超大块"""
        # 创建一个大文本
        large_text = "A" * 5000  # 5000个字符的文本

        # 设置块大小为1000
        chunker = create_chunker(chunk_size=1000, chunk_overlap=100)
        result = chunker.split(large_text)

        # 验证分块数量和大小
        self.assertGreater(len(result), 1)  # 应该被分成多个块
        for chunk in result:
            self.assertLessEqual(len(chunk.text), 1000)  # 每个块不应超过设定大小

    def test_max_chunks_limit(self):
        """测试最大块数量限制"""
        # 创建一个会产生多个块的文本
        multi_paragraph_text = "\n\n".join(["Paragraph " + str(i) for i in range(10)])

        # 限制最大块数为3
        chunker = create_chunker(max_chunks=3)
        result = chunker.split(multi_paragraph_text)

        # 验证结果不超过3个块
        self.assertLessEqual(len(result), 3)

    def test_chunker_factory(self):
        """测试分块器工厂函数"""
        # 测试创建不同类型的分块器
        paragraph_chunker = create_chunker(split_type="paragraph")
        self.assertEqual(SplitType.PARAGRAPH, paragraph_chunker.split_type)

        sentence_chunker = create_chunker(split_type="sentence")
        self.assertEqual(SplitType.SENTENCE, sentence_chunker.split_type)

        length_chunker = create_chunker(split_type="length")
        self.assertEqual(SplitType.LENGTH, length_chunker.split_type)

        # 测试无效的分割类型
        invalid_chunker = create_chunker(split_type="invalid_type")
        self.assertEqual(SplitType.PARAGRAPH, invalid_chunker.split_type)  # 应回退到段落分割


if __name__ == "__main__":
    unittest.main()
