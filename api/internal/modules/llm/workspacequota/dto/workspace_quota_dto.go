package dto

import "time"

const (
	QuotaTypeUnlimited = "unlimited"
	QuotaTypeCustom    = "custom"
)

// WorkspaceQuotaResponse represents workspace quota state in LLM billing.
type WorkspaceQuotaResponse struct {
	WorkspaceID    string     `json:"workspace_id"`
	OrganizationID string     `json:"organization_id"`
	UsedQuota      int64      `json:"used_quota"`
	RemainQuota    int64      `json:"remain_quota"`
	QuotaLimit     *int64     `json:"quota_limit,omitempty"`
	Configured     bool       `json:"configured"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// ListWorkspaceQuotaRequest is the query request for workspace quota list.
type ListWorkspaceQuotaRequest struct {
	Page  int `form:"page,default=1"`
	Limit int `form:"limit,default=20"`
}

// ListWorkspaceQuotaResponse is the paginated workspace quota response.
type ListWorkspaceQuotaResponse struct {
	Items      []*WorkspaceQuotaResponse `json:"items"`
	Total      int64                     `json:"total"`
	Page       int                       `json:"page"`
	Limit      int                       `json:"limit"`
	TotalPages int                       `json:"total_pages"`
}

// UpdateWorkspaceQuotaRequest updates workspace quota policy.
type UpdateWorkspaceQuotaRequest struct {
	QuotaType   string `json:"quota_type" binding:"required,oneof=unlimited custom"`
	QuotaAmount *int64 `json:"quota_amount,omitempty"`
	RemainQuota *int64 `json:"remain_quota,omitempty"`
}
