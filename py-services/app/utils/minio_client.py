import os
import io
import datetime
import mimetypes
from typing import BinaryIO, Dict, Any
from minio import Minio
from minio.error import S3Error

from app.utils.utils import logger, retry

class MinioClient:
    """MinIO 客户端封装类，处理文件存储操作"""

    _instance = None  # 单例实例

    def __new__(cls):
        if cls._instance is None:
            cls._instance = super(MinioClient, cls).__new__(cls)
            cls._instance._initialized = False
        return cls._instance

    def __init__(self):
        if self._initialized:
            return

        # 从环境变量获取MinIO配置
        self.endpoint = os.getenv("MINIO_ENDPOINT", "localhost:9000")
        self.access_key = os.getenv("MINIO_ACCESS_KEY", "minioadmin")
        self.secret_key = os.getenv("MINIO_SECRET_KEY", "minioadmin")
        self.use_ssl = os.getenv("MINIO_USE_SSL", "false").lower() == "true"
        self.bucket = os.getenv("MINIO_BUCKET", "docqa")

        # 创建MinIO客户端
        self.client = Minio(
            endpoint=self.endpoint,
            access_key=self.access_key,
            secret_key=self.secret_key,
            secure=self.use_ssl
        )

        # 确保存储桶存在
        self._ensure_bucket()

        self._initialized = True
        logger.info(f"MinIO client initialized with endpoint: {self.endpoint}, bucket: {self.bucket}")

    def _ensure_bucket(self):
        """确保存储桶存在，不存在则创建"""
        try:
            if not self.client.bucket_exists(self.bucket):
                self.client.make_bucket(self.bucket)
                logger.info(f"Created bucket: {self.bucket}")
        except S3Error as e:
            logger.error(f"Error ensuring bucket exists: {str(e)}")
            raise

    @retry(max_retries=3, delay=1, backoff=2, exceptions=(S3Error,))
    def get_object(self, file_path: str) -> BinaryIO:
        """
        从MinIO获取文件对象
        
        参数:
            file_path: MinIO中的文件路径
            
        返回:
            BinaryIO: 文件内容的二进制流
            
        异常:
            S3Error: MinIO操作错误
            FileNotFoundError: 文件不存在
        """
        # 检查是否是Windows风格的路径
        if file_path and (file_path.startswith('C:') or file_path.startswith('D:') or ':\\' in file_path):
            # Windows路径直接从本地文件系统获取
            if not os.path.exists(file_path):
                raise FileNotFoundError(f"File not found: {file_path}")
            return open(file_path, 'rb')
            
        try:
            response = self.client.get_object(self.bucket, file_path)
            return response
        except S3Error as e:
            if e.code == 'NoSuchKey':
                raise FileNotFoundError(f"File not found in MinIO: {file_path}")
            logger.error(f"Error retrieving file from MinIO: {str(e)}")
            raise

    def get_object_as_bytes(self, file_path: str) -> bytes:
        """获取文件内容并返回为bytes"""
        try:
            response = self.get_object(file_path)
            return response.read()
        finally:
            if 'response' in locals() and response:
                response.close()
                response.release_conn()

    def get_object_as_io(self, file_path: str) -> io.BytesIO:
        """获取文件内容并返回为BytesIO对象"""
        data = self.get_object_as_bytes(file_path)
        return io.BytesIO(data)

    def get_object_metadata(self, file_path: str) -> Dict[str, Any]:
        """获取文件元数据"""
        # 检查是否是Windows风格的路径
        if file_path and (file_path.startswith('C:') or file_path.startswith('D:') or ':\\' in file_path):
            # Windows路径不在MinIO中查找，使用本地文件系统
            if not os.path.exists(file_path):
                raise FileNotFoundError(f"File not found: {file_path}")
            
            stat = os.stat(file_path)
            return {
                "size": stat.st_size,
                "last_modified": datetime.datetime.fromtimestamp(stat.st_mtime),
                "content_type": mimetypes.guess_type(file_path)[0] or "application/octet-stream",
                "metadata": {}
            }
            
        try:
            stat = self.client.stat_object(self.bucket, file_path)
            return {
                "size": stat.size,
                "last_modified": stat.last_modified,
                "content_type": stat.content_type,
                "metadata": stat.metadata
            }
        except S3Error as e:
            if e.code == 'NoSuchKey':
                raise FileNotFoundError(f"File not found in MinIO: {file_path}")
            logger.error(f"Error retrieving metadata from MinIO: {str(e)}")
            raise

    def file_exists(self, file_path: str) -> bool:
        """检查文件是否存在"""
        # 检查是否是Windows风格的路径
        if file_path and (file_path.startswith('C:') or file_path.startswith('D:') or ':\\' in file_path):
            # Windows路径不在MinIO中查找
            return False
            
        try:
            self.client.stat_object(self.bucket, file_path)
            return True
        except S3Error as e:
            if e.code == 'NoSuchKey':
                return False
            logger.error(f"Error checking file existence in MinIO: {str(e)}")
            raise

    def download_file(self, file_path: str, local_path: str) -> bool:
        """
        从MinIO下载文件到本地路径
        参数:
            file_path: MinIO中的文件路径
            local_path: 本地保存路径
        返回:
            bool: 下载是否成功
        """
        try:
            # 从MinIO获取文件对象
            response = self.get_object(file_path)
            
            # 将文件内容写入本地文件
            with open(local_path, 'wb') as local_file:
                for data in response.stream(32*1024):
                    local_file.write(data)
            
            # 关闭响应对象
            response.close()
            response.release_conn()
            
            logger.info(f"Successfully downloaded file from MinIO: {file_path} to {local_path}")
            return True
        except FileNotFoundError:
            logger.error(f"File not found in MinIO: {file_path}")
            return False
        except Exception as e:
            logger.error(f"Error downloading file from MinIO: {str(e)}")
            return False

# 创建全局实例供导入使用
minio_client = MinioClient()

def get_minio_client() -> MinioClient:
    """获取MinIO客户端实例"""
    return minio_client