package embedding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPythonEmbeddingClient 测试Python嵌入客户端
func TestPythonEmbeddingClient(t *testing.T) {
    // 创建Python嵌入客户端
    client, err := NewPythonClient(
        WithBaseURL("http://localhost:8000/api"),
        WithTimeout(10*time.Second),
        WithModel("text-embedding-v3"),
    )
    require.NoError(t, err, "Should create Python embedding client without error")

    ctx := context.Background()

    // 测试单个文本嵌入
    text := "这是一个测试文本"
    embedding, err := client.Embed(ctx, text)
    
    require.NoError(t, err, "Should embed text without error")
    assert.NotEmpty(t, embedding, "Should return non-empty embedding")
    t.Logf("Embedding length: %d", len(embedding))

    // 测试批量文本嵌入
    texts := []string{
        "第一个测试文本",
        "第二个测试文本",
    }
    
    embeddings, err := client.EmbedBatch(ctx, texts)
    
    require.NoError(t, err, "Should batch embed texts without error")
    assert.Len(t, embeddings, len(texts), "Should return embeddings for all texts")
    for _, emb := range embeddings {
        assert.NotEmpty(t, emb, "Each embedding should be non-empty")
    }

    // 验证模型名称
    assert.Equal(t, "text-embedding-v3", client.Name(), "Should return correct model name")
}

// TestPythonClientWithoutModel 测试Python嵌入客户端使用默认模型
func TestPythonClientWithoutModel(t *testing.T) {
    // 创建Python嵌入客户端（不指定模型）
    client, err := NewPythonClient(
        WithBaseURL("http://localhost:8000/api"),
        WithTimeout(10*time.Second),
    )
    require.NoError(t, err, "Should create Python embedding client without error")

    ctx := context.Background()

    // 测试嵌入生成
    text := "默认模型测试"
    embedding, err := client.Embed(ctx, text)
    
    require.NoError(t, err, "Should embed text with default model without error")
    assert.NotEmpty(t, embedding, "Should return non-empty embedding")
    
    // 名称应该是默认值
    assert.Equal(t, "text-embedding-v1", client.Name(), "Should use default model name")
}

// TestPythonClientEmptyInputs 测试Python嵌入客户端处理空输入
func TestPythonClientEmptyInputs(t *testing.T) {
    // 创建Python嵌入客户端
    client, err := NewPythonClient(
        WithBaseURL("http://localhost:8000/api"),
        WithTimeout(5*time.Second),
    )
    require.NoError(t, err, "Should create Python embedding client without error")

    ctx := context.Background()

    // 测试空文本
    _, err = client.Embed(ctx, "")
    assert.Error(t, err, "Should return error for empty text")

    // 测试空批处理
    embeddings, err := client.EmbedBatch(ctx, []string{})
    assert.NoError(t, err, "Should handle empty batch without error")
    assert.Empty(t, embeddings, "Should return empty result for empty input batch")
}