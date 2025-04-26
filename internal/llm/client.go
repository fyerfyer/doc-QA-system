package llm

import (
	"context"
	"time"
)

// Client 大模型客户端接口
// 负责处理与大语言模型的交互
type Client interface {
	// Generate 根据提示词生成回答
	Generate(ctx context.Context, prompt string, options ...GenerateOption) (*Response, error)

	// Chat 进行多轮对话
	Chat(ctx context.Context, messages []Message, options ...ChatOption) (*Response, error)

	// Name 返回模型名称
	Name() string
}

// Config 大模型客户端配置
type Config struct {
	APIKey      string        // API密钥
	BaseURL     string        // API基础URL
	Model       string        // 模型名称
	Timeout     time.Duration // 请求超时时间
	MaxRetries  int           // 最大重试次数
	MaxTokens   int           // 最大生成Token数
	Temperature float32       // 采样温度(0.0-2.0)
	TopP        float32       // 核采样概率阈值(0.0-1.0)
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		BaseURL:     "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation",
		Model:       ModelQwenTurbo, // 默认使用通义千问-Turbo模型
		Timeout:     60 * time.Second,
		MaxRetries:  3,
		MaxTokens:   1024,
		Temperature: 0.7,
		TopP:        0.9,
	}
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

// WithMaxTokens 设置最大生成Token数
func WithMaxTokens(tokens int) Option {
	return func(c *Config) {
		c.MaxTokens = tokens
	}
}

// WithTemperature 设置采样温度
func WithTemperature(temp float32) Option {
	return func(c *Config) {
		c.Temperature = temp
	}
}

// WithTopP 设置核采样概率阈值
func WithTopP(topP float32) Option {
	return func(c *Config) {
		c.TopP = topP
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

// GenerateOption 生成请求的选项
type GenerateOption func(*GenerateOptions)

// GenerateOptions 生成请求的选项集合
type GenerateOptions struct {
	MaxTokens   *int     // 最大生成Token数
	Temperature *float32 // 采样温度
	TopP        *float32 // 核采样概率阈值
	TopK        *int     // 生成候选集大小
	Stream      bool     // 是否流式输出
}

// WithGenerateMaxTokens 设置生成请求的最大Token数
func WithGenerateMaxTokens(tokens int) GenerateOption {
	return func(o *GenerateOptions) {
		o.MaxTokens = &tokens
	}
}

// WithGenerateTemperature 设置生成请求的采样温度
func WithGenerateTemperature(temp float32) GenerateOption {
	return func(o *GenerateOptions) {
		o.Temperature = &temp
	}
}

// WithGenerateTopP 设置生成请求的核采样概率阈值
func WithGenerateTopP(topP float32) GenerateOption {
	return func(o *GenerateOptions) {
		o.TopP = &topP
	}
}

// WithGenerateTopK 设置生成请求的候选集大小
func WithGenerateTopK(topK int) GenerateOption {
	return func(o *GenerateOptions) {
		o.TopK = &topK
	}
}

// WithGenerateStream 设置是否流式输出
func WithGenerateStream(stream bool) GenerateOption {
	return func(o *GenerateOptions) {
		o.Stream = stream
	}
}

// ChatOption 聊天请求的选项
type ChatOption func(*ChatOptions)

// ChatOptions 聊天请求的选项集合
type ChatOptions struct {
	MaxTokens   *int     // 最大生成Token数
	Temperature *float32 // 采样温度
	TopP        *float32 // 核采样概率阈值
	TopK        *int     // 生成候选集大小
	Stream      bool     // 是否流式输出
}

// WithChatMaxTokens 设置聊天请求的最大Token数
func WithChatMaxTokens(tokens int) ChatOption {
	return func(o *ChatOptions) {
		o.MaxTokens = &tokens
	}
}

// WithChatTemperature 设置聊天请求的采样温度
func WithChatTemperature(temp float32) ChatOption {
	return func(o *ChatOptions) {
		o.Temperature = &temp
	}
}

// WithChatTopP 设置聊天请求的核采样概率阈值
func WithChatTopP(topP float32) ChatOption {
	return func(o *ChatOptions) {
		o.TopP = &topP
	}
}

// WithChatTopK 设置聊天请求的候选集大小
func WithChatTopK(topK int) ChatOption {
	return func(o *ChatOptions) {
		o.TopK = &topK
	}
}

// WithChatStream 设置是否流式输出
func WithChatStream(stream bool) ChatOption {
	return func(o *ChatOptions) {
		o.Stream = stream
	}
}

// Factory 大模型客户端工厂函数类型
type Factory func(opts ...Option) (Client, error)

// 全局注册的大模型客户端工厂函数
var clientFactories = make(map[string]Factory)

// RegisterClient 注册大模型客户端工厂函数
func RegisterClient(name string, factory Factory) {
	clientFactories[name] = factory
}

// NewClient 根据名称创建大模型客户端
func NewClient(name string, opts ...Option) (Client, error) {
	factory, exists := clientFactories[name]
	if !exists {
		return nil, NewLLMError(
			ErrCodeInvalidRequest,
			"llm client type not registered: "+name)
	}
	return factory(opts...)
}
