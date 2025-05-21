import logging
import time
from abc import ABC, abstractmethod
from typing import Dict, List, Any, Optional, Union, Callable, Generator
import json

# 初始化日志记录器
logger = logging.getLogger(__name__)

class BaseLLM(ABC):
    """LLM模型的基类，所有具体LLM模型实现都应继承此类"""

    def __init__(
            self, 
            model_name: str = "default", 
            api_key: Optional[str] = None,
            temperature: float = 0.7,
            max_tokens: int = 4096,
            **kwargs
        ):
        """
        初始化LLM基类

        参数:
            model_name: 模型名称
            api_key: API密钥
            temperature: 温度参数，控制随机性
            max_tokens: 生成的最大令牌数
            **kwargs: 其他配置参数
        """
        self.model_name = model_name
        self.api_key = api_key
        self.temperature = temperature
        self.max_tokens = max_tokens
        self.max_retries = kwargs.get("max_retries", 3)
        self.retry_delay = kwargs.get("retry_delay", 1.0)
        self.timeout = kwargs.get("timeout", 60.0)
        self.streaming = kwargs.get("streaming", False)
        self.logger = logger
        
        # 保存其他配置参数
        self.kwargs = kwargs
        
        self.logger.info(f"Initialized {self.__class__.__name__} with model: {model_name}")

    @abstractmethod
    def generate(self, prompt: str, **kwargs) -> str:
        """
        生成文本回复

        参数:
            prompt: 提示文本
            **kwargs: 其他参数

        返回:
            str: 生成的文本回复
        """
        pass

    @abstractmethod
    def generate_stream(self, prompt: str, **kwargs) -> Generator[str, None, None]:
        """
        流式生成文本回复

        参数:
            prompt: 提示文本
            **kwargs: 其他参数

        返回:
            Generator[str, None, None]: 生成的文本片段流
        """
        pass
    
    @abstractmethod
    def chat(self, messages: List[Dict[str, str]], **kwargs) -> str:
        """
        基于消息历史生成回复

        参数:
            messages: 消息历史列表，格式为[{"role": "user", "content": "Hello"}, ...]
            **kwargs: 其他参数

        返回:
            str: 生成的回复文本
        """
        pass

    @abstractmethod
    def chat_stream(self, messages: List[Dict[str, str]], **kwargs) -> Generator[str, None, None]:
        """
        流式生成对话回复

        参数:
            messages: 消息历史列表
            **kwargs: 其他参数

        返回:
            Generator[str, None, None]: 生成的回复文本片段流
        """
        pass

    def _retry_with_backoff(self, func: Callable, *args, **kwargs) -> Any:
        """
        使用退避重试策略执行函数

        参数:
            func: 要执行的函数
            *args, **kwargs: 函数参数

        返回:
            Any: 函数返回值

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

    def get_model_name(self) -> str:
        """
        获取模型名称

        返回:
            str: 模型名称
        """
        return self.model_name

    def get_model_parameters(self) -> Dict[str, Any]:
        """
        获取模型参数

        返回:
            Dict[str, Any]: 模型参数
        """
        return {
            "model": self.model_name,
            "temperature": self.temperature,
            "max_tokens": self.max_tokens,
            **{k: v for k, v in self.kwargs.items() if not k.startswith('_')}
        }

    def format_prompt(self, prompt: str, **kwargs) -> str:
        """
        格式化提示文本

        参数:
            prompt: 原始提示文本
            **kwargs: 格式化参数

        返回:
            str: 格式化后的提示文本
        """
        # 简单的参数替换
        if kwargs:
            return prompt.format(**kwargs)
        return prompt

    def format_chat_messages(self, messages: List[Dict[str, str]]) -> List[Dict[str, str]]:
        """
        格式化聊天消息列表

        参数:
            messages: 原始消息列表

        返回:
            List[Dict[str, str]]: 格式化后的消息列表
        """
        # 确保消息格式正确
        formatted_messages = []
        for msg in messages:
            if not isinstance(msg, dict):
                self.logger.warning(f"Invalid message format: {msg}, expected dict")
                continue
                
            role = msg.get("role", "").lower()
            content = msg.get("content", "")
            
            # 验证必要字段
            if not role or not content:
                self.logger.warning(f"Missing role or content in message: {msg}")
                continue
                
            # 规范化角色名称
            if role not in ["system", "user", "assistant"]:
                role = "user"  # 默认为用户
                
            formatted_messages.append({"role": role, "content": content})
            
        return formatted_messages

    def validate_api_key(self) -> bool:
        """
        验证API密钥是否有效

        返回:
            bool: API密钥有效返回True，否则False
        """
        # 基类提供一个简单的检查，子类可以扩展此功能
        return bool(self.api_key)
        
    def call_with_timeout(self, func: Callable, timeout: Optional[float] = None, *args, **kwargs) -> Any:
        """
        在指定超时时间内执行函数

        参数:
            func: 要执行的函数
            timeout: 超时时间(秒)，默认使用实例的timeout属性
            *args, **kwargs: 函数参数

        返回:
            Any: 函数返回值

        异常:
            TimeoutError: 如果函数执行超时
        """
        import threading
        import concurrent.futures
        
        if timeout is None:
            timeout = self.timeout
            
        with concurrent.futures.ThreadPoolExecutor(max_workers=1) as executor:
            future = executor.submit(func, *args, **kwargs)
            try:
                return future.result(timeout=timeout)
            except concurrent.futures.TimeoutError:
                self.logger.error(f"Function {func.__name__} timed out after {timeout} seconds")
                raise TimeoutError(f"LLM API call timed out after {timeout} seconds")

    def trim_messages_to_max_tokens(self, messages: List[Dict[str, str]], max_tokens: Optional[int] = None) -> List[Dict[str, str]]:
        """
        裁剪消息历史以符合最大令牌数限制

        参数:
            messages: 消息历史列表
            max_tokens: 最大令牌数，如果为None则使用实例的max_tokens属性

        返回:
            List[Dict[str, str]]: 裁剪后的消息列表
        """
        # TODO: 实现基于token计数的消息裁剪
        # 此功能需要依赖具体的tokenizer实现
        self.logger.warning("Message trimming not implemented in base class")
        return messages

    def wrap_prompt_with_context(self, prompt: str, context: str, template: Optional[str] = None) -> str:
        """
        将提示与上下文结合

        参数:
            prompt: 用户提示
            context: 上下文信息
            template: 可选的模板字符串

        返回:
            str: 结合后的提示
        """
        if template:
            return template.format(context=context, question=prompt)
            
        # 默认模板
        return (
            f"Context information is below.\n"
            f"---------------------\n"
            f"{context}\n"
            f"---------------------\n"
            f"Given the context information and not prior knowledge, "
            f"answer the question: {prompt}"
        )