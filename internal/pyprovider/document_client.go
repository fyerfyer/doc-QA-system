package pyprovider

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "net/url"
    "path/filepath"
    "strings"

    "github.com/google/uuid"
)

// DocumentParseResult 表示文档解析结果
type DocumentParseResult struct {
    DocumentID string         `json:"document_id"`
    Content    string         `json:"content"`
    Title      string         `json:"title"`
    Meta       map[string]any `json:"meta"`
    Pages      int            `json:"pages"`
    Words      int            `json:"words"`
    Chars      int            `json:"chars"`
}

// DocumentParseResponse 表示文档解析API的响应
type DocumentParseResponse struct {
    Success       bool                `json:"success"`
    DocumentID    string              `json:"document_id"`
    TaskID        string              `json:"task_id"`
    Result        DocumentParseResult `json:"result"`
    ProcessTimeMs int                 `json:"process_time_ms"`
}

// DocumentClient 是Python文档解析服务的客户端
type DocumentClient struct {
    client Client
}

// NewDocumentClient 创建一个新的文档解析客户端
func NewDocumentClient(client Client) *DocumentClient {
    return &DocumentClient{
        client: client,
    }
}

// ParseDocument 解析指定路径的文档
func (c *DocumentClient) ParseDocument(ctx context.Context, filePath string) (*DocumentParseResult, error) {
    // 生成唯一文档ID
    documentID := uuid.New().String()

    // 获取原始文件名及扩展名
    originalFilename := filepath.Base(filePath)

    // 构造表单数据
    formData := url.Values{}
    formData.Set("document_id", documentID)
    formData.Set("file_path", filePath)
    formData.Set("store_result", "true")
    formData.Set("original_filename", originalFilename) // 添加此行以发送原始文件名

    // 构建请求URL
    reqPath := "/python/documents/parse"

    // 创建自定义请求（因为需要发送表单数据而不是JSON）
    reqURL := fmt.Sprintf("%s%s", c.client.GetConfig().BaseURL, reqPath)
    req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(formData.Encode()))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    // 设置请求头
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    // 执行请求
    var response DocumentParseResponse
    httpClient := &http.Client{Timeout: c.client.GetConfig().Timeout}
    resp, err := httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    // 检查状态码
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("API call failed (status code: %d): %s", resp.StatusCode, string(body))
    }

    // 解析响应
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    // 检查API响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("document parsing failed: %s", response.Result.Content)
    }

    return &response.Result, nil
}

// ParseDocumentWithReader 从io.Reader解析文档
func (c *DocumentClient) ParseDocumentWithReader(ctx context.Context, reader io.Reader, fileName string) (*DocumentParseResult, error) {
    // 生成唯一文档ID
    documentID := uuid.New().String()

    // 创建multipart表单
    var requestBody bytes.Buffer
    writer := multipart.NewWriter(&requestBody)

    // 添加文档ID字段
    if err := writer.WriteField("document_id", documentID); err != nil {
        return nil, fmt.Errorf("failed to add document_id field: %w", err)
    }

    // 添加store_result字段
    if err := writer.WriteField("store_result", "true"); err != nil {
        return nil, fmt.Errorf("failed to add store_result field: %w", err)
    }

    // 添加文件内容
    part, err := writer.CreateFormFile("file", fileName)
    if err != nil {
        return nil, fmt.Errorf("failed to create file form field: %w", err)
    }

    // 从reader复制数据到请求体
    if _, err := io.Copy(part, reader); err != nil {
        return nil, fmt.Errorf("failed to copy file data: %w", err)
    }

    // 完成multipart表单
    if err := writer.Close(); err != nil {
        return nil, fmt.Errorf("failed to close form writer: %w", err)
    }

    // 构建请求URL
    reqPath := "/python/documents/parse"

    // 创建自定义请求
    reqURL := fmt.Sprintf("%s%s", c.client.GetConfig().BaseURL, reqPath)
    req, err := http.NewRequestWithContext(ctx, "POST", reqURL, &requestBody)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    // 设置请求头
    req.Header.Set("Content-Type", writer.FormDataContentType())

    // 执行请求
    var response DocumentParseResponse
    httpClient := &http.Client{Timeout: c.client.GetConfig().Timeout}
    resp, err := httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    // 检查状态码
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("API call failed (status code: %d): %s", resp.StatusCode, string(body))
    }

    // 解析响应
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    // 检查API响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("document parsing failed: %s", response.Result.Content)
    }

    return &response.Result, nil
}

// ParseDocumentAsync 异步解析文档，返回任务ID
func (c *DocumentClient) ParseDocumentAsync(ctx context.Context, filePath string) (string, error) {
    _, err := c.ParseDocument(ctx, filePath)
    if err != nil {
        return "", err
    }
    // 假设API在响应中返回了任务ID
    return "", errors.New("暂未实现异步解析功能")
    // TODO: 实现真正的异步解析，目前先返回错误
}

// GetDocumentContent 获取已解析文档的内容
func (c *DocumentClient) GetDocumentContent(ctx context.Context, documentID string) (*DocumentParseResult, error) {
    // 构建请求路径
    reqPath := fmt.Sprintf("/python/documents/%s", documentID)

    // 发送GET请求
    var response struct {
        Success    bool                `json:"success"`
        DocumentID string              `json:"document_id"`
        Result     DocumentParseResult `json:"result"`
    }

    err := c.client.Get(ctx, reqPath, &response)
    if err != nil {
        return nil, fmt.Errorf("failed to get document content: %w", err)
    }

    if !response.Success {
        return nil, fmt.Errorf("failed to get document content: request successful but API returned failure status")
    }

    return &response.Result, nil
}

// Content 表示文本块内容
type Content struct {
    Text  string `json:"text"`  // 块文本内容
    Index int    `json:"index"` // 块索引
}

// SplitOptions 表示文本分块的选项
type SplitOptions struct {
    ChunkSize    int            `json:"chunk_size"`    // 块大小
    ChunkOverlap int            `json:"chunk_overlap"` // 块重叠大小
    SplitType    string         `json:"split_type"`    // 分块策略：paragraph, sentence, length, semantic
    StoreResult  bool           `json:"store_result"`  // 是否存储结果
    Metadata     map[string]any `json:"metadata"`      // 附加元数据
}

// SplitTextRequest 表示分块API请求体
type SplitTextRequest struct {
    Text         string         `json:"text"`          // 文本内容
    DocumentID   string         `json:"document_id"`   // 文档ID
    ChunkSize    int            `json:"chunk_size"`    // 块大小
    ChunkOverlap int            `json:"chunk_overlap"` // 块重叠大小
    SplitType    string         `json:"split_type"`    // 分割策略
    StoreResult  bool           `json:"store_result"`  // 是否存储结果
    Metadata     map[string]any `json:"metadata"`      // 附加元数据
}

// ChunkResponse 表示分块API的响应
type ChunkResponse struct {
    Success       bool      `json:"success"`         // 是否成功
    DocumentID    string    `json:"document_id"`     // 文档ID
    TaskID        string    `json:"task_id"`         // 任务ID
    Chunks        []Content `json:"chunks"`          // 文本块列表
    ChunkCount    int       `json:"chunk_count"`     // 块数量
    ProcessTimeMs int       `json:"process_time_ms"` // 处理时间(毫秒)
}

// DefaultSplitOptions 返回默认的分块选项
func DefaultSplitOptions() *SplitOptions {
    return &SplitOptions{
        ChunkSize:    1000,
        ChunkOverlap: 200,
        SplitType:    "paragraph",
        StoreResult:  true,
        Metadata:     make(map[string]any),
    }
}

// SplitText 将文本分割成多个块
func (c *DocumentClient) SplitText(ctx context.Context, text string, documentID string, options *SplitOptions) ([]Content, string, error) {
    // 使用默认选项（如果未提供）
    if options == nil {
        options = DefaultSplitOptions()
    }

    // 构建请求数据
    reqData := SplitTextRequest{
        Text:         text,
        DocumentID:   documentID,
        ChunkSize:    options.ChunkSize,
        ChunkOverlap: options.ChunkOverlap,
        SplitType:    options.SplitType,
        StoreResult:  options.StoreResult,
        Metadata:     options.Metadata,
    }

    // 构建请求路径
    reqPath := "/python/documents/chunk"

    // 发送POST请求
    var response ChunkResponse
    if err := c.client.Post(ctx, reqPath, reqData, &response); err != nil {
        return nil, "", fmt.Errorf("failed to split text: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, "", fmt.Errorf("failed to split text: API returned failure status")
    }

    return response.Chunks, response.TaskID, nil
}

// GetDocumentChunks 获取存储的文档分块结果
func (c *DocumentClient) GetDocumentChunks(ctx context.Context, documentID string, taskID string) ([]Content, error) {
    // 构建请求路径
    reqPath := fmt.Sprintf("/python/documents/%s/chunks", documentID)

    // 添加可选的任务ID查询参数
    if taskID != "" {
        reqPath = fmt.Sprintf("%s?task_id=%s", reqPath, taskID)
    }

    // 发送GET请求
    var response struct {
        Success    bool      `json:"success"`
        DocumentID string    `json:"document_id"`
        TaskID     string    `json:"task_id"`
        Chunks     []Content `json:"chunks"`
        ChunkCount int       `json:"chunk_count"`
    }

    if err := c.client.Get(ctx, reqPath, &response); err != nil {
        return nil, fmt.Errorf("failed to get document chunks: %w", err)
    }

    // 检查响应是否成功
    if !response.Success {
        return nil, fmt.Errorf("failed to get document chunks: API returned failure status")
    }

    return response.Chunks, nil
}