package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/dto"
)

// AvailabilityService provides model availability checking
type AvailabilityService interface {
	// CheckModelAvailability checks if a specific model is available for the tenant
	CheckModelAvailability(ctx context.Context, organizationID, modelID uuid.UUID) (*dto.ModelAvailability, error)

	// BatchCheckAvailability checks multiple models at once
	BatchCheckAvailability(ctx context.Context, organizationID uuid.UUID, modelIDs []uuid.UUID) ([]*dto.ModelAvailability, error)
}
