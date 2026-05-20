package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

type SandboxCache interface {
	Get(context.Context, string) (*sandbox.Sandbox, bool, error)
	Set(context.Context, sandbox.Sandbox, time.Duration) error
	Delete(context.Context, string) error
}

func NewSandboxCache(cfg config.Config) (SandboxCache, error) {
	if cfg.RedisAddr == "" {
		return newMemorySandboxCache(), nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &redisSandboxCache{
		client: client,
		prefix: "zgi:sandbox:",
	}, nil
}

type memorySandboxCache struct {
	mu    sync.RWMutex
	items map[string]memoryItem
}

type memoryItem struct {
	box       sandbox.Sandbox
	expiresAt time.Time
}

func newMemorySandboxCache() SandboxCache {
	return &memorySandboxCache{
		items: map[string]memoryItem{},
	}
}

func (c *memorySandboxCache) Get(_ context.Context, id string) (*sandbox.Sandbox, bool, error) {
	c.mu.RLock()
	item, ok := c.items[id]
	c.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if !item.expiresAt.IsZero() && time.Now().UTC().After(item.expiresAt) {
		_ = c.Delete(context.Background(), id)
		return nil, false, nil
	}

	copyItem := item.box
	return &copyItem, true, nil
}

func (c *memorySandboxCache) Set(_ context.Context, box sandbox.Sandbox, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item := memoryItem{box: box}
	if ttl > 0 {
		item.expiresAt = time.Now().UTC().Add(ttl)
	}
	c.items[box.ID] = item
	return nil
}

func (c *memorySandboxCache) Delete(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, id)
	return nil
}

type redisSandboxCache struct {
	client *redis.Client
	prefix string
}

func (c *redisSandboxCache) key(id string) string {
	return c.prefix + id
}

func (c *redisSandboxCache) Get(ctx context.Context, id string) (*sandbox.Sandbox, bool, error) {
	payload, err := c.client.Get(ctx, c.key(id)).Bytes()
	switch {
	case err == redis.Nil:
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}

	var box sandbox.Sandbox
	if err := json.Unmarshal(payload, &box); err != nil {
		return nil, false, err
	}
	return &box, true, nil
}

func (c *redisSandboxCache) Set(ctx context.Context, box sandbox.Sandbox, ttl time.Duration) error {
	payload, err := json.Marshal(box)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(box.ID), payload, ttl).Err()
}

func (c *redisSandboxCache) Delete(ctx context.Context, id string) error {
	return c.client.Del(ctx, c.key(id)).Err()
}
