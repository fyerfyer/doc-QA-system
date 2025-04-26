package middleware

import (
	"bytes"
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// 初始化日志配置
func init() {
	// 设置输出到标准输出
	log.SetOutput(os.Stdout)
	// 设置日志格式为JSON格式
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	// 根据环境变量设置日志级别
	if os.Getenv("DEBUG") == "true" {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}
}

// Logger 日志中间件
// 记录请求信息和响应时间
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		start := time.Now()

		// 记录请求路径
		path := c.Request.URL.Path

		// 处理请求前
		c.Next()

		// 请求处理完成后获取结果信息
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// 记录日志
		log.WithFields(logrus.Fields{
			"status_code": statusCode,
			"latency":     latency.String(),
			"client_ip":   clientIP,
			"method":      method,
			"path":        path,
			"user_agent":  c.Request.UserAgent(),
		}).Info("HTTP request")
	}
}

// RequestBodyLog 请求体日志中间件
// 在DEBUG模式下记录请求体内容
func RequestBodyLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅在debug级别时记录请求体
		if log.Level >= logrus.DebugLevel {
			var buf bytes.Buffer
			tee := io.TeeReader(c.Request.Body, &buf)
			body, _ := io.ReadAll(tee)
			c.Request.Body = io.NopCloser(&buf)

			if len(body) > 0 {
				log.WithFields(logrus.Fields{
					"method": c.Request.Method,
					"path":   c.Request.URL.Path,
					"body":   string(body),
				}).Debug("Request body")
			}
		}

		c.Next()
	}
}

// ResponseLogger 响应日志中间件
// 记录响应体内容，通常仅用于开发调试
func ResponseLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅在debug级别时记录响应体
		if log.Level >= logrus.DebugLevel {
			// 创建一个自定义的写入器来捕获响应
			writer := &responseBodyWriter{
				ResponseWriter: c.Writer,
				body:           bytes.NewBufferString(""),
			}
			c.Writer = writer

			c.Next()

			// 请求完成后记录响应体
			log.WithFields(logrus.Fields{
				"method":      c.Request.Method,
				"path":        c.Request.URL.Path,
				"status_code": c.Writer.Status(),
				"response":    writer.body.String(),
			}).Debug("Response body")
		} else {
			c.Next()
		}
	}
}

// responseBodyWriter 自定义的响应写入器
// 用于捕获响应体内容
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

// Write 重写Write方法，将响应体同时写入buffer
func (r *responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// SetTraceID 将追踪ID设置到上下文和响应头中
func SetTraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头中获取追踪ID
		traceID := c.GetHeader("X-Trace-ID")

		// 如果没有，则生成一个新的
		if traceID == "" {
			traceID = generateTraceID()
		}

		// 设置到上下文
		c.Set("TraceID", traceID)

		// 设置到响应头
		c.Header("X-Trace-ID", traceID)

		c.Next()
	}
}

// generateTraceID 生成追踪ID
func generateTraceID() string {
	// 使用时间戳和随机数生成简单的追踪ID
	return time.Now().Format("20060102150405") + "-" +
		time.Now().Format("000000")
}

// 常用日志字段
const (
	FieldTraceID  = "trace_id"    // 追踪ID
	FieldUserID   = "user_id"     // 用户ID
	FieldPath     = "path"        // 请求路径
	FieldMethod   = "method"      // 请求方法
	FieldStatus   = "status_code" // 状态码
	FieldLatency  = "latency"     // 延迟时间
	FieldClientIP = "client_ip"   // 客户端IP
	FieldError    = "error"       // 错误信息
)

func GetLogger() *logrus.Logger {
	return log
}
