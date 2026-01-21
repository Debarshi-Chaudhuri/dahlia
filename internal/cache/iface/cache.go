package cache

import (
	"context"
	"time"
)

// Cache defines the interface for cache operations (Redis)
type Cache interface {
	// Basic operations
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error

	// List operations (for scheduler minute buckets)
	RPush(ctx context.Context, key string, values ...interface{}) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	LRem(ctx context.Context, key string, count int64, value interface{}) error
	LLen(ctx context.Context, key string) (int64, error)

	// Script execution (for atomic bucket transfers)
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)

	// Close connection
	Close() error
}
