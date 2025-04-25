package embedding

import "fmt"

// EmbeddingError 嵌入错误类型
type EmbeddingError struct {
	Code    int    // 错误码
	Message string // 错误消息
}

// Error 实现error接口
func (e EmbeddingError) Error() string {
	return fmt.Sprintf("embedding error (code=%d): %s", e.Code, e.Message)
}

// 错误码常量
const (
	ErrCodeInvalidAPIKey  = 1001 // 无效的API密钥
	ErrCodeInvalidRequest = 1002 // 无效的请求
	ErrCodeNetworkError   = 1003 // 网络连接错误
	ErrCodeRateLimited    = 1004 // 请求频率超限
	ErrCodeServerError    = 1005 // 服务器错误
	ErrCodeTimeout        = 1006 // 请求超时
	ErrCodeEmptyInput     = 1007 // 输入为空
)

// 错误消息常量
const (
	ErrMsgInvalidAPIKey  = "invalid API key"
	ErrMsgInvalidRequest = "invalid request parameters"
	ErrMsgRateLimited    = "too many requests, rate limit exceeded"
	ErrMsgServerError    = "server error occurred"
	ErrMsgTimeout        = "request timed out"
	ErrMsgEmptyInput     = "input text cannot be empty"
	ErrMsgNetworkError   = "network connection error"
)

// NewEmbeddingError 创建新的嵌入错误
func NewEmbeddingError(code int, message string) EmbeddingError {
	return EmbeddingError{
		Code:    code,
		Message: message,
	}
}
