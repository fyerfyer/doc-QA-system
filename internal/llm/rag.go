package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultRAGTemplate 默认RAG提示词模板
// 包含变量：
// {{.Question}} - 用户问题
// {{.Context}} - 检索的上下文
const DefaultRAGTemplate = `请你作为一个智能问答助手，基于下面提供的参考上下文回答问题。
如果参考上下文中没有足够信息回答问题，请直接说"抱歉，我没有找到相关信息"，不要猜测或编造信息。

参考上下文:
{{.Context}}

用户问题: {{.Question}}

请直接回答问题，不要重复问题内容，不要说参考上下文之类的话语。`

// DeepThinkingRAGTemplate 带有深度思考的RAG提示词模板
const DeepThinkingRAGTemplate = `请你作为一个智能问答助手，基于下面提供的参考上下文回答问题。
首先，分析一下问题的关键点。
然后，从参考上下文中找出相关信息。
最后，组织合理的语言回答问题。

如果参考上下文中没有足够信息回答问题，请直接说"抱歉，我没有找到相关信息"，不要猜测或编造信息。

参考上下文:
{{.Context}}

用户问题: {{.Question}}

思考过程（用户看不到这部分）:
1. 分析问题的关键点
2. 从上下文中找出相关的信息
3. 确定是否有足够信息回答问题
4. 组织回答

回答：`

// formatContext 格式化上下文内容
func formatContext(contexts []string) string {
	var formattedContext strings.Builder
	for i, ctx := range contexts {
		formattedContext.WriteString(fmt.Sprintf("【%d】%s\n\n", i+1, ctx))
	}
	return formattedContext.String()
}

// RAGConfig 检索增强生成配置
type RAGConfig struct {
	// 提示词模板
	Template string
	// 最大Token数
	MaxTokens int
	// 温度参数
	Temperature float32
	// 超时时间
	Timeout time.Duration
	// 是否带上引用来源
	IncludeSources bool
}

// DefaultRAGConfig 默认RAG配置
func DefaultRAGConfig() *RAGConfig {
	return &RAGConfig{
		Template:       DefaultRAGTemplate,
		MaxTokens:      2048,
		Temperature:    0.7,
		Timeout:        30 * time.Second,
		IncludeSources: true,
	}
}

// RAGService 实现检索增强生成服务
type RAGService struct {
	Client Client       // 大模型客户端
	config *RAGConfig   // 配置
	mu     sync.RWMutex // 配置互斥锁
}

// NewRAG 创建新的检索增强生成服务
func NewRAG(client Client, opts ...RAGOption) *RAGService {
	cfg := DefaultRAGConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &RAGService{
		Client: client,
		config: cfg,
	}
}

// RAGOption RAG配置选项函数类型
type RAGOption func(*RAGConfig)

// WithTemplate 设置提示词模板
func WithTemplate(template string) RAGOption {
	return func(c *RAGConfig) {
		c.Template = template
	}
}

// WithDeepThinking 启用深度思考模式
func WithDeepThinking() RAGOption {
	return func(c *RAGConfig) {
		c.Template = DeepThinkingRAGTemplate
	}
}

// WithRAGMaxTokens 设置最大Token数
func WithRAGMaxTokens(tokens int) RAGOption {
	return func(c *RAGConfig) {
		c.MaxTokens = tokens
	}
}

// WithRAGTemperature 设置温度参数
func WithRAGTemperature(temp float32) RAGOption {
	return func(c *RAGConfig) {
		c.Temperature = temp
	}
}

// WithRAGTimeout 设置请求超时时间
func WithRAGTimeout(timeout time.Duration) RAGOption {
	return func(c *RAGConfig) {
		c.Timeout = timeout
	}
}

// WithSources 设置是否包含引用来源
func WithSources(include bool) RAGOption {
	return func(c *RAGConfig) {
		c.IncludeSources = include
	}
}

// Answer 根据上下文和问题生成回答
func (r *RAGService) Answer(ctx context.Context, question string, contexts []string) (*RAGResponse, error) {
	if question == "" {
		return nil, NewLLMError(ErrCodeEmptyPrompt, "question cannot be empty")
	}

	r.mu.RLock()
	cfg := r.config
	r.mu.RUnlock()

	// 创建带超时的上下文
	ctxWithTimeout, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// 构建提示词
	prompt := r.buildPrompt(question, contexts)

	// 调用大模型生成回答
	response, err := r.Client.Generate(
		ctxWithTimeout,
		prompt,
		WithGenerateMaxTokens(cfg.MaxTokens),
		WithGenerateTemperature(cfg.Temperature),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %v", err)
	}

	// 构建RAG响应
	ragResponse := &RAGResponse{
		Answer: response.Text,
	}

	// 如果需要包含引用来源，添加到响应中
	if cfg.IncludeSources && len(contexts) > 0 {
		sources := make([]SourceReference, len(contexts))
		for i, ctx := range contexts {
			sources[i] = SourceReference{
				ID:      fmt.Sprintf("src-%d", i+1),
				Content: ctx,
				// 注意：文件ID和文件名需要调用方传入，这里没有这些信息
			}
		}
		ragResponse.Sources = sources
	}

	return ragResponse, nil
}

// buildPrompt 构建增强提示词
func (r *RAGService) buildPrompt(question string, contexts []string) string {
	r.mu.RLock()
	template := r.config.Template
	r.mu.RUnlock()

	// 格式化上下文
	formattedContext := formatContext(contexts)

	// 简单的模板替换
	prompt := template
	prompt = strings.ReplaceAll(prompt, "{{.Question}}", question)
	prompt = strings.ReplaceAll(prompt, "{{.Context}}", formattedContext)

	return prompt
}

// SetTemplate 设置自定义提示词模板
func (r *RAGService) SetTemplate(template string) *RAGService {
	r.mu.Lock()
	r.config.Template = template
	r.mu.Unlock()
	return r
}
