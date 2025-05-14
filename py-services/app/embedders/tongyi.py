import os
import logging
from typing import List, Optional, Union
import dashscope
from http import HTTPStatus

from app.embedders.base import BaseEmbedder

# 初始化日志记录器
logger = logging.getLogger(__name__)

class TongyiEmbedder(BaseEmbedder):
    """通义千问嵌入模型实现"""

    DEFAULT_MODEL = "text-embedding-v3"

    def __init__(
            self,
            api_key: Optional[str] = None,
            model_name: str = DEFAULT_MODEL,
            dimension: int = 1024,
            output_type: str = "dense",
            **kwargs
    ):
        """
        初始化通义千问嵌入器

        参数:
            api_key: 通义千问API密钥，如果为None则从环境变量获取
            model_name: 模型名称，默认为text-embedding-v3
            dimension: 嵌入向量维度，只支持1024、768、512、256、128或64
            output_type: 输出类型，可选值为dense、sparse或dense&sparse
            **kwargs: 其他配置参数
        """
        # 初始化基类
        super().__init__(model_name=model_name, dimension=dimension, **kwargs)

        # 验证API密钥
        self.api_key = api_key or os.getenv("DASHSCOPE_API_KEY")
        if not self.api_key:
            raise ValueError("API key must be provided or set as DASHSCOPE_API_KEY environment variable")

        # 验证向量维度
        valid_dimensions = [1024, 768, 512, 256, 128, 64]
        if dimension not in valid_dimensions:
            self.logger.warning(
                f"Invalid dimension {dimension} for tongyi embedding. "
                f"Using default dimension: 1024"
            )
            self.dimension = 1024

        # 验证输出类型
        valid_output_types = ["dense", "sparse", "dense&sparse"]
        self.output_type = output_type if output_type in valid_output_types else "dense"

        # 设置DashScope API密钥
        if api_key:
            dashscope.api_key = api_key

        self.logger.info(f"Initialized Tongyi embedder with model: {model_name}, dimension: {self.dimension}")

    def embed(self, text: str) -> List[float]:
        """
        将单个文本转换为嵌入向量

        参数:
            text: 要嵌入的文本

        返回:
            List[float]: 嵌入向量
        """
        # 使用批量嵌入并返回第一个结果
        results = self.embed_batch([text])
        return results[0]

    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """
        批量将文本转换为嵌入向量

        参数:
            texts: 要嵌入的文本列表

        返回:
            List[List[float]]: 嵌入向量列表
        """
        # 验证输入
        valid_texts = self.validate_inputs(texts)

        # 使用重试机制调用API
        def _call_api(batch):
            try:
                # 构建API请求参数
                params = {
                    "model": self.model_name,
                    "input": batch,
                }

                # text-embedding-v3模型支持设置维度
                if "v3" in self.model_name.lower():
                    params["dimension"] = self.dimension
                    params["output_type"] = self.output_type

                # 调用通义千问API
                resp = dashscope.TextEmbedding.call(**params)

                if resp.status_code == HTTPStatus.OK:
                    # 提取嵌入向量
                    embeddings = []
                    for embedding_data in resp.output.get('embeddings', []):
                        embeddings.append(embedding_data.get('embedding', []))
                    return embeddings
                else:
                    raise Exception(f"Embedding API error: {resp.code} - {resp.message}")

            except Exception as e:
                self.logger.error(f"Error calling Tongyi embedding API: {str(e)}")
                raise

        # 分批处理并合并结果
        all_embeddings = []
        for i in range(0, len(valid_texts), self.batch_size):
            batch = valid_texts[i:i+self.batch_size]
            batch_embeddings = self._retry_with_backoff(_call_api, batch)
            all_embeddings.extend(batch_embeddings)

        return all_embeddings

    def get_supported_dimensions(self) -> List[int]:
        """
        获取当前模型支持的向量维度

        返回:
            List[int]: 支持的维度列表
        """
        if "v3" in self.model_name.lower():
            return [1024, 768, 512, 256, 128, 64]
        # v1和v2版本不支持自定义维度
        return []

    def embed_with_openai_compatible(self, text: Union[str, List[str]]) -> Union[List[float], List[List[float]]]:
        """
        使用OpenAI兼容接口进行嵌入计算

        参数:
            text: 单个文本字符串或文本列表

        返回:
            如果输入是字符串，返回List[float]；如果输入是列表，返回List[List[float]]
        """
        try:
            from openai import OpenAI
        except ImportError:
            raise ImportError("OpenAI package is required. Please install with: pip install openai")

        is_batch = isinstance(text, list)
        inputs = text if is_batch else [text]

        # 验证输入
        valid_texts = self.validate_inputs(inputs)

        # 创建OpenAI兼容客户端
        client = OpenAI(
            api_key=self.api_key,
            base_url="https://dashscope.aliyuncs.com/compatible-mode/v1"
        )

        all_embeddings = []
        # 分批处理
        for i in range(0, len(valid_texts), self.batch_size):
            batch = valid_texts[i:i+self.batch_size]

            try:
                response = client.embeddings.create(
                    model=self.model_name,
                    input=batch,
                    dimensions=self.dimension,
                    encoding_format="float"
                )

                # 提取嵌入向量
                batch_embeddings = [item.embedding for item in response.data]
                all_embeddings.extend(batch_embeddings)

            except Exception as e:
                self.logger.error(f"Error using OpenAI compatible API: {str(e)}")
                raise

        # 如果输入是单个字符串，返回第一个结果
        return all_embeddings[0] if not is_batch else all_embeddings