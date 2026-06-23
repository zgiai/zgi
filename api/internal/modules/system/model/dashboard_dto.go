package model

// ModelsStats represents model statistics grouped by use case
type ModelsStats struct {
	Total     int64            `json:"total"`
	ByUseCase map[string]int64 `json:"by_usecase"`
}

// ResourceStats represents resource statistics
type ResourceStats struct {
	Workspaces  int64 `json:"workspaces"`
	Agents      int64 `json:"agents"`
	Datasets    int64 `json:"datasets"`
	DataSources int64 `json:"data_sources"`
	Files       int64 `json:"files"`
}

// DashboardStatsResponse represents the dashboard statistics response
type DashboardStatsResponse struct {
	Models    ModelsStats   `json:"models"`
	Resources ResourceStats `json:"resources"`
}

// DashboardWorkspaceScopes describes the workspace sets visible to the current account.
type DashboardWorkspaceScopes struct {
	WorkspaceIDs           []string
	AgentWorkspaceIDs      []string
	DatasetWorkspaceIDs    []string
	DataSourceWorkspaceIDs []string
	FileWorkspaceIDs       []string
}

// RecentWorkRequest describes a recent-work query for either overview or workspace scope.
type RecentWorkRequest struct {
	OrganizationID         string
	AccountID              string
	Limit                  int
	WorkspaceIDs           []string
	AgentWorkspaceIDs      []string
	DatasetWorkspaceIDs    []string
	DataSourceWorkspaceIDs []string
}

// RecentWorkItem represents one recently updated console work item.
type RecentWorkItem struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	ResourceID    string `json:"resource_id"`
	ParentID      string `json:"parent_id,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	UpdatedAt     int64  `json:"updated_at"`
}

// RecentWorkResponse represents recent work items for the console homepage.
type RecentWorkResponse struct {
	Items []RecentWorkItem `json:"items"`
}
