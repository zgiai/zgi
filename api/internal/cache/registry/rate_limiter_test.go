package registry

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/cache/keys"
)

func TestRedisRateLimiter(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer client.Close()

	limiter := NewRedisRateLimiter(client, keys.DefaultBuilder())
	ctx := context.Background()

	allowed, retryAfter, err := limiter.Allow(ctx, "module:llm.models:global:tester", time.Minute)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed || retryAfter != 0 {
		t.Fatalf("first Allow() = (%v, %s), want (true, 0)", allowed, retryAfter)
	}

	allowed, retryAfter, err = limiter.Allow(ctx, "module:llm.models:global:tester", time.Minute)
	if err != nil {
		t.Fatalf("Allow() second error = %v", err)
	}
	if allowed {
		t.Fatal("second Allow() allowed = true, want false")
	}
	if retryAfter <= 0 {
		t.Fatalf("second Allow() retryAfter = %s, want > 0", retryAfter)
	}

	server.FastForward(time.Minute)

	allowed, retryAfter, err = limiter.Allow(ctx, "module:llm.models:global:tester", time.Minute)
	if err != nil {
		t.Fatalf("Allow() after expiry error = %v", err)
	}
	if !allowed || retryAfter != 0 {
		t.Fatalf("Allow() after expiry = (%v, %s), want (true, 0)", allowed, retryAfter)
	}
}

func TestMemoryRateLimiterExpiresAndPrunesKeys(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	ctx := context.Background()

	allowed, _, err := limiter.Allow(ctx, "a", time.Minute)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !allowed {
		t.Fatal("first Allow() allowed = false, want true")
	}

	allowed, retryAfter, err := limiter.Allow(ctx, "a", time.Minute)
	if err != nil {
		t.Fatalf("second Allow() error = %v", err)
	}
	if allowed || retryAfter != time.Minute {
		t.Fatalf("second Allow() = (%v, %s), want (false, %s)", allowed, retryAfter, time.Minute)
	}

	now = now.Add(time.Minute)
	allowed, retryAfter, err = limiter.Allow(ctx, "b", time.Minute)
	if err != nil {
		t.Fatalf("Allow() after expiry error = %v", err)
	}
	if !allowed || retryAfter != 0 {
		t.Fatalf("Allow() after expiry = (%v, %s), want (true, 0)", allowed, retryAfter)
	}
	if _, ok := limiter.ttl["a"]; ok {
		t.Fatal("expired key was not pruned")
	}
}
