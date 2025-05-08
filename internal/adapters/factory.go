package adapters

import (
	"context"
	"fmt"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"time"

	"github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/taskqueue"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/sirupsen/logrus"
)

// DocumentProcessingService 是一个通用的文档处理服务接口
// 可以由普通的文档服务或Python处理服务实现
type DocumentProcessingService interface {
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

// CreateDocumentService 创建合适的文档处理服务
// 基于配置决定是使用本地处理还是Python微服务处理
func CreateDocumentService(
	cfg *config.Config,
	fileStorage storage.Storage,
	logger *logrus.Logger,
) (DocumentProcessingService, error) {
	// 检查是否启用Python处理
	if cfg.TaskQueue.Enable && cfg.TaskQueue.PythonTasks.DocumentProcess {
		return createPythonDocumentService(cfg, fileStorage, logger)
	}

	// 默认使用本地处理服务
	return createLocalDocumentService(cfg, fileStorage, logger)
}

// createPythonDocumentService 创建Python文档处理服务
func createPythonDocumentService(
	cfg *config.Config,
	fileStorage storage.Storage,
	logger *logrus.Logger,
) (*PythonDocumentService, error) {
	// 创建任务队列
	queueOpts := []taskqueue.QueueOption{
		taskqueue.WithMaxRetries(cfg.TaskQueue.MaxRetries),
		taskqueue.WithTimeout(time.Duration(cfg.TaskQueue.Timeout) * time.Second),
	}

	redisConfig := taskqueue.RedisQueueConfig{
		Address:  cfg.TaskQueue.Address,
		Password: cfg.TaskQueue.Password,
		DB:       cfg.TaskQueue.DB,
		Prefix:   cfg.TaskQueue.Prefix,
		Timeout:  time.Duration(cfg.TaskQueue.Timeout) * time.Second,
	}

	queue, err := taskqueue.NewRedisQueue(redisConfig, queueOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create task queue: %w", err)
	}

	// 创建文档状态管理器
	repo := repository.NewDocumentRepository()
	statusManager := services.NewDocumentStatusManager(repo, logger)

	// 创建Python文档服务
	pythonService := NewPythonDocumentService(
		fileStorage,
		queue,
		statusManager,
		WithPythonLogger(logger),
		WithPythonTimeout(time.Duration(cfg.TaskQueue.Timeout)*time.Second),
	)

	return pythonService, nil
}

// createLocalDocumentService 创建本地文档处理服务
func createLocalDocumentService(
	cfg *config.Config,
	fileStorage storage.Storage,
	logger *logrus.Logger,
) (*services.DocumentService, error) {
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

	// 创建本地文档服务
	docService := services.NewDocumentService(
		fileStorage,
		parser,
		splitter,
		embedder,
		vectorDB,
		services.WithLogger(logger),
		services.WithBatchSize(cfg.Embed.BatchSize),
	)

	return docService, nil
}
