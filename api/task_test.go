package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/fyerfyer/doc-QA-system/api/handler"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTaskHandlerTest(t *testing.T) (*handler.TaskHandler, taskqueue.Queue, *gin.Engine, func()) {
	// 设置 Gin 为测试模式
	gin.SetMode(gin.TestMode)

	// 创建 Redis 队列
	queueConfig := &taskqueue.Config{
		RedisAddr:  "localhost:6379",
		RedisDB:    15, // 使用 DB 15 进行测试
		RetryLimit: 2,
		RetryDelay: time.Second,
	}

	queue, err := taskqueue.NewRedisQueue(queueConfig)
	if err != nil {
		t.Skip("Redis not available, skipping task handler tests:", err)
		return nil, nil, nil, func() {}
	}

	// 创建任务处理程序
	taskHandler := handler.NewTaskHandler(queue)

	// 设置路由器
	router := gin.New()
	router.Use(gin.Recovery())

	// 注册路由
	router.POST("/api/tasks/callback", taskHandler.HandleCallback)
	router.GET("/api/tasks/:id", taskHandler.GetTaskStatus)
	router.GET("/api/document/:document_id/tasks", taskHandler.GetDocumentTasks)

	// 返回清理函数
	cleanup := func() {
		queue.Close()
	}

	return taskHandler, queue, router, cleanup
}

// TestTaskHandlerCallback 测试回调端点
func TestTaskHandlerCallback(t *testing.T) {
	handler, queue, router, cleanup := setupTaskHandlerTest(t)
	if handler == nil {
		return // Redis not available
	}
	defer cleanup()

	// 首先创建一个测试任务
	ctx := context.Background()
	documentID := "test-document"
	taskType := taskqueue.TaskDocumentParse
	payload := map[string]string{"test": "data"}

	taskID, err := queue.Enqueue(ctx, taskType, documentID, payload)
	require.NoError(t, err, "Failed to enqueue task")

	// 创建回调请求
	callbackReq := taskqueue.CallbackRequest{
		TaskID:     taskID,
		DocumentID: documentID,
		Status:     taskqueue.StatusCompleted,
		Type:       taskType,
		Result:     json.RawMessage(`{"result":"success"}`),
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	// 转换为 JSON
	jsonData, err := json.Marshal(callbackReq)
	require.NoError(t, err, "Failed to marshal callback request")

	// 创建 HTTP 请求
	req, _ := http.NewRequest(http.MethodPost, "/api/tasks/callback", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	// 执行请求
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, resp.Code)

	// 验证任务状态已更新
	task, err := queue.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, taskqueue.StatusCompleted, task.Status)
}

// TestTaskHandlerGetStatus 测试获取任务状态
func TestTaskHandlerGetStatus(t *testing.T) {
	handler, queue, router, cleanup := setupTaskHandlerTest(t)
	if handler == nil {
		return // Redis not available
	}
	defer cleanup()

	// 创建一个测试任务
	ctx := context.Background()
	documentID := "test-document"
	taskType := taskqueue.TaskDocumentParse
	payload := map[string]string{"test": "data"}

	taskID, err := queue.Enqueue(ctx, taskType, documentID, payload)
	require.NoError(t, err, "Failed to enqueue task")

	// 创建 HTTP 请求
	req, _ := http.NewRequest(http.MethodGet, "/api/tasks/"+taskID, nil)

	// 执行请求
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, resp.Code)

	// 验证响应中的任务信息
	taskInfo, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok, "Response data should be a map")
	assert.Equal(t, taskID, taskInfo["id"])
	assert.Equal(t, string(taskType), taskInfo["type"])
	assert.Equal(t, documentID, taskInfo["document_id"])
}

// TestTaskHandlerGetDocumentTasks 测试获取文档的任务
func TestTaskHandlerGetDocumentTasks(t *testing.T) {
	handler, queue, router, cleanup := setupTaskHandlerTest(t)
	if handler == nil {
		return // Redis not available
	}
	defer cleanup()

	// 为同一文档创建多个测试任务
	ctx := context.Background()
	documentID := "test-document-tasks"

	// 创建第一个任务
	_, err := queue.Enqueue(ctx, taskqueue.TaskDocumentParse, documentID, map[string]string{"test": "data1"})
	require.NoError(t, err, "Failed to enqueue first task")

	// 创建第二个任务
	_, err = queue.Enqueue(ctx, taskqueue.TaskTextChunk, documentID, map[string]string{"test": "data2"})
	require.NoError(t, err, "Failed to enqueue second task")

	// 创建 HTTP 请求
	req, _ := http.NewRequest(http.MethodGet, "/api/document/"+documentID+"/tasks", nil)

	// 执行请求
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, resp.Code)

	// 验证响应中的文档任务
	respData, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok, "Response data should be a map")
	assert.Equal(t, documentID, respData["document_id"])

	tasks, ok := respData["tasks"].([]interface{})
	assert.True(t, ok, "Tasks should be an array")
	assert.Equal(t, 2, len(tasks), "Should have two tasks")
}

// TestTaskHandlerInvalidTaskStatus 测试获取不存在任务的状态
func TestTaskHandlerInvalidTaskStatus(t *testing.T) {
	handler, _, router, cleanup := setupTaskHandlerTest(t)
	if handler == nil {
		return // Redis not available
	}
	defer cleanup()

	// 创建带有不存在任务 ID 的 HTTP 请求
	req, _ := http.NewRequest(http.MethodGet, "/api/tasks/non-existent-task", nil)

	// 执行请求
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 检查响应 - 应为 404 Not Found
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 验证错误响应
	var resp model.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.NotEqual(t, 0, resp.Code) // Error code should not be 0
}

// TestTaskHandlerInvalidCallback 测试处理无效的回调请求
func TestTaskHandlerInvalidCallback(t *testing.T) {
	handler, _, router, cleanup := setupTaskHandlerTest(t)
	if handler == nil {
		return // Redis not available
	}
	defer cleanup()

	// 创建无效的 JSON
	invalidJSON := []byte(`{"taskId": "missing-required-fields"}`)

	// 创建 HTTP 请求
	req, _ := http.NewRequest(http.MethodPost, "/api/tasks/callback", bytes.NewBuffer(invalidJSON))
	req.Header.Set("Content-Type", "application/json")

	// 执行请求
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 检查响应 - 应为 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证错误响应
	var resp model.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.NotEqual(t, 0, resp.Code) // Error code should not be 0
}

// TestTaskHandlerEmptyDocumentTasks 测试获取没有任务的文档的任务
func TestTaskHandlerEmptyDocumentTasks(t *testing.T) {
	handler, _, router, cleanup := setupTaskHandlerTest(t)
	if handler == nil {
		return // Redis not available
	}
	defer cleanup()

	// 创建带有没有任务的文档 ID 的 HTTP 请求
	req, _ := http.NewRequest(http.MethodGet, "/api/document/empty-document/tasks", nil)

	// 执行请求
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 检查响应 - 应成功但任务为空
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, resp.Code)

	// 验证响应中的文档任务
	respData, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok, "Response data should be a map")
	assert.Equal(t, "empty-document", respData["document_id"])

	tasks, ok := respData["tasks"].([]interface{})
	assert.True(t, ok, "Tasks should be an array")
	assert.Equal(t, 0, len(tasks), "Should have zero tasks")
}
