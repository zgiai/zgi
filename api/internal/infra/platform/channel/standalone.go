package channel

import (
	"context"

	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Standalone is the implementation for self-hosted/open-source users.
// Self-hosted deployments do not provide built-in platform channels.
type Standalone struct {
	db *gorm.DB
}

// NewStandalone creates a new standalone channel provider.
func NewStandalone(db *gorm.DB) *Standalone {
	return &Standalone{
		db: db,
	}
}

// ListChannels returns official channels.
// In standalone mode there are no built-in official channels.
func (s *Standalone) ListChannels(ctx context.Context, tenantID string) ([]*OfficialChannel, error) {
	// In standalone mode, no built-in official channels
	// Tenants configure their own channels locally
	logger.DebugContext(ctx, "No built-in official channels in standalone mode", zap.String("tenant_id", tenantID))
	return []*OfficialChannel{}, nil
}
