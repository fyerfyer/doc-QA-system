package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/taskqueue"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/sirupsen/logrus"
)

// PythonDocumentService 是对文档处理服务的包装
// 将文档处理任务委托给Python微服务，通过任务队列通信
type PythonDocumentService struct {
	storage       storage.Storage                 // 文件存储服务
	queue         taskqueue.Queue                 // 任务队列
	statusManager *services.DocumentStatusManager // 文档状态管理器
	repo          repository.DocumentRepository   // 文档仓储
	logger        *logrus.Logger                  // 日志记录器
	timeout       time.Duration                   // 处理超时时间
}

// PythonDocumentOption 配置选项函数类型
type PythonDocumentOption func(*PythonDocumentService)

// WithPythonLogger 设置日志记录器
func WithPythonLogger(logger *logrus.Logger) PythonDocumentOption {
	return func(s *PythonDocumentService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithPythonStatusManager 设置文档状态管理器
func WithPythonStatusManager(manager *services.DocumentStatusManager) PythonDocumentOption {
	return func(s *PythonDocumentService) {
		s.statusManager = manager
	}
}

// WithPythonTimeout 设置处理超时时间
func WithPythonTimeout(timeout time.Duration) PythonDocumentOption {
	return func(s *PythonDocumentService) {
		s.timeout = timeout
	}
}

// WithPythonRepository 设置文档仓储
func WithPythonRepository(repo repository.DocumentRepository) PythonDocumentOption {
	return func(s *PythonDocumentService) {
		s.repo = repo
	}
}

// NewPythonDocumentService 创建Python文档服务适配器
func NewPythonDocumentService(
	storage storage.Storage,
	queue taskqueue.Queue,
	statusManager *services.DocumentStatusManager,
	opts ...PythonDocumentOption,
) *PythonDocumentService {
	// 创建服务实例
	srv := &PythonDocumentService{
		storage:       storage,
		queue:         queue,
		statusManager: statusManager,
		logger:        logrus.New(),    // 默认日志记录器
		timeout:       5 * time.Minute, // 默认超时时间
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(srv)
	}

	return srv
}

// Init 初始化文档服务
func (s *PythonDocumentService) Init() error {
	// 如果没有设置仓储，创建默认仓储
	if s.repo == nil {
		s.repo = repository.NewDocumentRepository()
	}

	// 如果没有设置状态管理器，创建默认状态管理器
	if s.statusManager == nil {
		s.statusManager = services.NewDocumentStatusManager(s.repo, s.logger)
	}

	return nil
}

// ProcessDocument 将文档处理任务加入队列
// 实现与原DocumentService相同的接口，但将实际处理委托给Python微服务
func (s *PythonDocumentService) ProcessDocument(ctx context.Context, fileID string, filePath string) error {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"file_id":   fileID,
		"file_path": filePath,
	}).Info("Enqueueing document processing task")

	// 检查输入参数
	if fileID == "" {
		return fmt.Errorf("fileID cannot be empty")
	}
	if filePath == "" {
		return fmt.Errorf("filePath cannot be empty")
	}

	// 创建上下文超时
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// 更新文档状态为处理中
	if err := s.statusManager.MarkAsProcessing(ctx, fileID); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as processing")
		// 继续处理，不中断
	}

	// 创建任务
	task := taskqueue.NewTask(
		taskqueue.TaskTypeDocumentProcess,
		fileID,
		filePath,
		map[string]interface{}{
			"enqueued_at": time.Now().Format(time.RFC3339),
		},
	)

	// 将任务加入队列
	_, err := s.queue.Enqueue(task)
	if err != nil {
		// 如果入队失败，标记文档处理失败
		errMsg := fmt.Sprintf("failed to enqueue document processing task: %v", err)
		if markErr := s.statusManager.MarkAsFailed(ctx, fileID, errMsg); markErr != nil {
			s.logger.WithError(markErr).Error("Failed to mark document as failed")
		}
		return fmt.Errorf("failed to enqueue document processing task: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"file_id":   fileID,
		"task_id":   task.ID,
		"task_type": task.Type,
	}).Info("Document processing task enqueued successfully")

	return nil
}

// DeleteDocument 删除文档及其相关数据
func (s *PythonDocumentService) DeleteDocument(ctx context.Context, fileID string) error {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return err
	}

	s.logger.WithField("file_id", fileID).Info("Deleting document")

	// 1. 从存储中删除文件
	if err := s.storage.Delete(fileID); err != nil {
		// 文件可能已被删除，记录错误但不中断流程
		s.logger.WithError(err).Warn("Failed to delete file from storage")
	}

	// 2. 创建删除任务，让Python微服务也删除相关的向量数据
	task := taskqueue.NewTask(
		"document.delete",
		fileID,
		"",
		nil,
	)

	// 将任务加入队列
	_, err := s.queue.Enqueue(task)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to enqueue document deletion task")
		// 继续处理，不中断
	}

	// 3. 删除文档状态记录
	if err := s.statusManager.DeleteDocument(ctx, fileID); err != nil {
		s.logger.WithError(err).Error("Failed to delete document status")
		return fmt.Errorf("failed to delete document status: %w", err)
	}

	s.logger.WithField("file_id", fileID).Info("Document deleted successfully")
	return nil
}

// GetDocumentInfo 获取文档信息
func (s *PythonDocumentService) GetDocumentInfo(ctx context.Context, fileID string) (map[string]interface{}, error) {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return nil, err
	}

	// 获取文档状态
	doc, err := s.statusManager.GetDocument(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	// 构建文档信息
	info := map[string]interface{}{
		"file_id":    doc.ID,
		"filename":   doc.FileName,
		"status":     doc.Status,
		"created_at": doc.UploadedAt.Format(time.RFC3339),
		"updated_at": doc.UpdatedAt.Format(time.RFC3339),
		"size":       doc.FileSize,
		"progress":   doc.Progress,
	}

	// 如果有错误信息，添加到返回结果
	if doc.Error != "" {
		info["error"] = doc.Error
	}

	// 如果有处理完成时间，添加到返回结果
	if doc.ProcessedAt != nil {
		info["processed_at"] = doc.ProcessedAt.Format(time.RFC3339)
	}

	// 如果有标签，添加到返回结果
	if doc.Tags != "" {
		info["tags"] = doc.Tags
	}

	return info, nil
}

// CountDocumentSegments 统计文档段落数量
func (s *PythonDocumentService) CountDocumentSegments(ctx context.Context, fileID string) (int, error) {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return 0, err
	}

	// 使用仓储统计段落数量
	return s.repo.CountSegments(fileID)
}

// ListDocuments 获取文档列表
func (s *PythonDocumentService) ListDocuments(ctx context.Context, offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error) {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return nil, 0, err
	}

	// 使用状态管理器获取文档列表
	return s.statusManager.ListDocuments(ctx, offset, limit, filters)
}

// UpdateDocumentTags 更新文档标签
func (s *PythonDocumentService) UpdateDocumentTags(ctx context.Context, fileID string, tags string) error {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return err
	}

	// 获取文档
	doc, err := s.statusManager.GetDocument(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// 更新标签
	doc.Tags = tags

	// 保存更新
	return s.repo.Update(doc)
}

// GetStatusManager 返回文档状态管理器实例
func (s *PythonDocumentService) GetStatusManager() *services.DocumentStatusManager {
	return s.statusManager
}
