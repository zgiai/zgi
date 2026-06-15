package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
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

	workspaceIDs, err := s.runnableWebAppWorkspaceIDs(ctx, accountID, currentOrganization)
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

func (s *agentsService) runnableWebAppWorkspaceIDs(ctx context.Context, accountID string, currentOrganization *model.OrganizationMember) ([]string, error) {
	if currentOrganization.Role == model.OrganizationRoleOwner || currentOrganization.Role == model.OrganizationRoleAdmin {
		workspaces, err := s.enterpriseService.GetOrganizationWorkspacesList(ctx, currentOrganization.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("failed to get organization workspaces: %w", err)
		}
		return normalWorkspaceIDs(workspaces, currentOrganization.OrganizationID), nil
	}

	joins, err := s.tenantService.GetAccountWorkspaceJoins(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account workspace joins: %w", err)
	}
	if len(joins) == 0 {
		return []string{}, nil
	}

	workspaceIDSet := make(map[string]struct{}, len(joins))
	workspaceIDs := make([]string, 0, len(joins))
	for _, join := range joins {
		if join == nil || join.WorkspaceID == "" {
			continue
		}
		if _, exists := workspaceIDSet[join.WorkspaceID]; exists {
			continue
		}
		workspaceIDSet[join.WorkspaceID] = struct{}{}
		workspaceIDs = append(workspaceIDs, join.WorkspaceID)
	}
	if len(workspaceIDs) == 0 {
		return []string{}, nil
	}

	workspaces, err := s.tenantService.GetWorkspacesByIDs(ctx, workspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get joined workspaces: %w", err)
	}
	return normalWorkspaceIDs(workspaces, currentOrganization.OrganizationID), nil
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
