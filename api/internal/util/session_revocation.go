package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

// Check ownership and remove only this refresh token, including legacy aliases.
// A later login's per-account pointer must not be removed by an older logout.
var revokeAccountRefreshScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if raw then
  local data = cjson.decode(raw)
  if data['account_id'] ~= ARGV[1] then return -1 end
end
local legacy = redis.call('GET', KEYS[2])
if legacy and legacy ~= ARGV[1] then return -1 end
redis.call('DEL', KEYS[1], KEYS[2])
for i = 3, 4 do
  if redis.call('GET', KEYS[i]) == ARGV[2] then redis.call('DEL', KEYS[i]) end
end
return 1
`)

func (tm *TokenManager) RevokeAccountRefreshToken(ctx context.Context, token, accountID string) error {
	if token == "" {
		return nil // Older clients may only have an access token.
	}
	if accountID == "" {
		return errors.New("refresh token revocation requires an account")
	}
	client := redisutil.GetClient()
	if client == nil {
		return errors.New("refresh token revocation store unavailable")
	}
	result, err := revokeAccountRefreshScript.Run(ctx, client, []string{
		tm.getTokenKey(token, "refresh"), "refresh_token:" + token,
		"refresh_token:" + accountID, "current_token:refresh:" + accountID,
	}, accountID, token).Int()
	if err != nil {
		return fmt.Errorf("revoke account refresh token: %w", err)
	}
	if result != 1 {
		return errors.New("refresh token does not belong to account")
	}
	return nil
}
