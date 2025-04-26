package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// 通义千问API端点
	defaultTongyiEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation"
)

// TongyiClient 通义千问大模型客户端实现
type TongyiClient struct {
	apiKey      string       // API密钥
	baseURL     string       // API端点
	model       string       // 模型名称
	httpClient  *http.Client // HTTP客户端
	maxRetries  int          // 最大重试次数
	maxTokens   int          // 最大生成Token数
	temperature float32      // 温度参数
	topP        float32      // topP参数
}

// NewTongyiClient 创建新的通义千问大模型客户端
func NewTongyiClient(opts ...Option) (Client, error) {
	// 创建配置
	cfg := NewConfig(opts...)

	// 验证API密钥
	if cfg.APIKey == "" {
		return nil, NewLLMError(ErrCodeInvalidAPIKey, ErrMsgInvalidAPIKey)
	}

	// 确定API端点
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultTongyiEndpoint
	}

	// 创建HTTP客户端，设置超时
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	client := &TongyiClient{
		apiKey:      cfg.APIKey,
		baseURL:     baseURL,
		model:       cfg.Model,
		httpClient:  httpClient,
		maxRetries:  cfg.MaxRetries,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		topP:        cfg.TopP,
	}

	return client, nil
}

// Name 返回模型名称
func (c *TongyiClient) Name() string {
	return c.model
}

// Generate 根据提示词生成回答
func (c *TongyiClient) Generate(ctx context.Context, prompt string, options ...GenerateOption) (*Response, error) {
	if prompt == "" {
		return nil, NewLLMError(ErrCodeEmptyPrompt, ErrMsgEmptyPrompt)
	}

	// 将单个提示转换为消息格式进行调用
	messages := []Message{
		{
			Role:    RoleUser,
			Content: prompt,
		},
	}

	// 转换GenerateOptions为ChatOptions
	var chatOpts []ChatOption
	opts := &GenerateOptions{}
	for _, opt := range options {
		opt(opts)
	}

	if opts.MaxTokens != nil {
		chatOpts = append(chatOpts, WithChatMaxTokens(*opts.MaxTokens))
	}
	if opts.Temperature != nil {
		chatOpts = append(chatOpts, WithChatTemperature(*opts.Temperature))
	}
	if opts.TopP != nil {
		chatOpts = append(chatOpts, WithChatTopP(*opts.TopP))
	}
	if opts.TopK != nil {
		chatOpts = append(chatOpts, WithChatTopK(*opts.TopK))
	}
	if opts.Stream {
		chatOpts = append(chatOpts, WithChatStream(opts.Stream))
	}

	// 复用Chat方法
	return c.Chat(ctx, messages, chatOpts...)
}

// Chat 进行多轮对话
func (c *TongyiClient) Chat(ctx context.Context, messages []Message, options ...ChatOption) (*Response, error) {
	if len(messages) == 0 {
		return nil, NewLLMError(ErrCodeInvalidRequest, "messages cannot be empty")
	}

	// 应用选项
	opts := &ChatOptions{}
	for _, opt := range options {
		opt(opts)
	}

	// 准备请求参数
	params := &TongyiParameters{
		ResultFormat: "message", // 使用结构化返回格式
	}

	// 如果提供了选项，则使用
	if opts.MaxTokens != nil {
		params.MaxTokens = opts.MaxTokens
	} else if c.maxTokens > 0 {
		maxTokens := c.maxTokens
		params.MaxTokens = &maxTokens
	}

	if opts.Temperature != nil {
		params.Temperature = opts.Temperature
	} else if c.temperature > 0 {
		temp := c.temperature
		params.Temperature = &temp
	}

	if opts.TopP != nil {
		params.TopP = opts.TopP
	} else if c.topP > 0 {
		topP := c.topP
		params.TopP = &topP
	}

	if opts.TopK != nil {
		params.TopK = opts.TopK
	}

	// 创建请求
	req := &TongyiRequest{
		Model: c.model,
		Input: &TongyiRequestInput{
			Messages: messages,
		},
		Parameters: params,
	}

	// 发送请求
	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// 解析响应
	return c.processResponse(resp)
}

// sendRequest 发送API请求并解析响应
func (c *TongyiClient) sendRequest(ctx context.Context, req *TongyiRequest) (*TongyiResponse, error) {
	// 将请求数据转换为JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, NewLLMError(ErrCodeInvalidRequest, fmt.Sprintf("failed to marshal request: %v", err))
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL,
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, NewLLMError(ErrCodeInvalidRequest, fmt.Sprintf("failed to create request: %v", err))
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	httpReq.Header.Set("Accept", "application/json")

	// 使用重试机制发送请求
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避重试
			select {
			case <-ctx.Done():
				return nil, NewLLMError(ErrCodeTimeout, ctx.Err().Error())
			case <-time.After(time.Duration(1<<attempt) * 100 * time.Millisecond):
				// 等待后继续
			}
		}

		resp, err = c.httpClient.Do(httpReq)
		if err == nil && resp.StatusCode < 500 {
			// 成功或客户端错误，不需要重试
			break
		}

		if err != nil {
			lastErr = err
		} else {
			resp.Body.Close() // 关闭响应体，避免资源泄露
		}
	}

	if err != nil {
		return nil, NewLLMError(ErrCodeNetworkError, fmt.Sprintf("request failed: %v", lastErr))
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewLLMError(ErrCodeServerError, fmt.Sprintf("failed to read response: %v", err))
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		// 尝试解析错误响应
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Message != "" {
			return nil, NewLLMError(ErrCodeServerError,
				fmt.Sprintf("API error: %s (%s)", errResp.Message, errResp.Code))
		}

		// 如果无法解析，返回原始错误
		return nil, NewLLMError(ErrCodeServerError,
			fmt.Sprintf("API error (status %d): %s", resp.StatusCode, string(body)))
	}

	// 解析JSON响应
	var tongyiResp TongyiResponse
	if err := json.Unmarshal(body, &tongyiResp); err != nil {
		return nil, NewLLMError(ErrCodeServerError,
			fmt.Sprintf("failed to parse response: %v", err))
	}

	// 检查API返回的错误
	if tongyiResp.Code != "" {
		return nil, NewLLMError(ErrCodeServerError,
			fmt.Sprintf("API error: %s (%s)", tongyiResp.Message, tongyiResp.Code))
	}

	return &tongyiResp, nil
}

// processResponse 处理通义千问的响应
func (c *TongyiClient) processResponse(resp *TongyiResponse) (*Response, error) {
	result := &Response{
		ModelName:  c.model,
		TokenCount: resp.Usage.TotalTokens,
		FinishTime: time.Now(),
	}

	// 处理输出
	if resp.Output.Text != nil {
		// 处理文本格式输出
		result.Text = *resp.Output.Text
	} else if len(resp.Output.Choices) > 0 {
		// 处理消息格式输出
		choice := resp.Output.Choices[0]
		result.Text = choice.Message.Content
		result.Messages = append(result.Messages, choice.Message)
	} else {
		return nil, NewLLMError(ErrCodeServerError, "empty response from API")
	}

	return result, nil
}

// 在包初始化时注册通义千问客户端
func init() {
	RegisterClient("tongyi", NewTongyiClient)
}
