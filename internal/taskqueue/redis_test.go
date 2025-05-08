package taskqueue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 创建Redis队列用于测试
func setupRedisQueue(t *testing.T) (*RedisQueue, func()) {
	// Redis连接配置
	config := RedisQueueConfig{
		Address:  "localhost:6379",
		Password: "",
		DB:       1,
		Prefix:   "test_queue:",
		Timeout:  5 * time.Second,
	}

	// 创建Redis队列
	queue, err := NewRedisQueue(config)
	if err != nil {
		t.Skip("Skipping Redis tests: Redis not available. Run docker-compose up -d")
		return nil, func() {}
	}

	// 返回队列和清理函数
	return queue, func() {
		ctx := context.Background()
		// 清理所有测试key
		keys, err := queue.client.Keys(ctx, config.Prefix+"*").Result()
		if err == nil && len(keys) > 0 {
			queue.client.Del(ctx, keys...)
		}
		queue.Close()
	}
}

// 测试Enqueue功能
func TestRedisQueue_Enqueue(t *testing.T) {
	queue, cleanup := setupRedisQueue(t)
	if queue == nil {
		return // Redis不可用
	}
	defer cleanup()

	// 创建任务
	task := NewTask(
		TaskTypeDocumentProcess,
		"test-file-id",
		"/path/to/file.pdf",
		map[string]interface{}{
			"created_by": "test",
			"priority":   "high",
		},
	)

	// 入队任务
	taskID, err := queue.Enqueue(task)
	require.NoError(t, err, "Failed to enqueue task")
	require.NotEmpty(t, taskID, "Task ID should not be empty")
}

// 测试获取任务功能
func TestRedisQueue_GetTask(t *testing.T) {
	queue, cleanup := setupRedisQueue(t)
	if queue == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// 创建并入队任务
	task := NewTask(
		TaskTypeDocumentProcess,
		"get-task-file-id",
		"/path/to/get-task-file.pdf",
		map[string]interface{}{
			"created_by": "test",
		},
	)

	taskID, err := queue.Enqueue(task)
	require.NoError(t, err)

	// 获取任务
	retrievedTask, err := queue.GetTask(ctx, taskID)
	require.NoError(t, err, "Failed to get task")

	// 验证任务数据
	assert.Equal(t, task.ID, retrievedTask.ID)
	assert.Equal(t, task.Type, retrievedTask.Type)
	assert.Equal(t, task.FileID, retrievedTask.FileID)
	assert.Equal(t, task.FilePath, retrievedTask.FilePath)
	assert.Equal(t, TaskStatusPending, retrievedTask.Status)
	assert.Equal(t, "test", retrievedTask.Metadata["created_by"])
}

// 测试状态更新操作
func TestRedisQueue_StatusUpdates(t *testing.T) {
	queue, cleanup := setupRedisQueue(t)
	if queue == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// 创建并入队任务
	task := NewTask(
		TaskTypeDocumentProcess,
		"status-test-file",
		"/path/to/status-test-file.pdf",
		nil,
	)

	taskID, err := queue.Enqueue(task)
	require.NoError(t, err)

	// 测试标记为处理中
	t.Run("MarkProcessing", func(t *testing.T) {
		err = queue.MarkProcessing(ctx, taskID)
		require.NoError(t, err)

		// 验证状态
		updatedTask, err := queue.GetTask(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, TaskStatusProcessing, updatedTask.Status)
	})

	// 测试标记为完成
	t.Run("MarkComplete", func(t *testing.T) {
		result := map[string]interface{}{
			"segments_count":  42,
			"processing_time": 2.5,
		}
		err = queue.MarkComplete(ctx, taskID, result)
		require.NoError(t, err)

		// 验证状态和结果
		completedTask, err := queue.GetTask(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, TaskStatusCompleted, completedTask.Status)
		assert.Equal(t, float64(42), completedTask.Result["segments_count"])
		assert.Equal(t, 2.5, completedTask.Result["processing_time"])
	})

	// 测试标记为失败
	t.Run("MarkFailed", func(t *testing.T) {
		failTask := NewTask(
			TaskTypeDocumentProcess,
			"failure-test-file",
			"/path/to/failure-test-file.pdf",
			nil,
		)
		failTaskID, err := queue.Enqueue(failTask)
		require.NoError(t, err)

		// 标记为失败并附加错误信息
		errorMsg := "File format not supported"
		err = queue.MarkFailed(ctx, failTaskID, errorMsg)
		require.NoError(t, err)

		// 验证状态和错误
		failedTask, err := queue.GetTask(ctx, failTaskID)
		require.NoError(t, err)
		assert.Equal(t, TaskStatusFailed, failedTask.Status)
		assert.Equal(t, errorMsg, failedTask.Error)
	})
}

// 测试错误处理
func TestRedisQueue_ErrorConditions(t *testing.T) {
	queue, cleanup := setupRedisQueue(t)
	if queue == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// 测试获取不存在的任务
	t.Run("GetNonExistentTask", func(t *testing.T) {
		_, err := queue.GetTask(ctx, "non-existent-task-id")
		assert.Error(t, err)
		assert.Equal(t, ErrTaskNotFound, err)
	})

	// 测试更新不存在的任务
	t.Run("UpdateNonExistentTask", func(t *testing.T) {
		err := queue.MarkProcessing(ctx, "non-existent-task-id")
		assert.Error(t, err)
	})

	// 测试入队nil任务
	t.Run("EnqueueNilTask", func(t *testing.T) {
		_, err := queue.Enqueue(nil)
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidTask, err)
	})

	// 测试关闭队列后操作
	t.Run("ClosedQueue", func(t *testing.T) {
		tempQueue, _ := setupRedisQueue(t)
		if tempQueue == nil {
			return
		}

		err := tempQueue.Close()
		require.NoError(t, err)

		_, err = tempQueue.Enqueue(NewTask(TaskTypeDocumentProcess, "test", "test", nil))
		assert.Error(t, err)
		assert.Equal(t, ErrQueueClosed, err)
	})
}

// 测试队列工厂函数
func TestRedisQueue_NewQueueFactory(t *testing.T) {
	// 使用工厂模式测试队列创建
	t.Run("NewQueueWithRedis", func(t *testing.T) {
		// 首先注册实现（如未注册）
		RegisterQueueImplementation("redis-test", func(options QueueOptions) (Queue, error) {
			config := RedisQueueConfig{
				Address:  "localhost:6379",
				Password: "",
				DB:       1,
				Prefix:   "test_factory:",
				Timeout:  options.Timeout,
			}
			return NewRedisQueue(config)
		})

		// 使用工厂创建队列
		queue, err := NewQueue("redis-test", WithTimeout(10*time.Second))
		if err != nil {
			t.Skip("Skipping test: Redis not available")
			return
		}
		defer queue.Close()

		assert.NotNil(t, queue, "Queue should be created")
	})
}
