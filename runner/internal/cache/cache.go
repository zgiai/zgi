package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps redis.UniversalClient with helpers and a key prefix.
type Client struct {
	redis  redis.UniversalClient
	prefix string
}

func New(redis redis.UniversalClient, prefix string) *Client {
	return &Client{redis: redis, prefix: strings.TrimSuffix(prefix, ":")}
}

func (c *Client) Enabled() bool {
	return c != nil && c.redis != nil
}

// Key prefixes any name with the configured prefix.
func (c *Client) Key(parts ...string) string {
	all := append([]string{c.prefix}, parts...)
	return strings.Join(all, ":")
}

// Set stores a value with TTL.
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	return c.redis.Set(ctx, c.Key(key), value, ttl).Err()
}

// SetJSON marshals value to JSON and stores it.
func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, b, ttl)
}

// GetString retrieves a raw string.
func (c *Client) GetString(ctx context.Context, key string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("cache not configured")
	}
	return c.redis.Get(ctx, c.Key(key)).Result()
}

// GetJSON unmarshals JSON into dst.
func (c *Client) GetJSON(ctx context.Context, key string, dst any) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	val, err := c.redis.Get(ctx, c.Key(key)).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(val, dst)
}

// Del deletes keys.
func (c *Client) Del(ctx context.Context, key string) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	return c.redis.Del(ctx, c.Key(key)).Err()
}

// WithTransaction runs fn within a WATCH/TX pipeline.
func (c *Client) WithTransaction(ctx context.Context, keys []string, fn func(redis.Pipeliner) error) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	var prefixed []string
	for _, k := range keys {
		prefixed = append(prefixed, c.Key(k))
	}
	return c.redis.Watch(ctx, func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, fn)
		return err
	}, prefixed...)
}

// Lock tries to acquire a lock key with TTL, returns unlock func.
func (c *Client) Lock(ctx context.Context, key string, ttl time.Duration) (func() error, error) {
	if !c.Enabled() {
		return nil, errors.New("cache not configured")
	}
	lockKey := c.Key("lock", key)
	backoff := time.Millisecond * 50
	ok, err := c.redis.SetNX(ctx, lockKey, "1", ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		// retry a few times before failing
		for i := 0; i < 3; i++ {
			time.Sleep(backoff)
			backoff *= 2
			ok, err = c.redis.SetNX(ctx, lockKey, "1", ttl).Result()
			if err != nil {
				return nil, err
			}
			if ok {
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("lock %s already held", lockKey)
		}
	}
	return func() error {
		return c.redis.Del(context.Background(), lockKey).Err()
	}, nil
}

// Publish publishes a message with retry.
func (c *Client) Publish(ctx context.Context, channel string, msg any) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ch := c.Key("event", channel)
	const maxRetry = 3
	backoff := 100 * time.Millisecond
	for i := 0; i < maxRetry; i++ {
		if err := c.redis.Publish(ctx, ch, payload).Err(); err == nil {
			return nil
		} else if i == maxRetry-1 {
			return err
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return nil
}

// Subscribe runs handler for each message until ctx cancelled.
func (c *Client) Subscribe(ctx context.Context, channel string, handler func([]byte) error) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	ch := c.Key("event", channel)
	sub := c.redis.Subscribe(ctx, ch)
	defer sub.Close()
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			return err
		}
		if err := handler([]byte(msg.Payload)); err != nil {
			return err
		}
	}
}

// ScanKeys scans keys by pattern under prefix.
func (c *Client) ScanKeys(ctx context.Context, pattern string, fn func(string) error) error {
	if !c.Enabled() {
		return errors.New("cache not configured")
	}
	fullPattern := c.Key(pattern) + "*"
	var cursor uint64
	for {
		keys, next, err := c.redis.Scan(ctx, cursor, fullPattern, 100).Result()
		if err != nil {
			return err
		}
		for _, k := range keys {
			if err := fn(k); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
