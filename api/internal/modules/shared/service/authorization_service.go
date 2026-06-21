package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

var ErrAuthorizationDenied = errors.New("authorization denied")

type authorizationServiceImpl struct {
	organizationService interfaces.OrganizationService
}

func NewAuthorizationService(organizationService interfaces.OrganizationService) interfaces.AuthorizationService {
	return &authorizationServiceImpl{
		organizationService: organizationService,
	}
}

func (s *authorizationServiceImpl) RequireOrganizationMember(ctx context.Context, req interfaces.OrganizationScopeRequest) (*interfaces.OrganizationScope, error) {
	organizationID := strings.TrimSpace(req.OrganizationID)
	accountID := strings.TrimSpace(req.AccountID)
	if organizationID == "" || accountID == "" {
		return nil, fmt.Errorf("organization id and account id are required")
	}
	if s.organizationService == nil {
		return nil, fmt.Errorf("organization service is not initialized")
	}

	isMember, err := s.organizationService.IsOrganizationMember(ctx, organizationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check organization membership: %w", err)
	}
	if !isMember {
		return nil, ErrAuthorizationDenied
	}

	role, err := s.organizationService.GetUserOrganizationRole(ctx, organizationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization role: %w", err)
	}

	return &interfaces.OrganizationScope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		Role:           role,
		IsAdmin:        role == model.OrganizationRoleOwner || role == model.OrganizationRoleAdmin,
	}, nil
}

func (s *authorizationServiceImpl) CanUseOrganizationFeature(ctx context.Context, req interfaces.OrganizationFeatureRequest) (bool, error) {
	_, err := s.RequireOrganizationMember(ctx, interfaces.OrganizationScopeRequest{
		OrganizationID: req.OrganizationID,
		AccountID:      req.AccountID,
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrAuthorizationDenied) {
		return false, nil
	}
	return false, err
}

func (s *authorizationServiceImpl) RequireWorkspacePermission(ctx context.Context, req interfaces.WorkspaceScopeRequest) (*interfaces.WorkspaceScope, error) {
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	if len(req.PermissionCodes) == 0 {
		return nil, fmt.Errorf("at least one workspace permission code is required")
	}

	organizationScope, err := s.RequireOrganizationMember(ctx, interfaces.OrganizationScopeRequest{
		OrganizationID: req.OrganizationID,
		AccountID:      req.AccountID,
	})
	if err != nil {
		return nil, err
	}

	if err := s.ensureNormalWorkspaceInOrganization(ctx, organizationScope.OrganizationID, workspaceID); err != nil {
		return nil, err
	}

	if organizationScope.IsAdmin {
		return &interfaces.WorkspaceScope{
			OrganizationScope: *organizationScope,
			WorkspaceID:       workspaceID,
			PermissionCodes:   req.PermissionCodes,
			WorkspaceIsAdmin:  true,
		}, nil
	}

	hasPermission, err := s.organizationService.CheckWorkspaceOrganizationAnyPermission(
		ctx,
		organizationScope.OrganizationID,
		workspaceID,
		organizationScope.AccountID,
		req.PermissionCodes...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to check workspace permission: %w", err)
	}
	if !hasPermission {
		return nil, ErrAuthorizationDenied
	}

	return &interfaces.WorkspaceScope{
		OrganizationScope: *organizationScope,
		WorkspaceID:       workspaceID,
		PermissionCodes:   req.PermissionCodes,
	}, nil
}

func (s *authorizationServiceImpl) ListWorkspaceIDsByPermission(ctx context.Context, req interfaces.WorkspaceListPermissionRequest) ([]string, error) {
	if req.PermissionCode == "" {
		return nil, fmt.Errorf("workspace permission code is required")
	}
	organizationScope, err := s.RequireOrganizationMember(ctx, interfaces.OrganizationScopeRequest{
		OrganizationID: req.OrganizationID,
		AccountID:      req.AccountID,
	})
	if err != nil {
		return nil, err
	}

	workspaceIDs, err := s.organizationService.ListWorkspaceIDsByPermission(
		ctx,
		organizationScope.OrganizationID,
		organizationScope.AccountID,
		req.PermissionCode,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace ids by permission: %w", err)
	}
	return workspaceIDs, nil
}

func (s *authorizationServiceImpl) ensureNormalWorkspaceInOrganization(ctx context.Context, organizationID, workspaceID string) error {
	workspaces, err := s.organizationService.GetOrganizationWorkspacesList(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to list organization workspaces: %w", err)
	}

	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		if workspace.ID == workspaceID && workspace.Status == model.WorkspaceStatusNormal {
			return nil
		}
	}

	return ErrAuthorizationDenied
}
