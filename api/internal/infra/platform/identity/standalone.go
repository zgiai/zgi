package identity

import (
	"context"
	"fmt"

	appconfig "github.com/zgiai/zgi/api/config"
)

// Standalone is the implementation for self-hosted/open-source users.
// It uses a simple static admin password for authentication.
type Standalone struct {
	adminPass string
}

// NewStandalone creates a new standalone identity provider.
func NewStandalone() *Standalone {
	return &Standalone{
		adminPass: appconfig.Current().Platform.AdminPass,
	}
}

// Identify validates the token against the static admin password.
func (s *Standalone) Identify(ctx context.Context, token string) (*IdentityContext, error) {
	// Simple token validation: check against admin password
	if s.adminPass != "" && token == s.adminPass {
		return &IdentityContext{
			TenantID:  "default",
			AccountID: "admin",
			Role:      "admin",
		}, nil
	}

	// For open source, if no admin pass is set, allow all requests
	if s.adminPass == "" {
		return &IdentityContext{
			TenantID:  "default",
			AccountID: "anonymous",
			Role:      "user",
		}, nil
	}

	return nil, fmt.Errorf("invalid token")
}
