import logging
from typing import List, Dict, Any, Optional
import torch

from app.embedders.base import BaseEmbedder

# 初始化日志记录器
logger = logging.getLogger(__name__)

class HuggingFaceEmbedder(BaseEmbedder):
    """使用Hugging Face模型的文本嵌入器实现"""

    def __init__(
            self,
            model_name: str = "sentence-transformers/all-MiniLM-L6-v2",
            device: Optional[str] = None,
            proxies: Optional[Dict[str, str]] = None,  # 添加代理参数
            local_files_only: bool = False,  # 添加离线模式参数
            **kwargs
    ):
        """
        初始化Hugging Face嵌入器

        参数:
            model_name: 模型名称或路径，默认使用all-MiniLM-L6-v2
            device: 计算设备 ('cpu', 'cuda', 'cuda:0' 等)
            proxies: 代理设置，格式如 {'http': 'http://127.0.0.1:7897', 'https': 'http://127.0.0.1:7897'}
            local_files_only: 是否只使用本地文件（离线模式）
            **kwargs: 其他配置参数
        """
        # 设置默认维度 (可能会在加载模型后更新)
        dimension = kwargs.pop("dimension", 384)  # all-MiniLM-L6-v2的默认维度是384

        # 初始化基类
        super().__init__(model_name=model_name, dimension=dimension, **kwargs)

        # 确定计算设备
        if device is None:
            self.device = "cuda" if torch.cuda.is_available() else "cpu"
        else:
            self.device = device

        # 保存代理设置和离线模式设置
        self.proxies = proxies
        self.local_files_only = local_files_only

        # 标记模型尚未加载
        self.model = None

        # 延迟加载模型，首次使用时才加载
        self._model_loaded = False

        self.logger.info(f"Initialized HuggingFace embedder with model: {model_name}, will load on first use")

    def _ensure_model_loaded(self):
        """确保模型已加载，如果尚未加载则加载模型"""
        if not self._model_loaded:
            try:
                self._load_model()
                self._model_loaded = True
            except Exception as e:
                self.logger.error(f"Failed to load model: {str(e)}")
                raise

    def _load_model(self):
        """加载Hugging Face模型"""
        try:
            from sentence_transformers import SentenceTransformer
            import os

            # 如果有代理设置，设置环境变量
            original_http_proxy = os.environ.get('HTTP_PROXY')
            original_https_proxy = os.environ.get('HTTPS_PROXY')
            original_hf_offline = os.environ.get('TRANSFORMERS_OFFLINE')

            try:
                # 设置代理环境变量
                if self.proxies:
                    if 'http' in self.proxies:
                        os.environ['HTTP_PROXY'] = self.proxies['http']
                    if 'https' in self.proxies:
                        os.environ['HTTPS_PROXY'] = self.proxies['https']

                # 设置离线模式环境变量
                if self.local_files_only:
                    os.environ['TRANSFORMERS_OFFLINE'] = '1'
                    os.environ['HF_DATASETS_OFFLINE'] = '1'

                self.logger.info(f"Loading model '{self.model_name}' to {self.device}...")

                # SentenceTransformer不接受proxies参数，只接受device
                self.model = SentenceTransformer(
                    self.model_name,
                    device=self.device
                )

                # 更新嵌入维度为实际值
                self.dimension = self.model.get_sentence_embedding_dimension()

                self.logger.info(f"Model loaded successfully with embedding dimension: {self.dimension}")

            finally:
                # 恢复原始环境变量
                if self.proxies:
                    if original_http_proxy:
                        os.environ['HTTP_PROXY'] = original_http_proxy
                    elif 'HTTP_PROXY' in os.environ:
                        del os.environ['HTTP_PROXY']

                    if original_https_proxy:
                        os.environ['HTTPS_PROXY'] = original_https_proxy
                    elif 'HTTPS_PROXY' in os.environ:
                        del os.environ['HTTPS_PROXY']

                # 恢复离线模式环境变量
                if self.local_files_only:
                    if original_hf_offline:
                        os.environ['TRANSFORMERS_OFFLINE'] = original_hf_offline
                    elif 'TRANSFORMERS_OFFLINE' in os.environ:
                        del os.environ['TRANSFORMERS_OFFLINE']

                    if 'HF_DATASETS_OFFLINE' in os.environ:
                        del os.environ['HF_DATASETS_OFFLINE']

        except ImportError:
            self.logger.error("sentence-transformers package is required but not installed")
            raise ImportError("Please install sentence-transformers: pip install sentence-transformers")
        except Exception as e:
            self.logger.error(f"Failed to load model '{self.model_name}': {str(e)}")
            raise

    def embed(self, text: str) -> List[float]:
        """
        将单个文本转换为嵌入向量

        参数:
            text: 要嵌入的文本

        返回:
            List[float]: 嵌入向量
        """
        # 确保文本是字符串
        if not isinstance(text, str):
            raise ValueError("Input text must be a string")

        # 使用批量处理方法
        embeddings = self.embed_batch([text])
        return embeddings[0]

    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """
        批量将文本转换为嵌入向量

        参数:
            texts: 要嵌入的文本列表

        返回:
            List[List[float]]: 嵌入向量列表
        """
        # 确保模型已加载
        self._ensure_model_loaded()

        # 验证输入
        valid_texts = self.validate_inputs(texts)

        def _encode_batch(batch):
            try:
                # 使用sentence-transformers进行编码
                embeddings = self.model.encode(
                    batch,
                    show_progress_bar=False,
                    convert_to_numpy=True,
                    normalize_embeddings=True  # L2标准化嵌入向量
                )

                # 确保结果是二维数组
                if len(embeddings.shape) == 1:
                    embeddings = embeddings.reshape(1, -1)

                # 转换为Python列表
                return embeddings.tolist()

            except Exception as e:
                self.logger.error(f"Error encoding batch with HuggingFace model: {str(e)}")
                raise

        # 分批处理并合并结果
        all_embeddings = []
        for i in range(0, len(valid_texts), self.batch_size):
            batch = valid_texts[i:i+self.batch_size]

            self.logger.debug(f"Processing batch {i//self.batch_size + 1} with {len(batch)} texts")

            # 使用重试机制调用模型
            batch_embeddings = self._retry_with_backoff(_encode_batch, batch)
            all_embeddings.extend(batch_embeddings)

        return all_embeddings

    def get_model_info(self) -> Dict[str, Any]:
        """
        获取模型信息

        返回:
            Dict[str, Any]: 模型信息字典
        """
        info = {
            "model_name": self.model_name,
            "dimension": self.dimension,
            "device": self.device,
            "type": "huggingface",
            "normalize": True
        }

        # 如果模型已加载，添加额外信息
        if self._model_loaded and self.model is not None:
            info.update({
                "max_seq_length": getattr(self.model, "max_seq_length", 512),
                "model_loaded": True
            })
        else:
            info["model_loaded"] = False

        return info

    def get_supported_models(self) -> List[Dict[str, Any]]:
        """
        获取一些常用的Sentence Transformers模型列表

        返回:
            List[Dict[str, Any]]: 模型信息列表
        """
        return [
            {"name": "all-MiniLM-L6-v2", "dimension": 384, "description": "一个小型通用模型，速度快"},
            {"name": "all-mpnet-base-v2", "dimension": 768, "description": "一个效果更好的通用模型，但速度较慢"},
            {"name": "paraphrase-multilingual-MiniLM-L12-v2", "dimension": 384, "description": "多语言小型模型"},
            {"name": "distiluse-base-multilingual-cased-v1", "dimension": 512, "description": "多语言模型，支持50+语言"}
        ]

    def unload_model(self):
        """卸载模型以释放内存"""
        if self._model_loaded and self.model is not None:
            self.logger.info(f"Unloading model '{self.model_name}' from memory")
            self.model = None
            self._model_loaded = False

            # 强制执行垃圾回收
            import gc
            gc.collect()

            if torch.cuda.is_available():
                torch.cuda.empty_cache()