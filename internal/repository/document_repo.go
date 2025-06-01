package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"gorm.io/gorm"
)

// docRepository 文档仓储实现
type docRepository struct {
	db        *gorm.DB        // 数据库连接
	taskQueue taskqueue.Queue // 任务队列
	ctx       context.Context // 上下文，可用于事务或超时控制
}

// NewDocumentRepository 创建文档仓储实例
func NewDocumentRepository() DocumentRepository {
	return &docRepository{
		db:  database.MustDB(),
		ctx: context.Background(),
	}
}

// NewDocumentRepositoryWithDB 使用指定的数据库连接创建文档仓储实例
func NewDocumentRepositoryWithDB(db *gorm.DB) DocumentRepository {
	if db == nil {
		db = database.MustDB()
	}
	return &docRepository{
		db:  db,
		ctx: context.Background(),
	}
}

// NewDocumentRepositoryWithQueue 使用指定的数据库连接和任务队列创建文档仓储实例
func NewDocumentRepositoryWithQueue(db *gorm.DB, queue taskqueue.Queue) DocumentRepository {
	if db == nil {
		db = database.MustDB()
	}
	return &docRepository{
		db:        db,
		taskQueue: queue,
		ctx:       context.Background(),
	}
}

// Create 创建文档记录
func (r *docRepository) Create(doc *models.Document) error {
	if doc.ID == "" {
		return errors.New("document ID cannot be empty")
	}

	return r.db.Create(doc).Error
}

// Update 更新文档记录
func (r *docRepository) Update(doc *models.Document) error {
	if doc.ID == "" {
		return errors.New("document ID cannot be empty")
	}

	return r.db.Save(doc).Error
}

// GetByID 根据ID获取文档
func (r *docRepository) GetByID(id string) (*models.Document, error) {
	var doc models.Document
	err := r.db.Where("id = ?", id).First(&doc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("document not found: %s", id)
		}
		return nil, err
	}
	return &doc, nil
}

// List 列出文档列表，支持分页和筛选
func (r *docRepository) List(offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error) {
	var docs []*models.Document
	var total int64

	// 创建查询构造器
	query := r.db.Model(&models.Document{})

	// 应用筛选条件
	if filters != nil {
		// 状态过滤
		if status, ok := filters["status"]; ok {
			// 处理不同类型的status
			switch s := status.(type) {
			case models.DocumentStatus:
				// 如果是DocumentStatus类型，转换为string
				query = query.Where("status = ?", string(s))
			case string:
				// 如果已经是string，直接使用
				if s != "" {
					query = query.Where("status = ?", s)
				}
			default:
				// 其他类型，尝试转换为string
				statusStr := fmt.Sprintf("%v", status)
				if statusStr != "" {
					query = query.Where("status = ?", statusStr)
				}
			}
		}

		// 标签过滤
		if tags, ok := filters["tags"].(string); ok && tags != "" {
			// 使用LIKE查询匹配包含指定标签的文档
			query = query.Where("tags LIKE ?", "%"+tags+"%")
		}

		// 时间范围过滤
		if startTime, ok := filters["start_time"].(string); ok && startTime != "" {
			query = query.Where("uploaded_at >= ?", startTime)
		}

		if endTime, ok := filters["end_time"].(string); ok && endTime != "" {
			query = query.Where("uploaded_at <= ?", endTime)
		}

		// 文件名过滤
		if fileName, ok := filters["file_name"].(string); ok && fileName != "" {
			query = query.Where("file_name LIKE ?", "%"+fileName+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 应用排序、分页并执行查询
	err = query.Order("uploaded_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&docs).Error

	if err != nil {
		return nil, 0, err
	}

	return docs, total, nil
}

// Delete 删除文档记录
func (r *docRepository) Delete(id string) error {
	// 开启事务
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 删除文档段落
		if err := tx.Where("document_id = ?", id).Delete(&models.DocumentSegment{}).Error; err != nil {
			return err
		}

		// 2. 删除文档记录
		if err := tx.Where("id = ?", id).Delete(&models.Document{}).Error; err != nil {
			return err
		}

		// 3. 如果任务队列已初始化，尝试获取并删除相关任务
		if r.taskQueue != nil {
			ctx := r.getContext()
			tasks, err := r.taskQueue.GetTasksByDocument(ctx, id)
			if err == nil && len(tasks) > 0 {
				for _, task := range tasks {
					// 忽略错误，因为任务可能已经被删除
					_ = r.taskQueue.DeleteTask(ctx, task.ID)
				}
			}
		}

		return nil
	})
}

// UpdateStatus 更新文档状态
func (r *docRepository) UpdateStatus(id string, status models.DocumentStatus, errorMsg string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	// 如果有错误消息，更新错误字段
	if errorMsg != "" {
		updates["error"] = errorMsg
	}

	// 如果状态是已完成或失败，设置处理完成时间
	if status == models.DocStatusCompleted || status == models.DocStatusFailed {
		now := time.Now()
		updates["processed_at"] = &now
	}

	return r.db.Model(&models.Document{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateProgress 更新文档处理进度
func (r *docRepository) UpdateProgress(id string, progress int) error {
	// 确保进度在0-100范围内
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	return r.db.Model(&models.Document{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"progress":   progress,
			"updated_at": time.Now(),
		}).Error
}

// SaveSegment 保存文档段落
func (r *docRepository) SaveSegment(segment *models.DocumentSegment) error {
	return r.db.Create(segment).Error
}

// SaveSegments 批量保存段落
func (r *docRepository) SaveSegments(segments []*models.DocumentSegment) error {
	if len(segments) == 0 {
		return nil
	}

	// 使用事务批量插入
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 批量创建记录
		return tx.CreateInBatches(segments, 100).Error
	})
}

// GetSegments 获取文档的所有段落
func (r *docRepository) GetSegments(docID string) ([]*models.DocumentSegment, error) {
	var segments []*models.DocumentSegment
	err := r.db.Where("document_id = ?", docID).
		Order("position ASC").
		Find(&segments).Error
	return segments, err
}

// CountSegments 统计文档的段落数量
func (r *docRepository) CountSegments(docID string) (int, error) {
	var count int64
	err := r.db.Model(&models.DocumentSegment{}).
		Where("document_id = ?", docID).
		Count(&count).Error
	return int(count), err
}

// DeleteSegments 删除文档的所有段落
func (r *docRepository) DeleteSegments(docID string) error {
	return r.db.Where("document_id = ?", docID).
		Delete(&models.DocumentSegment{}).Error
}

// WithContext 创建带有上下文的仓储
func (r *docRepository) WithContext(ctx context.Context) DocumentRepository {
	return &docRepository{
		db:        r.db.WithContext(ctx),
		taskQueue: r.taskQueue,
		ctx:       ctx,
	}
}

// getContext 获取仓储的上下文，如果未设置则使用背景上下文
func (r *docRepository) getContext() context.Context {
	if r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

// GetDocumentTasks 获取文档相关的所有任务
func (r *docRepository) GetDocumentTasks(ctx context.Context, documentID string) ([]*taskqueue.Task, error) {
	if r.taskQueue == nil {
		return nil, errors.New("task queue not initialized")
	}

	return r.taskQueue.GetTasksByDocument(ctx, documentID)
}

// GetTaskByID 根据ID获取任务
func (r *docRepository) GetTaskByID(ctx context.Context, taskID string) (*taskqueue.Task, error) {
	if r.taskQueue == nil {
		return nil, errors.New("task queue not initialized")
	}

	return r.taskQueue.GetTask(ctx, taskID)
}

// CreateTask 创建任务并关联到文档
func (r *docRepository) CreateTask(ctx context.Context, taskType taskqueue.TaskType, documentID string, payload interface{}) (string, error) {
	if r.taskQueue == nil {
		return "", errors.New("task queue not initialized")
	}

	// 检查文档是否存在
	_, err := r.GetByID(documentID)
	if err != nil {
		return "", err
	}

	// 将任务加入队列
	taskID, err := r.taskQueue.Enqueue(ctx, taskType, documentID, payload)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	// 更新文档状态为处理中
	err = r.UpdateStatus(documentID, models.DocStatusProcessing, "")
	if err != nil {
		// 记录错误但继续，因为任务已创建
		fmt.Printf("Failed to update document status: %v\n", err)
	}

	return taskID, nil
}

// UpdateTaskStatus 更新任务状态
func (r *docRepository) UpdateTaskStatus(ctx context.Context, taskID string, status taskqueue.TaskStatus, result interface{}, errorMsg string) error {
	if r.taskQueue == nil {
		return errors.New("task queue not initialized")
	}

	// 获取任务信息
	task, err := r.taskQueue.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	// 更新任务状态
	if err := r.taskQueue.UpdateTaskStatus(ctx, taskID, status, result, errorMsg); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// 通知任务状态更新
	if err := r.taskQueue.NotifyTaskUpdate(ctx, taskID); err != nil {
		// 记录错误但继续，通知失败不是致命错误
		fmt.Printf("Failed to notify task update: %v\n", err)
	}

	// 根据任务状态更新文档状态
	if task.DocumentID != "" {
		var docStatus models.DocumentStatus
		var docError string

		switch status {
		case taskqueue.StatusCompleted:
			docStatus = models.DocStatusCompleted
		case taskqueue.StatusFailed:
			docStatus = models.DocStatusFailed
			docError = errorMsg
		case taskqueue.StatusProcessing:
			docStatus = models.DocStatusProcessing
		default:
			// 对于其他状态，不更新文档状态
			return nil
		}

		// 更新文档状态
		err = r.UpdateStatus(task.DocumentID, docStatus, docError)
		if err != nil {
			return fmt.Errorf("failed to update document status: %w", err)
		}
	}

	return nil
}

// DeleteTask 删除任务
func (r *docRepository) DeleteTask(ctx context.Context, taskID string) error {
	if r.taskQueue == nil {
		return errors.New("task queue not initialized")
	}

	return r.taskQueue.DeleteTask(ctx, taskID)
}
