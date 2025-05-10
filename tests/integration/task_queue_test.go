package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/taskqueue"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTaskQueueTest 初始化任务队列测试环境
func setupTaskQueueTest(t *testing.T) (*taskqueue.RedisQueue, *logrus.Logger) {
	// 加载测试配置
	cfg, err := config.Load("../config.test.yml")
	require.NoError(t, err, "Failed to load test config")

	// 创建测试日志记录器
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// 配置Redis队列
	redisConfig := taskqueue.RedisQueueConfig{
		Address:  cfg.TaskQueue.Address, // 使用测试配置中的Redis地址
		Password: cfg.TaskQueue.Password,
		DB:       cfg.TaskQueue.DB,
		Prefix:   cfg.TaskQueue.Prefix,
		Timeout:  time.Duration(cfg.TaskQueue.Timeout) * time.Second,
	}

	// 创建Redis队列实例
	queue, err := taskqueue.NewRedisQueue(redisConfig,
		taskqueue.WithMaxRetries(cfg.TaskQueue.MaxRetries),
		taskqueue.WithTimeout(time.Duration(cfg.TaskQueue.Timeout)*time.Second))
	require.NoError(t, err, "Failed to create Redis queue")

	return queue, logger
}

// TestTaskCreation 测试任务创建
func TestTaskCreation(t *testing.T) {
	// 创建测试环境
	_, logger := setupTaskQueueTest(t)
	logger.Info("Testing task creation")

	// 创建文档处理任务
	docTask := taskqueue.NewTask(
		taskqueue.TaskTypeDocumentProcess,
		"test-file-1",
		"/test/path/document.pdf",
		map[string]interface{}{
			"test_meta": "value",
			"priority":  "high",
		},
	)

	// 验证任务属性
	assert.Equal(t, taskqueue.TaskTypeDocumentProcess, docTask.Type, "Task type should match")
	assert.Equal(t, "test-file-1", docTask.FileID, "File ID should match")
	assert.Equal(t, "/test/path/document.pdf", docTask.FilePath, "File path should match")
	assert.Equal(t, taskqueue.TaskStatusPending, docTask.Status, "Task should be in pending status")
	assert.NotEmpty(t, docTask.ID, "Task ID should not be empty")
	assert.NotZero(t, docTask.CreatedAt, "Task creation time should be set")
	assert.Equal(t, "high", docTask.Metadata["priority"], "Task metadata should be set correctly")

	// 创建嵌入向量生成任务
	embedTask := taskqueue.NewTask(
		taskqueue.TaskTypeEmbeddingGenerate,
		"test-file-2",
		"",
		nil,
	)

	// 验证任务属性
	assert.Equal(t, taskqueue.TaskTypeEmbeddingGenerate, embedTask.Type, "Task type should match")
	assert.Equal(t, "test-file-2", embedTask.FileID, "File ID should match")
	assert.Equal(t, taskqueue.TaskStatusPending, embedTask.Status, "Task should be in pending status")
}

// TestTaskEnqueueAndGet 测试任务入队和获取
func TestTaskEnqueueAndGet(t *testing.T) {
	// 创建测试环境
	queue, logger := setupTaskQueueTest(t)
	defer queue.Close()
	logger.Info("Testing task enqueue and retrieval")

	// 创建测试任务
	task := taskqueue.NewTask(
		taskqueue.TaskTypeDocumentProcess,
		"test-file-3",
		"/test/path/document.txt",
		map[string]interface{}{
			"test_time": time.Now().Format(time.RFC3339),
		},
	)

	// 将任务入队
	taskID, err := queue.Enqueue(task)
	require.NoError(t, err, "Failed to enqueue task")
	require.NotEmpty(t, taskID, "Task ID should not be empty")

	// 通过ID获取任务
	ctx := context.Background()
	retrievedTask, err := queue.GetTask(ctx, taskID)
	require.NoError(t, err, "Failed to retrieve task")

	// 验证获取的任务属性与原始任务相符
	assert.Equal(t, task.ID, retrievedTask.ID, "Task ID should match")
	assert.Equal(t, task.Type, retrievedTask.Type, "Task type should match")
	assert.Equal(t, task.FileID, retrievedTask.FileID, "File ID should match")
	assert.Equal(t, task.FilePath, retrievedTask.FilePath, "File path should match")
	assert.Equal(t, task.Status, retrievedTask.Status, "Task status should match")
	assert.Equal(t, task.Metadata["test_time"], retrievedTask.Metadata["test_time"], "Task metadata should match")
}

// TestTaskStatusUpdates 测试任务状态更新
func TestTaskStatusUpdates(t *testing.T) {
	// 创建测试环境
	queue, logger := setupTaskQueueTest(t)
	defer queue.Close()
	logger.Info("Testing task status updates")

	// 创建测试任务
	task := taskqueue.NewTask(
		taskqueue.TaskTypeDocumentProcess,
		"test-file-4",
		"/test/path/status-test.pdf",
		nil,
	)

	// 将任务入队
	taskID, err := queue.Enqueue(task)
	require.NoError(t, err, "Failed to enqueue task")

	ctx := context.Background()

	// 标记任务为处理中
	err = queue.MarkProcessing(ctx, taskID)
	require.NoError(t, err, "Failed to mark task as processing")

	// 验证状态更新
	updatedTask, err := queue.GetTask(ctx, taskID)
	require.NoError(t, err, "Failed to get updated task")
	assert.Equal(t, taskqueue.TaskStatusProcessing, updatedTask.Status, "Task status should be processing")

	// 标记任务为完成
	result := map[string]interface{}{
		"success":       true,
		"processed_at":  time.Now().Format(time.RFC3339),
		"segment_count": 5,
	}
	err = queue.MarkComplete(ctx, taskID, result)
	require.NoError(t, err, "Failed to mark task as completed")

	// 验证任务完成状态和结果
	completedTask, err := queue.GetTask(ctx, taskID)
	require.NoError(t, err, "Failed to get completed task")
	assert.Equal(t, taskqueue.TaskStatusCompleted, completedTask.Status, "Task status should be completed")
	assert.Equal(t, true, completedTask.Result["success"], "Task result should match")
	assert.Equal(t, 5, completedTask.Result["segment_count"], "Segment count should match")

	// 创建一个新任务用于测试失败状态
	failTask := taskqueue.NewTask(
		taskqueue.TaskTypeDocumentProcess,
		"test-file-fail",
		"/test/path/fail-test.pdf",
		nil,
	)

	// 将失败任务入队
	failTaskID, err := queue.Enqueue(failTask)
	require.NoError(t, err, "Failed to enqueue fail task")

	// 标记任务为失败
	err = queue.MarkFailed(ctx, failTaskID, "Processing error: document format not supported")
	require.NoError(t, err, "Failed to mark task as failed")

	// 验证任务失败状态和错误信息
	failedTask, err := queue.GetTask(ctx, failTaskID)
	require.NoError(t, err, "Failed to get failed task")
	assert.Equal(t, taskqueue.TaskStatusFailed, failedTask.Status, "Task status should be failed")
	assert.Contains(t, failedTask.Error, "document format not supported", "Error message should match")
}

// TestTaskProcessingWithCallback 测试任务处理和回调
func TestTaskProcessingWithCallback(t *testing.T) {
	// 从环境变量获取API密钥
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping test: No API key provided in TONGYI_API_KEY environment variable")
	}

	// 创建测试环境
	queue, logger := setupTaskQueueTest(t)
	defer queue.Close()
	logger.Info("Testing task processing with callback")

	// 创建任务监视器
	monitor := taskqueue.NewTaskMonitor(queue, taskqueue.WithInterval(1*time.Second))

	// 用于回调的通道
	callbackCalled := make(chan bool, 1)

	// 注册文档处理任务的处理器
	monitor.RegisterHandler(taskqueue.TaskTypeDocumentProcess, func(ctx context.Context, task *taskqueue.Task) error {
		// 验证任务属性
		if task.Type == taskqueue.TaskTypeDocumentProcess && task.Status == taskqueue.TaskStatusCompleted {
			// 验证API处理结果
			if task.Result != nil && task.Result["segment_count"] != nil {
				callbackCalled <- true
			}
		}
		return nil
	})

	// 启动监视器
	monitor.Start()
	defer monitor.Stop()

	// 创建包含API调用测试的任务
	task := taskqueue.NewTask(
		taskqueue.TaskTypeDocumentProcess,
		"test-api-file",
		"/test/path/sample.txt",
		map[string]interface{}{
			"api_key": apiKey,
		},
	)

	// 将任务入队
	taskID, err := queue.Enqueue(task)
	require.NoError(t, err, "Failed to enqueue task with API key")

	ctx := context.Background()

	// 模拟任务处理完成
	// 在真实环境中，这部分应该由Python worker处理
	// 这里我们手动标记为完成，并提供结果
	result := map[string]interface{}{
		"segment_count": 3,
		"embedding_dim": 1024,
		"api_provider":  "tongyi",
	}
	err = queue.MarkComplete(ctx, taskID, result)
	require.NoError(t, err, "Failed to mark API task as completed")

	// 检查任务完成状态
	completedTask, err := queue.GetTask(ctx, taskID)
	require.NoError(t, err, "Failed to get API task")
	assert.Equal(t, taskqueue.TaskStatusCompleted, completedTask.Status, "API task status should be completed")

	// 等待回调被调用或超时
	select {
	case <-callbackCalled:
		// 回调被成功调用
		t.Log("Task callback was successfully called")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for task callback")
	}
}

// TestQueueImplementation 测试队列接口实现
func TestQueueImplementation(t *testing.T) {
	// 创建测试环境
	queue, logger := setupTaskQueueTest(t)
	defer queue.Close()
	logger.Info("Testing queue implementation")

	// 测试队列接口注册
	queueImpl, err := taskqueue.NewQueue("redis",
		taskqueue.WithMaxRetries(3),
		taskqueue.WithTimeout(5*time.Second),
	)

	// 这个测试应该失败，因为NewQueue需要更多的配置
	// 我们期望它返回错误
	assert.Error(t, err, "NewQueue should require configuration")
	assert.Nil(t, queueImpl, "Queue implementation should be nil")

	// 测试 Redis 队列实现的错误处理
	ctx := context.Background()
	_, err = queue.GetTask(ctx, "non-existent-task")
	assert.Error(t, err, "GetTask should return error for non-existent task")
	assert.Equal(t, taskqueue.ErrTaskNotFound, err, "Error should be ErrTaskNotFound")
}

// TestMultipleTaskEnqueueAndProcessing 测试多个任务的入队和处理
func TestMultipleTaskEnqueueAndProcessing(t *testing.T) {
	// 创建测试环境
	queue, logger := setupTaskQueueTest(t)
	defer queue.Close()
	logger.Info("Testing multiple task enqueue and processing")

	ctx := context.Background()
	taskIDs := make([]string, 5)

	// 创建并入队多个任务
	for i := 0; i < 5; i++ {
		task := taskqueue.NewTask(
			taskqueue.TaskTypeDocumentProcess,
			fmt.Sprintf("multi-test-file-%d", i),
			fmt.Sprintf("/test/path/doc%d.txt", i),
			map[string]interface{}{
				"index": i,
			},
		)

		taskID, err := queue.Enqueue(task)
		require.NoError(t, err, "Failed to enqueue task")
		taskIDs[i] = taskID
	}

	// 模拟处理多个任务
	for i, taskID := range taskIDs {
		// 先标记为处理中
		err := queue.MarkProcessing(ctx, taskID)
		require.NoError(t, err, "Failed to mark task as processing")

		// 根据索引的奇偶性模拟任务成功或失败
		if i%2 == 0 {
			// 偶数索引的任务标记为完成
			result := map[string]interface{}{
				"index":      i,
				"successful": true,
			}
			err = queue.MarkComplete(ctx, taskID, result)
			require.NoError(t, err, "Failed to mark task as completed")

			// 验证任务状态
			task, err := queue.GetTask(ctx, taskID)
			require.NoError(t, err, "Failed to get task")
			assert.Equal(t, taskqueue.TaskStatusCompleted, task.Status, "Task should be completed")
			assert.Equal(t, i, task.Result["index"], "Task result index should match")
		} else {
			// 奇数索引的任务标记为失败
			err = queue.MarkFailed(ctx, taskID, fmt.Sprintf("Simulated error for task %d", i))
			require.NoError(t, err, "Failed to mark task as failed")

			// 验证任务状态
			task, err := queue.GetTask(ctx, taskID)
			require.NoError(t, err, "Failed to get task")
			assert.Equal(t, taskqueue.TaskStatusFailed, task.Status, "Task should be failed")
			assert.Contains(t, task.Error, fmt.Sprintf("Simulated error for task %d", i), "Task error message should match")
		}
	}
}
