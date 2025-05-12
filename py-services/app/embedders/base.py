import logging
from abc import ABC, abstractmethod
from typing import List, Dict, Any, Optional, Union
import time

# 初始化日志记录器
logger = logging.getLogger(__name__)

class BaseEmbedder(ABC):
    """嵌入模型的基类，所有具体嵌入模型实现都应继承此类"""

    def __init__(self, model_name: str = "default", dimension: int = 1536, **kwargs):
        """
        初始化嵌入器基类

        参数:
            model_name: 模型名称
            dimension: 向量维度
            **kwargs: 其他配置参数
        """
        self.model_name = model_name
        self.dimension = dimension
        self.batch_size = kwargs.get("batch_size", 16)
        self.max_retries = kwargs.get("max_retries", 3)
        self.retry_delay = kwargs.get("retry_delay", 1.0)
        self.timeout = kwargs.get("timeout", 30.0)
        self.logger = logger

    @abstractmethod
    def embed(self, text: str) -> List[float]:
        """
        将单个文本转换为嵌入向量

        参数:
            text: 要嵌入的文本

        返回:
            List[float]: 嵌入向量
        """
        pass

    @abstractmethod
    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """
        批量将文本转换为嵌入向量

        参数:
            texts: 要嵌入的文本列表

        返回:
            List[List[float]]: 嵌入向量列表
        """
        pass

    def get_model_name(self) -> str:
        """
        获取模型名称

        返回:
            str: 模型名称
        """
        return self.model_name

    def get_dimension(self) -> int:
        """
        获取向量维度

        返回:
            int: 向量维度
        """
        return self.dimension

    def _batch_generator(self, texts: List[str]) -> List[List[str]]:
        """
        将文本列表分批

        参数:
            texts: 要分批的文本列表

        返回:
            List[List[str]]: 分批后的文本列表
        """
        for i in range(0, len(texts), self.batch_size):
            yield texts[i:i+self.batch_size]

    def _retry_with_backoff(self, func, *args, **kwargs):
        """
        使用退避重试策略执行函数

        参数:
            func: 要执行的函数
            *args, **kwargs: 函数参数

        返回:
            函数返回值

        异常:
            Exception: 如果所有重试都失败，则抛出最后一个异常
        """
        retries = 0
        last_exception = None

        while retries < self.max_retries:
            try:
                return func(*args, **kwargs)
            except Exception as e:
                last_exception = e
                retries += 1
                if retries < self.max_retries:
                    delay = self.retry_delay * (2 ** (retries - 1))  # 指数退避
                    self.logger.warning(f"Retry {retries}/{self.max_retries} after {delay:.2f}s due to: {str(e)}")
                    time.sleep(delay)

        # 所有重试都失败
        self.logger.error(f"All {self.max_retries} retries failed: {str(last_exception)}")
        raise last_exception

    def embed_with_metadata(self, text: str, metadata: Dict[str, Any] = None) -> Dict[str, Any]:
        """
        将单个文本转换为嵌入向量，并包含元数据

        参数:
            text: 要嵌入的文本
            metadata: 相关元数据

        返回:
            Dict[str, Any]: 包含嵌入向量和元数据的字典
        """
        vector = self.embed(text)
        result = {
            "embedding": vector,
            "dimension": len(vector),
            "model": self.model_name
        }

        # 添加元数据（如果提供）
        if metadata:
            result["metadata"] = metadata

        return result

    def embed_batch_with_metadata(self, texts: List[str], metadata_list: Optional[List[Dict[str, Any]]] = None) -> List[Dict[str, Any]]:
        """
        批量将文本转换为嵌入向量，并包含元数据

        参数:
            texts: 要嵌入的文本列表
            metadata_list: 每个文本对应的元数据列表

        返回:
            List[Dict[str, Any]]: 包含嵌入向量和元数据的字典列表
        """
        vectors = self.embed_batch(texts)
        results = []

        for i, vector in enumerate(vectors):
            result = {
                "embedding": vector,
                "dimension": len(vector),
                "model": self.model_name,
                "index": i
            }

            # 添加元数据（如果提供）
            if metadata_list and i < len(metadata_list):
                result["metadata"] = metadata_list[i]

            results.append(result)

        return results

    def validate_inputs(self, texts: Union[str, List[str]]) -> List[str]:
        """
        验证输入文本并进行预处理

        参数:
            texts: 单个文本或文本列表

        返回:
            List[str]: 处理后的文本列表

        异常:
            ValueError: 如果输入无效
        """
        # 转换单个文本为列表
        if isinstance(texts, str):
            texts = [texts]

        # 验证文本列表
        if not texts:
            raise ValueError("Input texts cannot be empty")

        # 检查列表中是否有无效项（None、空字符串或非字符串）
        for i, text in enumerate(texts):
            if not text or not isinstance(text, str):
                raise ValueError(f"Invalid text at position {i}: text must be a non-empty string")

        # 检查并截断过长文本
        valid_texts = []
        max_length = 8192  # 通义千问的最大长度限制
        for i, text in enumerate(texts):
            if len(text) > max_length:
                self.logger.warning(f"Text at index {i} exceeds maximum length of {max_length}. Truncating.")
                text = text[:max_length]
            valid_texts.append(text)

        return valid_texts

    def normalize_vectors(self, vectors: List[List[float]]) -> List[List[float]]:
        """
        标准化向量（使其长度为1）

        参数:
            vectors: 向量列表

        返回:
            List[List[float]]: 标准化后的向量列表
        """
        import numpy as np

        normalized = []
        for vector in vectors:
            # 转换为numpy数组
            v = np.array(vector)
            # 计算L2范数
            norm = np.linalg.norm(v)
            # 避免除零错误
            if norm > 0:
                normalized.append((v / norm).tolist())
            else:
                normalized.append(vector)  # 如果是零向量，保持不变

        return normalized

    def cosine_similarity(self, vec1: List[float], vec2: List[float]) -> float:
        """
        计算两个向量间的余弦相似度

        参数:
            vec1: 第一个向量
            vec2: 第二个向量

        返回:
            float: 余弦相似度 (0-1)
        """
        import numpy as np

        # 转换为numpy数组
        v1 = np.array(vec1)
        v2 = np.array(vec2)

        # 计算余弦相似度
        dot_product = np.dot(v1, v2)
        norm_v1 = np.linalg.norm(v1)
        norm_v2 = np.linalg.norm(v2)

        # 避免除零错误
        if norm_v1 == 0 or norm_v2 == 0:
            return 0.0

        similarity = dot_product / (norm_v1 * norm_v2)

        # 确保结果在0-1范围内
        return float(max(0.0, min(1.0, similarity)))