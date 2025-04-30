package vectordb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestTimedCacheBasics 测试TimedCache的基本功能
func TestTimedCacheBasics(t *testing.T) {
	// 创建一个较长TTL的缓存以便测试
	cache := NewTimedCache(5 * time.Second)
	assert.NotNil(t, cache, "Cache should be created")

	// 测试设置和获取
	cache.Set("key1", "value1")
	val, found := cache.Get("key1")
	assert.True(t, found, "Key should be found")
	assert.Equal(t, "value1", val, "Value should match")

	// 测试覆盖已存在的值
	cache.Set("key1", "updated_value")
	val, found = cache.Get("key1")
	assert.True(t, found, "Key should be found after update")
	assert.Equal(t, "updated_value", val, "Value should be updated")

	// 测试不存在的键
	val, found = cache.Get("non_existent")
	assert.False(t, found, "Non-existent key should not be found")
	assert.Nil(t, val, "Value for non-existent key should be nil")
}

// TestTimedCacheExpiration 测试缓存过期功能
func TestTimedCacheExpiration(t *testing.T) {
	// 创建一个短TTL的缓存
	shortTTL := 100 * time.Millisecond
	cache := NewTimedCache(shortTTL)

	// 设置测试值
	cache.Set("expires_soon", "temp_value")

	// 立即获取，应该能找到
	val, found := cache.Get("expires_soon")
	assert.True(t, found, "Key should be found before expiration")
	assert.Equal(t, "temp_value", val, "Value should match before expiration")

	// 等待超过TTL的时间
	time.Sleep(shortTTL * 2)

	// 再次获取，应该已经过期
	val, found = cache.Get("expires_soon")
	assert.False(t, found, "Key should not be found after expiration")
	assert.Nil(t, val, "Value should be nil after expiration")
}

// TestTimedCacheCleanup 测试缓存清理功能
func TestTimedCacheCleanup(t *testing.T) {
	// 创建一个短TTL的缓存
	shortTTL := 100 * time.Millisecond
	cache := NewTimedCache(shortTTL)

	// 添加多个测试值
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// 验证所有值都已添加
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")
	assert.True(t, found1 && found2 && found3, "All keys should be found initially")

	// 手动触发清理
	cache.Cleanup()

	// 验证清理前的键仍然存在（因为还未过期）
	_, found1 = cache.Get("key1")
	_, found2 = cache.Get("key2")
	_, found3 = cache.Get("key3")
	assert.True(t, found1 && found2 && found3, "Keys should still exist after cleanup if not expired")

	// 等待超过TTL的时间
	time.Sleep(shortTTL * 2)

	// 手动触发清理
	cache.Cleanup()

	// 验证键已被清理
	_, found1 = cache.Get("key1")
	_, found2 = cache.Get("key2")
	_, found3 = cache.Get("key3")
	assert.False(t, found1 || found2 || found3, "All keys should be removed after expiration and cleanup")
}

// TestTimedCacheMultipleValues 测试缓存中存储不同类型的值
func TestTimedCacheMultipleValues(t *testing.T) {
	cache := NewTimedCache(5 * time.Second)

	// 测试不同类型的值
	testCases := []struct {
		key   string
		value interface{}
	}{
		{"string_key", "string_value"},
		{"int_key", 42},
		{"float_key", 3.14},
		{"bool_key", true},
		{"slice_key", []string{"a", "b", "c"}},
		{"map_key", map[string]int{"one": 1, "two": 2}},
	}

	// 设置所有测试值
	for _, tc := range testCases {
		cache.Set(tc.key, tc.value)
	}

	// 验证所有值都正确存储
	for _, tc := range testCases {
		val, found := cache.Get(tc.key)
		assert.True(t, found, "Key %s should be found", tc.key)
		assert.Equal(t, tc.value, val, "Value for key %s should match", tc.key)
	}
}

// TestTimedCacheConcurrentAccess 测试并发访问场景
func TestTimedCacheConcurrentAccess(t *testing.T) {
	cache := NewTimedCache(5 * time.Second)
	const concurrentRoutines = 10
	const operationsPerRoutine = 100

	// 使用通道作为同步机制
	done := make(chan bool, concurrentRoutines)

	// 启动多个goroutine同时操作缓存
	for i := 0; i < concurrentRoutines; i++ {
		go func(routineID int) {
			baseKey := "key_" + string(rune('A'+routineID))

			// 执行多次设置和获取操作
			for j := 0; j < operationsPerRoutine; j++ {
				key := baseKey + string(rune('0'+j%10))
				value := routineID*1000 + j

				cache.Set(key, value)
				val, _ := cache.Get(key)

				// 检查值是否正确，但不使用assert以避免并发问题
				if val != value {
					t.Errorf("Concurrent value mismatch: expected %v, got %v", value, val)
				}
			}

			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < concurrentRoutines; i++ {
		<-done
	}
}
