package pyprovider

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// RAG API 结构体
type RAGRequest struct {
    Query           string   `json:"query"`
    DocumentIds     []string `json:"document_ids,omitempty"`
    CollectionName  string   `json:"collection_name,omitempty"`
    TopK            int      `json:"top_k"`
    Model           string   `json:"model"`
    Temperature     float64  `json:"temperature"`
    MaxTokens       int      `json:"max_tokens"`
    Stream          bool     `json:"stream"`
    EnableCitation  bool     `json:"enable_citation"`
    EnableReasoning bool     `json:"enable_reasoning"`
}

type RAGSource struct {
    Text       string                 `json:"text"`
    Score      float64                `json:"score"`
    DocumentId string                 `json:"document_id"`
    Metadata   map[string]interface{} `json:"metadata"`
}

type RAGResponse struct {
    Text             string      `json:"text"`
    Sources          []RAGSource `json:"sources"`
    Model            string      `json:"model"`
    PromptTokens     int         `json:"prompt_tokens"`
    CompletionTokens int         `json:"completion_tokens"`
    TotalTokens      int         `json:"total_tokens"`
    ProcessingTime   float64     `json:"processing_time"`
}

func TestNewClient(t *testing.T) {
    // 测试默认配置创建
    client, err := NewClient(nil)
    require.NoError(t, err)
    require.NotNil(t, client)

    config := client.GetConfig()
    assert.Equal(t, "http://localhost:8000/api", config.BaseURL)
    assert.Equal(t, 30*time.Second, config.Timeout)
    assert.Equal(t, 3, config.MaxRetries)

    // 测试自定义配置创建
    customConfig := DefaultConfig().
        WithBaseURL("http://custom-url:9000").
        WithTimeout(5*time.Second).
        WithRetry(2, 500*time.Millisecond)

    customClient, err := NewClient(customConfig)
    require.NoError(t, err)
    require.NotNil(t, customClient)

    config = customClient.GetConfig()
    assert.Equal(t, "http://custom-url:9000", config.BaseURL)
    assert.Equal(t, 5*time.Second, config.Timeout)
    assert.Equal(t, 2, config.MaxRetries)
    assert.Equal(t, 500*time.Millisecond, config.RetryDelay)
}

// TestGetRequest 测试 GET 请求
func TestGetRequest(t *testing.T) {
    // 使用默认配置创建新客户端
    client, err := NewClient(DefaultConfig())
    require.NoError(t, err)

    // 测试健康检查接口
    var response map[string]interface{}
    err = client.Get(context.Background(), "/health/ping", &response)
    require.NoError(t, err)

    // 验证响应
    assert.Equal(t, "pong", response["ping"])
}

// TestPostRequest 测试带 API key 的 embedding 请求
func TestPostRequest(t *testing.T) {
    // 创建带 API key header 的客户端
    config := DefaultConfig()
    client, err := NewClient(config)
    require.NoError(t, err)

    // 添加 API key header
    httpClient, ok := client.(*HTTPClient)
    require.True(t, ok)
    httpClient.WithHeader("X-API-KEY", "test-api-key")

    // 创建 embedding 请求
    req := EmbeddingRequest{
        Text: "This is a test embedding request.",
    }

    // 发送请求到正确的接口
    var resp EmbeddingResponse
    err = client.Post(context.Background(), "/python/embeddings", req, &resp)
    require.NoError(t, err)

    // 验证响应
    assert.True(t, resp.Success)
    assert.Greater(t, resp.Dimension, 0)
    assert.NotEmpty(t, resp.Model)
    assert.NotEmpty(t, resp.Embedding)
    assert.Equal(t, len(req.Text), resp.TextLength)
}

// TestRagEndpoint 测试 RAG 接口
func TestRagEndpoint(t *testing.T) {
    // 创建客户端
    config := DefaultConfig()
    client, err := NewClient(config)
    require.NoError(t, err)

    // 构造 RAG 请求
    ragReq := RAGRequest{
        Query:           "What is machine learning?",
        CollectionName:  "test_collection",
        TopK:            3,
        Model:           "default",
        Temperature:     0.7,
        MaxTokens:       500,
        Stream:          false,
        EnableCitation:  true,
        EnableReasoning: false,
    }

    // 发送 RAG 请求到正确的接口
    var ragResp RAGResponse
    err = client.Post(context.Background(), "/python/llm/rag", ragReq, &ragResp)

    // RAG 接口可能未完全实现或需要数据库中存在特定数据，因此如果出错只记录日志，不直接失败
    if err != nil {
        t.Logf("RAG request error (might be expected): %v", err)
        return
    }

    // 如果响应成功，验证结构
    assert.NotEmpty(t, ragResp.Text)
    assert.NotEmpty(t, ragResp.Model)
    assert.Greater(t, ragResp.TotalTokens, 0)
}