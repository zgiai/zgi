package interfaces

import (
	"context"
)

type BillingService interface {
	CheckResourceQuota(ctx context.Context, accountID string, resourceType string, requestAmount ...int64) error

	// GetBillingInfo(ctx context.Context, accountID string) (*BillingInfo, error)
	// UpdateBillingStatus(ctx context.Context, accountID string, status string) error
}
