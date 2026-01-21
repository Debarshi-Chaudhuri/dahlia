package redis

import (
	"context"
	"testing"
	"time"

	cache "dahlia/internal/cache/iface"
	"dahlia/internal/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCache(t *testing.T) cache.Cache {
	log, err := logger.NewZapLoggerForDev()
	require.NoError(t, err)
	cache, err := NewRedisCache("localhost:6379", "", 0, log)
	require.NoError(t, err)
	return cache
}

func TestBasicOperations(t *testing.T) {
	cache := setupCache(t)
	defer cache.Close()

	ctx := context.Background()

	// Test Set and Get
	t.Run("Set and Get", func(t *testing.T) {
		key := "test:basic:key1"
		value := "test-value"

		err := cache.Set(ctx, key, value, 0)
		require.NoError(t, err)

		result, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)

		// Cleanup
		cache.Delete(ctx, key)
	})

	// Test Set with TTL
	t.Run("Set with TTL", func(t *testing.T) {
		key := "test:basic:key2"
		value := "test-value-with-ttl"

		err := cache.Set(ctx, key, value, 2*time.Second)
		require.NoError(t, err)

		// Should exist immediately
		result, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)

		// Wait for expiry
		time.Sleep(3 * time.Second)

		// Should not exist
		_, err = cache.Get(ctx, key)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "key not found")
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		key := "test:basic:key3"
		value := "test-value-delete"

		err := cache.Set(ctx, key, value, 0)
		require.NoError(t, err)

		err = cache.Delete(ctx, key)
		require.NoError(t, err)

		_, err = cache.Get(ctx, key)
		assert.Error(t, err)
	})
}

func TestListOperations(t *testing.T) {
	cache := setupCache(t)
	defer cache.Close()

	ctx := context.Background()
	key := "test:list:bucket1"

	// Cleanup before test
	defer cache.Delete(ctx, key)

	// Test RPush
	t.Run("RPush", func(t *testing.T) {
		jobIDs := []interface{}{"job1", "job2", "job3"}
		err := cache.RPush(ctx, key, jobIDs...)
		require.NoError(t, err)
	})

	// Test LLen
	t.Run("LLen", func(t *testing.T) {
		length, err := cache.LLen(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, int64(3), length)
	})

	// Test LRange
	t.Run("LRange", func(t *testing.T) {
		results, err := cache.LRange(ctx, key, 0, -1)
		require.NoError(t, err)
		assert.Equal(t, 3, len(results))
		assert.Equal(t, "job1", results[0])
		assert.Equal(t, "job2", results[1])
		assert.Equal(t, "job3", results[2])
	})

	// Test LRem
	t.Run("LRem", func(t *testing.T) {
		err := cache.LRem(ctx, key, 1, "job2")
		require.NoError(t, err)

		results, err := cache.LRange(ctx, key, 0, -1)
		require.NoError(t, err)
		assert.Equal(t, 2, len(results))
		assert.Equal(t, "job1", results[0])
		assert.Equal(t, "job3", results[1])
	})
}

func TestAtomicBucketTransfer(t *testing.T) {
	cache := setupCache(t)
	defer cache.Close()

	ctx := context.Background()

	sourceKey := "test:bucket:2026:01:21:10:00:NODE1"
	tempKey := "test:bucket:2026:01:21:10:00:NODE1_TEMP"

	// Cleanup
	defer cache.Delete(ctx, sourceKey)
	defer cache.Delete(ctx, tempKey)

	// Setup: Add job IDs to source bucket
	jobIDs := []interface{}{"job1", "job2", "job3"}
	err := cache.RPush(ctx, sourceKey, jobIDs...)
	require.NoError(t, err)

	// Lua script for atomic transfer (pop from source, push to temp)
	luaScript := `
		local jobs = redis.call('LRANGE', KEYS[1], 0, -1)
		redis.call('DEL', KEYS[1])
		if #jobs > 0 then
			redis.call('RPUSH', KEYS[2], unpack(jobs))
		end
		return jobs
	`

	// Execute atomic transfer
	result, err := cache.Eval(ctx, luaScript, []string{sourceKey, tempKey})
	require.NoError(t, err)

	// Verify result
	jobsTransferred := result.([]interface{})
	assert.Equal(t, 3, len(jobsTransferred))

	// Verify source bucket is empty
	sourceLength, err := cache.LLen(ctx, sourceKey)
	require.NoError(t, err)
	assert.Equal(t, int64(0), sourceLength)

	// Verify temp bucket has all jobs
	tempJobs, err := cache.LRange(ctx, tempKey, 0, -1)
	require.NoError(t, err)
	assert.Equal(t, 3, len(tempJobs))
	assert.Equal(t, "job1", tempJobs[0])
	assert.Equal(t, "job2", tempJobs[1])
	assert.Equal(t, "job3", tempJobs[2])
}

func TestSchedulerWorkflow(t *testing.T) {
	cache := setupCache(t)
	defer cache.Close()

	ctx := context.Background()

	// Simulate scheduler workflow
	bucketKey := "test:scheduler:2026:01:21:10:15:NODE1"
	tempKey := "test:scheduler:2026:01:21:10:15:NODE1_TEMP"

	// Cleanup
	defer cache.Delete(ctx, bucketKey)
	defer cache.Delete(ctx, tempKey)

	t.Run("Schedule jobs to bucket", func(t *testing.T) {
		// Simulate scheduling 3 jobs
		err := cache.RPush(ctx, bucketKey, "job-abc-123")
		require.NoError(t, err)

		err = cache.RPush(ctx, bucketKey, "job-def-456")
		require.NoError(t, err)

		err = cache.RPush(ctx, bucketKey, "job-ghi-789")
		require.NoError(t, err)

		// Verify count
		length, err := cache.LLen(ctx, bucketKey)
		require.NoError(t, err)
		assert.Equal(t, int64(3), length)
	})

	t.Run("Atomic transfer to TEMP bucket", func(t *testing.T) {
		luaScript := `
			local jobs = redis.call('LRANGE', KEYS[1], 0, -1)
			redis.call('DEL', KEYS[1])
			if #jobs > 0 then
				redis.call('RPUSH', KEYS[2], unpack(jobs))
			end
			return jobs
		`

		result, err := cache.Eval(ctx, luaScript, []string{bucketKey, tempKey})
		require.NoError(t, err)

		jobs := result.([]interface{})
		assert.Equal(t, 3, len(jobs))
	})

	t.Run("Process and remove from TEMP", func(t *testing.T) {
		// Get all jobs from temp
		jobs, err := cache.LRange(ctx, tempKey, 0, -1)
		require.NoError(t, err)
		assert.Equal(t, 3, len(jobs))

		// Simulate processing: remove each job after processing
		for _, job := range jobs {
			err := cache.LRem(ctx, tempKey, 1, job)
			require.NoError(t, err)
		}

		// Verify TEMP bucket is empty
		length, err := cache.LLen(ctx, tempKey)
		require.NoError(t, err)
		assert.Equal(t, int64(0), length)
	})
}

func TestMultipleNodeBuckets(t *testing.T) {
	cache := setupCache(t)
	defer cache.Close()

	ctx := context.Background()

	// Simulate multiple nodes
	node1Bucket := "test:multi:2026:01:21:11:00:NODE1"
	node2Bucket := "test:multi:2026:01:21:11:00:NODE2"
	node3Bucket := "test:multi:2026:01:21:11:00:NODE3"

	// Cleanup
	defer cache.Delete(ctx, node1Bucket)
	defer cache.Delete(ctx, node2Bucket)
	defer cache.Delete(ctx, node3Bucket)

	// Each node schedules jobs to its own bucket
	err := cache.RPush(ctx, node1Bucket, "node1-job1", "node1-job2")
	require.NoError(t, err)

	err = cache.RPush(ctx, node2Bucket, "node2-job1", "node2-job2")
	require.NoError(t, err)

	err = cache.RPush(ctx, node3Bucket, "node3-job1")
	require.NoError(t, err)

	// Verify each bucket is isolated
	node1Jobs, err := cache.LRange(ctx, node1Bucket, 0, -1)
	require.NoError(t, err)
	assert.Equal(t, 2, len(node1Jobs))

	node2Jobs, err := cache.LRange(ctx, node2Bucket, 0, -1)
	require.NoError(t, err)
	assert.Equal(t, 2, len(node2Jobs))

	node3Jobs, err := cache.LRange(ctx, node3Bucket, 0, -1)
	require.NoError(t, err)
	assert.Equal(t, 1, len(node3Jobs))
}
