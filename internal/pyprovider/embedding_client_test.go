package pyprovider

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestEmbeddingClientReal 测试真实的Python嵌入服务连接
func TestEmbeddingClientReal(t *testing.T) {
    // 创建配置
    config := DefaultConfig()
    config.WithTimeout(10 * time.Second)

    // 创建HTTP客户端
    client, err := NewClient(config)
    require.NoError(t, err, "Should create client without error")

    // 创建嵌入客户端
    embeddingClient := NewEmbeddingClient(client)
    require.NotNil(t, embeddingClient, "Should create embedding client")

    // 创建上下文
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    t.Run("Test single text embedding", func(t *testing.T) {
        // 测试单个文本的嵌入
        text := "这是一段测试文本，用于测试嵌入功能。"
        embedding, err := embeddingClient.Embed(ctx, text)
        
        require.NoError(t, err, "Should embed text without error")
        assert.NotEmpty(t, embedding, "Should return non-empty embedding")
        t.Logf("Received embedding with length: %d", len(embedding))
    })

    t.Run("Test batch text embedding", func(t *testing.T) {
        // 测试批量文本的嵌入
        texts := []string{
            "第一段测试文本",
            "第二段测试文本",
            "第三段测试文本",
        }
        
        embeddings, err := embeddingClient.EmbedBatch(ctx, texts)
        
        require.NoError(t, err, "Should batch embed texts without error")
        assert.Len(t, embeddings, len(texts), "Should return embeddings for all texts")
        for i, embedding := range embeddings {
            assert.NotEmpty(t, embedding, "Embedding %d should not be empty", i)
        }
    })

    t.Run("Test embedding with model", func(t *testing.T) {
        // 测试使用指定模型的嵌入
        text := "使用特定模型测试"
        model := "text-embedding-v3"
        
        embedding, err := embeddingClient.EmbedWithModel(ctx, text, model)
        
        require.NoError(t, err, "Should embed with specified model without error")
        assert.NotEmpty(t, embedding, "Should return non-empty embedding")
    })

    t.Run("Test embed with dimension", func(t *testing.T) {
        // 测试使用指定维度的嵌入
        text := "测试维度设置"
        model := "text-embedding-v3"
        dimension := 128 // 较小维度，节省测试资源
        
        embedding, err := embeddingClient.EmbedWithDimension(ctx, text, model, dimension)
        
        require.NoError(t, err, "Should embed with specified dimension without error")
        assert.Len(t, embedding, dimension, "Should return embedding with specified dimension")
    })

    t.Run("Test batch embedding with dimension", func(t *testing.T) {
        // 测试批量使用指定维度的嵌入
        texts := []string{"批量维度测试1", "批量维度测试2"}
        model := "text-embedding-v3"
        dimension := 128
        normalize := true
        
        embeddings, err := embeddingClient.EmbedBatchWithDimension(ctx, texts, model, dimension, normalize)
        
        require.NoError(t, err, "Should embed batch with dimension without error")
        assert.Len(t, embeddings, len(texts), "Should return embeddings for all texts")
        for _, embedding := range embeddings {
            assert.Len(t, embedding, dimension, "Each embedding should have specified dimension")
        }
    })

    t.Run("Test list models", func(t *testing.T) {
        // 测试获取可用模型列表
        models, err := embeddingClient.ListModels(ctx)
        
        require.NoError(t, err, "Should list models without error")
        assert.NotEmpty(t, models, "Should return available models")
        t.Logf("Available models: %v", models)
    })

    t.Run("Test calculate similarity", func(t *testing.T) {
        // 测试计算文本相似度
        text1 := "人工智能是计算机科学的一个子领域"
        text2 := "AI是计算机科学研究的重要方向"
        model := "text-embedding-v3"
        
        similarity, err := embeddingClient.CalculateSimilarity(ctx, text1, text2, model)
        
        require.NoError(t, err, "Should calculate similarity without error")
        assert.GreaterOrEqual(t, similarity, float32(0.0), "Similarity should be >= 0")
        assert.LessOrEqual(t, similarity, float32(1.0), "Similarity should be <= 1")
        t.Logf("Similarity between texts: %f", similarity)
    })
}

// TestEmbeddingClientErrors 测试嵌入客户端错误处理
func TestEmbeddingClientErrors(t *testing.T) {
    // 创建带有无效URL的配置，以测试错误处理
    config := DefaultConfig()
    config.WithBaseURL("http://invalid-url-that-does-not-exist.example")
    config.WithTimeout(2 * time.Second)
    config.WithRetry(1, time.Millisecond)

    // 创建HTTP客户端
    client, err := NewClient(config)
    require.NoError(t, err, "Should create client without error")

    // 创建嵌入客户端
    embeddingClient := NewEmbeddingClient(client)
    require.NotNil(t, embeddingClient, "Should create embedding client")

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    // 测试连接错误
    _, err = embeddingClient.Embed(ctx, "测试文本")
    assert.Error(t, err, "Should return error for invalid connection")
    t.Logf("Expected error received: %v", err)

    // 测试空输入
    _, err = embeddingClient.Embed(ctx, "")
    assert.Error(t, err, "Should return error for empty input")
    t.Logf("Expected error received: %v", err)
}