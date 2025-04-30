package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"gorm.io/gorm"
)

// DocumentRepository 文档仓储接口
// 负责文档元数据的存储和检索
type DocumentRepository interface {
	// Create 创建文档记录
	Create(doc *models.Document) error

	// Update 更新文档记录
	Update(doc *models.Document) error

	// GetByID 根据ID获取文档
	GetByID(id string) (*models.Document, error)

	// List 列出文档列表，支持分页和筛选
	List(offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error)

	// Delete 删除文档
	Delete(id string) error

	// UpdateStatus 更新文档状态
	UpdateStatus(id string, status models.DocumentStatus, errorMsg string) error

	// UpdateProgress 更新文档处理进度
	UpdateProgress(id string, progress int) error

	// SaveSegment 保存文档段落
	SaveSegment(segment *models.DocumentSegment) error

	// SaveSegments 批量保存文档段落
	SaveSegments(segments []*models.DocumentSegment) error

	// GetSegments 获取文档的所有段落
	GetSegments(docID string) ([]*models.DocumentSegment, error)

	// CountSegments 统计文档的段落数量
	CountSegments(docID string) (int, error)

	// DeleteSegments 删除文档的所有段落
	DeleteSegments(docID string) error
}

// docRepository 文档仓储实现
type docRepository struct {
	db *gorm.DB // 数据库连接
}

// NewDocumentRepository 创建文档仓储实例
func NewDocumentRepository() DocumentRepository {
	return &docRepository{
		db: database.DB,
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
		if status, ok := filters["status"].(string); ok && status != "" {
			query = query.Where("status = ?", status)
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
// 可用于事务处理或超时控制
func (r *docRepository) WithContext(ctx context.Context) DocumentRepository {
	return &docRepository{
		db: r.db.WithContext(ctx),
	}
}

// 批量保存段落
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
