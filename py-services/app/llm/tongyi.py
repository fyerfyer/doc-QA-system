import os
import logging
from typing import Dict, List, Any, Optional, Generator, Union

import dashscope
from dashscope.api_entities.dashscope_response import DashScopeAPIResponse
from http import HTTPStatus

from app.llm.base import BaseLLM
from app.llm.model import (
    TokenUsage, TextResponse,
    ChatResponse, LLMError, LLMErrorCode
)

# 初始化日志记录器
logger = logging.getLogger(__name__)

class TongyiLLM(BaseLLM):
    """通义千问大语言模型实现"""

    # 默认模型
    DEFAULT_MODEL = "qwen-turbo"

    def __init__(
            self,
            api_key: Optional[str] = None,
            model_name: str = DEFAULT_MODEL,
            temperature: float = 0.7,
            max_tokens: int = 2048,
            **kwargs
    ):
        """
        初始化通义千问LLM

        参数:
            api_key: 通义千问API密钥，如果为None则从环境变量获取
            model_name: 模型名称，默认为qwen-plus
            temperature: 温度参数，控制随机性
            max_tokens: 生成的最大token数
            **kwargs: 其他配置参数
        """
        # 初始化基类
        super().__init__(
            model_name=model_name,
            api_key=api_key,
            temperature=temperature,
            max_tokens=max_tokens,
            **kwargs
        )

        # 验证API密钥
        self.api_key = api_key or os.getenv("DASHSCOPE_API_KEY")
        if not self.api_key:
            raise ValueError("API key must be provided or set as DASHSCOPE_API_KEY environment variable")

        # 设置DashScope API密钥
        dashscope.api_key = self.api_key

        # 其他配置参数
        self.top_p = kwargs.get("top_p", 0.8)
        self.top_k = kwargs.get("top_k", 50)
        self.repetition_penalty = kwargs.get("repetition_penalty", 1.0)
        self.seed = kwargs.get("seed", None)
        self.result_format = kwargs.get("result_format", "message")  # 推荐使用message格式

        self.logger.info(f"Initialized Tongyi LLM with model: {model_name}")

    def generate(self, prompt: str, **kwargs) -> str:
        """
        生成文本回复

        参数:
            prompt: 提示文本
            **kwargs: 其他参数

        返回:
            str: 生成的文本回复
        """
        # 创建只包含用户消息的单条消息
        messages = [{"role": "user", "content": prompt}]
        
        # 调用chat方法，处理单条消息的情况
        return self.chat(messages, **kwargs)

    def generate_stream(self, prompt: str, **kwargs) -> Generator[str, None, None]:
        """
        流式生成文本回复

        参数:
            prompt: 提示文本
            **kwargs: 其他参数

        返回:
            Generator[str, None, None]: 生成的文本片段流
        """
        # 创建只包含用户消息的单条消息
        messages = [{"role": "user", "content": prompt}]
        
        # 调用chat_stream方法，处理单条消息的情况
        yield from self.chat_stream(messages, **kwargs)

    def chat(self, messages: List[Dict[str, str]], **kwargs) -> str:
        """
        基于消息历史生成回复

        参数:
            messages: 消息历史列表，格式为[{"role": "user", "content": "Hello"}, ...]
            **kwargs: 其他参数

        返回:
            str: 生成的回复文本
        """
        # 格式化消息
        formatted_messages = self.format_chat_messages(messages)

        # 合并默认参数和传入参数
        params = self._get_generation_parameters(**kwargs)
        
        try:
            # 通过retry_with_backoff执行API调用
            response = self._retry_with_backoff(self._call_chat_api, formatted_messages, params)
            
            # 处理响应
            return self._process_chat_response(response)
            
        except Exception as e:
            error_msg = f"Tongyi chat API call failed: {str(e)}"
            self.logger.error(error_msg)
            if "rate limit" in str(e).lower():
                raise LLMError(error_msg, LLMErrorCode.RATE_LIMIT_EXCEEDED)
            elif "quota" in str(e).lower():
                raise LLMError(error_msg, LLMErrorCode.QUOTA_EXCEEDED)
            else:
                raise LLMError(error_msg, LLMErrorCode.API_ERROR)

    def chat_stream(self, messages: List[Dict[str, str]], **kwargs) -> Generator[str, None, None]:
        """
        流式生成对话回复

        参数:
            messages: 消息历史列表
            **kwargs: 其他参数

        返回:
            Generator[str, None, None]: 生成的回复文本片段流
        """
        # 格式化消息
        formatted_messages = self.format_chat_messages(messages)

        # 设置流式输出参数
        kwargs["stream"] = True
        # 默认开启增量输出
        kwargs.setdefault("incremental_output", True)
        
        # 合并默认参数和传入参数
        params = self._get_generation_parameters(**kwargs)
        
        try:
            # 调用流式API
            stream_response = dashscope.Generation.call(
                model=self.model_name,
                messages=formatted_messages,
                **params
            )
            
            # 处理流式响应
            for chunk in stream_response:
                # 提取文本内容
                if chunk.status_code == HTTPStatus.OK:
                    if params.get("incremental_output", True):
                        # 增量模式：每次只返回新生成的部分
                        if chunk.output and chunk.output.choices:
                            content = chunk.output.choices[0].message.content
                            if content and content.strip():
                                yield content
                    else:
                        # 非增量模式：返回累积内容
                        if chunk.output and chunk.output.choices:
                            content = chunk.output.choices[0].message.content
                            if content and content.strip():
                                yield content
                else:
                    error_msg = f"Stream error: {chunk.code} - {chunk.message}"
                    self.logger.error(error_msg)
                    # 在流中也反馈错误信息
                    yield f"Error: {error_msg}"
                    break
                
        except Exception as e:
            error_msg = f"Tongyi stream chat API call failed: {str(e)}"
            self.logger.error(error_msg)
            # 在流中反馈错误信息
            yield f"Stream error: {str(e)}"
            # 流式输出不抛出异常，而是作为流的一部分返回错误

    def _call_chat_api(self, messages: List[Dict[str, str]], params: Dict[str, Any]) -> Any:
        """
        调用通义千问聊天API

        参数:
            messages: 格式化后的消息列表
            params: API参数

        返回:
            Any: API响应
        """
        # 调用API生成回复
        response = dashscope.Generation.call(
            model=self.model_name,
            messages=messages,
            **params
        )
        
        # 检查响应状态
        if response.status_code != HTTPStatus.OK:
            raise LLMError(
                f"API call failed with status {response.status_code}: {response.code} - {response.message}",
                LLMErrorCode.API_ERROR
            )
            
        return response

    def _process_chat_response(self, response: DashScopeAPIResponse) -> str:
        """
        处理聊天API响应

        参数:
            response: API响应对象

        返回:
            str: 提取的回复文本
        """
        if not response.output or not response.output.choices:
            raise LLMError(
                "No output in response",
                LLMErrorCode.API_ERROR
            )
            
        # 根据响应格式提取文本
        try:
            if hasattr(response.output.choices[0], 'message'):
                # message格式
                return response.output.choices[0].message.content
            elif hasattr(response.output, 'text'):
                # text格式
                return response.output.text
            else:
                # 尝试作为一般对象处理
                return response.output.choices[0].get('message', {}).get('content', '')
        except Exception as e:
            raise LLMError(
                f"Failed to extract text from response: {str(e)}",
                LLMErrorCode.API_ERROR
            )

    def _get_generation_parameters(self, **kwargs) -> Dict[str, Any]:
        """
        获取生成参数

        参数:
            **kwargs: 覆盖默认参数的额外参数

        返回:
            Dict[str, Any]: 合并后的参数
        """
        # 基本参数
        params = {
            "temperature": kwargs.get("temperature", self.temperature),
            "top_p": kwargs.get("top_p", self.top_p),
            "top_k": kwargs.get("top_k", self.top_k),
            "repetition_penalty": kwargs.get("repetition_penalty", self.repetition_penalty),
            "max_tokens": kwargs.get("max_tokens", self.max_tokens),
            "result_format": kwargs.get("result_format", self.result_format),
            "stream": kwargs.get("stream", False),
        }
        
        # 可选参数
        if self.seed is not None:
            params["seed"] = kwargs.get("seed", self.seed)
        
        if "stop" in kwargs and kwargs["stop"]:
            params["stop"] = kwargs["stop"]
            
        # 增量输出选项（流式时使用）
        if kwargs.get("stream", False):
            params["incremental_output"] = kwargs.get("incremental_output", True)
            
        # 处理思考模式选项 (Qwen3系列模型支持)
        if "enable_thinking" in kwargs:
            params["enable_thinking"] = kwargs["enable_thinking"]
            
        if "thinking_budget" in kwargs:
            params["thinking_budget"] = kwargs["thinking_budget"]
            
        # 处理结构化输出选项
        if kwargs.get("json_output", False):
            params["response_format"] = {"type": "json_object"}
            
        return params

    def create_rich_response(self, response: DashScopeAPIResponse, request_id: str = None) -> Union[TextResponse, ChatResponse]:
        """
        创建结构化响应对象

        参数:
            response: API响应
            request_id: 请求ID

        返回:
            Union[TextResponse, ChatResponse]: 结构化响应对象
        """
        # 提取文本内容
        text = self._process_chat_response(response)
        
        # 提取token使用情况
        usage = TokenUsage(
            prompt_tokens=response.usage.input_tokens,
            completion_tokens=response.usage.output_tokens,
            total_tokens=response.usage.total_tokens
        )
        
        # 提取结束原因
        finish_reason = response.output.choices[0].finish_reason if response.output.choices else None
        
        # 创建响应对象
        return TextResponse(
            text=text,
            model=self.model_name,
            usage=usage,
            finish_reason=finish_reason,
            request_id=request_id
        )

    def validate_api_key(self) -> bool:
        """
        验证API密钥是否有效

        返回:
            bool: API密钥有效返回True，否则False
        """
        try:
            # 发送简单请求测试API密钥
            test_messages = [{"role": "user", "content": "Hello"}]
            test_params = {
                "model": self.model_name,
                "messages": test_messages,
                "max_tokens": 10
            }
            
            response = dashscope.Generation.call(**test_params)
            return response.status_code == HTTPStatus.OK
        except Exception as e:
            self.logger.warning(f"API key validation failed: {str(e)}")
            return False

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
            "top_p": self.top_p,
            "top_k": self.top_k,
            "repetition_penalty": self.repetition_penalty,
        }