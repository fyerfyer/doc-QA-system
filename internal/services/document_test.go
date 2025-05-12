package services

import (
	"context"
	"encoding/json"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
)

// Update setupDocumentTestEnv to initialize DB and return statusManager
func setupDocumentTestEnv(t *testing.T, tempDir string) (*DocumentService, vectordb.Repository, *DocumentStatusManager) {
	// Setup database first
	_, cleanup := setupTestDB(t)
	t.Cleanup(cleanup) // Ensure DB is cleaned up after test

	// Create repository and status manager
	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	statusManager := NewDocumentStatusManager(repo, logger)

	// Create storage service
	storageConfig := storage.LocalConfig{
		Path: tempDir,
	}
	storageService, err := storage.NewLocalStorage(storageConfig)
	require.NoError(t, err)

	// Create text splitter
	splitterConfig := document.DefaultSplitterConfig()
	splitterConfig.ChunkSize = 100 // Small chunks for testing
	textSplitter := document.NewTextSplitter(splitterConfig)

	// Create embedding client
	embeddingClient := &testEmbeddingClient{dimension: 4}

	// Create vector database
	vectorDBConfig := vectordb.Config{
		Type:      "memory",
		Dimension: 4,
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(t, err)

	// Create document service with status manager
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
	)

	return docService, vectorDB, statusManager
}

// setupTestDB 创建测试数据库环境
func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	// 使用临时文件作为测试数据库
	tempFile := "test_document_status.db"

	// 创建数据库连接
	db, err := gorm.Open(sqlite.Open(tempFile), &gorm.Config{})
	require.NoError(t, err, "Failed to connect to test database")

	// 运行迁移
	err = db.AutoMigrate(&models.Document{}, &models.DocumentSegment{})
	require.NoError(t, err, "Failed to run migrations")

	// 保存原始DB引用并替换
	originalDB := database.DB
	database.DB = db

	// 返回清理函数
	cleanup := func() {
		// 关闭连接
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		// 恢复原始DB引用
		database.DB = originalDB
		// 删除临时数据库文件
		os.Remove(tempFile)
	}

	return db, cleanup
}

// TestDocumentService 测试文档服务的基本功能
func TestDocumentService(t *testing.T) {
	// Create temp directory and file
	tempDir, err := ioutil.TempDir("", "docqa-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testContent := "这是一个测试文档内容。\n\n这是第二段落。\n\n这是第三段落。"
	testFile := filepath.Join(tempDir, "test.txt")
	err = ioutil.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Initialize test environment and services
	docService, vectorDB, statusManager := setupDocumentTestEnv(t, tempDir)

	// Create a document record in the database first!
	ctx := context.Background()
	fileID := "test-file-id"
	fileName := filepath.Base(testFile)
	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err)

	// Mark document as uploaded before processing
	err = statusManager.MarkAsUploaded(ctx, fileID, fileName, testFile, fileInfo.Size())
	require.NoError(t, err, "Failed to create initial document record")

	// Now process the document
	err = docService.ProcessDocument(ctx, fileID, testFile)
	require.NoError(t, err, "Document processing should succeed")

	// Rest of your test remains the same...
	filter := vectordb.SearchFilter{
		FileIDs:    []string{fileID},
		MaxResults: 10,
	}
	queryVector := make([]float32, 4)
	results, err := vectorDB.Search(queryVector, filter)
	require.NoError(t, err)
	assert.Equal(t, 3, len(results), "There should be 3 paragraphs saved")

	// Continue with other assertions...
}

// TestProcessDocumentWithDifferentTypes 测试处理不同类型的文档
func TestProcessDocumentWithDifferentTypes(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "docqa-multitype-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 创建各种测试文件
	testFiles := map[string]string{
		"text.txt": "纯文本测试内容",
		"doc.md":   "# 标题\n\n这是**Markdown**文件",
	}

	createdFiles := make(map[string]string)
	for name, content := range testFiles {
		filePath := filepath.Join(tempDir, name)
		err = ioutil.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)
		createdFiles[name] = filePath
	}

	// 初始化服务
	docService, vectorDB, _ := setupDocumentTestEnv(t, tempDir)
	ctx := context.Background()

	// 测试处理不同类型的文件
	for name, path := range createdFiles {
		fileID := "file-" + name
		err = docService.ProcessDocument(ctx, fileID, path)
		require.NoError(t, err, "Processing %s should succeed", name)

		// 验证向量库中是否存在该文件的段落
		filter := vectordb.SearchFilter{
			FileIDs:    []string{fileID},
			MaxResults: 10,
		}
		queryVector := make([]float32, 4)
		results, err := vectorDB.Search(queryVector, filter)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find paragraphs for file %s", name)
	}
}

// TestDocumentStatusManager_BasicFlow 测试文档状态管理基本流程
func TestDocumentStatusManager_BasicFlow(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	// 创建文档仓储
	repo := repository.NewDocumentRepository()

	// 创建状态管理器
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	statusManager := NewDocumentStatusManager(repo, logger)

	ctx := context.Background()
	docID := "test-doc-1"
	fileName := "test.pdf"
	filePath := "/path/to/test.pdf"
	fileSize := int64(1024)

	// 测试标记为已上传
	t.Run("mark as uploaded", func(t *testing.T) {
		err := statusManager.MarkAsUploaded(ctx, docID, fileName, filePath, fileSize)
		require.NoError(t, err)

		// 验证状态
		status, err := statusManager.GetStatus(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusUploaded, status)

		// 验证文档信息
		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, docID, doc.ID)
		assert.Equal(t, fileName, doc.FileName)
		assert.Equal(t, "pdf", doc.FileType)
		assert.Equal(t, filePath, doc.FilePath)
		assert.Equal(t, fileSize, doc.FileSize)
		assert.Equal(t, 0, doc.Progress)
	})

	// 测试标记为处理中
	t.Run("mark as processing", func(t *testing.T) {
		err := statusManager.MarkAsProcessing(ctx, docID)
		require.NoError(t, err)

		status, err := statusManager.GetStatus(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusProcessing, status)
	})

	// 测试更新进度
	t.Run("update progress", func(t *testing.T) {
		err := statusManager.UpdateProgress(ctx, docID, 50)
		require.NoError(t, err)

		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, 50, doc.Progress)
	})

	// 测试标记为已完成
	t.Run("mark as completed", func(t *testing.T) {
		segmentCount := 5
		err := statusManager.MarkAsCompleted(ctx, docID, segmentCount)
		require.NoError(t, err)

		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusCompleted, doc.Status)
		assert.Equal(t, segmentCount, doc.SegmentCount)
		assert.Equal(t, 100, doc.Progress)
		assert.NotNil(t, doc.ProcessedAt)
	})
}

// TestDocumentStatusManager_FailureFlow 测试失败状态处理
func TestDocumentStatusManager_FailureFlow(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	statusManager := NewDocumentStatusManager(repo, logger)

	ctx := context.Background()
	docID := "test-doc-2"
	fileName := "fail_test.pdf"
	filePath := "/path/to/fail_test.pdf"

	// 创建文档
	err := statusManager.MarkAsUploaded(ctx, docID, fileName, filePath, 2048)
	require.NoError(t, err)

	// 标记为处理中
	err = statusManager.MarkAsProcessing(ctx, docID)
	require.NoError(t, err)

	// 标记为失败
	t.Run("mark as failed", func(t *testing.T) {
		errorMsg := "Processing error: unsupported format"
		err := statusManager.MarkAsFailed(ctx, docID, errorMsg)
		require.NoError(t, err)

		// 验证状态和错误信息
		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusFailed, doc.Status)
		assert.Equal(t, errorMsg, doc.Error)
		assert.NotNil(t, doc.ProcessedAt)
	})
}

// TestDocumentStatusManager_InvalidTransitions 测试无效的状态转换
func TestDocumentStatusManager_InvalidTransitions(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	statusManager := NewDocumentStatusManager(repo, logger)

	// 测试有效和无效的状态转换
	t.Run("validate state transitions", func(t *testing.T) {
		// 有效转换
		assert.NoError(t, statusManager.ValidateStateTransition(models.DocStatusUploaded, models.DocStatusProcessing))
		assert.NoError(t, statusManager.ValidateStateTransition(models.DocStatusProcessing, models.DocStatusCompleted))
		assert.NoError(t, statusManager.ValidateStateTransition(models.DocStatusProcessing, models.DocStatusFailed))
		assert.NoError(t, statusManager.ValidateStateTransition(models.DocStatusFailed, models.DocStatusProcessing)) // 允许重试

		// 无效转换
		assert.Error(t, statusManager.ValidateStateTransition(models.DocStatusCompleted, models.DocStatusProcessing))
		assert.Error(t, statusManager.ValidateStateTransition(models.DocStatusCompleted, models.DocStatusUploaded))
	})
}

// TestDocumentStatusManager_ListDocuments 测试文档列表功能
func TestDocumentStatusManager_ListDocuments(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	statusManager := NewDocumentStatusManager(repo, logger)

	ctx := context.Background()

	// 创建多个测试文档
	docs := []struct {
		ID     string
		Name   string
		Status models.DocumentStatus
		Tags   string
	}{
		{"list-doc-1", "doc1.pdf", models.DocStatusUploaded, "tag1,report"},
		{"list-doc-2", "doc2.pdf", models.DocStatusProcessing, "tag2,report"},
		{"list-doc-3", "doc3.pdf", models.DocStatusCompleted, "tag3"},
		{"list-doc-4", "doc4.pdf", models.DocStatusFailed, "tag4,report"},
	}

	// 添加测试文档
	for _, doc := range docs {
		err := statusManager.MarkAsUploaded(ctx, doc.ID, doc.Name, "/path/to/"+doc.Name, 1024)
		require.NoError(t, err)

		// 更新状态和标签
		if doc.Status != models.DocStatusUploaded {
			err = statusManager.MarkAsProcessing(ctx, doc.ID)
			require.NoError(t, err)
		}

		if doc.Status == models.DocStatusCompleted {
			err = statusManager.MarkAsCompleted(ctx, doc.ID, 3)
			require.NoError(t, err)
		} else if doc.Status == models.DocStatusFailed {
			err = statusManager.MarkAsFailed(ctx, doc.ID, "Test error")
			require.NoError(t, err)
		}

		// 更新标签
		dbDoc, err := repo.GetByID(doc.ID)
		require.NoError(t, err)
		dbDoc.Tags = doc.Tags
		err = repo.Update(dbDoc)
		require.NoError(t, err)
	}

	// 测试列出所有文档
	t.Run("list all documents", func(t *testing.T) {
		docList, total, err := statusManager.ListDocuments(ctx, 0, 10, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(len(docs)), total)
		assert.Len(t, docList, len(docs))
	})

	// 测试按状态筛选
	t.Run("filter by status", func(t *testing.T) {
		filters := map[string]interface{}{
			"status": string(models.DocStatusCompleted),
		}
		docList, total, err := statusManager.ListDocuments(ctx, 0, 10, filters)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		if len(docList) > 0 {
			assert.Equal(t, models.DocStatusCompleted, docList[0].Status)
		}
	})

	// 测试按标签筛选
	t.Run("filter by tags", func(t *testing.T) {
		filters := map[string]interface{}{
			"tags": "report",
		}
		_, total, err := statusManager.ListDocuments(ctx, 0, 10, filters)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total) // 应该找到3个带有report标签的文档
	})
}

// TestDocumentStatusManager_DeleteDocument 测试删除文档
func TestDocumentStatusManager_DeleteDocument(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	statusManager := NewDocumentStatusManager(repo, logger)

	ctx := context.Background()
	docID := "test-delete-doc"
	fileName := "delete_test.pdf"

	// 创建测试文档
	err := statusManager.MarkAsUploaded(ctx, docID, fileName, "/path/to/delete_test.pdf", 1024)
	require.NoError(t, err)

	// 确认文档存在
	_, err = statusManager.GetDocument(ctx, docID)
	require.NoError(t, err)

	// 删除文档
	err = statusManager.DeleteDocument(ctx, docID)
	require.NoError(t, err)

	// 验证文档已被删除
	_, err = statusManager.GetDocument(ctx, docID)
	assert.Error(t, err, "Document should be deleted")
}

// TestDocumentStatusManager_EdgeCases 测试边缘情况
func TestDocumentStatusManager_EdgeCases(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	statusManager := NewDocumentStatusManager(repo, logger)

	ctx := context.Background()

	// 测试获取不存在的文档
	t.Run("get non-existent document", func(t *testing.T) {
		_, err := statusManager.GetDocument(ctx, "non-existent-id")
		assert.Error(t, err)
	})

	// 测试无效的进度值
	t.Run("update progress with invalid values", func(t *testing.T) {
		docID := "progress-test-doc"
		err := statusManager.MarkAsUploaded(ctx, docID, "progress.pdf", "/path/to/progress.pdf", 1024)
		require.NoError(t, err)

		err = statusManager.MarkAsProcessing(ctx, docID)
		require.NoError(t, err)

		// 测试负进度值
		err = statusManager.UpdateProgress(ctx, docID, -10)
		require.NoError(t, err)
		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, 0, doc.Progress, "Negative progress should be adjusted to 0")

		// 测试超过100的进度值
		err = statusManager.UpdateProgress(ctx, docID, 150)
		require.NoError(t, err)
		doc, err = statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, 100, doc.Progress, "Progress over 100 should be adjusted to 100")
	})

	// 测试对非处理中文档更新进度
	t.Run("update progress on non-processing document", func(t *testing.T) {
		docID := "non-processing-doc"
		err := statusManager.MarkAsUploaded(ctx, docID, "test.pdf", "/path/to/test.pdf", 1024)
		require.NoError(t, err)

		err = statusManager.MarkAsCompleted(ctx, docID, 0)
		require.NoError(t, err)

		// 尝试更新已完成文档的进度
		err = statusManager.UpdateProgress(ctx, docID, 50)
		assert.Error(t, err, "Should not be able to update progress of completed document")
	})
}

// TestAsyncDocumentProcessing 测试异步文档处理功能
func TestAsyncDocumentProcessing(t *testing.T) {
	// 如果 Redis 不可用则跳过
	redisConn, err := taskqueue.NewRedisQueue(&taskqueue.Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15, // 使用 DB 15 进行测试
	})

	if err != nil {
		t.Skip("Redis 不可用，跳过异步文档处理测试:", err)
		return
	}
	defer redisConn.Close()

	// 创建临时目录和文件
	tempDir, err := os.MkdirTemp("", "docqa-async-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testContent := "这是一个用于测试异步处理的文档内容。\n\n包含多个段落。\n\n用于测试异步处理功能。"
	testFile := filepath.Join(tempDir, "async_test.txt")
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// 初始化测试环境和服务
	docService, _, statusManager := setupDocumentTestEnv(t, tempDir)

	// 首先在数据库中创建一个文档记录
	ctx := context.Background()
	fileID := "async-test-file"
	fileName := filepath.Base(testFile)
	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err)

	// 在处理之前将文档标记为已上传
	err = statusManager.MarkAsUploaded(ctx, fileID, fileName, testFile, fileInfo.Size())
	require.NoError(t, err, "创建初始文档记录失败")

	// 创建用于异步处理的 Redis 队列
	queueConfig := &taskqueue.Config{
		RedisAddr:   "localhost:6379",
		RedisDB:     15, // 使用 DB 15 进行测试
		Concurrency: 2,
		RetryLimit:  2,
		RetryDelay:  time.Second,
	}
	queue, err := taskqueue.NewRedisQueue(queueConfig)
	require.NoError(t, err, "创建 Redis 队列失败")
	defer queue.Close()

	// 测试 EnableAsyncProcessing
	t.Run("EnableAsyncProcessing", func(t *testing.T) {
		docService.EnableAsyncProcessing(queue)
		assert.True(t, docService.asyncEnabled)
		assert.NotNil(t, docService.taskQueue)
	})

	// 测试 DisableAsyncProcessing
	t.Run("DisableAsyncProcessing", func(t *testing.T) {
		docService.EnableAsyncProcessing(queue)
		docService.DisableAsyncProcessing()
		assert.False(t, docService.asyncEnabled)
		// 即使禁用了异步处理，队列仍应可用
		assert.NotNil(t, docService.taskQueue)
	})

	// 重新启用异步处理以进行后续测试
	docService.EnableAsyncProcessing(queue)

	// 测试 ProcessDocumentAsync
	t.Run("ProcessDocumentAsync", func(t *testing.T) {
		err := docService.ProcessDocumentAsync(ctx, fileID, testFile)
		require.NoError(t, err, "ProcessDocumentAsync 不应失败")

		// 检查文档状态（应为处理中）
		status, err := statusManager.GetStatus(ctx, fileID)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusProcessing, status)

		// 获取文档的任务
		tasks, err := queue.GetTasksByDocument(ctx, fileID)
		require.NoError(t, err)
		assert.NotEmpty(t, tasks, "预期至少创建一个任务")

		// 验证任务类型
		assert.Equal(t, taskqueue.TaskProcessComplete, tasks[0].Type)
	})

	// 测试带选项的 ProcessDocumentAsync
	t.Run("ProcessDocumentAsyncWithOptions", func(t *testing.T) {
		// 创建一个新的文档 ID
		fileID2 := "async-test-options"
		err = statusManager.MarkAsUploaded(ctx, fileID2, fileName, testFile, fileInfo.Size())
		require.NoError(t, err)

		// 使用选项进行处理
		err = docService.ProcessDocumentAsync(
			ctx,
			fileID2,
			testFile,
			WithChunkSize(200),
			WithChunkOverlap(50),
			WithSplitType("sentence"),
			WithMetadata(map[string]string{"test": "value"}),
		)
		require.NoError(t, err)

		// 获取任务并验证负载
		tasks, err := queue.GetTasksByDocument(ctx, fileID2)
		require.NoError(t, err)
		require.NotEmpty(t, tasks)

		// 检查任务负载
		var payload taskqueue.ProcessCompletePayload
		err = json.Unmarshal(tasks[0].Payload, &payload)
		require.NoError(t, err)

		assert.Equal(t, 200, payload.ChunkSize)
		assert.Equal(t, 50, payload.Overlap)
		assert.Equal(t, "sentence", payload.SplitType)
		assert.Equal(t, map[string]string{"test": "value"}, payload.Metadata)
	})

	// 测试 WaitForDocumentProcessing
	t.Run("WaitForDocumentProcessing", func(t *testing.T) {
		// 尝试使用短超时时间 - 应超时，因为文档仍在处理中
		err := docService.WaitForDocumentProcessing(ctx, fileID, 100*time.Millisecond)
		assert.Error(t, err, "预期超时错误")

		// 手动更新任务状态以模拟完成
		tasks, err := queue.GetTasksByDocument(ctx, fileID)
		require.NoError(t, err)
		require.NotEmpty(t, tasks)

		taskID := tasks[0].ID
		err = queue.UpdateTaskStatus(ctx, taskID, taskqueue.StatusCompleted, json.RawMessage(`{}`), "")
		require.NoError(t, err)

		// 将文档标记为已完成
		err = statusManager.MarkAsCompleted(ctx, fileID, 3)
		require.NoError(t, err)

		// 现在等待应成功
		err = docService.WaitForDocumentProcessing(ctx, fileID, 1*time.Second)
		assert.NoError(t, err, "文档完成后等待应成功")
	})

	// 测试 GetDocumentTasks
	t.Run("GetDocumentTasks", func(t *testing.T) {
		tasks, err := docService.GetDocumentTasks(ctx, fileID)
		require.NoError(t, err)
		assert.NotEmpty(t, tasks, "应返回文档的任务")
	})
}

// TestCallbackHandlers 测试异步处理的各种回调处理程序
func TestCallbackHandlers(t *testing.T) {
	// 如果 Redis 不可用则跳过
	redisConn, err := taskqueue.NewRedisQueue(&taskqueue.Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15,
	})

	if err != nil {
		t.Skip("Redis 不可用，跳过回调处理程序测试:", err)
		return
	}
	defer redisConn.Close()

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa-handlers-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 设置测试环境
	docService, _, statusManager := setupDocumentTestEnv(t, tempDir)

	// 创建 Redis 队列
	queue, err := taskqueue.NewRedisQueue(&taskqueue.Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15,
	})
	require.NoError(t, err)
	defer queue.Close()

	// 启用异步处理
	docService.EnableAsyncProcessing(queue)

	ctx := context.Background()

	// 在数据库中创建一个测试文档
	docID := "callback-test-doc"
	status := statusManager.MarkAsUploaded(ctx, docID, "test.txt", "path/to/test.txt", 1000)
	require.NoError(t, status)

	// 测试文档解析结果处理程序
	t.Run("DocumentParseResultHandler", func(t *testing.T) {
		task := &taskqueue.Task{
			ID:         "parse-task",
			Type:       taskqueue.TaskDocumentParse,
			DocumentID: docID,
			Status:     taskqueue.StatusCompleted,
		}

		result := taskqueue.DocumentParseResult{
			Content: "This is test content for parse result",
			Title:   "Test Document",
			Words:   5,
			Chars:   35,
		}

		resultJSON, _ := json.Marshal(result)

		// 首先将文档设置为处理中状态
		err = statusManager.MarkAsProcessing(ctx, docID)
		require.NoError(t, err)

		// 调用处理程序
		err = docService.handleDocumentParseResult(ctx, task, resultJSON)
		require.NoError(t, err)

		// 检查文档进度是否更新
		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Greater(t, doc.Progress, 0)
	})

	// 测试文本分块结果处理程序
	t.Run("TextChunkResultHandler", func(t *testing.T) {
		task := &taskqueue.Task{
			ID:         "chunk-task",
			Type:       taskqueue.TaskTextChunk,
			DocumentID: docID,
			Status:     taskqueue.StatusCompleted,
		}

		result := taskqueue.TextChunkResult{
			DocumentID: docID,
			Chunks: []taskqueue.ChunkInfo{
				{Text: "Chunk 1", Index: 0},
				{Text: "Chunk 2", Index: 1},
			},
			ChunkCount: 2,
		}

		resultJSON, _ := json.Marshal(result)

		// 调用处理程序
		err = docService.handleTextChunkResult(ctx, task, resultJSON)
		require.NoError(t, err)

		// 检查文档进度是否更新
		doc, err := statusManager.GetDocument(ctx, docID)
		require.NoError(t, err)
		assert.Greater(t, doc.Progress, 30)
	})

	// 测试向量化结果处理程序
	t.Run("VectorizeResultHandler", func(t *testing.T) {
		task := &taskqueue.Task{
			ID:         "vector-task",
			Type:       taskqueue.TaskVectorize,
			DocumentID: docID,
			Status:     taskqueue.StatusCompleted,
		}

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

		resultJSON, _ := json.Marshal(result)

		// 调用处理程序
		err = docService.handleVectorizeResult(ctx, task, resultJSON)
		require.NoError(t, err)

		// 检查文档是否完成
		status, err := statusManager.GetStatus(ctx, docID)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusCompleted, status)
	})

	// 测试处理完成结果处理程序
	t.Run("ProcessCompleteResultHandler", func(t *testing.T) {
		// 为此测试创建另一个文档
		docID2 := "complete-test-doc"
		err = statusManager.MarkAsUploaded(ctx, docID2, "test2.txt", "path/to/test2.txt", 1000)
		require.NoError(t, err)

		task := &taskqueue.Task{
			ID:         "complete-task",
			Type:       taskqueue.TaskProcessComplete,
			DocumentID: docID2,
			Status:     taskqueue.StatusCompleted,
		}

		result := taskqueue.ProcessCompleteResult{
			DocumentID:   docID2,
			ChunkCount:   3,
			VectorCount:  3,
			Dimension:    4,
			ParseStatus:  "success",
			ChunkStatus:  "success",
			VectorStatus: "success",
		}

		resultJSON, _ := json.Marshal(result)

		// 调用处理程序
		err = docService.handleProcessCompleteResult(ctx, task, resultJSON)
		require.NoError(t, err)

		// 检查文档是否完成
		status, err := statusManager.GetStatus(ctx, docID2)
		require.NoError(t, err)
		assert.Equal(t, models.DocStatusCompleted, status)
	})
}

// testParser 用于测试的简单解析器
type testParser struct{}

func (p *testParser) Parse(filePath string) (string, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (p *testParser) ParseReader(r io.Reader, filename string) (string, error) {
	content, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// testEmbeddingClient 用于测试的简单嵌入客户端
type testEmbeddingClient struct {
	dimension int
}

func (c *testEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	// 返回固定维度的向量
	return generateTestVector(c.dimension, text), nil
}

func (c *testEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// 为每个文本生成一个向量
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = generateTestVector(c.dimension, text)
	}
	return vectors, nil
}

func (c *testEmbeddingClient) Name() string {
	return "test-embedding"
}

// generateTestVector 生成用于测试的向量
// 使用text的第一个字符作为种子以生成具有一定相似度的向量
func generateTestVector(dim int, text string) []float32 {
	vec := make([]float32, dim)
	seed := 0.1 // 默认种子
	if len(text) > 0 {
		// 使用第一个字符作为种子
		seed = float64(text[0]) / 255.0
	}

	for i := range vec {
		vec[i] = float32(seed + float64(i)*0.1)
	}
	return vec
}
