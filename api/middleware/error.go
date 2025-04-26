package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// 定义应用中的错误类型常量
const (
	ErrorTypeValidation   = "VALIDATION_ERROR"   // 输入验证错误
	ErrorTypeUnauthorized = "UNAUTHORIZED_ERROR" // 未授权错误
	ErrorTypeForbidden    = "FORBIDDEN_ERROR"    // 禁止访问错误
	ErrorTypeNotFound     = "NOT_FOUND_ERROR"    // 资源不存在错误
	ErrorTypeInternal     = "INTERNAL_ERROR"     // 内部服务器错误
	ErrorTypeBusiness     = "BUSINESS_ERROR"     // 业务逻辑错误
)

// AppError 应用错误结构体
type AppError struct {
	Type    string // 错误类型
	Message string // 错误消息
	Details string // 详细错误信息
	Code    int    // 错误代码
}

// Error 实现error接口的方法
func (e AppError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Type, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// NewValidationError 创建输入验证错误
func NewValidationError(message string, details ...string) AppError {
	return AppError{
		Type:    ErrorTypeValidation,
		Message: message,
		Details: strings.Join(details, "; "),
		Code:    http.StatusBadRequest,
	}
}

// NewUnauthorizedError 创建未授权错误
func NewUnauthorizedError(message string) AppError {
	return AppError{
		Type:    ErrorTypeUnauthorized,
		Message: message,
		Code:    http.StatusUnauthorized,
	}
}

// NewForbiddenError 创建禁止访问错误
func NewForbiddenError(message string) AppError {
	return AppError{
		Type:    ErrorTypeForbidden,
		Message: message,
		Code:    http.StatusForbidden,
	}
}

// NewNotFoundError 创建资源不存在错误
func NewNotFoundError(message string) AppError {
	return AppError{
		Type:    ErrorTypeNotFound,
		Message: message,
		Code:    http.StatusNotFound,
	}
}

// NewInternalError 创建内部服务器错误
func NewInternalError(message string, details ...string) AppError {
	return AppError{
		Type:    ErrorTypeInternal,
		Message: message,
		Details: strings.Join(details, "; "),
		Code:    http.StatusInternalServerError,
	}
}

// NewBusinessError 创建业务逻辑错误
func NewBusinessError(message string, details ...string) AppError {
	return AppError{
		Type:    ErrorTypeBusiness,
		Message: message,
		Details: strings.Join(details, "; "),
		Code:    http.StatusBadRequest,
	}
}

// ErrorMiddleware 统一错误处理中间件
func ErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 捕获 panic
		defer func() {
			if err := recover(); err != nil {
				// 获取堆栈跟踪信息
				stack := string(debug.Stack())

				// 记录错误日志
				log.WithFields(logrus.Fields{
					"error": err,
					"stack": stack,
					"path":  c.Request.URL.Path,
				}).Error("Panic recovered in API request")

				// 构造客户端响应
				errorResponse := model.NewErrorResponse(
					http.StatusInternalServerError,
					"An unexpected error occurred",
				)

				// 在开发环境中可以返回详细错误
				if gin.Mode() == gin.DebugMode {
					errorResponse.Message = fmt.Sprintf("Panic: %v", err)
				}

				// 添加请求跟踪ID
				traceID, exists := c.Get("TraceID")
				if exists {
					errorResponse.TraceID = traceID.(string)
				}

				// 中止请求处理并返回错误响应
				c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse)
			}
		}()

		// 处理请求
		c.Next()

		// 检查是否已经有错误被处理
		if len(c.Errors) > 0 {
			// 取最后一个错误进行处理
			err := c.Errors.Last().Err

			// 获取跟踪ID
			traceID := ""
			if traceIDValue, exists := c.Get("TraceID"); exists {
				traceID = traceIDValue.(string)
			}

			// 根据错误类型进行处理
			switch e := err.(type) {
			case AppError:
				// 记录应用错误日志
				log.WithFields(logrus.Fields{
					"error_type": e.Type,
					"trace_id":   traceID,
					"path":       c.Request.URL.Path,
				}).Error(e.Message)

				// 返回对应的HTTP状态码和错误响应
				errResp := model.NewErrorResponse(e.Code, e.Message)
				errResp.TraceID = traceID

				c.JSON(e.Code, errResp)

			case *AppError:
				// 指针类型的应用错误处理
				log.WithFields(logrus.Fields{
					"error_type": e.Type,
					"trace_id":   traceID,
					"path":       c.Request.URL.Path,
				}).Error(e.Message)

				errResp := model.NewErrorResponse(e.Code, e.Message)
				errResp.TraceID = traceID

				c.JSON(e.Code, errResp)

			default:
				// 处理其他类型的错误（如标准库错误）
				log.WithFields(logrus.Fields{
					"trace_id": traceID,
					"path":     c.Request.URL.Path,
				}).Error(err.Error())

				// 默认作为内部服务器错误处理
				errResp := model.NewErrorResponse(
					http.StatusInternalServerError,
					"Internal server error",
				)
				errResp.TraceID = traceID

				// 在开发环境下显示具体错误信息
				if gin.Mode() == gin.DebugMode {
					errResp.Message = err.Error()
				}

				c.JSON(http.StatusInternalServerError, errResp)
			}

			// 中止继续处理
			c.Abort()
		}
	}
}

// HandleError 在处理器中使用的错误处理辅助函数
func HandleError(c *gin.Context, err error) {
	// 添加错误到上下文中
	_ = c.Error(err)
}
