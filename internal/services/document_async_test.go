package services

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAsyncTestEnv 创建用于测试异步文档处理的环境
func setupAsyncTestEnv(t *testing.T, tempDir string) (*DocumentService, *DocumentStatusManager, taskqueue.Queue) {
	// 设置数据库
	_, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)

	// 创建任务队列
	queueConfig := &taskqueue.Config{
		RedisAddr:   "localhost:6379",
		RedisDB:     15, // 使用 DB 15 进行测试
		RetryLimit:  2,
		RetryDelay:  time.Second,
		Concurrency: 2,
	}
	taskQueue, err := taskqueue.NewRedisQueue(queueConfig)
	if err != nil {
		t.Skip("Redis 不可用，跳过异步处理测试:", err)
		return nil, nil, nil
	}
	t.Cleanup(func() {
		taskQueue.Close()
	})

	// 创建文档仓储和状态管理器
	repo := repository.NewDocumentRepositoryWithQueue(nil, taskQueue)
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	statusManager := NewDocumentStatusManager(repo, logger)

	// 创建存储服务
	storageConfig := storage.LocalConfig{
		Path: tempDir,
	}
	storageService, err := storage.NewLocalStorage(storageConfig)
	require.NoError(t, err)

	// 创建文本分割器
	splitterConfig := document.DefaultSplitterConfig()
	splitterConfig.ChunkSize = 100 // 小块用于测试
	textSplitter, err := document.NewTextSplitter(splitterConfig)
	require.NoError(t, err)

	// 创建嵌入客户端
	embeddingClient := &testEmbeddingClient{dimension: 4}

	// 创建向量数据库
	vectorDBConfig := vectordb.Config{
		Type:      "memory",
		Dimension: 4,
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(t, err)

	// 创建文档服务
	docService := NewDocumentService(
		storageService,
		&testParser{},
		textSplitter,
		embeddingClient,
		vectorDB,
		WithBatchSize(2),
		WithTimeout(5*time.Second),
		WithDocumentRepository(repo),
		WithStatusManager(statusManager),
		WithLogger(logger),
	)

	return docService, statusManager, taskQueue
}

// createTestDocument 创建测试文档记录和文件
func createTestDocument(t *testing.T, tempDir string, statusManager *DocumentStatusManager) (string, string) {
	// 创建测试内容和文件
	testContent := "This is a test document for async processing.\n\nIt contains multiple paragraphs.\n\nEach paragraph should be processed separately."
	fileName := "test_async_doc.txt"
	filePath := filepath.Join(tempDir, fileName)
	err := ioutil.WriteFile(filePath, []byte(testContent), 0644)
	require.NoError(t, err)

	// 生成文档 ID
	docID := "test-async-doc-" + time.Now().Format("150405")

	// 创建文档记录
	ctx := context.Background()
	err = statusManager.MarkAsUploaded(ctx, docID, fileName, filePath, int64(len(testContent)))
	require.NoError(t, err)

	return docID, filePath
}

// TestEnableDisableAsyncProcessing 测试启用和禁用异步处理
func TestEnableDisableAsyncProcessing(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-async-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, _, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 测试启用异步处理
	t.Run("enable async processing", func(t *testing.T) {
		// 初始状态应为未启用异步处理
		assert.False(t, docService.asyncEnabled)
		assert.Nil(t, docService.taskQueue)

		// 启用异步处理
		docService.EnableAsyncProcessing(taskQueue)

		// 检查是否启用了异步处理
		assert.True(t, docService.asyncEnabled)
		assert.NotNil(t, docService.taskQueue)
	})

	// 测试禁用异步处理
	t.Run("disable async processing", func(t *testing.T) {
		// 确保已启用
		docService.EnableAsyncProcessing(taskQueue)

		// 然后禁用
		docService.DisableAsyncProcessing()

		// 检查是否禁用了异步处理
		assert.False(t, docService.asyncEnabled)
		// 任务队列引用应保留
		assert.NotNil(t, docService.taskQueue)
	})
}

// TestProcessDocumentAsync 测试异步文档处理
func TestProcessDocumentAsync(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-async-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, filePath := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 异步处理文档
	ctx := context.Background()
	err = docService.ProcessDocumentAsync(ctx, docID, filePath)
	require.NoError(t, err)

	// 检查文档状态是否更改为处理中
	status, err := statusManager.GetStatus(ctx, docID)
	require.NoError(t, err)
	assert.Equal(t, models.DocStatusProcessing, status)

	// 获取文档的任务
	tasks, err := taskQueue.GetTasksByDocument(ctx, docID)
	require.NoError(t, err)
	assert.NotEmpty(t, tasks, "预期至少创建一个任务")

	// 检查任务类型是否正确
	assert.Equal(t, taskqueue.TaskProcessComplete, tasks[0].Type)
}

// TestProcessDocumentAsyncWithOptions 测试带选项的异步文档处理
func TestProcessDocumentAsyncWithOptions(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-async-options-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, filePath := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 使用自定义选项异步处理文档
	ctx := context.Background()
	err = docService.ProcessDocumentAsync(
		ctx,
		docID,
		filePath,
		WithChunkSize(200),
		WithChunkOverlap(50),
		WithSplitType("sentence"),
		WithEmbeddingModel("test-model"),
		WithMetadata(map[string]string{"source": "test"}),
		WithPriority("high"),
	)
	require.NoError(t, err)

	// 获取任务并验证负载包含正确的选项
	tasks, err := taskQueue.GetTasksByDocument(ctx, docID)
	require.NoError(t, err)
	require.NotEmpty(t, tasks)

	// 使用类型断言将负载 JSON 转换为预期结构
	var payload taskqueue.ProcessCompletePayload
	err = json.Unmarshal(tasks[0].Payload, &payload)
	require.NoError(t, err)

	// 检查选项是否正确传递到负载
	assert.Equal(t, 200, payload.ChunkSize)
	assert.Equal(t, 50, payload.Overlap)
	assert.Equal(t, "sentence", payload.SplitType)
	assert.Equal(t, "test-model", payload.Model)
	assert.Equal(t, map[string]string{"source": "test"}, payload.Metadata)
}

// TestDocumentParseCallback 测试文档解析回调处理
func TestDocumentParseCallback(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-parse-callback-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, _ := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 标记文档为处理中状态
	ctx := context.Background()
	err = statusManager.MarkAsProcessing(ctx, docID)
	require.NoError(t, err)

	// 创建模拟任务
	task := &taskqueue.Task{
		ID:         "test-parse-task-id",
		Type:       taskqueue.TaskDocumentParse,
		DocumentID: docID,
		Status:     taskqueue.StatusCompleted,
	}

	// 创建结果数据
	result := taskqueue.DocumentParseResult{
		Content: "Test content for parsing",
		Title:   "Test Document",
		Words:   5,
		Chars:   24,
	}

	// 转换为 JSON
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)

	// 调用处理函数
	err = docService.handleDocumentParseResult(ctx, task, resultJSON)
	require.NoError(t, err)

	// 验证进度是否更新
	doc, err := statusManager.GetDocument(ctx, docID)
	require.NoError(t, err)
	assert.Greater(t, doc.Progress, 0)
}

// TestTextChunkCallback 测试文本分块回调处理
func TestTextChunkCallback(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-chunk-callback-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, _ := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 标记文档为处理中状态
	ctx := context.Background()
	err = statusManager.MarkAsProcessing(ctx, docID)
	require.NoError(t, err)

	// 创建模拟任务
	task := &taskqueue.Task{
		ID:         "test-chunk-task-id",
		Type:       taskqueue.TaskTextChunk,
		DocumentID: docID,
		Status:     taskqueue.StatusCompleted,
	}

	// 创建结果数据
	result := taskqueue.TextChunkResult{
		DocumentID: docID,
		Chunks: []taskqueue.ChunkInfo{
			{Text: "Chunk 1", Index: 0},
			{Text: "Chunk 2", Index: 1},
		},
		ChunkCount: 2,
	}

	// 转换为 JSON
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)

	// 调用处理函数
	err = docService.handleTextChunkResult(ctx, task, resultJSON)
	require.NoError(t, err)

	// 验证进度是否更新
	doc, err := statusManager.GetDocument(ctx, docID)
	require.NoError(t, err)
	assert.Greater(t, doc.Progress, 30)
}

// TestVectorizeCallback 测试向量化回调处理
func TestVectorizeCallback(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-vectorize-callback-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, _ := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 标记文档为处理中状态
	ctx := context.Background()
	err = statusManager.MarkAsProcessing(ctx, docID)
	require.NoError(t, err)

	// 创建模拟任务
	task := &taskqueue.Task{
		ID:         "test-vector-task-id",
		Type:       taskqueue.TaskVectorize,
		DocumentID: docID,
		Status:     taskqueue.StatusCompleted,
	}

	// 创建结果数据
	result := taskqueue.VectorizeResult{
		DocumentID: docID,
		Vectors: []taskqueue.VectorInfo{
			{ChunkIndex: 0, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
			{ChunkIndex: 1, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
		},
		VectorCount: 2,
		Model:       "test-model",
		Dimension:   4,
	}

	// 转换为 JSON
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)

	// 调用处理函数
	err = docService.handleVectorizeResult(ctx, task, resultJSON)
	require.NoError(t, err)

	// 验证文档是否标记为已完成
	status, err := statusManager.GetStatus(ctx, docID)
	require.NoError(t, err)
	assert.Equal(t, models.DocStatusCompleted, status)
}

// TestProcessCompleteCallback 测试处理完成回调处理
func TestProcessCompleteCallback(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-complete-callback-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, _ := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 标记文档为处理中状态
	ctx := context.Background()
	err = statusManager.MarkAsProcessing(ctx, docID)
	require.NoError(t, err)

	// 创建模拟任务
	task := &taskqueue.Task{
		ID:         "test-complete-task-id",
		Type:       taskqueue.TaskProcessComplete,
		DocumentID: docID,
		Status:     taskqueue.StatusCompleted,
	}

	// 创建结果数据
	result := taskqueue.ProcessCompleteResult{
		DocumentID:   docID,
		ChunkCount:   3,
		VectorCount:  3,
		Dimension:    4,
		ParseStatus:  "success",
		ChunkStatus:  "success",
		VectorStatus: "success",
	}

	// 转换为 JSON
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)

	// 调用处理函数
	err = docService.handleProcessCompleteResult(ctx, task, resultJSON)
	require.NoError(t, err)

	// 验证文档是否标记为已完成
	status, err := statusManager.GetStatus(ctx, docID)
	require.NoError(t, err)
	assert.Equal(t, models.DocStatusCompleted, status)
}

// TestWaitForDocumentProcessing 测试文档处理等待机制
func TestWaitForDocumentProcessing(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-wait-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, filePath := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 异步处理文档
	ctx := context.Background()
	err = docService.ProcessDocumentAsync(ctx, docID, filePath)
	require.NoError(t, err)

	// 尝试使用短超时时间等待 - 应超时
	err = docService.WaitForDocumentProcessing(ctx, docID, 100*time.Millisecond)
	assert.Error(t, err, "预期超时错误")

	// 修改任务和文档状态来模拟处理完成
	tasks, err := taskQueue.GetTasksByDocument(ctx, docID)
	require.NoError(t, err)
	require.NotEmpty(t, tasks)

	// 更新任务状态为已完成
	taskID := tasks[0].ID
	result := taskqueue.ProcessCompleteResult{
		DocumentID:  docID,
		ChunkCount:  2,
		VectorCount: 2,
	}
	resultJSON, _ := json.Marshal(result)

	err = taskQueue.UpdateTaskStatus(ctx, taskID, taskqueue.StatusCompleted, resultJSON, "")
	require.NoError(t, err)

	// 通知任务更新
	err = taskQueue.NotifyTaskUpdate(ctx, taskID)
	require.NoError(t, err)

	// 将文档标记为已完成
	err = statusManager.MarkAsCompleted(ctx, docID, 2)
	require.NoError(t, err)

	// 再次等待，现在应该成功
	err = docService.WaitForDocumentProcessing(ctx, docID, 2*time.Second)
	assert.NoError(t, err, "当文档已完成时等待应成功")
}

// TestGetDocumentTasks 测试获取文档任务
func TestGetDocumentTasks(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-tasks-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, filePath := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 异步处理文档
	ctx := context.Background()
	err = docService.ProcessDocumentAsync(ctx, docID, filePath)
	require.NoError(t, err)

	// 获取文档任务
	tasks, err := docService.GetDocumentTasks(ctx, docID)
	require.NoError(t, err)
	assert.NotEmpty(t, tasks, "应返回文档的任务")
}

// TestWaitForTaskResult 测试等待任务结果
func TestWaitForTaskResult(t *testing.T) {
	// 创建临时目录
	tempDir, err := ioutil.TempDir("", "docqa-task-result-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, statusManager, taskQueue := setupAsyncTestEnv(t, tempDir)
	if docService == nil {
		t.Skip("测试环境设置失败，跳过测试")
	}

	// 创建测试文档
	docID, _ := createTestDocument(t, tempDir, statusManager)

	// 启用异步处理
	docService.EnableAsyncProcessing(taskQueue)

	// 创建测试任务
	ctx := context.Background()
	taskID, err := taskQueue.Enqueue(ctx, taskqueue.TaskProcessComplete, docID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// 模拟任务完成
	result := taskqueue.ProcessCompleteResult{DocumentID: docID}
	resultJSON, _ := json.Marshal(result)
	err = taskQueue.UpdateTaskStatus(ctx, taskID, taskqueue.StatusCompleted, resultJSON, "")
	require.NoError(t, err)
	err = taskQueue.NotifyTaskUpdate(ctx, taskID)
	require.NoError(t, err)

	// 等待任务结果
	task, err := docService.WaitForTaskResult(ctx, taskID, 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, taskqueue.StatusCompleted, task.Status)
}
