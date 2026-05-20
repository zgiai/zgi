package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (r *agentsRepository) UpdateWebAppStatus(ctx context.Context, agentID string, status AgentWebAppStatus, reason string, updatedBy string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"web_app_status": status,
		"updated_at":     now,
	}

	if updaterID, err := uuid.Parse(updatedBy); err == nil {
		updates["updated_by"] = updaterID.String()
	}

	if status == AgentWebAppStatusInactive {
		updates["web_app_offlined_at"] = now
		updates["web_app_offline_reason"] = reason
		if updaterID, err := uuid.Parse(updatedBy); err == nil {
			updates["web_app_offlined_by"] = updaterID.String()
		}
	} else {
		updates["web_app_offlined_at"] = nil
		updates["web_app_offlined_by"] = nil
		updates["web_app_offline_reason"] = ""
	}

	result := r.db.WithContext(ctx).
		Model(&Agent{}).
		Where("id = ? AND deleted_at IS NULL", agentID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update web app status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent not found")
	}
	return nil
}
