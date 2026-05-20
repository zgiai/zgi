package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/quota/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// FormatQuotaError Format quota error message
// Returns corresponding error code and formatted error message based on resource type
func FormatQuotaError(resourceType model.ResourceType, current, limit, attempt int64) error {
	// Log quota check failure
	logger.Warn("Quota check failed",
		"resource_type", string(resourceType),
		"current_usage", current,
		"limit", limit,
		"attempt_amount", attempt,
	)

	switch resourceType {
	case model.ResourceTypeSeats:
		return fmt.Errorf("seats quota exceeded. Current: %d, Limit: %d, Attempting: +%d", current, limit, attempt)

	case model.ResourceTypeStorage:
		// Storage space needs to be converted to MB for display
		currentMB := float64(current) / (1024 * 1024)
		limitMB := float64(limit) / (1024 * 1024)
		attemptMB := float64(attempt) / (1024 * 1024)
		return fmt.Errorf("storage quota exceeded. Current: %.2fMB, Limit: %.2fMB, Attempting: +%.2fMB",
			currentMB, limitMB, attemptMB)

	case model.ResourceTypeDBRows:
		return fmt.Errorf("database rows quota exceeded. Current: %d, Limit: %d, Attempting: +%d", current, limit, attempt)

	case model.ResourceTypeKnowledgeBases:
		return fmt.Errorf("knowledge bases quota exceeded. Current: %d, Limit: %d", current, limit)

	case model.ResourceTypeAIAgents:
		return fmt.Errorf("AI agents quota exceeded. Current: %d, Limit: %d", current, limit)

	case model.ResourceTypeWorkflows:
		return fmt.Errorf("workflows quota exceeded. Current: %d, Limit: %d", current, limit)

	case model.ResourceTypeWorkflowExecutions:
		return fmt.Errorf("workflow executions quota exceeded. Current: %d, Limit: %d", current, limit)

	case model.ResourceTypeOCRPages:
		return fmt.Errorf("OCR pages quota exceeded. Current: %d, Limit: %d, Attempting: +%d", current, limit, attempt)

	default:
		// Unknown resource type, return generic error
		return fmt.Errorf("quota exceeded")
	}
}

// CheckQuotaWithError Check quota and return formatted error
// This is a convenience method that wraps CheckQuota and error formatting
func (s *quotaService) CheckQuotaWithError(ctx context.Context, groupID uuid.UUID, resourceType model.ResourceType, amount int64) error {
	canProceed, current, limit, err := s.CheckQuota(ctx, groupID, resourceType, amount)
	if err != nil {
		return err
	}

	if !canProceed {
		return FormatQuotaError(resourceType, current, limit, amount)
	}

	return nil
}

// FormatStorageSize Format storage size to human-readable format
func FormatStorageSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// LogQuotaCheck Log quota check
func LogQuotaCheck(groupID string, resourceType model.ResourceType, amount, current, limit int64, canProceed bool) {
	if canProceed {
		logger.Debug("Quota check passed",
			"group_id", groupID,
			"resource_type", string(resourceType),
			"amount", amount,
			"current_usage", current,
			"limit", limit,
		)
	} else {
		logger.Warn("Quota check failed",
			"group_id", groupID,
			"resource_type", string(resourceType),
			"amount", amount,
			"current_usage", current,
			"limit", limit,
		)
	}
}
