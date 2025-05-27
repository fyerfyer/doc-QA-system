package services

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/pyprovider"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/google/uuid"
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
	taskQueue     taskqueue.Queue               // 任务队列
	asyncEnabled  bool                          // 是否启用异步处理
	batchSize     int                           // 批处理大小
	timeout       time.Duration                 // 处理超时时间
	logger        *logrus.Logger                // 日志记录器
	pythonClient  *pyprovider.DocumentClient    // Python文档解析客户端
	usePythonAPI  bool                          // 是否使用Python API
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
		storage:      storage,
		parser:       parser,
		splitter:     splitter,
		embedder:     embedder,
		vectorDB:     vectorDB,
		batchSize:    16,              // 默认批处理大小
		timeout:      time.Minute * 5, // 默认超时时间
		logger:       logrus.New(),    // 默认日志记录器
		asyncEnabled: false,           // 默认不启用异步处理
		usePythonAPI: false,           // 默认不使用Python API
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
		s.taskQueue = queue
		s.asyncEnabled = queue != nil
	}
}

// WithAsyncProcessing 设置是否启用异步处理
func WithAsyncProcessing(enabled bool) DocumentOption {
	return func(s *DocumentService) {
		s.asyncEnabled = enabled
	}
}

// WithPythonClient 配置Python文档解析客户端
func WithPythonClient(client *pyprovider.DocumentClient) DocumentOption {
	return func(s *DocumentService) {
		s.pythonClient = client
		s.usePythonAPI = client != nil
	}
}

// WithUsePythonAPI 设置是否使用Python API
func WithUsePythonAPI(enabled bool) DocumentOption {
	return func(s *DocumentService) {
		s.usePythonAPI = enabled
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

	// 如果启用了Python API但没有设置客户端，尝试创建默认的Python客户端
	if s.usePythonAPI && s.pythonClient == nil {
		s.logger.Info("Attempting to create default Python document client")
		config := pyprovider.DefaultConfig()
		baseClient, err := pyprovider.NewClient(config)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to create Python client, disabling Python API")
			s.usePythonAPI = false
		} else {
			s.pythonClient = pyprovider.NewDocumentClient(baseClient)
			s.logger.Info("Successfully created default Python document client")
		}
	}

	return nil
}

// processDocumentSync 同步处理文档
// 直接在当前进程中处理文档
func (s *DocumentService) processDocumentSync(ctx context.Context, fileID string, filePath string) error {
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
// 优先使用Python API解析，如果不可用或失败则回退到本地解析
func (s *DocumentService) parseDocument(filePath string) (string, error) {
	s.logger.WithField("file_path", filePath).Debug("parsing document")

	// 如果启用了Python API且客户端已设置，尝试使用Python解析
	if s.usePythonAPI && s.pythonClient != nil {
		s.logger.Debug("attempting to parse document using Python API")

		// 创建解析上下文
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		// 尝试使用Python客户端解析文档
		result, err := s.pythonClient.ParseDocument(ctx, filePath)
		if err != nil {
			s.logger.WithError(err).Warn("failed to parse document using Python API")
			// 这里不返回，继续使用本地解析作为回退
		} else {
			s.logger.WithField("content_length", len(result.Content)).Info("Python解析成功")
			return result.Content, nil
		}
	}

	// 回退到本地解析逻辑
	s.logger.Debug("falling back to local parser")

	// 首先尝试从存储获取文件
	fileID := filepath.Base(filePath)
	// 移除扩展名
	fileID = strings.TrimSuffix(fileID, filepath.Ext(fileID))

	// 尝试获取文件
	reader, err := s.storage.Get(fileID)
	if err != nil {
		// 尝试将整个路径作为ID
		reader, err = s.storage.Get(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}
	defer reader.Close()

	// 如果设置了解析器，直接使用
	if s.parser != nil {
		return s.parser.ParseReader(reader, filePath)
	}

	// 否则使用工厂创建解析器
	parser, err := document.ParserFactory(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create parser: %w", err)
	}

	// 解析文档
	content, err := parser.ParseReader(reader, filePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse document: %w", err)
	}

	return content, nil
}

// parseDocumentWithReader 从reader解析文档
// 优先使用Python API解析，如果不可用或失败则回退到本地解析
func (s *DocumentService) parseDocumentWithReader(reader io.Reader, fileName string) (string, error) {
	// 如果启用了Python API且客户端已设置，尝试使用Python解析
	if s.usePythonAPI && s.pythonClient != nil {
		s.logger.Debug("Attempting to parse document from reader using Python API")

		// 需要一个可重复读取的reader，因为如果Python解析失败，我们需要再次读取内容
		// 这里我们先读取所有内容到内存中
		content, err := io.ReadAll(reader)
		if err != nil {
			return "", fmt.Errorf("failed to read file content: %w", err)
		}

		// 为Python API创建一个新的reader
		pythonReader := strings.NewReader(string(content))

		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		// 尝试使用Python API解析
		result, err := s.pythonClient.ParseDocumentWithReader(ctx, pythonReader, fileName)
		if err == nil && result != nil {
			s.logger.Info("Successfully parsed document from reader using Python API")
			return result.Content, nil
		}

		// 如果Python解析失败，记录错误并回退到本地解析
		s.logger.WithError(err).Warn("Failed to parse document from reader using Python API, falling back to local parser")

		// 为本地解析创建新的reader
		localReader := strings.NewReader(string(content))

		// 如果设置了解析器，使用设置的解析器
		if s.parser != nil {
			return s.parser.ParseReader(localReader, fileName)
		}

		// 否则创建新的解析器
		parser, err := document.ParserFactory(fileName)
		if err != nil {
			return "", fmt.Errorf("failed to create parser: %w", err)
		}

		return parser.ParseReader(localReader, fileName)
	}

	// 如果没有启用Python API，直接使用本地解析
	if s.parser != nil {
		return s.parser.ParseReader(reader, fileName)
	}

	// 使用工厂创建解析器
	parser, err := document.ParserFactory(fileName)
	if err != nil {
		return "", fmt.Errorf("failed to create parser: %w", err)
	}

	return parser.ParseReader(reader, fileName)
}

// splitContent 使用python API或本地分块器进行文本分块
func (s *DocumentService) splitContent(content string) ([]document.Content, error) {
	if s.usePythonAPI && s.pythonClient != nil {
		s.logger.Debug("using Python text chunker")

		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		// 生成一个临时文档ID
		tempDocID := fmt.Sprintf("temp_%s", uuid.New().String()[:8])

		options := &pyprovider.SplitOptions{
			ChunkSize:    s.splitter.(*document.PythonSplitter).GetChunkSize(),
			ChunkOverlap: s.splitter.(*document.PythonSplitter).GetOverlap(),
			SplitType:    "sentence", 
			StoreResult:  false,      // 临时分块不需要存储
		}

		// 调用python API进行文本分块
		pyContents, _, err := s.pythonClient.SplitText(ctx, content, tempDocID, options)
		if err != nil {
			s.logger.WithError(err).Warn("python chunking failed, falling back to local chunking")
		} else {
			// 将Python返回的内容转换为本地格式
			contents := make([]document.Content, len(pyContents))
			for i, pyContent := range pyContents {
				contents[i] = document.Content{
					Text:  pyContent.Text,
					Index: pyContent.Index,
				}
			}
			return contents, nil
		}
	}

	// Use local chunker as fallback
	return s.splitContentLocal(content)
}

// splitContentLocal 使用本地分块器进行文本分块
// 注：此函数将作为回退方案，在迁移全部完成后可以移除
func (s *DocumentService) splitContentLocal(content string) ([]document.Content, error) {
	s.logger.Debug("Using local text chunker")
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
			// 创建向量数据库文档
			docs[j] = vectordb.Document{
				ID:        fmt.Sprintf("%s_%d", fileID, batch[j].Index),
				FileID:    fileID,
				FileName:  fileName,
				Position:  batch[j].Index,
				Text:      batch[j].Text,
				Vector:    vectors[j],
				CreatedAt: time.Now(),
				Metadata: map[string]interface{}{
					"source": filePath,
					"index":  batch[j].Index,
				},
			}

			// 创建数据库段落记录
			dbSegments[j] = &models.DocumentSegment{
				DocumentID: fileID,
				SegmentID:  fmt.Sprintf("%s_%d", fileID, batch[j].Index),
				Position:   batch[j].Index,
				Text:       batch[j].Text,
			}
		}

		// 批量插入向量数据库
		if err := s.vectorDB.AddBatch(docs); err != nil {
			return fmt.Errorf("failed to store vectors: %w", err)
		}

		// 批量保存段落到数据库
		if err := s.repo.SaveSegments(dbSegments); err != nil {
			s.logger.WithError(err).Error("Failed to save segments to database")
			// 不中断处理
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

// ProcessDocument 处理文档
// 解析文档内容，分段处理并生成向量表示，存入向量数据库
func (s *DocumentService) ProcessDocument(ctx context.Context, fileID string, filePath string) error {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return fmt.Errorf("failed to initialize document service: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"file_id":       fileID,
		"file_path":     filePath,
		"async_enabled": s.asyncEnabled,
	}).Info("Processing document")

	// 如果启用了异步处理，将任务加入队列
	if s.asyncEnabled && s.taskQueue != nil {
		s.logger.Info("Using async processing for document")
		// 使用默认的异步处理选项
		return s.ProcessDocumentAsync(ctx, fileID, filePath)
	}

	// 否则，使用同步处理
	s.logger.Info("Using sync processing for document")
	return s.processDocumentSync(ctx, fileID, filePath)
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
		s.logger.WithError(err).Error("Failed to delete document vectors")
		return fmt.Errorf("failed to delete document vectors: %w", err)
	}

	// 2. 从存储中删除文件
	if err := s.storage.Delete(fileID); err != nil {
		// 文件可能已被删除，记录错误但不中断流程
		s.logger.WithError(err).Warn("Failed to delete file from storage")
	}

	// 3. 删除文档状态记录
	if err := s.statusManager.DeleteDocument(ctx, fileID); err != nil {
		s.logger.WithError(err).Error("Failed to delete document status record")
		return fmt.Errorf("failed to delete document status record: %w", err)
	}

	// 4. 如果任务队列已配置，删除相关任务
	if s.taskQueue != nil {
		tasks, err := s.repo.GetDocumentTasks(ctx, fileID)
		if err == nil && len(tasks) > 0 {
			for _, task := range tasks {
				if err := s.repo.DeleteTask(ctx, task.ID); err != nil {
					s.logger.WithError(err).WithField("task_id", task.ID).Warn("Failed to delete document task")
				}
			}
		}
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

	// 如果启用了异步处理，尝试获取相关任务信息
	if s.asyncEnabled && s.taskQueue != nil {
		tasks, err := s.repo.GetDocumentTasks(ctx, fileID)
		if err == nil && len(tasks) > 0 {
			// 添加最近的任务信息
			latestTask := tasks[0]
			for _, task := range tasks {
				if task.UpdatedAt.After(latestTask.UpdatedAt) {
					latestTask = task
				}
			}

			info["task_id"] = latestTask.ID
			info["task_status"] = latestTask.Status
			info["task_created_at"] = latestTask.CreatedAt.Format(time.RFC3339)
			info["task_updated_at"] = latestTask.UpdatedAt.Format(time.RFC3339)

			if latestTask.StartedAt != nil {
				info["task_started_at"] = latestTask.StartedAt.Format(time.RFC3339)
			}
			if latestTask.CompletedAt != nil {
				info["task_completed_at"] = latestTask.CompletedAt.Format(time.RFC3339)
			}
			if latestTask.Error != "" {
				info["task_error"] = latestTask.Error
			}
		}
	}

	return info, nil
}

// GetDocumentStatus 获取文档处理状态
func (s *DocumentService) GetDocumentStatus(ctx context.Context, fileID string) (models.DocumentStatus, error) {
	// 确保初始化完成
	if err := s.Init(); err != nil {
		return "", err
	}

	return s.statusManager.GetStatus(ctx, fileID)
}

// WaitForDocumentProcessing 等待文档处理完成
func (s *DocumentService) WaitForDocumentProcessing(ctx context.Context, fileID string, timeout time.Duration) error {
	// s.logger.WithFields(logrus.Fields{
	//     "file_id": fileID,
	//     "timeout": timeout,
	//     "async_enabled": s.asyncEnabled,
	//     "task_queue_initialized": s.taskQueue != nil,
	//     "repo_initialized": s.repo != nil,
	// }).Info("Waiting for document processing")

	// 确保初始化完成
	if err := s.Init(); err != nil {
		s.logger.WithError(err).Error("Failed to initialize document service")
		return err
	}

	if !s.asyncEnabled || s.taskQueue == nil {
		// 如果未启用异步处理，直接检查文档状态
		// s.logger.Info("Async processing not enabled, checking document status directly")
		status, err := s.statusManager.GetStatus(ctx, fileID)
		if err != nil {
			return err
		}
		if status == models.DocStatusFailed {
			return fmt.Errorf("document processing failed")
		}
		if status != models.DocStatusCompleted {
			return fmt.Errorf("document not processed")
		}
		return nil
	}

	// 设置上下文超时
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 获取文档相关的任务
	// s.logger.WithField("document_id", fileID).Info("Getting document tasks")
	tasks, err := s.repo.GetDocumentTasks(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get document tasks: %w", err)
	}

	if len(tasks) == 0 {
		return fmt.Errorf("no processing tasks found for document %s", fileID)
	}

	// 找到最新的处理任务
	var latestTask *taskqueue.Task
	for _, task := range tasks {
		// s.logger.WithFields(logrus.Fields{
		//     "task_id": task.ID,
		//     "task_type": task.Type,
		//     "task_status": task.Status,
		//     "created_at": task.CreatedAt,
		// }).Info("Task details")

		if task.Type == taskqueue.TaskProcessComplete {
			if latestTask == nil || task.CreatedAt.After(latestTask.CreatedAt) {
				latestTask = task
			}
		}
	}

	if latestTask == nil {
		return fmt.Errorf("no complete processing task found for document %s", fileID)
	}

	// s.logger.WithFields(logrus.Fields{
	//     "task_id": latestTask.ID,
	//     "task_status": latestTask.Status,
	// }).Info("Waiting for task to complete")

	// 等待任务完成
	_, err = s.taskQueue.WaitForTask(ctx, latestTask.ID, timeout)
	if err != nil {
		s.logger.WithError(err).Error("Failed to wait for task")
		return fmt.Errorf("failed to wait for document processing: %w", err)
	}

	// 再次检查文档状态
	status, err := s.statusManager.GetStatus(ctx, fileID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get document status after waiting")
		return err
	}

	// s.logger.WithField("status", status).Info("Document status after waiting")

	if status == models.DocStatusFailed {
		s.logger.Error("Document processing failed after waiting")
		return fmt.Errorf("document processing failed")
	}

	if status != models.DocStatusCompleted {
		s.logger.WithField("status", status).Error("Document processing incomplete after waiting")
		return fmt.Errorf("document processing incomplete")
	}

	// s.logger.Info("Document processing completed successfully")

	return nil
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
		s.logger.WithFields(logrus.Fields{
			"file_id": fileID,
			"error":   err,
		}).Error("Failed to mark document as failed")
	}
}

// GetStatusManager 返回文档状态管理器实例
func (s *DocumentService) GetStatusManager() *DocumentStatusManager {
	return s.statusManager
}

// GetTaskQueue 返回任务队列实例
func (s *DocumentService) GetTaskQueue() taskqueue.Queue {
	return s.taskQueue
}
