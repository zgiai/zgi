package identity

import (
	"context"
	"crypto/rsa"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

// Remote is the Cloud implementation using JWT validation.
type Remote struct {
	publicKey *rsa.PublicKey
}

// NewRemote creates a new remote identity provider with JWT validation.
func NewRemote(publicKeyPath string) (*Remote, error) {
	keyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &Remote{publicKey: publicKey}, nil
}

// Identify validates a JWT token and extracts user identity.
func (r *Remote) Identify(ctx context.Context, token string) (*IdentityContext, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return r.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	tenantID, _ := claims["tenant_id"].(string)
	accountID, _ := claims["account_id"].(string)
	role, _ := claims["role"].(string)

	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id not found in token")
	}

	return &IdentityContext{
		TenantID:  tenantID,
		AccountID: accountID,
		Role:      role,
	}, nil
}
