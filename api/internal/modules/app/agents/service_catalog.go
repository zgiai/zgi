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

	workspaceIDs, err := s.runnableWebAppWorkspaceIDs(ctx, currentOrganization)
	if err != nil {
		return nil, err
	}

	resp := &dto.RunnableWebAppsResponse{
		Items: make([]dto.RunnableWebAppItem, 0),
	}

	if len(workspaceIDs) == 0 {
		return resp, nil
	}

	if req.WorkspaceID != "" && !slices.Contains(workspaceIDs, req.WorkspaceID) {
		return resp, nil
	}

	items, err := s.agentsRepo.ListRunnableWebApps(ctx, workspaceIDs, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list runnable web apps: %w", err)
	}
	items, err = s.filterRunnableWebAppsByRuntimeAuthorization(ctx, items, currentOrganization.OrganizationID, accountID)
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
				AgentType: item.AgentType,
			},
		})
	}

	return resp, nil
}

func (s *agentsService) runnableWebAppWorkspaceIDs(ctx context.Context, currentOrganization *model.OrganizationMember) ([]string, error) {
	workspaces, err := s.enterpriseService.GetOrganizationWorkspacesList(ctx, currentOrganization.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization workspaces: %w", err)
	}
	return normalWorkspaceIDs(workspaces, currentOrganization.OrganizationID), nil
}

func (s *agentsService) filterRunnableWebAppsByRuntimeAuthorization(ctx context.Context, items []runnableWebAppItem, organizationID, accountID string) ([]runnableWebAppItem, error) {
	if len(items) == 0 || s.db == nil {
		return items, nil
	}

	organizationUUID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return nil, fmt.Errorf("invalid organization id for runnable web app authorization: %w", err)
	}
	accountUUID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("invalid account id for runnable web app authorization: %w", err)
	}

	store := runtimeauth.NewStore(s.db)
	audience := runtimeauth.RuntimeAudience{
		OrganizationID: organizationUUID,
		AccountID:      accountUUID,
	}
	departmentIDs, err := s.runnableWebAppDepartmentIDsForAudience(ctx, audience)
	if err != nil {
		return nil, err
	}
	audience.DepartmentIDs = departmentIDs

	candidates := make([]runtimeauth.ResourceAuthorizationCandidate, 0, len(items))
	for _, item := range items {
		agentID, err := uuid.Parse(strings.TrimSpace(item.AgentID))
		if err != nil {
			return nil, fmt.Errorf("invalid agent id for runnable web app authorization: %w", err)
		}

		candidates = append(candidates, runtimeauth.ResourceAuthorizationCandidate{
			ResourceID: agentID,
			Fallback:   runtimeauth.PolicyFromAgentFields(item.WebAppStatus, false),
		})
	}

	allowedAgentIDs, err := store.FilterAuthorizedResourceIDs(ctx, runtimeauth.PublishedRuntimeResourceAgent, runtimeauth.PublishedRuntimeSurfaceBuiltinApp, organizationUUID, candidates, audience)
	if err != nil {
		return nil, fmt.Errorf("failed to filter runnable web app authorization: %w", err)
	}
	allowedAgentIDSet := make(map[uuid.UUID]struct{}, len(allowedAgentIDs))
	for _, agentID := range allowedAgentIDs {
		allowedAgentIDSet[agentID] = struct{}{}
	}

	filtered := make([]runnableWebAppItem, 0, len(items))
	for _, item := range items {
		agentID, err := uuid.Parse(strings.TrimSpace(item.AgentID))
		if err != nil {
			return nil, fmt.Errorf("invalid agent id for runnable web app authorization: %w", err)
		}
		if _, ok := allowedAgentIDSet[agentID]; ok {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
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
