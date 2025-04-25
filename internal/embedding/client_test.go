package embedding

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestMockClientEmbed 测试Mock客户端的单文本嵌入功能
func TestMockClientEmbed(t *testing.T) {
	// 创建mock客户端
	mockClient := NewMockClient(t)

	// 设置期望
	expectedVector := []float32{0.1, 0.2, 0.3}
	mockClient.EXPECT().Embed(mock.Anything, "测试文本").Return(expectedVector, nil)

	// 调用方法
	ctx := context.Background()
	result, err := mockClient.Embed(ctx, "测试文本")

	// 验证结果
	assert.NoError(t, err)
	assert.Equal(t, expectedVector, result)
}

// TestMockClientEmbedBatch 测试Mock客户端的批量文本嵌入功能
func TestMockClientEmbedBatch(t *testing.T) {
	// 创建mock客户端
	mockClient := NewMockClient(t)

	// 设置期望
	inputTexts := []string{"测试文本1", "测试文本2"}
	expectedVectors := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}
	mockClient.EXPECT().EmbedBatch(mock.Anything, inputTexts).Return(expectedVectors, nil)

	// 调用方法
	ctx := context.Background()
	results, err := mockClient.EmbedBatch(ctx, inputTexts)

	// 验证结果
	assert.NoError(t, err)
	assert.Equal(t, expectedVectors, results)
}

// TestMockClientErrors 测试Mock客户端的错误处理
func TestMockClientErrors(t *testing.T) {
	// 创建mock客户端
	mockClient := NewMockClient(t)

	// 设置期望 - 模拟API错误
	expectedErr := NewEmbeddingError(ErrCodeServerError, "Simulated server error")
	mockClient.EXPECT().Embed(mock.Anything, "错误文本").Return(nil, expectedErr)

	// 调用方法
	ctx := context.Background()
	_, err := mockClient.Embed(ctx, "错误文本")

	// 验证错误
	assert.Error(t, err)
	var embErr EmbeddingError
	assert.True(t, errors.As(err, &embErr))
	assert.Equal(t, ErrCodeServerError, embErr.Code)
}

// TestMockClientName 测试Mock客户端的Name方法
func TestMockClientName(t *testing.T) {
	// 创建mock客户端
	mockClient := NewMockClient(t)

	// 设置期望
	mockClient.EXPECT().Name().Return("mock-model")

	// 调用方法
	name := mockClient.Name()

	// 验证结果
	assert.Equal(t, "mock-model", name)
}

// TestTongyiClientIntegration 测试通义千问客户端的集成测试
// 只有设置了TONGYI_API_KEY环境变量才会执行
func TestTongyiClientIntegration(t *testing.T) {
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("TONGYI_API_KEY environment variable not set, skipping integration test")
	}

	// 创建超短超时以便快速失败
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建客户端 - 使用v3模型的最小维度，减少token消耗
	client, err := NewTongyiClient(
		WithAPIKey(apiKey),
		WithModel("text-embedding-v3"),
		WithDimensions(64), // 使用最低维度
		WithTimeout(10*time.Second),
	)
	require.NoError(t, err, "Failed to create client")

	// 测试只有单一、非常短的文本，最小化API调用和token使用
	t.Run("comprehensive test", func(t *testing.T) {
		// 1. 测试单个短文本
		text := "测试"
		vector, err := client.Embed(ctx, text)
		require.NoError(t, err, "Embedding generation failed")
		assert.Equal(t, 64, len(vector), "Dimension should be 64")

		// 2. 在同一个测试中复用相同的文本，批量测试避免多次调用
		vectors, err := client.EmbedBatch(ctx, []string{text})
		require.NoError(t, err, "Batch embedding generation failed")
		assert.Equal(t, 1, len(vectors), "Should return 1 vector")
		assert.Equal(t, 64, len(vectors[0]), "Dimension should be 64")

		// 3. 验证模型名称
		assert.Contains(t, client.Name(), "embedding", "Model name should contain embedding")
	})
}

// TestClientFactory 测试客户端工厂函数
func TestClientFactory(t *testing.T) {
	// 注册测试工厂函数
	testFactory := func(opts ...Option) (Client, error) {
		return NewMockClient(t), nil
	}
	RegisterClient("test-client", testFactory)

	// 使用工厂函数创建客户端
	client, err := NewClient("test-client")
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// 测试无效的客户端类型
	_, err = NewClient("invalid-client")
	assert.Error(t, err)
	var embErr EmbeddingError
	assert.True(t, errors.As(err, &embErr))
}

// TestConfigOptions 测试配置选项
func TestConfigOptions(t *testing.T) {
	// 测试默认配置
	defaultCfg := DefaultConfig()
	assert.Equal(t, "text-embedding-v1", defaultCfg.Model)
	assert.Equal(t, 30*time.Second, defaultCfg.Timeout)

	// 测试配置选项
	cfg := NewConfig(
		WithAPIKey("test-key"),
		WithModel("test-model"),
		WithTimeout(5*time.Second),
		WithMaxRetries(2),
		WithDimensions(128),
		WithBatchSize(5),
		WithCache(true),
	)

	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "test-model", cfg.Model)
	assert.Equal(t, 5*time.Second, cfg.Timeout)
	assert.Equal(t, 2, cfg.MaxRetries)
	assert.Equal(t, 128, cfg.Dimensions)
	assert.Equal(t, 5, cfg.BatchSize)
	assert.True(t, cfg.EnableCache)
}
