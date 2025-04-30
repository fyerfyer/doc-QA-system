package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
)

// ErrorType 错误类型常量
type ErrorType string

const (
	// ErrorTypeValidation 输入验证错误
	ErrorTypeValidation ErrorType = "VALIDATION_ERROR"
	// ErrorTypeUnauthorized 未授权错误
	ErrorTypeUnauthorized ErrorType = "UNAUTHORIZED_ERROR"
	// ErrorTypeForbidden 禁止访问错误
	ErrorTypeForbidden ErrorType = "FORBIDDEN_ERROR"
	// ErrorTypeNotFound 资源不存在错误
	ErrorTypeNotFound ErrorType = "NOT_FOUND_ERROR"
	// ErrorTypeInternal 内部服务器错误
	ErrorTypeInternal ErrorType = "INTERNAL_ERROR"
	// ErrorTypeBusiness 业务逻辑错误
	ErrorTypeBusiness ErrorType = "BUSINESS_ERROR"
	// ErrorTypeTimeout 超时错误
	ErrorTypeTimeout ErrorType = "TIMEOUT_ERROR"
	// ErrorTypeTooManyRequests 请求过多错误
	ErrorTypeTooManyRequests ErrorType = "TOO_MANY_REQUESTS_ERROR"
)

// AppError 应用错误结构体
type AppError struct {
	Type      ErrorType           // 错误类型
	Message   string              // 错误消息
	Details   string              // 详细错误信息
	Code      int                 // HTTP状态码
	Timestamp time.Time           // 错误发生时间
	Path      string              // 发生错误的路径
	Fields    map[string]string   // 字段验证错误详情
	Cause     error               // 原始错误
}

// Error 实现error接口的方法
func (e AppError) Error() string {
	msg := fmt.Sprintf("%s: %s", e.Type, e.Message)
	if e.Details != "" {
		msg += fmt.Sprintf(" (%s)", e.Details)
	}
	return msg
}

// Unwrap 实现errors.Unwrap接口，用于错误链
func (e AppError) Unwrap() error {
	return e.Cause
}

// ResponseJSON 生成用于API响应的JSON表示
func (e AppError) ResponseJSON(traceID string) gin.H {
	resp := gin.H{
		"code":    e.Code,
		"type":    e.Type,
		"message": e.Message,
		"time":    e.Timestamp.Format(time.RFC3339),
	}

	if e.Details != "" {
		resp["details"] = e.Details
	}

	if len(e.Fields) > 0 {
		resp["fields"] = e.Fields
	}

	if traceID != "" {
		resp["trace_id"] = traceID
	}

	return resp
}

// SetCause 设置原始错误
func (e *AppError) SetCause(err error) *AppError {
	e.Cause = err
	if err != nil && e.Details == "" {
		e.Details = err.Error()
	}
	return e
}

// SetPath 设置错误发生的路径
func (e *AppError) SetPath(path string) *AppError {
	e.Path = path
	return e
}

// AddFieldError 添加字段错误
func (e *AppError) AddFieldError(field, message string) *AppError {
	if e.Fields == nil {
		e.Fields = make(map[string]string)
	}
	e.Fields[field] = message
	return e
}

// NewValidationError 创建输入验证错误
func NewValidationError(message string, details ...string) *AppError {
	return &AppError{
		Type:      ErrorTypeValidation,
		Message:   message,
		Details:   strings.Join(details, "; "),
		Code:      http.StatusBadRequest,
		Timestamp: time.Now(),
		Fields:    make(map[string]string),
	}
}

// NewUnauthorizedError 创建未授权错误
func NewUnauthorizedError(message string) *AppError {
	return &AppError{
		Type:      ErrorTypeUnauthorized,
		Message:   message,
		Code:      http.StatusUnauthorized,
		Timestamp: time.Now(),
	}
}

// NewForbiddenError 创建禁止访问错误
func NewForbiddenError(message string) *AppError {
	return &AppError{
		Type:      ErrorTypeForbidden,
		Message:   message,
		Code:      http.StatusForbidden,
		Timestamp: time.Now(),
	}
}

// NewNotFoundError 创建资源不存在错误
func NewNotFoundError(message string) *AppError {
	return &AppError{
		Type:      ErrorTypeNotFound,
		Message:   message,
		Code:      http.StatusNotFound,
		Timestamp: time.Now(),
	}
}

// NewInternalError 创建内部服务器错误
func NewInternalError(message string, cause error) *AppError {
	details := ""
	if cause != nil {
		details = cause.Error()
	}

	return &AppError{
		Type:      ErrorTypeInternal,
		Message:   message,
		Details:   details,
		Cause:     cause,
		Code:      http.StatusInternalServerError,
		Timestamp: time.Now(),
	}
}

// NewBusinessError 创建业务逻辑错误
func NewBusinessError(message string, details ...string) *AppError {
	return &AppError{
		Type:      ErrorTypeBusiness,
		Message:   message,
		Details:   strings.Join(details, "; "),
		Code:      http.StatusBadRequest,
		Timestamp: time.Now(),
	}
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(message string) *AppError {
	return &AppError{
		Type:      ErrorTypeTimeout,
		Message:   message,
		Code:      http.StatusGatewayTimeout,
		Timestamp: time.Now(),
	}
}

// NewTooManyRequestsError 创建请求过多错误
func NewTooManyRequestsError(message string) *AppError {
	return &AppError{
		Type:      ErrorTypeTooManyRequests,
		Message:   message,
		Code:      http.StatusTooManyRequests,
		Timestamp: time.Now(),
	}
}

// FromValidationErrors 从验证错误创建AppError
func FromValidationErrors(err validator.ValidationErrors) *AppError {
	appErr := NewValidationError("请求参数验证失败")

	for _, fieldErr := range err {
		// 字段名转换为小写驼峰格式
		fieldName := fieldErr.Field()
		if fieldName != "" {
			fieldName = strings.ToLower(fieldName[:1]) + fieldName[1:]
		}

		// 生成友好的错误消息
		message := ""
		switch fieldErr.Tag() {
		case "required":
			message = "不能为空"
		case "email":
			message = "必须是有效的电子邮件地址"
		case "min":
			message = fmt.Sprintf("最小值为 %s", fieldErr.Param())
		case "max":
			message = fmt.Sprintf("最大值为 %s", fieldErr.Param())
		case "len":
			message = fmt.Sprintf("长度必须为 %s", fieldErr.Param())
		default:
			message = fmt.Sprintf("不满足验证条件: %s", fieldErr.Tag())
		}

		appErr.AddFieldError(fieldName, message)
	}

	return appErr
}

// ErrorHandler 统一错误处理中间件
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 前置操作：准备处理请求
		c.Next()

		// 检查c.Errors是否有错误
		if len(c.Errors) > 0 {
			// 提取跟踪ID
			traceID, _ := c.Get("TraceID")
			traceIDStr := ""
			if traceID != nil {
				traceIDStr = traceID.(string)
			}

			// 获取最后一个错误
			err := c.Errors.Last().Err

			// 转换为AppError
			var appErr *AppError

			switch typed := err.(type) {
			case *AppError:
				// 已经是AppError类型
				appErr = typed
				appErr.SetPath(c.Request.URL.Path)

			case validator.ValidationErrors:
				// 处理验证错误
				appErr = FromValidationErrors(typed)

			default:
				// 处理特定类型的错误
				if errors.Is(err, models.ErrDocumentNotFound) {
					appErr = NewNotFoundError("文档不存在")
				} else if errors.Is(err, models.ErrInvalidDocumentStatus) {
					appErr = NewBusinessError("文档状态无效")
				} else {
					// 将其他错误包装为内部错误
					appErr = NewInternalError("处理请求时发生错误", err)
				}
			}

			// 记录错误日志
			logEntry := log.WithFields(logrus.Fields{
				"trace_id": traceIDStr,
				"path":     c.Request.URL.Path,
				"method":   c.Request.Method,
				"status":   appErr.Code,
				"error":    appErr.Error(),
			})

			if appErr.Type == ErrorTypeInternal {
				logEntry.Error("Internal server error")
			} else {
				logEntry.Warn("Request error")
			}

			// 生成并返回错误响应
			c.JSON(appErr.Code, appErr.ResponseJSON(traceIDStr))
			c.Abort()
			return
		}
	}
}

// Recovery 错误恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 获取跟踪ID
				traceID, _ := c.Get("TraceID")
				traceIDStr := ""
				if traceID != nil {
					traceIDStr = traceID.(string)
				}

				// 记录日志
				log.WithFields(logrus.Fields{
					"trace_id": traceIDStr,
					"path":     c.Request.URL.Path,
					"method":   c.Request.Method,
					"panic":    err,
				}).Error("Panic recovered")

				// 创建并返回错误响应
				var message string
				switch v := err.(type) {
				case string:
					message = v
				case error:
					message = v.Error()
				default:
					message = "服务器内部错误"
				}

				appErr := NewInternalError(message, fmt.Errorf("%v", err))
				c.JSON(http.StatusInternalServerError, appErr.ResponseJSON(traceIDStr))
				c.Abort()
			}
		}()

		c.Next()
	}
}

// HandleAppError 在控制器中捕获并处理错误
func HandleAppError(c *gin.Context, err error) {
	_ = c.Error(err)
}

// IsAppError 检查错误是否为AppError类型
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

// AsAppError 将错误转换为AppError类型
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// WrapError 将普通错误包装为AppError
func WrapError(err error, message string) *AppError {
	if err == nil {
		return nil
	}

	// 检查是否已经是AppError
	if appErr, ok := AsAppError(err); ok {
		return appErr
	}

	// 包装为内部错误
	return NewInternalError(message, err)
}
