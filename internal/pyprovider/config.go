package pyprovider

import (
    "time"
)

// PyServiceConfig 存储Python服务连接配置
type PyServiceConfig struct {
    BaseURL     string        // Python服务基础URL
    Timeout     time.Duration // 请求超时时间
    MaxRetries  int           // 最大重试次数
    RetryDelay  time.Duration // 重试间隔
    DialTimeout time.Duration // 连接超时
    EnableTLS   bool          // 是否启用TLS
}

// DefaultConfig 返回默认配置
func DefaultConfig() *PyServiceConfig {
    return &PyServiceConfig{
        BaseURL:     "http://localhost:8000/api",
        Timeout:     30 * time.Second,
        MaxRetries:  3,
        RetryDelay:  time.Second,
        DialTimeout: 5 * time.Second,
        EnableTLS:   false,
    }
}

// WithBaseURL 设置基础URL
func (c *PyServiceConfig) WithBaseURL(url string) *PyServiceConfig {
    c.BaseURL = url
    return c
}

// WithTimeout 设置请求超时时间
func (c *PyServiceConfig) WithTimeout(timeout time.Duration) *PyServiceConfig {
    c.Timeout = timeout
    return c
}

// WithRetry 设置重试参数
func (c *PyServiceConfig) WithRetry(maxRetries int, retryDelay time.Duration) *PyServiceConfig {
    c.MaxRetries = maxRetries
    c.RetryDelay = retryDelay
    return c
}

// WithTLS 设置是否启用TLS
func (c *PyServiceConfig) WithTLS(enable bool) *PyServiceConfig {
    c.EnableTLS = enable
    return c
}