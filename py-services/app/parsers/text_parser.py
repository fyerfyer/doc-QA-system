import os
import logging
from typing import BinaryIO, Dict, Any
import chardet
import io

from app.parsers.base import BaseParser


class TextParser(BaseParser):
    """纯文本文档解析器，负责解析文本文件并提取内容"""

    def __init__(self, logger=None):
        """
        初始化文本解析器

        Args:
            logger: 可选的日志记录器，如果不提供则创建一个新的
        """
        super().__init__(logger)
        self.logger = logger or logging.getLogger(__name__)

    def parse(self, file_path: str) -> str:
        """
        解析文本文件并返回内容

        Args:
            file_path: 文本文件路径

        Returns:
            str: 文本内容

        Raises:
            FileNotFoundError: 文件不存在
            ValueError: 文件格式不支持或解析错误
        """
        # 验证文件
        self.validate_file(file_path)

        try:
            # 获取文件内容（支持MinIO和本地文件系统）
            file_content = self.get_file_content(file_path)

            try:
                # 读取文件内容
                content = file_content.read()

                # 检测编码
                if isinstance(content, bytes):
                    encoding = chardet.detect(content)['encoding'] or 'utf-8'
                    content = content.decode(encoding, errors='replace')

                # 处理内容
                return self.parse_reader(io.StringIO(content), os.path.basename(file_path))
            finally:
                # 确保关闭文件
                if hasattr(file_content, 'close'):
                    file_content.close()
                if hasattr(file_content, 'release_conn'):
                    file_content.release_conn()
        except UnicodeDecodeError as e:
            self.logger.error(f"Failed to decode text file {file_path}: {str(e)}")
            # 尝试使用不同的编码
            try:
                # 重新获取文件内容
                file_content = self.get_file_content(file_path)
                content = file_content.read()
                if isinstance(content, bytes):
                    content = content.decode('utf-8-sig', errors='replace')
                return self.parse_reader(io.StringIO(content), os.path.basename(file_path))
            except Exception as e2:
                raise ValueError(f"Could not decode file with any supported encoding: {str(e2)}")
            finally:
                if 'file_content' in locals() and hasattr(file_content, 'close'):
                    file_content.close()
        except Exception as e:
            self.logger.error(f"Error parsing text file {file_path}: {str(e)}")
            raise ValueError(f"Text file parsing error: {str(e)}")

    def parse_reader(self, reader: BinaryIO, filename: str) -> str:
        """
        从文件读取器解析文本文档

        Args:
            reader: 文件读取器对象
            filename: 文件名（用于错误报告和日志）

        Returns:
            str: 提取的文本内容

        Raises:
            ValueError: 解析错误
        """
        try:
            # 如果输入是BinaryIO，先将二进制内容转换为字符串
            if hasattr(reader, 'mode') and 'b' in reader.mode:
                content = reader.read()
                if isinstance(content, bytes):
                    # 尝试检测编码
                    encoding = chardet.detect(content)['encoding'] or 'utf-8'
                    content = content.decode(encoding, errors='replace')
            else:
                # 否则直接读取
                content = reader.read()

            self.logger.info(f"Processing text document '{filename}': {len(content)} characters")

            # 预处理文本
            processed_text = self.preprocess_text(content)

            return processed_text

        except Exception as e:
            self.logger.error(f"Error parsing text from reader: {str(e)}")
            raise ValueError(f"Text parsing error: {str(e)}")

    def get_metadata(self, file_path: str) -> Dict[str, Any]:
        """
        提取文本文件的元数据

        Args:
            file_path: 文本文件路径

        Returns:
            Dict: 文件元数据，如大小、修改时间等
        """
        # 使用基类方法获取基本文件信息
        metadata = super().get_metadata(file_path)

        # 尝试获取文本文件的行数和字符数
        try:
            # 获取文件内容
            file_content = self.get_file_content(file_path)

            try:
                # 读取文件内容
                content_bytes = file_content.read()

                if isinstance(content_bytes, bytes):
                    encoding = chardet.detect(content_bytes)['encoding'] or 'utf-8'
                    content = content_bytes.decode(encoding, errors='replace')
                else:
                    content = content_bytes

                lines = content.count('\n') + 1
                chars = len(content)

                # 添加文本特有的元数据
                metadata.update({
                    "line_count": lines,
                    "char_count": chars,
                    "word_count": len(content.split())
                })
            finally:
                # 确保关闭文件
                if hasattr(file_content, 'close'):
                    file_content.close()
                if hasattr(file_content, 'release_conn'):
                    file_content.release_conn()

        except Exception as e:
            self.logger.warning(f"Failed to extract detailed text metadata from {file_path}: {str(e)}")

        return metadata

    def supports_extension(self, extension: str) -> bool:
        """
        检查是否支持该文件扩展名

        Args:
            extension: 文件扩展名（不含点号）

        Returns:
            bool: 如果支持则返回True，否则False
        """
        # 支持的文本文件扩展名列表
        return extension.lower() in [
            'txt', 'text', 'log', 'csv', 'tsv',
            'md', 'markdown',  # 基本文本格式
            'json', 'xml', 'yaml', 'yml',  # 数据格式
            'html', 'htm', 'css', 'js',  # Web格式
            'py', 'go', 'java', 'c', 'cpp', 'cs', 'rs',  # 代码格式
            'sh', 'bat', 'ps1',  # 脚本格式
        ]