import os
import logging
import time
from typing import List, Dict, Any, Optional, Union
import numpy as np

import dashscope
from dashscope.api_entities.dashscope_response import DashScopeAPIResponse

# 设置默认日志级别
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)

class TextEmbedder:
    """文本向量化服务，用于将文本转换为向量表示"""

    def __init__(self, model_name: str = "text-embedding-v3",
                 api_key: Optional[str] = None,
                 dimension: int = 1024,
                 max_retries: int = 3,
                 retry_delay: float = 1.0):
        """
        初始化文本向量化服务

        Args:
            model_name: 模型名称，默认使用"text-embedding-v3"
            api_key: 通义千问API密钥，默认从环境变量获取
            dimension: 向量维度，仅用于v3模型，可选1024, 768, 512
            max_retries: 最大重试次数
            retry_delay: 重试延迟时间(秒)
        """
        self.logger = logging.getLogger(__name__)
        self.model_name = model_name
        self.api_key = api_key or os.environ.get("DASHSCOPE_API_KEY")
        self.max_retries = max_retries
        self.retry_delay = retry_delay
        self.dimension = dimension

        # 验证API密钥
        if not self.api_key:
            self.logger.warning("No API key provided and DASHSCOPE_API_KEY not found in environment variables")

        # 验证模型名称
        valid_models = ["text-embedding-v1", "text-embedding-v2", "text-embedding-v3"]
        if self.model_name not in valid_models:
            self.logger.warning(f"Unsupported model: {self.model_name}, falling back to text-embedding-v3")
            self.model_name = "text-embedding-v3"

        # 验证维度参数（仅适用于v3模型）
        if self.model_name == "text-embedding-v3":
            valid_dimensions = [1024, 768, 512]
            if self.dimension not in valid_dimensions:
                self.logger.warning(f"Invalid dimension for {self.model_name}: {self.dimension}, using default 1024")
                self.dimension = 1024

        self.logger.info(f"Initialized TextEmbedder with model: {self.model_name}")

    def _call_embedding_api(self, texts: List[str]) -> DashScopeAPIResponse:
        """调用通义千问embedding API"""
        # 对于v3模型，需要限制每批次最多10条文本
        if self.model_name == "text-embedding-v3" and len(texts) > 10:
            self.logger.warning(f"{self.model_name} supports maximum 10 texts per batch, received {len(texts)}")
            texts = texts[:10]
        # 对于v1和v2模型，限制每批次最多25条文本
        elif self.model_name in ["text-embedding-v1", "text-embedding-v2"] and len(texts) > 25:
            self.logger.warning(f"{self.model_name} supports maximum 25 texts per batch, received {len(texts)}")
            texts = texts[:25]
    
        # 设置API调用参数
        kwargs = {
            "model": self.model_name,
            "input": texts
        }

        # 对于v3模型，添加维度参数
        if self.model_name == "text-embedding-v3":
            kwargs["dimension"] = self.dimension

        for attempt in range(self.max_retries + 1):
            try:
                # 使用通义千问官方API调用方式
                response = dashscope.TextEmbedding.call(**kwargs)
                return response
            except Exception as e:
                if attempt < self.max_retries:
                    wait_time = self.retry_delay * (2 ** attempt)  # 指数退避
                    self.logger.warning(f"API call failed (attempt {attempt+1}/{self.max_retries+1}): {str(e)}. "
                                        f"Retrying in {wait_time:.2f} seconds...")
                    time.sleep(wait_time)
                else:
                    self.logger.error(f"API call failed after {self.max_retries+1} attempts: {str(e)}")
                    raise

    def embed_text(self, text: str) -> List[float]:
        """
        将单条文本转换为向量

        Args:
            text: 要转换的文本

        Returns:
            向量表示（浮点数列表）

        Raises:
            ValueError: 输入文本为空时抛出
            Exception: API调用失败时抛出
        """
        if not text or not text.strip():
            raise ValueError("Input text cannot be empty")

        self.logger.debug(f"Embedding single text of length {len(text)}")
        response = self._call_embedding_api([text])

        # 检查响应
        if hasattr(response, 'status_code') and response.status_code != 200:
            error_msg = getattr(response, 'message', 'Unknown error')
            raise Exception(f"API error: {error_msg}")

        # 从响应中提取向量
        embedding = response.output['embeddings'][0]['embedding']
        return embedding

    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """
        批量将多条文本转换为向量

        Args:
            texts: 要转换的文本列表

        Returns:
            向量表示列表

        Raises:
            ValueError: 输入文本列表为空时抛出
            Exception: API调用失败时抛出
        """
        if not texts:
            return []

        # 过滤空文本
        valid_texts = [t for t in texts if t and t.strip()]
        if not valid_texts:
            raise ValueError("All input texts are empty")

        self.logger.debug(f"Embedding batch of {len(valid_texts)} texts")

        # 根据模型限制，适当分批处理
        batch_size = 6 if self.model_name == "text-embedding-v3" else 25
        all_embeddings = []

        for i in range(0, len(valid_texts), batch_size):
            batch = valid_texts[i:i + batch_size]
            self.logger.debug(f"Processing batch {i//batch_size + 1}, size {len(batch)}")

            response = self._call_embedding_api(batch)

            # 检查响应
            if hasattr(response, 'status_code') and response.status_code != 200:
                error_msg = getattr(response, 'message', 'Unknown error')
                raise Exception(f"API error: {error_msg}")

            # 从响应中提取向量
            batch_embeddings = [item['embedding'] for item in response.output['embeddings']]
            all_embeddings.extend(batch_embeddings)

        # 确保结果和输入长度一致
        if len(all_embeddings) != len(valid_texts):
            self.logger.warning(f"Expected {len(valid_texts)} embeddings, but got {len(all_embeddings)}")

        return all_embeddings

    def get_embedding_dimension(self) -> int:
        """
        获取当前模型的向量维度

        Returns:
            向量维度
        """
        # V3模型的维度是可配置的
        if self.model_name == "text-embedding-v3":
            return self.dimension
        # V2模型固定1024维度
        elif self.model_name == "text-embedding-v2":
            return 1024
        # V1模型固定1536维度
        else:
            return 1536

    def get_model_info(self) -> Dict[str, Any]:
        """
        获取模型信息

        Returns:
            包含模型名称、维度等信息的字典
        """
        return {
            "model_name": self.model_name,
            "dimension": self.get_embedding_dimension(),
            "provider": "tongyi"
        }


# 创建默认实例，方便直接导入使用
default_embedder = TextEmbedder()

def create_embedder(model_name: str = "text-embedding-v3",
                    api_key: Optional[str] = None,
                    dimension: int = 1024) -> TextEmbedder:
    """
    创建文本向量化服务实例

    Args:
        model_name: 模型名称
        api_key: API密钥
        dimension: 向量维度（仅用于v3模型）

    Returns:
        TextEmbedder实例
    """
    return TextEmbedder(model_name=model_name, api_key=api_key, dimension=dimension)


if __name__ == "__main__":
    # 简单的测试代码
    try:
        embedder = create_embedder()
        test_text = "这是一个测试文本，用于验证向量化服务是否正常工作。"
        vector = embedder.embed_text(test_text)
        print(f"Successfully generated embedding with dimension: {len(vector)}")
        print(f"Sample values: {vector[:5]}...")
    except Exception as e:
        print(f"Test failed with error: {str(e)}")