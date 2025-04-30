package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库连接
var DB *gorm.DB

// Config 数据库配置
type Config struct {
	Type         string        // 数据库类型：sqlite, mysql, postgres
	DSN          string        // 数据源名称
	MaxOpenConns int           // 最大打开连接数
	MaxIdleConns int           // 最大空闲连接数
	MaxLifetime  time.Duration // 连接最大生命周期
}

// DefaultConfig 返回默认数据库配置
func DefaultConfig() *Config {
	return &Config{
		Type:         "sqlite",
		DSN:          "data/database.db", // 默认SQLite数据库路径
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		MaxLifetime:  time.Hour,
	}
}

// Setup 设置并初始化数据库连接
func Setup(cfg *Config, log *logrus.Logger) error {
	var err error
	var dialector gorm.Dialector

	// 根据数据库类型创建方言
	switch cfg.Type {
	case "sqlite":
		// 确保目录存在
		if err := ensureDir(cfg.DSN); err != nil {
			return fmt.Errorf("failed to create database directory: %v", err)
		}
		dialector = sqlite.Open(cfg.DSN)
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	// 创建GORM日志配置
	gormLogger := logger.New(
		&logrusWriter{log}, // 使用logrus作为日志输出
		logger.Config{
			SlowThreshold:             200 * time.Millisecond, // 慢查询阈值
			LogLevel:                  logger.Warn,            // 日志级别
			IgnoreRecordNotFoundError: true,                   // 忽略记录未找到错误
			Colorful:                  false,                  // 无色彩输出
		},
	)

	// 连接数据库
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// 配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %v", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)

	// 自动迁移模型
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("failed to auto migrate: %v", err)
	}

	log.Info("Database connection established successfully")
	return nil
}

// Close 关闭数据库连接
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %v", err)
	}

	return sqlDB.Close()
}

// autoMigrate 自动迁移数据库模型
func autoMigrate() error {
	// 这里添加所有需要迁移的模型
	return DB.AutoMigrate(
		&models.Document{},
		&models.DocumentSegment{},
	)
}

// ensureDir 确保目录存在
func ensureDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir != "." {
		if err := createDirIfNotExists(dir); err != nil {
			return err
		}
	}
	return nil
}

// createDirIfNotExists 如果目录不存在则创建
func createDirIfNotExists(dir string) error {
	if dir == "" {
		return nil
	}

	// 创建目录树
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	return nil
}

// logrusWriter 实现gorm.Writer接口，将日志输出到logrus
type logrusWriter struct {
	logger *logrus.Logger
}

// Printf 实现io.Writer接口，将GORM日志转发到logrus
func (w *logrusWriter) Printf(format string, args ...interface{}) {
	w.logger.Tracef(format, args...)
}
