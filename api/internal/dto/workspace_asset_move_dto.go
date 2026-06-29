package dto

type WorkspaceAssetMoveItem struct {
	Type string `json:"type" binding:"required"`
	ID   string `json:"id" binding:"required"`
}

type WorkspaceAssetMoveRequest struct {
	TargetWorkspaceID string                   `json:"target_workspace_id" binding:"required"`
	TargetFolderID    string                   `json:"target_folder_id,omitempty"`
	Items             []WorkspaceAssetMoveItem `json:"items" binding:"required"`
}

type WorkspaceAssetMoveWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type WorkspaceAssetMovePreviewItem struct {
	Type              string                       `json:"type"`
	ID                string                       `json:"id"`
	FromWorkspace     *WorkspaceAssetMoveWorkspace `json:"from_workspace,omitempty"`
	TargetWorkspace   *WorkspaceAssetMoveWorkspace `json:"target_workspace,omitempty"`
	Movable           bool                         `json:"movable"`
	Blockers          []string                     `json:"blockers"`
	Warnings          []string                     `json:"warnings"`
	ResolvedAgentID   string                       `json:"-"`
	ResolvedAgentType string                       `json:"-"`
	TargetFolderID    string                       `json:"-"`
	FromWorkspaceID   string                       `json:"-"`
	TargetWorkspaceID string                       `json:"-"`
}

type WorkspaceAssetMovePreviewResponse struct {
	Movable bool                            `json:"movable"`
	Items   []WorkspaceAssetMovePreviewItem `json:"items"`
}

type WorkspaceAssetMoveResponse struct {
	Moved   bool                              `json:"moved"`
	Preview WorkspaceAssetMovePreviewResponse `json:"preview"`
}
