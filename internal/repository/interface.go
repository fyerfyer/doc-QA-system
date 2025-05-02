package repository

import "github.com/fyerfyer/doc-QA-system/internal/models"

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
