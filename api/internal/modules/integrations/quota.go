package integrations

import (
	"context"
	"fmt"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type DailyQuota interface {
	Acquire(ctx context.Context, organizationID string) error
}

type RedisDailyQuota struct {
	client *redis.Client
	limit  int64
	now    func() time.Time
}

var acquireDailyQuotaScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIREAT", KEYS[1], ARGV[1])
end
return current
`)

func NewRedisDailyQuota(client *redis.Client, limit int) *RedisDailyQuota {
	return &RedisDailyQuota{client: client, limit: int64(limit), now: time.Now}
}

func (q *RedisDailyQuota) Acquire(ctx context.Context, organizationID string) error {
	if q == nil || q.client == nil {
		return fmt.Errorf("integration quota store is unavailable")
	}
	if q.limit <= 0 {
		return fmt.Errorf("integration daily quota is invalid")
	}
	now := q.now().UTC()
	organizationID = strings.TrimSpace(organizationID)
	key := fmt.Sprintf("zgi:integration:web-search:daily:%s:%s", organizationID, now.Format("2006-01-02"))
	expiresAt := now.Truncate(24 * time.Hour).Add(25 * time.Hour).Unix()
	current, err := acquireDailyQuotaScript.Run(ctx, q.client, []string{key}, expiresAt).Int64()
	if err != nil {
		return fmt.Errorf("acquire integration daily quota: %w", err)
	}
	if current > q.limit {
		return ErrQuotaExceeded
	}
	return nil
}
