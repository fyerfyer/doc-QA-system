package repository

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	// 使用唯一的内存数据库标识符
	dbName := fmt.Sprintf("file:memdb_%d?mode=memory", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err, "Failed to open in-memory database")

	// 运行迁移以创建所需的表
	err = db.AutoMigrate(&models.Document{}, &models.DocumentSegment{})
	require.NoError(t, err, "Failed to run migrations")

	// 保存原始全局DB引用
	originalDB := database.DB

	// 替换全局DB为测试DB
	database.DB = db

	// 返回测试DB和清理函数
	cleanup := func() {
		// 恢复原始DB引用
		database.DB = originalDB
	}

	return db, cleanup
}

func TestDocumentRepository_Create(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	doc := &models.Document{
		ID:        "test-doc-1",
		FileName:  "test.txt",
		FileType:  "txt",
		FilePath:  "/path/to/test.txt",
		FileSize:  1024,
		Status:    models.DocStatusUploaded,
		Tags:      "test,document",
		Progress:  0,
		UpdatedAt: time.Now(),
	}

	// 测试创建
	err := repo.Create(doc)
	assert.NoError(t, err, "Document creation should succeed")

	// 验证文档已创建
	savedDoc, err := repo.GetByID(doc.ID)
	assert.NoError(t, err, "Should be able to retrieve created document")
	assert.Equal(t, doc.ID, savedDoc.ID, "Document ID should match")
	assert.Equal(t, doc.FileName, savedDoc.FileName, "Document filename should match")
	assert.Equal(t, doc.Status, savedDoc.Status, "Document status should match")
}

func TestDocumentRepository_Update(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	doc := &models.Document{
		ID:        "test-doc-2",
		FileName:  "test.txt",
		FileType:  "txt",
		Status:    models.DocStatusUploaded,
		UpdatedAt: time.Now(),
	}

	err := repo.Create(doc)
	require.NoError(t, err, "Document creation should succeed")

	// 更新文档
	doc.Status = models.DocStatusProcessing
	doc.Progress = 50
	doc.Tags = "updated"

	err = repo.Update(doc)
	assert.NoError(t, err, "Document update should succeed")

	// 验证更新
	updatedDoc, err := repo.GetByID(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.DocStatusProcessing, updatedDoc.Status, "Status should be updated")
	assert.Equal(t, 50, updatedDoc.Progress, "Progress should be updated")
	assert.Equal(t, "updated", updatedDoc.Tags, "Tags should be updated")
}

func TestDocumentRepository_GetByID(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 测试获取不存在的文档
	doc, err := repo.GetByID("non-existing")
	assert.Error(t, err, "Should return error for non-existing document")
	assert.Nil(t, doc, "Should return nil for non-existing document")

	// 创建测试文档
	testDoc := &models.Document{
		ID:       "test-doc-3",
		FileName: "test.txt",
		FileType: "txt",
		Status:   models.DocStatusUploaded,
	}
	err = repo.Create(testDoc)
	require.NoError(t, err)

	// 测试获取存在的文档
	doc, err = repo.GetByID("test-doc-3")
	assert.NoError(t, err, "Should retrieve existing document without error")
	assert.NotNil(t, doc, "Should return document object")
	assert.Equal(t, "test.txt", doc.FileName, "Document properties should match")
}

func TestDocumentRepository_List(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	docs := []*models.Document{
		{
			ID:         "test-doc-4",
			FileName:   "doc1.txt",
			Status:     models.DocStatusUploaded,
			Tags:       "important,report",
			UploadedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID:         "test-doc-5",
			FileName:   "doc2.txt",
			Status:     models.DocStatusProcessing,
			Tags:       "report",
			UploadedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:         "test-doc-6",
			FileName:   "doc3.txt",
			Status:     models.DocStatusCompleted,
			Tags:       "memo",
			UploadedAt: time.Now(),
		},
	}

	for _, doc := range docs {
		err := repo.Create(doc)
		require.NoError(t, err)
	}

	// 测试无过滤器列表
	resultDocs, total, err := repo.List(0, 10, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total, "Total count should be 3")
	assert.Len(t, resultDocs, 3, "Should return 3 documents")

	// 测试分页
	resultDocs, total, err = repo.List(1, 2, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total, "Total count should still be 3")
	assert.Len(t, resultDocs, 2, "Should return 2 documents with offset 1")

	// 测试状态过滤器
	filters := map[string]interface{}{
		"status": string(models.DocStatusProcessing),
	}
	resultDocs, total, err = repo.List(0, 10, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total, "Total count should be 1")
	assert.Len(t, resultDocs, 1, "Should return 1 document")
	assert.Equal(t, "test-doc-5", resultDocs[0].ID, "Should return the processing document")

	// 测试标签过滤器
	filters = map[string]interface{}{
		"tags": "report",
	}
	resultDocs, total, err = repo.List(0, 10, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total, "Total count should be 2")
	assert.Len(t, resultDocs, 2, "Should return 2 documents with report tag")
}

func TestDocumentRepository_Delete(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	doc := &models.Document{
		ID:       "test-doc-7",
		FileName: "test.txt",
		Status:   models.DocStatusUploaded,
	}

	err := repo.Create(doc)
	require.NoError(t, err)

	// 添加一些文档段落
	segment := &models.DocumentSegment{
		DocumentID: doc.ID,
		SegmentID:  "seg-1",
		Position:   1,
		Text:       "Test segment text",
	}
	err = repo.SaveSegment(segment)
	require.NoError(t, err)

	// 测试删除
	err = repo.Delete(doc.ID)
	assert.NoError(t, err, "Delete should succeed")

	// 验证文档已删除
	_, err = repo.GetByID(doc.ID)
	assert.Error(t, err, "Document should no longer exist")

	// 验证段落已删除
	segments, err := repo.GetSegments(doc.ID)
	assert.NoError(t, err, "GetSegments should not error even if document is deleted")
	assert.Empty(t, segments, "Segments should be deleted along with the document")
}

func TestDocumentRepository_UpdateStatus(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	doc := &models.Document{
		ID:       "test-doc-8",
		FileName: "test.txt",
		Status:   models.DocStatusUploaded,
	}

	err := repo.Create(doc)
	require.NoError(t, err)

	// 测试更新状态
	err = repo.UpdateStatus(doc.ID, models.DocStatusProcessing, "")
	assert.NoError(t, err, "Status update should succeed")

	// 验证状态已更新
	updatedDoc, err := repo.GetByID(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.DocStatusProcessing, updatedDoc.Status, "Status should be updated")

	// 测试带错误消息的状态更新
	err = repo.UpdateStatus(doc.ID, models.DocStatusFailed, "Processing error")
	assert.NoError(t, err)

	updatedDoc, err = repo.GetByID(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.DocStatusFailed, updatedDoc.Status, "Status should be updated to failed")
	assert.Equal(t, "Processing error", updatedDoc.Error, "Error message should be set")
	assert.NotNil(t, updatedDoc.ProcessedAt, "ProcessedAt should be set for failed status")
}

func TestDocumentRepository_UpdateProgress(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	doc := &models.Document{
		ID:       "test-doc-9",
		FileName: "test.txt",
		Status:   models.DocStatusProcessing,
		Progress: 0,
	}

	err := repo.Create(doc)
	require.NoError(t, err)

	// 测试更新进度
	err = repo.UpdateProgress(doc.ID, 50)
	assert.NoError(t, err, "Progress update should succeed")

	// 验证进度已更新
	updatedDoc, err := repo.GetByID(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, 50, updatedDoc.Progress, "Progress should be updated to 50")

	// 测试负进度值被调整为0
	err = repo.UpdateProgress(doc.ID, -20)
	assert.NoError(t, err)

	updatedDoc, err = repo.GetByID(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, updatedDoc.Progress, "Negative progress should be adjusted to 0")

	// 测试超过100的进度值被调整为100
	err = repo.UpdateProgress(doc.ID, 120)
	assert.NoError(t, err)

	updatedDoc, err = repo.GetByID(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, 100, updatedDoc.Progress, "Progress over 100 should be adjusted to 100")
}

func TestDocumentRepository_SegmentOperations(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDocumentRepository()

	// 创建测试文档
	doc := &models.Document{
		ID:       "test-doc-10",
		FileName: "test.txt",
		Status:   models.DocStatusProcessing,
	}

	err := repo.Create(doc)
	require.NoError(t, err)

	// 测试保存段落
	segment1 := &models.DocumentSegment{
		DocumentID: doc.ID,
		SegmentID:  "seg-1",
		Position:   1,
		Text:       "First test segment",
	}

	segment2 := &models.DocumentSegment{
		DocumentID: doc.ID,
		SegmentID:  "seg-2",
		Position:   2,
		Text:       "Second test segment",
	}

	// 单个保存
	err = repo.SaveSegment(segment1)
	assert.NoError(t, err, "SaveSegment should succeed")

	// 批量保存
	err = repo.SaveSegments([]*models.DocumentSegment{segment2})
	assert.NoError(t, err, "SaveSegments should succeed")

	// 测试获取段落
	segments, err := repo.GetSegments(doc.ID)
	assert.NoError(t, err)
	assert.Len(t, segments, 2, "Should return 2 segments")
	assert.Equal(t, "First test segment", segments[0].Text, "Segment content should match")
	assert.Equal(t, "Second test segment", segments[1].Text, "Segment content should match")

	// 测试统计段落数量
	count, err := repo.CountSegments(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, 2, count, "Should count 2 segments")

	// 测试删除段落
	err = repo.DeleteSegments(doc.ID)
	assert.NoError(t, err, "DeleteSegments should succeed")

	// 验证段落已删除
	segments, err = repo.GetSegments(doc.ID)
	assert.NoError(t, err)
	assert.Empty(t, segments, "Segments should be deleted")

	count, err = repo.CountSegments(doc.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "Segment count should be 0 after deletion")
}

func TestMain(m *testing.M) {
	// 确保测试目录存在
	os.MkdirAll("../../data", 0755)

	// 运行测试
	exitCode := m.Run()

	// 退出
	os.Exit(exitCode)
}
