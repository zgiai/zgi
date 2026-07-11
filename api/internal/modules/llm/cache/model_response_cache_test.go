package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestResponseCacheUsesGenerationForInvalidation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() { redisutil.SetClient(previous) })

	ctx := context.Background()
	organizationID := "org-1"
	generation := Generation(ctx, organizationID)
	SetJSON(ctx, "default", organizationID, generation, nil, map[string]string{"model": "one"})

	var first map[string]string
	if !GetJSON(ctx, "default", organizationID, generation, nil, &first) || first["model"] != "one" {
		t.Fatalf("initial cached response = %#v", first)
	}

	Invalidate(ctx, organizationID)
	var stale map[string]string
	if GetJSON(ctx, "default", organizationID, Generation(ctx, organizationID), nil, &stale) {
		t.Fatalf("stale response remained readable: %#v", stale)
	}
}

func TestFillContextSurvivesCallerCancellation(t *testing.T) {
	callerCtx, cancelCaller := context.WithCancel(context.Background())
	fillCtx, cancelFill := FillContext(callerCtx)
	defer cancelFill()

	cancelCaller()
	select {
	case <-fillCtx.Done():
		t.Fatalf("fill context was canceled with caller: %v", fillCtx.Err())
	default:
	}
}

func TestGenerationReadFailureDisablesResponseCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() { redisutil.SetClient(previous) })

	ctx := context.Background()
	SetJSON(ctx, "default", "org-1", "0", nil, map[string]string{"model": "stale"})
	if err := client.Close(); err != nil {
		t.Fatalf("close Redis client: %v", err)
	}

	if generation := Generation(ctx, "org-1"); generation != "" {
		t.Fatalf("generation after Redis failure = %q, want empty", generation)
	}
	var cached map[string]string
	if GetJSON(ctx, "default", "org-1", "", nil, &cached) {
		t.Fatalf("response cache remained enabled after generation failure: %#v", cached)
	}
}
