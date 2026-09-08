package jwt

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

const revocationTimeout = 2 * time.Second

var ErrTokenRevoked = errors.New("access token revoked")
var ErrRevocationStoreUnavailable = errors.New("access token revocation store unavailable")

func revocationKey(token string) string {
	return fmt.Sprintf("auth:revoked-access:%x", sha256.Sum256([]byte(token)))
}

func checkRevocation(ctx context.Context, token string) error {
	client := redisutil.GetClient()
	if client == nil {
		return ErrRevocationStoreUnavailable
	}
	count, err := client.Exists(ctx, revocationKey(token)).Result()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRevocationStoreUnavailable, err)
	}
	if count != 0 {
		return ErrTokenRevoked
	}
	return nil
}

// RevokeToken validates the signature and expiry claim, revokes this exact access
// token, and returns its authenticated account ID for refresh-token ownership
// checks. Repeating logout is safe; this deliberately does not reject a token
// already on the denylist. It must not be used to authorize other operations.
func RevokeToken(ctx context.Context, token string) (string, error) {
	// An expired access token still identifies the owner of a presented refresh
	// token for logout. Signature validation is mandatory; expiry checks remain
	// unchanged on every authentication path.
	claims, err := parseSignedTokenWithOptions(token, jwtlib.WithoutClaimsValidation())
	if err != nil {
		return "", err
	}
	accountID, ok := claims["user_id"].(string)
	if !ok || accountID == "" {
		return "", errors.New("access token has no account")
	}
	expires, err := jwtlib.MapClaims(claims).GetExpirationTime()
	if err != nil || expires == nil {
		return "", errors.New("access token has no valid expiry")
	}
	ttl := time.Until(expires.Time)
	if ttl <= 0 {
		return accountID, nil // Already unusable for authentication; revoke refresh below.
	}
	client := redisutil.GetClient()
	if client == nil {
		return "", ErrRevocationStoreUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, revocationTimeout)
	defer cancel()
	if err := client.Set(ctx, revocationKey(token), "1", ttl).Err(); err != nil {
		return "", fmt.Errorf("%w: %w", ErrRevocationStoreUnavailable, err)
	}
	return accountID, nil
}
