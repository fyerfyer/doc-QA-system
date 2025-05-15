package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"

	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/sirupsen/logrus"
)

// AsyncDocumentOptions 异步文档处理的选项
type AsyncDocumentOptions struct {
	ChunkSize    int               // 分块大小
	ChunkOverlap int               // 分块重叠
	SplitType    string            // 分割类型
	Model        string            // 嵌入模型
	Metadata     map[string]string // 元数据
	Priority     string            // 任务优先级
}

// DefaultAsyncOptions 返回默认的异步处理选项
func DefaultAsyncOptions() *AsyncDocumentOptions {
	return &AsyncDocumentOptions{
		ChunkSize:    1000,
		ChunkOverlap: 200,
		SplitType:    "paragraph",
		Model:        "default",
		Priority:     "default",
	}
}

// EnableAsyncProcessing 启用异步处理
func (s *DocumentService) EnableAsyncProcessing(queue taskqueue.Queue) {
	s.asyncEnabled = true
	s.taskQueue = queue

	// 确保重要依赖已设置
	if s.statusManager == nil {
		s.logger.Warn("Status manager not set, creating default one")
		if s.repo == nil {
			s.repo = s.createDefaultRepository()
		}
		s.statusManager = NewDocumentStatusManager(s.repo, s.logger)
	}

	// 使用已有的仓库和新的队列创建新的仓库
	s.repo = repository.NewDocumentRepositoryWithQueue(database.DB, queue)

	// 注册任务回调处理器
	s.registerTaskHandlers()

	s.logger.Info("Async document processing enabled")
}

// DisableAsyncProcessing 禁用异步处理
func (s *DocumentService) DisableAsyncProcessing() {
	s.asyncEnabled = false
	s.logger.Info("Async document processing disabled")
}

// processDocumentAsync 异步处理文档
// 将任务加入队列并立即返回
func (s *DocumentService) processDocumentAsync(ctx context.Context, fileID string, filePath string, options *AsyncDocumentOptions) error {
	s.logger.WithFields(logrus.Fields{
		"file_id":   fileID,
		"file_path": filePath,
	}).Info("Enqueuing document for async processing")

	if !s.asyncEnabled || s.taskQueue == nil {
		return fmt.Errorf("async processing not enabled or task queue not configured")
	}

	// 确保有选项
	if options == nil {
		options = DefaultAsyncOptions()
	}

	// 更新文档状态为处理中
	if err := s.statusManager.MarkAsProcessing(ctx, fileID); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as processing")
		return fmt.Errorf("failed to update document status: %w", err)
	}

	// 创建处理任务载荷
	fileName := filepath.Base(filePath)
	fileType := filepath.Ext(fileName)
	if fileType != "" && fileType[0] == '.' {
		fileType = fileType[1:] // 去掉开头的点号
	}

	// 修改为HTTP调用Python API
	pythonServiceURL := os.Getenv("PYTHONSERVICE_URL")
	if pythonServiceURL == "" {
		pythonServiceURL = "http://py-api:8000"
	}

	// 准备API请求参数
	requestBody := map[string]interface{}{
		"document_id": fileID,
		"file_path":   filePath,
		"file_name":   fileName,
		"file_type":   fileType,
		"chunk_size":  options.ChunkSize,
		"overlap":     options.ChunkOverlap,
		"split_type":  options.SplitType,
		"model":       options.Model,
		"metadata":    options.Metadata,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal document processing request")
		return fmt.Errorf("failed to marshal document processing request: %w", err)
	}

	// 发送HTTP请求到Python服务
	req, err := http.NewRequestWithContext(ctx, "POST", pythonServiceURL+"/api/tasks/process", bytes.NewBuffer(jsonBody))
	if err != nil {
		s.logger.WithError(err).Error("Failed to create document processing request")
		return fmt.Errorf("failed to create document processing request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.WithError(err).Error("Failed to send document processing request")
		return fmt.Errorf("failed to send document processing request: %w", err)
	}
	defer resp.Body.Close()

	// 处理响应
	var respBody struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		s.logger.WithError(err).Error("Failed to decode document processing response")
		return fmt.Errorf("failed to decode document processing response: %w", err)
	}

	// 使用响应中的任务ID
	taskID := respBody.TaskID

	s.logger.WithFields(logrus.Fields{
		"file_id": fileID,
		"task_id": taskID,
	}).Info("Document processing task created successfully")

	return nil
}

// ProcessDocumentAsync 异步处理文档
func (s *DocumentService) ProcessDocumentAsync(ctx context.Context, fileID string, filePath string, opts ...AsyncOption) error {
	options := DefaultAsyncOptions()

	// 应用选项
	for _, opt := range opts {
		opt(options)
	}

	return s.processDocumentAsync(ctx, fileID, filePath, options)
}

// AsyncOption 异步选项函数类型
type AsyncOption func(*AsyncDocumentOptions)

// WithChunkSize 设置分块大小
func WithChunkSize(size int) AsyncOption {
	return func(o *AsyncDocumentOptions) {
		o.ChunkSize = size
	}
}

// WithChunkOverlap 设置分块重叠大小
func WithChunkOverlap(overlap int) AsyncOption {
	return func(o *AsyncDocumentOptions) {
		o.ChunkOverlap = overlap
	}
}

// WithSplitType 设置分割类型
func WithSplitType(splitType string) AsyncOption {
	return func(o *AsyncDocumentOptions) {
		o.SplitType = splitType
	}
}

// WithEmbeddingModel 设置嵌入模型
func WithEmbeddingModel(model string) AsyncOption {
	return func(o *AsyncDocumentOptions) {
		o.Model = model
	}
}

// WithMetadata 设置元数据
func WithMetadata(metadata map[string]string) AsyncOption {
	return func(o *AsyncDocumentOptions) {
		o.Metadata = metadata
	}
}

// WithPriority 设置任务优先级
func WithPriority(priority string) AsyncOption {
	return func(o *AsyncDocumentOptions) {
		o.Priority = priority
	}
}

// registerTaskHandlers 注册任务回调处理器
func (s *DocumentService) registerTaskHandlers() {
	if s.taskQueue == nil {
		s.logger.Warn("Task queue not available, cannot register handlers")
		return
	}

	// 创建回调处理器
	processor := taskqueue.NewCallbackProcessor(s.taskQueue, s.logger)

	// 注册文档解析任务处理器
	processor.RegisterHandler(taskqueue.TaskDocumentParse, s.handleDocumentParseResult)

	// 注册文本分块任务处理器
	processor.RegisterHandler(taskqueue.TaskTextChunk, s.handleTextChunkResult)

	// 注册向量化任务处理器
	processor.RegisterHandler(taskqueue.TaskVectorize, s.handleVectorizeResult)

	// 注册完整流程任务处理器
	processor.RegisterHandler(taskqueue.TaskProcessComplete, s.handleProcessCompleteResult)

	s.logger.Info("Registered task handlers")
}

// handleDocumentParseResult 处理文档解析任务结果
func (s *DocumentService) handleDocumentParseResult(ctx context.Context, task *taskqueue.Task, result json.RawMessage) error {
	s.logger.WithFields(logrus.Fields{
		"task_id":     task.ID,
		"document_id": task.DocumentID,
	}).Info("Handling document parse result")

	// 解析结果
	var parseResult taskqueue.DocumentParseResult
	if err := json.Unmarshal(result, &parseResult); err != nil {
		return fmt.Errorf("failed to unmarshal document parse result: %w", err)
	}

	// 更新文档处理进度
	if err := s.statusManager.UpdateProgress(ctx, task.DocumentID, 30); err != nil {
		s.logger.WithError(err).Warn("Failed to update document progress")
	}

	// 检查内容是否为空
	if parseResult.Content == "" {
		err := fmt.Errorf("empty document content")
		_ = s.statusManager.MarkAsFailed(ctx, task.DocumentID, err.Error())
		return err
	}

	return nil
}

// handleTextChunkResult 处理文本分块任务结果
func (s *DocumentService) handleTextChunkResult(ctx context.Context, task *taskqueue.Task, result json.RawMessage) error {
	s.logger.WithFields(logrus.Fields{
		"task_id":     task.ID,
		"document_id": task.DocumentID,
	}).Info("Handling text chunk result")

	// 解析结果
	var chunkResult taskqueue.TextChunkResult
	if err := json.Unmarshal(result, &chunkResult); err != nil {
		return fmt.Errorf("failed to unmarshal text chunk result: %w", err)
	}

	// 更新文档处理进度
	if err := s.statusManager.UpdateProgress(ctx, task.DocumentID, 60); err != nil {
		s.logger.WithError(err).Warn("Failed to update document progress")
	}

	return nil
}

// handleVectorizeResult 处理向量化任务结果
func (s *DocumentService) handleVectorizeResult(ctx context.Context, task *taskqueue.Task, result json.RawMessage) error {
	s.logger.WithFields(logrus.Fields{
		"task_id":     task.ID,
		"document_id": task.DocumentID,
	}).Info("Handling vectorize result")

	// 解析结果
	var vectorizeResult taskqueue.VectorizeResult
	if err := json.Unmarshal(result, &vectorizeResult); err != nil {
		return fmt.Errorf("failed to unmarshal vectorize result: %w", err)
	}

	// 将向量数据保存到向量数据库
	if len(vectorizeResult.Vectors) > 0 {
		// 更新文档信息
		if err := s.saveVectorsToDatabase(ctx, task.DocumentID, &vectorizeResult); err != nil {
			s.logger.WithError(err).Error("Failed to save vectors to database")
			return err
		}
	}

	// 更新文档完成状态
	if err := s.statusManager.MarkAsCompleted(ctx, task.DocumentID, vectorizeResult.VectorCount); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as completed")
		return err
	}

	return nil
}

// handleProcessCompleteResult 处理完整流程任务结果
func (s *DocumentService) handleProcessCompleteResult(ctx context.Context, task *taskqueue.Task, result json.RawMessage) error {
	s.logger.WithFields(logrus.Fields{
		"task_id":     task.ID,
		"document_id": task.DocumentID,
	}).Info("Handling process complete result")

	// 解析结果
	var completeResult taskqueue.ProcessCompleteResult
	if err := json.Unmarshal(result, &completeResult); err != nil {
		return fmt.Errorf("failed to unmarshal process complete result: %w", err)
	}

	// 检查处理状态
	if completeResult.Error != "" {
		s.logger.WithFields(logrus.Fields{
			"document_id": task.DocumentID,
			"error":       completeResult.Error,
		}).Error("Document processing failed")

		// 标记文档为失败状态
		if err := s.statusManager.MarkAsFailed(ctx, task.DocumentID, completeResult.Error); err != nil {
			s.logger.WithError(err).Error("Failed to mark document as failed")
		}
		return fmt.Errorf("document processing failed: %s", completeResult.Error)
	}

	// 如果向量数据已生成，保存到向量数据库
	if len(completeResult.Vectors) > 0 {
		// 处理向量数据
		vectorResult := taskqueue.VectorizeResult{
			DocumentID:  task.DocumentID,
			Vectors:     completeResult.Vectors,
			VectorCount: completeResult.VectorCount,
			Model:       strconv.Itoa(completeResult.Dimension),
			Dimension:   completeResult.Dimension,
		}

		if err := s.saveVectorsToDatabase(ctx, task.DocumentID, &vectorResult); err != nil {
			s.logger.WithError(err).Error("Failed to save vectors from complete result")
			return err
		}
	}

	// 标记文档为已完成状态
	if err := s.statusManager.MarkAsCompleted(ctx, task.DocumentID, completeResult.ChunkCount); err != nil {
		s.logger.WithError(err).Error("Failed to mark document as completed")
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"document_id":  task.DocumentID,
		"chunk_count":  completeResult.ChunkCount,
		"vector_count": completeResult.VectorCount,
	}).Info("Document processing completed successfully")

	return nil
}

// saveVectorsToDatabase 将向量保存到向量数据库
func (s *DocumentService) saveVectorsToDatabase(ctx context.Context, documentID string, result *taskqueue.VectorizeResult) error {
	// 获取文档信息，用于保存向量数据
	doc, err := s.statusManager.GetDocument(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to get document info: %w", err)
	}

	// 构建文档对象批量列表
	docs := make([]vectordb.Document, 0, len(result.Vectors))
	for _, vector := range result.Vectors {
		// 检查向量数据有效性
		if vector.ChunkIndex < 0 || len(vector.Vector) == 0 {
			s.logger.WithFields(logrus.Fields{
				"chunk_index": vector.ChunkIndex,
				"document_id": documentID,
			}).Warn("Invalid vector data, skipping")
			continue
		}

		// 将float64向量转换为float32向量(如果需要)
		vectorData := make([]float32, len(vector.Vector))
		copy(vectorData, vector.Vector)

		// 构建向量数据库文档对象
		vectorDoc := vectordb.Document{
			ID:        fmt.Sprintf("%s_%d", documentID, vector.ChunkIndex),
			FileID:    documentID,
			FileName:  doc.FileName,
			Position:  vector.ChunkIndex,
			Vector:    vectorData,
			CreatedAt: time.Now(),
			Metadata: map[string]interface{}{
				"file_type": doc.FileType,
			},
		}

		docs = append(docs, vectorDoc)
	}

	// 批量添加到向量数据库
	if len(docs) > 0 {
		if err := s.vectorDB.AddBatch(docs); err != nil {
			return fmt.Errorf("failed to add vectors to database: %w", err)
		}
		s.logger.WithFields(logrus.Fields{
			"document_id":  documentID,
			"vector_count": len(docs),
		}).Info("Vectors saved to database")
	}

	return nil
}

// createDefaultRepository 创建默认的文档仓库
func (s *DocumentService) createDefaultRepository() repository.DocumentRepository {
	return repository.NewDocumentRepository()
}

// WaitForTaskResult 等待任务完成并返回结果
func (s *DocumentService) WaitForTaskResult(ctx context.Context, taskID string, timeout time.Duration) (*taskqueue.Task, error) {
	if !s.asyncEnabled || s.taskQueue == nil {
		return nil, fmt.Errorf("async processing not enabled or task queue not configured")
	}

	// 设置超时上下文
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 等待任务完成
	task, err := s.taskQueue.WaitForTask(ctx, taskID, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for task: %w", err)
	}

	// 检查任务状态
	if task.Status == taskqueue.StatusFailed {
		return task, fmt.Errorf("task failed: %s", task.Error)
	}

	return task, nil
}

// GetDocumentTasks 获取文档相关的任务列表
func (s *DocumentService) GetDocumentTasks(ctx context.Context, documentID string) ([]*taskqueue.Task, error) {
	if !s.asyncEnabled || s.taskQueue == nil {
		return nil, fmt.Errorf("async processing not enabled or task queue not configured")
	}

	return s.taskQueue.GetTasksByDocument(ctx, documentID)
}
