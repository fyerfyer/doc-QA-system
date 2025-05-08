import os
import logging
from typing import Union, BinaryIO, IO
import fitz  # PyMuPDF for PDF parsing

class DocumentParser:
    """文档解析服务，支持PDF、Markdown和纯文本格式"""

    def __init__(self):
        """初始化解析器"""
        self.logger = logging.getLogger(__name__)

    def parse(self, file_path: str) -> str:
        """
        从文件路径解析文档

        Args:
            file_path: 文件路径

        Returns:
            提取的文本内容

        Raises:
            ValueError: 不支持的文件类型或解析失败
            FileNotFoundError: 文件不存在
        """
        if not os.path.exists(file_path):
            self.logger.error(f"File not found: {file_path}")
            raise FileNotFoundError(f"File not found: {file_path}")

        # 读取文件内容
        with open(file_path, 'rb') as f:
            return self.parse_reader(f, file_path)

    def parse_reader(self, reader: Union[BinaryIO, IO[bytes]], filename: str) -> str:
        """
        从文件对象解析文档

        Args:
            reader: 文件对象，支持read()方法
            filename: 文件名，用于确定文件类型

        Returns:
            提取的文本内容

        Raises:
            ValueError: 不支持的文件类型或解析失败
        """
        # 读取所有内容
        content = reader.read()

        # 获取文件扩展名
        _, ext = os.path.splitext(filename.lower())

        # 根据文件类型选择解析方法
        if ext == '.pdf':
            return self._parse_pdf(content)
        elif ext in ['.md', '.markdown']:
            return self._parse_markdown(content)
        elif ext == '.txt' or ext == '':  # 支持无扩展名文件作为文本处理
            return self._parse_text(content)
        else:
            self.logger.error(f"Unsupported file type: {ext}")
            raise ValueError(f"Unsupported file type: {ext}")

    def _parse_pdf(self, content: bytes) -> str:
        """
        解析PDF文件内容

        Args:
            content: PDF文件的二进制内容

        Returns:
            提取的文本内容
        """
        try:
            # 使用PyMuPDF解析PDF
            doc = fitz.open(stream=content, filetype="pdf")
            text = ""
            for page_num in range(len(doc)):
                page = doc[page_num]
                text += page.get_text()
            return text
        except Exception as e:
            self.logger.error(f"Failed to parse PDF: {e}")
            raise ValueError(f"Failed to parse PDF: {e}")

    def _parse_markdown(self, content: bytes) -> str:
        """
        解析Markdown文件内容

        Args:
            content: Markdown文件的二进制内容

        Returns:
            提取的文本内容
        """
        try:
            # 转换为UTF-8字符串
            md_text = content.decode('utf-8', errors='replace')
            # 直接返回原始文本，保留格式信息
            return md_text
        except Exception as e:
            self.logger.error(f"Failed to parse Markdown: {e}")
            raise ValueError(f"Failed to parse Markdown: {e}")

    def _parse_text(self, content: bytes) -> str:
        """
        解析纯文本文件内容

        Args:
            content: 文本文件的二进制内容

        Returns:
            提取的文本内容
        """
        try:
            # 转换为UTF-8字符串
            return content.decode('utf-8', errors='replace')
        except Exception as e:
            self.logger.error(f"Failed to parse text: {e}")
            raise ValueError(f"Failed to parse text: {e}")


def create_parser() -> DocumentParser:
    """
    创建文档解析器实例

    Returns:
        DocumentParser实例
    """
    return DocumentParser()

# 默认解析器实例，可直接使用
parser = DocumentParser()