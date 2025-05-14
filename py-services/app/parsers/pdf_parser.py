import os
import logging
from typing import BinaryIO, Dict, Any

import pypdf
from pypdf.errors import PdfReadError
from app.parsers.base import BaseParser


class PDFParser(BaseParser):
    """PDF文档解析器，负责解析PDF文件并提取文本内容"""

    def __init__(self, logger=None):
        """
        初始化PDF解析器

        Args:
            logger: 可选的日志记录器，如果不提供则创建一个新的
        """
        super().__init__(logger)
        self.logger = logger or logging.getLogger(__name__)

    def parse(self, file_path: str) -> str:
        """
        解析PDF文件并返回文本内容

        Args:
            file_path: PDF文件路径

        Returns:
            str: 提取的文本内容

        Raises:
            FileNotFoundError: 文件不存在
            ValueError: 文件格式不支持或解析错误
        """
        # 验证文件
        self.validate_file(file_path)

        try:
            # 打开PDF文件
            with open(file_path, 'rb') as file:
                return self.parse_reader(file, os.path.basename(file_path))
        except PdfReadError as e:
            self.logger.error(f"Failed to read PDF file {file_path}: {str(e)}")
            raise ValueError(f"Invalid or corrupted PDF file: {str(e)}")
        except Exception as e:
            self.logger.error(f"Error while parsing PDF file {file_path}: {str(e)}")
            raise ValueError(f"PDF parsing error: {str(e)}")

    def parse_reader(self, reader: BinaryIO, filename: str) -> str:
        """
        从文件读取器解析PDF文档

        Args:
            reader: PDF文件读取器对象
            filename: 文件名（用于错误报告和日志）

        Returns:
            str: 提取的文本内容

        Raises:
            ValueError: 不支持的文件类型或解析错误
        """
        try:
            # 创建PDF读取器
            pdf_reader = pypdf.PdfReader(reader)

            # 获取页面数量
            num_pages = len(pdf_reader.pages)
            self.logger.info(f"Processing PDF document '{filename}' with {num_pages} pages")

            # 提取所有页面的文本
            text_content = []
            for page_num in range(num_pages):
                try:
                    page = pdf_reader.pages[page_num]
                    page_text = page.extract_text()

                    # 如果页面提取出的文本为空，尝试使用备用方法
                    if not page_text:
                        self.logger.warning(f"Could not extract text from page {page_num + 1} using primary method, trying fallback")
                        # 如果有OCR功能，可以在这里实现
                        # TODO: 实现OCR功能，当常规文本提取失败时使用

                    text_content.append(page_text or f"[Page {page_num + 1} - No extractable text]")

                except Exception as e:
                    self.logger.warning(f"Error extracting text from page {page_num + 1}: {str(e)}")
                    text_content.append(f"[Page {page_num + 1} - Error: {str(e)}]")

            # 合并所有页面的文本，使用双换行符分隔
            combined_text = "\n\n".join(text_content)

            # 预处理文本（删除多余空白等）
            processed_text = self.preprocess_text(combined_text)

            self.logger.info(f"Successfully extracted text from PDF '{filename}': {len(processed_text)} characters")
            return processed_text

        except PdfReadError as e:
            self.logger.error(f"Failed to read PDF from reader: {str(e)}")
            raise ValueError(f"Invalid or corrupted PDF file: {str(e)}")
        except Exception as e:
            self.logger.error(f"Error parsing PDF from reader: {str(e)}")
            raise ValueError(f"PDF parsing error: {str(e)}")

    def get_metadata(self, file_path: str) -> Dict[str, Any]:
        """
        提取PDF文件的元数据

        Args:
            file_path: PDF文件路径

        Returns:
            Dict: PDF元数据，如标题、作者、创建日期等

        Raises:
            FileNotFoundError: 文件不存在
            ValueError: 文件格式不支持或解析错误
        """
        # 先获取基本文件信息
        base_metadata = super().get_metadata(file_path)

        try:
            with open(file_path, 'rb') as file:
                pdf_reader = pypdf.PdfReader(file)

                # 提取PDF特定的元数据
                pdf_info = pdf_reader.metadata
                if pdf_info:
                    # 合并基本元数据和PDF特定元数据
                    metadata = {
                        **base_metadata,
                        "page_count": len(pdf_reader.pages),
                    }

                    # 添加PDF文档信息
                    for key in pdf_info:
                        # 清理键名 (移除前导斜杠，这是PDF元数据的标准格式)
                        clean_key = key
                        if key.startswith('/'):
                            clean_key = key[1:]

                        # 添加到元数据中
                        metadata[clean_key.lower()] = pdf_info[key]

                    return metadata

                # 如果没有提取到PDF元数据，只返回基本元数据加页数
                return {
                    **base_metadata,
                    "page_count": len(pdf_reader.pages)
                }

        except Exception as e:
            self.logger.warning(f"Failed to extract PDF metadata from {file_path}: {str(e)}")
            return base_metadata  # 失败时返回基本元数据

    def extract_title(self, text: str, filename: str) -> str:
        """
        从PDF文件中提取标题

        Args:
            text: 文档文本内容
            filename: 文件名，作为备选标题

        Returns:
            str: 提取的标题
        """
        # 尝试从PDF元数据中提取标题
        try:
            if os.path.exists(filename):
                with open(filename, 'rb') as file:
                    pdf_reader = pypdf.PdfReader(file)
                    if pdf_reader.metadata and '/Title' in pdf_reader.metadata:
                        title = pdf_reader.metadata['/Title']
                        if title and isinstance(title, str) and len(title) > 0:
                            return title
        except Exception as e:
            self.logger.warning(f"Failed to extract title from PDF metadata: {str(e)}")

        # 如果从元数据中无法提取标题，使用基类的方法
        return super().extract_title(text, filename)

    def supports_extension(self, extension: str) -> bool:
        """
        检查是否支持该文件扩展名

        Args:
            extension: 文件扩展名（不含点号）

        Returns:
            bool: 如果支持则返回True，否则返回False
        """
        return extension.lower() in ['pdf']