package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/taskqueue"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// 设置Python文档服务测试环境
func setupPythonServiceTest(t *testing.T) (*PythonDocumentService, *taskqueue.MockQueue, *services.DocumentStatusManager, string, storage.Storage) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "pythonservice-test-*")
	require.NoError(t, err)

	// 创建存储服务
	storageConfig := storage.LocalConfig{
		Path: tempDir,
	}
	storageService, err := storage.NewLocalStorage(storageConfig)
	require.NoError(t, err)

	// 创建mock队列
	mockQueue := taskqueue.NewMockQueue(t)

	// 创建仓库和状态管理器
	repo := repository.NewDocumentRepository()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	statusManager := services.NewDocumentStatusManager(repo, logger)

	// 创建Python文档服务
	pythonService := NewPythonDocumentService(
		storageService,
		mockQueue,
		statusManager,
		WithPythonLogger(logger),
	)

	return pythonService, mockQueue, statusManager, tempDir, storageService
}

// 测试Python文档服务初始化
func TestPythonDocumentService_Init(t *testing.T) {
	// 设置测试环境
	pythonService, _, _, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 测试初始化
	err := pythonService.Init()
	assert.NoError(t, err)

	// 验证字段是否正确设置
	assert.NotNil(t, pythonService.repo)
	assert.NotNil(t, pythonService.statusManager)
}

// 测试文档处理功能
func TestPythonDocumentService_ProcessDocument(t *testing.T) {
	// 设置测试环境
	pythonService, mockQueue, statusManager, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 创建测试文件
	fileContent := "This is a test document"
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte(fileContent), 0644)
	require.NoError(t, err)

	// 创建文档记录
	ctx := context.Background()
	fileID := "test-doc-id"
	fileName := filepath.Base(testFile)
	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err)

	// 标记为已上传
	err = statusManager.MarkAsUploaded(ctx, fileID, fileName, testFile, fileInfo.Size())
	require.NoError(t, err)

	// 设置mock队列期望
	mockQueue.EXPECT().Enqueue(mock.AnythingOfType("*taskqueue.Task")).Return("task-123", nil)

	// 处理文档
	err = pythonService.ProcessDocument(ctx, fileID, testFile)
	assert.NoError(t, err)

	// 验证文档状态更新为处理中
	doc, err := statusManager.GetDocument(ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, models.DocStatusProcessing, doc.Status)
}

// 测试文档删除功能
func TestPythonDocumentService_DeleteDocument(t *testing.T) {
	// 设置测试环境
	pythonService, mockQueue, statusManager, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 创建测试文件
	fileContent := "This is a test document"
	testFile := filepath.Join(tempDir, "delete_test.txt")
	err := os.WriteFile(testFile, []byte(fileContent), 0644)
	require.NoError(t, err)

	// 创建文档记录
	ctx := context.Background()
	fileID := "test-delete-doc-id"
	fileName := filepath.Base(testFile)
	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err)

	// 标记为已上传
	err = statusManager.MarkAsUploaded(ctx, fileID, fileName, testFile, fileInfo.Size())
	require.NoError(t, err)

	// 设置mock队列期望
	mockQueue.EXPECT().Enqueue(mock.AnythingOfType("*taskqueue.Task")).Return("task-delete-123", nil)

	// 删除文档
	err = pythonService.DeleteDocument(ctx, fileID)
	assert.NoError(t, err)

	// 验证文档已从数据库删除
	_, err = statusManager.GetDocument(ctx, fileID)
	assert.Error(t, err, "Document should be deleted from database")
}

// 测试获取文档信息功能
func TestPythonDocumentService_GetDocumentInfo(t *testing.T) {
	// 设置测试环境
	pythonService, _, statusManager, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 创建文档记录
	ctx := context.Background()
	fileID := "info-doc-id"
	fileName := "info_test.txt"
	filePath := filepath.Join(tempDir, fileName)

	// 标记为已上传
	err := statusManager.MarkAsUploaded(ctx, fileID, fileName, filePath, 123)
	require.NoError(t, err)

	// 获取文档信息
	info, err := pythonService.GetDocumentInfo(ctx, fileID)
	require.NoError(t, err)

	// 验证文档信息
	assert.Equal(t, fileID, info["file_id"])
	assert.Equal(t, fileName, info["filename"])
	assert.Equal(t, models.DocStatusUploaded, info["status"])
	assert.Equal(t, int64(123), info["size"])
}

// 测试列出文档功能
func TestPythonDocumentService_ListDocuments(t *testing.T) {
	// 设置测试环境
	pythonService, _, statusManager, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 创建多个文档记录
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		fileID := fmt.Sprintf("list-doc-id-%d", i)
		fileName := fmt.Sprintf("list_test_%d.txt", i)
		filePath := filepath.Join(tempDir, fileName)

		err := statusManager.MarkAsUploaded(ctx, fileID, fileName, filePath, int64(100*i))
		require.NoError(t, err)
	}

	// 列出文档
	docs, total, err := pythonService.ListDocuments(ctx, 0, 10, nil)
	require.NoError(t, err)

	// 验证文档列表
	assert.Equal(t, int64(5), total)
	assert.Len(t, docs, 5)
}

// 测试更新文档标签功能
func TestPythonDocumentService_UpdateDocumentTags(t *testing.T) {
	// 设置测试环境
	pythonService, _, statusManager, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 创建文档记录
	ctx := context.Background()
	fileID := "tags-doc-id"
	fileName := "tags_test.txt"
	filePath := filepath.Join(tempDir, fileName)

	// 标记为已上传
	err := statusManager.MarkAsUploaded(ctx, fileID, fileName, filePath, 123)
	require.NoError(t, err)

	// 更新标签
	newTags := "tag1,tag2,tag3"
	err = pythonService.UpdateDocumentTags(ctx, fileID, newTags)
	require.NoError(t, err)

	// 验证标签已更新
	doc, err := statusManager.GetDocument(ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, newTags, doc.Tags)
}

// 测试获取状态管理器功能
func TestPythonDocumentService_GetStatusManager(t *testing.T) {
	// 设置测试环境
	pythonService, _, statusManager, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 获取状态管理器
	retrievedManager := pythonService.GetStatusManager()

	// 验证是同一个实例
	assert.Same(t, statusManager, retrievedManager)
}

// 测试统计文档段落功能
func TestPythonDocumentService_CountDocumentSegments(t *testing.T) {
	// 设置测试环境
	pythonService, _, _, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	// 初始化服务以确保repo已设置
	err := pythonService.Init()
	require.NoError(t, err)

	// 创建文档记录
	ctx := context.Background()
	fileID := "count-doc-id"
	fileName := "count_test.txt"
	filePath := filepath.Join(tempDir, fileName)

	// 标记为已上传
	err = pythonService.statusManager.MarkAsUploaded(ctx, fileID, fileName, filePath, 123)
	require.NoError(t, err)

	// 向文档添加段落
	segments := []*models.DocumentSegment{
		{DocumentID: fileID, SegmentID: "seg1", Position: 1, Text: "Segment 1"},
		{DocumentID: fileID, SegmentID: "seg2", Position: 2, Text: "Segment 2"},
		{DocumentID: fileID, SegmentID: "seg3", Position: 3, Text: "Segment 3"},
	}

	err = pythonService.repo.SaveSegments(segments)
	require.NoError(t, err)

	// 统计段落
	count, err := pythonService.CountDocumentSegments(ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

// 测试各种错误情况
func TestPythonDocumentService_ErrorCases(t *testing.T) {
	// 设置测试环境
	pythonService, mockQueue, _, tempDir, _ := setupPythonServiceTest(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// 测试用例1：ProcessDocument传入空fileID
	t.Run("empty fileID in ProcessDocument", func(t *testing.T) {
		err := pythonService.ProcessDocument(ctx, "", "path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fileID cannot be empty")
	})

	// 测试用例2：ProcessDocument传入空filePath
	t.Run("empty filePath in ProcessDocument", func(t *testing.T) {
		err := pythonService.ProcessDocument(ctx, "id", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filePath cannot be empty")
	})

	// 测试用例3：ProcessDocument队列入队错误
	t.Run("queue error in ProcessDocument", func(t *testing.T) {
		mockQueue.EXPECT().Enqueue(mock.AnythingOfType("*taskqueue.Task")).Return("", fmt.Errorf("queue error"))

		err := pythonService.ProcessDocument(ctx, "id", "path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to enqueue")
	})

	// 测试用例4：GetDocumentInfo获取不存在的文档
	t.Run("non-existent document in GetDocumentInfo", func(t *testing.T) {
		_, err := pythonService.GetDocumentInfo(ctx, "non-existent")
		assert.Error(t, err)
	})
}

// 测试通过选项配置服务
func TestPythonDocumentService_Options(t *testing.T) {
	// 创建存储服务
	tempDir, err := os.MkdirTemp("", "options-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	storageService, err := storage.NewLocalStorage(storage.LocalConfig{Path: tempDir})
	require.NoError(t, err)

	// 创建mock队列和自定义日志器
	mockQueue := taskqueue.NewMockQueue(t)
	customLogger := logrus.New()
	customLogger.SetLevel(logrus.WarnLevel)

	// 使用自定义日志器创建状态管理器
	repo := repository.NewDocumentRepository()
	statusManager := services.NewDocumentStatusManager(repo, customLogger)

	// 使用自定义选项创建Python文档服务
	customTimeout := 10 * time.Second
	pythonService := NewPythonDocumentService(
		storageService,
		mockQueue,
		statusManager,
		WithPythonLogger(customLogger),
		WithPythonTimeout(customTimeout),
		WithPythonRepository(repo),
	)

	// 验证选项是否生效
	assert.Equal(t, customTimeout, pythonService.timeout)
	assert.Same(t, customLogger, pythonService.logger)
	assert.Same(t, repo, pythonService.repo)
}
