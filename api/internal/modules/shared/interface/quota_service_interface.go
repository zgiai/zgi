package interfaces

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/dto"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
)

// QuotaService Quota service interface
type QuotaService interface {
	// CheckQuota Check if quota is sufficient
	// Parameters:
	//   - groupID: Organization ID
	//   - resourceType: Resource type
	//   - amount: Operation amount (positive number)
	// Returns:
	//   - canProceed: Whether operation is allowed
	//   - currentUsage: Current usage (calculated by aggregating history records)
	//   - limit: Plan limit (-1 means unlimited)
	//   - error: Error information
	CheckQuota(ctx context.Context, groupID uuid.UUID, resourceType quota_model.ResourceType, amount int64) (canProceed bool, currentUsage int64, limit int64, err error)

	// GetCurrentUsage Get current usage
	// Calculate by aggregating history table: SELECT SUM(delta) FROM quota_usage_history WHERE ...
	GetCurrentUsage(ctx context.Context, groupID uuid.UUID, resourceType quota_model.ResourceType) (int64, error)

	// RecordUsage Record usage history
	// delta: Positive number means increase, negative number means decrease
	// Must be called after business operation succeeds, and within the same transaction
	RecordUsage(ctx context.Context, record *quota_model.QuotaUsageHistory) error

	// RecordUsageInTx Record usage history in transaction
	RecordUsageInTx(ctx context.Context, tx *gorm.DB, record *quota_model.QuotaUsageHistory) error

	// GetQuotaStatus Get quota usage status
	// Returns current usage and limits for all resource types
	GetQuotaStatus(ctx context.Context, groupID uuid.UUID) (*dto.QuotaStatusDTO, error)

	// GetUsageHistory Query usage history
	// Supports filtering by organization, resource type, time range, etc.
	GetUsageHistory(ctx context.Context, filter *dto.QuotaUsageHistoryFilterDTO) (*dto.QuotaUsageHistoryListResponseDTO, error)
}
