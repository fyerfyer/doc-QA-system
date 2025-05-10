package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/adapters"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestEnvironment 设置测试环境
func setupTestEnvironment(t *testing.T) (*config.Config, *logrus.Logger) {
	// 创建测试配置
	cfg, err := config.Load("../config.test.yml")
	require.NoError(t, err, "Failed to load test config")

	// 从环境变量获取API密钥（优先级高于配置文件）
	if apiKey := os.Getenv("TONGYI_API_KEY"); apiKey != "" {
		cfg.Embed.APIKey = apiKey
		cfg.LLM.APIKey = apiKey
	}

	// 确保API密钥可用
	require.NotEmpty(t, cfg.Embed.APIKey, "Embedding API key is required for testing")

	// 创建测试日志记录器
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// 创建测试目录
	err = os.MkdirAll(cfg.Storage.Path, 0755)
	require.NoError(t, err, "Failed to create test storage directory")

	err = os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755)
	require.NoError(t, err, "Failed to create test database directory")

	return cfg, logger
}

// cleanupTestEnvironment 清理测试环境
func cleanupTestEnvironment(t *testing.T, cfg *config.Config) {
	// 清理测试文件
	// 注意：在实际测试环境中，你可能希望保留这些文件以便调试
	// os.RemoveAll(cfg.Storage.Path)
	t.Log("Test environment cleaned up")
}

// TestDocumentProcessing 测试文档处理流程
func TestDocumentProcessing(t *testing.T) {
	// 设置测试环境
	cfg, logger := setupTestEnvironment(t)
	defer cleanupTestEnvironment(t, cfg)

	// 创建文件存储
	fileStorage, err := storage.NewLocalStorage(storage.LocalConfig{
		Path: cfg.Storage.Path,
	})
	require.NoError(t, err, "Failed to create file storage")

	// 创建文档处理服务
	docService, err := adapters.CreateDocumentService(cfg, fileStorage, logger)
	require.NoError(t, err, "Failed to create document service")
	require.NotNil(t, docService, "Document service should not be nil")

	// 初始化服务
	err = docService.Init()
	require.NoError(t, err, "Failed to initialize document service")

	// 创建临时文件
	testContent := `文档问答系统简介

文档问答系统是一种能够理解和回答用户关于特定文档内容问题的智能系统。这类系统结合了自然语言处理、信息检索和机器学习技术，能够从大量文档中找出相关内容并生成准确的回答。

工作原理

文档问答系统的核心工作流程分为几个关键步骤：
1. 文档处理：系统首先接收并解析用户上传的文档，支持PDF、Markdown和纯文本等多种格式。
2. 文本分段：将文档内容划分为更小的语义单元，如段落或句子。
3. 向量化：使用嵌入模型将每个文本片段转换为向量形式，以便于后续的语义搜索。
4. 索引构建：将生成的向量存入向量数据库中，建立高效的检索索引。
5. 查询处理：当用户提出问题时，系统将问题同样向量化，然后在向量数据库中查找相似的文本片段。
6. 答案生成：基于检索到的相关文本，使用大语言模型生成连贯、准确的答案。`

	// 创建临时文件目录
	tmpDir, err := os.MkdirTemp("", "docqa-test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tmpDir) // 清理临时目录

	// 创建测试文件
	testFilePath := filepath.Join(tmpDir, "sample.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// 打开测试文件
	file, err := os.Open(testFilePath)
	require.NoError(t, err, "Failed to open test file")
	defer file.Close()

	// 上传文件到存储
	fileInfo, err := fileStorage.Save(file, "sample.txt")
	require.NoError(t, err, "Failed to save file to storage")
	require.NotEmpty(t, fileInfo.ID, "File ID should not be empty")

	// 记录文件信息
	t.Logf("File uploaded: ID=%s, Name=%s, Path=%s, Size=%d",
		fileInfo.ID, fileInfo.Name, fileInfo.Path, fileInfo.Size)

	// 获取状态管理器
	statusManager := docService.GetStatusManager()
	require.NotNil(t, statusManager, "Status manager should not be nil")

	// 将文档标记为已上传
	ctx := context.Background()
	err = statusManager.MarkAsUploaded(ctx, fileInfo.ID, fileInfo.Name, fileInfo.Path, fileInfo.Size)
	require.NoError(t, err, "Failed to mark document as uploaded")

	// 启动文档处理
	err = docService.ProcessDocument(ctx, fileInfo.ID, fileInfo.Path)
	require.NoError(t, err, "Failed to process document")

	// 等待处理完成（或失败）
	t.Log("Waiting for document processing to complete...")

	// 最大等待时间（3分钟）
	timeout := time.Now().Add(3 * time.Minute)
	var doc *models.Document
	var docStatus models.DocumentStatus
	var completed bool

	for time.Now().Before(timeout) {
		// 获取文档状态
		doc, err = statusManager.GetDocument(ctx, fileInfo.ID)
		require.NoError(t, err, "Failed to get document status")

		docStatus = doc.Status
		t.Logf("Current document status: %s, progress: %d%%", docStatus, doc.Progress)

		// 检查是否完成或失败
		if docStatus == models.DocStatusCompleted || docStatus == models.DocStatusFailed {
			completed = true
			break
		}

		// 等待一段时间再次检查
		time.Sleep(10 * time.Second)
	}

	// 验证处理是否完成
	assert.True(t, completed, "Document processing did not complete within the timeout period")

	if docStatus == models.DocStatusFailed {
		t.Logf("Document processing failed with error: %s", doc.Error)
		t.Fail()
	} else {
		// 验证处理结果
		assert.Equal(t, models.DocStatusCompleted, docStatus, "Document should be in completed status")

		// 验证是否有段落被处理
		segmentCount, err := docService.CountDocumentSegments(ctx, fileInfo.ID)
		assert.NoError(t, err, "Failed to count document segments")
		assert.Greater(t, segmentCount, 0, "Document should have at least one segment")

		t.Logf("Document processed successfully with %d segments", segmentCount)
	}

	// 测试获取文档信息
	docInfo, err := docService.GetDocumentInfo(ctx, fileInfo.ID)
	assert.NoError(t, err, "Failed to get document info")
	assert.Equal(t, fileInfo.ID, docInfo["file_id"], "Document ID should match")
	assert.Equal(t, "sample.txt", docInfo["filename"], "Filename should match")

	// 测试文档删除
	err = docService.DeleteDocument(ctx, fileInfo.ID)
	assert.NoError(t, err, "Failed to delete document")

	// 验证文档已被删除
	_, err = statusManager.GetDocument(ctx, fileInfo.ID)
	assert.Error(t, err, "Document should be deleted")
}

// TestDocumentProcessingWithListDocuments 测试文档列表功能
func TestDocumentProcessingWithListDocuments(t *testing.T) {
	// 设置测试环境
	cfg, logger := setupTestEnvironment(t)
	defer cleanupTestEnvironment(t, cfg)

	// 创建文件存储
	fileStorage, err := storage.NewLocalStorage(storage.LocalConfig{
		Path: cfg.Storage.Path,
	})
	require.NoError(t, err, "Failed to create file storage")

	// 创建文档仓储
	docRepo := repository.NewDocumentRepository()

	// 创建几个测试文档记录
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		doc := &models.Document{
			ID:       fmt.Sprintf("test-doc-%d", i),
			FileName: fmt.Sprintf("test-doc-%d.txt", i),
			FileType: "txt",
			FilePath: fmt.Sprintf("/test/path/test-doc-%d.txt", i),
			FileSize: int64(100 * i),
			Status:   models.DocStatusCompleted,
			Progress: 100,
		}
		err = docRepo.Create(doc)
		require.NoError(t, err, "Failed to create test document")
	}

	// 创建文档处理服务
	docService, err := adapters.CreateDocumentService(cfg, fileStorage, logger)
	require.NoError(t, err, "Failed to create document service")

	// 测试列出文档
	docs, total, err := docService.ListDocuments(ctx, 0, 10, nil)
	assert.NoError(t, err, "Failed to list documents")
	assert.GreaterOrEqual(t, total, int64(3), "Should have at least 3 documents")
	assert.GreaterOrEqual(t, len(docs), 3, "Should return at least 3 documents")

	// 测试筛选已完成的文档
	completedFilter := map[string]interface{}{
		"status": models.DocStatusCompleted,
	}
	docs, total, err = docService.ListDocuments(ctx, 0, 10, completedFilter)
	assert.NoError(t, err, "Failed to list completed documents")
	assert.GreaterOrEqual(t, total, int64(3), "Should have at least 3 completed documents")

	// 清理测试数据
	for i := 1; i <= 3; i++ {
		docID := fmt.Sprintf("test-doc-%d", i)
		err = docRepo.Delete(docID)
		assert.NoError(t, err, "Failed to delete test document")
	}
}
