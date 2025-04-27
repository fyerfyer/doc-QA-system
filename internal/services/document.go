package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
)

// 文档处理状态
const (
	DocStatusPending    = "pending"    // 待处理
	DocStatusProcessing = "processing" // 处理中
	DocStatusCompleted  = "completed"  // 已完成
	DocStatusFailed     = "failed"     // 处理失败
)

// DocumentService 文档服务
// 负责协调文档解析、分段、嵌入和存储
type DocumentService struct {
	storage   storage.Storage     // 文件存储服务
	parser    document.Parser     // 文档解析器
	splitter  document.Splitter   // 文本分段器
	embedder  embedding.Client    // 嵌入模型客户端
	vectorDB  vectordb.Repository // 向量数据库
	batchSize int                 // 批处理大小
	timeout   time.Duration       // 处理超时时间
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
		storage:   storage,
		parser:    parser,
		splitter:  splitter,
		embedder:  embedder,
		vectorDB:  vectorDB,
		batchSize: 16,              // 默认批处理大小
		timeout:   time.Minute * 5, // 默认超时时间
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

// ProcessDocument 处理文档(解析、分段、向量化、入库)
func (s *DocumentService) ProcessDocument(ctx context.Context, fileID string, filePath string) error {
	fmt.Printf("DEBUG: ProcessDocument started with fileID=%s, filePath=%s\n", fileID, filePath)

	// 检查输入参数
	if fileID == "" {
		return errors.New("fileID cannot be empty")
	}
	if filePath == "" {
		return errors.New("filePath cannot be empty")
	}

	// 设置上下文超时
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// 解析文档内容
	fmt.Printf("DEBUG: Parsing document content\n")
	content, err := s.parseDocument(filePath)
	if err != nil {
		fmt.Printf("DEBUG: Document parsing failed: %v\n", err)
		return fmt.Errorf("failed to parse document: %w", err)
	}
	fmt.Printf("DEBUG: Document parsed successfully, content length: %d\n", len(content))

	// 文本分段
	fmt.Printf("DEBUG: Splitting content\n")
	segments, err := s.splitContent(content)
	if err != nil {
		fmt.Printf("DEBUG: Content splitting failed: %v\n", err)
		return fmt.Errorf("failed to split content: %w", err)
	}
	fmt.Printf("DEBUG: Content split into %d segments\n", len(segments))

	// 批量处理文本段落
	fmt.Printf("DEBUG: Processing batches\n")
	err = s.processBatches(ctx, fileID, filePath, segments)
	if err != nil {
		fmt.Printf("DEBUG: Batch processing failed: %v\n", err)
		return fmt.Errorf("failed to process batches: %w", err)
	}
	fmt.Printf("DEBUG: Batch processing completed successfully\n")

	return nil
}

// parseDocument 解析文档内容
func (s *DocumentService) parseDocument(filePath string) (string, error) {
	fmt.Printf("DEBUG: Attempting to parse document from path: %s\n", filePath)

	// 首先尝试从存储获取文件
	fileID := filepath.Base(filePath)
	// 移除扩展名
	fileID = strings.TrimSuffix(fileID, filepath.Ext(fileID))

	// 尝试获取文件
	reader, err := s.storage.Get(fileID)
	if err != nil {
		fmt.Printf("DEBUG: Failed to get file '%s' directly, trying with path: %v\n", fileID, err)
		// 尝试将整个路径作为ID
		reader, err = s.storage.Get(filePath)
		if err != nil {
			fmt.Printf("DEBUG: Failed to get file with path '%s': %v\n", filePath, err)
			return "", fmt.Errorf("failed to get file from storage: %w", err)
		}
	}
	defer reader.Close()

	fmt.Printf("DEBUG: Successfully retrieved file from storage\n")

	// 创建解析器
	parser, err := document.ParserFactory(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create parser: %w", err)
	}

	// 直接从reader解析文档
	content, err := parser.ParseReader(reader, filePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse document: %w", err)
	}

	return content, nil
}

// splitContent 将内容分割成段落
func (s *DocumentService) splitContent(content string) ([]document.Content, error) {
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
		for j := range batch {
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
		}

		// 批量插入向量数据库
		if err := s.vectorDB.AddBatch(docs); err != nil {
			return fmt.Errorf("failed to store vectors: %w", err)
		}
	}

	return nil
}

// DeleteDocument 删除文档及其相关数据
func (s *DocumentService) DeleteDocument(ctx context.Context, fileID string) error {
	// 从向量数据库中删除
	if err := s.vectorDB.DeleteByFileID(fileID); err != nil {
		return fmt.Errorf("failed to delete document vectors: %w", err)
	}

	// 从存储中删除文件
	if err := s.storage.Delete(fileID); err != nil {
		// 文件可能已被删除，记录错误但不中断流程
		fmt.Printf("Warning: failed to delete file from storage: %v\n", err)
	}

	return nil
}

// GetDocumentInfo 获取文档信息
func (s *DocumentService) GetDocumentInfo(ctx context.Context, fileID string) (map[string]interface{}, error) {
	// 这里可以实现获取文档元信息的逻辑
	// 例如从数据库或缓存中获取文档的处理状态、创建时间等

	// TODO: 实现文档信息存储和检索
	// 注意：这需要一个存储文档元信息的组件，可能是关系数据库或NoSQL

	// 临时返回基本信息
	return map[string]interface{}{
		"file_id": fileID,
		"status":  DocStatusCompleted,
	}, nil
}

// CountDocumentSegments 统计文档段落数量
func (s *DocumentService) CountDocumentSegments(ctx context.Context, fileID string) (int, error) {
	// TODO: 实现统计段落数量的逻辑
	// 可以通过向量数据库的查询功能实现

	// 临时实现：获取所有段落并计数
	filter := vectordb.SearchFilter{
		FileIDs:    []string{fileID},
		MaxResults: 0, // 不限制结果数量
	}

	// 使用一个空向量查询，主要是为了应用过滤器
	// 注意：这不是最优实现，应该有更高效的计数方法
	dummyVector := make([]float32, s.vectorDB.(interface{ Dimension() int }).Dimension())
	results, err := s.vectorDB.Search(dummyVector, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count document segments: %w", err)
	}

	return len(results), nil
}
