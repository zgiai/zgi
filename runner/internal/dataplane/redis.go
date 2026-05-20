package dataplane

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"plugin_runner/internal/config"
)

func initRedis(cfg *config.Config, log *zap.Logger) (redis.UniversalClient, error) {
	if cfg.RedisSentinelEnabled {
		return initRedisSentinel(cfg, log)
	}
	return initRedisSingle(cfg, log)
}

func initRedisSingle(cfg *config.Config, log *zap.Logger) (redis.UniversalClient, error) {
	address := fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort)
	opts := &redis.Options{
		Addr:         address,
		Username:     cfg.RedisUsername,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	}

	if cfg.RedisUseTLS {
		opts.TLSConfig = &tls.Config{}
	}

	client := redis.NewClient(opts)
	if err := pingRedis(client, cfg.RedisDialTimeout); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	log.Info("redis connected",
		zap.Bool("sentinel", false),
		zap.String("host", cfg.RedisHost),
		zap.Int("port", cfg.RedisPort),
		zap.Int("db", cfg.RedisDB),
	)
	return client, nil
}

func initRedisSentinel(cfg *config.Config, log *zap.Logger) (redis.UniversalClient, error) {
	sentinels := splitAndTrim(cfg.RedisSentinelAddrs)
	opts := &redis.FailoverOptions{
		MasterName:       cfg.RedisSentinelMaster,
		SentinelAddrs:    sentinels,
		Username:         cfg.RedisUsername,
		Password:         cfg.RedisPassword,
		DB:               cfg.RedisDB,
		SentinelUsername: cfg.RedisSentinelUsername,
		SentinelPassword: cfg.RedisSentinelPassword,
		DialTimeout:      cfg.RedisDialTimeout,
		ReadTimeout:      cfg.RedisReadTimeout,
		WriteTimeout:     cfg.RedisWriteTimeout,
	}
	if cfg.RedisUseTLS {
		opts.TLSConfig = &tls.Config{}
	}

	client := redis.NewFailoverClient(opts)
	if err := pingRedis(client, cfg.RedisDialTimeout); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis sentinel: %w", err)
	}

	log.Info("redis connected",
		zap.Bool("sentinel", true),
		zap.Strings("sentinel_addrs", sentinels),
		zap.String("master_name", cfg.RedisSentinelMaster),
		zap.Int("db", cfg.RedisDB),
	)
	return client, nil
}

func pingRedis(client redis.UniversalClient, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.Ping(ctx).Err()
}

func splitAndTrim(value string) []string {
	raw := strings.Split(value, ",")
	var items []string
	for _, v := range raw {
		item := strings.TrimSpace(v)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
