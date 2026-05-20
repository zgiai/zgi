package redis

import (
	"context"
	"time"
)

func ExampleSet(key string, value interface{}, expiration time.Duration) error {
	ctx := context.Background()
	return RedisClient.Set(ctx, key, value, expiration).Err()
}

func ExampleGet(key string) (string, error) {
	ctx := context.Background()
	return RedisClient.Get(ctx, key).Result()
}

func ExampleDel(key string) error {
	ctx := context.Background()
	return RedisClient.Del(ctx, key).Err()
}

func ExampleExists(key string) (bool, error) {
	ctx := context.Background()
	result, err := RedisClient.Exists(ctx, key).Result()
	return result > 0, err
}

func ExampleExpire(key string, expiration time.Duration) (bool, error) {
	ctx := context.Background()
	return RedisClient.Expire(ctx, key, expiration).Result()
}

func ExampleHSet(key string, field string, value interface{}) error {
	ctx := context.Background()
	return RedisClient.HSet(ctx, key, field, value).Err()
}

func ExampleHGet(key string, field string) (string, error) {
	ctx := context.Background()
	return RedisClient.HGet(ctx, key, field).Result()
}

func ExampleLPush(key string, values ...interface{}) error {
	ctx := context.Background()
	return RedisClient.LPush(ctx, key, values...).Err()
}

func ExampleLRange(key string, start, stop int64) ([]string, error) {
	ctx := context.Background()
	return RedisClient.LRange(ctx, key, start, stop).Result()
}
