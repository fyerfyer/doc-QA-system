package adapters

import (
	"context"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/services"
)

// DocumentProcessingService 是一个通用的文档处理服务接口
// 可以由普通的文档服务或Python处理服务实现
type DocumentProcessingService interface {
	// Init 初始化服务
	Init() error

	// ProcessDocument 处理文档
	ProcessDocument(ctx context.Context, fileID string, filePath string) error

	// DeleteDocument 删除文档
	DeleteDocument(ctx context.Context, fileID string) error

	// GetDocumentInfo 获取文档信息
	GetDocumentInfo(ctx context.Context, fileID string) (map[string]interface{}, error)

	// CountDocumentSegments 统计文档段落数量
	CountDocumentSegments(ctx context.Context, fileID string) (int, error)

	// ListDocuments 获取文档列表
	ListDocuments(ctx context.Context, offset, limit int, filters map[string]interface{}) ([]*models.Document, int64, error)

	// UpdateDocumentTags 更新文档标签
	UpdateDocumentTags(ctx context.Context, fileID string, tags string) error

	// GetStatusManager 获取文档状态管理器
	GetStatusManager() *services.DocumentStatusManager
}
