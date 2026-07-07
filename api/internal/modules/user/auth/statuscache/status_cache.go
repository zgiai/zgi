package statuscache

import (
	"context"
	"errors"
	"fmt"
	"time"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/pkg/database"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"golang.org/x/sync/singleflight"
)

const (
	accountStatusCachePrefix = "auth:account_status:"
	accountStatusCacheTTL    = 30 * time.Second
	accountStatusDBLoadLimit = 32
	redisOperationTimeout    = 50 * time.Millisecond
)

var (
	accountStatusGroup        singleflight.Group
	accountStatusDBLoadTokens = make(chan struct{}, accountStatusDBLoadLimit)
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

		status, err := loadAccountStatusFromDB(ctx, accountID)
		if err != nil {
			return auth_model.AccountStatus(""), err
		}
		setAccountStatusToRedis(ctx, accountID, status)
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
	_ = client.Del(redisCtx, accountStatusCacheKey(accountID)).Err()
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

func setAccountStatusToRedis(ctx context.Context, accountID string, status auth_model.AccountStatus) {
	client := redisutil.GetClient()
	if client == nil {
		return
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOperationTimeout)
	defer cancel()
	_ = client.SetEx(redisCtx, accountStatusCacheKey(accountID), string(status), accountStatusCacheTTL).Err()
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
	return accountStatusCachePrefix + accountID
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
