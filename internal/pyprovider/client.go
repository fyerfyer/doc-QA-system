package pyprovider

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// Client 是Python服务的HTTP客户端接口
type Client interface {
    // Get 发送GET请求
    Get(ctx context.Context, path string, result interface{}) error
    // Post 发送POST请求
    Post(ctx context.Context, path string, data interface{}, result interface{}) error
    // GetConfig 获取客户端配置
    GetConfig() *PyServiceConfig
}

// HTTPClient 实现了Python服务的HTTP客户端
type HTTPClient struct {
    client  *http.Client
    config  *PyServiceConfig
    headers map[string]string
}

// APIError 表示API调用返回的错误
type APIError struct {
    StatusCode int    `json:"status_code"`
    Message    string `json:"message"`
    Detail     string `json:"detail"`
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error (status code: %d): %s - %s", e.StatusCode, e.Message, e.Detail)
}

// NewClient 创建一个新的Python服务HTTP客户端
func NewClient(config *PyServiceConfig) (Client, error) {
    if config == nil {
        config = DefaultConfig()
    }

    client := &http.Client{
        Timeout: config.Timeout,
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 20,
            IdleConnTimeout:     90 * time.Second,
        },
    }

    return &HTTPClient{
        client: client,
        config: config,
        headers: map[string]string{
            "Content-Type": "application/json",
            "Accept":       "application/json",
            "User-Agent":   "Doc-QA-Go-Client/1.0",
        },
    }, nil
}

// Get 发送GET请求到Python服务
func (c *HTTPClient) Get(ctx context.Context, path string, result interface{}) error {
    url := fmt.Sprintf("%s%s", c.config.BaseURL, path)

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }

    // 添加请求头
    for key, value := range c.headers {
        req.Header.Set(key, value)
    }

    // 执行带重试的请求
    return c.doRequestWithRetry(req, result)
}

// Post 发送POST请求到Python服务
func (c *HTTPClient) Post(ctx context.Context, path string, data interface{}, result interface{}) error {
    url := fmt.Sprintf("%s%s", c.config.BaseURL, path)

    // 将数据序列化为JSON
    var body io.Reader
    if data != nil {
        jsonData, err := json.Marshal(data)
        if err != nil {
            return fmt.Errorf("failed to marshal request data: %w", err)
        }
        body = bytes.NewReader(jsonData)
    }

    // 创建请求
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }

    // 添加请求头
    for key, value := range c.headers {
        req.Header.Set(key, value)
    }

    // 执行带重试的请求
    return c.doRequestWithRetry(req, result)
}

// doRequestWithRetry 执行HTTP请求并支持重试
func (c *HTTPClient) doRequestWithRetry(req *http.Request, result interface{}) error {
    var lastErr error
    var resp *http.Response

    // 重试逻辑
    for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
        if attempt > 0 {
            select {
            case <-req.Context().Done():
                return fmt.Errorf("request context canceled: %w", req.Context().Err())
            case <-time.After(c.config.RetryDelay * time.Duration(attempt)):
                // 增加退避时间
            }
        }

        resp, lastErr = c.client.Do(req)
        if lastErr == nil {
            break
        }

        fmt.Printf("Request attempt %d failed: %v\n", attempt+1, lastErr)
    }

    if lastErr != nil {
        return fmt.Errorf("HTTP request failed: %w", lastErr)
    }
    defer resp.Body.Close()

    // 读取响应体
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("failed to read response body: %w", err)
    }

    // 检查状态码
    if resp.StatusCode >= 400 {
        apiErr := &APIError{
            StatusCode: resp.StatusCode,
            Message:    "API call failed",
        }

        // 尝试解析错误详情
        var errResp struct {
            Detail string `json:"detail"`
        }
        if err := json.Unmarshal(body, &errResp); err == nil && errResp.Detail != "" {
            apiErr.Detail = errResp.Detail
        } else {
            apiErr.Detail = string(body)
        }

        return apiErr
    }

    // 解析响应体到结果对象
    if result != nil && len(body) > 0 {
        if err := json.Unmarshal(body, result); err != nil {
            return fmt.Errorf("failed to unmarshal response JSON: %w", err)
        }
    }

    return nil
}

// GetConfig 返回客户端配置
func (c *HTTPClient) GetConfig() *PyServiceConfig {
    return c.config
}

// WithHeader 添加自定义请求头
func (c *HTTPClient) WithHeader(key, value string) *HTTPClient {
    c.headers[key] = value
    return c
}