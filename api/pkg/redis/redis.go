package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

var (
	RedisClient *redis.Client
)

func Init(cfg *config.Config) error {
	opt := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		MinIdleConns: cfg.Redis.MinIdleConns,
	}

	RedisClient = redis.NewClient(opt)
	if observability.RedisEnabled() {
		if err := redisotel.InstrumentTracing(RedisClient, redisotel.WithDBStatement(false)); err != nil {
			logger.Warn("failed to register opentelemetry Redis tracing", zap.Error(err))
		} else {
			logger.Info("opentelemetry Redis tracing registered successfully")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	ExampleSet("testredis", "1111", 10*time.Second)
	exampleGet, err := ExampleGet("testredis")

	if err != nil {
		logger.Warn("Redis example read failed", zap.Error(err))
	} else {
		logger.Debug("Redis example read completed", zap.Bool("value_present", exampleGet != ""))
	}

	RedisClient.SetEx(ctx, "testredis", "1111", 10*time.Second)

	return nil
}

func Close() error {
	if RedisClient != nil {
		return RedisClient.Close()
	}
	return nil
}

func GetClient() *redis.Client {
	return RedisClient
}

// SetClient sets the Redis client (mainly for testing)
func SetClient(client *redis.Client) {
	RedisClient = client
}

func GetString(ctx context.Context, key string) (string, error) {
	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func SetEx(ctx context.Context, key, value string, expiration time.Duration) error {
	return RedisClient.SetEx(ctx, key, value, expiration).Err()
}

func Incr(ctx context.Context, key string) (int64, error) {
	return RedisClient.Incr(ctx, key).Result()
}

func Expire(ctx context.Context, key string, expiration time.Duration) error {
	return RedisClient.Expire(ctx, key, expiration).Err()
}

func GetInt(ctx context.Context, key string) (int, error) {
	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	intVal, convErr := strconv.Atoi(val)
	if convErr != nil {
		return 0, convErr
	}
	return intVal, nil
}

func Exists(ctx context.Context, key string) (bool, error) {
	count, err := RedisClient.Exists(ctx, key).Result()
	return count > 0, err
}

// DeleteKeysByPrefix deletes all keys matching a given prefix
func DeleteKeysByPrefix(ctx context.Context, prefix string) error {
	iter := RedisClient.Scan(ctx, 0, prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		err := RedisClient.Del(ctx, iter.Val()).Err()
		if err != nil {
			return err
		}
	}
	return iter.Err()
}

// GetKeysByPrefix gets all keys matching a given prefix
func GetKeysByPrefix(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	iter := RedisClient.Scan(ctx, 0, prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}
