package embedding

import (
	"context"
	"errors"
	"time"
)

// 常见的错误定义
var (
	ErrEmptyText     = errors.New("empty text for embedding")
	ErrBatchTooLarge = errors.New("batch size exceeds maximum allowed")
	ErrRateLimited   = errors.New("embedding API rate limited")
)

// Client 嵌入向量客户端接口
// 负责将文本转换为向量表示
type Client interface {
	// Embed 将单个文本转换为向量
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch 批量将多个文本转换为向量
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// Config 嵌入客户端配置
type Config struct {
	Provider   string        // 提供商名称 (如 "openai", "local" 等)
	Model      string        // 模型名称 (如 "text-embedding-3-small")
	APIKey     string        // API密钥
	Endpoint   string        // API端点URL
	BatchSize  int           // 批处理最大大小
	Timeout    time.Duration // 请求超时时间
	Dimensions int           // 向量维度
	Retries    int           // 失败重试次数
}

// DefaultConfig 返回默认的嵌入配置
func DefaultConfig() Config {
	return Config{
		Provider:   "openai",
		Model:      "text-embedding-3-small",
		Endpoint:   "https://api.openai.com/v1",
		BatchSize:  16,
		Timeout:    time.Second * 30,
		Dimensions: 1536, // 适用于OpenAI text-embedding-3-small
		Retries:    3,
	}
}

// Factory 创建嵌入客户端的工厂函数类型
type Factory func(config Config) (Client, error)

// ClientRegistry 注册可用的嵌入客户端
var ClientRegistry = map[string]Factory{}

// RegisterClient 注册嵌入客户端工厂函数
func RegisterClient(name string, factory Factory) {
	ClientRegistry[name] = factory
}

// NewClient 根据配置创建新的嵌入客户端
func NewClient(config Config) (Client, error) {
	factory, ok := ClientRegistry[config.Provider]
	if !ok {
		// 默认使用OpenAI
		factory = NewOpenAIClient
	}
	return factory(config)
}

// EmbeddingResult 包含文本ID和其嵌入向量
type EmbeddingResult struct {
	ID     string    // 文本标识符
	Text   string    // 原始文本
	Vector []float32 // 嵌入向量
}

// EmbeddingRequest 嵌入请求参数
type EmbeddingRequest struct {
	Texts      []string      // 要嵌入的文本
	MaxRetries int           // 最大重试次数
	Timeout    time.Duration // 超时时间
}

// BatchProcessor 将大批量文本分成较小批次处理的工具接口
type BatchProcessor interface {
	// Process 处理批量文本并返回对应的向量
	Process(ctx context.Context, texts []string) ([][]float32, error)
}
