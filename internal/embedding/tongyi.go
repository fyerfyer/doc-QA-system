package embedding

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
	// 默认API端点
	defaultDashScopeEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"
	defaultOpenAIEndpoint    = "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings"

	// 默认模型
	defaultModel = "text-embedding-v1"
)

// DashScopeRequest 定义DashScope请求结构体
type DashScopeRequest struct {
	Model      string                `json:"model"`
	Input      DashScopeRequestInput `json:"input"`
	Parameters *DashScopeParameters  `json:"parameters,omitempty"`
}

type DashScopeRequestInput struct {
	Texts []string `json:"texts"`
}

type DashScopeParameters struct {
	Dimension  int    `json:"dimension,omitempty"`
	OutputType string `json:"output_type,omitempty"`
}

// TongyiClient 实现通义千问嵌入API客户端
type TongyiClient struct {
	apiKey       string       // API密钥
	endpoint     string       // API端点
	model        string       // 模型名称
	httpClient   *http.Client // HTTP客户端
	maxRetries   int          // 最大重试次数
	dimensions   int          // 向量维度
	useOpenAIAPI bool         // 是否使用OpenAI兼容接口
}

// NewTongyiClient 创建新的通义千问嵌入客户端
func NewTongyiClient(opts ...Option) (Client, error) {
	// 创建配置
	cfg := NewConfig(opts...)

	// 验证API密钥
	if cfg.APIKey == "" {
		return nil, NewEmbeddingError(ErrCodeInvalidAPIKey, ErrMsgInvalidAPIKey)
	}

	// 确定API端点
	endpoint := cfg.BaseURL
	useOpenAIAPI := false
	if endpoint == "" {
		// 默认使用DashScope API
		endpoint = defaultDashScopeEndpoint
	} else if endpoint == "openai" || endpoint == "compatible" {
		// 使用OpenAI兼容API
		endpoint = defaultOpenAIEndpoint
		useOpenAIAPI = true
	}

	// 确定模型名称
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	// 创建HTTP客户端，设置超时
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	// 确定维度设置
	dimensions := cfg.Dimensions
	if dimensions == 0 {
		dimensions = 1024 // 默认维度
	}

	client := &TongyiClient{
		apiKey:       cfg.APIKey,
		endpoint:     endpoint,
		model:        model,
		httpClient:   httpClient,
		maxRetries:   cfg.MaxRetries,
		dimensions:   dimensions,
		useOpenAIAPI: useOpenAIAPI,
	}

	return client, nil
}

// Name 返回模型名称
func (c *TongyiClient) Name() string {
	return c.model
}

// Embed 生成单条文本的向量表示
func (c *TongyiClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, NewEmbeddingError(ErrCodeEmptyInput, ErrMsgEmptyInput)
	}

	// 调用批处理API处理单个文本
	vectors, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(vectors) == 0 {
		return nil, NewEmbeddingError(ErrCodeServerError, "no embedding vectors returned")
	}

	return vectors[0], nil
}

// EmbedBatch 批量生成文本的向量表示
func (c *TongyiClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 检查批量大小限制
	if c.isV3Model() && len(texts) > 10 {
		return nil, NewEmbeddingError(ErrCodeInvalidRequest, "text-embedding-v3 model supports maximum 10 texts per batch")
	} else if !c.isV3Model() && len(texts) > 25 {
		return nil, NewEmbeddingError(ErrCodeInvalidRequest, "text-embedding-v1/v2 models support maximum 25 texts per batch")
	}

	// 根据API类型选择不同的处理方式
	if c.useOpenAIAPI {
		return c.embedBatchOpenAI(ctx, texts)
	}
	return c.embedBatchDashScope(ctx, texts)
}

// embedBatchOpenAI 使用OpenAI兼容接口处理批量文本
func (c *TongyiClient) embedBatchOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	// 准备请求数据
	reqData := map[string]interface{}{
		"model": c.model,
		"input": texts,
	}

	// 如果是v3模型且维度不是默认值，添加维度参数
	if c.isV3Model() && c.dimensions != 1024 {
		if !isValidDimension(c.dimensions) {
			return nil, NewEmbeddingError(ErrCodeInvalidRequest, fmt.Sprintf("invalid dimension: %d", c.dimensions))
		}
		reqData["dimensions"] = c.dimensions
	}

	reqData["encoding_format"] = "float"

	// 发送请求
	var resp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := c.sendRequest(ctx, reqData, &resp); err != nil {
		return nil, err
	}

	// 解析结果
	result := make([][]float32, len(resp.Data))
	for _, item := range resp.Data {
		result[item.Index] = item.Embedding
	}

	return result, nil
}

// embedBatchDashScope 使用DashScope原生接口处理批量文本
func (c *TongyiClient) embedBatchDashScope(ctx context.Context, texts []string) ([][]float32, error) {
	// 准备请求数据，使用结构体而不是map
	reqData := DashScopeRequest{
		Model: c.model,
		Input: DashScopeRequestInput{
			Texts: texts,
		},
	}

	// 如果是v3模型且维度不是默认值，添加维度参数
	if c.isV3Model() {
		params := &DashScopeParameters{
			OutputType: "dense",
		}

		// 只有当维度不是默认值时才设置维度
		if c.dimensions != 1024 {
			if !isValidDimension(c.dimensions) {
				return nil, NewEmbeddingError(ErrCodeInvalidRequest, fmt.Sprintf("invalid dimension: %d", c.dimensions))
			}
			params.Dimension = c.dimensions
		}

		reqData.Parameters = params
	}

	// 发送请求
	var resp struct {
		StatusCode int    `json:"status_code,omitempty"` // 使用omitempty因为可能不存在
		RequestID  string `json:"request_id"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		Output     struct {
			Embeddings []struct {
				Embedding []float32 `json:"embedding"`
				TextIndex int       `json:"text_index"`
			} `json:"embeddings"`
		} `json:"output"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := c.sendRequest(ctx, reqData, &resp); err != nil {
		return nil, err
	}

	// 修改错误检查逻辑
	// 只有当存在错误代码和错误消息时才认为是错误
	if resp.StatusCode != 0 && resp.StatusCode != 200 {
		return nil, NewEmbeddingError(ErrCodeServerError,
			fmt.Sprintf("API error: %s (%s)", resp.Message, resp.Code))
	}

	// 解析结果
	embeddings := resp.Output.Embeddings
	if len(embeddings) == 0 {
		return nil, NewEmbeddingError(ErrCodeServerError, "no embeddings returned")
	}

	// 构建结果，按照原始文本顺序
	result := make([][]float32, len(texts))
	for _, emb := range embeddings {
		if emb.TextIndex < 0 || emb.TextIndex >= len(texts) {
			continue // 跳过索引越界的情况
		}
		result[emb.TextIndex] = emb.Embedding
	}

	return result, nil
}

// sendRequest 发送API请求并解析响应
func (c *TongyiClient) sendRequest(ctx context.Context, reqData interface{}, respObj interface{}) error {
	// 将请求数据转换为JSON
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return NewEmbeddingError(ErrCodeInvalidRequest, fmt.Sprintf("failed to marshal request: %v", err))
	}

	// 打印请求体以便调试
	//fmt.Printf("[DEBUG] Request body: %s\n", string(jsonData))

	// 创建请求
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return NewEmbeddingError(ErrCodeInvalidRequest, fmt.Sprintf("failed to create request: %v", err))
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("Accept", "application/json")

	// 使用重试机制发送请求
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避重试
			select {
			case <-ctx.Done():
				return NewEmbeddingError(ErrCodeTimeout, ctx.Err().Error())
			case <-time.After(time.Duration(1<<attempt) * 100 * time.Millisecond):
				// 等待后继续
			}
		}

		resp, err = c.httpClient.Do(req)
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
		return NewEmbeddingError(ErrCodeNetworkError, fmt.Sprintf("request failed: %v", lastErr))
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return NewEmbeddingError(ErrCodeServerError, fmt.Sprintf("failed to read response: %v", err))
	}

	// 打印响应体以便调试
	//fmt.Printf("[DEBUG] Response body: %s\n", string(body))

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		// 尝试解析错误响应
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil {
			if errResp.Error != "" {
				return NewEmbeddingError(ErrCodeServerError, errResp.Error)
			}
			if errResp.Message != "" {
				return NewEmbeddingError(ErrCodeServerError, errResp.Message)
			}
		}

		// 如果无法解析，返回原始错误信息
		return NewEmbeddingError(ErrCodeServerError,
			fmt.Sprintf("API error (status %d): %s", resp.StatusCode, string(body)))
	}

	// 解析JSON响应
	if err := json.Unmarshal(body, respObj); err != nil {
		return NewEmbeddingError(ErrCodeServerError,
			fmt.Sprintf("failed to parse response: %v", err))
	}

	return nil
}

// isV3Model 检查是否为v3模型
func (c *TongyiClient) isV3Model() bool {
	return c.model == "text-embedding-v3"
}

// isValidDimension 检查维度是否有效 (仅对v3模型)
func isValidDimension(dim int) bool {
	validDims := []int{1024, 768, 512, 256, 128, 64}
	for _, validDim := range validDims {
		if dim == validDim {
			return true
		}
	}
	return false
}

// 注册通义千问客户端
func init() {
	RegisterClient("tongyi", NewTongyiClient)
}
