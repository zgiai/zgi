package agents

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"strings"
)

func (s *agentsService) GetPublishedAgentWebAppConfig(ctx context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	ag, err := s.agentsRepo.GetByWebAppID(ctx, webAppID)
	if err != nil {
		return nil, err
	}
	policy, err := s.publishedRuntimePolicyForAgent(ctx, ag)
	if err != nil {
		return nil, err
	}
	if !policy.Allows(runtimeauth.PublishedRuntimeSurfaceWebApp) {
		return nil, errAgentWebAppOffline
	}
	if ag.AgentsType != "AGENT" {
		return nil, fmt.Errorf("web app is not an AGENT runtime")
	}
	version, err := s.agentsRepo.GetLatestAgentPublishedVersion(ctx, ag.ID.String())
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, errAgentWebAppNotPublished
	}
	workspaceID := ag.TenantID.String()
	organizationID := workspaceID
	if s.enterpriseService != nil {
		org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve web app organization: %w", err)
		}
		if org != nil {
			organizationID = org.ID
		}
	}
	cfg := agentConfigResponseFromSnapshot(ag.ID.String(), version.ConfigSnapshot)
	cfg.WorkflowBindings = s.hydrateAgentWorkflowBindingRuntimeInputs(ctx, workspaceID, cfg.WorkflowBindings)
	if _, ok := version.ConfigSnapshot["supports_vision"]; !ok {
		cfg.SupportsVision = s.resolveAgentModelSupportsVision(ctx, organizationID, cfg.ModelProvider, cfg.Model)
	}
	iconURL := ""
	if ag.IconType != nil && *ag.IconType == "image" && ag.Icon != nil && *ag.Icon != "" && s.fileService != nil {
		if fileURL, err := s.fileService.GetFileURL(ctx, *ag.Icon); err == nil {
			iconURL = fileURL
		}
	}
	return &dto.AgentWebAppRuntimeConfigResponse{
		AgentID:        ag.ID.String(),
		WebAppID:       ag.WebAppID.String(),
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AgentType:      ag.AgentsType,
		Name:           ag.Name,
		Description:    ag.Description,
		Icon:           stringPtrValue(ag.Icon),
		IconType:       stringPtrValue(ag.IconType),
		IconURL:        iconURL,
		Version:        version.Version,
		VersionUUID:    version.VersionUUID.String(),
		Config:         *cfg,
	}, nil
}

func (s *agentsService) organizationIDForAgentWorkspace(ctx context.Context, workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || s.enterpriseService == nil {
		return workspaceID
	}
	org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
	if err != nil || org == nil {
		return workspaceID
	}
	return org.ID
}

func (s *agentsService) resolveAgentModelSupportsVision(ctx context.Context, organizationID, provider, modelName string) bool {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	if s.db == nil {
		return inferAgentModelSupportsVision(modelName)
	}

	var custom struct {
		SupportsVision bool `gorm:"column:vision"`
	}
	if organizationUUID, err := uuid.Parse(strings.TrimSpace(organizationID)); err == nil && provider != "" {
		result := s.db.WithContext(ctx).
			Table("llm_custom_models").
			Select("vision").
			Where("organization_id = ? AND provider = ? AND name = ? AND is_active = ?", organizationUUID, provider, modelName, true).
			Order("sort_order ASC, created_at DESC").
			Limit(1).
			Scan(&custom)
		if result.Error == nil && result.RowsAffected > 0 {
			return custom.SupportsVision
		}
	}

	var global struct {
		SupportsVision bool `gorm:"column:vision"`
	}
	query := s.db.WithContext(ctx).Table("llm_models").Select("vision").Where("name = ? AND is_active = ?", modelName, true)
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	result := query.Limit(1).Scan(&global)
	if result.Error == nil && result.RowsAffected > 0 {
		return global.SupportsVision
	}

	return inferAgentModelSupportsVision(modelName)
}

func inferAgentModelSupportsVision(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	if normalized == "" {
		return false
	}
	visionMarkers := []string{
		"vision",
		"vis",
		"vl",
		"vlm",
		"omni",
		"multimodal",
		"multi-modal",
		"qvq",
	}
	for _, marker := range visionMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
