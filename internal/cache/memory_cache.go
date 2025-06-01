package cache

import (
	"time"

	gocache "github.com/patrickmn/go-cache"
)

// MemoryCache 基于go-cache实现的内存缓存
type MemoryCache struct {
	cache *gocache.Cache
}

// NewMemoryCache 创建一个新的内存缓存
func NewMemoryCache(config Config) (Cache, error) {
	// 默认过期时间和清理间隔
	defaultExpiration := config.DefaultTTL
	if defaultExpiration == 0 {
		defaultExpiration = 24 * time.Hour
	}

	cleanupInterval := config.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 10 * time.Minute
	}

	// 创建go-cache实例
	cache := gocache.New(defaultExpiration, cleanupInterval)

	return &MemoryCache{
		cache: cache,
	}, nil
}

// Get 获取缓存内容
func (m *MemoryCache) Get(key string) (string, bool, error) {
	if value, found := m.cache.Get(key); found {
		str, ok := value.(string)
		if !ok {
			return "", false, nil
		}
		return str, true, nil
	}
	return "", false, nil
}

// Set 设置缓存内容
func (m *MemoryCache) Set(key string, value string, ttl time.Duration) error {
	// 如果ttl为0，使用默认过期时间
	if ttl == 0 {
		ttl = gocache.DefaultExpiration
	}
	m.cache.Set(key, value, ttl)
	return nil
}

// Delete 删除缓存项
func (m *MemoryCache) Delete(key string) error {
	m.cache.Delete(key)
	return nil
}

// Clear 清空所有缓存
func (m *MemoryCache) Clear() error {
	m.cache.Flush()
	return nil
}

// 在包初始化时注册内存缓存
func init() {
	RegisterCache("memory", NewMemoryCache)
}
