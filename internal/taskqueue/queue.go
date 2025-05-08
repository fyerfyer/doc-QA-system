package taskqueue

import (
	"context"
	"errors"
	"time"
)

// 定义常见错误
var (
	ErrTaskNotFound = errors.New("task not found")  // 任务未找到
	ErrQueueClosed  = errors.New("queue is closed") // 队列已关闭
	ErrInvalidTask  = errors.New("invalid task")    // 无效任务
)

// QueueOption 队列配置选项函数类型
type QueueOption func(*QueueOptions)

// QueueOptions 队列配置选项
type QueueOptions struct {
	MaxRetries int           // 最大重试次数
	Timeout    time.Duration // 操作超时时间
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(retries int) QueueOption {
	return func(o *QueueOptions) {
		o.MaxRetries = retries
	}
}

// WithTimeout 设置操作超时时间
func WithTimeout(timeout time.Duration) QueueOption {
	return func(o *QueueOptions) {
		o.Timeout = timeout
	}
}

// Queue 定义任务队列接口
// 负责异步任务的入队、状态查询和结果获取
type Queue interface {
	// Enqueue 将任务添加到队列中
	// 返回任务ID和可能的错误
	Enqueue(task *Task) (string, error)

	// GetTask 获取任务详情
	// 如果任务不存在，返回 ErrTaskNotFound
	GetTask(ctx context.Context, taskID string) (*Task, error)

	// MarkComplete 将任务标记为已完成状态
	// 同时保存任务结果
	MarkComplete(ctx context.Context, taskID string, result map[string]interface{}) error

	// MarkFailed 将任务标记为失败状态
	// 同时记录错误信息
	MarkFailed(ctx context.Context, taskID string, errMsg string) error

	// MarkProcessing 将任务标记为处理中状态
	MarkProcessing(ctx context.Context, taskID string) error

	// Close 关闭队列连接并释放资源
	Close() error
}

// Factory 队列工厂函数类型
type Factory func(options QueueOptions) (Queue, error)

// 全局注册的队列实现
var implementations = make(map[string]Factory)

// RegisterQueueImplementation 注册队列实现
func RegisterQueueImplementation(name string, factory Factory) {
	implementations[name] = factory
}

// NewQueue 根据名称创建任务队列
// name: 队列实现名称（如 "redis", "memory" 等）
// options: 队列配置选项
func NewQueue(name string, options ...QueueOption) (Queue, error) {
	// 应用默认配置
	opts := QueueOptions{
		MaxRetries: 3,                // 默认重试3次
		Timeout:    30 * time.Second, // 默认30秒超时
	}

	// 应用用户配置
	for _, option := range options {
		option(&opts)
	}

	// 查找注册的实现
	factory, ok := implementations[name]
	if !ok {
		return nil, errors.New("unknown queue implementation: " + name)
	}

	// 创建队列实例
	return factory(opts)
}
