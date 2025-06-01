package taskqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// TaskCallbackHandler 任务回调处理函数类型
// 处理特定类型任务的回调，返回处理结果
type TaskCallbackHandler func(ctx context.Context, task *Task, result json.RawMessage) error

// CallbackProcessor 回调处理器
// 负责接收和处理任务回调
type CallbackProcessor struct {
	queue     Queue                            // 任务队列
	handlers  map[TaskType]TaskCallbackHandler // 任务类型对应的处理函数
	defaultFn TaskCallbackHandler              // 默认处理函数
	logger    *logrus.Logger                   // 日志记录器
}

// NewCallbackProcessor 创建新的回调处理器
func NewCallbackProcessor(queue Queue, logger *logrus.Logger) *CallbackProcessor {
	if logger == nil {
		logger = logrus.New()
	}

	return &CallbackProcessor{
		queue:    queue,
		handlers: make(map[TaskType]TaskCallbackHandler),
		logger:   logger,
	}
}

// RegisterHandler 注册特定类型的任务处理函数
func (p *CallbackProcessor) RegisterHandler(taskType TaskType, handler TaskCallbackHandler) {
	p.handlers[taskType] = handler
	p.logger.Infof("Registered handler for task type: %s", taskType)
}

// ProcessCallback 处理回调数据
func (p *CallbackProcessor) ProcessCallback(ctx context.Context, callbackData []byte) error {
	// 解析回调数据
	var callback TaskCallback
	if err := json.Unmarshal(callbackData, &callback); err != nil {
		return fmt.Errorf("failed to unmarshal callback data: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"task_id":     callback.TaskID,
		"document_id": callback.DocumentID,
		"status":      callback.Status,
		"type":        callback.Type,
	}).Info("Processing task callback")

	// 获取任务
	task, err := p.queue.GetTask(ctx, callback.TaskID)
	if err != nil {
		p.logger.WithError(err).Errorf("Failed to get task: %s", callback.TaskID)
		return fmt.Errorf("failed to get task: %w", err)
	}

	// 更新任务状态
	err = p.queue.UpdateTaskStatus(ctx, callback.TaskID, callback.Status, callback.Result, callback.Error)
	if err != nil {
		p.logger.WithError(err).Errorf("Failed to update task status: %s", callback.TaskID)
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// 通知状态更新
	if err := p.queue.NotifyTaskUpdate(ctx, callback.TaskID); err != nil {
		// 继续处理，不返回错误
	}

	// 如果任务失败，记录错误但不调用处理函数
	if callback.Status == StatusFailed {
		p.logger.WithFields(logrus.Fields{
			"task_id": callback.TaskID,
			"error":   callback.Error,
		}).Error("Task failed")
	}

	// 找到对应的处理函数
	handlerType := TaskType(callback.Type) // 将字符串转换为TaskType
	handler, exists := p.handlers[handlerType]
	if !exists {
		handler = p.defaultFn
		p.logger.WithField("type", callback.Type).Info("No handler registered for task type: " + callback.Type)
	}

	// 如果没有处理函数，直接返回
	if handler == nil {
		p.logger.Debug("No handler available for task type: " + callback.Type)
		return nil
	}

	// 调用处理函数
	p.logger.Debugf("Calling handler for task: %s (type: %s)", task.ID, task.Type)
	return handler(ctx, task, callback.Result)
}

// CallbackRequest HTTP回调请求结构体
type CallbackRequest struct {
	TaskID     string          `json:"task_id"`     // 任务ID
	DocumentID string          `json:"document_id"` // 文档ID
	Status     TaskStatus      `json:"status"`      // 任务状态
	Type       TaskType        `json:"type"`        // 任务类型
	Result     json.RawMessage `json:"result"`      // 任务结果
	Error      string          `json:"error"`       // 错误信息
	Timestamp  string          `json:"timestamp"`   // 时间戳
}

// CallbackResponse HTTP回调响应结构体
type CallbackResponse struct {
	Success   bool   `json:"success"`           // 是否成功
	Message   string `json:"message,omitempty"` // 消息
	TaskID    string `json:"task_id"`           // 任务ID
	Timestamp string `json:"timestamp"`         // 时间戳
}

// HandleCallback 处理HTTP回调请求
func (p *CallbackProcessor) HandleCallback(ctx context.Context, req *CallbackRequest) (*CallbackResponse, error) {
	// 记录返回的回调信息
	p.logger.WithFields(logrus.Fields{
		"task_id":     req.TaskID,
		"document_id": req.DocumentID,
		"status":      req.Status,
		"type":        req.Type,
	}).Info("Received callback request")

	// 使用不同的时间格式解析时间戳，以兼容py-services的时间格式
	var timestamp time.Time
	if req.Timestamp != "" {
		formats := []string{
			time.RFC3339,                 // 2006-01-02T15:04:05Z07:00
			"2006-01-02T15:04:05Z",       // 带Z的UTC时间
			"2006-01-02T15:04:05.999999", // 带毫秒不带时区
			"2006-01-02T15:04:05",        // 不带时区
		}

		var parseErr error
		for _, format := range formats {
			timestamp, parseErr = time.Parse(format, req.Timestamp)
			if parseErr == nil {
				break
			}
		}

		if parseErr != nil {
			// 如果解析失败，使用当前时间并记录警告
			p.logger.WithFields(logrus.Fields{
				"timestamp": req.Timestamp,
				"error":     parseErr,
			}).Warn("Failed to parse timestamp, using current time")
			timestamp = time.Now()
		}
	} else {
		// 如果没有提供时间戳，使用当前时间
		timestamp = time.Now()
	}

	// 创建回调对象
	callback := &TaskCallback{
		TaskID:     req.TaskID,
		DocumentID: req.DocumentID,
		Status:     req.Status,
		Type:       req.Type,
		Result:     req.Result,
		Error:      req.Error,
		Timestamp:  timestamp,
	}

	callbackData, err := json.Marshal(callback)
	if err != nil {
		p.logger.WithError(err).Error("Failed to marshal callback data")
		return &CallbackResponse{
			Success:   false,
			Message:   fmt.Sprintf("failed to marshal callback: %v", err),
			TaskID:    req.TaskID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}, err
	}

	// 处理回调
	err = p.ProcessCallback(ctx, callbackData)
	if err != nil {
		p.logger.WithError(err).Error("Failed to process callback")
		return &CallbackResponse{
			Success:   false,
			Message:   err.Error(),
			TaskID:    req.TaskID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}, err
	}

	return &CallbackResponse{
		Success:   true,
		Message:   "Task callback processed successfully",
		TaskID:    req.TaskID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// DefaultDocumentParseHandler 默认的文档解析回调处理函数
// 处理完成后创建分块任务
func DefaultDocumentParseHandler(ctx context.Context, queue Queue, logger *logrus.Logger) TaskCallbackHandler {
	return func(ctx context.Context, task *Task, result json.RawMessage) error {
		// 解析结果
		var parseResult DocumentParseResult
		if err := json.Unmarshal(result, &parseResult); err != nil {
			logger.WithError(err).Error("Failed to unmarshal document parse result")
			return fmt.Errorf("failed to unmarshal document parse result: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"task_id":     task.ID,
			"document_id": task.DocumentID,
			"title":       parseResult.Title,
			"chars":       parseResult.Chars,
		}).Info("Document parse completed")

		// 如果文档内容为空，不创建后续任务
		if parseResult.Content == "" {
			logger.Warn("Empty document content, skipping chunk task")
			return nil
		}

		// 创建文本分块任务
		chunkPayload := TextChunkPayload{
			DocumentID: task.DocumentID,
			Content:    parseResult.Content,
			ChunkSize:  1000,        // 默认分块大小
			Overlap:    200,         // 默认重叠大小
			SplitType:  "paragraph", // 默认分割类型
		}

		// 将任务加入队列
		taskID, err := queue.Enqueue(ctx, TaskTextChunk, task.DocumentID, chunkPayload)
		if err != nil {
			logger.WithError(err).Error("Failed to enqueue chunk task")
			return fmt.Errorf("failed to enqueue chunk task: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"document_id":   task.DocumentID,
			"chunk_task_id": taskID,
		}).Info("Created text chunk task")

		return nil
	}
}

// DefaultTextChunkHandler 默认的文本分块回调处理函数
// 处理完成后创建向量化任务
func DefaultTextChunkHandler(ctx context.Context, queue Queue, logger *logrus.Logger) TaskCallbackHandler {
	return func(ctx context.Context, task *Task, result json.RawMessage) error {
		// 解析结果
		var chunkResult TextChunkResult
		if err := json.Unmarshal(result, &chunkResult); err != nil {
			logger.WithError(err).Error("Failed to unmarshal text chunk result")
			return fmt.Errorf("failed to unmarshal text chunk result: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"task_id":     task.ID,
			"document_id": task.DocumentID,
			"chunk_count": chunkResult.ChunkCount,
		}).Info("Text chunking completed")

		// 如果没有分块，不创建向量化任务
		if len(chunkResult.Chunks) == 0 {
			logger.Warn("No text chunks generated, skipping vectorization")
			return nil
		}

		// 创建向量化任务
		vectorizePayload := VectorizePayload{
			DocumentID: task.DocumentID,
			Chunks:     chunkResult.Chunks,
			Model:      "default", // 默认嵌入模型
		}

		// 将任务加入队列
		taskID, err := queue.Enqueue(ctx, TaskVectorize, task.DocumentID, vectorizePayload)
		if err != nil {
			logger.WithError(err).Error("Failed to enqueue vectorize task")
			return fmt.Errorf("failed to enqueue vectorize task: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"document_id":       task.DocumentID,
			"vectorize_task_id": taskID,
			"chunk_count":       len(chunkResult.Chunks),
		}).Info("Created vectorization task")

		return nil
	}
}

// DefaultVectorizeHandler 默认的向量化回调处理函数
// 向量化是任务流程的最后一步，处理完成后更新文档状态
func DefaultVectorizeHandler(ctx context.Context, queue Queue, logger *logrus.Logger) TaskCallbackHandler {
	return func(ctx context.Context, task *Task, result json.RawMessage) error {
		// 解析结果
		var vectorizeResult VectorizeResult
		if err := json.Unmarshal(result, &vectorizeResult); err != nil {
			logger.WithError(err).Error("Failed to unmarshal vectorize result")
			return fmt.Errorf("failed to unmarshal vectorize result: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"task_id":      task.ID,
			"document_id":  task.DocumentID,
			"vector_count": vectorizeResult.VectorCount,
			"model":        vectorizeResult.Model,
			"dimension":    vectorizeResult.Dimension,
		}).Info("Vectorization completed")

		// TODO: 处理向量数据，存入向量数据库
		// 这部分逻辑应由调用方实现，此处仅提供回调框架
		// 调用方应在服务层中注册具体的处理函数

		return nil
	}
}

// DefaultProcessCompleteHandler 默认的完整处理流程回调处理函数
func DefaultProcessCompleteHandler(ctx context.Context, queue Queue, logger *logrus.Logger) TaskCallbackHandler {
	return func(ctx context.Context, task *Task, result json.RawMessage) error {
		// 解析结果
		var completeResult ProcessCompleteResult
		if err := json.Unmarshal(result, &completeResult); err != nil {
			logger.WithError(err).Error("Failed to unmarshal process complete result")
			return fmt.Errorf("failed to unmarshal process complete result: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"task_id":       task.ID,
			"document_id":   task.DocumentID,
			"chunk_count":   completeResult.ChunkCount,
			"vector_count":  completeResult.VectorCount,
			"parse_status":  completeResult.ParseStatus,
			"chunk_status":  completeResult.ChunkStatus,
			"vector_status": completeResult.VectorStatus,
		}).Info("Document processing completed")

		// TODO: 处理完整流程的结果
		// 这部分逻辑应由调用方实现，此处仅提供回调框架

		return nil
	}
}

// RegisterDefaultHandlers 注册默认的任务处理函数
func (p *CallbackProcessor) RegisterDefaultHandlers(queue Queue) {
	p.RegisterHandler(TaskDocumentParse, DefaultDocumentParseHandler(context.Background(), queue, p.logger))
	p.RegisterHandler(TaskTextChunk, DefaultTextChunkHandler(context.Background(), queue, p.logger))
	p.RegisterHandler(TaskVectorize, DefaultVectorizeHandler(context.Background(), queue, p.logger))
	p.RegisterHandler(TaskProcessComplete, DefaultProcessCompleteHandler(context.Background(), queue, p.logger))

	p.logger.Info("Registered default task handlers")
}

func (p *CallbackProcessor) GetRegisteredHandlerTypes() map[TaskType]bool {
	result := make(map[TaskType]bool)
	for handlerType := range p.handlers {
		result[handlerType] = true
	}
	return result
}
