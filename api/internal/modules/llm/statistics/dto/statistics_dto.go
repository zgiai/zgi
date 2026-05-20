package dto

// WorkspaceQuotaRequest represents workspace quota query request.
type WorkspaceQuotaRequest struct {
	WorkspaceID *string `form:"workspace_id"`
}

// WorkspaceQuotaSummary represents aggregated workspace quota snapshot.
type WorkspaceQuotaSummary struct {
	TotalWorkspaces  int64 `json:"total_workspaces"`
	UnlimitedCount   int64 `json:"unlimited_count"`
	TotalUsedQuota   int64 `json:"total_used_quota"`
	TotalRemainQuota int64 `json:"total_remain_quota"`
	TotalQuotaLimit  int64 `json:"total_quota_limit"`
}

// WorkspaceQuotaItem represents a single workspace quota snapshot item.
type WorkspaceQuotaItem struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	UsedQuota     int64  `json:"used_quota"`
	RemainQuota   int64  `json:"remain_quota"`
	QuotaLimit    *int64 `json:"quota_limit,omitempty"`
	IsUnlimited   bool   `json:"is_unlimited"`
}

// WorkspaceQuotaResponse represents workspace quota statistics response.
type WorkspaceQuotaResponse struct {
	Summary WorkspaceQuotaSummary `json:"summary"`
	Items   []WorkspaceQuotaItem  `json:"items"`
}
