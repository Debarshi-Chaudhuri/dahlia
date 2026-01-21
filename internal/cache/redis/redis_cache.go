package redis

import (
	"context"
	"fmt"
	"time"

	cache "dahlia/internal/cache/iface"
	"dahlia/internal/logger"

	"github.com/redis/go-redis/v9"
)

type redisCache struct {
	client *redis.Client
	logger logger.Logger
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(addr string, password string, db int, log logger.Logger) (cache.Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info("connected to Redis successfully", logger.String("addr", addr))

	return &redisCache{
		client: client,
		logger: log.With(logger.String("component", "redis_cache")),
	}, nil
}

// Set stores a value with optional TTL
func (r *redisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	err := r.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		r.logger.Error("failed to set key",
			logger.String("key", key),
			logger.Error(err))
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// Get retrieves a value by key
func (r *redisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		r.logger.Error("failed to get key",
			logger.String("key", key),
			logger.Error(err))
		return "", fmt.Errorf("redis get failed: %w", err)
	}

	return val, nil
}

// Delete removes a key
func (r *redisCache) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		r.logger.Error("failed to delete key",
			logger.String("key", key),
			logger.Error(err))
		return fmt.Errorf("redis delete failed: %w", err)
	}

	return nil
}

// RPush appends values to a list
func (r *redisCache) RPush(ctx context.Context, key string, values ...interface{}) error {
	err := r.client.RPush(ctx, key, values...).Err()
	if err != nil {
		r.logger.Error("failed to rpush",
			logger.String("key", key),
			logger.Error(err))
		return fmt.Errorf("redis rpush failed: %w", err)
	}

	return nil
}

// LRange returns a range of elements from a list
func (r *redisCache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	vals, err := r.client.LRange(ctx, key, start, stop).Result()
	if err != nil {
		r.logger.Error("failed to lrange",
			logger.String("key", key),
			logger.Error(err))
		return nil, fmt.Errorf("redis lrange failed: %w", err)
	}

	return vals, nil
}

// LRem removes elements from a list
func (r *redisCache) LRem(ctx context.Context, key string, count int64, value interface{}) error {
	err := r.client.LRem(ctx, key, count, value).Err()
	if err != nil {
		r.logger.Error("failed to lrem",
			logger.String("key", key),
			logger.Error(err))
		return fmt.Errorf("redis lrem failed: %w", err)
	}

	return nil
}

// LLen returns the length of a list
func (r *redisCache) LLen(ctx context.Context, key string) (int64, error) {
	length, err := r.client.LLen(ctx, key).Result()
	if err != nil {
		r.logger.Error("failed to llen",
			logger.String("key", key),
			logger.Error(err))
		return 0, fmt.Errorf("redis llen failed: %w", err)
	}

	return length, nil
}

// Eval executes a Lua script (for atomic operations)
func (r *redisCache) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	result, err := r.client.Eval(ctx, script, keys, args...).Result()
	if err != nil {
		r.logger.Error("failed to eval script",
			logger.Int("key_count", len(keys)),
			logger.Error(err))
		return nil, fmt.Errorf("redis eval failed: %w", err)
	}

	return result, nil
}

// Close closes the Redis connection
func (r *redisCache) Close() error {
	return r.client.Close()
}
