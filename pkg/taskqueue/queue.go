package taskqueue

import (
	"context"
	"encoding/json"
	"time"
)

// Queue 定义任务队列的接口
// 负责任务的入队、获取状态和结果等操作
type Queue interface {
	// Enqueue 将任务加入队列
	Enqueue(ctx context.Context, taskType TaskType, documentID string, payload interface{}) (string, error)

	// EnqueueAt 在指定时间将任务加入队列
	EnqueueAt(ctx context.Context, taskType TaskType, documentID string, payload interface{}, processAt time.Time) (string, error)

	// EnqueueIn 在指定延迟后将任务加入队列
	EnqueueIn(ctx context.Context, taskType TaskType, documentID string, payload interface{}, delay time.Duration) (string, error)

	// GetTask 获取任务信息
	GetTask(ctx context.Context, taskID string) (*Task, error)

	// GetTasksByDocument 获取文档相关的所有任务
	GetTasksByDocument(ctx context.Context, documentID string) ([]*Task, error)

	// WaitForTask 等待任务完成并返回结果
	// timeout为0表示不设置超时
	WaitForTask(ctx context.Context, taskID string, timeout time.Duration) (*Task, error)

	// DeleteTask 删除任务
	DeleteTask(ctx context.Context, taskID string) error

	// UpdateTaskStatus 更新任务状态和结果
	UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus, result interface{}, errorMsg string) error

	// NotifyTaskUpdate 通知任务状态已更新
	NotifyTaskUpdate(ctx context.Context, taskID string) error

	// Close 关闭队列连接
	Close() error
}

// Handler 任务处理器接口
// 负责实际执行任务的逻辑
type Handler interface {
	// ProcessTask 处理任务
	ProcessTask(ctx context.Context, task *Task) error

	// GetTaskTypes 返回此处理器支持的任务类型
	GetTaskTypes() []TaskType
}

// TaskResult 表示任务执行的结果
// 通常由Handler返回，用于更新任务状态和结果
type TaskResult struct {
	Status  TaskStatus  // 任务状态
	Result  interface{} // 任务结果数据
	Error   string      // 错误信息
	Metrics TaskMetrics // 任务执行指标
}

// TaskMetrics 任务执行指标
type TaskMetrics struct {
	ProcessingTime time.Duration // 处理时间
	RetryCount     int           // 重试次数
	UsedMemory     int64         // 使用内存
}

// Worker 工作者接口
// 负责运行一组Handler来处理队列中的任务
type Worker interface {
	// RegisterHandler 注册任务处理器
	RegisterHandler(taskType TaskType, handler Handler)

	// Start 启动工作者，开始处理任务
	Start() error

	// Stop 停止工作者
	Stop()
}

// Config 队列配置
type Config struct {
	RedisAddr     string         // Redis地址
	RedisPassword string         // Redis密码
	RedisDB       int            // Redis数据库
	Concurrency   int            // 并发处理任务数
	RetryLimit    int            // 最大重试次数
	RetryDelay    time.Duration  // 重试延迟
	Queues        map[string]int // 队列名称到优先级的映射
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		RedisAddr:   "localhost:6379",
		RedisDB:     0,
		Concurrency: 10,
		RetryLimit:  3,
		RetryDelay:  time.Minute,
		Queues: map[string]int{
			"critical": 6, // 关键任务
			"default":  3, // 默认任务
			"low":      1, // 低优先级任务
		},
	}
}

// TaskInfo 表示任务的元信息
// 用于传递给客户端的简化任务信息
type TaskInfo struct {
	ID          string     `json:"id"`           // 任务唯一标识符
	Type        TaskType   `json:"type"`         // 任务类型
	DocumentID  string     `json:"document_id"`  // 关联的文档ID
	Status      TaskStatus `json:"status"`       // 任务状态
	Error       string     `json:"error"`        // 错误信息
	CreatedAt   time.Time  `json:"created_at"`   // 创建时间
	StartedAt   *time.Time `json:"started_at"`   // 开始处理时间
	CompletedAt *time.Time `json:"completed_at"` // 完成时间
	Progress    float64    `json:"progress"`     // 处理进度（0-100）
}

// ProgressCallback 进度回调函数
// 用于报告任务处理进度
type ProgressCallback func(taskID string, progress float64, status string)

// Factory 队列工厂函数类型
// 用于创建不同类型的队列实现
type Factory func(cfg *Config) (Queue, error)

// NewTaskInfo 从Task创建TaskInfo
func NewTaskInfo(task *Task) *TaskInfo {
	return &TaskInfo{
		ID:          task.ID,
		Type:        task.Type,
		DocumentID:  task.DocumentID,
		Status:      task.Status,
		Error:       task.Error,
		CreatedAt:   task.CreatedAt,
		StartedAt:   task.StartedAt,
		CompletedAt: task.CompletedAt,
		Progress:    getTaskProgress(task),
	}
}

// getTaskProgress 根据任务状态计算进度
func getTaskProgress(task *Task) float64 {
	switch task.Status {
	case StatusPending:
		return 0.0
	case StatusProcessing:
		// 处理中默认返回50%，实际进度应该由任务处理器更新
		return 50.0
	case StatusCompleted:
		return 100.0
	case StatusFailed:
		// 失败任务的进度取决于失败时的处理阶段
		// 默认视为处理到一半
		return 50.0
	default:
		return 0.0
	}
}

// ErrTaskNotFound 任务未找到错误
var ErrTaskNotFound = TaskError("task not found")

// ErrTaskTimeout 任务超时错误
var ErrTaskTimeout = TaskError("task timed out")

// ErrInvalidPayload 无效的任务载荷错误
var ErrInvalidPayload = TaskError("invalid task payload")

// TaskError 任务错误类型
type TaskError string

// Error 实现error接口
func (e TaskError) Error() string {
	return string(e)
}

// MarshalPayload 将任务载荷序列化为JSON
func MarshalPayload(payload interface{}) (json.RawMessage, error) {
	if payload == nil {
		return json.RawMessage("{}"), nil
	}
	return json.Marshal(payload)
}

// UnmarshalPayload 将JSON反序列化为任务载荷
func UnmarshalPayload(data json.RawMessage, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}
