package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/taskqueue"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/sirupsen/logrus"
)

// DocumentService 文档服务
// 负责协调文档解析、分段、嵌入和存储
type DocumentService struct {
	storage       storage.Storage               // 文件存储服务
	parser        document.Parser               // 文档解析器
	splitter      document.Splitter             // 文本分段器
	embedder      embedding.Client              // 嵌入模型客户端
	vectorDB      vectordb.Repository           // 向量数据库
	repo          repository.DocumentRepository // 文档元数据存储
	statusManager *DocumentStatusManager        // 文档状态管理器
	batchSize     int                           // 批处理大小
	timeout       time.Duration                 // 处理超时时间
	logger        *logrus.Logger                // 日志记录器
	queue         taskqueue.Queue               // 任务队列
	usePython     bool                          // 是否使用Python处理
}

// DocumentOption 文档服务配置选项
type DocumentOption func(*DocumentService)

// NewDocumentService 创建一个新的文档服务
func NewDocumentService(
	storage storage.Storage,
	parser document.Parser,
	splitter document.Splitter,
	embedder embedding.Client,
	vectorDB vectordb.Repository,
	opts ...DocumentOption,
) *DocumentService {
	// 创建服务实例
	srv := &DocumentService{
		storage:   storage,
		parser:    parser,
		splitter:  splitter,
		embedder:  embedder,
		vectorDB:  vectorDB,
		batchSize: 16,              // 默认批处理大小
		timeout:   time.Minute * 5, // 默认超时时间
		logger:    logrus.New(),    // 默认日志记录器
		usePython: false,           // 默认不使用Python处理
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(srv)
	}

	return srv
}

// WithBatchSize 设置批处理大小
func WithBatchSize(size int) DocumentOption {
	return func(s *DocumentService) {
		if size > 0 {
			s.batchSize = size
		}
	}
}

// WithTimeout 设置处理超时时间
func WithTimeout(timeout time.Duration) DocumentOption {
	return func(s *DocumentService) {
		s.timeout = timeout
	}
}

// WithLogger 设置日志记录器
func WithLogger(logger *logrus.Logger) DocumentOption {
	return func(s *DocumentService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithDocumentRepository 设置文档仓储
func WithDocumentRepository(repo repository.DocumentRepository) DocumentOption {
	return func(s *DocumentService) {
		s.repo = repo
	}
}

// WithStatusManager 设置状态管理器
func WithStatusManager(manager *DocumentStatusManager) DocumentOption {
	return func(s *DocumentService) {
		s.statusManager = manager
	}
}

// WithTaskQueue 设置任务队列
func WithTaskQueue(queue taskqueue.Queue) DocumentOption {
	return func(s *DocumentService) {
		s.queue = queue
	}
}

// WithPythonProcessing 启用Python微服务处理
func WithPythonProcessing(enabled bool) DocumentOption {
	return func(s *DocumentService) {
		s.usePython = enabled
	}
}

// Init 初始化文档服务
// 确保必要的依赖都已设置
func (s *DocumentService) Init() error {
	// 如果没有设置仓储，创建默认仓储
	if s.repo == nil {
		s.repo = repository.NewDocumentRepository()
	}

	// 如果没有设置状态管理器，创建默认状态管理器
	if s.statusManager == nil {
		s.statusManager = NewDocumentStatusManager(s.repo, s.logger)
	}

	return nil
}

// ProcessDocument 处理文档(解析、分段、向量化、入库)
func (s *DocumentService) ProcessDocument(ctx context.Context, fileID string, filePath string) error {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"file_id":    fileID,
		"file_path":  filePath,
		"use_python": s.usePython,
	}).Info("Starting document processing")

	// 检查输入参数
	if fileID == "" {
		return errors.New("fileID cannot be empty")
	}
	if filePath == "" {
		return errors.New("filePath cannot be empty")
	}

	// 如果使用Python微服务处理，则将任务加入队列
	if s.usePython && s.queue != nil {
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
		taskID, err := s.queue.Enqueue(task)
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
			"task_id":   taskID,
			"task_type": task.Type,
		}).Info("Document processing task enqueued successfully")

		return nil
	}

	// 以下是本地处理文档的流程
	// 设置上下文超时
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// 更新文档状态为处理中
	if err := s.statusManager.MarkAsProcessing(ctx, fileID); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as processing")
		// 继续处理，不中断
	}

	// 解析文档内容
	content, err := s.parseDocument(filePath)
	if err != nil {
		s.failDocument(ctx, fileID, fmt.Sprintf("failed to parse document: %v", err))
		return fmt.Errorf("failed to parse document: %w", err)
	}

	// 文本分段
	segments, err := s.splitContent(content)
	if err != nil {
		s.failDocument(ctx, fileID, fmt.Sprintf("failed to split content: %v", err))
		return fmt.Errorf("failed to split content: %w", err)
	}

	// 更新进度到20%
	if err := s.statusManager.UpdateProgress(ctx, fileID, 20); err != nil {
		s.logger.WithError(err).Warn("Failed to update document progress")
	}

	// 批量处理文本段落
	err = s.processBatches(ctx, fileID, filePath, segments)
	if err != nil {
		s.failDocument(ctx, fileID, fmt.Sprintf("failed to process batches: %v", err))
		return fmt.Errorf("failed to process batches: %w", err)
	}

	// 文档处理完成，更新状态
	if err := s.statusManager.MarkAsCompleted(ctx, fileID, len(segments)); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as completed")
		// 虽然状态更新失败，但文档处理成功，所以不返回错误
	}

	s.logger.WithFields(logrus.Fields{
		"file_id":       fileID,
		"segment_count": len(segments),
	}).Info("Document processing completed successfully")

	return nil
}

// parseDocument 解析文档内容
func (s *DocumentService) parseDocument(filePath string) (string, error) {
	s.logger.WithField("file_path", filePath).Debug("Parsing document")

	// 首先尝试从存储获取文件
	fileID := filepath.Base(filePath)
	// 移除扩展名
	fileID = strings.TrimSuffix(fileID, filepath.Ext(fileID))

	// 尝试获取文件
	reader, err := s.storage.Get(fileID)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get file directly, trying with path")
		// 尝试将整个路径作为ID
		reader, err = s.storage.Get(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to get file from storage: %w", err)
		}
	}
	defer reader.Close()

	// 创建解析器
	parser, err := document.ParserFactory(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create parser: %w", err)
	}

	// 直接从reader解析文档
	content, err := parser.ParseReader(reader, filePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse document: %w", err)
	}

	return content, nil
}

// splitContent 将内容分割成段落
func (s *DocumentService) splitContent(content string) ([]document.Content, error) {
	segments, err := s.splitter.Split(content)
	if err != nil {
		return nil, fmt.Errorf("failed to split content: %w", err)
	}

	return segments, nil
}

// processBatches 批量处理文本段落
func (s *DocumentService) processBatches(ctx context.Context, fileID string, filePath string, segments []document.Content) error {
	// 获取文件名
	fileName := filepath.Base(filePath)

	// 检查是否有段落需要处理
	if len(segments) == 0 {
		return nil
	}

	totalBatches := (len(segments) + s.batchSize - 1) / s.batchSize
	processedBatches := 0

	// 按批次处理
	for i := 0; i < len(segments); i += s.batchSize {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 继续处理
		}

		// 计算当前批次结束位置
		end := i + s.batchSize
		if end > len(segments) {
			end = len(segments)
		}

		// 获取当前批次的段落
		batch := segments[i:end]

		// 提取文本内容
		texts := make([]string, len(batch))
		for j, segment := range batch {
			texts[j] = segment.Text
		}

		// 生成向量嵌入
		vectors, err := s.embedder.EmbedBatch(ctx, texts)
		if err != nil {
			return fmt.Errorf("failed to generate embeddings: %w", err)
		}

		// 构建文档对象并存入向量数据库
		docs := make([]vectordb.Document, len(batch))
		dbSegments := make([]*models.DocumentSegment, len(batch))

		for j := range batch {
			segmentID := fmt.Sprintf("%s_%d", fileID, batch[j].Index)

			// 创建向量数据库文档
			docs[j] = vectordb.Document{
				ID:        segmentID,
				FileID:    fileID,
				FileName:  fileName,
				Position:  batch[j].Index,
				Text:      batch[j].Text,
				Vector:    vectors[j],
				CreatedAt: time.Now(),
			}

			// 创建数据库段落记录
			dbSegments[j] = &models.DocumentSegment{
				DocumentID: fileID,
				SegmentID:  segmentID,
				Position:   batch[j].Index,
				Text:       batch[j].Text,
			}
		}

		// 批量插入向量数据库
		if err := s.vectorDB.AddBatch(docs); err != nil {
			return fmt.Errorf("failed to add documents to vector database: %w", err)
		}

		// 批量保存段落到数据库
		if err := s.repo.SaveSegments(dbSegments); err != nil {
			return fmt.Errorf("failed to save document segments: %w", err)
		}

		processedBatches++
		// 计算并更新进度（20%到90%的范围）
		progress := 20 + int(float64(processedBatches)/float64(totalBatches)*70)
		if err := s.statusManager.UpdateProgress(ctx, fileID, progress); err != nil {
			s.logger.WithError(err).Warn("Failed to update document progress")
		}
	}

	return nil
}

// DeleteDocument 删除文档及其相关数据
func (s *DocumentService) DeleteDocument(ctx context.Context, fileID string) error {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return err
	}

	s.logger.WithField("file_id", fileID).Info("Deleting document")

	// 1. 从向量数据库中删除
	if err := s.vectorDB.DeleteByFileID(fileID); err != nil {
		// 可能文件在向量数据库中不存在，记录错误但继续
		s.logger.WithError(err).Warn("Failed to delete document from vector database")
	}

	// 2. 从存储中删除文件
	if err := s.storage.Delete(fileID); err != nil {
		// 文件可能已被删除，记录错误但不中断流程
		s.logger.WithError(err).Warn("Failed to delete file from storage")
	}

	// 如果使用Python微服务，则创建删除任务
	if s.usePython && s.queue != nil {
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
		}
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
func (s *DocumentService) GetDocumentInfo(ctx context.Context, fileID string) (map[string]interface{}, error) {
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
func (s *DocumentService) CountDocumentSegments(ctx context.Context, fileID string) (int, error) {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return 0, err
	}

	// 使用仓储统计段落数量
	return s.repo.CountSegments(fileID)
}

// ListDocuments 获取文档列表
func (s *DocumentService) ListDocuments(ctx context.Context, offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error) {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return nil, 0, err
	}

	// 使用状态管理器获取文档列表
	return s.statusManager.ListDocuments(ctx, offset, limit, filters)
}

// UpdateDocumentTags 更新文档标签
func (s *DocumentService) UpdateDocumentTags(ctx context.Context, fileID string, tags string) error {
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

// failDocument 将文档标记为失败状态
func (s *DocumentService) failDocument(ctx context.Context, fileID string, errorMsg string) {
	if s.statusManager == nil {
		s.logger.Error("Cannot mark document as failed: status manager not initialized")
		return
	}

	if err := s.statusManager.MarkAsFailed(ctx, fileID, errorMsg); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as failed")
	}
}

// GetStatusManager 返回文档状态管理器实例
func (s *DocumentService) GetStatusManager() *DocumentStatusManager {
	return s.statusManager
}

// CreateFromConfig 从配置创建文档服务
// 根据配置决定使用本地处理还是Python微服务处理
func CreateFromConfig(cfg *config.Config, storage storage.Storage, logger *logrus.Logger) (*DocumentService, error) {
	// 创建文本分段器
	splitterConfig := document.SplitterConfig{
		SplitType:    document.ByParagraph,
		ChunkSize:    1000,
		ChunkOverlap: 200,
	}
	splitter := document.NewTextSplitter(splitterConfig)

	// 创建嵌入客户端
	embeddingOptions := []embedding.Option{
		embedding.WithAPIKey(cfg.Embed.APIKey),
		embedding.WithBaseURL(cfg.Embed.Endpoint),
		embedding.WithModel(cfg.Embed.Model),
		embedding.WithTimeout(30 * time.Second),
	}

	embedder, err := embedding.NewClient("tongyi", embeddingOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding client: %w", err)
	}

	// 创建向量数据库
	vectorDBConfig := vectordb.Config{
		Type:              cfg.VectorDB.Type,
		Path:              cfg.VectorDB.Path,
		Dimension:         cfg.VectorDB.Dim,
		DistanceType:      vectordb.DistanceType(cfg.VectorDB.Distance),
		CreateIfNotExists: true,
	}

	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector database: %w", err)
	}

	// 创建文档解析器
	parser, err := document.ParserFactory("dummy.txt") // 先创建一个通用解析器，具体文件会在运行时确定
	if err != nil {
		return nil, fmt.Errorf("failed to create document parser: %w", err)
	}

	// 创建文档服务选项
	options := []DocumentOption{
		WithLogger(logger),
		WithBatchSize(cfg.Embed.BatchSize),
	}

	// 如果启用任务队列和Python处理，则创建任务队列并配置
	if cfg.TaskQueue.Enable && cfg.TaskQueue.PythonTasks.DocumentProcess {
		// 创建Redis任务队列
		redisConfig := taskqueue.RedisQueueConfig{
			Address:  cfg.TaskQueue.Address,
			Password: cfg.TaskQueue.Password,
			DB:       cfg.TaskQueue.DB,
			Prefix:   cfg.TaskQueue.Prefix,
			Timeout:  time.Duration(cfg.TaskQueue.Timeout) * time.Second,
		}

		queue, err := taskqueue.NewRedisQueue(redisConfig,
			taskqueue.WithMaxRetries(cfg.TaskQueue.MaxRetries),
			taskqueue.WithTimeout(time.Duration(cfg.TaskQueue.Timeout)*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create task queue: %w", err)
		}

		// 添加队列和Python处理选项
		options = append(options, WithTaskQueue(queue))
		options = append(options, WithPythonProcessing(true))

		logger.Info("Python document processing enabled via task queue")
	} else {
		logger.Info("Using local document processing")
	}

	// 创建本地文档服务
	docService := NewDocumentService(
		storage,
		parser,
		splitter,
		embedder,
		vectorDB,
		options...,
	)

	return docService, nil
}
