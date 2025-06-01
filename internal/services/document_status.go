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
		// 设置初始处理阶段
		CurrentStage: models.StageParsing,
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id":   docID,
		"filename": fileName,
		"tags":     doc.Tags,
	}).Debug("Creating document record with tags")

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
	doc.Status = models.DocStatusProcessing
	doc.UpdatedAt = time.Now()
	// 设置初始处理阶段（如果尚未设置）
	if doc.CurrentStage == "" {
		doc.CurrentStage = models.StageParsing
	}

	return m.repo.Update(doc)
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

	// 更新文档记录
	doc.Status = models.DocStatusCompleted
	doc.SegmentCount = segmentCount
	doc.Progress = 100
	now := time.Now()
	doc.ProcessedAt = &now
	doc.UpdatedAt = now
	doc.CurrentStage = models.StageCompleted

	return m.repo.Update(doc)
}

// MarkAsFailed 将文档标记为处理失败状态
func (m *DocumentStatusManager) MarkAsFailed(ctx context.Context, docID string, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id": docID,
		"error":  errorMsg,
	}).Error("Marking document as failed")

	// 更新文档记录
	doc.Status = models.DocStatusFailed
	doc.Error = errorMsg
	now := time.Now()
	doc.ProcessedAt = &now
	doc.UpdatedAt = now

	return m.repo.Update(doc)
}

// UpdateProgress 更新文档处理进度
func (m *DocumentStatusManager) UpdateProgress(ctx context.Context, docID string, progress int) error {
	// 确保进度在0-100范围内
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	// 获取文档状态
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// 只有处理中的文档可以更新进度
	if doc.Status != models.DocStatusProcessing {
		return fmt.Errorf("cannot update progress for document with status: %s", doc.Status)
	}

	// 更新进度
	return m.repo.UpdateProgress(docID, progress)
}

// UpdateStage 更新文档处理阶段
func (m *DocumentStatusManager) UpdateStage(ctx context.Context, docID string, stage models.ProcessStage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// 只有处理中的文档才能更新阶段
	if doc.Status != models.DocStatusProcessing {
		return fmt.Errorf("cannot update stage: document %s is not in processing state", docID)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id":     docID,
		"stage":      stage,
		"prev_stage": doc.CurrentStage,
	}).Debug("Updating document stage")

	// 更新处理阶段
	doc.CurrentStage = stage
	doc.UpdatedAt = time.Now()

	// 根据阶段设置进度指示
	switch stage {
	case models.StageParsing:
		doc.Progress = 20
	case models.StageChunking:
		doc.Progress = 50
	case models.StageVectorizing:
		doc.Progress = 75
	case models.StageCompleted:
		doc.Progress = 100
	}

	return m.repo.Update(doc)
}

// UpdateCurrentTask 更新文档关联的当前任务
func (m *DocumentStatusManager) UpdateCurrentTask(ctx context.Context, docID string, taskID string, taskStatus string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id":       docID,
		"task_id":      taskID,
		"task_status":  taskStatus,
		"prev_task_id": doc.CurrentTaskID,
	}).Debug("Updating document current task")

	// 更新任务ID和状态
	doc.CurrentTaskID = taskID
	doc.LastTaskStatus = taskStatus
	doc.UpdatedAt = time.Now()

	return m.repo.Update(doc)
}

// UpdatePythonService 更新处理文档的Python服务
func (m *DocumentStatusManager) UpdatePythonService(ctx context.Context, docID string, serviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"doc_id":       docID,
		"service_name": serviceName,
		"prev_service": doc.PythonService,
	}).Debug("Updating document python service")

	// 更新Python服务名称
	doc.PythonService = serviceName
	doc.UpdatedAt = time.Now()

	return m.repo.Update(doc)
}

// IncrementRetryCount 增加重试计数并返回新值
func (m *DocumentStatusManager) IncrementRetryCount(ctx context.Context, docID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取当前文档
	doc, err := m.repo.GetByID(docID)
	if err != nil {
		return 0, fmt.Errorf("failed to get document: %w", err)
	}

	// 增加重试计数
	doc.RetryCount++
	doc.UpdatedAt = time.Now()

	m.logger.WithFields(logrus.Fields{
		"doc_id":      docID,
		"retry_count": doc.RetryCount,
	}).Info("Incrementing document retry count")

	if err := m.repo.Update(doc); err != nil {
		return 0, err
	}

	return doc.RetryCount, nil
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

// GetRepo 获取文档仓储
func (m *DocumentStatusManager) GetRepo() repository.DocumentRepository {
	return m.repo
}
