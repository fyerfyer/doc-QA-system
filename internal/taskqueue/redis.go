package taskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisQueue 基于Redis实现的任务队列
type RedisQueue struct {
	client     *redis.Client // Redis客户端
	options    QueueOptions  // 队列选项
	closed     bool          // 是否已关闭
	queueKey   string        // 队列键名
	taskPrefix string        // 任务前缀
}

// RedisQueueConfig Redis队列配置
type RedisQueueConfig struct {
	Address  string        // Redis服务器地址
	Password string        // Redis密码
	DB       int           // Redis数据库编号
	Prefix   string        // 键前缀
	Timeout  time.Duration // 操作超时
}

// NewRedisQueue 创建Redis任务队列
func NewRedisQueue(config RedisQueueConfig, opts ...QueueOption) (*RedisQueue, error) {
	// 应用默认配置
	options := QueueOptions{
		MaxRetries: 3,
		Timeout:    30 * time.Second,
	}

	// 应用用户配置
	for _, opt := range opts {
		opt(&options)
	}

	// 创建Redis客户端
	client := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.DB,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// 使用前缀，没有则使用默认前缀
	prefix := config.Prefix
	if prefix == "" {
		prefix = "taskqueue:"
	}

	queue := &RedisQueue{
		client:     client,
		options:    options,
		queueKey:   prefix + "queue",
		taskPrefix: prefix + "task:",
	}

	// 注册Redis队列实现
	RegisterQueueImplementation("redis", func(options QueueOptions) (Queue, error) {
		queue.options = options
		return queue, nil
	})

	return queue, nil
}

// Enqueue 将任务添加到队列
func (q *RedisQueue) Enqueue(task *Task) (string, error) {
	if q.closed {
		return "", ErrQueueClosed
	}

	if task == nil {
		return "", ErrInvalidTask
	}

	ctx, cancel := context.WithTimeout(context.Background(), q.options.Timeout)
	defer cancel()

	// 序列化任务
	taskData, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task: %w", err)
	}

	// 使用管道批量执行命令
	pipe := q.client.Pipeline()

	// 保存任务详情
	taskKey := q.taskPrefix + task.ID
	pipe.Set(ctx, taskKey, taskData, 7*24*time.Hour) // 默认存储一周

	// 将任务ID添加到队列
	pipe.LPush(ctx, q.queueKey+":"+task.Type, task.ID)

	// 执行管道命令
	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	return task.ID, nil
}

// GetTask 获取任务信息
func (q *RedisQueue) GetTask(ctx context.Context, taskID string) (*Task, error) {
	if q.closed {
		return nil, ErrQueueClosed
	}

	// 获取任务数据
	taskKey := q.taskPrefix + taskID
	data, err := q.client.Get(ctx, taskKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// 反序列化任务
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	return &task, nil
}

// MarkComplete 标记任务为已完成
func (q *RedisQueue) MarkComplete(ctx context.Context, taskID string, result map[string]interface{}) error {
	// 使用管道执行多个操作
	pipe := q.client.Pipeline()

	// 更新任务状态
	err := q.updateTaskStatus(ctx, taskID, func(task *Task) {
		task.SetResult(result)
	})
	if err != nil {
		return err
	}

	// 将任务ID添加到完成列表
	completedListKey := q.taskPrefix + "completed_list"
	if err := pipe.LPush(ctx, completedListKey, taskID).Err(); err != nil {
		return fmt.Errorf("failed to add task to completed list: %w", err)
	}

	// 执行管道命令
	_, err = pipe.Exec(ctx)
	return err
}

// MarkFailed 标记任务为失败
func (q *RedisQueue) MarkFailed(ctx context.Context, taskID string, errMsg string) error {
	// 使用管道执行多个操作
	pipe := q.client.Pipeline()

	// 更新任务状态
	err := q.updateTaskStatus(ctx, taskID, func(task *Task) {
		task.Error = errMsg
		task.Status = TaskStatusFailed
		task.UpdatedAt = time.Now()
	})
	if err != nil {
		return err
	}

	// 将任务ID添加到完成列表
	completedListKey := q.taskPrefix + "completed_list"
	if err := pipe.LPush(ctx, completedListKey, taskID).Err(); err != nil {
		return fmt.Errorf("failed to add task to completed list: %w", err)
	}

	// 执行管道命令
	_, err = pipe.Exec(ctx)
	return err
}

// MarkProcessing 标记任务为处理中
func (q *RedisQueue) MarkProcessing(ctx context.Context, taskID string) error {
	return q.updateTaskStatus(ctx, taskID, func(task *Task) {
		task.SetStatus(TaskStatusProcessing)
	})
}

// updateTaskStatus 更新任务状态的通用方法
func (q *RedisQueue) updateTaskStatus(ctx context.Context, taskID string, updateFn func(*Task)) error {
	if q.closed {
		return ErrQueueClosed
	}

	// 获取任务
	task, err := q.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	// 应用更新
	updateFn(task)

	// 序列化任务
	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	// 保存更新后的任务
	taskKey := q.taskPrefix + taskID
	if err := q.client.Set(ctx, taskKey, taskData, 7*24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// Close 关闭队列连接
func (q *RedisQueue) Close() error {
	if q.closed {
		return nil
	}

	q.closed = true
	return q.client.Close()
}

// CreateRedisQueueFromConfig 从配置创建Redis任务队列
func CreateRedisQueueFromConfig(config interface{}) (Queue, error) {
	// 将配置转换为正确的类型
	cfg, ok := config.(struct {
		Address  string
		Password string
		DB       int
		Prefix   string
		Timeout  int
	})

	if !ok {
		return nil, fmt.Errorf("invalid config type for Redis queue")
	}

	// 创建配置
	redisConfig := RedisQueueConfig{
		Address:  cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
		Prefix:   cfg.Prefix,
		Timeout:  time.Duration(cfg.Timeout) * time.Second,
	}

	// 创建队列
	return NewRedisQueue(redisConfig)
}

func init() {
	// 注册Redis队列工厂函数
	// 这里现在只提供基本注册，具体参数会在实际使用时从配置提供
	RegisterQueueImplementation("redis", func(options QueueOptions) (Queue, error) {
		// 默认配置，应该被实际应用中的配置覆盖
		return nil, fmt.Errorf("redis queue requires configuration")
	})
}
