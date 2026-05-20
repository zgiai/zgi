package model

// ModelsStats represents model statistics grouped by use case
type ModelsStats struct {
	Total     int64            `json:"total"`
	ByUseCase map[string]int64 `json:"by_usecase"`
}

// ResourceStats represents resource statistics
type ResourceStats struct {
	Agents      int64 `json:"agents"`
	Datasets    int64 `json:"datasets"`
	DataSources int64 `json:"data_sources"`
}

// DashboardStatsResponse represents the dashboard statistics response
type DashboardStatsResponse struct {
	Models    ModelsStats   `json:"models"`
	Resources ResourceStats `json:"resources"`
}

// RecentWorkItem represents one recently updated console work item.
type RecentWorkItem struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	ResourceID string `json:"resource_id"`
	ParentID   string `json:"parent_id,omitempty"`
	UpdatedAt  int64  `json:"updated_at"`
}

// RecentWorkResponse represents recent work items for the console homepage.
type RecentWorkResponse struct {
	Items []RecentWorkItem `json:"items"`
}
