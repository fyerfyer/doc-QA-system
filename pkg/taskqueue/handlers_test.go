package taskqueue

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCallbackProcessor 测试创建一个回调处理器
func TestNewCallbackProcessor(t *testing.T) {
	// 创建依赖项
	mockQueue := new(MockQueue)
	logger := logrus.New()

	// 创建处理器
	processor := NewCallbackProcessor(mockQueue, logger)

	// 验证处理器是否正确初始化
	assert.NotNil(t, processor)
	assert.Equal(t, mockQueue, processor.queue)
	assert.Equal(t, logger, processor.logger)
	assert.NotNil(t, processor.handlers)
}

// TestRegisterHandler 测试注册一个处理函数
func TestRegisterHandler(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 注册一个处理函数
	handlerCalled := false
	handler := func(ctx context.Context, task *Task, result json.RawMessage) error {
		handlerCalled = true
		return nil
	}
	processor.RegisterHandler(TaskDocumentParse, handler)

	// 验证处理函数是否正确注册
	assert.NotNil(t, processor.handlers[TaskDocumentParse])

	// 调用处理函数以验证其是否正常工作
	err := processor.handlers[TaskDocumentParse](context.Background(), nil, nil)
	assert.NoError(t, err)
	assert.True(t, handlerCalled)
}

// TestSetDefaultHandler 测试设置默认处理函数
func TestSetDefaultHandler(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 设置默认处理函数
	defaultHandlerCalled := false
	defaultHandler := func(ctx context.Context, task *Task, result json.RawMessage) error {
		defaultHandlerCalled = true
		return nil
	}
	processor.SetDefaultHandler(defaultHandler)

	// 验证默认处理函数是否正常工作
	assert.NotNil(t, processor.defaultFn)
	err := processor.defaultFn(context.Background(), nil, nil)
	assert.NoError(t, err)
	assert.True(t, defaultHandlerCalled)
}

// TestProcessCallback_ValidData 测试处理有效的回调数据
func TestProcessCallback_ValidData(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 设置模拟期望
	taskID := "test-task-id"
	documentID := "test-doc-id"
	testTask := &Task{
		ID:         taskID,
		Type:       TaskDocumentParse,
		DocumentID: documentID,
		Status:     StatusPending,
	}

	mockQueue.EXPECT().GetTask(context.Background(), taskID).Return(testTask, nil)
	mockQueue.EXPECT().UpdateTaskStatus(context.Background(), taskID, StatusCompleted, json.RawMessage(`{"test":"data"}`), "").Return(nil)
	mockQueue.EXPECT().NotifyTaskUpdate(context.Background(), taskID).Return(nil)

	// 注册一个处理函数
	handlerCalled := false
	processor.RegisterHandler(TaskDocumentParse, func(ctx context.Context, task *Task, result json.RawMessage) error {
		handlerCalled = true
		assert.Equal(t, testTask, task)
		assert.Equal(t, json.RawMessage(`{"test":"data"}`), result)
		return nil
	})

	// 创建回调数据
	callback := &TaskCallback{
		TaskID:     taskID,
		DocumentID: documentID,
		Status:     StatusCompleted,
		Type:       TaskDocumentParse,
		Result:     json.RawMessage(`{"test":"data"}`),
		Timestamp:  time.Now(),
	}

	callbackData, err := json.Marshal(callback)
	require.NoError(t, err)

	// 处理回调
	err = processor.ProcessCallback(context.Background(), callbackData)
	assert.NoError(t, err)
	assert.True(t, handlerCalled)
}

// TestProcessCallback_InvalidData 测试处理无效的回调数据
func TestProcessCallback_InvalidData(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 处理无效的回调数据
	err := processor.ProcessCallback(context.Background(), []byte("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal callback data")
}

// TestProcessCallback_TaskNotFound 测试处理不存在任务的回调
func TestProcessCallback_TaskFailed(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 设置模拟期望
	taskID := "test-task-id"
	documentID := "test-doc-id"
	testTask := &Task{
		ID:         taskID,
		Type:       TaskDocumentParse,
		DocumentID: documentID,
		Status:     StatusPending,
	}

	mockQueue.EXPECT().GetTask(context.Background(), taskID).Return(testTask, nil)
	mockQueue.EXPECT().UpdateTaskStatus(context.Background(), taskID, StatusFailed, json.RawMessage(`{}`), "test error").Return(nil)
	mockQueue.EXPECT().NotifyTaskUpdate(context.Background(), taskID).Return(nil)

	// 注册一个处理函数
	handlerCalled := false
	processor.RegisterHandler(TaskDocumentParse, func(ctx context.Context, task *Task, result json.RawMessage) error {
		handlerCalled = true
		return nil
	})

	// 创建失败任务的回调数据
	callback := &TaskCallback{
		TaskID:     taskID,
		DocumentID: documentID,
		Status:     StatusFailed,
		Type:       TaskDocumentParse,
		Error:      "test error",
		Result:     json.RawMessage(`{}`), // 显式设置 Result 为 {}
		Timestamp:  time.Now(),
	}

	callbackData, err := json.Marshal(callback)
	require.NoError(t, err)

	// 处理失败任务的回调
	err = processor.ProcessCallback(context.Background(), callbackData)
	assert.NoError(t, err)
	// 对于失败的任务，处理函数不应被调用
	assert.False(t, handlerCalled)
}

// TestHandleCallback 测试 HTTP 回调处理
func TestHandleCallback(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 设置模拟期望
	taskID := "test-task-id"
	documentID := "test-doc-id"
	testTask := &Task{
		ID:         taskID,
		Type:       TaskDocumentParse,
		DocumentID: documentID,
		Status:     StatusPending,
	}

	mockQueue.EXPECT().GetTask(context.Background(), taskID).Return(testTask, nil)
	mockQueue.EXPECT().UpdateTaskStatus(context.Background(), taskID, StatusCompleted, json.RawMessage(`{"test":"data"}`), "").Return(nil)
	mockQueue.EXPECT().NotifyTaskUpdate(context.Background(), taskID).Return(nil)

	// 注册一个处理函数
	handlerCalled := false
	processor.RegisterHandler(TaskDocumentParse, func(ctx context.Context, task *Task, result json.RawMessage) error {
		handlerCalled = true
		return nil
	})

	// 创建回调请求
	req := &CallbackRequest{
		TaskID:     taskID,
		DocumentID: documentID,
		Status:     StatusCompleted,
		Type:       TaskDocumentParse,
		Result:     json.RawMessage(`{"test":"data"}`),
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	// 处理回调
	resp, err := processor.HandleCallback(context.Background(), req)
	assert.NoError(t, err)
	assert.True(t, handlerCalled)
	assert.True(t, resp.Success)
	assert.Equal(t, taskID, resp.TaskID)
}

// TestHandleCallback_InvalidTimestamp 测试处理带有无效时间戳格式的回调
func TestHandleCallback_InvalidTimestamp(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 设置模拟期望
	taskID := "test-task-id"
	documentID := "test-doc-id"
	testTask := &Task{
		ID:         taskID,
		Type:       TaskDocumentParse,
		DocumentID: documentID,
		Status:     StatusPending,
	}

	mockQueue.EXPECT().GetTask(context.Background(), taskID).Return(testTask, nil)
	mockQueue.EXPECT().UpdateTaskStatus(context.Background(), taskID, StatusCompleted, json.RawMessage(`{"test":"data"}`), "").Return(nil)
	mockQueue.EXPECT().NotifyTaskUpdate(context.Background(), taskID).Return(nil)

	// 创建带有无效时间戳的回调请求
	req := &CallbackRequest{
		TaskID:     taskID,
		DocumentID: documentID,
		Status:     StatusCompleted,
		Type:       TaskDocumentParse,
		Result:     json.RawMessage(`{"test":"data"}`),
		Timestamp:  "invalid-timestamp",
	}

	// 处理回调
	resp, err := processor.HandleCallback(context.Background(), req)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
}

// TestRegisterDefaultHandlers 测试注册默认处理函数
func TestRegisterDefaultHandlers(t *testing.T) {
	// 创建处理器
	mockQueue := new(MockQueue)
	logger := logrus.New()
	processor := NewCallbackProcessor(mockQueue, logger)

	// 注册默认处理函数
	processor.RegisterDefaultHandlers(mockQueue)

	// 验证所有默认处理函数是否已注册
	assert.NotNil(t, processor.handlers[TaskDocumentParse])
	assert.NotNil(t, processor.handlers[TaskTextChunk])
	assert.NotNil(t, processor.handlers[TaskVectorize])
	assert.NotNil(t, processor.handlers[TaskProcessComplete])
}

// TestIntegration_WithRedis 测试回调处理器与真实 Redis 实例的集成
func TestIntegration_WithRedis(t *testing.T) {
	// 如果启用了短测试模式，则跳过
	if testing.Short() {
		t.Skip("Skipping Redis integration test in short mode")
	}

	// 创建 Redis 队列
	queueConfig := &Config{
		RedisAddr: "localhost:6379", // 使用默认的 Redis 端口
		RedisDB:   15,               // 使用 DB 15 进行测试以避免冲突
	}

	queue, err := NewRedisQueue(queueConfig)
	if err != nil {
		t.Skip("Redis not available, skipping test:", err)
		return
	}
	defer queue.Close()

	// 清理任何现有的测试任务
	ctx := context.Background()
	testTaskID := "redis-test-task"
	testDocID := "redis-test-doc"
	_ = queue.DeleteTask(ctx, testTaskID) // 如果任务不存在，忽略错误

	// 创建一个测试任务
	task := &Task{
		ID:         testTaskID,
		Type:       TaskDocumentParse,
		DocumentID: testDocID,
		Status:     StatusPending,
		Payload:    json.RawMessage(`{"test":"payload"}`),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// 将任务保存到 Redis
	// 将 Redis 队列转换为访问内部方法
	redisQueue, ok := queue.(*RedisQueue)
	require.True(t, ok, "Queue should be a RedisQueue")
	require.NoError(t, redisQueue.saveTaskToRedis(ctx, task))

	// 使用真实队列创建处理器
	logger := logrus.New()
	processor := NewCallbackProcessor(queue, logger)

	// 注册一个处理函数
	handlerResult := make(chan struct{}, 1)
	processor.RegisterHandler(TaskDocumentParse, func(ctx context.Context, task *Task, result json.RawMessage) error {
		assert.Equal(t, testTaskID, task.ID)
		assert.Equal(t, testDocID, task.DocumentID)
		assert.Equal(t, json.RawMessage(`{"result":"success"}`), result)
		handlerResult <- struct{}{}
		return nil
	})

	// 创建回调请求
	req := &CallbackRequest{
		TaskID:     testTaskID,
		DocumentID: testDocID,
		Status:     StatusCompleted,
		Type:       TaskDocumentParse,
		Result:     json.RawMessage(`{"result":"success"}`),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	// 处理回调
	resp, err := processor.HandleCallback(ctx, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// 等待处理函数在超时时间内被调用
	select {
	case <-handlerResult:
		// 处理函数已成功调用
	case <-time.After(5 * time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	// 验证任务是否已更新
	updatedTask, err := queue.GetTask(ctx, testTaskID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, updatedTask.Status)
	assert.Equal(t, json.RawMessage(`{"result":"success"}`), updatedTask.Result)

	// 清理
	err = queue.DeleteTask(ctx, testTaskID)
	assert.NoError(t, err)
}

// TestDefaultHandlers 测试默认处理函数的实现
func TestDefaultHandlers(t *testing.T) {
	ctx := context.Background()
	mockQueue := new(MockQueue)
	logger := logrus.New()

	// 测试文档解析处理函数
	t.Run("DefaultDocumentParseHandler", func(t *testing.T) {
		taskID := "parse-task-id"
		documentID := "parse-doc-id"

		mockQueue.EXPECT().Enqueue(ctx, TaskTextChunk, documentID, mock.Anything).Return("chunk-task-id", nil)

		handler := DefaultDocumentParseHandler(ctx, mockQueue, logger)
		task := &Task{
			ID:         taskID,
			DocumentID: documentID,
			Type:       TaskDocumentParse,
		}

		result := json.RawMessage(`{"content":"test content","title":"test"}`)
		err := handler(ctx, task, result)
		assert.NoError(t, err)
	})

	// 测试文本分块处理函数
	t.Run("DefaultTextChunkHandler", func(t *testing.T) {
		taskID := "chunk-task-id"
		documentID := "chunk-doc-id"

		mockQueue.EXPECT().Enqueue(ctx, TaskVectorize, documentID, mock.Anything).Return("vector-task-id", nil)

		handler := DefaultTextChunkHandler(ctx, mockQueue, logger)
		task := &Task{
			ID:         taskID,
			DocumentID: documentID,
			Type:       TaskTextChunk,
		}

		result := json.RawMessage(`{"chunks":[{"text":"chunk 1","index":0}],"chunk_count":1}`)
		err := handler(ctx, task, result)
		assert.NoError(t, err)
	})

	// 测试向量化处理函数
	t.Run("DefaultVectorizeHandler", func(t *testing.T) {
		taskID := "vector-task-id"
		documentID := "vector-doc-id"

		handler := DefaultVectorizeHandler(ctx, mockQueue, logger)
		task := &Task{
			ID:         taskID,
			DocumentID: documentID,
			Type:       TaskVectorize,
		}

		result := json.RawMessage(`{"vector_count":2,"model":"test-model"}`)
		err := handler(ctx, task, result)
		assert.NoError(t, err)
	})
}
