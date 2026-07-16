package statuscache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/cache/keys"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/pkg/database"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"golang.org/x/sync/singleflight"
)

const (
	accountStatusCacheModule     = "auth.account_status"
	accountStatusGenerationPart  = "generation"
	accountStatusCacheTTL        = 30 * time.Second
	accountStatusGenerationTTL   = 24 * time.Hour
	accountStatusDBLoadLimit     = 32
	accountStatusDBLoadTimeout   = 2 * time.Second
	lastActiveTouchInterval      = 10 * time.Minute
	lastActiveTouchRetryInterval = time.Minute
	lastActiveTouchTimeout       = time.Second
	redisOperationTimeout        = 50 * time.Millisecond
)

var (
	accountStatusGroup        singleflight.Group
	accountStatusDBLoadTokens = make(chan struct{}, accountStatusDBLoadLimit)
	lastActiveTouchedAt       sync.Map
)

// GetAccountStatus returns the authenticated account status using Redis as a shared
// short-lived cache. Redis failures fall back to the database so auth remains available.
func GetAccountStatus(ctx context.Context, accountID string) (auth_model.AccountStatus, error) {
	if accountID == "" {
		return "", errors.New("account id is required")
	}

	if status, ok := getAccountStatusFromRedis(ctx, accountID); ok {
		return status, nil
	}

	result, err, _ := accountStatusGroup.Do(accountID, func() (interface{}, error) {
		if status, ok := getAccountStatusFromRedis(ctx, accountID); ok {
			return status, nil
		}

		fillCtx, cancel := context.WithTimeout(context.Background(), accountStatusDBLoadTimeout)
		defer cancel()

		generation, canCache := getAccountStatusGeneration(fillCtx, accountID)
		status, err := loadAccountStatusFromDB(fillCtx, accountID)
		if err != nil {
			return auth_model.AccountStatus(""), err
		}
		if canCache {
			setAccountStatusToRedisIfGeneration(fillCtx, accountID, status, generation)
		}
		return status, nil
	})
	if err != nil {
		return "", err
	}

	status, ok := result.(auth_model.AccountStatus)
	if !ok {
		return "", fmt.Errorf("unexpected account status cache value %T", result)
	}
	return status, nil
}

// InvalidateAccountStatus removes the shared account status cache entry.
func InvalidateAccountStatus(ctx context.Context, accountID string) {
	if accountID == "" {
		return
	}

	client := redisutil.GetClient()
	if client == nil {
		return
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOperationTimeout)
	defer cancel()
	_, _ = client.Pipelined(redisCtx, func(pipe goredis.Pipeliner) error {
		generationKey := accountStatusGenerationKey(accountID)
		pipe.Incr(redisCtx, generationKey)
		pipe.Expire(redisCtx, generationKey, accountStatusGenerationTTL)
		pipe.Del(redisCtx, accountStatusCacheKey(accountID))
		return nil
	})
}

// TouchAccountLastActive refreshes last_active_at at most once per process interval.
// The actual database update runs asynchronously and is guarded by a DB-side time
// predicate so multiple API processes do not keep writing the same account.
func TouchAccountLastActive(accountID string) {
	if accountID == "" {
		return
	}

	now := time.Now()
	if lastTouch, ok := lastActiveTouchedAt.Load(accountID); ok && now.Sub(lastTouch.(time.Time)) < lastActiveTouchInterval {
		return
	}
	lastActiveTouchedAt.Store(accountID, now)

	go touchAccountLastActive(accountID, now)
}

func getAccountStatusFromRedis(ctx context.Context, accountID string) (auth_model.AccountStatus, bool) {
	client := redisutil.GetClient()
	if client == nil {
		return "", false
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOperationTimeout)
	defer cancel()

	value, err := client.Get(redisCtx, accountStatusCacheKey(accountID)).Result()
	if err != nil {
		return "", false
	}
	if value == "" {
		return "", false
	}

	return auth_model.AccountStatus(value), true
}

func getAccountStatusGeneration(ctx context.Context, accountID string) (string, bool) {
	client := redisutil.GetClient()
	if client == nil {
		return "", false
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOperationTimeout)
	defer cancel()

	value, err := client.Get(redisCtx, accountStatusGenerationKey(accountID)).Result()
	if errors.Is(err, goredis.Nil) {
		return "0", true
	}
	if err != nil {
		return "", false
	}
	if value == "" {
		return "0", true
	}
	return value, true
}

func setAccountStatusToRedisIfGeneration(ctx context.Context, accountID string, status auth_model.AccountStatus, generation string) {
	client := redisutil.GetClient()
	if client == nil {
		return
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOperationTimeout)
	defer cancel()
	_ = client.Watch(redisCtx, func(tx *goredis.Tx) error {
		currentGeneration, err := tx.Get(redisCtx, accountStatusGenerationKey(accountID)).Result()
		if errors.Is(err, goredis.Nil) {
			currentGeneration = "0"
		} else if err != nil {
			return err
		}
		if currentGeneration != generation {
			return nil
		}

		_, err = tx.TxPipelined(redisCtx, func(pipe goredis.Pipeliner) error {
			pipe.SetEx(redisCtx, accountStatusCacheKey(accountID), string(status), accountStatusCacheTTL)
			return nil
		})
		return err
	}, accountStatusGenerationKey(accountID))
}

func loadAccountStatusFromDB(ctx context.Context, accountID string) (auth_model.AccountStatus, error) {
	if err := acquireAccountStatusDBLoadSlot(ctx); err != nil {
		return "", err
	}
	defer releaseAccountStatusDBLoadSlot()

	var account auth_model.Account
	if err := database.GetDB().
		WithContext(ctx).
		Select("id", "status").
		Where("id = ?", accountID).
		Take(&account).Error; err != nil {
		return "", err
	}

	return account.Status, nil
}

func accountStatusCacheKey(accountID string) string {
	return keys.DefaultBuilder().Build(accountStatusCacheModule, accountID)
}

func accountStatusGenerationKey(accountID string) string {
	return keys.DefaultBuilder().Build(accountStatusCacheModule, accountStatusGenerationPart, accountID)
}

func acquireAccountStatusDBLoadSlot(ctx context.Context) error {
	select {
	case accountStatusDBLoadTokens <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseAccountStatusDBLoadSlot() {
	<-accountStatusDBLoadTokens
}

func touchAccountLastActive(accountID string, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), lastActiveTouchTimeout)
	defer cancel()

	cutoff := now.Add(-lastActiveTouchInterval)
	result := database.GetDB().
		WithContext(ctx).
		Model(&auth_model.Account{}).
		Where("id = ?", accountID).
		Where("last_active_at IS NULL OR last_active_at < ?", cutoff).
		Update("last_active_at", now)
	if result.Error == nil {
		return
	}

	if lastTouch, ok := lastActiveTouchedAt.Load(accountID); ok && lastTouch.(time.Time).Equal(now) {
		lastActiveTouchedAt.Store(accountID, now.Add(-lastActiveTouchInterval+lastActiveTouchRetryInterval))
	}
}
