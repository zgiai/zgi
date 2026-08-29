package integrations

import (
	redis "github.com/redis/go-redis/v9"
	integrationquota "github.com/zgiai/zgi/api/internal/modules/integrations/quota"
)

type DailyQuota = integrationquota.DailyQuota

type RedisDailyQuota = integrationquota.RedisDailyQuota

func NewRedisDailyQuota(client *redis.Client, limit int) *RedisDailyQuota {
	return integrationquota.NewRedisDailyQuota(client, limit)
}
