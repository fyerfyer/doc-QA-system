package cache

import (
	"time"
)

// Cache 缓存接口
type Cache interface {
	Get(key string) (value string, found bool, err error)
	Set(key string, value string, ttl time.Duration) error
	Delete(key string) error
	Clear() error
}

// Factory 缓存工厂函数类型
type Factory func(config Config) (Cache, error)

// 注册的缓存实现
var registry = make(map[string]Factory)

// RegisterCache 注册缓存实现
func RegisterCache(name string, factory Factory) {
	registry[name] = factory
}

// NewCache 创建缓存实例
func NewCache(config Config) (Cache, error) {
	if factory, ok := registry[config.Type]; ok {
		return factory(config)
	}
	// 默认使用内存缓存
	return NewMemoryCache(config)
}

// Config 缓存配置
type Config struct {
	// 缓存类型: "memory", "redis" 等
	Type string
	// Redis连接地址 (仅Redis缓存使用)
	RedisAddr string
	// Redis密码 (仅Redis缓存使用)
	RedisPassword string
	// Redis数据库编号 (仅Redis缓存使用)
	RedisDB int
	// 默认缓存过期时间
	DefaultTTL time.Duration
	// 自动清理间隔时间 (仅内存缓存使用)
	CleanupInterval time.Duration
}

// DefaultConfig 返回默认缓存配置
func DefaultConfig() Config {
	return Config{
		Type:            "memory",
		DefaultTTL:      time.Hour * 24,
		CleanupInterval: time.Minute * 10,
	}
}

// GenerateCacheKey 生成标准化的缓存键
// 可以基于不同参数生成一致的键
func GenerateCacheKey(prefix string, parts ...string) string {
	if len(parts) == 0 {
		return prefix
	}

	key := prefix
	for _, part := range parts {
		key += ":" + part
	}
	return key
}
