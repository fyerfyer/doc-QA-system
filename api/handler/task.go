package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// TaskHandler 处理任务相关的API请求
type TaskHandler struct {
	queue     taskqueue.Queue              // 任务队列
	processor *taskqueue.CallbackProcessor // 回调处理器
	logger    *logrus.Logger               // 日志记录器
}

// NewTaskHandler 创建新的任务处理器
func NewTaskHandler(queue taskqueue.Queue) *TaskHandler {
	logger := middleware.GetLogger()
	processor := taskqueue.GetSharedCallbackProcessor(queue, logger)

	// 不注册默认处理器
	// processor.RegisterDefaultHandlers(queue)

	return &TaskHandler{
		queue:     queue,
		processor: processor,
		logger:    logger,
	}
}

// HandleCallback 处理任务回调请求
// POST /api/tasks/callback
func (h *TaskHandler) HandleCallback(c *gin.Context) {
	var req taskqueue.CallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid callback request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的回调请求",
		))
		return
	}

	// 添加必要字段验证
	if req.TaskID == "" {
		h.logger.Warn("Empty task_id in callback request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"任务ID不能为空",
		))
		return
	}

	h.logger.WithFields(logrus.Fields{
		"task_id":     req.TaskID,
		"document_id": req.DocumentID,
		"status":      req.Status,
	}).Info("Received task callback")

	registeredHandlers := h.processor.GetRegisteredHandlerTypes()
	taskType := taskqueue.TaskType(req.Type)
	if _, exists := registeredHandlers[taskType]; !exists {
		h.logger.WithFields(logrus.Fields{
			"task_type":           req.Type,
			"registered_handlers": registeredHandlers,
		}).Warn("Handler not registered for this task type")
	}

	// 使用处理器处理回调
	resp, err := h.processor.HandleCallback(c.Request.Context(), &req)
	if err != nil {
		h.logger.WithError(err).Error("Failed to process callback")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"处理回调失败: "+err.Error(),
		))
		return
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// GetTaskStatus 获取任务状态
// GET /api/tasks/:id
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"任务ID不能为空",
		))
		return
	}

	task, err := h.queue.GetTask(c.Request.Context(), taskID)
	if err != nil {
		// 检查是否是任务不存在错误
		if errors.Is(err, taskqueue.ErrTaskNotFound) {
			c.JSON(http.StatusNotFound, model.NewErrorResponse(
				http.StatusNotFound,
				"任务未找到",
			))
			return
		}

		h.logger.WithError(err).WithField("task_id", taskID).Error("Failed to get task")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取任务状态失败: "+err.Error(),
		))
		return
	}

	if task == nil {
		c.JSON(http.StatusNotFound, model.NewErrorResponse(
			http.StatusNotFound,
			"任务未找到",
		))
		return
	}

	// 将任务信息转换为JSON安全的Map
	taskInfo := map[string]interface{}{
		"id":          task.ID,
		"type":        string(task.Type),
		"document_id": task.DocumentID,
		"status":      string(task.Status),
		"created_at":  task.CreatedAt,
		"updated_at":  task.UpdatedAt,
	}

	// 如果有错误信息，添加到响应中
	if task.Error != "" {
		taskInfo["error"] = task.Error
	}

	// 如果有结果，添加到响应中
	if len(task.Result) > 0 {
		var result map[string]interface{}
		if err := json.Unmarshal(task.Result, &result); err == nil {
			taskInfo["result"] = result
		}
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(taskInfo))
}

// GetDocumentTasks 获取文档相关的所有任务
// GET /api/document/:document_id/tasks
func (h *TaskHandler) GetDocumentTasks(c *gin.Context) {
	documentID := c.Param("document_id")
	if documentID == "" {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"文档ID不能为空",
		))
		return
	}

	tasks, err := h.queue.GetTasksByDocument(c.Request.Context(), documentID)
	if err != nil {
		h.logger.WithError(err).WithField("document_id", documentID).Error("Failed to get document tasks")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取文档任务列表失败: "+err.Error(),
		))
		return
	}

	// 将任务列表转换为JSON安全的格式
	tasksInfo := make([]map[string]interface{}, len(tasks))
	for i, task := range tasks {
		tasksInfo[i] = map[string]interface{}{
			"id":         task.ID,
			"type":       string(task.Type),
			"status":     string(task.Status),
			"created_at": task.CreatedAt,
			"updated_at": task.UpdatedAt,
		}

		if task.Error != "" {
			tasksInfo[i]["error"] = task.Error
		}
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(map[string]interface{}{
		"document_id": documentID,
		"tasks":       tasksInfo,
	}))
}
