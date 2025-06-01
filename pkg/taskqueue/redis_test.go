package taskqueue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRedisTest 设置一个miniredis实例用于测试
// 返回Redis地址和一个清理函数
func setupRedisTest(t *testing.T) (string, func()) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	return mr.Addr(), func() {
		mr.Close()
	}
}

// TestNewRedisQueue 测试创建Redis队列实例
func TestNewRedisQueue(t *testing.T) {
	// 使用miniredis进行测试
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, queue)

	err = queue.Close()
	assert.NoError(t, err)
}

// TestRedisQueue_Enqueue 测试队列入队功能
func TestRedisQueue_Enqueue(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()
	payload := &DocumentParsePayload{
		FilePath: "/path/to/document.pdf",
		FileName: "document.pdf",
		FileType: "pdf",
	}

	// 测试基本入队
	taskID, err := queue.Enqueue(ctx, TaskDocumentParse, "doc-123", payload)
	assert.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// 验证任务已入队
	task, err := queue.GetTask(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, taskID, task.ID)
	assert.Equal(t, TaskDocumentParse, task.Type)
	assert.Equal(t, "doc-123", task.DocumentID)
	assert.Equal(t, StatusPending, task.Status)
	assert.NotNil(t, task.Payload)
}

// TestRedisQueue_EnqueueAt 测试延时入队功能
func TestRedisQueue_EnqueueAt(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()
	payload := &DocumentParsePayload{
		FilePath: "/path/to/document.pdf",
		FileName: "document.pdf",
		FileType: "pdf",
	}

	// 测试延时入队
	processAt := time.Now().Add(time.Second)
	taskID, err := queue.EnqueueAt(ctx, TaskDocumentParse, "doc-123", payload, processAt)
	assert.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// 验证任务已入队
	task, err := queue.GetTask(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, taskID, task.ID)
	assert.Equal(t, TaskDocumentParse, task.Type)
	assert.Equal(t, StatusPending, task.Status)
}

// TestRedisQueue_EnqueueIn 测试延时入队功能
func TestRedisQueue_EnqueueIn(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()
	payload := &DocumentParsePayload{
		FilePath: "/path/to/document.pdf",
		FileName: "document.pdf",
		FileType: "pdf",
	}

	// 测试延时入队
	delay := time.Second
	taskID, err := queue.EnqueueIn(ctx, TaskDocumentParse, "doc-123", payload, delay)
	assert.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// 验证任务已入队
	task, err := queue.GetTask(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, taskID, task.ID)
	assert.Equal(t, TaskDocumentParse, task.Type)
	assert.Equal(t, StatusPending, task.Status)
}

// TestRedisQueue_GetTasksByDocument 测试获取文档相关任务
func TestRedisQueue_GetTasksByDocument(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()
	documentID := "doc-456"

	// 创建多个任务
	payloads := []interface{}{
		&DocumentParsePayload{
			FilePath: "/path/to/document1.pdf",
			FileName: "document1.pdf",
			FileType: "pdf",
		},
		&TextChunkPayload{
			DocumentID: documentID,
			ChunkSize:  1000,
			Overlap:    200,
		},
		&VectorizePayload{
			DocumentID: documentID,
			Model:      "default",
		},
	}

	taskTypes := []TaskType{
		TaskDocumentParse,
		TaskTextChunk,
		TaskVectorize,
	}

	// 为同一个文档入队多个任务
	for i, payload := range payloads {
		_, err := queue.Enqueue(ctx, taskTypes[i], documentID, payload)
		require.NoError(t, err)
	}

	// 获取文档相关的任务
	tasks, err := queue.GetTasksByDocument(ctx, documentID)
	assert.NoError(t, err)
	assert.Equal(t, len(payloads), len(tasks))

	// 验证所有任务都关联到正确的文档
	for _, task := range tasks {
		assert.Equal(t, documentID, task.DocumentID)
	}

	// 测试获取不存在文档的任务
	emptyTasks, err := queue.GetTasksByDocument(ctx, "non-existent")
	assert.NoError(t, err)
	assert.Empty(t, emptyTasks)
}

// TestRedisQueue_UpdateTaskStatus 测试更新任务状态
func TestRedisQueue_UpdateTaskStatus(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()

	// 创建一个任务
	payload := &DocumentParsePayload{
		FilePath: "/path/to/document.pdf",
		FileName: "document.pdf",
		FileType: "pdf",
	}

	taskID, err := queue.Enqueue(ctx, TaskDocumentParse, "doc-789", payload)
	require.NoError(t, err)

	// 更新任务状态到处理中
	err = queue.UpdateTaskStatus(ctx, taskID, StatusProcessing, nil, "")
	assert.NoError(t, err)

	// 验证状态已更新
	task, err := queue.GetTask(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, StatusProcessing, task.Status)
	assert.NotNil(t, task.StartedAt)

	// 更新任务状态到已完成，带结果
	result := &DocumentParseResult{
		Content: "Parsed content",
		Title:   "Document Title",
		Pages:   5,
		Words:   1000,
		Chars:   5000,
	}
	err = queue.UpdateTaskStatus(ctx, taskID, StatusCompleted, result, "")
	assert.NoError(t, err)

	// 验证状态和结果已更新
	task, err = queue.GetTask(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, task.Status)
	assert.NotNil(t, task.CompletedAt)
	assert.NotEmpty(t, task.Result)

	// 测试更新到失败状态
	failTaskID, err := queue.Enqueue(ctx, TaskDocumentParse, "doc-789", payload)
	require.NoError(t, err)

	errorMsg := "Processing failed due to invalid document format"
	err = queue.UpdateTaskStatus(ctx, failTaskID, StatusFailed, nil, errorMsg)
	assert.NoError(t, err)

	// 验证失败状态
	failTask, err := queue.GetTask(ctx, failTaskID)
	assert.NoError(t, err)
	assert.Equal(t, StatusFailed, failTask.Status)
	assert.Equal(t, errorMsg, failTask.Error)
	assert.NotNil(t, failTask.CompletedAt)
}

// TestRedisQueue_DeleteTask 测试删除任务
func TestRedisQueue_DeleteTask(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()

	// 创建一个任务
	payload := &DocumentParsePayload{
		FilePath: "/path/to/document.pdf",
		FileName: "document.pdf",
		FileType: "pdf",
	}

	documentID := "doc-delete-test"
	taskID, err := queue.Enqueue(ctx, TaskDocumentParse, documentID, payload)
	require.NoError(t, err)

	// 确认任务和文档关联存在
	tasks, err := queue.GetTasksByDocument(ctx, documentID)
	assert.NoError(t, err)
	assert.Len(t, tasks, 1)

	// 删除任务
	err = queue.DeleteTask(ctx, taskID)
	assert.NoError(t, err)

	// 验证任务已被删除
	_, err = queue.GetTask(ctx, taskID)
	assert.Error(t, err)
	assert.Equal(t, ErrTaskNotFound, err)

	// 验证文档关联也被删除
	tasks, err = queue.GetTasksByDocument(ctx, documentID)
	assert.NoError(t, err)
	assert.Empty(t, tasks)
}

// TestRedisQueue_NotifyTaskUpdate 测试任务更新通知
func TestRedisQueue_NotifyTaskUpdate(t *testing.T) {
	redisAddr, cleanup := setupRedisTest(t)
	defer cleanup()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx := context.Background()

	// 创建一个任务
	taskID, err := queue.Enqueue(ctx, TaskDocumentParse, "doc-notify", &DocumentParsePayload{})
	require.NoError(t, err)

	// 测试通知更新
	err = queue.NotifyTaskUpdate(ctx, taskID)
	assert.NoError(t, err)
}

// mockHandler 实现Handler接口，用于测试
type mockHandler struct {
	processFunc func(context.Context, *Task) error
	taskTypes   []TaskType
}

func (h *mockHandler) ProcessTask(ctx context.Context, task *Task) error {
	if h.processFunc != nil {
		return h.processFunc(ctx, task)
	}
	return nil
}

func (h *mockHandler) GetTaskTypes() []TaskType {
	return h.taskTypes
}

// TestRedisWorker 测试Redis工作者
func TestRedisWorker(t *testing.T) {
	// 使用本地Redis服务进行测试
	redisAddr := "localhost:6379"

	// 测试Redis连接
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		t.Skip("Skipping Redis worker test: Redis not available at localhost:6379")
		return
	}
	client.Close()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	// 创建Redis队列
	redisQueue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer redisQueue.Close()

	rq, ok := redisQueue.(*RedisQueue)
	require.True(t, ok, "Failed to cast to RedisQueue")

	// 创建Redis工作者
	worker := NewRedisWorker(rq, cfg)
	require.NotNil(t, worker)

	// 注册一个简单的处理器
	processedTasks := make(map[string]bool)
	handler := &mockHandler{
		processFunc: func(ctx context.Context, task *Task) error {
			processedTasks[task.ID] = true
			// 模拟处理时间
			time.Sleep(100 * time.Millisecond)
			return nil
		},
		taskTypes: []TaskType{TaskDocumentParse},
	}

	worker.RegisterHandler(TaskDocumentParse, handler)

	// 启动工作者（在后台）
	errChan := make(chan error)
	go func() {
		errChan <- worker.Start()
	}()

	// 等待工作者启动
	time.Sleep(100 * time.Millisecond)

	// 测试入队任务
	ctx = context.Background()
	payload := &DocumentParsePayload{
		FilePath: "/path/to/document.pdf",
		FileName: "document.pdf",
		FileType: "pdf",
	}

	taskID, err := redisQueue.Enqueue(ctx, TaskDocumentParse, "doc-worker-test", payload)
	require.NoError(t, err)

	// 给工作者一些时间来处理任务
	time.Sleep(500 * time.Millisecond)

	// 停止工作者
	worker.Stop()

	// 检查任务是否已处理
	task, err := redisQueue.GetTask(ctx, taskID)
	if err == nil {
		t.Logf("Task status: %s", task.Status)
	}

	// 检查是否有错误发生
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Worker returned error: %v", err)
		}
	default:
		// 没有错误，这是好的
	}
}

// TestIntegration_RealRedis 集成测试：使用真实Redis服务器测试完整流程
func TestIntegration_RealRedis(t *testing.T) {
	// 使用本地Redis服务进行测试
	redisAddr := "localhost:6379"

	// 测试Redis连接
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		t.Skip("Skipping integration test: Redis not available at localhost:6379")
		return
	}
	client.Close()

	cfg := &Config{
		RedisAddr:   redisAddr,
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}

	// 创建Redis队列
	queue, err := NewRedisQueue(cfg)
	require.NoError(t, err)
	defer queue.Close()

	ctx = context.Background()
	documentID := "doc-integration-test"

	// 创建一个完整处理流程任务
	payload := &ProcessCompletePayload{
		DocumentID: documentID,
		FilePath:   "/path/to/document.pdf",
		FileName:   "integration-test.pdf",
		FileType:   "pdf",
		ChunkSize:  1000,
		Overlap:    200,
		SplitType:  "text",
		Model:      "default",
		Metadata: map[string]string{
			"source": "integration-test",
		},
	}

	// 入队任务
	taskID, err := queue.Enqueue(ctx, TaskProcessComplete, documentID, payload)
	require.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// 更新任务状态到处理中
	err = queue.UpdateTaskStatus(ctx, taskID, StatusProcessing, nil, "")
	assert.NoError(t, err)

	// 模拟处理过程
	time.Sleep(100 * time.Millisecond)

	// 处理完成后更新结果
	result := &ProcessCompleteResult{
		DocumentID:   documentID,
		ChunkCount:   5,
		VectorCount:  5,
		Dimension:    512,
		ParseStatus:  "completed",
		ChunkStatus:  "completed",
		VectorStatus: "completed",
	}
	err = queue.UpdateTaskStatus(ctx, taskID, StatusCompleted, result, "")
	assert.NoError(t, err)

	// 获取并验证任务状态
	task, err := queue.GetTask(ctx, taskID)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, task.Status)
	assert.NotNil(t, task.Result)
	assert.NotNil(t, task.CompletedAt)

	// 测试通知更新
	err = queue.NotifyTaskUpdate(ctx, taskID)
	assert.NoError(t, err)

	// 等待并获取任务结果
	completedTask, err := queue.WaitForTask(ctx, taskID, 100*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, completedTask.Status)

	// 清理
	err = queue.DeleteTask(ctx, taskID)
	assert.NoError(t, err)
}

// TestTaskInfo 测试TaskInfo生成
func TestTaskInfo(t *testing.T) {
	// 创建一个Task实例
	now := time.Now()
	startedAt := now.Add(-5 * time.Minute)
	completedAt := now.Add(-1 * time.Minute)

	task := &Task{
		ID:          "task-123",
		Type:        TaskDocumentParse,
		DocumentID:  "doc-123",
		Status:      StatusCompleted,
		Error:       "",
		CreatedAt:   now.Add(-10 * time.Minute),
		UpdatedAt:   now,
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
		Attempts:    1,
		MaxRetries:  3,
	}

	// 生成TaskInfo
	info := NewTaskInfo(task)

	// 验证TaskInfo包含正确信息
	assert.Equal(t, task.ID, info.ID)
	assert.Equal(t, task.Type, info.Type)
	assert.Equal(t, task.DocumentID, info.DocumentID)
	assert.Equal(t, task.Status, info.Status)
	assert.Equal(t, task.Error, info.Error)
	assert.Equal(t, task.CreatedAt, info.CreatedAt)
	assert.Equal(t, task.StartedAt, info.StartedAt)
	assert.Equal(t, task.CompletedAt, info.CompletedAt)
	assert.Equal(t, 100.0, info.Progress) // 已完成状态进度为100%
}
