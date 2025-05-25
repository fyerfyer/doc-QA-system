package pyprovider

import (
    "context"
    "fmt"
)

// EmbeddingClient 是Python嵌入服务的客户端
type EmbeddingClient struct {
    client Client
}

// EmbeddingRequest 表示单个文本的嵌入请求
type EmbeddingRequest struct {
    Text string `json:"text"`
}

// BatchEmbeddingRequest 表示批量文本的嵌入请求
type BatchEmbeddingRequest struct {
    Texts []string `json:"texts"`
}

// EmbeddingResponse 表示单个文本的嵌入响应
type EmbeddingResponse struct {
    Success      bool      `json:"success"`
    Model        string    `json:"model"`
    Dimension    int       `json:"dimension"`
    Embedding    []float32 `json:"embedding"`
    TextLength   int       `json:"text_length"`
    ProcessTimeMs int      `json:"process_time_ms"`
}

// BatchEmbeddingResponse 表示批量文本的嵌入响应
type BatchEmbeddingResponse struct {
    Success      bool        `json:"success"`
    Model        string      `json:"model"`
    Count        int         `json:"count"`
    Dimension    int         `json:"dimension"`
    Embeddings   [][]float32 `json:"embeddings"`
    TextLengths  []int       `json:"text_lengths"`
    Normalized   bool        `json:"normalized"`
    ProcessTimeMs int        `json:"process_time_ms"`
}

// ModelListResponse 表示模型列表响应
type ModelListResponse struct {
    Success bool                     `json:"success"`
    Models  map[string][]interface{} `json:"models"`
}

// SimilarityRequest 表示文本相似度请求
type SimilarityRequest struct {
    Text1 string `json:"text1"`
    Text2 string `json:"text2"`
}

// SimilarityResponse 表示文本相似度响应
type SimilarityResponse struct {
    Success    bool    `json:"success"`
    Similarity float32 `json:"similarity"`
    Model      string  `json:"model"`
}

// NewEmbeddingClient 创建一个新的嵌入客户端
func NewEmbeddingClient(client Client) *EmbeddingClient {
    return &EmbeddingClient{
        client: client,
    }
}

// Embed 将单个文本转换为嵌入向量
func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
    return c.EmbedWithModel(ctx, text, "default")
}

// EmbedWithModel 使用指定模型将文本转换为嵌入向量
func (c *EmbeddingClient) EmbedWithModel(ctx context.Context, text string, model string) ([]float32, error) {
    // 验证输入
    if text == "" {
        return nil, fmt.Errorf("empty text provided for embedding")
    }

    // 构建请求路径和查询参数
    reqPath := fmt.Sprintf("/python/embeddings?model=%s", model)

    // 构造请求体
    reqBody := EmbeddingRequest{
        Text: text,
    }

    // 发送POST请求
    var response EmbeddingResponse
    if err := c.client.Post(ctx, reqPath, reqBody, &response); err != nil {
        return nil, fmt.Errorf("failed to generate embedding: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("embedding generation failed: API returned failure status")
    }

    return response.Embedding, nil
}

// EmbedBatch 批量将文本转换为嵌入向量
func (c *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
    return c.EmbedBatchWithOptions(ctx, texts, "default", false)
}

// EmbedBatchWithOptions 带有选项的批量嵌入生成
func (c *EmbeddingClient) EmbedBatchWithOptions(ctx context.Context, texts []string, model string, normalize bool) ([][]float32, error) {
    // 验证输入
    if len(texts) == 0 {
        return nil, fmt.Errorf("empty text list provided for batch embedding")
    }

    // 构建请求路径和查询参数
    reqPath := fmt.Sprintf("/python/embeddings/batch?model=%s", model)
    if normalize {
        reqPath = fmt.Sprintf("%s&normalize=true", reqPath)
    }

    // 构造请求体
    reqBody := BatchEmbeddingRequest{
        Texts: texts,
    }

    // 发送POST请求
    var response BatchEmbeddingResponse
    if err := c.client.Post(ctx, reqPath, reqBody, &response); err != nil {
        return nil, fmt.Errorf("failed to generate batch embeddings: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("batch embedding generation failed: API returned failure status")
    }

    return response.Embeddings, nil
}

// EmbedWithDimension 使用指定维度生成嵌入向量
func (c *EmbeddingClient) EmbedWithDimension(ctx context.Context, text string, model string, dimension int) ([]float32, error) {
    // 验证输入
    if text == "" {
        return nil, fmt.Errorf("empty text provided for embedding")
    }

    // 构建请求路径和查询参数
    reqPath := fmt.Sprintf("/python/embeddings?model=%s&dimension=%d", model, dimension)

    // 构造请求体
    reqBody := EmbeddingRequest{
        Text: text,
    }

    // 发送POST请求
    var response EmbeddingResponse
    if err := c.client.Post(ctx, reqPath, reqBody, &response); err != nil {
        return nil, fmt.Errorf("failed to generate embedding with dimension: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("embedding generation failed: API returned failure status")
    }

    return response.Embedding, nil
}

// EmbedBatchWithDimension 使用指定维度批量生成嵌入向量
func (c *EmbeddingClient) EmbedBatchWithDimension(ctx context.Context, texts []string, model string, dimension int, normalize bool) ([][]float32, error) {
    // 验证输入
    if len(texts) == 0 {
        return nil, fmt.Errorf("empty text list provided for batch embedding")
    }

    // 构建请求路径和查询参数
    reqPath := fmt.Sprintf("/python/embeddings/batch?model=%s&dimension=%d", model, dimension)
    if normalize {
        reqPath = fmt.Sprintf("%s&normalize=true", reqPath)
    }

    // 构造请求体
    reqBody := BatchEmbeddingRequest{
        Texts: texts,
    }

    // 发送POST请求
    var response BatchEmbeddingResponse
    if err := c.client.Post(ctx, reqPath, reqBody, &response); err != nil {
        return nil, fmt.Errorf("failed to generate batch embeddings with dimension: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("batch embedding generation failed: API returned failure status")
    }

    return response.Embeddings, nil
}

// ListModels 获取所有可用的嵌入模型列表
func (c *EmbeddingClient) ListModels(ctx context.Context) (map[string][]interface{}, error) {
    // 构建请求路径
    reqPath := "/python/embeddings/models"
    
    var response ModelListResponse
    if err := c.client.Get(ctx, reqPath, &response); err != nil {
        return nil, fmt.Errorf("failed to get embedding models: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("failed to get embedding models: API returned failure status")
    }

    return response.Models, nil
}

// CalculateSimilarity 计算两段文本的相似度
func (c *EmbeddingClient) CalculateSimilarity(ctx context.Context, text1, text2 string, model string) (float32, error) {
    // 验证输入
    if text1 == "" || text2 == "" {
        return 0, fmt.Errorf("both texts must be non-empty for similarity calculation")
    }

    if model == "" {
        model = "default"
    }

    // 构建请求路径
    reqPath := fmt.Sprintf("/python/embeddings/similarity?model=%s", model)

    // 构造请求体
    reqBody := SimilarityRequest{
        Text1: text1,
        Text2: text2,
    }

    // 发送POST请求
    var response SimilarityResponse
    if err := c.client.Post(ctx, reqPath, reqBody, &response); err != nil {
        return 0, fmt.Errorf("failed to calculate similarity: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return 0, fmt.Errorf("similarity calculation failed: API returned failure status")
    }

    return response.Similarity, nil
}