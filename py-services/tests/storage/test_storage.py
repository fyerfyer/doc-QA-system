import os
import unittest
import tempfile
import shutil
from unittest.mock import patch, MagicMock
import pytest
from minio.error import S3Error
from dotenv import load_dotenv

# 加载环境变量
load_dotenv(dotenv_path=os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), '.env.example'))

from common.storage import Storage

@pytest.mark.minio
class TestMinioStorageReal(unittest.TestCase):
    """MinIO实际集成测试（需要真实的MinIO服务）"""

    @classmethod
    def setUpClass(cls):
        """设置MinIO连接参数"""
        # 从.env.example加载MinIO配置
        cls.minio_endpoint = os.environ.get('MINIO_ENDPOINT')
        cls.minio_access_key = os.environ.get('MINIO_ACCESS_KEY')
        cls.minio_secret_key = os.environ.get('MINIO_SECRET_KEY')

        # 检查是否有MinIO环境可供测试
        if not cls.minio_endpoint or not cls.minio_access_key or not cls.minio_secret_key:
            pytest.skip("Skipping MinIO tests: Missing MinIO configuration in .env.example")

        # 保存原始环境变量
        cls.orig_env = {}
        for key in ['STORAGE_TYPE']:
            cls.orig_env[key] = os.environ.get(key)

        # 设置存储类型
        os.environ['STORAGE_TYPE'] = 'minio'

        # 创建测试桶名称
        cls.test_bucket = os.environ.get('MINIO_BUCKET', 'docqa-test')

    @classmethod
    def tearDownClass(cls):
        """恢复环境变量"""
        for key, value in cls.orig_env.items():
            if value is None:
                if key in os.environ:
                    del os.environ[key]
            else:
                os.environ[key] = value

    def setUp(self):
        """准备测试环境"""
        try:
            # 创建存储实例
            self.storage = Storage()

            # 清理可能存在的测试文件
            self._clean_test_files()
        except Exception as e:
            pytest.skip(f"Failed to setup MinIO test: {str(e)}")

    def tearDown(self):
        """清理测试环境"""
        self._clean_test_files()

    def _clean_test_files(self):
        """清理测试文件"""
        try:
            test_files = ["test_file.txt", "test_folder/nested_file.txt"]
            for file_path in test_files:
                try:
                    self.storage.delete_file(file_path)
                except:
                    pass
        except:
            pass

    def test_real_minio_basic_operations(self):
        """测试实际MinIO的基本操作"""
        try:
            # 保存文件
            test_content = b"This is a test file for MinIO storage."
            file_path = "test_file.txt"

            self.storage.save_file(file_path, test_content)

            # 检查文件是否存在
            self.assertTrue(self.storage.file_exists(file_path))

            # 获取并检查内容
            retrieved_content = self.storage.get_file(file_path)
            self.assertEqual(retrieved_content, test_content)

            # 删除文件
            result = self.storage.delete_file(file_path)
            self.assertTrue(result)

            # 确认文件已删除
            self.assertFalse(self.storage.file_exists(file_path))

        except Exception as e:
            self.fail(f"MinIO integration test failed: {str(e)}")

    def test_real_minio_nested_paths(self):
        """测试实际MinIO的嵌套路径操作"""
        try:
            # 保存嵌套路径的文件
            test_content = b"This is a nested file."
            file_path = "test_folder/nested_file.txt"

            self.storage.save_file(file_path, test_content)

            # 检查文件是否存在
            self.assertTrue(self.storage.file_exists(file_path))

            # 列出文件
            files = self.storage.list_files("test_folder")
            self.assertEqual(len(files), 1)
            self.assertEqual(files[0]["path"], file_path)

            # 删除文件
            self.storage.delete_file(file_path)

        except Exception as e:
            self.fail(f"MinIO nested path test failed: {str(e)}")

if __name__ == "__main__":
    unittest.main()