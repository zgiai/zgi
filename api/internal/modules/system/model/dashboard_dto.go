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

// ActivityStats represents stable account activity used by workspace onboarding.
type ActivityStats struct {
	DirectConversations int64 `json:"direct_conversations"`
	CreatedAgents       int64 `json:"created_agents"`
	CreatedDatasets     int64 `json:"created_datasets"`
}

// DashboardStatsResponse represents the dashboard statistics response
type DashboardStatsResponse struct {
	Models    ModelsStats   `json:"models"`
	Resources ResourceStats `json:"resources"`
	Activity  ActivityStats `json:"activity"`
}

// DashboardWorkspaceScopes describes the workspace sets visible to the current account.
type DashboardWorkspaceScopes struct {
	WorkspaceIDs                     []string
	AgentWorkspaceIDs                []string
	WorkflowWorkspaceIDs             []string
	AgentConversationWorkspaceIDs    []string
	WorkflowConversationWorkspaceIDs []string
	DatasetWorkspaceIDs              []string
	DataSourceWorkspaceIDs           []string
	FileWorkspaceIDs                 []string
	// AgentDraftWorkspaceIDs and WorkflowDraftWorkspaceIDs are the workspaces
	// where the account holds a management code and may see other members'
	// unpublished assets. Elsewhere only published assets and its own drafts show.
	AgentDraftWorkspaceIDs    []string
	WorkflowDraftWorkspaceIDs []string
	// WorkflowTestLibraryWorkspaceIDs are the workspaces where the account may
	// open a workflow's batch-test library (workflow.view).
	WorkflowTestLibraryWorkspaceIDs []string
}

// RecentWorkRequest describes a recent-work query for either overview or workspace scope.
type RecentWorkRequest struct {
	OrganizationID                   string
	AccountID                        string
	Limit                            int
	WorkspaceIDs                     []string
	AgentWorkspaceIDs                []string
	WorkflowWorkspaceIDs             []string
	AgentConversationWorkspaceIDs    []string
	WorkflowConversationWorkspaceIDs []string
	DatasetWorkspaceIDs              []string
	DataSourceWorkspaceIDs           []string
	FileWorkspaceIDs                 []string
	// AgentDraftWorkspaceIDs and WorkflowDraftWorkspaceIDs are the workspaces
	// where the account holds a management code and may see other members'
	// unpublished assets. Elsewhere only published assets and its own drafts show.
	AgentDraftWorkspaceIDs    []string
	WorkflowDraftWorkspaceIDs []string
	// WorkflowTestLibraryWorkspaceIDs are the workspaces where the account may
	// open a workflow's batch-test library (workflow.view).
	WorkflowTestLibraryWorkspaceIDs []string
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
	// Agent and workflow items carry what the console needs to pick a detail
	// route the account may open; other item types omit them.
	AgentType     string `json:"agent_type,omitempty"`
	IsPublished   *bool  `json:"is_published,omitempty"`
	CanOpenEditor *bool  `json:"can_open_editor,omitempty"`
	CanViewLogs   *bool  `json:"can_view_logs,omitempty"`
	// CanViewBatchTest and PreferBatchTestLibrary are set for workflow items only.
	CanViewBatchTest       *bool `json:"can_view_batch_test,omitempty"`
	PreferBatchTestLibrary *bool `json:"prefer_batch_test_library,omitempty"`
}

// RecentWorkResponse represents recent work items for the console homepage.
type RecentWorkResponse struct {
	Items []RecentWorkItem `json:"items"`
}
