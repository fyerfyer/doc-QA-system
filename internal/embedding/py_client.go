package embedding

import (
	"context"
	"fmt"
	"time"
	
	"github.com/fyerfyer/doc-QA-system/internal/pyprovider"
)

// PythonEmbeddingClient 使用Python服务的嵌入客户端
type PythonEmbeddingClient struct {
	client    *pyprovider.EmbeddingClient // Python嵌入服务客户端
	modelName string                      // 模型名称
}

// NewPythonClient 创建一个新的Python嵌入服务客户端
func NewPythonClient(opts ...Option) (Client, error) {
	// 应用选项
	cfg := NewConfig(opts...)

	// 创建Python服务配置
	pyConfig := pyprovider.DefaultConfig()

	// 如果提供了BaseURL，则使用；否则使用默认值
	if cfg.BaseURL != "" {
		pyConfig.WithBaseURL(cfg.BaseURL)
	}

	// 设置超时时间
	pyConfig.WithTimeout(cfg.Timeout)

	// 设置重试参数
	pyConfig.WithRetry(cfg.MaxRetries, time.Second)

	// 创建HTTP客户端
	httpClient, err := pyprovider.NewClient(pyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Python service HTTP client: %w", err)
	}

	// 创建嵌入客户端
	embeddingClient := pyprovider.NewEmbeddingClient(httpClient)

	return &PythonEmbeddingClient{
		client:    embeddingClient,
		modelName: cfg.Model,
	}, nil
}

// Embed 生成单条文本的向量表示
func (c *PythonEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text provided for embedding")
	}

	// 如果指定了模型名称，则使用该模型；否则使用默认模型
	if c.modelName != "" && c.modelName != "default" {
		return c.client.EmbedWithModel(ctx, text, c.modelName)
	}

	// 使用默认模型
	return c.client.Embed(ctx, text)
}

// EmbedBatch 批量生成多条文本的向量表示
func (c *PythonEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 如果指定了模型名称，则使用该模型；否则使用默认模型
	if c.modelName != "" && c.modelName != "default" {
		return c.client.EmbedBatchWithOptions(ctx, texts, c.modelName, false)
	}

	// 使用默认模型
	return c.client.EmbedBatch(ctx, texts)
}

// Name 返回模型名称
func (c *PythonEmbeddingClient) Name() string {
	return c.modelName
}

// 注册Python嵌入客户端
func init() {
	RegisterClient("python", NewPythonClient)
}
