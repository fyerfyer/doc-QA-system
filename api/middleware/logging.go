package middleware

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log     *logrus.Logger // 全局日志实例
	logOnce sync.Once      // 确保日志只初始化一次
	logDir  = "logs"       // 日志目录
	appName = "doc-qa"     // 应用名称
)

// LogConfig 日志配置
type LogConfig struct {
	Level         string // 日志级别: debug, info, warn, error
	Format        string // 日志格式: json, text
	Output        string // 输出: stdout, file, both
	Directory     string // 日志文件目录
	FileName      string // 日志文件名
	MaxSize       int    // 单个日志文件最大大小(MB)
	MaxAge        int    // 日志文件最长保留时间(天)
	MaxBackups    int    // 最多保留备份数量
	AddCallerInfo bool   // 是否添加调用位置信息
}

// DefaultLogConfig 返回默认日志配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:         "info",
		Format:        "json",
		Output:        "both",
		Directory:     "logs",
		FileName:      appName + ".log",
		MaxSize:       100,
		MaxAge:        7,
		MaxBackups:    10,
		AddCallerInfo: true,
	}
}

// InitLogger 初始化日志系统
func InitLogger(config LogConfig) {
	logOnce.Do(func() {
		logger := logrus.New()

		// 设置日志级别
		switch strings.ToLower(config.Level) {
		case "debug":
			logger.SetLevel(logrus.DebugLevel)
		case "info":
			logger.SetLevel(logrus.InfoLevel)
		case "warn":
			logger.SetLevel(logrus.WarnLevel)
		case "error":
			logger.SetLevel(logrus.ErrorLevel)
		default:
			logger.SetLevel(logrus.InfoLevel)
		}

		// 设置日志格式
		if strings.ToLower(config.Format) == "json" {
			logger.SetFormatter(&logrus.JSONFormatter{
				TimestampFormat: time.RFC3339,
				// 添加额外日志字段
				FieldMap: logrus.FieldMap{
					logrus.FieldKeyTime:  "timestamp",
					logrus.FieldKeyLevel: "level",
					logrus.FieldKeyMsg:   "message",
				},
			})
		} else {
			logger.SetFormatter(&logrus.TextFormatter{
				TimestampFormat: time.RFC3339,
				FullTimestamp:   true,
				ForceColors:     true,
			})
		}

		// 添加调用者信息
		if config.AddCallerInfo {
			logger.SetReportCaller(true)
		}

		// 配置输出
		if config.Output == "file" || config.Output == "both" {
			// 确保日志目录存在
			if config.Directory != "" {
				logDir = config.Directory
			}
			if err := os.MkdirAll(logDir, 0755); err != nil {
				logger.WithError(err).Error("Failed to create log directory")
			}

			// 设置日志轮转
			logPath := filepath.Join(logDir, config.FileName)
			rotator := &lumberjack.Logger{
				Filename:   logPath,
				MaxSize:    config.MaxSize,
				MaxAge:     config.MaxAge,
				MaxBackups: config.MaxBackups,
				Compress:   true,
				LocalTime:  true,
			}

			// 根据配置输出到文件或同时输出
			if config.Output == "both" {
				// 同时输出到控制台和文件
				mw := io.MultiWriter(os.Stdout, rotator)
				logger.SetOutput(mw)
			} else {
				// 只输出到文件
				logger.SetOutput(rotator)
			}
		} else {
			// 默认输出到控制台
			logger.SetOutput(os.Stdout)
		}

		log = logger
		log.Info("Logger initialized")
	})
}

// GetLogger 获取共享的日志实例
func GetLogger() *logrus.Logger {
	// 如果日志还没初始化，使用默认配置初始化
	if log == nil {
		InitLogger(DefaultLogConfig())
	}
	return log
}

// WithTraceContext 向日志添加跟踪上下文
func WithTraceContext(ctx *gin.Context) *logrus.Entry {
	// 从上下文中获取追踪ID，如果没有则生成一个
	traceID, exists := ctx.Get("TraceID")
	if !exists {
		// 生成新的追踪ID
		traceID = uuid.New().String()
		ctx.Set("TraceID", traceID)
		ctx.Header("X-Trace-ID", traceID.(string))
	}

	// 获取用户信息(如果有)
	var userID interface{}
	if user, exists := ctx.Get("UserID"); exists {
		userID = user
	}

	// 构建带上下文的日志条目
	return log.WithFields(logrus.Fields{
		"trace_id": traceID,
		"user_id":  userID,
		"path":     ctx.Request.URL.Path,
		"method":   ctx.Request.Method,
		"ip":       ctx.ClientIP(),
	})
}

// Logger 日志中间件
// 记录请求信息和响应时间
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		start := time.Now()

		// 为请求设置追踪ID
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.New().String()
		}
		c.Set("TraceID", traceID)
		c.Header("X-Trace-ID", traceID)

		// 处理请求，记录路径
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算请求延迟
		latency := time.Since(start)

		// 获取状态码
		statusCode := c.Writer.Status()

		// 添加查询参数
		if raw != "" {
			path = path + "?" + raw
		}

		// 通过状态码确定日志级别
		logFunc := log.Infof
		if statusCode >= 400 && statusCode < 500 {
			logFunc = log.Warnf
		} else if statusCode >= 500 {
			logFunc = log.Errorf
		}

		// 记录请求信息
		logFunc("Request completed | %s | %d | %s | %s | %s",
			c.Request.Method,
			statusCode,
			c.ClientIP(),
			path,
			latency.String(),
		)
	}
}

// RequestLogger 增强的请求日志中间件
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 创建请求日志条目
		reqLogger := WithTraceContext(c)

		// 记录请求开始
		reqLogger.WithFields(logrus.Fields{
			"user_agent": c.Request.UserAgent(),
		}).Info("Request started")

		// 如果是调试模式，记录请求体
		if log.GetLevel() >= logrus.DebugLevel {
			var buf bytes.Buffer
			tee := io.TeeReader(c.Request.Body, &buf)
			body, _ := io.ReadAll(tee)
			c.Request.Body = io.NopCloser(&buf)

			if len(body) > 0 {
				reqLogger.WithField("request_body", string(body)).Debug("Request body")
			}
		}

		// 开始计时
		start := time.Now()

		// 处理请求
		c.Next()

		// 计算延迟并记录日志
		latency := time.Since(start)
		reqLogger.WithFields(logrus.Fields{
			"status":     c.Writer.Status(),
			"latency_ms": latency.Milliseconds(),
			"size":       c.Writer.Size(),
			"errors":     len(c.Errors),
		}).Info("Request completed")
	}
}

// WithContext 创建一个带有字段的日志条目
func WithContext(fields map[string]interface{}) *logrus.Entry {
	return log.WithFields(logrus.Fields(fields))
}

// WithError 创建一个带有错误信息的日志条目
func WithError(err error) *logrus.Entry {
	return log.WithError(err)
}

// WithField 创建一个带有指定字段的日志条目
func WithField(key string, value interface{}) *logrus.Entry {
	return log.WithField(key, value)
}

// GetCallerInfo 获取调用者信息
func GetCallerInfo(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}

	// 获取相对路径，避免显示整个绝对路径
	short := filepath.Base(file)

	return fmt.Sprintf("%s:%d", short, line)
}

// LogError 记录错误并添加调用位置
func LogError(err error, message string) {
	caller := GetCallerInfo(2) // 跳过两层调用栈
	log.WithFields(logrus.Fields{
		"error":  err.Error(),
		"caller": caller,
	}).Error(message)
}

// SetTraceID 设置请求跟踪ID
func SetTraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 读取请求的跟踪ID，如果没有则创建一个新的
		// 这个功能已经在 Logger 中间件中实现，这里只是为了保持API兼容
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			// 使用 WithTraceContext 创建跟踪ID
			traceID, _ := c.Get("TraceID")
			c.Set("TraceID", traceID)
		}
		c.Next()
	}
}

// 初始化日志
func init() {
	if log == nil {
		// 根据环境变量设置日志级别
		config := DefaultLogConfig()
		if level := os.Getenv("LOG_LEVEL"); level != "" {
			config.Level = level
		}

		// 初始化日志系统
		InitLogger(config)
	}
}
