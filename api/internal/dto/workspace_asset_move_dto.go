package dto

import "github.com/zgiai/zgi/api/internal/capabilities/agentbindings"

type WorkspaceAssetMoveItem struct {
	Type string `json:"type" binding:"required"`
	ID   string `json:"id" binding:"required"`
}

type WorkspaceAssetMoveRequest struct {
	TargetWorkspaceID  string                   `json:"target_workspace_id" binding:"required"`
	TargetFolderID     string                   `json:"target_folder_id,omitempty"`
	Items              []WorkspaceAssetMoveItem `json:"items" binding:"required"`
	AgentBindingAction string                   `json:"agent_binding_action,omitempty"`
	ImpactToken        string                   `json:"impact_token,omitempty"`
}

type WorkspaceAssetMoveEligibleTargetsRequest struct {
	Items   []WorkspaceAssetMoveItem `json:"items" binding:"required"`
	Keyword string                   `json:"keyword,omitempty"`
	Page    int                      `json:"page,omitempty"`
	Limit   int                      `json:"limit,omitempty"`
}

type WorkspaceAssetMoveDependencyPreviewRequest struct {
	Items []WorkspaceAssetMoveItem `json:"items" binding:"required"`
}

type WorkspaceAssetMoveDependencyImpact struct {
	Agents []agentbindings.ImpactAgent `json:"agents"`
}

type WorkspaceAssetMoveDependencyPreviewResponse struct {
	AgentBindingImpact *WorkspaceAssetMoveDependencyImpact `json:"agent_binding_impact,omitempty"`
}

type WorkspaceAssetMoveWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type WorkspaceAssetMoveEligibleTargetsResponse struct {
	Data    []WorkspaceAssetMoveWorkspace `json:"data"`
	Page    int                           `json:"page"`
	Limit   int                           `json:"limit"`
	Total   int64                         `json:"total"`
	HasMore bool                          `json:"has_more"`
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
	Movable            bool                            `json:"movable"`
	Items              []WorkspaceAssetMovePreviewItem `json:"items"`
	AgentBindingImpact *agentbindings.Impact           `json:"agent_binding_impact,omitempty"`
}

type WorkspaceAssetMoveResponse struct {
	Moved   bool                              `json:"moved"`
	Preview WorkspaceAssetMovePreviewResponse `json:"preview"`
}
