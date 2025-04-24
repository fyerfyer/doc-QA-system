package embedding

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient OpenAI嵌入向量客户端
type OpenAIClient struct {
	client     *openai.Client // OpenAI API客户端
	model      string         // 使用的嵌入模型
	config     Config         // 客户端配置
	dimensions int            // 向量维度
}

// NewOpenAIClient 创建一个新的OpenAI嵌入客户端
func NewOpenAIClient(config Config) (Client, error) {
	// 检查必要配置
	if config.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	// 设置默认模型
	if config.Model == "" {
		config.Model = "text-embedding-3-small"
	}

	// 创建OpenAI客户端配置
	clientConfig := openai.DefaultConfig(config.APIKey)

	// 如果指定了自定义端点，则使用它
	if config.Endpoint != "" {
		clientConfig.BaseURL = config.Endpoint
	}

	// 创建OpenAI客户端
	client := openai.NewClientWithConfig(clientConfig)

	return &OpenAIClient{
		client:     client,
		model:      config.Model,
		config:     config,
		dimensions: config.Dimensions,
	}, nil
}

// Embed 对单个文本生成嵌入向量
func (c *OpenAIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, ErrEmptyText
	}

	// 设置重试相关变量
	var embedding []float32
	retries := 0
	maxRetries := c.config.Retries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// 带重试的嵌入请求
	for retries <= maxRetries {
		// 使用带超时的上下文
		timeoutCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)

		// 创建嵌入请求
		req := openai.EmbeddingRequest{
			Input: []string{text},
			Model: openai.EmbeddingModel(c.model),
		}

		// 发送请求
		resp, err := c.client.CreateEmbeddings(timeoutCtx, req)

		// 请求完成后取消上下文
		cancel()

		if err == nil && len(resp.Data) > 0 {
			embedding = resp.Data[0].Embedding
			break
		}

		// 处理错误
		if err != nil {
			// 如果是速率限制错误，等待后重试
			if isRateLimitError(err) {
				retries++
				if retries <= maxRetries {
					// 指数退避策略
					waitTime := time.Duration(1<<retries) * time.Second
					time.Sleep(waitTime)
					continue
				}
				return nil, ErrRateLimited
			}
			// 其他错误直接返回
			return nil, fmt.Errorf("embedding API error: %v", err)
		}
	}

	return embedding, nil
}

// EmbedBatch 对多个文本生成嵌入向量
func (c *OpenAIClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	if len(texts) > c.config.BatchSize {
		return nil, ErrBatchTooLarge
	}

	// 过滤空文本
	var nonEmptyTexts []string
	for _, text := range texts {
		if text != "" {
			nonEmptyTexts = append(nonEmptyTexts, text)
		}
	}

	if len(nonEmptyTexts) == 0 {
		return [][]float32{}, nil
	}

	// 设置重试相关变量
	var embeddings [][]float32
	retries := 0
	maxRetries := c.config.Retries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// 带重试的批量嵌入请求
	for retries <= maxRetries {
		// 使用带超时的上下文
		timeoutCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()

		// 创建嵌入请求
		req := openai.EmbeddingRequest{
			Input: nonEmptyTexts,
			Model: openai.EmbeddingModel(c.model),
		}

		// 发送请求
		resp, err := c.client.CreateEmbeddings(timeoutCtx, req)
		if err == nil && len(resp.Data) > 0 {
			// 提取嵌入向量
			embeddings = make([][]float32, len(resp.Data))
			for i, data := range resp.Data {
				embeddings[i] = data.Embedding
			}
			break
		}

		// 处理错误
		if err != nil {
			// 如果是速率限制错误，等待后重试
			if isRateLimitError(err) {
				retries++
				if retries <= maxRetries {
					// 指数退避策略
					waitTime := time.Duration(1<<retries) * time.Second
					time.Sleep(waitTime)
					continue
				}
				return nil, ErrRateLimited
			}
			// 其他错误直接返回
			return nil, fmt.Errorf("batch embedding API error: %v", err)
		}
	}

	return embeddings, nil
}

// isRateLimitError 检查是否为速率限制错误
func isRateLimitError(err error) bool {
	// 检查错误是否包含速率限制相关的信息
	return err != nil && (contains(err.Error(), "rate_limit") ||
		contains(err.Error(), "rate limit") ||
		contains(err.Error(), "too many requests"))
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// 在包初始化时注册OpenAI客户端
func init() {
	RegisterClient("openai", NewOpenAIClient)
}
