import logging
import os
from typing import BinaryIO

import markdown
from bs4 import BeautifulSoup
from app.parsers.base import BaseParser


class MarkdownParser(BaseParser):
    """Markdown文档解析器，负责解析Markdown文件并提取文本内容"""

    def __init__(self, logger=None):
        """
        初始化Markdown解析器

        Args:
            logger: 可选的日志记录器，如果不提供则创建一个新的
        """
        super().__init__(logger)
        self.logger = logger or logging.getLogger(__name__)
        self.markdown_extensions = [
            'tables',          # 表格支持
            'fenced_code',     # 围栏式代码块
            'codehilite',      # 代码高亮
            'nl2br',           # 换行符转换为<br>标签
            'toc',             # 目录支持
        ]

    def parse(self, file_path: str) -> str:
        """
        解析Markdown文件并返回文本内容

        Args:
            file_path: Markdown文件路径

        Returns:
            str: 提取的文本内容

        Raises:
            FileNotFoundError: 文件不存在
            ValueError: 文件格式不支持或解析错误
        """
        # 验证文件
        self.validate_file(file_path)

        try:
            # 打开并读取Markdown文件
            with open(file_path, 'r', encoding='utf-8') as file:
                return self.parse_reader(file, os.path.basename(file_path))
        except UnicodeDecodeError:
            # 如果UTF-8解码失败，尝试其他编码
            self.logger.warning(f"UTF-8 decoding failed for {file_path}, trying with other encodings")
            encodings = ['latin-1', 'cp1252', 'iso-8859-1']
            for encoding in encodings:
                try:
                    with open(file_path, 'r', encoding=encoding) as file:
                        return self.parse_reader(file, os.path.basename(file_path))
                except UnicodeDecodeError:
                    continue

            self.logger.error(f"Failed to decode file {file_path} with all attempted encodings")
            raise ValueError(f"Could not decode file with any supported encoding")
        except Exception as e:
            self.logger.error(f"Error parsing Markdown file {file_path}: {str(e)}")
            raise ValueError(f"Markdown parsing error: {str(e)}")

    def parse_reader(self, reader: BinaryIO, filename: str) -> str:
        """
        从文件读取器解析Markdown文档

        Args:
            reader: 文件读取器对象
            filename: 文件名（用于错误报告和日志）

        Returns:
            str: 提取的文本内容

        Raises:
            ValueError: 解析错误
        """
        try:
            # 如果输入是BinaryIO，先转换为字符串
            if isinstance(reader, BinaryIO):
                content = reader.read()
                if isinstance(content, bytes):
                    content = content.decode('utf-8', errors='replace')
            else:
                # 假设reader是TextIOBase
                content = reader.read()

            self.logger.info(f"Processing Markdown document '{filename}'")

            # 将Markdown转换为HTML
            html = markdown.markdown(content, extensions=self.markdown_extensions)

            # 使用BeautifulSoup从HTML中提取纯文本
            soup = BeautifulSoup(html, 'html.parser')

            # 提取文本前的预处理
            # 保留标题的结构
            for heading in soup.find_all(['h1', 'h2', 'h3', 'h4', 'h5', 'h6']):
                # 在标题前后添加换行符
                heading.insert_before('\n\n')
                heading.append('\n')

            # 保证列表项的格式
            for li in soup.find_all('li'):
                li.insert_before('• ')

            # 确保段落间有足够的空行
            for p in soup.find_all('p'):
                p.insert_after('\n\n')

            # 处理代码块，添加前后标记
            for code in soup.find_all('pre'):
                code.insert_before('\n```\n')
                code.append('\n```\n')

            # 提取纯文本
            text = soup.get_text()

            # 预处理文本（删除多余空白等）
            processed_text = self.preprocess_text(text)

            self.logger.info(f"Successfully extracted text from Markdown '{filename}': {len(processed_text)} characters")
            return processed_text

        except Exception as e:
            self.logger.error(f"Error parsing Markdown from reader: {str(e)}")
            raise ValueError(f"Markdown parsing error: {str(e)}")

    def extract_title(self, text: str, filename: str) -> str:
        """
        从Markdown文件中提取标题

        Args:
            text: 文档文本内容
            filename: 文件名，作为备选标题

        Returns:
            str: 提取的标题
        """
        if not text:
            return os.path.splitext(filename)[0]

        # 查找Markdown标题格式（# 标题）
        lines = text.split('\n')
        for line in lines:
            line = line.strip()
            # 匹配H1标题
            if line.startswith('# '):
                title = line.lstrip('# ').strip()
                if title:
                    return title

        # 使用基类方法查找首行文本作为标题
        return super().extract_title(text, filename)

    def get_metadata(self, file_path: str) -> dict:
        """
        提取Markdown文件的元数据

        Args:
            file_path: Markdown文件路径

        Returns:
            dict: 文档元数据
        """
        # 获取基本文件元数据
        base_metadata = super().get_metadata(file_path)

        try:
            with open(file_path, 'r', encoding='utf-8') as file:
                content = file.read(4096)  # 只读取前面一部分内容来寻找元数据

            # 尝试提取YAML前置元数据（如果存在）
            # YAML前置元数据通常位于文件开头，由三个连字符 --- 包围
            yaml_metadata = {}
            if content.startswith('---'):
                end_pos = content.find('---', 3)
                if end_pos > 0:
                    # 这里仅检测存在性，不做完整解析
                    # 实际使用时可以添加YAML解析
                    yaml_metadata = {"has_frontmatter": True}

            # 合并元数据
            metadata = {**base_metadata, **yaml_metadata}

            # 尝试提取标题
            title = self.extract_title(content, os.path.basename(file_path))
            if title:
                metadata["title"] = title

            return metadata

        except Exception as e:
            self.logger.warning(f"Failed to extract Markdown metadata from {file_path}: {str(e)}")
            return base_metadata

    def supports_extension(self, extension: str) -> bool:
        """
        检查是否支持该文件扩展名

        Args:
            extension: 文件扩展名（不含点号）

        Returns:
            bool: 如果支持则返回True，否则False
        """
        return extension.lower() in ['md', 'markdown', 'mdown', 'mkd']