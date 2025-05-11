package repository

import (
	"context"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
)

// DocumentRepository 文档仓储接口
// 负责文档元数据的存储和检索
type DocumentRepository interface {
	// 基础文档操作

	// Create 创建文档记录
	Create(doc *models.Document) error

	// Update 更新文档记录
	Update(doc *models.Document) error

	// GetByID 根据ID获取文档
	GetByID(id string) (*models.Document, error)

	// List 列出文档列表，支持分页和筛选
	List(offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error)

	// Delete 删除文档记录
	Delete(id string) error

	// 状态和进度

	// UpdateStatus 更新文档状态
	UpdateStatus(id string, status models.DocumentStatus, errorMsg string) error

	// UpdateProgress 更新文档处理进度
	UpdateProgress(id string, progress int) error

	// 文档段落相关

	// SaveSegment 保存文档段落
	SaveSegment(segment *models.DocumentSegment) error

	// SaveSegments 批量保存段落
	SaveSegments(segments []*models.DocumentSegment) error

	// GetSegments 获取文档的所有段落
	GetSegments(docID string) ([]*models.DocumentSegment, error)

	// CountSegments 统计文档的段落数量
	CountSegments(docID string) (int, error)

	// DeleteSegments 删除文档的所有段落
	DeleteSegments(docID string) error

	// 任务相关

	// GetDocumentTasks 获取文档相关的所有任务
	GetDocumentTasks(ctx context.Context, documentID string) ([]*taskqueue.Task, error)

	// GetTaskByID 根据ID获取任务
	GetTaskByID(ctx context.Context, taskID string) (*taskqueue.Task, error)

	// CreateTask 创建任务并关联到文档
	CreateTask(ctx context.Context, taskType taskqueue.TaskType, documentID string, payload interface{}) (string, error)

	// UpdateTaskStatus 更新任务状态
	UpdateTaskStatus(ctx context.Context, taskID string, status taskqueue.TaskStatus, result interface{}, errorMsg string) error

	// DeleteTask 删除任务
	DeleteTask(ctx context.Context, taskID string) error

	// 事务支持

	// WithContext 创建带有上下文的仓储
	// 可用于事务处理或超时控制
	WithContext(ctx context.Context) DocumentRepository
}

// TaskQueueAdapter 任务队列适配器
// 连接文档仓储和任务队列
type TaskQueueAdapter interface {
	// EnqueueDocumentProcessing 将文档处理任务加入队列
	EnqueueDocumentProcessing(ctx context.Context, documentID string, options map[string]interface{}) (string, error)

	// EnqueueDocumentParsing 将文档解析任务加入队列
	EnqueueDocumentParsing(ctx context.Context, documentID string, filePath string) (string, error)

	// EnqueueTextChunking 将文本分块任务加入队列
	EnqueueTextChunking(ctx context.Context, documentID string, content string, options map[string]interface{}) (string, error)

	// EnqueueVectorization 将向量化任务加入队列
	EnqueueVectorization(ctx context.Context, documentID string, chunks []taskqueue.ChunkInfo, model string) (string, error)

	// WaitForTaskCompletion 等待任务完成
	WaitForTaskCompletion(ctx context.Context, taskID string, timeout int) (*taskqueue.Task, error)

	// GetTaskQueue 获取任务队列实例
	GetTaskQueue() taskqueue.Queue
}
