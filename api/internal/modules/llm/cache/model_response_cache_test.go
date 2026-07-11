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
