package taskqueue

import (
	"encoding/json"
	"time"
)

// TaskType 任务类型
type TaskType string

const (
	// TaskDocumentParse 文档解析任务
	TaskDocumentParse TaskType = "document_parse"
	// TaskTextChunk 文本分块任务
	TaskTextChunk TaskType = "text_chunk"
	// TaskVectorize 文本向量化任务
	TaskVectorize TaskType = "vectorize"
	// TaskProcessComplete 文档处理完整流程任务
	TaskProcessComplete TaskType = "process_complete"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	// StatusPending 等待处理
	StatusPending TaskStatus = "pending"
	// StatusProcessing 处理中
	StatusProcessing TaskStatus = "processing"
	// StatusCompleted 已完成
	StatusCompleted TaskStatus = "completed"
	// StatusFailed 处理失败
	StatusFailed TaskStatus = "failed"
)

// Task 任务基础结构
type Task struct {
	ID          string          `json:"id"`           // 任务唯一标识符
	Type        TaskType        `json:"type"`         // 任务类型
	DocumentID  string          `json:"document_id"`  // 关联的文档ID
	Status      TaskStatus      `json:"status"`       // 任务状态
	Payload     json.RawMessage `json:"payload"`      // 任务载荷数据，不同任务类型对应不同结构
	Result      json.RawMessage `json:"result"`       // 任务结果数据，不同任务类型对应不同结构
	Error       string          `json:"error"`        // 错误信息（如果处理失败）
	CreatedAt   time.Time       `json:"created_at"`   // 创建时间
	UpdatedAt   time.Time       `json:"updated_at"`   // 更新时间
	StartedAt   *time.Time      `json:"started_at"`   // 开始处理时间
	CompletedAt *time.Time      `json:"completed_at"` // 完成时间
	Attempts    int             `json:"attempts"`     // 尝试次数
	MaxRetries  int             `json:"max_retries"`  // 最大重试次数
}

// DocumentParsePayload 文档解析任务载荷
type DocumentParsePayload struct {
	FilePath string            `json:"file_path"` // 文件存储路径
	FileName string            `json:"file_name"` // 文件名
	FileType string            `json:"file_type"` // 文件类型
	Metadata map[string]string `json:"metadata"`  // 元数据
}

// DocumentParseResult 文档解析任务结果
type DocumentParseResult struct {
	Content string            `json:"content"` // 解析后的文本内容
	Title   string            `json:"title"`   // 文档标题（如果有）
	Meta    map[string]string `json:"meta"`    // 提取的元数据
	Error   string            `json:"error"`   // 错误信息（如果有）
	Pages   int               `json:"pages"`   // 文档页数（如果适用）
	Words   int               `json:"words"`   // 单词数
	Chars   int               `json:"chars"`   // 字符数
}

// TextChunkPayload 文本分块任务载荷
type TextChunkPayload struct {
	DocumentID string `json:"document_id"` // 文档ID
	Content    string `json:"content"`     // 文本内容
	ChunkSize  int    `json:"chunk_size"`  // 分块大小
	Overlap    int    `json:"overlap"`     // 重叠大小
	SplitType  string `json:"split_type"`  // 分割类型: paragraph, sentence, length
}

// ChunkInfo 分块信息
type ChunkInfo struct {
	Text  string `json:"text"`  // 分块文本
	Index int    `json:"index"` // 分块索引
}

// TextChunkResult 文本分块任务结果
type TextChunkResult struct {
	DocumentID string      `json:"document_id"` // 文档ID
	Chunks     []ChunkInfo `json:"chunks"`      // 分块列表
	ChunkCount int         `json:"chunk_count"` // 分块数量
	Error      string      `json:"error"`       // 错误信息（如果有）
}

// VectorizePayload 文本向量化任务载荷
type VectorizePayload struct {
	DocumentID string      `json:"document_id"` // 文档ID
	Chunks     []ChunkInfo `json:"chunks"`      // 文本分块
	Model      string      `json:"model"`       // 嵌入模型名称
}

// VectorInfo 向量信息
type VectorInfo struct {
	ChunkIndex int       `json:"chunk_index"` // 分块索引
	Vector     []float32 `json:"vector"`      // 向量数据
}

// VectorizeResult 向量化任务结果
type VectorizeResult struct {
	DocumentID  string       `json:"document_id"`  // 文档ID
	Vectors     []VectorInfo `json:"vectors"`      // 向量列表
	VectorCount int          `json:"vector_count"` // 向量数量
	Model       string       `json:"model"`        // 使用的模型
	Dimension   int          `json:"dimension"`    // 向量维度
	Error       string       `json:"error"`        // 错误信息（如果有）
}

// ProcessCompletePayload 完整处理流程任务载荷
type ProcessCompletePayload struct {
	DocumentID string            `json:"document_id"` // 文档ID
	FilePath   string            `json:"file_path"`   // 文件路径
	FileName   string            `json:"file_name"`   // 文件名
	FileType   string            `json:"file_type"`   // 文件类型
	ChunkSize  int               `json:"chunk_size"`  // 分块大小
	Overlap    int               `json:"overlap"`     // 重叠大小
	SplitType  string            `json:"split_type"`  // 分割类型
	Model      string            `json:"model"`       // 嵌入模型
	Metadata   map[string]string `json:"metadata"`    // 元数据
}

// ProcessCompleteResult 完整处理流程结果
type ProcessCompleteResult struct {
	DocumentID   string       `json:"document_id"`   // 文档ID
	ChunkCount   int          `json:"chunk_count"`   // 分块数量
	VectorCount  int          `json:"vector_count"`  // 向量数量
	Dimension    int          `json:"dimension"`     // 向量维度
	ParseStatus  string       `json:"parse_status"`  // 解析状态
	ChunkStatus  string       `json:"chunk_status"`  // 分块状态
	VectorStatus string       `json:"vector_status"` // 向量化状态
	Error        string       `json:"error"`         // 错误信息（如果有）
	Vectors      []VectorInfo `json:"vectors"`       // 可选，根据配置决定是否返回向量数据
}

// TaskCallback 任务回调信息
type TaskCallback struct {
	TaskID     string          `json:"task_id"`     // 任务ID
	DocumentID string          `json:"document_id"` // 文档ID
	Status     TaskStatus      `json:"status"`      // 任务状态
	Type       TaskType        `json:"type"`        // 任务类型
	Result     json.RawMessage `json:"result"`      // 任务结果
	Error      string          `json:"error"`       // 错误信息
	Timestamp  time.Time       `json:"timestamp"`   // 回调时间戳
}
