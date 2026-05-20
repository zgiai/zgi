package identity

import "context"

// IdentityContext represents the authenticated user context.
type IdentityContext struct {
	TenantID  string
	AccountID string
	Role      string
}

// IdentityProvider defines the interface for user authentication.
type IdentityProvider interface {
	// Identify extracts identity from a token
	Identify(ctx context.Context, token string) (*IdentityContext, error)
}
