import os
import tempfile
import unittest
from unittest.mock import MagicMock, patch
from io import StringIO

from app.parsers.base import BaseParser
from app.parsers.pdf_parser import PDFParser
from app.parsers.markdown_parser import MarkdownParser
from app.parsers.text_parser import TextParser
from app.parsers.factory import create_parser, detect_content_type

class TestParsers(unittest.TestCase):
    """解析器单元测试"""

    def setUp(self):
        """测试前创建临时文件"""
        # 创建临时目录用于存储测试文件
        self.temp_dir = tempfile.TemporaryDirectory()

        # 创建测试文件
        self.text_file = os.path.join(self.temp_dir.name, "sample.txt")
        with open(self.text_file, "w", encoding="utf-8") as f:
            f.write("这是一个测试文本文件。\n包含多行内容。\n这是第三行。")

        self.markdown_file = os.path.join(self.temp_dir.name, "sample.md")
        with open(self.markdown_file, "w", encoding="utf-8") as f:
            f.write("# 测试标题\n\n这是**Markdown**格式的文件。\n\n## 子标题\n\n- 列表项1\n- 列表项2\n")

        # 注意：无法轻松创建实际的PDF文件，将使用模拟对象测试PDF解析器

    def tearDown(self):
        """测试结束后清理临时文件"""
        self.temp_dir.cleanup()

    def test_text_parser(self):
        """测试纯文本解析器"""
        parser = TextParser()

        # 测试解析文件
        content = parser.parse(self.text_file)
        self.assertIsInstance(content, str)
        self.assertIn("这是一个测试文本文件", content)
        self.assertIn("这是第三行", content)

        # 测试获取元数据
        metadata = parser.get_metadata(self.text_file)
        self.assertIsInstance(metadata, dict)
        self.assertEqual(metadata["extension"], "txt")
        self.assertIn("file_size", metadata)
        self.assertIn("line_count", metadata)
        self.assertEqual(metadata["line_count"], 3)

        # 测试支持的扩展名
        self.assertTrue(parser.supports_extension("txt"))
        self.assertTrue(parser.supports_extension("log"))
        self.assertFalse(parser.supports_extension("pdf"))

    def test_markdown_parser(self):
        """测试Markdown解析器"""
        parser = MarkdownParser()

        # 测试解析文件
        content = parser.parse(self.markdown_file)
        self.assertIsInstance(content, str)
        self.assertIn("测试标题", content)
        self.assertIn("Markdown", content)
        self.assertIn("子标题", content)
        self.assertIn("列表项", content)

        # 测试提取标题
        title = parser.extract_title(content, "sample.md")
        self.assertEqual(title, "测试标题")

        # 测试获取元数据
        metadata = parser.get_metadata(self.markdown_file)
        self.assertIsInstance(metadata, dict)
        self.assertEqual(metadata["extension"], "md")
        self.assertIn("title", metadata)
        self.assertEqual(metadata["title"], "测试标题")

        # 测试支持的扩展名
        self.assertTrue(parser.supports_extension("md"))
        self.assertTrue(parser.supports_extension("markdown"))
        self.assertFalse(parser.supports_extension("txt"))

    def test_pdf_parser(self):
        """测试PDF解析器（使用模拟对象）"""
        mock_pdf_file = os.path.join(self.temp_dir.name, "mock.pdf")
        with open(mock_pdf_file, "wb") as f:
            f.write(b"%PDF-mock")

        parser = PDFParser()

        # Patch for parse method
        with patch('pypdf.PdfReader') as mock_reader_parse:
            mock_instance_parse = MagicMock()
            mock_reader_parse.return_value = mock_instance_parse

            mock_instance_parse.pages = [MagicMock(), MagicMock()]
            mock_instance_parse.pages[0].extract_text.return_value = "PDF第一页内容"
            mock_instance_parse.pages[1].extract_text.return_value = "PDF第二页内容"

            content = parser.parse(mock_pdf_file)
            self.assertIsInstance(content, str)
            self.assertIn("PDF第一页内容", content)
            self.assertIn("PDF第二页内容", content)

        with patch('pypdf.PdfReader') as mock_reader_meta:
            mock_instance_meta = MagicMock()
            mock_reader_meta.return_value = mock_instance_meta

            mock_instance_meta.pages = [MagicMock(), MagicMock()]
            mock_instance_meta.metadata = {"/Title": "PDF测试文档", "/Author": "测试作者"}

            metadata = parser.get_metadata(mock_pdf_file)
            self.assertIsInstance(metadata, dict)
            self.assertEqual(metadata["extension"], "pdf")
            self.assertEqual(metadata["title"], "PDF测试文档")
            self.assertEqual(metadata["author"], "测试作者")
            self.assertEqual(metadata["page_count"], 2)

        def test_parser_reader_methods(self):
            """测试解析器从读取器解析内容的方法"""
            # 测试文本解析器的parse_reader方法
            text_parser = TextParser()
            text_content = "这是要解析的文本内容"

            # 使用StringIO模拟文件对象
            text_reader = StringIO(text_content)
            parsed_text = text_parser.parse_reader(text_reader, "test.txt")
            self.assertEqual(parsed_text, text_content)

            # 测试Markdown解析器的parse_reader方法
            md_parser = MarkdownParser()
            md_content = "# 标题\n\n内容段落"
            md_reader = StringIO(md_content)
            parsed_md = md_parser.parse_reader(md_reader, "test.md")
            self.assertIn("标题", parsed_md)
            self.assertIn("内容段落", parsed_md)

        def test_preprocess_text(self):
            """测试文本预处理方法"""
            parser = TextParser()

            # 测试多余空格的处理
            text = "这是  一段   有多余空格   的文本"
            processed = parser.preprocess_text(text)
            self.assertEqual(processed, "这是 一段 有多余空格 的文本")

            # 测试多余换行的处理
            text = "第一行\n\n\n\n第二行\n\n\n第三行"
            processed = parser.preprocess_text(text)
            self.assertEqual(processed, "第一行\n\n第二行\n\n第三行")

            # 测试控制字符的处理
            text = "包含控制字符\x00\x01\x02的文本"
            processed = parser.preprocess_text(text)
            self.assertEqual(processed, "包含控制字符的文本")

        def test_factory_create_parser(self):
            """测试解析器工厂创建解析器"""
            # 测试根据文件扩展名创建解析器
            parser = create_parser(file_path=self.text_file)
            self.assertIsInstance(parser, TextParser)

            parser = create_parser(file_path=self.markdown_file)
            self.assertIsInstance(parser, MarkdownParser)

            # 测试根据MIME类型创建解析器
            parser = create_parser(mime_type="text/plain")
            self.assertIsInstance(parser, TextParser)

            parser = create_parser(mime_type="text/markdown")
            self.assertIsInstance(parser, MarkdownParser)

            parser = create_parser(mime_type="application/pdf")
            self.assertIsInstance(parser, PDFParser)

            # 测试无法识别的类型
            with self.assertRaises(ValueError):
                parser = create_parser(file_path="unknown.xyz")

            with self.assertRaises(ValueError):
                parser = create_parser(mime_type="application/unknown")

        def test_detect_content_type(self):
            """测试内容类型检测"""
            self.assertEqual(detect_content_type(self.text_file), "text/plain")
            self.assertEqual(detect_content_type(self.markdown_file), "text/markdown")

            # 测试未知扩展名
            unknown_file = os.path.join(self.temp_dir.name, "unknown.xyz")
            with open(unknown_file, "w") as f:
                f.write("unknown content")

            # 应该返回通用类型
            self.assertEqual(detect_content_type(unknown_file), "application/octet-stream")