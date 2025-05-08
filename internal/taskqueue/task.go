package taskqueue

import (
	"time"

	"github.com/google/uuid"
)

// 任务状态常量
const (
	TaskStatusPending    = "pending"    // 等待处理
	TaskStatusProcessing = "processing" // 处理中
	TaskStatusCompleted  = "completed"  // 完成
	TaskStatusFailed     = "failed"     // 失败
)

// 任务类型常量
const (
	TaskTypeDocumentProcess   = "document.process"   // 文档处理任务
	TaskTypeEmbeddingGenerate = "embedding.generate" // 生成嵌入向量任务
)

// Task 表示一个异步处理任务
type Task struct {
	ID        string                 `json:"id"`                 // 任务唯一标识符
	Type      string                 `json:"type"`               // 任务类型
	FileID    string                 `json:"file_id"`            // 文件标识符
	FilePath  string                 `json:"file_path"`          // 文件路径
	Status    string                 `json:"status"`             // 任务状态
	Result    map[string]interface{} `json:"result,omitempty"`   // 任务结果
	Error     string                 `json:"error,omitempty"`    // 错误信息
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // 元数据
	CreatedAt time.Time              `json:"created_at"`         // 创建时间
	UpdatedAt time.Time              `json:"updated_at"`         // 更新时间
}

// NewTask 创建一个新的任务
func NewTask(taskType, fileID, filePath string, metadata map[string]interface{}) *Task {
	now := time.Now()
	return &Task{
		ID:        uuid.New().String(), // 使用UUID生成唯一ID
		Type:      taskType,
		FileID:    fileID,
		FilePath:  filePath,
		Status:    TaskStatusPending, // 初始状态为等待处理
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// SetStatus 设置任务状态
func (t *Task) SetStatus(status string) {
	t.Status = status
	t.UpdatedAt = time.Now()
}

// SetResult 设置任务结果
func (t *Task) SetResult(result map[string]interface{}) {
	t.Result = result
	t.Status = TaskStatusCompleted
	t.UpdatedAt = time.Now()
}

// SetError 设置错误信息
func (t *Task) SetError(err error) {
	if err != nil {
		t.Error = err.Error()
		t.Status = TaskStatusFailed
		t.UpdatedAt = time.Now()
	}
}

// IsCompleted 检查任务是否完成
func (t *Task) IsCompleted() bool {
	return t.Status == TaskStatusCompleted
}

// IsFailed 检查任务是否失败
func (t *Task) IsFailed() bool {
	return t.Status == TaskStatusFailed
}

// IsPending 检查任务是否等待处理
func (t *Task) IsPending() bool {
	return t.Status == TaskStatusPending
}

// IsProcessing 检查任务是否处理中
func (t *Task) IsProcessing() bool {
	return t.Status == TaskStatusProcessing
}
