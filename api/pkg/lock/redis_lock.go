package lock

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DefaultLockTTL is the default lock expiration time (10 minutes)
const DefaultLockTTL = 10 * time.Minute

// ErrLockNotHeld is returned when trying to release a lock that is not held
var ErrLockNotHeld = errors.New("lock not held")

// ErrLockFailed is returned when unable to acquire the lock
var ErrLockFailed = errors.New("failed to acquire lock")

// RedisLock represents a distributed lock based on Redis
type RedisLock struct {
	client *redis.Client
	key    string
	value  string // Unique identifier for safe release
	ttl    time.Duration
}

// NewRedisLock creates a new distributed lock instance
// key: The lock key (e.g., "gf:kb:uuid:align")
// ttl: Lock expiration time, use DefaultLockTTL for 10 minutes
func NewRedisLock(client *redis.Client, key string, ttl time.Duration) *RedisLock {
	return &RedisLock{
		client: client,
		key:    key,
		value:  uuid.New().String(), // Unique value for ownership verification
		ttl:    ttl,
	}
}

// Acquire attempts to acquire the lock
// Returns true if the lock was acquired, false otherwise
func (l *RedisLock) Acquire(ctx context.Context) (bool, error) {
	result, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	if err != nil {
		return false, err
	}
	return result, nil
}

// AcquireWithRetry attempts to acquire the lock with retries
// retryInterval: Time to wait between retry attempts
// maxRetries: Maximum number of retry attempts
func (l *RedisLock) AcquireWithRetry(ctx context.Context, retryInterval time.Duration, maxRetries int) (bool, error) {
	for i := 0; i <= maxRetries; i++ {
		acquired, err := l.Acquire(ctx)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}

		if i < maxRetries {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}
	}
	return false, nil
}

// Release releases the lock only if we still own it
// Uses Lua script to ensure atomic check-and-delete
func (l *RedisLock) Release(ctx context.Context) error {
	// Lua script: Only delete if the value matches (we own the lock)
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		end
		return 0
	`)

	result, err := script.Run(ctx, l.client, []string{l.key}, l.value).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// Extend extends the lock TTL if we still own it
// Returns true if the extension was successful
func (l *RedisLock) Extend(ctx context.Context, ttl time.Duration) (bool, error) {
	// Lua script: Only extend if we own the lock
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		end
		return 0
	`)

	result, err := script.Run(ctx, l.client, []string{l.key}, l.value, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// IsHeld checks if we currently hold the lock
func (l *RedisLock) IsHeld(ctx context.Context) (bool, error) {
	val, err := l.client.Get(ctx, l.key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == l.value, nil
}

// Key returns the lock key
func (l *RedisLock) Key() string {
	return l.key
}

// GraphFlowLockKey generates a lock key for GraphFlow operations
// Example: gf:kb:123e4567-e89b-12d3-a456-426614174000:align
func GraphFlowLockKey(kbID string, operation string) string {
	return "gf:kb:" + kbID + ":" + operation
}

// LockOperation constants for GraphFlow
const (
	LockOpAlignment = "align"
	LockOpSync      = "sync"
	LockOpCleanup   = "cleanup"
)
