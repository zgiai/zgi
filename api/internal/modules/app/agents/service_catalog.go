package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type textIcon struct {
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
}

func convertBase64IconToText(name string, base64Icon string) (icon string, iconType string) {
	if !strings.HasPrefix(base64Icon, "data:image") {
		return base64Icon, "base64"
	}

	namePrefix := getNamePrefix(name)
	tIcon := textIcon{
		Icon:           namePrefix,
		IconBackground: "#000000",
	}
	iconJSON, _ := json.Marshal(tIcon)
	return string(iconJSON), "text"
}

func getNamePrefix(name string) string {
	name = strings.TrimSpace(name)
	runeCount := 0
	for range name {
		runeCount++
	}

	if runeCount >= 2 {
		runes := []rune(name)
		return string(runes[0]) + string(runes[1])
	} else if runeCount == 1 {
		runes := []rune(name)
		return string(runes[0])
	}
	return "?"
}

func (s *agentsService) GetRunnableWebApps(ctx context.Context, accountID string, req dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error) {
	currentOrganization, err := s.tenantService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current organization: %w", err)
	}
	if currentOrganization == nil || currentOrganization.OrganizationID == "" {
		return nil, errCurrentOrganizationNotFound
	}

	visibleWorkspaceIDs, err := s.runnableWebAppWorkspaceIDs(ctx, currentOrganization)
	if err != nil {
		return nil, err
	}

	resp := &dto.RunnableWebAppsResponse{
		Items: make([]dto.RunnableWebAppItem, 0),
	}

	if req.WorkspaceID != "" && !slices.Contains(visibleWorkspaceIDs, req.WorkspaceID) {
		return resp, nil
	}

	candidateWorkspaceIDs := visibleWorkspaceIDs
	if req.WorkspaceID == "" && s.db != nil {
		candidateWorkspaceIDs, err = s.runnableWebAppOrganizationWorkspaceIDs(ctx, currentOrganization)
		if err != nil {
			return nil, err
		}
	}
	if len(candidateWorkspaceIDs) == 0 {
		return resp, nil
	}

	items, err := s.agentsRepo.ListRunnableWebApps(ctx, candidateWorkspaceIDs, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list runnable web apps: %w", err)
	}
	items, err = s.filterRunnableWebAppsByAppCenterAuthorization(ctx, currentOrganization, items)
	if err != nil {
		return nil, err
	}

	resp.Items = make([]dto.RunnableWebAppItem, 0, len(items))
	for _, item := range items {
		icon := item.AgentIcon
		iconType := item.AgentIconType
		iconUrl := ""

		if iconType != nil && *iconType == "base64" && icon != nil && strings.HasPrefix(*icon, "data:image") {
			convertedIcon, convertedType := convertBase64IconToText(item.AgentName, *icon)
			icon = &convertedIcon
			iconType = &convertedType
		} else if iconType != nil && *iconType == "image" && icon != nil && *icon != "" {
			fileURL, err := s.fileService.GetFileURL(ctx, *icon)
			if err != nil {
				logger.Warn(fmt.Sprintf("failed to get file URL for icon %s: %v", *icon, err))
			} else {
				iconUrl = fileURL
			}
		}

		resp.Items = append(resp.Items, dto.RunnableWebAppItem{
			AgentID:      item.AgentID,
			WorkspaceID:  item.WorkspaceID,
			WebAppID:     item.WebAppID,
			WebAppStatus: string(NormalizeAgentWebAppStatus(AgentWebAppStatus(item.WebAppStatus))),
			MetaData: dto.RunnableWebAppMetaData{
				Name:      item.AgentName,
				Icon:      icon,
				IconType:  iconType,
				IconUrl:   iconUrl,
				Desc:      item.AgentDesc,
				AgentType: normalizeAgentTypeForResponse(item.AgentType),
			},
		})
	}

	return resp, nil
}

type runnableWebAppAuthorizationCandidate struct {
	item        runnableWebAppItem
	resourceID  uuid.UUID
	workspaceID uuid.UUID
}

func (s *agentsService) filterRunnableWebAppsByAppCenterAuthorization(ctx context.Context, currentOrganization *model.OrganizationMember, items []runnableWebAppItem) ([]runnableWebAppItem, error) {
	items = activeRunnableWebAppItems(items)
	if len(items) == 0 || s.db == nil {
		return items, nil
	}
	if currentOrganization == nil {
		return nil, errCurrentOrganizationNotFound
	}

	organizationID, err := uuid.Parse(strings.TrimSpace(currentOrganization.OrganizationID))
	if err != nil {
		return nil, fmt.Errorf("invalid app center organization id: %w", err)
	}
	accountID, err := uuid.Parse(strings.TrimSpace(currentOrganization.AccountID))
	if err != nil {
		return nil, fmt.Errorf("invalid app center account id: %w", err)
	}

	parsedCandidates := make([]runnableWebAppAuthorizationCandidate, 0, len(items))
	candidates := make([]runtimeauth.ResourceAuthorizationCandidate, 0, len(items))
	resourceIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		resourceID, err := uuid.Parse(strings.TrimSpace(item.AgentID))
		if err != nil {
			return nil, fmt.Errorf("invalid runnable web app agent id: %w", err)
		}
		workspaceID, err := uuid.Parse(strings.TrimSpace(item.WorkspaceID))
		if err != nil {
			return nil, fmt.Errorf("invalid runnable web app workspace id: %w", err)
		}
		parsedCandidates = append(parsedCandidates, runnableWebAppAuthorizationCandidate{
			item:        item,
			resourceID:  resourceID,
			workspaceID: workspaceID,
		})
		candidates = append(candidates, runtimeauth.ResourceAuthorizationCandidate{
			ResourceID: resourceID,
			Fallback:   runtimeauth.PublishedRuntimePolicy{},
		})
		resourceIDs = append(resourceIDs, resourceID)
	}
	if len(parsedCandidates) == 0 {
		return []runnableWebAppItem{}, nil
	}

	audience := runtimeauth.RuntimeAudience{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}
	departmentIDs, err := s.runnableWebAppDepartmentIDsForAudience(ctx, audience)
	if err != nil {
		return nil, err
	}
	workspaceIDs, err := s.runnableWebAppWorkspaceIDsForAudience(ctx, audience)
	if err != nil {
		return nil, err
	}
	audience.DepartmentIDs = departmentIDs
	audience.WorkspaceIDs = workspaceIDs

	persistedSurfaceIDs, err := s.persistedAppCenterSurfaceResourceIDs(ctx, organizationID, resourceIDs)
	if err != nil {
		return nil, err
	}
	authorizedIDs, err := runtimeauth.NewStore(s.db).FilterAuthorizedResourceIDs(
		ctx,
		runtimeauth.PublishedRuntimeResourceAgent,
		runtimeauth.PublishedRuntimeSurfaceAppCenter,
		organizationID,
		candidates,
		audience,
	)
	if err != nil {
		return nil, err
	}
	authorizedSet := make(map[uuid.UUID]struct{}, len(authorizedIDs))
	for _, id := range authorizedIDs {
		authorizedSet[id] = struct{}{}
	}
	audienceWorkspaceSet := make(map[uuid.UUID]struct{}, len(audience.WorkspaceIDs))
	for _, workspaceID := range audience.WorkspaceIDs {
		audienceWorkspaceSet[workspaceID] = struct{}{}
	}

	out := make([]runnableWebAppItem, 0, len(items))
	for _, candidate := range parsedCandidates {
		if _, ok := authorizedSet[candidate.resourceID]; ok {
			out = append(out, candidate.item)
			continue
		}
		if _, hasPersistedSurface := persistedSurfaceIDs[candidate.resourceID]; hasPersistedSurface {
			continue
		}
		if _, workspaceVisible := audienceWorkspaceSet[candidate.workspaceID]; workspaceVisible {
			out = append(out, candidate.item)
		}
	}
	return out, nil
}

func activeRunnableWebAppItems(items []runnableWebAppItem) []runnableWebAppItem {
	if len(items) == 0 {
		return items
	}
	out := make([]runnableWebAppItem, 0, len(items))
	for _, item := range items {
		if NormalizeAgentWebAppStatus(AgentWebAppStatus(item.WebAppStatus)) == AgentWebAppStatusActive {
			out = append(out, item)
		}
	}
	return out
}

func (s *agentsService) persistedAppCenterSurfaceResourceIDs(ctx context.Context, organizationID uuid.UUID, resourceIDs []uuid.UUID) (map[uuid.UUID]struct{}, error) {
	out := make(map[uuid.UUID]struct{})
	if s.db == nil || organizationID == uuid.Nil || len(resourceIDs) == 0 {
		return out, nil
	}

	type row struct {
		ResourceID uuid.UUID `gorm:"column:resource_id"`
	}
	var rows []row
	if err := s.db.WithContext(ctx).
		Table("published_runtime_surfaces").
		Select("resource_id").
		Where("resource_type = ? AND surface = ? AND organization_id = ? AND resource_id IN ? AND deleted_at IS NULL",
			string(runtimeauth.PublishedRuntimeResourceAgent),
			string(runtimeauth.PublishedRuntimeSurfaceAppCenter),
			organizationID,
			resourceIDs,
		).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load app center runtime surfaces: %w", err)
	}

	for _, row := range rows {
		out[row.ResourceID] = struct{}{}
	}
	return out, nil
}

func (s *agentsService) runnableWebAppWorkspaceIDs(ctx context.Context, currentOrganization *model.OrganizationMember) ([]string, error) {
	if currentOrganization == nil || strings.TrimSpace(currentOrganization.OrganizationID) == "" || strings.TrimSpace(currentOrganization.AccountID) == "" {
		return nil, nil
	}
	workspaceIDs, err := s.enterpriseService.ListWorkspaceIDsByPermission(
		ctx,
		currentOrganization.OrganizationID,
		currentOrganization.AccountID,
		model.WorkspacePermissionWorkspaceView,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list app center workspaces: %w", err)
	}
	return workspaceIDs, nil
}

func (s *agentsService) runnableWebAppOrganizationWorkspaceIDs(ctx context.Context, currentOrganization *model.OrganizationMember) ([]string, error) {
	if currentOrganization == nil || strings.TrimSpace(currentOrganization.OrganizationID) == "" {
		return nil, nil
	}

	workspaces, err := s.enterpriseService.GetOrganizationWorkspacesList(ctx, currentOrganization.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list app center organization workspaces: %w", err)
	}
	return normalWorkspaceIDs(workspaces, currentOrganization.OrganizationID), nil
}

func (s *agentsService) runnableWebAppDepartmentIDsForAudience(ctx context.Context, audience runtimeauth.RuntimeAudience) ([]uuid.UUID, error) {
	if s.db == nil || audience.OrganizationID == uuid.Nil || audience.AccountID == uuid.Nil {
		return nil, nil
	}

	var rawDepartmentIDs []string
	if err := s.db.WithContext(ctx).
		Table("department_members").
		Select("department_members.department_id").
		Joins("JOIN departments ON departments.id = department_members.department_id").
		Where("department_members.account_id = ? AND departments.group_id = ? AND departments.status = ?", audience.AccountID, audience.OrganizationID, model.DepartmentStatusActive).
		Scan(&rawDepartmentIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load runnable web app audience departments: %w", err)
	}

	departmentIDs := make([]uuid.UUID, 0, len(rawDepartmentIDs))
	for _, rawDepartmentID := range rawDepartmentIDs {
		departmentID, err := uuid.Parse(strings.TrimSpace(rawDepartmentID))
		if err != nil {
			return nil, fmt.Errorf("invalid runnable web app audience department id: %w", err)
		}
		departmentIDs = append(departmentIDs, departmentID)
	}
	return departmentIDs, nil
}

func (s *agentsService) runnableWebAppWorkspaceIDsForAudience(ctx context.Context, audience runtimeauth.RuntimeAudience) ([]uuid.UUID, error) {
	if s.db == nil || audience.OrganizationID == uuid.Nil || audience.AccountID == uuid.Nil {
		return nil, nil
	}

	if s.enterpriseService != nil {
		rawWorkspaceIDs, err := s.enterpriseService.ListWorkspaceIDsByPermission(
			ctx,
			audience.OrganizationID.String(),
			audience.AccountID.String(),
			model.WorkspacePermissionWorkspaceView,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load runnable web app audience workspaces: %w", err)
		}
		return parseUUIDStrings(rawWorkspaceIDs, "runnable web app audience workspace id")
	}

	var rawWorkspaceIDs []string
	if err := s.db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_members.workspace_id").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.account_id = ? AND workspaces.organization_id = ? AND workspaces.status = ?", audience.AccountID, audience.OrganizationID, model.WorkspaceStatusNormal).
		Scan(&rawWorkspaceIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load runnable web app audience workspaces: %w", err)
	}

	workspaceIDs := make([]uuid.UUID, 0, len(rawWorkspaceIDs))
	for _, rawWorkspaceID := range rawWorkspaceIDs {
		workspaceID, err := uuid.Parse(strings.TrimSpace(rawWorkspaceID))
		if err != nil {
			return nil, fmt.Errorf("invalid runnable web app audience workspace id: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	return workspaceIDs, nil
}

func normalWorkspaceIDs(workspaces []*model.Workspace, organizationID string) []string {
	ids := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil || workspace.Status != model.WorkspaceStatusNormal {
			continue
		}
		if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
			continue
		}
		ids = append(ids, workspace.ID)
	}
	return ids
}
