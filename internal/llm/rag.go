package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/pyprovider"
)

// DefaultRAGTemplate 默认RAG提示词模板
// 包含变量：
// {{.Question}} - 用户问题
// {{.Context}} - 检索的上下文
const DefaultRAGTemplate = `请你作为一个智能问答助手，基于下面提供的参考上下文回答问题。
即使参考上下文中的信息不完整，也请尽量根据提供的线索回答问题。如果参考上下文中完全没有相关信息，请直接说"抱歉，我没有找到相关信息"，不要猜测或编造信息。

请特别注意：
1. 即使参考上下文中只有部分相关信息，仍然需要尝试回答问题
2. 如果参考上下文中有多个片段包含相关信息，请整合这些信息提供完整回答
3. 回答应当简洁、准确、全面

参考上下文:
{{.Context}}

用户问题: {{.Question}}

请直接回答问题，不要重复问题内容，不要说"根据参考上下文"之类的话语。`

// EmptyContextTemplate 无上下文时的提示词模板
const EmptyContextTemplate = `请回答以下问题。如果你不确定答案，请诚实地说你不知道，而不是猜测。

用户问题: {{.Question}}

回答:`

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
	// 空上下文提示词模板
	EmptyTemplate string
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
		EmptyTemplate:  EmptyContextTemplate,
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

// WithEmptyContextTemplate 设置空上下文提示词模板
func WithEmptyContextTemplate(template string) RAGOption {
	return func(c *RAGConfig) {
		c.EmptyTemplate = template
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

	// 构建提示词，区分有上下文和无上下文情况
	var prompt string
	if len(contexts) == 0 {
		prompt = r.buildEmptyPrompt(question)
	} else {
		prompt = r.buildPrompt(question, contexts)
	}

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

// buildEmptyPrompt 构建无上下文提示词
func (r *RAGService) buildEmptyPrompt(question string) string {
	r.mu.RLock()
	template := r.config.EmptyTemplate
	r.mu.RUnlock()

	// 简单的模板替换
	prompt := template
	prompt = strings.ReplaceAll(prompt, "{{.Question}}", question)

	return prompt
}

// SetTemplate 设置自定义提示词模板
func (r *RAGService) SetTemplate(template string) *RAGService {
	r.mu.Lock()
	r.config.Template = template
	r.mu.Unlock()
	return r
}

// SetEmptyTemplate 设置自定义空上下文提示词模板
func (r *RAGService) SetEmptyTemplate(template string) *RAGService {
	r.mu.Lock()
	r.config.EmptyTemplate = template
	r.mu.Unlock()
	return r
}

// PythonRAGService 实现通过Python API的RAG服务
type PythonRAGService struct {
	llmClient *pyprovider.LLMClient
	config    *RAGConfig
	mu        sync.RWMutex // 配置互斥锁
}

// NewPythonRAG 创建新的Python RAG服务
func NewPythonRAG(client *pyprovider.LLMClient, opts ...RAGOption) *PythonRAGService {
	cfg := DefaultRAGConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &PythonRAGService{
		llmClient: client,
		config:    cfg,
	}
}

// Answer 通过Python API生成基于上下文的回答
func (r *PythonRAGService) Answer(ctx context.Context, question string, contexts []string) (*RAGResponse, error) {
	if question == "" {
		return nil, NewLLMError(ErrCodeEmptyPrompt, "question cannot be empty")
	}

	r.mu.RLock()
	cfg := r.config
	r.mu.RUnlock()

	// 创建带超时的上下文
	ctxWithTimeout, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// 准备RAG选项
	var options []pyprovider.RAGOption

	// 设置基本选项
	options = append(options,
		pyprovider.WithRAGMaxTokens(cfg.MaxTokens),
		pyprovider.WithRAGTemperature(float64(cfg.Temperature)),
		pyprovider.WithTopK(5), // 默认值
	)

	// 启用引用（如果配置中要求）
	if cfg.IncludeSources {
		options = append(options, pyprovider.WithEnableCitation(true))
	}

	// 调用Python API
	response, err := r.llmClient.Answer(ctxWithTimeout, question, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response via Python API: %w", err)
	}

	// 转换源引用
	sources := make([]SourceReference, len(response.Sources))
	for i, src := range response.Sources {
		sources[i] = SourceReference{
			ID:       fmt.Sprintf("src-%d", i+1),
			Content:  src.Text,
			Metadata: src.Metadata,
		}

		// 如果有文档ID，添加到源引用中
		if src.DocumentID != "" {
			sources[i].ID = src.DocumentID
		}
	}

	// 构建RAG响应
	ragResponse := &RAGResponse{
		Answer:  response.Text,
		Sources: sources,
	}

	return ragResponse, nil
}

// 下面添加创建Python RAG服务的便捷函数

// NewRAGWithPython 创建使用Python API的RAG服务
func NewRAGWithPython(pyConfig *pyprovider.PyServiceConfig, opts ...RAGOption) (*PythonRAGService, error) {
	// 创建HTTP客户端
	httpClient, err := pyprovider.NewClient(pyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Python client: %w", err)
	}

	// 创建LLM客户端
	llmClient := pyprovider.NewLLMClient(httpClient)

	// 创建RAG服务
	return NewPythonRAG(llmClient, opts...), nil
}
