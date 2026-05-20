package interfaces

import (
	"context"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

type TokenManager interface {
	GenerateToken(
		ctx context.Context,
		tokenType string,
		account *auth_model.Account,
		email *string,
		additionalData map[string]interface{},
	) (string, error)

	RevokeToken(token string, tokenType string) error

	GetTokenData(token string, tokenType string) (interface{}, error)
}

type RateLimiter interface {
	IsRateLimited(ctx context.Context, email string) (bool, error)
	IncrementRateLimit(email string)
}
