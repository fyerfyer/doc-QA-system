package embedding

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// MockClient 实现了Client接口的模拟客户端
type MockClient struct {
	vectors map[string][]float32 // 预设的向量结果
}

// 创建一个新的模拟客户端
func NewMockClient() *MockClient {
	return &MockClient{
		vectors: map[string][]float32{
			"hello": {0.1, 0.2, 0.3},
			"world": {0.4, 0.5, 0.6},
		},
	}
}

// Embed 实现Client接口的Embed方法
func (m *MockClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, ErrEmptyText
	}

	// 对于特定关键词有固定返回值
	if vec, ok := m.vectors[text]; ok {
		return vec, nil
	}

	// 否则返回一个固定向量
	return []float32{1.0, 0.0, 0.0}, nil
}

// EmbedBatch 实现Client接口的EmbedBatch方法
func (m *MockClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	if len(texts) > 10 {
		return nil, ErrBatchTooLarge
	}

	results := make([][]float32, len(texts))
	for i, text := range texts {
		if text == "" {
			results[i] = []float32{}
			continue
		}

		if vec, ok := m.vectors[text]; ok {
			results[i] = vec
		} else {
			// 默认向量，索引作为值的一部分，便于验证
			results[i] = []float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3}
		}
	}

	return results, nil
}

// testRegisterMockClient 注册模拟客户端
func testRegisterMockClient() {
	RegisterClient("mock", func(config Config) (Client, error) {
		return NewMockClient(), nil
	})
}

// TestClientCreation 测试客户端创建
func TestClientCreation(t *testing.T) {
	// 注册模拟客户端
	testRegisterMockClient()

	// 测试创建默认客户端
	t.Run("Default Client", func(t *testing.T) {
		config := DefaultConfig()
		config.Provider = "mock" // 使用模拟客户端避免实际API调用

		client, err := NewClient(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	// 测试无效提供商
	t.Run("Invalid Provider", func(t *testing.T) {
		config := DefaultConfig()
		config.Provider = "invalid"

		// 应该回退到默认的OpenAI，但我们没有设置API密钥，所以应该失败
		_, err := NewClient(config)
		if err == nil {
			t.Fatal("Should fail with invalid provider without API key")
		}
	})

	// 测试配置值
	t.Run("Config Values", func(t *testing.T) {
		config := DefaultConfig()
		if config.Provider != "openai" {
			t.Errorf("Default provider should be openai, got %s", config.Provider)
		}
		if config.BatchSize != 16 {
			t.Errorf("Default batch size should be 16, got %d", config.BatchSize)
		}
	})
}

// TestMockEmbedding 测试模拟嵌入客户端
func TestMockEmbedding(t *testing.T) {
	// 创建模拟客户端
	client := NewMockClient()

	// 测试单个文本嵌入
	t.Run("Single Text", func(t *testing.T) {
		ctx := context.Background()

		// 测试预设值
		vector, err := client.Embed(ctx, "hello")
		if err != nil {
			t.Fatalf("Embed failed: %v", err)
		}

		if len(vector) != 3 {
			t.Errorf("Expected vector length 3, got %d", len(vector))
		}

		if vector[0] != 0.1 || vector[1] != 0.2 || vector[2] != 0.3 {
			t.Errorf("Unexpected vector values: %v", vector)
		}

		// 测试空文本
		_, err = client.Embed(ctx, "")
		if err != ErrEmptyText {
			t.Errorf("Expected ErrEmptyText, got %v", err)
		}
	})

	// 测试批量文本嵌入
	t.Run("Batch Text", func(t *testing.T) {
		ctx := context.Background()

		texts := []string{"hello", "world", "test"}
		vectors, err := client.EmbedBatch(ctx, texts)
		if err != nil {
			t.Fatalf("EmbedBatch failed: %v", err)
		}

		if len(vectors) != len(texts) {
			t.Errorf("Expected %d vectors, got %d", len(texts), len(vectors))
		}

		// 检验第一个预设向量
		if vectors[0][0] != 0.1 || vectors[0][1] != 0.2 || vectors[0][2] != 0.3 {
			t.Errorf("Unexpected vector values for 'hello': %v", vectors[0])
		}

		// 检验第二个预设向量
		if vectors[1][0] != 0.4 || vectors[1][1] != 0.5 || vectors[1][2] != 0.6 {
			t.Errorf("Unexpected vector values for 'world': %v", vectors[1])
		}

		// 检验自动生成的向量
		if vectors[2][0] != 0.2 || vectors[2][1] != 0.4 || vectors[2][2] != 0.6 {
			t.Errorf("Unexpected vector values for 'test': %v", vectors[2])
		}

		// 测试空批量
		emptyVectors, err := client.EmbedBatch(ctx, []string{})
		if err != nil {
			t.Errorf("EmbedBatch with empty texts failed: %v", err)
		}
		if len(emptyVectors) != 0 {
			t.Errorf("Expected empty vectors, got %d vectors", len(emptyVectors))
		}

		// 测试批量过大
		largeBatch := make([]string, 11)
		_, err = client.EmbedBatch(ctx, largeBatch)
		if err != ErrBatchTooLarge {
			t.Errorf("Expected ErrBatchTooLarge, got %v", err)
		}
	})
}

// TestBatchProcessor 测试批处理器
func TestBatchProcessor(t *testing.T) {
	// 创建模拟客户端
	mockClient := NewMockClient()

	// 创建批处理器
	batchSize := 2
	maxWorkers := 2
	processor := NewBatchProcessor(mockClient, batchSize, maxWorkers)

	// 测试批处理
	t.Run("Batch Processing", func(t *testing.T) {
		ctx := context.Background()
		texts := []string{"hello", "world", "test", "example"}

		vectors, err := processor.Process(ctx, texts)
		if err != nil {
			t.Fatalf("Batch processing failed: %v", err)
		}

		if len(vectors) != len(texts) {
			t.Errorf("Expected %d vectors, got %d", len(texts), len(vectors))
		}

		// 验证结果
		if len(vectors[0]) != 3 || vectors[0][0] != 0.1 {
			t.Errorf("Expected first vector to be [0.1, 0.2, 0.3], got %v", vectors[0])
		}

		if len(vectors[1]) != 3 || vectors[1][0] != 0.4 {
			t.Errorf("Expected second vector to be [0.4, 0.5, 0.6], got %v", vectors[1])
		}
	})

	// 测试空文本处理
	t.Run("Empty Texts", func(t *testing.T) {
		ctx := context.Background()
		emptyVectors, err := processor.Process(ctx, []string{})
		if err != nil {
			t.Errorf("Process with empty texts failed: %v", err)
		}
		if len(emptyVectors) != 0 {
			t.Errorf("Expected empty vectors, got %d vectors", len(emptyVectors))
		}

		// 测试处理含空字符串的批次
		mixedTexts := []string{"hello", "", "world"}
		vectors, err := processor.Process(ctx, mixedTexts)
		if err != nil {
			t.Fatalf("Process with mixed texts failed: %v", err)
		}
		if len(vectors) != 3 {
			t.Errorf("Expected 3 results, got %d", len(vectors))
		}
		if vectors[1] != nil {
			t.Errorf("Expected nil for empty string, got %v", vectors[1])
		}
	})
}

// TestRealOpenAIClient 测试实际的OpenAI客户端
func TestRealOpenAIClient(t *testing.T) {
	// 获取API密钥
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping OpenAI client test")
	}

	// 创建OpenAI配置
	config := DefaultConfig()
	config.APIKey = apiKey

	fmt.Printf("api key: %v\n", config.APIKey)

	// 创建客户端
	client, err := NewOpenAIClient(config)
	if err != nil {
		t.Fatalf("Failed to create OpenAI client: %v", err)
	}

	// 测试单个文本嵌入
	t.Run("Actual API Single Embed", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		vector, err := client.Embed(ctx, "This is a test sentence.")
		if err != nil {
			t.Fatalf("OpenAI embed failed: %v", err)
		}

		// OpenAI text-embedding-3-small 应该返回1536维的向量
		if len(vector) != config.Dimensions {
			t.Errorf("Expected vector dimension %d, got %d", config.Dimensions, len(vector))
		}
	})

	// 测试批量文本嵌入
	t.Run("Actual API Batch Embed", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		texts := []string{
			"First test sentence.",
			"Second completely different sentence.",
		}

		vectors, err := client.EmbedBatch(ctx, texts)
		if err != nil {
			t.Fatalf("OpenAI batch embed failed: %v", err)
		}

		if len(vectors) != len(texts) {
			t.Errorf("Expected %d vectors, got %d", len(texts), len(vectors))
		}

		// 验证维度
		for i, vec := range vectors {
			if len(vec) != config.Dimensions {
				t.Errorf("Vector %d should have dimension %d, got %d", i, config.Dimensions, len(vec))
			}
		}
	})
}
