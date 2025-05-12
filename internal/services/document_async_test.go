package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DocAsyncTestSuite 定义异步文档处理的测试套件
type DocAsyncTestSuite struct {
	suite.Suite
	tempDir         string
	documentService *DocumentService
	storageService  storage.Storage
	vectorDB        vectordb.Repository
	taskQueue       taskqueue.Queue
	statusManager   *DocumentStatusManager
	repo            repository.DocumentRepository
	logger          *logrus.Logger
}

// TestDocAsyncSuite 运行测试套件
func TestDocAsyncSuite(t *testing.T) {
	// 如果 Redis 不可用则跳过
	conn, err := taskqueue.NewRedisQueue(&taskqueue.Config{
		RedisAddr: "localhost:6379",
		RedisDB:   15, // 使用 DB 15 进行测试
	})

	if err != nil {
		t.Skip("Redis 不可用，跳过异步文档测试:", err)
		return
	}
	conn.Close()

	suite.Run(t, new(DocAsyncTestSuite))
}

// SetupSuite 准备测试套件环境
func (s *DocAsyncTestSuite) SetupSuite() {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa-async-test-*")
	require.NoError(s.T(), err)
	s.tempDir = tempDir

	// 设置日志记录器
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	s.logger = logger

	// 创建存储服务
	storageConfig := storage.LocalConfig{
		Path: tempDir,
	}
	storageService, err := storage.NewLocalStorage(storageConfig)
	require.NoError(s.T(), err)
	s.storageService = storageService

	// 创建向量数据库
	vectorDBConfig := vectordb.Config{
		Type:              "memory",
		Dimension:         4,
		DistanceType:      vectordb.Cosine,
		CreateIfNotExists: true,
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(s.T(), err)
	s.vectorDB = vectorDB

	// 创建嵌入客户端模拟
	mockEmbedding := new(embedding.MockClient)
	mockEmbedding.On("Name").Return("mock-embedding")
	mockEmbedding.On("Embed", context.Background(), "This is a test document for async processing.\n\nIt contains multiple paragraphs.\n\nEach paragraph should be processed separately.").Return(
		[]float32{0.1, 0.2, 0.3, 0.4}, nil,
	)
	mockEmbedding.On("EmbedBatch", context.Background(), []string{
		"This is a test document for async processing.",
		"It contains multiple paragraphs.",
		"Each paragraph should be processed separately.",
	}).Return(
		[][]float32{{0.1, 0.2, 0.3, 0.4}, {0.5, 0.6, 0.7, 0.8}, {0.9, 0.8, 0.7, 0.6}}, nil,
	)

	// 创建文档存储库
	s.repo = repository.NewDocumentRepository()
	s.statusManager = NewDocumentStatusManager(s.repo, logger)

	// 设置基于 Redis 的任务队列
	queueConfig := &taskqueue.Config{
		RedisAddr:   "localhost:6379",
		RedisDB:     14, // 使用 DB 14 进行测试
		RetryLimit:  2,
		RetryDelay:  time.Second,
		Concurrency: 2,
	}
	taskQueue, err := taskqueue.NewRedisQueue(queueConfig)
	require.NoError(s.T(), err)
	s.taskQueue = taskQueue

	// 创建文本分割器
	splitterConfig := document.DefaultSplitterConfig()
	splitterConfig.ChunkSize = 100 // 测试用的小块
	textSplitter := document.NewTextSplitter(splitterConfig)

	// 创建文档服务
	s.documentService = NewDocumentService(
		storageService,
		&testParser{},
		textSplitter,
		mockEmbedding,
		vectorDB,
		WithBatchSize(2),
		WithTimeout(5*time.Second),
		WithDocumentRepository(s.repo),
		WithStatusManager(s.statusManager),
		WithLogger(logger),
	)
}

// TearDownSuite 清理所有测试后的环境
func (s *DocAsyncTestSuite) TearDownSuite() {
	// 关闭任务队列
	if s.taskQueue != nil {
		s.taskQueue.Close()
	}

	// 删除临时目录
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
}

// setupTestDocument 创建测试文档记录和文件
func (s *DocAsyncTestSuite) setupTestDocument() (string, string, error) {
	// 创建测试内容和文件
	testContent := "This is a test document for async processing.\n\nIt contains multiple paragraphs.\n\nEach paragraph should be processed separately."
	fileName := "test_async_doc.txt"
	filePath := filepath.Join(s.tempDir, fileName)
	err := os.WriteFile(filePath, []byte(testContent), 0644)
	if err != nil {
		return "", "", err
	}

	// 生成文档 ID
	docID := "test-async-doc-" + time.Now().Format("150405")

	// 创建文档记录
	ctx := context.Background()
	err = s.statusManager.MarkAsUploaded(ctx, docID, fileName, filePath, int64(len(testContent)))
	if err != nil {
		return "", "", err
	}

	return docID, filePath, nil
}

// TestEnableAsyncProcessing 测试启用异步处理
func (s *DocAsyncTestSuite) TestEnableAsyncProcessing() {
	// 初始状态应为未启用异步处理
	assert.False(s.T(), s.documentService.asyncEnabled)
	assert.Nil(s.T(), s.documentService.taskQueue)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

	// 检查是否启用了异步处理
	assert.True(s.T(), s.documentService.asyncEnabled)
	assert.NotNil(s.T(), s.documentService.taskQueue)
}

// TestDisableAsyncProcessing 测试禁用异步处理
func (s *DocAsyncTestSuite) TestDisableAsyncProcessing() {
	// 首先启用
	s.documentService.EnableAsyncProcessing(s.taskQueue)

	// 然后禁用
	s.documentService.DisableAsyncProcessing()

	// 检查是否禁用了异步处理
	assert.False(s.T(), s.documentService.asyncEnabled)
	// 任务队列引用应保留
	assert.NotNil(s.T(), s.documentService.taskQueue)
}

// TestProcessDocumentAsync 测试异步文档处理流程
func (s *DocAsyncTestSuite) TestProcessDocumentAsync() {
	// 创建测试文档
	docID, filePath, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

	// 异步处理文档
	ctx := context.Background()
	err = s.documentService.ProcessDocumentAsync(ctx, docID, filePath)
	require.NoError(s.T(), err)

	// 检查文档状态是否更改为处理中
	status, err := s.statusManager.GetStatus(ctx, docID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), models.DocStatusProcessing, status)

	// 获取文档的任务
	tasks, err := s.taskQueue.GetTasksByDocument(ctx, docID)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), tasks, "预期至少创建一个任务")

	// 检查任务类型是否正确
	assert.Equal(s.T(), taskqueue.TaskProcessComplete, tasks[0].Type)
}

// TestProcessCompleteCallback 测试处理完成回调
func (s *DocAsyncTestSuite) TestProcessCompleteCallback() {
	// 创建测试文档
	docID, _, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

	// 创建模拟任务
	task := &taskqueue.Task{
		ID:         "test-task-id",
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
	require.NoError(s.T(), err)

	// 创建上下文
	ctx := context.Background()

	// 调用处理函数
	err = s.documentService.handleProcessCompleteResult(ctx, task, resultJSON)
	require.NoError(s.T(), err)

	// 验证文档状态是否更新为已完成
	status, err := s.statusManager.GetStatus(ctx, docID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), models.DocStatusCompleted, status)
}

// TestParseCallback 测试处理文档解析回调
func (s *DocAsyncTestSuite) TestParseCallback() {
	// 创建测试文档
	docID, _, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

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
	require.NoError(s.T(), err)

	// 创建上下文
	ctx := context.Background()

	// 调用处理函数
	err = s.documentService.handleDocumentParseResult(ctx, task, resultJSON)
	require.NoError(s.T(), err)

	// 验证进度是否更新
	doc, err := s.statusManager.GetDocument(ctx, docID)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), doc.Progress, 0)
}

// TestChunkCallback 测试处理文本分块回调
func (s *DocAsyncTestSuite) TestChunkCallback() {
	// 创建测试文档
	docID, _, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

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
	require.NoError(s.T(), err)

	// 创建上下文
	ctx := context.Background()

	// 调用处理函数
	err = s.documentService.handleTextChunkResult(ctx, task, resultJSON)
	require.NoError(s.T(), err)

	// 验证进度是否更新
	doc, err := s.statusManager.GetDocument(ctx, docID)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), doc.Progress, 30)
}

// TestVectorizeCallback 测试处理向量化回调
func (s *DocAsyncTestSuite) TestVectorizeCallback() {
	// 创建测试文档
	docID, _, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

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
	require.NoError(s.T(), err)

	// 创建上下文
	ctx := context.Background()

	// 调用处理函数
	err = s.documentService.handleVectorizeResult(ctx, task, resultJSON)
	require.NoError(s.T(), err)

	// 验证文档是否标记为已完成
	status, err := s.statusManager.GetStatus(ctx, docID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), models.DocStatusCompleted, status)
}

// TestWaitForDocumentProcessing 测试文档处理等待机制
func (s *DocAsyncTestSuite) TestWaitForDocumentProcessing() {
	// 创建测试文档
	docID, filePath, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

	// 异步处理文档
	ctx := context.Background()
	err = s.documentService.ProcessDocumentAsync(ctx, docID, filePath)
	require.NoError(s.T(), err)

	// 模拟文档被处理并完成
	// 首先获取任务
	tasks, err := s.taskQueue.GetTasksByDocument(ctx, docID)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), tasks)

	// 更新任务为已完成状态
	taskID := tasks[0].ID
	result := taskqueue.ProcessCompleteResult{
		DocumentID:  docID,
		ChunkCount:  2,
		VectorCount: 2,
	}
	resultJSON, _ := json.Marshal(result)

	// 更新任务状态
	err = s.taskQueue.UpdateTaskStatus(ctx, taskID, taskqueue.StatusCompleted, resultJSON, "")
	require.NoError(s.T(), err)

	// 通知任务更新
	err = s.taskQueue.NotifyTaskUpdate(ctx, taskID)
	require.NoError(s.T(), err)

	// 标记文档为已完成
	err = s.statusManager.MarkAsCompleted(ctx, docID, 2)
	require.NoError(s.T(), err)

	// 现在使用短超时时间等待
	err = s.documentService.WaitForDocumentProcessing(ctx, docID, 2*time.Second)
	assert.NoError(s.T(), err, "当文档已完成时等待不应超时")
}

// TestProcessDocumentAsyncOptions 测试异步处理的不同选项
func (s *DocAsyncTestSuite) TestProcessDocumentAsyncOptions() {
	// 创建测试文档
	docID, filePath, err := s.setupTestDocument()
	require.NoError(s.T(), err)

	// 启用异步处理
	s.documentService.EnableAsyncProcessing(s.taskQueue)

	// 定义选项
	ctx := context.Background()
	err = s.documentService.ProcessDocumentAsync(
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
	require.NoError(s.T(), err)

	// 获取任务并验证负载包含正确的选项
	tasks, err := s.taskQueue.GetTasksByDocument(ctx, docID)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), tasks)

	// 使用类型断言将负载 JSON 转换为预期结构
	var payload taskqueue.ProcessCompletePayload
	err = json.Unmarshal(tasks[0].Payload, &payload)
	require.NoError(s.T(), err)

	// 检查选项是否正确传递到负载
	assert.Equal(s.T(), 200, payload.ChunkSize)
	assert.Equal(s.T(), 50, payload.Overlap)
	assert.Equal(s.T(), "sentence", payload.SplitType)
	assert.Equal(s.T(), "test-model", payload.Model)
	assert.Equal(s.T(), map[string]string{"source": "test"}, payload.Metadata)
}
