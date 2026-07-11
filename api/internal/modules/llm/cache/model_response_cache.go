package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zgiai/zgi/api/internal/cache/keys"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

const (
	modulePrefix = "llm.model_response"
	entryTTL     = 30 * time.Second
	opTimeout    = 10 * time.Millisecond
)

func Generation(ctx context.Context, organizationID string) string {
	client := redisutil.GetClient()
	if client == nil || organizationID == "" {
		return "0"
	}
	cacheCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	value, err := client.Get(cacheCtx, generationKey(organizationID)).Result()
	if err != nil || value == "" {
		return "0"
	}
	return value
}

func Invalidate(ctx context.Context, organizationID string) {
	client := redisutil.GetClient()
	if client == nil || organizationID == "" {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_, _ = client.Incr(cacheCtx, generationKey(organizationID)).Result()
}

func GlobalGeneration(ctx context.Context) string {
	return Generation(ctx, "global")
}

func InvalidateGlobal(ctx context.Context) {
	Invalidate(ctx, "global")
}

func GetJSON(ctx context.Context, kind, organizationID, generation string, parts []string, value any) bool {
	client := redisutil.GetClient()
	if client == nil || organizationID == "" {
		return false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	payload, err := client.Get(cacheCtx, responseKey(kind, organizationID, generation, parts...)).Bytes()
	return err == nil && json.Unmarshal(payload, value) == nil
}

func SetJSON(ctx context.Context, kind, organizationID, generation string, parts []string, value any) {
	client := redisutil.GetClient()
	if client == nil || organizationID == "" || value == nil {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_ = client.SetEx(cacheCtx, responseKey(kind, organizationID, generation, parts...), payload, entryTTL).Err()
}

func generationKey(organizationID string) string {
	return keys.DefaultBuilder().Build(modulePrefix, "generation", organizationID)
}

func responseKey(kind, organizationID, generation string, parts ...string) string {
	return keys.DefaultBuilder().Build(modulePrefix, append([]string{kind, organizationID, generation}, parts...)...)
}
