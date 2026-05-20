package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/redis"
	"go.uber.org/zap"
)

// cacheRefreshThreshold defines how often to refresh cache in background
const cacheRefreshThreshold = 2 * time.Minute

// activeRequests tracks ongoing fetch operations to prevent duplicate requests
var (
	activeRequests   = make(map[string]chan struct{})
	activeRequestsMu sync.RWMutex
)

// addPrefixToCacheKey adds a unified prefix to all cache keys
func addPrefixToCacheKey(cacheKey string) string {
	return "zgi_cache:" + cacheKey
}

func cacheLogArgs(cacheKey string, fields ...zap.Field) []interface{} {
	hash := sha256.Sum256([]byte(cacheKey))
	args := []interface{}{
		zap.String("cache_key_hash", hex.EncodeToString(hash[:])),
		zap.Int("cache_key_length", len(cacheKey)),
	}
	for _, field := range fields {
		args = append(args, field)
	}
	return args
}

// GetCachedData retrieves data from cache or calls the provided fetch function
// T is the type of data to be cached
func GetCachedData[T any](
	ctx context.Context,
	cacheKey string,
	ttl time.Duration,
	fetchFunc func() (T, error),
	forceRefresh bool,
) (T, error) {
	// Add unified prefix to cache key
	prefixedCacheKey := addPrefixToCacheKey(cacheKey)
	cacheKey = prefixedCacheKey
	var result T

	// Try to get data from cache if not forcing refresh
	if !forceRefresh {
		startTime := time.Now()
		cachedData, err := redis.GetString(ctx, cacheKey)
		if err == nil && cachedData != "" {
			if jsonErr := json.Unmarshal([]byte(cachedData), &result); jsonErr == nil {
				// Log cache hit and duration
				duration := time.Since(startTime)
				logger.DebugContext(ctx, "Cache hit", cacheLogArgs(cacheKey, zap.Duration("duration", duration))...)

				// Check remaining TTL to decide if background refresh is needed
				ttlRemaining, ttlErr := redis.RedisClient.TTL(ctx, cacheKey).Result()
				if ttlErr == nil && ttlRemaining > 0 && ttlRemaining < cacheRefreshThreshold {
					// Only trigger background refresh if cache is about to expire (< 2 minutes)
					go func() {
						// Check if there's already an active request for this key
						activeRequestsMu.RLock()
						doneChan, exists := activeRequests[cacheKey]
						if exists {
							// Another request is already in progress, skip this refresh
							activeRequestsMu.RUnlock()
							return
						}
						activeRequestsMu.RUnlock()

						// Mark this key as being processed
						doneChan = make(chan struct{})
						activeRequestsMu.Lock()
						activeRequests[cacheKey] = doneChan
						activeRequestsMu.Unlock()

						// Ensure cleanup of the active request tracking
						defer func() {
							activeRequestsMu.Lock()
							delete(activeRequests, cacheKey)
							activeRequestsMu.Unlock()
							close(doneChan)
						}()

						// Create a background context since the original request may be done
						bgCtx := context.Background()

						// Fetch fresh data
						freshData, fetchErr := fetchFunc()
						if fetchErr != nil {
							logger.WarnContext(bgCtx, "Background cache refresh failed",
								cacheLogArgs(cacheKey, zap.Error(fetchErr))...,
							)
							return
						}

						// Update cache with fresh data
						jsonData, jsonErr := json.Marshal(freshData)
						if jsonErr != nil {
							logger.WarnContext(bgCtx, "Failed to marshal fresh cache data",
								cacheLogArgs(cacheKey, zap.Error(jsonErr))...,
							)
							return
						}

						if setErr := redis.SetEx(bgCtx, cacheKey, string(jsonData), ttl); setErr != nil {
							logger.WarnContext(bgCtx, "Failed to set fresh cache data",
								cacheLogArgs(cacheKey, zap.Error(setErr))...,
							)
							return
						}

						logger.DebugContext(bgCtx, "Cache refreshed in background", cacheLogArgs(cacheKey)...)
					}()
				}

				return result, nil
			} else {
				logger.WarnContext(ctx, "Failed to unmarshal cached data",
					cacheLogArgs(cacheKey, zap.Error(jsonErr))...,
				)
			}
		}
		// Log cache miss
		duration := time.Since(startTime)
		logger.DebugContext(ctx, "Cache miss", cacheLogArgs(cacheKey, zap.Duration("duration", duration))...)
	}

	// Cache miss or force refresh - fetch fresh data
	startTime := time.Now()
	data, err := fetchFunc()
	if err != nil {
		return result, err
	}

	// Log fetch duration
	duration := time.Since(startTime)
	logger.DebugContext(ctx, "Cache source data fetched", cacheLogArgs(cacheKey, zap.Duration("duration", duration))...)

	// Update cache
	jsonData, jsonErr := json.Marshal(data)
	if jsonErr != nil {
		logger.WarnContext(ctx, "Failed to marshal cache data",
			cacheLogArgs(cacheKey, zap.Error(jsonErr))...,
		)
		return data, nil // Return data even if caching fails
	}

	if setErr := redis.SetEx(ctx, cacheKey, string(jsonData), ttl); setErr != nil {
		logger.WarnContext(ctx, "Failed to set cache data",
			cacheLogArgs(cacheKey, zap.Error(setErr))...,
		)
		return data, nil // Return data even if caching fails
	}

	return data, nil
}

// DeleteCache removes a cache entry
func DeleteCache(ctx context.Context, cacheKey string) error {
	prefixedCacheKey := addPrefixToCacheKey(cacheKey)
	cacheKey = prefixedCacheKey
	return redis.RedisClient.Del(ctx, cacheKey).Err()
}

// DeleteCacheByPrefix removes all cache entries matching a given prefix
func DeleteCacheByPrefix(ctx context.Context, prefix string) error {
	prefixedPrefix := addPrefixToCacheKey(prefix)
	return redis.DeleteKeysByPrefix(ctx, prefixedPrefix)
}

// ForceRefreshCache forces a cache refresh by deleting the existing cache entry
func ForceRefreshCache(ctx context.Context, cacheKey string) error {
	prefixedCacheKey := addPrefixToCacheKey(cacheKey)
	cacheKey = prefixedCacheKey
	return DeleteCache(ctx, cacheKey)
}
