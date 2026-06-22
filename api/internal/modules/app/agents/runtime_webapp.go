package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	agentWebAppCapabilityReasonLoginRequired = "login_required"
	agentWebAppCapabilityReasonNoAccess      = "no_access"
)

func (s *agentsService) GetPublishedAgentWebAppConfig(ctx context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	ag, err := s.agentsRepo.GetByWebAppID(ctx, webAppID)
	if err != nil {
		return nil, err
	}
	_, auth, err := s.publishedRuntimeAuthorizationForAgent(ctx, ag)
	if err != nil {
		return nil, err
	}
	if !webAppRuntimeSurfaceEnabled(auth) {
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

func webAppRuntimeSurfaceEnabled(auth *runtimeauth.ResourceAuthorization) bool {
	surface, ok := auth.Surface(runtimeauth.PublishedRuntimeSurfaceWebApp)
	return ok && surface.Enabled
}

func (s *agentsService) GetWebAppRuntimeCapability(ctx context.Context, webAppID, accountID string, authenticated bool) (*dto.AgentWebAppRuntimeCapabilityResponse, error) {
	ag, err := s.agentsRepo.GetByWebAppID(ctx, webAppID)
	if err != nil {
		return nil, err
	}
	fallback, auth, err := s.publishedRuntimeAuthorizationForAgent(ctx, ag)
	if err != nil {
		return nil, err
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
	organizationID := s.webAppRuntimeOrganizationID(ctx, ag, auth)
	organizationUUID, _ := uuid.Parse(strings.TrimSpace(organizationID))

	surface, ok := auth.Surface(runtimeauth.PublishedRuntimeSurfaceWebApp)
	decision := runtimeauth.RuntimeAccessDecision{Reason: runtimeauth.RuntimeAccessDeniedMissingSurface}
	if ok {
		decision = surface.Evaluate(runtimeauth.RuntimeAudience{})
	}
	privateAudienceEnabled := webAppSurfaceHasPrivateAudience(surface)
	if ok && !decision.Allowed && privateAudienceEnabled {
		if !authenticated {
			decision.Reason = runtimeauth.RuntimeAccessDecisionReason(agentWebAppCapabilityReasonLoginRequired)
		} else {
			audience, err := s.webAppRuntimeAudienceForAccount(ctx, organizationUUID, accountID)
			if err != nil {
				return nil, err
			}
			decision = surface.Evaluate(audience)
			if !decision.Allowed && decision.Reason == runtimeauth.RuntimeAccessDeniedNoMatchingGrant {
				decision.Reason = runtimeauth.RuntimeAccessDecisionReason(agentWebAppCapabilityReasonNoAccess)
			}
		}
	}

	return &dto.AgentWebAppRuntimeCapabilityResponse{
		AgentID:                ag.ID.String(),
		WebAppID:               ag.WebAppID.String(),
		WorkspaceID:            workspaceID,
		OrganizationID:         organizationID,
		Surface:                string(runtimeauth.PublishedRuntimeSurfaceWebApp),
		Allowed:                decision.Allowed,
		Reason:                 string(webAppCapabilityReason(decision, fallback, surface)),
		PublicOnly:             !privateAudienceEnabled,
		PrivateAudienceEnabled: privateAudienceEnabled,
		SupportedSubjectTypes: []string{
			string(runtimeauth.PublishedRuntimeSubjectPublic),
			string(runtimeauth.PublishedRuntimeSubjectOrganization),
			string(runtimeauth.PublishedRuntimeSubjectDepartment),
			string(runtimeauth.PublishedRuntimeSubjectWorkspace),
			string(runtimeauth.PublishedRuntimeSubjectAccount),
		},
		VersionUUID: version.VersionUUID.String(),
	}, nil
}

func webAppCapabilityReason(decision runtimeauth.RuntimeAccessDecision, fallback runtimeauth.PublishedRuntimePolicy, surface runtimeauth.SurfaceAuthorization) runtimeauth.RuntimeAccessDecisionReason {
	if decision.Allowed && len(surface.Grants) == 0 && fallback.Allows(runtimeauth.PublishedRuntimeSurfaceWebApp) {
		return runtimeauth.RuntimeAccessDecisionReason(agentWebAppCapabilityReasonPublicCompatible)
	}
	return decision.Reason
}

func webAppSurfaceHasPrivateAudience(surface runtimeauth.SurfaceAuthorization) bool {
	for _, grant := range surface.Grants {
		if !grant.Enabled {
			continue
		}
		switch grant.SubjectType {
		case runtimeauth.PublishedRuntimeSubjectOrganization,
			runtimeauth.PublishedRuntimeSubjectAccount,
			runtimeauth.PublishedRuntimeSubjectDepartment,
			runtimeauth.PublishedRuntimeSubjectWorkspace:
			return true
		}
	}
	return false
}

func (s *agentsService) webAppRuntimeAudienceForAccount(ctx context.Context, organizationID uuid.UUID, accountID string) (runtimeauth.RuntimeAudience, error) {
	accountUUID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil || organizationID == uuid.Nil {
		return runtimeauth.RuntimeAudience{}, nil
	}
	audience := runtimeauth.RuntimeAudience{AccountID: accountUUID}
	if s.db == nil {
		audience.OrganizationID = organizationID
		return audience, nil
	}

	var memberCount int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID.String(), accountUUID.String(), workspacemodel.OrganizationMemberStatusActive).
		Count(&memberCount).Error; err != nil {
		return runtimeauth.RuntimeAudience{}, fmt.Errorf("failed to validate webapp runtime organization member: %w", err)
	}
	if memberCount == 0 {
		return runtimeauth.RuntimeAudience{}, nil
	}
	audience.OrganizationID = organizationID

	departmentIDs, err := s.runnableWebAppDepartmentIDsForAudience(ctx, audience)
	if err != nil {
		return runtimeauth.RuntimeAudience{}, err
	}
	audience.DepartmentIDs = departmentIDs

	workspaceIDs, err := s.runnableWebAppWorkspaceIDsForAudience(ctx, audience)
	if err != nil {
		return runtimeauth.RuntimeAudience{}, err
	}
	audience.WorkspaceIDs = workspaceIDs
	return audience, nil
}

func (s *agentsService) webAppRuntimeOrganizationID(ctx context.Context, ag *Agent, auth *runtimeauth.ResourceAuthorization) string {
	if auth != nil && auth.OrganizationID != uuid.Nil {
		return auth.OrganizationID.String()
	}
	workspaceID := ""
	if ag != nil {
		workspaceID = ag.TenantID.String()
	}
	return s.organizationIDForAgentWorkspace(ctx, workspaceID)
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
