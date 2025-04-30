package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/sirupsen/logrus"
)

// DocumentStatusManager 文档状态管理器
// 负责管理文档处理的生命周期状态
type DocumentStatusManager struct {
	repo   repository.DocumentRepository // 文档仓储接口
	logger *logrus.Logger                // 日志记录器
	mu     sync.Mutex                    // 互斥锁，保证状态转换的原子性
}

// NewDocumentStatusManager 创建文档状态管理器
func NewDocumentStatusManager(repo repository.DocumentRepository, logger *logrus.Logger) *DocumentStatusManager {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	return &DocumentStatusManager{
		repo:   repo,
		logger: logger,
	}
}

// MarkAsUploaded 将文档标记为已上传状态
func (m *DocumentStatusManager) MarkAsUploaded(ctx context.Context, docID string, fileName string, filePath string, fileSize int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.WithFields(logrus.Fields{
		"doc_id":   docID,
		"filename": fileName,
	}).Info("Marking document as uploaded")

	// 创建新的文档记录
	doc := &models.Document{
		ID:         docID,
		FileName:   fileName,
		FileType:   getFileType(fileName),
		FilePath:   filePath,
		FileSize:   fileSize,
		Status:     models.DocStatusUploaded,
		UploadedAt: time.Now(),
		UpdatedAt:  time.Now(),
		Progress:   0,
	}

	// 保存到仓储
	return m.repo.Create(doc)
}

// MarkAsProcessing 将文档标记为处理中状态
func (m *DocumentStatusManager) MarkAsProcessing(ctx context.Context, docID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// 检查状态转换的有效性
	if doc.Status != models.DocStatusUploaded {
		return fmt.Errorf("invalid state transition: document %s is in %s state, expected %s",
			docID, doc.Status, models.DocStatusUploaded)
	}

	m.logger.WithField("doc_id", docID).Info("Marking document as processing")

	// 更新状态
	return m.repo.UpdateStatus(docID, models.DocStatusProcessing, "")
}

// MarkAsCompleted 将文档标记为处理完成状态
func (m *DocumentStatusManager) MarkAsCompleted(ctx context.Context, docID string, segmentCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// 检查状态转换的有效性
	if doc.Status != models.DocStatusProcessing && doc.Status != models.DocStatusUploaded {
		return fmt.Errorf("invalid state transition: document %s is in %s state, expected %s or %s",
			docID, doc.Status, models.DocStatusProcessing, models.DocStatusUploaded)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id":        docID,
		"segment_count": segmentCount,
	}).Info("Marking document as completed")

	// 更新状态
	if err := m.repo.UpdateStatus(docID, models.DocStatusCompleted, ""); err != nil {
		return err
	}

	// 更新文档记录，添加段落数量
	doc.Status = models.DocStatusCompleted
	doc.SegmentCount = segmentCount
	doc.Progress = 100
	return m.repo.Update(doc)
}

// MarkAsFailed 将文档标记为处理失败状态
func (m *DocumentStatusManager) MarkAsFailed(ctx context.Context, docID string, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	_, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id": docID,
		"error":  errorMsg,
	}).Error("Marking document as failed")

	// 更新状态
	return m.repo.UpdateStatus(docID, models.DocStatusFailed, errorMsg)
}

// UpdateProgress 更新文档处理进度
func (m *DocumentStatusManager) UpdateProgress(ctx context.Context, docID string, progress int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// 只有处理中的文档才能更新进度
	if doc.Status != models.DocStatusProcessing {
		return fmt.Errorf("cannot update progress: document %s is not in processing state", docID)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id":   docID,
		"progress": progress,
	}).Debug("Updating document progress")

	// 更新进度
	return m.repo.UpdateProgress(docID, progress)
}

// GetStatus 获取文档当前状态
func (m *DocumentStatusManager) GetStatus(ctx context.Context, docID string) (models.DocumentStatus, error) {
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return "", fmt.Errorf("failed to get document status: %w", err)
	}
	return doc.Status, nil
}

// GetDocument 获取完整的文档对象
func (m *DocumentStatusManager) GetDocument(ctx context.Context, docID string) (*models.Document, error) {
	return m.repo.GetByID(docID)
}

// ListDocuments 获取文档列表
func (m *DocumentStatusManager) ListDocuments(ctx context.Context, offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error) {
	return m.repo.List(offset, limit, filters)
}

// DeleteDocument 删除文档状态记录
func (m *DocumentStatusManager) DeleteDocument(ctx context.Context, docID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.WithField("doc_id", docID).Info("Deleting document status record")
	return m.repo.Delete(docID)
}

// ValidateStateTransition 验证状态转换的有效性
func (m *DocumentStatusManager) ValidateStateTransition(from, to models.DocumentStatus) error {
	// 定义有效的状态转换
	validTransitions := map[models.DocumentStatus][]models.DocumentStatus{
		models.DocStatusUploaded: {
			models.DocStatusProcessing,
			models.DocStatusCompleted, // 小文件可能直接完成
			models.DocStatusFailed,    // 上传后可能立即失败
		},
		models.DocStatusProcessing: {
			models.DocStatusCompleted,
			models.DocStatusFailed,
		},
		// 终态
		models.DocStatusCompleted: {},
		models.DocStatusFailed:    {models.DocStatusProcessing}, // 允许重试
	}

	// 检查是否是有效转换
	allowed := false
	for _, validTo := range validTransitions[from] {
		if validTo == to {
			allowed = true
			break
		}
	}

	if !allowed {
		return errors.New("invalid state transition")
	}

	return nil
}

// getFileType 根据文件名获取文件类型
func getFileType(fileName string) string {
	ext := ""
	for i := len(fileName) - 1; i >= 0 && fileName[i] != '.'; i-- {
		ext = string(fileName[i]) + ext
	}
	return ext
}
