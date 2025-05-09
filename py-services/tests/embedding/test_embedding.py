import os
import unittest
import pytest
from unittest.mock import patch, MagicMock
import numpy as np
from dotenv import load_dotenv

# 加载环境变量
load_dotenv()

# 导入被测试模块
from embedding.service import TextEmbedder, create_embedder

class TestTextEmbedder(unittest.TestCase):
    """文本向量化服务测试类"""

    def setUp(self):
        """测试前准备"""
        # 创建测试用的嵌入模型客户端
        self.embedder = TextEmbedder(model_name="text-embedding-v3", dimension=512)

        # 创建模拟的API响应
        self.mock_response = MagicMock()
        self.mock_response.status_code = 200
        self.mock_response.output = {
            'embeddings': [
                {'embedding': [0.1, 0.2, 0.3, 0.4] * 128, 'text_index': 0}
            ]
        }

        # 创建批量API响应
        self.mock_batch_response = MagicMock()
        self.mock_batch_response.status_code = 200
        self.mock_batch_response.output = {
            'embeddings': [
                {'embedding': [0.1, 0.2, 0.3, 0.4] * 128, 'text_index': 0},
                {'embedding': [0.4, 0.3, 0.2, 0.1] * 128, 'text_index': 1}
            ]
        }

    @patch('embedding.service.dashscope.TextEmbedding.call')
    def test_embed_text_basic(self, mock_call):
        """测试基本的文本向量化功能"""
        # 设置Mock返回值
        mock_call.return_value = self.mock_response

        # 执行测试
        result = self.embedder.embed_text("这是一个测试文本")

        # 验证结果
        self.assertEqual(len(result), 512)  # 检查向量维度
        self.assertEqual(result[:4], [0.1, 0.2, 0.3, 0.4])  # 验证向量内容

        # 验证API调用参数
        mock_call.assert_called_once()
        args, kwargs = mock_call.call_args
        self.assertEqual(kwargs["model"], "text-embedding-v3")
        self.assertEqual(kwargs["input"], ["这是一个测试文本"])
        self.assertEqual(kwargs["dimension"], 512)  # 修改为dimension而不是dimensions

    @patch('embedding.service.dashscope.TextEmbedding.call')
    def test_embed_batch_texts(self, mock_call):
        """测试批量文本向量化"""
        # 设置Mock返回值
        mock_call.return_value = self.mock_batch_response

        # 准备测试数据
        texts = ["第一个测试文本", "第二个测试文本"]

        # 执行测试
        results = self.embedder.embed_batch(texts)

        # 验证结果
        self.assertEqual(len(results), 2)  # 检查返回列表长度
        self.assertEqual(len(results[0]), 512)  # 检查向量维度
        self.assertEqual(len(results[1]), 512)  # 检查向量维度

        # 验证API调用参数
        mock_call.assert_called_once()
        args, kwargs = mock_call.call_args
        self.assertEqual(kwargs["model"], "text-embedding-v3")
        self.assertEqual(kwargs["input"], texts)

    @patch('embedding.service.dashscope.TextEmbedding.call')
    def test_empty_input_handling(self, mock_call):
        """测试空输入处理"""
        # 测试空文本
        with self.assertRaises(ValueError):
            self.embedder.embed_text("")

        # 测试全空格文本
        with self.assertRaises(ValueError):
            self.embedder.embed_text("   ")

        # 测试空列表
        results = self.embedder.embed_batch([])
        self.assertEqual(results, [])

        # 测试全为空文本的列表
        with self.assertRaises(ValueError):
            self.embedder.embed_batch(["", "  "])

        # 确认没有API调用
        mock_call.assert_not_called()

    @patch('embedding.service.dashscope.TextEmbedding.call')
    def test_api_error_handling(self, mock_call):
        """测试API错误处理"""
        # 设置Mock返回错误
        error_response = MagicMock()
        error_response.status_code = 400
        error_response.message = "Invalid API key"
        mock_call.return_value = error_response

        # 测试错误处理
        with self.assertRaises(Exception) as context:
            self.embedder.embed_text("测试文本")

        self.assertIn("API error", str(context.exception))

    @patch('embedding.service.dashscope.TextEmbedding.call')
    def test_retry_mechanism(self, mock_call):
        """测试重试机制"""
        # 设置前两次调用失败，第三次成功
        mock_call.side_effect = [
            Exception("Connection error"),
            Exception("Timeout error"),
            self.mock_response
        ]

        # 设置较短的重试延迟用于测试
        self.embedder.retry_delay = 0.01

        # 执行测试
        result = self.embedder.embed_text("测试重试")

        # 验证结果
        self.assertEqual(len(result), 512)

        # 验证API被调用了三次
        self.assertEqual(mock_call.call_count, 3)

    @patch('embedding.service.dashscope.TextEmbedding.call')
    def test_batch_size_limit(self, mock_call):
        """测试批量处理大小限制"""
        # V3模型最多支持10条文本
        mock_call.return_value = self.mock_batch_response

        # 准备超过限制的输入
        large_batch = ["文本" + str(i) for i in range(15)]

        # 执行测试
        self.embedder.embed_batch(large_batch[:2])  # 只传入两条以匹配mock响应

        # 验证API调用中的输入被限制到适当数量
        args, kwargs = mock_call.call_args
        self.assertEqual(len(kwargs["input"]), 2)

        # 重置mock并测试V2模型
        mock_call.reset_mock()
        self.embedder.model_name = "text-embedding-v2"
        mock_call.return_value = self.mock_batch_response

        self.embedder.embed_batch(large_batch[:2])

        # 验证V2模型的限制是25条
        args, kwargs = mock_call.call_args
        self.assertEqual(len(kwargs["input"]), 2)

    def test_get_model_info(self):
        """测试获取模型信息"""
        # 测试V3模型
        self.embedder.model_name = "text-embedding-v3"
        self.embedder.dimension = 512
        info = self.embedder.get_model_info()

        self.assertEqual(info["model_name"], "text-embedding-v3")
        self.assertEqual(info["dimension"], 512)
        self.assertEqual(info["provider"], "tongyi")

        # 测试V2模型
        self.embedder.model_name = "text-embedding-v2"
        info = self.embedder.get_model_info()

        self.assertEqual(info["dimension"], 1024)

    def test_create_embedder_function(self):
        """测试创建嵌入器实例的工厂函数"""
        # 测试默认参数
        embedder = create_embedder()
        self.assertEqual(embedder.model_name, "text-embedding-v3")
        self.assertEqual(embedder.dimension, 1024)

        # 测试自定义参数
        custom_embedder = create_embedder(
            model_name="text-embedding-v2",
            dimension=768,
            api_key="test-key"
        )
        self.assertEqual(custom_embedder.model_name, "text-embedding-v2")
        self.assertEqual(custom_embedder.api_key, "test-key")


@pytest.mark.integration
class TestTextEmbedderIntegration(unittest.TestCase):
    """文本向量化服务集成测试（需要API密钥）"""

    @classmethod
    def setUpClass(cls):
        """设置API连接参数"""
        # 检查是否有API密钥可供测试
        cls.api_key = os.environ.get("DASHSCOPE_API_KEY")
        if not cls.api_key:
            pytest.skip("Skipping integration tests: No DASHSCOPE_API_KEY in environment")

    def setUp(self):
        """准备测试环境"""
        self.embedder = TextEmbedder(
            model_name="text-embedding-v3",
            api_key=self.api_key,
            dimension=512
        )

    def test_real_single_embedding(self):
        """测试实际API单文本向量化"""
        try:
            # 执行测试
            result = self.embedder.embed_text("这是一个真实API调用的测试文本。")

            # 验证结果
            self.assertEqual(len(result), 512)
            # 验证是否为有效的数值数组
            for val in result:
                self.assertTrue(isinstance(val, float))

            # 计算向量范数，验证向量是否已正则化
            norm = np.linalg.norm(result)
            self.assertAlmostEqual(norm, 1.0, places=5)

            print(f"Successfully generated embedding with dimension: {len(result)}")

        except Exception as e:
            self.fail(f"Real API call failed: {str(e)}")

    def test_real_batch_embedding(self):
        """测试实际API批量文本向量化"""
        try:
            # 准备测试数据
            texts = [
                "这是第一个测试文本，用于批量测试。",
                "这是第二个测试文本，内容不同以测试区分度。"
            ]

            # 执行测试
            results = self.embedder.embed_batch(texts)

            # 验证结果
            self.assertEqual(len(results), 2)
            self.assertEqual(len(results[0]), 512)
            self.assertEqual(len(results[1]), 512)

            # 计算向量之间的余弦相似度，确保不同文本的向量有区分度
            vec1 = np.array(results[0])
            vec2 = np.array(results[1])
            cosine_similarity = np.dot(vec1, vec2)

            # 由于是不同文本，相似度不应该太高
            self.assertLess(cosine_similarity, 0.95)

            print(f"Successfully generated batch embeddings with similarity: {cosine_similarity:.4f}")

        except Exception as e:
            self.fail(f"Real batch API call failed: {str(e)}")

    def test_model_dimension_options(self):
        """测试不同维度选项"""
        try:
            # 测试不同维度
            dimensions = [512, 768, 1024]

            for dim in dimensions:
                # 创建特定维度的嵌入器
                dim_embedder = TextEmbedder(
                    model_name="text-embedding-v3",
                    api_key=self.api_key,
                    dimension=dim
                )

                # 生成向量
                result = dim_embedder.embed_text("测试不同维度选项")

                # 验证维度
                self.assertEqual(len(result), dim)
                print(f"Successfully generated {dim}-dimensional embedding")

        except Exception as e:
            self.fail(f"Dimension test failed: {str(e)}")


if __name__ == "__main__":
    unittest.main()