package visibility

import (
	"context"
	"slices"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type VisibleWorkspaceScope struct {
	WorkspaceIDs            []string
	AllowOrganizationScoped bool
}

func ResolveVisibleWorkspaceScope(
	ctx context.Context,
	organizationService interfaces.OrganizationService,
	organizationID, accountID, workspaceID string,
	permissionCodes ...workspace_model.WorkspacePermissionCode,
) (VisibleWorkspaceScope, error) {
	if organizationService == nil || organizationID == "" || accountID == "" {
		return VisibleWorkspaceScope{}, nil
	}

	workspaces, err := organizationService.GetOrganizationWorkspacesList(ctx, organizationID)
	if err != nil {
		return VisibleWorkspaceScope{}, err
	}

	normalWorkspaceIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil || workspace.Status != workspace_model.WorkspaceStatusNormal {
			continue
		}
		normalWorkspaceIDs = append(normalWorkspaceIDs, workspace.ID)
	}

	if workspaceID != "" && !slices.Contains(normalWorkspaceIDs, workspaceID) {
		return VisibleWorkspaceScope{}, nil
	}

	workspaceIDs, err := ResolveVisibleWorkspaceIDs(
		ctx,
		organizationService,
		organizationID,
		accountID,
		workspaceID,
		permissionCodes...,
	)
	if err != nil {
		return VisibleWorkspaceScope{}, err
	}

	return VisibleWorkspaceScope{WorkspaceIDs: workspaceIDs}, nil
}

func ResolveVisibleWorkspaceIDs(
	ctx context.Context,
	organizationService interfaces.OrganizationService,
	organizationID, accountID, workspaceID string,
	permissionCodes ...workspace_model.WorkspacePermissionCode,
) ([]string, error) {
	workspaces, err := organizationService.GetOrganizationWorkspacesList(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	normalWorkspaceIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil || workspace.Status != workspace_model.WorkspaceStatusNormal {
			continue
		}
		normalWorkspaceIDs = append(normalWorkspaceIDs, workspace.ID)
	}

	if workspaceID != "" {
		if !slices.Contains(normalWorkspaceIDs, workspaceID) {
			return []string{}, nil
		}

		hasPermission, err := organizationService.CheckWorkspaceOrganizationAnyPermission(
			ctx,
			organizationID,
			workspaceID,
			accountID,
			permissionCodes...,
		)
		if err != nil {
			return nil, err
		}
		if !hasPermission {
			return []string{}, nil
		}

		return []string{workspaceID}, nil
	}

	visibleWorkspaceIDs := make([]string, 0, len(normalWorkspaceIDs))
	for _, id := range normalWorkspaceIDs {
		hasPermission, err := organizationService.CheckWorkspaceOrganizationAnyPermission(
			ctx,
			organizationID,
			id,
			accountID,
			permissionCodes...,
		)
		if err != nil {
			return nil, err
		}
		if hasPermission {
			visibleWorkspaceIDs = append(visibleWorkspaceIDs, id)
		}
	}

	return visibleWorkspaceIDs, nil
}
