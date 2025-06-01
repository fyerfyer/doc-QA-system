from dataclasses import dataclass, field
from enum import Enum
from typing import Dict, List, Optional, Union, Any
import uuid


class Role(str, Enum):
    """消息角色枚举"""
    SYSTEM = "system"
    USER = "user" 
    ASSISTANT = "assistant"
    FUNCTION = "function"


@dataclass
class Message:
    """LLM对话消息结构"""
    role: str
    content: str
    name: Optional[str] = None
    
    def to_dict(self) -> Dict[str, str]:
        """转换为字典格式"""
        result = {
            "role": self.role,
            "content": self.content
        }
        if self.name:
            result["name"] = self.name
        return result


@dataclass
class GenerateRequest:
    """文本生成请求"""
    prompt: str
    model: Optional[str] = None
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    stop: Optional[List[str]] = None
    request_id: str = field(default_factory=lambda: str(uuid.uuid4()))


@dataclass
class ChatRequest:
    """聊天请求"""
    messages: List[Union[Message, Dict[str, str]]]
    model: Optional[str] = None
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    stop: Optional[List[str]] = None
    request_id: str = field(default_factory=lambda: str(uuid.uuid4()))

    def get_formatted_messages(self) -> List[Dict[str, str]]:
        """获取标准格式的消息列表"""
        formatted = []
        for msg in self.messages:
            if isinstance(msg, Message):
                formatted.append(msg.to_dict())
            elif isinstance(msg, dict):
                # 确保必要的字段
                if "role" not in msg or "content" not in msg:
                    raise ValueError("消息必须包含 'role' 和 'content' 字段")
                formatted.append(msg)
        return formatted


@dataclass
class TokenUsage:
    """Token使用统计"""
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


@dataclass
class TextResponse:
    """文本生成响应"""
    text: str
    model: str
    usage: TokenUsage = field(default_factory=TokenUsage)
    finish_reason: Optional[str] = None
    request_id: Optional[str] = None


@dataclass
class ChatResponse:
    """聊天响应"""
    message: Message
    model: str
    usage: TokenUsage = field(default_factory=TokenUsage)
    finish_reason: Optional[str] = None
    request_id: Optional[str] = None


@dataclass
class StreamChunk:
    """流式响应的数据块"""
    text: str
    index: int
    finish_reason: Optional[str] = None
    is_last: bool = False


@dataclass
class SearchResult:
    """向量搜索结果"""
    text: str
    score: float
    metadata: Dict[str, Any] = field(default_factory=dict)
    chunk_index: Optional[int] = None
    document_id: Optional[str] = None


@dataclass
class RAGRequest:
    """RAG检索增强生成请求"""
    query: str
    document_ids: Optional[List[str]] = None
    collection_name: Optional[str] = None
    top_k: int = 5
    model: Optional[str] = None
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    filter_metadata: Optional[Dict[str, Any]] = None
    context_prompt: Optional[str] = None
    contexts: Optional[List[str]] = None
    request_id: str = field(default_factory=lambda: str(uuid.uuid4()))


@dataclass
class RAGResponse:
    """RAG检索增强生成响应"""
    text: str
    sources: List[SearchResult]
    model: str
    usage: TokenUsage = field(default_factory=TokenUsage)
    finish_reason: Optional[str] = None
    request_id: Optional[str] = None


class LLMErrorCode(str, Enum):
    """LLM错误代码"""
    INVALID_REQUEST = "invalid_request"
    MODEL_NOT_FOUND = "model_not_found"
    CONTEXT_TOO_LARGE = "context_too_large"
    RATE_LIMIT_EXCEEDED = "rate_limit_exceeded"
    QUOTA_EXCEEDED = "quota_exceeded"
    SERVICE_UNAVAILABLE = "service_unavailable"
    TIMEOUT = "timeout"
    API_ERROR = "api_error"
    INTERNAL_ERROR = "internal_error"


@dataclass
class LLMError(Exception):
    """LLM错误，用于异常处理"""
    message: str
    code: LLMErrorCode = LLMErrorCode.INTERNAL_ERROR
    request_id: Optional[str] = None
    
    def __str__(self) -> str:
        return f"{self.code}: {self.message}"