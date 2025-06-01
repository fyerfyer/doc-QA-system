package llm

import "fmt"

// LLMError 大模型调用错误类型
type LLMError struct {
	Code    int    // 错误码
	Message string // 错误消息
}

// Error 实现error接口
func (e LLMError) Error() string {
	return fmt.Sprintf("llm error (code=%d): %s", e.Code, e.Message)
}

// 错误码常量
const (
	ErrCodeInvalidAPIKey  = 1001 // 无效的API密钥
	ErrCodeInvalidRequest = 1002 // 无效的请求
	ErrCodeNetworkError   = 1003 // 网络连接错误
	ErrCodeRateLimited    = 1004 // 请求频率超限
	ErrCodeServerError    = 1005 // 服务器错误
	ErrCodeTimeout        = 1006 // 请求超时
	ErrCodeEmptyPrompt    = 1007 // 提示词为空
	ErrCodeContentFilter  = 1008 // 内容安全过滤
	ErrCodeModelOverload  = 1009 // 模型过载
	ErrCodeContextTooLong = 1010 // 上下文过长
)

// 错误消息常量
const (
	ErrMsgInvalidAPIKey  = "invalid API key"
	ErrMsgInvalidRequest = "invalid request parameters"
	ErrMsgRateLimited    = "too many requests, rate limit exceeded"
	ErrMsgServerError    = "server error occurred"
	ErrMsgTimeout        = "request timed out"
	ErrMsgEmptyPrompt    = "prompt cannot be empty"
	ErrMsgNetworkError   = "network connection error"
	ErrMsgContentFilter  = "content filtered due to safety concerns"
	ErrMsgModelOverload  = "model is currently overloaded"
	ErrMsgContextTooLong = "context length exceeds model's maximum"
)

// NewLLMError 创建新的大模型错误
func NewLLMError(code int, message string) LLMError {
	return LLMError{
		Code:    code,
		Message: message,
	}
}

// WrapError 包装普通错误为LLM错误
func WrapError(err error, code int) LLMError {
	if err == nil {
		return LLMError{Code: code, Message: "unknown error"}
	}

	// 如果已经是LLMError类型，则直接返回
	if llmErr, ok := err.(LLMError); ok {
		return llmErr
	}

	return LLMError{
		Code:    code,
		Message: err.Error(),
	}
}
