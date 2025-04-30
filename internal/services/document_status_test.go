package services

import (
	"context"
	"os"
	"testing"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
