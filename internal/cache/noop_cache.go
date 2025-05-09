package cache

import "time"

// NoopCache 是一个无操作的缓存实现
// 用于禁用缓存功能时使用
type NoopCache struct{}

// NewNoopCache 创建一个新的无操作缓存
func NewNoopCache() Cache {
	return &NoopCache{}
}

// Get 获取缓存内容（总是返回未找到）
func (n *NoopCache) Get(key string) (string, bool, error) {
	return "", false, nil
}

// Set 设置缓存内容（什么都不做）
func (n *NoopCache) Set(key string, value string, ttl time.Duration) error {
	return nil
}

// Delete 删除缓存项（什么都不做）
func (n *NoopCache) Delete(key string) error {
	return nil
}

// Clear 清空所有缓存（什么都不做）
func (n *NoopCache) Clear() error {
	return nil
}
