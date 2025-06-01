package pyprovider

import (
	"context"
	"fmt"
)

// 消息角色常量
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleFunction  = "function"
)

// Message 表示聊天消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// GenerateRequest 表示生成文本的请求
type GenerateRequest struct {
	Prompt      string   `json:"prompt"`
	Model       string   `json:"model,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// ChatRequest 表示聊天请求
type ChatRequest struct {
	Messages    []Message `json:"messages"`
	Model       string    `json:"model,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
}

// RAGRequest 表示RAG增强生成请求
type RAGRequest struct {
	Query           string   `json:"query"`
	DocumentIDs     []string `json:"document_ids,omitempty"`
	CollectionName  string   `json:"collection_name,omitempty"`
	TopK            int      `json:"top_k,omitempty"`
	Model           string   `json:"model,omitempty"`
	Temperature     float64  `json:"temperature,omitempty"`
	MaxTokens       int      `json:"max_tokens,omitempty"`
	Stream          bool     `json:"stream,omitempty"`
	EnableCitation  bool     `json:"enable_citation,omitempty"`
	EnableReasoning bool     `json:"enable_reasoning,omitempty"`
	Contexts        []string `json:"contexts,omitempty"` // Add this field
}

// TokenUsage 表示token使用情况
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// TextResponse 表示生成文本的响应
type TextResponse struct {
	Text             string  `json:"text"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	FinishReason     string  `json:"finish_reason,omitempty"`
	ProcessingTime   float64 `json:"processing_time"`
}

// SourceInfo 表示RAG响应中的来源信息
type SourceInfo struct {
	Text       string         `json:"text"`
	Score      float64        `json:"score"`
	DocumentID string         `json:"document_id"`
	Metadata   map[string]any `json:"metadata"`
}

// RAGResponse 表示RAG增强生成的响应
type RAGResponse struct {
	Text             string       `json:"text"`
	Sources          []SourceInfo `json:"sources"`
	Model            string       `json:"model"`
	PromptTokens     int          `json:"prompt_tokens"`
	CompletionTokens int          `json:"completion_tokens"`
	TotalTokens      int          `json:"total_tokens"`
	FinishReason     string       `json:"finish_reason,omitempty"`
	ProcessingTime   float64      `json:"processing_time"`
}

// LLMClient 是LLM服务的客户端
type LLMClient struct {
	client Client
}

// GenerateOption 是Generate方法的选项函数
type GenerateOption func(*GenerateRequest)

// ChatOption 是Chat方法的选项函数
type ChatOption func(*ChatRequest)

// RAGOption 是Answer方法的选项函数
type RAGOption func(*RAGRequest)

// NewLLMClient 创建一个新的LLM客户端
func NewLLMClient(client Client) *LLMClient {
	return &LLMClient{
		client: client,
	}
}

// WithModel 设置模型名称
func WithModel(model string) GenerateOption {
	return func(req *GenerateRequest) {
		req.Model = model
	}
}

// WithTemperature 设置温度参数
func WithTemperature(temperature float64) GenerateOption {
	return func(req *GenerateRequest) {
		req.Temperature = temperature
	}
}

// WithMaxTokens 设置最大token数
func WithMaxTokens(maxTokens int) GenerateOption {
	return func(req *GenerateRequest) {
		req.MaxTokens = maxTokens
	}
}

// WithStop 设置停止序列
func WithStop(stop []string) GenerateOption {
	return func(req *GenerateRequest) {
		req.Stop = stop
	}
}

// WithChatModel 设置聊天模型名称
func WithChatModel(model string) ChatOption {
	return func(req *ChatRequest) {
		req.Model = model
	}
}

// WithChatTemperature 设置聊天温度参数
func WithChatTemperature(temperature float64) ChatOption {
	return func(req *ChatRequest) {
		req.Temperature = temperature
	}
}

// WithChatMaxTokens 设置聊天最大token数
func WithChatMaxTokens(maxTokens int) ChatOption {
	return func(req *ChatRequest) {
		req.MaxTokens = maxTokens
	}
}

// WithChatStop 设置聊天停止序列
func WithChatStop(stop []string) ChatOption {
	return func(req *ChatRequest) {
		req.Stop = stop
	}
}

// WithDocumentIDs 设置RAG文档ID列表
func WithDocumentIDs(documentIDs []string) RAGOption {
	return func(req *RAGRequest) {
		req.DocumentIDs = documentIDs
	}
}

// WithCollectionName 设置RAG集合名称
func WithCollectionName(collectionName string) RAGOption {
	return func(req *RAGRequest) {
		req.CollectionName = collectionName
	}
}

// WithRAGModel 设置RAG模型名称
func WithRAGModel(model string) RAGOption {
	return func(req *RAGRequest) {
		req.Model = model
	}
}

// WithTopK 设置RAG返回的结果数量
func WithTopK(topK int) RAGOption {
	return func(req *RAGRequest) {
		req.TopK = topK
	}
}

// WithRAGTemperature 设置RAG温度参数
func WithRAGTemperature(temperature float64) RAGOption {
	return func(req *RAGRequest) {
		req.Temperature = temperature
	}
}

// WithRAGMaxTokens 设置RAG最大token数
func WithRAGMaxTokens(maxTokens int) RAGOption {
	return func(req *RAGRequest) {
		req.MaxTokens = maxTokens
	}
}

// WithEnableCitation 设置是否启用引用
func WithEnableCitation(enable bool) RAGOption {
	return func(req *RAGRequest) {
		req.EnableCitation = enable
	}
}

// WithEnableReasoning 设置是否启用推理
func WithEnableReasoning(enable bool) RAGOption {
	return func(req *RAGRequest) {
		req.EnableReasoning = enable
	}
}

// Add a new RAG option function to pass contexts directly
// WithContexts sets the contexts for RAG
func WithContexts(contexts []string) RAGOption {
	return func(req *RAGRequest) {
		req.Contexts = contexts
	}
}

// Generate 生成文本回复
func (c *LLMClient) Generate(ctx context.Context, prompt string, options ...GenerateOption) (*TextResponse, error) {
	// 创建默认请求
	req := GenerateRequest{
		Prompt:      prompt,
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	// 应用选项
	for _, option := range options {
		option(&req)
	}

	// 构建请求路径
	reqPath := "/python/llm/generate"

	// 发送POST请求
	var response TextResponse
	if err := c.client.Post(ctx, reqPath, req, &response); err != nil {
		return nil, fmt.Errorf("failed to generate text: %w", err)
	}

	return &response, nil
}

// Chat 基于消息历史生成回复
func (c *LLMClient) Chat(ctx context.Context, messages []Message, options ...ChatOption) (*TextResponse, error) {
	// 验证消息
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages cannot be empty")
	}

	// 创建默认请求
	req := ChatRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	// 应用选项
	for _, option := range options {
		option(&req)
	}

	// 构建请求路径
	reqPath := "/python/llm/chat"

	// 发送POST请求
	var response TextResponse
	if err := c.client.Post(ctx, reqPath, req, &response); err != nil {
		return nil, fmt.Errorf("failed to generate chat response: %w", err)
	}

	return &response, nil
}

// Answer 使用RAG生成回答
func (c *LLMClient) Answer(ctx context.Context, query string, options ...RAGOption) (*RAGResponse, error) {
	// 创建默认请求
	req := RAGRequest{
		Query:       query,
		TopK:        5,
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	// 应用选项
	for _, option := range options {
		option(&req)
	}

	// 构建请求路径
	reqPath := "/python/llm/rag"

	// 发送POST请求
	var response RAGResponse
	if err := c.client.Post(ctx, reqPath, req, &response); err != nil {
		return nil, fmt.Errorf("failed to generate RAG answer: %w", err)
	}

	return &response, nil
}

// NewUserMessage 创建用户消息
func NewUserMessage(content string) Message {
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

// NewSystemMessage 创建系统消息
func NewSystemMessage(content string) Message {
	return Message{
		Role:    RoleSystem,
		Content: content,
	}
}

// NewAssistantMessage 创建助手消息
func NewAssistantMessage(content string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: content,
	}
}

// IsValidRole 检查是否是有效的角色
func IsValidRole(role string) bool {
	return role == RoleSystem || role == RoleUser || role == RoleAssistant || role == RoleFunction
}
