package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/pyprovider"
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
	MaxTokens   *int      // 最大生成Token数
	Temperature *float32  // 采样温度
	TopP        *float32  // 核采样概率阈值
	TopK        *int      // 生成候选集大小
	Stream      bool      // 是否流式输出
	Stop        *[]string // 停止序列
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

// WithGenerateStop 设置生成请求的停止序列
func WithGenerateStop(stop []string) GenerateOption {
	return func(o *GenerateOptions) {
		o.Stop = &stop
	}
}

// ChatOption 聊天请求的选项
type ChatOption func(*ChatOptions)

// ChatOptions 聊天请求的选项集合
type ChatOptions struct {
    MaxTokens   *int      // 最大生成Token数
    Temperature *float32  // 采样温度
    TopP        *float32  // 核采样概率阈值
    TopK        *int      // 生成候选集大小
    Stream      bool      // 是否流式输出
    Stop        *[]string // 停止序列
}

// WithChatStop 设置聊天请求的停止序列
func WithChatStop(stop []string) ChatOption {
    return func(o *ChatOptions) {
        o.Stop = &stop
    }
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

// PythonClient 通过Python服务提供的API实现LLM功能
type PythonClient struct {
	llmClient   *pyprovider.LLMClient
	modelName   string
	maxTokens   int
	temperature float32
	topP        float32
}

// NewPythonClient 创建新的Python LLM客户端
func NewPythonClient(opts ...Option) (Client, error) {
	// 创建配置
	cfg := NewConfig(opts...)

	// 创建基础HTTP客户端
	pyConfig := &pyprovider.PyServiceConfig{}

	// 使用配置的BaseURL，如果为空则使用默认值
	if cfg.BaseURL != "" {
		pyConfig.WithBaseURL(cfg.BaseURL)
	}

	// 设置超时
	pyConfig.WithTimeout(cfg.Timeout)

	// 创建HTTP客户端
	httpClient, err := pyprovider.NewClient(pyConfig)
	if err != nil {
		return nil, NewLLMError(ErrCodeServerError, fmt.Sprintf("failed to create Python client: %v", err))
	}

	// 创建LLM客户端
	llmClient := pyprovider.NewLLMClient(httpClient)

	client := &PythonClient{
		llmClient:   llmClient,
		modelName:   cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		topP:        cfg.TopP,
	}

	return client, nil
}

// Name 返回模型名称
func (c *PythonClient) Name() string {
	return c.modelName
}

// Generate 根据提示词生成回答
func (c *PythonClient) Generate(ctx context.Context, prompt string, options ...GenerateOption) (*Response, error) {
	if prompt == "" {
		return nil, NewLLMError(ErrCodeEmptyPrompt, ErrMsgEmptyPrompt)
	}

	// 应用选项
	opts := &GenerateOptions{}
	for _, opt := range options {
		opt(opts)
	}

	// 准备Python API的选项
	var pyOptions []pyprovider.GenerateOption

	// 设置模型
	pyOptions = append(pyOptions, pyprovider.WithModel(c.modelName))

	// 转换选项
	if opts.MaxTokens != nil {
		pyOptions = append(pyOptions, pyprovider.WithMaxTokens(*opts.MaxTokens))
	} else if c.maxTokens > 0 {
		pyOptions = append(pyOptions, pyprovider.WithMaxTokens(c.maxTokens))
	}

	if opts.Temperature != nil {
		pyOptions = append(pyOptions, pyprovider.WithTemperature(float64(*opts.Temperature)))
	} else if c.temperature > 0 {
		pyOptions = append(pyOptions, pyprovider.WithTemperature(float64(c.temperature)))
	}

	if opts.Stop != nil {
		pyOptions = append(pyOptions, pyprovider.WithStop(*opts.Stop))
	}

	// 调用Python API
	textResponse, err := c.llmClient.Generate(ctx, prompt, pyOptions...)
	if err != nil {
		return nil, WrapError(err, ErrCodeServerError)
	}

	// 转换响应格式
	return &Response{
		Text:       textResponse.Text,
		TokenCount: textResponse.TotalTokens,
		ModelName:  textResponse.Model,
		FinishTime: time.Now(),
	}, nil
}

// Chat 进行多轮对话
func (c *PythonClient) Chat(ctx context.Context, messages []Message, options ...ChatOption) (*Response, error) {
	if len(messages) == 0 {
		return nil, NewLLMError(ErrCodeInvalidRequest, "messages cannot be empty")
	}

	// 应用选项
	opts := &ChatOptions{}
	for _, opt := range options {
		opt(opts)
	}

	// 转换消息格式
	pyMessages := make([]pyprovider.Message, len(messages))
	for i, msg := range messages {
		pyMessages[i] = pyprovider.Message{
			Role:    string(msg.Role),
			Content: msg.Content,
			Name:    msg.Name,
		}
	}

	// 准备Python API的选项
	var pyOptions []pyprovider.ChatOption

	// 设置模型
	pyOptions = append(pyOptions, pyprovider.WithChatModel(c.modelName))

	// 转换选项
	if opts.MaxTokens != nil {
		pyOptions = append(pyOptions, pyprovider.WithChatMaxTokens(*opts.MaxTokens))
	} else if c.maxTokens > 0 {
		pyOptions = append(pyOptions, pyprovider.WithChatMaxTokens(c.maxTokens))
	}

	if opts.Temperature != nil {
		pyOptions = append(pyOptions, pyprovider.WithChatTemperature(float64(*opts.Temperature)))
	} else if c.temperature > 0 {
		pyOptions = append(pyOptions, pyprovider.WithChatTemperature(float64(c.temperature)))
	}

	if opts.Stop != nil {
		pyOptions = append(pyOptions, pyprovider.WithChatStop(*opts.Stop))
	}

	// 调用Python API
	textResponse, err := c.llmClient.Chat(ctx, pyMessages, pyOptions...)
	if err != nil {
		return nil, WrapError(err, ErrCodeServerError)
	}

	// 转换响应中的消息（如果有）
	responseMessage := Message{
		Role:    RoleAssistant,
		Content: textResponse.Text,
	}

	// 转换响应格式
	return &Response{
		Text:       textResponse.Text,
		Messages:   []Message{responseMessage},
		TokenCount: textResponse.TotalTokens,
		ModelName:  textResponse.Model,
		FinishTime: time.Now(),
	}, nil
}

// 在包初始化时注册Python客户端
func init() {
	RegisterClient("python", NewPythonClient)
}
