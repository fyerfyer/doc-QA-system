package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache 基于Redis实现的缓存
type RedisCache struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisCache 创建一个新的Redis缓存
func NewRedisCache(config Config) (Cache, error) {
	// 配置Redis客户端
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// 测试连接
	ctx := context.Background()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return &RedisCache{
		client: client,
		ctx:    ctx,
	}, nil
}

// Get 获取缓存内容
func (r *RedisCache) Get(key string) (string, bool, error) {
	value, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		// 键不存在
		return "", false, nil
	} else if err != nil {
		// 其他错误
		return "", false, err
	}

	return value, true, nil
}

// Set 设置缓存内容
func (r *RedisCache) Set(key string, value string, ttl time.Duration) error {
	return r.client.Set(r.ctx, key, value, ttl).Err()
}

// Delete 删除缓存项
func (r *RedisCache) Delete(key string) error {
	return r.client.Del(r.ctx, key).Err()
}

// Clear 清空所有缓存
// 注意：这会清空整个Redis数据库，谨慎使用
func (r *RedisCache) Clear() error {
	return r.client.FlushDB(r.ctx).Err()
}

// 在包初始化时注册Redis缓存
func init() {
	RegisterCache("redis", NewRedisCache)
}
