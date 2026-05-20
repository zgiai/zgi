package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// KBLimiter implements a Token Bucket rate limiter for KB-level throttling
type KBLimiter struct {
	client     *redis.Client
	maxTokens  int           // Maximum tokens in the bucket (max concurrent tasks)
	refillRate int           // Tokens to add per refill interval
	refillUnit time.Duration // Refill interval
	keyPrefix  string        // Redis key prefix
}

// DefaultKBLimiterConfig provides default configuration
const (
	DefaultMaxConcurrentTasks = 5              // Max 5 concurrent tasks per KB
	DefaultRefillRate         = 2              // Refill 2 tokens per interval
	DefaultRefillUnit         = time.Second    // Refill every second
	DefaultKeyPrefix          = "gf:ratelimit" // Redis key prefix
)

// KBLimiterConfig holds configuration for the KB limiter
type KBLimiterConfig struct {
	MaxConcurrentTasks int
	RefillRate         int
	RefillUnit         time.Duration
	KeyPrefix          string
}

// NewKBLimiter creates a new KB rate limiter with default configuration
func NewKBLimiter(client *redis.Client) *KBLimiter {
	return NewKBLimiterWithConfig(client, KBLimiterConfig{
		MaxConcurrentTasks: DefaultMaxConcurrentTasks,
		RefillRate:         DefaultRefillRate,
		RefillUnit:         DefaultRefillUnit,
		KeyPrefix:          DefaultKeyPrefix,
	})
}

// NewKBLimiterWithConfig creates a new KB rate limiter with custom configuration
func NewKBLimiterWithConfig(client *redis.Client, cfg KBLimiterConfig) *KBLimiter {
	if cfg.MaxConcurrentTasks <= 0 {
		cfg.MaxConcurrentTasks = DefaultMaxConcurrentTasks
	}
	if cfg.RefillRate <= 0 {
		cfg.RefillRate = DefaultRefillRate
	}
	if cfg.RefillUnit <= 0 {
		cfg.RefillUnit = DefaultRefillUnit
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = DefaultKeyPrefix
	}

	return &KBLimiter{
		client:     client,
		maxTokens:  cfg.MaxConcurrentTasks,
		refillRate: cfg.RefillRate,
		refillUnit: cfg.RefillUnit,
		keyPrefix:  cfg.KeyPrefix,
	}
}

// Allow checks if a task can proceed for the given KB
// Returns true if allowed, false if rate limited
func (l *KBLimiter) Allow(ctx context.Context, kbID string) (bool, error) {
	key := l.getKey(kbID)

	// Lua script for Token Bucket algorithm
	// KEYS[1] = bucket key
	// ARGV[1] = max tokens
	// ARGV[2] = refill rate (tokens per interval)
	// ARGV[3] = refill interval (milliseconds)
	// ARGV[4] = current timestamp (milliseconds)
	// ARGV[5] = tokens requested (always 1)
	script := redis.NewScript(`
		local key = KEYS[1]
		local max_tokens = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local refill_interval_ms = tonumber(ARGV[3])
		local now = tonumber(ARGV[4])
		local requested = tonumber(ARGV[5])

		-- Get current bucket state
		local bucket = redis.call("HMGET", key, "tokens", "last_refill")
		local tokens = tonumber(bucket[1])
		local last_refill = tonumber(bucket[2])

		-- Initialize bucket if not exists
		if tokens == nil then
			tokens = max_tokens
			last_refill = now
		end

		-- Calculate tokens to add based on time elapsed
		local elapsed_ms = now - last_refill
		local intervals = math.floor(elapsed_ms / refill_interval_ms)
		if intervals > 0 then
			tokens = math.min(max_tokens, tokens + (intervals * refill_rate))
			last_refill = last_refill + (intervals * refill_interval_ms)
		end

		-- Check if we have enough tokens
		if tokens >= requested then
			tokens = tokens - requested
			redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
			redis.call("EXPIRE", key, 3600) -- Expire after 1 hour of inactivity
			return 1
		else
			-- Update state even if denied (for accurate refill timing)
			redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
			redis.call("EXPIRE", key, 3600)
			return 0
		end
	`)

	now := time.Now().UnixMilli()
	result, err := script.Run(ctx, l.client, []string{key},
		l.maxTokens,
		l.refillRate,
		l.refillUnit.Milliseconds(),
		now,
		1,
	).Int64()

	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// Release returns a token to the bucket (call when task completes)
func (l *KBLimiter) Release(ctx context.Context, kbID string) error {
	key := l.getKey(kbID)

	// Lua script to return a token, capped at max
	script := redis.NewScript(`
		local key = KEYS[1]
		local max_tokens = tonumber(ARGV[1])
		
		local tokens = tonumber(redis.call("HGET", key, "tokens")) or max_tokens
		tokens = math.min(max_tokens, tokens + 1)
		redis.call("HSET", key, "tokens", tokens)
		redis.call("EXPIRE", key, 3600)
		return tokens
	`)

	_, err := script.Run(ctx, l.client, []string{key}, l.maxTokens).Result()
	return err
}

// GetAvailableTokens returns the current number of available tokens for a KB
func (l *KBLimiter) GetAvailableTokens(ctx context.Context, kbID string) (int, error) {
	key := l.getKey(kbID)
	result, err := l.client.HGet(ctx, key, "tokens").Int()
	if err == redis.Nil {
		return l.maxTokens, nil
	}
	return result, err
}

// Reset resets the rate limiter for a KB (useful for testing)
func (l *KBLimiter) Reset(ctx context.Context, kbID string) error {
	key := l.getKey(kbID)
	return l.client.Del(ctx, key).Err()
}

// getKey generates the Redis key for a KB
func (l *KBLimiter) getKey(kbID string) string {
	return fmt.Sprintf("%s:kb:%s", l.keyPrefix, kbID)
}

// WaitForToken blocks until a token is available or context is cancelled
// pollInterval: How often to check for available tokens
func (l *KBLimiter) WaitForToken(ctx context.Context, kbID string, pollInterval time.Duration) error {
	for {
		allowed, err := l.Allow(ctx, kbID)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			continue
		}
	}
}
