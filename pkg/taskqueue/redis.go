package taskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	// 任务键前缀
	taskKeyPrefix = "task:"
	// 文档任务集合键前缀
	documentTasksKeyPrefix = "document_tasks:"
	// 默认任务过期时间（7天）
	defaultTaskExpiry = 7 * 24 * time.Hour
)

// RedisQueue Redis任务队列实现
type RedisQueue struct {
	client      *asynq.Client    // 用于添加任务
	inspector   *asynq.Inspector // 用于检查任务状态
	redisClient *redis.Client    // Redis客户端，用于存储任务数据
	cfg         *Config          // 队列配置
	logger      *logrus.Logger   // 日志记录器
}

// NewRedisQueue 创建Redis任务队列实例
func NewRedisQueue(cfg *Config) (Queue, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 使用配置创建asynq客户端
	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// 创建任务检查器
	inspector := asynq.NewInspector(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// 创建Redis客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// 测试Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	return &RedisQueue{
		client:      client,
		inspector:   inspector,
		redisClient: redisClient,
		cfg:         cfg,
		logger:      logger,
	}, nil
}

// Enqueue 将任务加入队列
func (q *RedisQueue) Enqueue(ctx context.Context, taskType TaskType, documentID string, payload interface{}) (string, error) {
	taskID := uuid.New().String() // 生成任务ID

	// 将payload序列化为JSON
	payloadBytes, err := MarshalPayload(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 创建任务结构体
	task := &Task{
		ID:         taskID,
		Type:       taskType,
		DocumentID: documentID,
		Status:     StatusPending,
		Payload:    payloadBytes,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		MaxRetries: q.cfg.RetryLimit,
	}

	// 将任务信息存储到Redis
	err = q.saveTaskToRedis(ctx, task)
	if err != nil {
		return "", fmt.Errorf("failed to save task to redis: %w", err)
	}

	// 创建asynq任务，使用taskID作为任务负载
	asynqTask := asynq.NewTask(string(taskType), []byte(taskID))

	// 将任务加入队列
	_, err = q.client.EnqueueContext(ctx, asynqTask)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	q.logger.WithFields(logrus.Fields{
		"task_id":     taskID,
		"task_type":   taskType,
		"document_id": documentID,
	}).Info("Task enqueued successfully")

	return taskID, nil
}

// EnqueueAt 在指定时间将任务加入队列
func (q *RedisQueue) EnqueueAt(ctx context.Context, taskType TaskType, documentID string, payload interface{}, processAt time.Time) (string, error) {
	taskID := uuid.New().String()

	payloadBytes, err := MarshalPayload(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := &Task{
		ID:         taskID,
		Type:       taskType,
		DocumentID: documentID,
		Status:     StatusPending,
		Payload:    payloadBytes,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		MaxRetries: q.cfg.RetryLimit,
	}

	err = q.saveTaskToRedis(ctx, task)
	if err != nil {
		return "", fmt.Errorf("failed to save task to redis: %w", err)
	}

	asynqTask := asynq.NewTask(string(taskType), []byte(taskID))
	_, err = q.client.EnqueueContext(ctx, asynqTask, asynq.ProcessAt(processAt))
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task with delay: %w", err)
	}

	return taskID, nil
}

// EnqueueIn 在指定延迟后将任务加入队列
func (q *RedisQueue) EnqueueIn(ctx context.Context, taskType TaskType, documentID string, payload interface{}, delay time.Duration) (string, error) {
	return q.EnqueueAt(ctx, taskType, documentID, payload, time.Now().Add(delay))
}

// GetTask 获取任务信息
func (q *RedisQueue) GetTask(ctx context.Context, taskID string) (*Task, error) {
	key := taskKeyPrefix + taskID
	data, err := q.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}

	// 处理无效或空数据
	if len(data) == 0 {
		return nil, ErrTaskNotFound
	}

	// 检查JSON数据中的时间格式并预处理
	var jsonData map[string]interface{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal task data for preprocessing: %w", err)
	}

	// 处理CreatedAt字段
	if createdAt, ok := jsonData["created_at"].(string); ok {
		// 如果时间戳没有时区信息，添加UTC时区
		if !strings.Contains(createdAt, "Z") && !strings.Contains(createdAt, "+") {
			jsonData["created_at"] = createdAt + "Z"
		}
	}

	// 处理UpdatedAt字段
	if updatedAt, ok := jsonData["updated_at"].(string); ok {
		// 如果时间戳没有时区信息，添加UTC时区
		if !strings.Contains(updatedAt, "Z") && !strings.Contains(updatedAt, "+") {
			jsonData["updated_at"] = updatedAt + "Z"
		}
	}

	// 处理StartedAt字段，如果存在
	if startedAt, ok := jsonData["started_at"].(string); ok && startedAt != "" {
		if !strings.Contains(startedAt, "Z") && !strings.Contains(startedAt, "+") {
			jsonData["started_at"] = startedAt + "Z"
		}
	}

	// 处理CompletedAt字段，如果存在
	if completedAt, ok := jsonData["completed_at"].(string); ok && completedAt != "" {
		if !strings.Contains(completedAt, "Z") && !strings.Contains(completedAt, "+") {
			jsonData["completed_at"] = completedAt + "Z"
		}
	}

	// 重新序列化预处理后的JSON数据
	fixedData, err := json.Marshal(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal preprocessed task data: %w", err)
	}

	var task Task
	if err := json.Unmarshal(fixedData, &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task data: %w", err)
	}

	return &task, nil
}

// GetTasksByDocument 获取文档相关的所有任务
func (q *RedisQueue) GetTasksByDocument(ctx context.Context, documentID string) ([]*Task, error) {
	key := documentTasksKeyPrefix + documentID
	taskIDs, err := q.redisClient.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get document tasks: %w", err)
	}

	if len(taskIDs) == 0 {
		return []*Task{}, nil
	}

	tasks := make([]*Task, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		task, err := q.GetTask(ctx, taskID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				// 任务可能已过期被删除，跳过
				continue
			}
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// WaitForTask 等待任务完成并返回结果
func (q *RedisQueue) WaitForTask(ctx context.Context, taskID string, timeout time.Duration) (*Task, error) {
	// q.logger.WithFields(logrus.Fields{
	// 	"task_id": taskID,
	// 	"timeout": timeout,
	// }).Info("Starting to wait for task completion")

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 初始检查任务状态
	task, err := q.GetTask(ctx, taskID)
	if err != nil {
		q.logger.WithError(err).Error("Failed to get task in WaitForTask")
		return nil, err
	}

	// q.logger.WithFields(logrus.Fields{
	// 	"task_id": taskID,
	// 	"status":  task.Status,
	// }).Info("Initial task status check")

	// 如果任务已完成或失败，直接返回
	if task.Status == StatusCompleted || task.Status == StatusFailed {
		// q.logger.WithFields(logrus.Fields{
		// 	"task_id": taskID,
		// 	"status":  task.Status,
		// }).Info("Task already completed or failed, returning immediately")
		return task, nil
	}

	// 使用发布/订阅监听任务状态变化
	pubsub := q.redisClient.Subscribe(ctx, "task_status:"+taskID)
	defer pubsub.Close()
	q.logger.WithField("channel", "task_status:"+taskID).Info("Subscribed to task status channel")

	// 每1秒轮询一次任务状态
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// q.logger.WithField("task_id", taskID).Error("Task wait timed out")
			return nil, ErrTaskTimeout
		case <-pubsub.Channel():
			// q.logger.WithFields(logrus.Fields{
			// 	"task_id": taskID,
			// 	"channel": msg.Channel,
			// 	"payload": msg.Payload,
			// }).Info("Received message from pubsub channel")

			task, err := q.GetTask(ctx, taskID)
			if err != nil {
				q.logger.WithError(err).Error("Failed to get task after pubsub notification")
				return nil, err
			}

			if task.Status == StatusCompleted || task.Status == StatusFailed {
				// q.logger.WithFields(logrus.Fields{
				// 	"task_id": taskID,
				// 	"status":  task.Status,
				// }).Info("Task completed after pubsub notification")
				return task, nil
			}
		case <-ticker.C:
			// q.logger.WithField("task_id", taskID).Debug("Polling task status")
			task, err := q.GetTask(ctx, taskID)
			if err != nil {
				q.logger.WithError(err).Error("Failed to get task during polling")
				return nil, err
			}

			// q.logger.WithFields(logrus.Fields{
			// 	"task_id": taskID,
			// 	"status":  task.Status,
			// }).Debug("Task status during polling")

			if task.Status == StatusCompleted || task.Status == StatusFailed {
				// q.logger.WithFields(logrus.Fields{
				// 	"task_id": taskID,
				// 	"status":  task.Status,
				// }).Info("Task completed during polling")
				return task, nil
			}
		}
	}
}

// DeleteTask 删除任务
func (q *RedisQueue) DeleteTask(ctx context.Context, taskID string) error {
	task, err := q.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	// 从文档任务集合中移除
	if task.DocumentID != "" {
		key := documentTasksKeyPrefix + task.DocumentID
		err := q.redisClient.SRem(ctx, key, taskID).Err()
		if err != nil {
			return fmt.Errorf("failed to remove task from document tasks: %w", err)
		}
	}

	// 删除任务数据
	key := taskKeyPrefix + taskID
	err = q.redisClient.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	// 尝试从asynq队列中删除任务（如果尚未处理）
	// 注意：已在处理中的任务可能无法删除
	err = q.inspector.DeleteTask("default", taskID)
	if err != nil {
		q.logger.WithError(err).WithField("task_id", taskID).Warn("Failed to delete task from asynq queue")
	}

	return nil
}

// Close 关闭队列连接
func (q *RedisQueue) Close() error {
	if err := q.client.Close(); err != nil {
		return err
	}
	if err := q.redisClient.Close(); err != nil {
		return err
	}
	return nil
}

// saveTaskToRedis 将任务信息保存到Redis
func (q *RedisQueue) saveTaskToRedis(ctx context.Context, task *Task) error {
	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	// 保存任务数据，设置7天过期
	key := taskKeyPrefix + task.ID
	err = q.redisClient.Set(ctx, key, taskData, defaultTaskExpiry).Err()
	if err != nil {
		return fmt.Errorf("failed to save task data: %w", err)
	}

	// 将任务ID添加到文档任务集合
	if task.DocumentID != "" {
		docKey := documentTasksKeyPrefix + task.DocumentID
		err = q.redisClient.SAdd(ctx, docKey, task.ID).Err()
		if err != nil {
			return fmt.Errorf("failed to add task to document tasks: %w", err)
		}
		// 设置文档任务集合的过期时间
		q.redisClient.Expire(ctx, docKey, defaultTaskExpiry)
	}

	return nil
}

// UpdateTaskStatus 更新任务状态
func (q *RedisQueue) UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus, result interface{}, errMsg string) error {
	task, err := q.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	task.Status = status
	task.UpdatedAt = time.Now()

	if status == StatusProcessing && task.StartedAt == nil {
		now := time.Now()
		task.StartedAt = &now
	}

	if status == StatusCompleted || status == StatusFailed {
		now := time.Now()
		task.CompletedAt = &now
	}

	if result != nil {
		resultBytes, err := MarshalPayload(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		task.Result = resultBytes
	}

	if errMsg != "" {
		task.Error = errMsg
	}

	// 保存更新后的任务状态
	err = q.saveTaskToRedis(ctx, task)
	if err != nil {
		return err
	}

	// 状态更新成功后自动发送通知
	if err := q.NotifyTaskUpdate(ctx, taskID); err != nil {
		q.logger.WithError(err).WithField("task_id", taskID).Warn("Failed to notify task update")
		// 通知失败通常不应该阻止整个操作成功，所以这里只记录日志而不返回错误
	}

	return nil
}

// NotifyTaskUpdate 通知任务状态更新
func (q *RedisQueue) NotifyTaskUpdate(ctx context.Context, taskID string) error {
	return q.redisClient.Publish(ctx, "task_status:"+taskID, "updated").Err()
}

// RedisWorker Redis工作者实现
type RedisWorker struct {
	server   *asynq.Server
	queue    *RedisQueue
	handlers map[TaskType]Handler
	logger   *logrus.Logger
}

// NewRedisWorker 创建Redis工作者
func NewRedisWorker(queue *RedisQueue, cfg *Config) Worker {
	if cfg == nil {
		cfg = queue.cfg
	}

	// 配置服务器
	serverConfig := asynq.Config{
		Concurrency: cfg.Concurrency,
		Queues:      cfg.Queues,
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			return cfg.RetryDelay
		},
		Logger: queue.logger,
	}

	server := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		serverConfig,
	)

	return &RedisWorker{
		server:   server,
		queue:    queue,
		handlers: make(map[TaskType]Handler),
		logger:   queue.logger,
	}
}

// RegisterHandler 注册任务处理器
func (w *RedisWorker) RegisterHandler(taskType TaskType, handler Handler) {
	w.handlers[taskType] = handler
}

// Start 启动工作者
func (w *RedisWorker) Start() error {
	mux := asynq.NewServeMux()

	// 为每种任务类型注册处理函数
	for taskType, handler := range w.handlers {
		// 使用闭包捕获handler变量
		h := handler
		taskTypeStr := string(taskType)

		mux.HandleFunc(taskTypeStr, func(ctx context.Context, task *asynq.Task) error {
			taskID := string(task.Payload())

			// 获取任务信息
			taskInfo, err := w.queue.GetTask(ctx, taskID)
			if err != nil {
				w.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get task info")
				return err
			}

			// 更新任务状态为处理中
			err = w.queue.UpdateTaskStatus(ctx, taskID, StatusProcessing, nil, "")
			if err != nil {
				w.logger.WithError(err).WithField("task_id", taskID).Error("Failed to update task status to processing")
			}

			// 通知状态更新
			w.queue.NotifyTaskUpdate(ctx, taskID)

			// 调用处理器处理任务
			err = h.ProcessTask(ctx, taskInfo)

			// 根据处理结果更新任务状态
			if err != nil {
				errMsg := err.Error()
				updateErr := w.queue.UpdateTaskStatus(ctx, taskID, StatusFailed, nil, errMsg)
				if updateErr != nil {
					w.logger.WithError(updateErr).WithField("task_id", taskID).Error("Failed to update task status after failure")
				}
				w.queue.NotifyTaskUpdate(ctx, taskID)
				return err
			}

			// 处理成功，更新任务状态
			err = w.queue.UpdateTaskStatus(ctx, taskID, StatusCompleted, nil, "")
			if err != nil {
				w.logger.WithError(err).WithField("task_id", taskID).Error("Failed to update task status after completion")
			}
			w.queue.NotifyTaskUpdate(ctx, taskID)
			return nil
		})

		w.logger.WithField("task_type", taskType).Info("Registered handler for task type")
	}

	// 启动服务器
	return w.server.Start(mux)
}

// Stop 停止工作者
func (w *RedisWorker) Stop() {
	w.server.Shutdown()
}

// 注册Redis队列工厂函数
func init() {
	// 注册Redis队列工厂函数
	RegisterQueueFactory("redis", func(cfg *Config) (Queue, error) {
		return NewRedisQueue(cfg)
	})
}

// RegisterQueueFactory 注册队列工厂函数
func RegisterQueueFactory(name string, factory Factory) {
	queueFactories[name] = factory
}

// 队列工厂函数映射
var queueFactories = make(map[string]Factory)

// NewQueue 根据名称创建队列实例
func NewQueue(name string, cfg *Config) (Queue, error) {
	factory, exists := queueFactories[name]
	if !exists {
		return nil, fmt.Errorf("unknown queue implementation: %s", name)
	}
	return factory(cfg)
}
