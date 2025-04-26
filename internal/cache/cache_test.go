package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMemoryCache 测试内存缓存的基本功能
func TestMemoryCache(t *testing.T) {
	// 创建内存缓存
	config := Config{
		Type:            "memory",
		DefaultTTL:      time.Second * 2,
		CleanupInterval: time.Second,
	}
	cache, err := NewMemoryCache(config)
	assert.NoError(t, err)
	assert.NotNil(t, cache)

	// 测试Set和Get
	err = cache.Set("key1", "value1", 0) // 使用默认TTL
	assert.NoError(t, err)

	val, found, err := cache.Get("key1")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "value1", val)

	// 测试不存在的键
	val, found, err = cache.Get("non-existent")
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, val)

	// 测试过期
	err = cache.Set("expire-soon", "temp-value", time.Millisecond*500)
	assert.NoError(t, err)

	// 等待过期
	time.Sleep(time.Second)

	val, found, err = cache.Get("expire-soon")
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, val)

	// 测试删除
	err = cache.Set("to-delete", "delete-me", 0)
	assert.NoError(t, err)

	err = cache.Delete("to-delete")
	assert.NoError(t, err)

	val, found, err = cache.Get("to-delete")
	assert.NoError(t, err)
	assert.False(t, found)

	// 测试清空
	err = cache.Set("key2", "value2", 0)
	assert.NoError(t, err)

	err = cache.Clear()
	assert.NoError(t, err)

	val, found, err = cache.Get("key2")
	assert.NoError(t, err)
	assert.False(t, found)
}

// TestRedisCache 测试Redis缓存
// 需要本地运行Redis服务器在默认端口
func TestRedisCache(t *testing.T) {
	config := Config{
		Type:       "redis",
		RedisAddr:  "localhost:6379",
		DefaultTTL: time.Second * 2,
	}

	// 尝试创建Redis缓存，如果失败则跳过测试
	cache, err := NewRedisCache(config)
	if err != nil {
		t.Skip("Redis server not available, skipping Redis cache tests")
		return
	}

	assert.NotNil(t, cache)

	// 测试Set和Get
	err = cache.Set("redis-key1", "redis-value1", 0)
	assert.NoError(t, err)

	val, found, err := cache.Get("redis-key1")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "redis-value1", val)

	// 测试不存在的键
	val, found, err = cache.Get("redis-non-existent")
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, val)

	// 测试过期
	err = cache.Set("redis-expire-soon", "redis-temp-value", time.Second)
	assert.NoError(t, err)

	// 等待过期
	time.Sleep(time.Second * 2)

	val, found, err = cache.Get("redis-expire-soon")
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, val)

	// 测试删除
	err = cache.Set("redis-to-delete", "redis-delete-me", 0)
	assert.NoError(t, err)

	err = cache.Delete("redis-to-delete")
	assert.NoError(t, err)

	val, found, err = cache.Get("redis-to-delete")
	assert.NoError(t, err)
	assert.False(t, found)

	// 注意：不测试Clear方法，因为它会清空整个Redis数据库
}

// TestCacheFactory 测试缓存工厂函数
func TestCacheFactory(t *testing.T) {
	// 测试内存缓存创建
	memConfig := DefaultConfig()
	memCache, err := NewCache(memConfig)
	assert.NoError(t, err)
	assert.NotNil(t, memCache)

	// 测试Redis缓存创建
	redisConfig := Config{
		Type:      "redis",
		RedisAddr: "localhost:6379",
	}

	// 尝试创建，不判断错误（可能Redis不可用）
	redisCache, _ := NewCache(redisConfig)
	if redisCache != nil {
		// 如果创建成功，简单测试功能
		err = redisCache.Set("factory-test", "value", 0)
		assert.NoError(t, err)

		// 清理
		redisCache.Delete("factory-test")
	}

	// 测试未知缓存类型（应该返回默认内存缓存）
	unknownConfig := Config{
		Type: "unknown-type",
	}
	unknownCache, err := NewCache(unknownConfig)
	assert.NoError(t, err)
	assert.NotNil(t, unknownCache)
}

// TestGenerateCacheKey 测试缓存键生成
func TestGenerateCacheKey(t *testing.T) {
	// 测试没有部分
	key := GenerateCacheKey("prefix")
	assert.Equal(t, "prefix", key)

	// 测试单部分
	key = GenerateCacheKey("prefix", "part1")
	assert.Equal(t, "prefix:part1", key)

	// 测试多部分
	key = GenerateCacheKey("prefix", "part1", "part2", "part3")
	assert.Equal(t, "prefix:part1:part2:part3", key)
}
