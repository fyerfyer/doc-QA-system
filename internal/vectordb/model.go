package vectordb

import (
	"errors"
	"time"
)

// 常用错误定义
var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrEmptyVector      = errors.New("empty vector")
	ErrInvalidID        = errors.New("invalid document ID")
	ErrInvalidDimension = errors.New("vector dimension mismatch")
)

// Document 文档段落模型
// 包含向量表示及其元数据
type Document struct {
	ID        string                 // 唯一标识符
	FileID    string                 // 所属文件ID
	FileName  string                 // 文件名
	Position  int                    // 在原文档中的段落位置
	Text      string                 // 原始文本内容
	Vector    []float32              // 向量表示
	CreatedAt time.Time              // 创建时间
	Metadata  map[string]interface{} // 附加元数据
}

// DistanceType 向量距离计算方法
type DistanceType string

const (
	// Cosine 余弦相似度
	Cosine DistanceType = "cosine"
	// DotProduct 点积
	DotProduct DistanceType = "dot"
	// Euclidean 欧几里得距离
	Euclidean DistanceType = "l2"
)

// SearchResult 搜索结果
type SearchResult struct {
	Document Document // 文档对象
	Score    float32  // 相似度得分
	Distance float32  // 计算的距离
}

// SearchFilter 搜索过滤条件
type SearchFilter struct {
	FileIDs    []string               // 按文件ID过滤
	Metadata   map[string]interface{} // 按元数据过滤
	MinScore   float32                // 最小相似度分数
	MaxResults int                    // 最大返回结果数
}

// DefaultSearchFilter 返回默认的搜索过滤器
func DefaultSearchFilter() SearchFilter {
	return SearchFilter{
		MinScore:   0.0,
		MaxResults: 5,
	}
}

// Repository 向量数据库仓库接口
// 定义向量数据的基本操作
type Repository interface {
	// Add 添加单个文档
	Add(doc Document) error

	// AddBatch 批量添加文档
	AddBatch(docs []Document) error

	// Get 获取单个文档
	Get(id string) (Document, error)

	// Delete 删除单个文档
	Delete(id string) error

	// DeleteByFileID 删除指定文件的所有段落
	DeleteByFileID(fileID string) error

	// Search 相似度搜索
	Search(vector []float32, filter SearchFilter) ([]SearchResult, error)

	// Count 获取文档总数
	Count() (int, error)

	// GetDimension 返回向量维数
	GetDimension() int

	// Close 关闭数据库连接
	Close() error
}

// Config 向量数据库配置
type Config struct {
	Type              string       // 数据库类型，如 "memory", "faiss", "qdrant"
	Path              string       // 数据库文件路径或服务器地址
	Dimension         int          // 向量维度
	DistanceType      DistanceType // 距离计算类型
	CreateIfNotExists bool         // 如果不存在是否创建
	InMemory          bool         // 是否仅在内存中运行
}

// Factory 向量数据库工厂函数类型
type Factory func(config Config) (Repository, error)

// RepositoryRegistry 注册可用的向量数据库实现
var RepositoryRegistry = map[string]Factory{}

// RegisterRepository 注册向量数据库工厂函数
func RegisterRepository(name string, factory Factory) {
	RepositoryRegistry[name] = factory
}

// NewRepository 根据配置创建向量数据库实例
func NewRepository(config Config) (Repository, error) {
	factory, ok := RepositoryRegistry[config.Type]
	if !ok {
		// 默认使用内存实现
		factory = NewMemoryRepository
	}
	return factory(config)
}
