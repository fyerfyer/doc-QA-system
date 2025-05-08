package taskqueue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// TaskHandler 任务处理器函数类型
// 当任务状态变化时会调用此类型的函数
type TaskHandler func(ctx context.Context, task *Task) error

// TaskMonitor 任务监控器
// 定期检查任务状态并处理已完成或失败的任务
type TaskMonitor struct {
	queue          Queue                  // 任务队列
	interval       time.Duration          // 检查间隔
	handlers       map[string]TaskHandler // 任务处理器映射表（按任务类型）
	defaultHandler TaskHandler            // 默认任务处理器
	stopCh         chan struct{}          // 停止信号通道
	wg             sync.WaitGroup         // 等待组
	mu             sync.RWMutex           // 读写锁，保护handlers
	ctx            context.Context        // 上下文
	cancel         context.CancelFunc     // 上下文取消函数
}

// MonitorOption 监控器配置选项函数类型
type MonitorOption func(*TaskMonitor)

// WithInterval 设置检查间隔
func WithInterval(interval time.Duration) MonitorOption {
	return func(m *TaskMonitor) {
		m.interval = interval
	}
}

// WithDefaultHandler 设置默认处理器
func WithDefaultHandler(handler TaskHandler) MonitorOption {
	return func(m *TaskMonitor) {
		m.defaultHandler = handler
	}
}

// NewTaskMonitor 创建新的任务监控器
func NewTaskMonitor(queue Queue, options ...MonitorOption) *TaskMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	monitor := &TaskMonitor{
		queue:    queue,
		interval: 5 * time.Second, // 默认5秒检查一次
		handlers: make(map[string]TaskHandler),
		stopCh:   make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	// 应用配置选项
	for _, option := range options {
		option(monitor)
	}

	return monitor
}

// RegisterHandler 注册任务类型特定的处理器
func (m *TaskMonitor) RegisterHandler(taskType string, handler TaskHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[taskType] = handler
}

// Start 启动监控器
func (m *TaskMonitor) Start() {
	m.wg.Add(1)
	go m.run()
	log.Println("Task monitor started")
}

// Stop 停止监控器
func (m *TaskMonitor) Stop() {
	m.cancel()
	close(m.stopCh)
	m.wg.Wait()
	log.Println("Task monitor stopped")
}

// run 监控循环
func (m *TaskMonitor) run() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkCompletedTasks()
		case <-m.stopCh:
			return
		case <-m.ctx.Done():
			return
		}
	}
}

// checkCompletedTasks 检查已完成的任务
// 从队列中获取并处理最近完成或失败的任务
func (m *TaskMonitor) checkCompletedTasks() {
	// 获取队列实现，以访问特定于实现的方法
	redisQueue, ok := m.queue.(*RedisQueue)
	if !ok {
		// 如果不是RedisQueue，我们无法使用特定的方法
		log.Println("Task monitor requires RedisQueue implementation")
		return
	}

	// 获取最近完成的任务ID
	taskIDs, err := redisQueue.GetRecentlyCompletedTasks(m.ctx)
	if err != nil {
		log.Printf("Error getting completed tasks: %v", err)
		return
	}

	// 处理每个已完成的任务
	for _, taskID := range taskIDs {
		// 在单独的goroutine中处理每个任务，避免一个任务阻塞其他任务
		m.wg.Add(1)
		go func(id string) {
			defer m.wg.Done()

			// 创建新的上下文，避免主上下文取消影响单个任务处理
			taskCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()

			if err := m.processTask(taskCtx, id); err != nil {
				log.Printf("Error processing task %s: %v", id, err)
			}

			// 处理完后从完成列表中移除
			if err := redisQueue.RemoveFromCompletedList(taskCtx, id); err != nil {
				log.Printf("Error removing task %s from completed list: %v", id, err)
			}
		}(taskID)
	}
}

// processTask 处理单个任务
func (m *TaskMonitor) processTask(ctx context.Context, taskID string) error {
	// 获取任务详情
	task, err := m.queue.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task details: %w", err)
	}

	// 只处理已完成或失败的任务
	if !task.IsCompleted() && !task.IsFailed() {
		return nil
	}

	// 获取对应的处理器
	handler := m.getHandler(task.Type)
	if handler == nil {
		// 没有处理器，任务无法处理
		log.Printf("No handler for task type: %s", task.Type)
		return nil
	}

	// 调用处理器处理任务
	return handler(ctx, task)
}

// getHandler 获取任务类型对应的处理器
func (m *TaskMonitor) getHandler(taskType string) TaskHandler {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 优先使用注册的特定处理器
	if handler, ok := m.handlers[taskType]; ok {
		return handler
	}

	// 如果没有找到，则使用默认处理器
	return m.defaultHandler
}

// 下面为Redis队列实现添加的方法

// filepath: e:\GolandProjects\doc-QA-system\internal\taskqueue\redis.go

// 需要在RedisQueue中添加以下方法:

// GetRecentlyCompletedTasks 获取最近完成或失败的任务ID列表
func (q *RedisQueue) GetRecentlyCompletedTasks(ctx context.Context) ([]string, error) {
	if q.closed {
		return nil, ErrQueueClosed
	}

	// 从Redis中获取最近完成的任务ID列表
	// 使用列表范围查询，一次最多处理100个任务
	completedListKey := q.taskPrefix + "completed_list"
	taskIDs, err := q.client.LRange(ctx, completedListKey, 0, 99).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get completed task list: %w", err)
	}

	return taskIDs, nil
}

// RemoveFromCompletedList 从完成列表中移除任务ID
func (q *RedisQueue) RemoveFromCompletedList(ctx context.Context, taskID string) error {
	if q.closed {
		return ErrQueueClosed
	}

	completedListKey := q.taskPrefix + "completed_list"
	// 从列表中移除指定的任务ID
	_, err := q.client.LRem(ctx, completedListKey, 1, taskID).Result()
	if err != nil {
		return fmt.Errorf("failed to remove task from completed list: %w", err)
	}

	return nil
}
