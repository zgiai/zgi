package registry

import (
	"context"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/cache/keys"
)

type RedisRateLimiter struct {
	Client     *goredis.Client
	KeyBuilder keys.Builder
}

func NewRedisRateLimiter(client *goredis.Client, builder keys.Builder) *RedisRateLimiter {
	if builder.GlobalPrefix() == "" {
		builder = keys.DefaultBuilder()
	}
	return &RedisRateLimiter{Client: client, KeyBuilder: builder}
}

func (l *RedisRateLimiter) Allow(ctx context.Context, key string, interval time.Duration) (bool, time.Duration, error) {
	if l == nil || l.Client == nil || interval <= 0 {
		return true, 0, nil
	}

	cacheKey := l.KeyBuilder.Build("refresh.limit", key)
	allowed, err := l.Client.SetNX(ctx, cacheKey, time.Now().Unix(), interval).Result()
	if err != nil {
		return false, 0, err
	}
	if allowed {
		return true, 0, nil
	}

	ttl, err := l.Client.TTL(ctx, cacheKey).Result()
	if err != nil {
		return false, 0, err
	}
	switch ttl {
	case -2 * time.Second:
		return l.Allow(ctx, key, interval)
	case -1 * time.Second:
		if err := l.Client.Expire(ctx, cacheKey, interval).Err(); err != nil {
			return false, 0, err
		}
		ttl = interval
	}
	if ttl < 0 {
		return true, 0, nil
	}
	return false, ttl, nil
}

type MemoryRateLimiter struct {
	now func() time.Time
	mu  sync.Mutex
	ttl map[string]time.Time
}

func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{
		now: time.Now,
		ttl: make(map[string]time.Time),
	}
}

func (l *MemoryRateLimiter) Allow(ctx context.Context, key string, interval time.Duration) (bool, time.Duration, error) {
	_ = ctx
	if l == nil || interval <= 0 {
		return true, 0, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for existingKey, expiresAt := range l.ttl {
		if !expiresAt.After(now) {
			delete(l.ttl, existingKey)
		}
	}
	next := l.ttl[key]
	if !next.IsZero() && now.Before(next) {
		return false, next.Sub(now), nil
	}
	l.ttl[key] = now.Add(interval)
	return true, 0, nil
}
