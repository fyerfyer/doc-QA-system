import os
import logging
from abc import ABC, abstractmethod
from typing import Dict, Any, BinaryIO, Union, Optional
from pathlib import Path

from app.utils.minio_client import get_minio_client

class BaseParser(ABC):
    """文档解析器的基类，提供通用接口和功能"""

    def __init__(self, logger=None):
        """
        初始化解析器

        Args:
            logger: 可选的日志记录器，如果不提供则创建一个新的
        """
        self.logger = logger or logging.getLogger(__name__)
        self.minio_client = get_minio_client()

    @abstractmethod
    def parse(self, file_path: str) -> str:
        """
        解析文档文件并返回文本内容

        Args:
            file_path: 文档文件路径 (可以是本地路径或MinIO中的路径)

        Returns:
            str: 提取的文本内容

        Raises:
            FileNotFoundError: 文件不存在
            ValueError: 文件格式不支持或解析错误
        """
        pass

    @abstractmethod
    def parse_reader(self, reader: BinaryIO, filename: str) -> str:
        """
        从文件读取器解析文档

        Args:
            reader: 文件读取器对象
            filename: 文件名（用于确定文件类型）

        Returns:
            str: 提取的文本内容

        Raises:
            ValueError: 不支持的文件类型或解析错误
        """
        pass

    def get_metadata(self, file_path: str) -> Dict[str, Any]:
        """
        提取文档元数据

        Args:
            file_path: 文档文件路径

        Returns:
            Dict: 文档元数据，如标题、作者、页数等
        """
        try:
            # 尝试从MinIO获取元数据
            try:
                minio_metadata = self.minio_client.get_object_metadata(file_path)
                file_name = os.path.basename(file_path)

                return {
                    "filename": file_name,
                    "file_size": minio_metadata["size"],
                    "modified_time": minio_metadata["last_modified"].timestamp() if hasattr(minio_metadata["last_modified"], "timestamp") else 0,
                    "extension": self.get_file_extension(file_path),
                    "content_type": minio_metadata.get("content_type", "")
                }
            except FileNotFoundError:
                # 如果MinIO中不存在，尝试本地文件系统
                if not os.path.exists(file_path):
                    self.logger.error(f"File not found: {file_path}")
                    raise FileNotFoundError(f"File not found: {file_path}")

                file_stats = os.stat(file_path)
                file_name = os.path.basename(file_path)

                return {
                    "filename": file_name,
                    "file_size": file_stats.st_size,
                    "modified_time": file_stats.st_mtime,
                    "extension": self.get_file_extension(file_path)
                }
        except Exception as e:
            self.logger.error(f"Error getting metadata: {str(e)}")
            raise

    def validate_file(self, file_path: str, max_size_mb: int = 100) -> None:
        """
        验证文件是否可以解析

        Args:
            file_path: 文档文件路径
            max_size_mb: 最大文件大小（MB）

        Raises:
            FileNotFoundError: 文件不存在
            ValueError: 文件太大或格式不支持
        """
        try:
            # 检查是否是Windows风格的路径
            is_windows_path = file_path and (file_path.startswith('C:') or file_path.startswith('D:') or ':\\' in file_path)
            
            # 检查文件是否存在
            file_exists = False
            file_size = 0

            # 如果是Windows路径，只检查本地文件系统
            if is_windows_path:
                if os.path.exists(file_path):
                    file_exists = True
                    file_size = os.path.getsize(file_path)
            else:
                # 非Windows路径，先尝试MinIO，再尝试本地
                try:
                    if self.minio_client.file_exists(file_path):
                        metadata = self.minio_client.get_object_metadata(file_path)
                        file_exists = True
                        file_size = metadata["size"]
                except Exception as e:
                    self.logger.debug(f"MinIO check failed, trying local file system: {str(e)}")
                    
                    if os.path.exists(file_path):
                        file_exists = True
                        file_size = os.path.getsize(file_path)

            if not file_exists:
                self.logger.error(f"File not found: {file_path}")
                raise FileNotFoundError(f"File not found: {file_path}")

            # 检查文件大小
            file_size_mb = file_size / (1024 * 1024)
            if file_size_mb > max_size_mb:
                self.logger.error(f"File too large: {file_size_mb:.2f}MB > {max_size_mb}MB")
                raise ValueError(f"File too large: {file_size_mb:.2f}MB exceeds limit of {max_size_mb}MB")

            # 检查文件类型
            ext = self.get_file_extension(file_path)
            if not self.supports_extension(ext):
                self.logger.error(f"Unsupported file extension: {ext}")
                raise ValueError(f"Unsupported file extension: {ext}")
        except FileNotFoundError:
            raise
        except Exception as e:
            self.logger.error(f"Error validating file: {str(e)}")
            raise

    def get_file_extension(self, file_path: str) -> str:
        """
        获取文件扩展名

        Args:
            file_path: 文件路径

        Returns:
            str: 文件扩展名（小写，不含点号）
        """
        return Path(file_path).suffix.lower().lstrip('.')

    def supports_extension(self, extension: str) -> bool:
        """
        检查是否支持该文件扩展名

        Args:
            extension: 文件扩展名（不含点号）

        Returns:
            bool: 支持返回True，否则False
        """
        # 子类需要重写此方法以指定支持的扩展名
        return False

    def preprocess_text(self, text: str) -> str:
        """
        预处理提取的文本

        Args:
            text: 原始文本内容

        Returns:
            str: 预处理后的文本
        """
        if text is None:
            return ""

        import re

        # 统一换行符
        text = text.replace('\r\n', '\n').replace('\r', '\n')

        # 替换连续多个换行为两个换行
        text = re.sub(r'\n{3,}', '\n\n', text)

        # 移除连续多个空格
        text = re.sub(r' {2,}', ' ', text)

        # 移除控制字符
        text = re.sub(r'[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]', '', text)

        return text.strip()

    def extract_title(self, text: str, filename: str) -> str:
        """
        从文本中提取标题

        Args:
            text: 文档文本内容
            filename: 文件名，作为备选标题

        Returns:
            str: 提取的标题
        """
        # 默认实现：尝试从文本的第一行提取标题，否则使用文件名
        if not text:
            return os.path.splitext(filename)[0]

        # 获取第一个非空行
        lines = text.split('\n')
        for line in lines:
            line = line.strip()
            if line and len(line) <= 100:  # 标题不太可能超过100个字符
                return line

        # 如果没有找到合适的标题，使用文件名
        return os.path.splitext(filename)[0]

    def get_file_content(self, file_path: str) -> BinaryIO:
        """
        获取文件内容（支持MinIO和本地文件系统）

        Args:
            file_path: 文件路径

        Returns:
            BinaryIO: 文件二进制流
        """
        # 检查是否是Windows风格的路径
        is_windows_path = file_path and (file_path.startswith('C:') or file_path.startswith('D:') or ':\\' in file_path)
        
        # Windows路径直接从本地文件系统读取，不尝试从MinIO获取
        if is_windows_path:
            if os.path.exists(file_path):
                return open(file_path, 'rb')
            else:
                raise FileNotFoundError(f"File not found: {file_path}")
        
        # 非Windows路径，尝试从MinIO获取
        try:
            # 尝试从MinIO获取
            return self.minio_client.get_object(file_path)
        except Exception as e:
            self.logger.debug(f"Error getting file from MinIO: {str(e)}, trying local filesystem")
            
            # 如果从MinIO获取失败，尝试从本地文件系统获取
            if os.path.exists(file_path):
                return open(file_path, 'rb')
            else:
                raise FileNotFoundError(f"File not found: {file_path}")