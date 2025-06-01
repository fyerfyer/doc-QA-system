package embedding

import (
	"context"
	"time"
)

// Client 嵌入模型客户端接口
// 负责将文本转换为向量表示
type Client interface {
	// Embed 生成单条文本的向量表示
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch 批量生成多条文本的向量表示
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Name 返回模型名称
	Name() string
}

// Config 嵌入客户端配置
type Config struct {
	APIKey      string        // API密钥
	BaseURL     string        // API基础URL
	Model       string        // 模型名称
	Timeout     time.Duration // 请求超时时间
	MaxRetries  int           // 最大重试次数
	Dimensions  int           // 向量维度
	BatchSize   int           // 批处理大小
	EnableCache bool          // 是否启用缓存
}

// Option 客户端配置选项函数类型
type Option func(*Config)

// WithAPIKey 设置API密钥
func WithAPIKey(apiKey string) Option {
	return func(c *Config) {
		c.APIKey = apiKey
	}
}

// WithBaseURL 设置API基础URL
func WithBaseURL(url string) Option {
	return func(c *Config) {
		c.BaseURL = url
	}
}

// WithModel 设置模型名称
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithTimeout 设置请求超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(retries int) Option {
	return func(c *Config) {
		c.MaxRetries = retries
	}
}

// WithDimensions 设置向量维度
func WithDimensions(dimensions int) Option {
	return func(c *Config) {
		c.Dimensions = dimensions
	}
}

// WithBatchSize 设置批处理大小
func WithBatchSize(size int) Option {
	return func(c *Config) {
		c.BatchSize = size
	}
}

// WithCache 启用或禁用缓存
func WithCache(enable bool) Option {
	return func(c *Config) {
		c.EnableCache = enable
	}
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		BaseURL:     "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding",
		Model:       "text-embedding-v1", // 通义千问默认嵌入模型
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		Dimensions:  1024, // 通义千问模型默认维度，可能需要根据实际模型调整
		BatchSize:   16,
		EnableCache: false,
	}
}

// NewConfig 创建一个新的配置并应用选项
func NewConfig(opts ...Option) *Config {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Factory 嵌入客户端工厂函数类型
type Factory func(opts ...Option) (Client, error)

// 全局注册的嵌入客户端工厂函数
var clientFactories = make(map[string]Factory)

// RegisterClient 注册嵌入客户端工厂函数
func RegisterClient(name string, factory Factory) {
	clientFactories[name] = factory
}

// NewClient 根据名称创建嵌入客户端
func NewClient(name string, opts ...Option) (Client, error) {
	factory, exists := clientFactories[name]
	if !exists {
		return nil, NewEmbeddingError(
			ErrCodeInvalidRequest,
			"embedding client type not registered: "+name)
	}
	return factory(opts...)
}
