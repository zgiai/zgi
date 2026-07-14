package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	llmcache "github.com/zgiai/zgi/api/internal/modules/llm/cache"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestModelCachePreservesInternalCapabilities(t *testing.T) {
	model := &llmmodel.LLMModel{
		Provider:          "openai",
		Model:             "gpt-5",
		Responses:         true,
		ChatCompletions:   true,
		SupportsStreaming: true,
		SupportsToolCall:  true,
		SystemPrompt:      true,
	}

	data, err := marshalCachedModel(model)
	if err != nil {
		t.Fatalf("marshalCachedModel() error = %v", err)
	}

	var got llmmodel.LLMModel
	if err := unmarshalCachedModel(data, &got); err != nil {
		t.Fatalf("unmarshalCachedModel() error = %v", err)
	}
	if !got.Responses {
		t.Fatal("Responses = false, want true")
	}
	if !got.ChatCompletions {
		t.Fatal("ChatCompletions = false, want true")
	}
	if !got.SupportsStreaming {
		t.Fatal("SupportsStreaming = false, want true")
	}
	if !got.SupportsToolCall {
		t.Fatal("SupportsToolCall = false, want true")
	}
}

func TestModelCacheRejectsLegacyJSONWithoutInternalCapabilities(t *testing.T) {
	legacy, err := json.Marshal(&llmmodel.LLMModel{
		Provider:  "openai",
		Model:     "gpt-5",
		Responses: true,
	})
	if err != nil {
		t.Fatalf("marshal legacy model: %v", err)
	}

	var got llmmodel.LLMModel
	if err := unmarshalCachedModel(legacy, &got); err == nil {
		t.Fatal("unmarshalCachedModel() error = nil, want legacy cache rejection")
	}
}

func TestConfigCacheInvalidateModelCacheDeletesOnlyModelKeys(t *testing.T) {
	ctx := context.Background()
	redisClient := newGatewayTestRedis(t)
	previousRedisClient := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	t.Cleanup(func() { redisutil.SetClient(previousRedisClient) })
	cache := NewConfigCache(redisClient, nil, &ConfigCacheConfig{
		ModelTTL:    time.Minute,
		ProviderTTL: time.Minute,
	})
	keys := map[string]string{
		cache.prefix + "model:name:gpt-4o":    "model-name",
		cache.prefix + "model:id:model-id":    "model-id",
		cache.prefix + "provider:name:openai": "provider",
		cache.prefix + "shadow:tenant-id":     "shadow",
		"unrelated:model:name:gpt-4o":         "unrelated",
	}
	for key, value := range keys {
		if err := redisClient.Set(ctx, key, value, time.Minute).Err(); err != nil {
			t.Fatalf("seed key %s: %v", key, err)
		}
	}

	globalGenerationBefore := llmcache.GlobalGeneration(ctx)
	cache.InvalidateModelCache(ctx)
	if globalGenerationAfter := llmcache.GlobalGeneration(ctx); globalGenerationAfter == globalGenerationBefore {
		t.Fatalf("global available-model generation = %q, want change from %q", globalGenerationAfter, globalGenerationBefore)
	}

	for _, key := range []string{
		cache.prefix + "model:name:gpt-4o",
		cache.prefix + "model:id:model-id",
	} {
		if err := redisClient.Get(ctx, key).Err(); err != redis.Nil {
			t.Fatalf("model key %s still exists or returned unexpected error: %v", key, err)
		}
	}
	for _, key := range []string{
		cache.prefix + "provider:name:openai",
		cache.prefix + "shadow:tenant-id",
		"unrelated:model:name:gpt-4o",
	} {
		if got, err := redisClient.Get(ctx, key).Result(); err != nil || got != keys[key] {
			t.Fatalf("non-model key %s = %q, %v; want %q, nil", key, got, err, keys[key])
		}
	}
}
