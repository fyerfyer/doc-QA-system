import os
import tempfile
import unittest
from unittest.mock import patch, mock_open

from parser.service import DocumentParser, create_parser


class TestDocumentParser(unittest.TestCase):
    """文档解析器测试类"""

    def setUp(self):
        """测试前初始化"""
        self.parser = DocumentParser()

        # 创建临时文件目录
        self.temp_dir = tempfile.mkdtemp()

        # 准备测试内容
        self.text_content = "这是一个测试文本文件"
        self.md_content = "# 标题\n\n这是一个 **Markdown** 文件"

    def tearDown(self):
        """测试后清理临时文件"""
        for file in os.listdir(self.temp_dir):
            os.unlink(os.path.join(self.temp_dir, file))
        os.rmdir(self.temp_dir)

    def test_parse_text_file(self):
        """测试解析文本文件"""
        # 创建临时文本文件
        file_path = os.path.join(self.temp_dir, "test.txt")
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(self.text_content)

        # 解析文件
        result = self.parser.parse(file_path)

        # 验证结果
        self.assertEqual(result, self.text_content)
        
    def test_parse_markdown_file(self):
        """测试解析Markdown文件"""
        # 创建临时Markdown文件
        file_path = os.path.join(self.temp_dir, "test.md")
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(self.md_content)

        # 解析文件
        result = self.parser.parse(file_path)

        # 规范化换行符后比较 (将 \r\n 替换为 \n)
        normalized_result = result.replace('\r\n', '\n')

        # 验证结果
        self.assertEqual(normalized_result, self.md_content)

    @patch('parser.service.fitz.open')
    def test_parse_pdf_file(self, mock_fitz_open):
        """测试解析PDF文件"""
        # 模拟PDF文档对象和页面
        mock_doc = unittest.mock.MagicMock()
        mock_page = unittest.mock.MagicMock()
        mock_page.get_text.return_value = "这是PDF文件内容"
        mock_doc.__len__.return_value = 1
        mock_doc.__getitem__.return_value = mock_page
        mock_fitz_open.return_value = mock_doc

        # 模拟读取PDF文件
        with patch("builtins.open", mock_open(read_data=b"fake pdf content")):
            result = self.parser.parse_reader(open("fake.pdf", "rb"), "fake.pdf")

        # 验证结果
        self.assertEqual(result, "这是PDF文件内容")
        mock_fitz_open.assert_called_once()

    def test_unsupported_format(self):
        """测试不支持的文件格式"""
        # 创建临时文件
        file_path = os.path.join(self.temp_dir, "test.docx")
        with open(file_path, "w") as f:
            f.write("some content")

        # 验证抛出错误
        with self.assertRaises(ValueError) as context:
            self.parser.parse(file_path)

        self.assertIn("Unsupported file type", str(context.exception))

    def test_file_not_found(self):
        """测试文件不存在的情况"""
        non_existent_file = os.path.join(self.temp_dir, "not_exists.txt")

        with self.assertRaises(FileNotFoundError) as context:
            self.parser.parse(non_existent_file)

        self.assertIn("File not found", str(context.exception))

    def test_create_parser(self):
        """测试解析器创建函数"""
        parser = create_parser()
        self.assertIsInstance(parser, DocumentParser)

    def test_parse_reader_with_bytes(self):
        """测试使用字节流解析文本内容"""
        content = self.text_content.encode('utf-8')
        result = self.parser._parse_text(content)
        self.assertEqual(result, self.text_content)

    def test_parse_reader_error_handling(self):
        """测试解析时的异常处理"""
        with patch('parser.service.DocumentParser._parse_text') as mock_parse:
            print("Setting up mock to raise exception")
            mock_parse.side_effect = Exception("Parsing error")

            print("Creating mock reader")
            mock_reader = unittest.mock.MagicMock(read=lambda: b"content")

            print("Before calling parse_reader")
            try:
                self.parser.parse_reader(mock_reader, "test.txt")
                print("parse_reader did not raise exception")
            except Exception as e:
                print(f"Caught exception: {type(e).__name__}: {e}")

            with self.assertRaises(ValueError) as context:
                self.parser.parse_reader(mock_reader, "test.txt")

            self.assertIn("Failed to parse text", str(context.exception))

if __name__ == '__main__':
    unittest.main()