from dataclasses import dataclass, field
from enum import Enum
from typing import Dict, List, Optional, Any, Union
import json
from datetime import datetime


class TaskType(str, Enum):
    """任务类型枚举，对应Go中的TaskType"""
    DOCUMENT_PARSE = "document_parse"
    TEXT_CHUNK = "text_chunk"
    VECTORIZE = "vectorize"
    PROCESS_COMPLETE = "process_complete"


class TaskStatus(str, Enum):
    """任务状态枚举，对应Go中的TaskStatus"""
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"


@dataclass
class Task:
    """任务基础结构，对应Go中的Task"""
    id: str
    type: TaskType
    document_id: str
    status: TaskStatus
    payload: dict = field(default_factory=dict)
    result: dict = field(default_factory=dict)
    error: str = ""
    created_at: datetime = field(default_factory=datetime.now)
    updated_at: datetime = field(default_factory=datetime.now)
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None
    attempts: int = 0
    max_retries: int = 3

    @classmethod
    def from_json(cls, json_data: Union[str, bytes]) -> 'Task':
        """从JSON字符串创建Task对象"""
        data = json.loads(json_data) if isinstance(json_data, (str, bytes)) else json_data

        # 处理日期字段
        for date_field in ['created_at', 'updated_at', 'started_at', 'completed_at']:
            if date_field in data and data[date_field] and isinstance(data[date_field], str):
                data[date_field] = datetime.fromisoformat(data[date_field].replace('Z', '+00:00'))

        # 转换枚举类型
        if 'type' in data:
            data['type'] = TaskType(data['type'])
        if 'status' in data:
            data['status'] = TaskStatus(data['status'])

        return cls(**data)

    def to_json(self) -> str:
        """将Task对象转换为JSON字符串"""
        data = self.__dict__.copy()

        # 处理枚举类型
        if isinstance(data['type'], TaskType):
            data['type'] = data['type'].value
        if isinstance(data['status'], TaskStatus):
            data['status'] = data['status'].value

        # 处理日期字段
        for k, v in data.items():
            if isinstance(v, datetime):
                data[k] = v.isoformat()

        return json.dumps(data)


@dataclass
class DocumentParsePayload:
    """文档解析任务载荷，对应Go中的DocumentParsePayload"""
    file_path: str
    file_name: str
    file_type: str
    metadata: Dict[str, str] = field(default_factory=dict)


@dataclass
class DocumentParseResult:
    """文档解析任务结果，对应Go中的DocumentParseResult"""
    content: str
    title: str = ""
    meta: Dict[str, str] = field(default_factory=dict)
    error: str = ""
    pages: int = 0
    words: int = 0
    chars: int = 0


@dataclass
class ChunkInfo:
    """分块信息，对应Go中的ChunkInfo"""
    text: str
    index: int


@dataclass
class TextChunkPayload:
    """文本分块任务载荷，对应Go中的TextChunkPayload"""
    document_id: str
    content: str
    chunk_size: int
    overlap: int
    split_type: str  # paragraph, sentence, length


@dataclass
class TextChunkResult:
    """文本分块任务结果，对应Go中的TextChunkResult"""
    document_id: str
    chunks: List[ChunkInfo] = field(default_factory=list)
    chunk_count: int = 0
    error: str = ""


@dataclass
class VectorizePayload:
    """文本向量化任务载荷，对应Go中的VectorizePayload"""
    document_id: str
    chunks: List[ChunkInfo]
    model: str


@dataclass
class VectorInfo:
    """向量信息，对应Go中的VectorInfo"""
    chunk_index: int
    vector: List[float]


@dataclass
class VectorizeResult:
    """向量化任务结果，对应Go中的VectorizeResult"""
    document_id: str
    vectors: List[VectorInfo] = field(default_factory=list)
    vector_count: int = 0
    model: str = ""
    dimension: int = 0
    error: str = ""


@dataclass
class ProcessCompletePayload:
    """完整处理流程任务载荷，对应Go中的ProcessCompletePayload"""
    document_id: str
    file_path: str
    file_name: str
    file_type: str
    chunk_size: int
    overlap: int
    split_type: str
    model: str
    metadata: Dict[str, str] = field(default_factory=dict)


@dataclass
class ProcessCompleteResult:
    """完整处理流程结果，对应Go中的ProcessCompleteResult"""
    document_id: str
    chunk_count: int = 0
    vector_count: int = 0
    dimension: int = 0
    parse_status: str = ""
    chunk_status: str = ""
    vector_status: str = ""
    error: str = ""
    vectors: List[VectorInfo] = field(default_factory=list)


@dataclass
class TaskCallback:
    """任务回调信息，对应Go中的TaskCallback"""
    task_id: str
    document_id: str
    status: TaskStatus
    type: TaskType
    result: dict
    error: str
    timestamp: datetime = field(default_factory=datetime.now)