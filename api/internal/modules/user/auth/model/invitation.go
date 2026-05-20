package model

import (
	"time"

	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
)

// Invitation invitation information
type Invitation struct {
	ID        string                              `json:"id"`
	TenantID  string                              `json:"tenant_id"`
	AccountID string                              `json:"account_id"`
	Role      workspace_model.WorkspaceMemberRole `json:"role"`
	CreatedAt time.Time                           `json:"created_at"`
	ExpiresAt time.Time                           `json:"expires_at"`
}
