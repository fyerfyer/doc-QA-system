import os
import io
import logging
from typing import BinaryIO, Dict, List, Optional, Tuple, Union
from minio import Minio
from minio.error import S3Error

class Storage:
    """存储服务接口，兼容本地文件系统和MinIO对象存储"""

    def __init__(self):
        """初始化存储服务，根据环境变量配置选择存储类型"""
        self.logger = logging.getLogger(__name__)
        self.storage_type = os.environ.get("STORAGE_TYPE", "local")

        if self.storage_type == "local":
            self.base_path = os.environ.get("STORAGE_PATH", "./uploads")
            # 确保目录存在
            os.makedirs(self.base_path, exist_ok=True)
            self.logger.info(f"Using local storage at path: {self.base_path}")
        elif self.storage_type == "minio":
            endpoint = os.environ.get("MINIO_ENDPOINT", "localhost:9000")
            access_key = os.environ.get("MINIO_ACCESS_KEY", "")
            secret_key = os.environ.get("MINIO_SECRET_KEY", "")
            secure = os.environ.get("MINIO_USE_SSL", "false").lower() == "true"
            self.bucket = os.environ.get("MINIO_BUCKET", "docqa")

            self.client = Minio(
                endpoint=endpoint,
                access_key=access_key,
                secret_key=secret_key,
                secure=secure
            )

            # 确保桶存在
            if not self.client.bucket_exists(self.bucket):
                self.client.make_bucket(self.bucket)
                self.logger.info(f"MinIO bucket {self.bucket} created")
            self.logger.info(f"Using MinIO storage at endpoint: {endpoint}, bucket: {self.bucket}")
        else:
            raise ValueError(f"Unsupported storage type: {self.storage_type}")

    def get_file(self, file_id: str) -> bytes:
        """获取文件内容

        Args:
            file_id: 文件ID或路径

        Returns:
            文件内容的二进制数据

        Raises:
            FileNotFoundError: 文件不存在时抛出
            IOError: 读取文件失败时抛出
        """
        if self.storage_type == "local":
            # 对于本地存储，尝试两种路径：直接用ID或完整路径
            try:
                # 首先尝试将file_id作为相对路径
                file_path = os.path.join(self.base_path, file_id)
                if os.path.exists(file_path):
                    with open(file_path, 'rb') as f:
                        return f.read()

                # 如果不存在，再尝试直接使用file_id
                if os.path.exists(file_id):
                    with open(file_id, 'rb') as f:
                        return f.read()

                raise FileNotFoundError(f"File not found: {file_id}")
            except FileNotFoundError as e:
                # 直接继续抛出FileNotFoundError，不转换为IOError
                self.logger.error(f"Failed to read file: {e}")
                raise
            except IOError as e:
                # 其他IO错误保持原样抛出
                self.logger.error(f"IO error when reading file: {e}")
                raise IOError(f"Failed to read file: {e}")
        else:
            # MinIO存储
            try:
                response = self.client.get_object(self.bucket, file_id)
                return response.read()
            except S3Error as e:
                self.logger.error(f"Failed to get file from MinIO: {e}")
                raise FileNotFoundError(f"File not found or inaccessible: {file_id}")
            finally:
                # 确保关闭response
                if 'response' in locals():
                    response.close()
                    response.release_conn()

    def save_file(self, file_path: str, content: bytes) -> str:
        """保存文件内容

        Args:
            file_path: 文件保存路径
            content: 文件二进制内容

        Returns:
            保存的文件路径

        Raises:
            IOError: 保存文件失败时抛出
        """
        if self.storage_type == "local":
            try:
                # 确保目录存在
                os.makedirs(os.path.dirname(os.path.join(self.base_path, file_path)), exist_ok=True)
                full_path = os.path.join(self.base_path, file_path)

                with open(full_path, 'wb') as f:
                    f.write(content)
                return file_path
            except IOError as e:
                self.logger.error(f"Failed to save file: {e}")
                raise IOError(f"Failed to save file: {e}")
        else:
            # MinIO存储
            try:
                content_bytes_io = io.BytesIO(content)
                self.client.put_object(
                    self.bucket,
                    file_path,
                    content_bytes_io,
                    length=len(content)
                )
                return file_path
            except S3Error as e:
                self.logger.error(f"Failed to save file to MinIO: {e}")
                raise IOError(f"Failed to save file: {e}")

    def delete_file(self, file_id: str) -> bool:
        """删除文件

        Args:
            file_id: 文件ID或路径

        Returns:
            是否成功删除
        """
        if self.storage_type == "local":
            try:
                file_path = os.path.join(self.base_path, file_id)
                if os.path.exists(file_path):
                    os.remove(file_path)
                    return True
                return False
            except OSError as e:
                self.logger.error(f"Failed to delete file: {e}")
                return False
        else:
            # MinIO存储
            try:
                self.client.remove_object(self.bucket, file_id)
                return True
            except S3Error as e:
                self.logger.error(f"Failed to delete file from MinIO: {e}")
                return False

    def file_exists(self, file_id: str) -> bool:
        """检查文件是否存在

        Args:
            file_id: 文件ID或路径

        Returns:
            文件是否存在
        """
        if self.storage_type == "local":
            file_path = os.path.join(self.base_path, file_id)
            return os.path.exists(file_path)
        else:
            # MinIO存储
            try:
                self.client.stat_object(self.bucket, file_id)
                return True
            except S3Error:
                return False

    def list_files(self, prefix: str = "") -> List[Dict[str, str]]:
        """列出存储中的文件

        Args:
            prefix: 文件前缀过滤器

        Returns:
            文件信息列表
        """
        files = []

        if self.storage_type == "local":
            base_dir = os.path.join(self.base_path, prefix)
            if not os.path.exists(base_dir):
                return files

            for root, _, filenames in os.walk(base_dir):
                for filename in filenames:
                    full_path = os.path.join(root, filename)
                    rel_path = os.path.relpath(full_path, self.base_path)
                    files.append({
                        "id": rel_path,
                        "name": filename,
                        "path": rel_path
                    })
        else:
            # MinIO存储
            objects = self.client.list_objects(self.bucket, prefix=prefix, recursive=True)
            for obj in objects:
                files.append({
                    "id": obj.object_name,
                    "name": os.path.basename(obj.object_name),
                    "path": obj.object_name
                })

        return files