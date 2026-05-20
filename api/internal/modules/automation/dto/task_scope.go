package dto

import automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"

// TaskScope defines the organization and workspace boundaries for task operations.
type TaskScope struct {
	OrganizationID string `json:"organization_id"`
	WorkspaceID    string `json:"workspace_id"`
}

// TaskFilter represents list filters for automation task queries.
type TaskFilter struct {
	TaskScope
	Statuses []automationmodel.AutomationTaskStatus `json:"statuses,omitempty"`
	Page     int                                    `json:"page,omitempty"`
	Limit    int                                    `json:"limit,omitempty"`
}
