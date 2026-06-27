package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

func (s *agentsService) GetAgentRuntimeSurfaces(ctx context.Context, agentID, accountID string) (*dto.AgentRuntimeSurfaceAuthorizationResponse, error) {
	agentID = strings.TrimSpace(agentID)
	accountID = strings.TrimSpace(accountID)
	if agentID == "" {
		return nil, fmt.Errorf("%w: agent id is required", runtimeservice.ErrInvalidInput)
	}
	if accountID == "" {
		return nil, runtimeservice.ErrUnauthorized
	}
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid agent id", runtimeservice.ErrInvalidInput)
	}

	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil || isSystemManagedAgent(ag) {
		return nil, fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	if err := s.ensureCanViewAgent(ctx, ag, accountID); err != nil {
		return nil, err
	}

	fallback := runtimeauth.PolicyFromAgentFields(string(ag.WebAppStatus), ag.EnableAPI)
	auth, err := runtimeauth.NewStore(s.db).GetResourceAuthorization(ctx, runtimeauth.PublishedRuntimeResourceAgent, agentUUID, fallback)
	if err != nil {
		return nil, err
	}
	auth.Surfaces = agentRuntimeSupportedSurfaces(auth.Surfaces, ag.TenantID)

	workspaceID := ag.TenantID.String()
	if auth.WorkspaceID != nil && *auth.WorkspaceID != uuid.Nil {
		workspaceID = auth.WorkspaceID.String()
	}

	organizationID := ""
	if auth.OrganizationID != uuid.Nil {
		organizationID = auth.OrganizationID.String()
	}
	if organizationID == "" {
		organizationID = s.organizationIDForAgentWorkspace(ctx, workspaceID)
	}
	if organizationID == "" {
		organizationID = workspaceID
	}

	return &dto.AgentRuntimeSurfaceAuthorizationResponse{
		AgentID:        ag.ID.String(),
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		Surfaces:       agentRuntimeSurfaceAuthorizationDTOs(auth.Surfaces),
	}, nil
}

func (s *agentsService) UpdateAgentRuntimeSurfaces(ctx context.Context, agentID, accountID string, req dto.UpdateAgentRuntimeSurfacesRequest) (*dto.AgentRuntimeSurfaceAuthorizationResponse, error) {
	agentID = strings.TrimSpace(agentID)
	accountID = strings.TrimSpace(accountID)
	if agentID == "" {
		return nil, fmt.Errorf("%w: agent id is required", runtimeservice.ErrInvalidInput)
	}
	if accountID == "" {
		return nil, runtimeservice.ErrUnauthorized
	}
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid agent id", runtimeservice.ErrInvalidInput)
	}
	if s.db == nil {
		return nil, fmt.Errorf("database is required")
	}

	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil || isSystemManagedAgent(ag) {
		return nil, fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	if err := s.ensureCanManageAgentRuntimeSurfaces(ctx, ag, accountID); err != nil {
		if strings.EqualFold(err.Error(), "permission denied") {
			return nil, runtimeservice.ErrPermissionDenied
		}
		return nil, err
	}

	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	auth, legacyUpdates, err := agentRuntimeAuthorizationFromUpdateRequest(agentUUID, ag.TenantID, organizationID, req)
	if err != nil {
		return nil, err
	}
	if err := s.validateAgentRuntimeGrantSubjects(ctx, organizationID, accountID, auth.Surfaces); err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := runtimeauth.NewStore(tx).SaveResourceAuthorization(ctx, auth); err != nil {
			return err
		}
		return syncAgentRuntimeSurfaceLegacyFields(ctx, tx, ag, accountID, legacyUpdates)
	}); err != nil {
		return nil, err
	}

	return s.GetAgentRuntimeSurfaces(ctx, agentID, accountID)
}

func (s *agentsService) publishedRuntimePolicyForAgent(ctx context.Context, ag *Agent) (runtimeauth.PublishedRuntimePolicy, error) {
	fallback, auth, err := s.publishedRuntimeAuthorizationForAgent(ctx, ag)
	if err != nil {
		return runtimeauth.PublishedRuntimePolicy{}, err
	}
	return runtimeauth.PolicyFromAuthorization(fallback, auth), nil
}

func (s *agentsService) publishedRuntimeAuthorizationForAgent(ctx context.Context, ag *Agent) (runtimeauth.PublishedRuntimePolicy, *runtimeauth.ResourceAuthorization, error) {
	if ag == nil {
		return runtimeauth.PublishedRuntimePolicy{}, nil, fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	fallback := runtimeauth.PolicyFromAgentFields(string(ag.WebAppStatus), ag.EnableAPI)
	auth, err := runtimeauth.NewStore(s.db).GetResourceAuthorization(ctx, runtimeauth.PublishedRuntimeResourceAgent, ag.ID, fallback)
	if err != nil {
		return runtimeauth.PublishedRuntimePolicy{}, nil, err
	}
	return fallback, auth, nil
}

func (s *agentsService) ensureCanViewAgent(ctx context.Context, ag *Agent, accountID string) error {
	if ag == nil {
		return fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return runtimeservice.ErrUnauthorized
	}

	creatorID := ""
	if ag.CreatedBy != nil {
		creatorID = ag.CreatedBy.String()
	}

	canView := false
	var err error
	callerOrganizationID := callerOrganizationIDFromContext(ctx)
	if callerOrganizationID != "" && s.enterpriseService != nil {
		canView, err = s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(
			ctx,
			callerOrganizationID,
			ag.TenantID.String(),
			accountID,
			agentAssetVisiblePermissionCodes()...,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("ensureCanViewAgent: failed to check workspace permission for agent %s, account %s", ag.ID.String(), accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	} else if s.resourcePermissionService != nil {
		canView, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID:       accountID,
			TenantID:        ag.TenantID.String(),
			OrganizationID:  callerOrganizationID,
			CreatedBy:       creatorID,
			GroupID:         nil,
			PermissionCodes: agentAssetVisiblePermissionCodes(),
		})
		if err != nil {
			logger.Error(fmt.Sprintf("ensureCanViewAgent: failed to check legacy resource permission for agent %s, account %s", ag.ID.String(), accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	} else if creatorID != "" && strings.EqualFold(creatorID, accountID) {
		canView = true
	}
	if !canView {
		return runtimeservice.ErrPermissionDenied
	}
	return nil
}

type agentRuntimeLegacyUpdates struct {
	webAppStatus *AgentWebAppStatus
	apiEnabled   *bool
}

func agentRuntimeAuthorizationFromUpdateRequest(agentID, workspaceID, organizationID uuid.UUID, req dto.UpdateAgentRuntimeSurfacesRequest) (runtimeauth.ResourceAuthorization, agentRuntimeLegacyUpdates, error) {
	if len(req.Surfaces) == 0 {
		return runtimeauth.ResourceAuthorization{}, agentRuntimeLegacyUpdates{}, fmt.Errorf("%w: at least one runtime surface is required", runtimeservice.ErrInvalidInput)
	}

	seen := make(map[runtimeauth.PublishedRuntimeSurface]struct{}, len(req.Surfaces))
	surfaces := make([]runtimeauth.SurfaceAuthorization, 0, len(req.Surfaces))
	legacyUpdates := agentRuntimeLegacyUpdates{}

	for _, item := range req.Surfaces {
		surface := runtimeauth.NormalizeSurface(item.Surface)
		if !runtimeauth.IsKnownSurface(surface) {
			return runtimeauth.ResourceAuthorization{}, agentRuntimeLegacyUpdates{}, fmt.Errorf("%w: unknown runtime surface", runtimeservice.ErrInvalidInput)
		}
		if surface == runtimeauth.PublishedRuntimeSurfaceBuiltinApp {
			continue
		}
		if _, exists := seen[surface]; exists {
			return runtimeauth.ResourceAuthorization{}, agentRuntimeLegacyUpdates{}, fmt.Errorf("%w: duplicate runtime surface", runtimeservice.ErrInvalidInput)
		}
		seen[surface] = struct{}{}

		if surface == runtimeauth.PublishedRuntimeSurfaceInternal && !item.Enabled {
			return runtimeauth.ResourceAuthorization{}, agentRuntimeLegacyUpdates{}, fmt.Errorf("%w: internal runtime surface cannot be disabled", runtimeservice.ErrInvalidInput)
		}

		grants, err := agentRuntimeSurfaceGrantsFromRequest(surface, organizationID, workspaceID, item.Enabled, item.Grants)
		if err != nil {
			return runtimeauth.ResourceAuthorization{}, agentRuntimeLegacyUpdates{}, err
		}
		surfaces = append(surfaces, runtimeauth.SurfaceAuthorization{
			Surface:             surface,
			Enabled:             item.Enabled,
			CompatibilitySource: runtimeauth.PublishedRuntimeSourceGrant,
			Grants:              grants,
		})

		switch surface {
		case runtimeauth.PublishedRuntimeSurfaceWebApp:
			status := AgentWebAppStatusInactive
			if item.Enabled {
				status = AgentWebAppStatusActive
			}
			legacyUpdates.webAppStatus = &status
		case runtimeauth.PublishedRuntimeSurfaceAPI:
			enabled := item.Enabled
			legacyUpdates.apiEnabled = &enabled
		}
	}

	if len(surfaces) == 0 {
		return runtimeauth.ResourceAuthorization{}, agentRuntimeLegacyUpdates{}, fmt.Errorf("%w: no supported agent runtime surface provided", runtimeservice.ErrInvalidInput)
	}

	workspaceIDCopy := workspaceID
	return runtimeauth.ResourceAuthorization{
		ResourceType:   runtimeauth.PublishedRuntimeResourceAgent,
		ResourceID:     agentID,
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceIDCopy,
		Surfaces:       surfaces,
	}, legacyUpdates, nil
}

func (s *agentsService) validateAgentRuntimeGrantSubjects(ctx context.Context, organizationID uuid.UUID, accountID string, surfaces []runtimeauth.SurfaceAuthorization) error {
	isOrganizationAdmin := false
	isOrganizationAdminLoaded := false
	loadOrganizationAdmin := func() (bool, error) {
		if isOrganizationAdminLoaded {
			return isOrganizationAdmin, nil
		}
		ok, err := s.runtimeGrantOperatorIsOrganizationAdmin(ctx, organizationID, accountID)
		if err != nil {
			return false, err
		}
		isOrganizationAdmin = ok
		isOrganizationAdminLoaded = true
		return isOrganizationAdmin, nil
	}

	for _, surface := range surfaces {
		for _, grant := range surface.Grants {
			if grant.SubjectID == nil {
				continue
			}
			switch grant.SubjectType {
			case runtimeauth.PublishedRuntimeSubjectOrganization:
				if *grant.SubjectID != organizationID {
					return fmt.Errorf("%w: runtime grant organization is not current organization", runtimeservice.ErrInvalidInput)
				}
				isOrganizationAdmin, err := loadOrganizationAdmin()
				if err != nil {
					return err
				}
				if !isOrganizationAdmin {
					return fmt.Errorf("%w: only organization owners or admins can grant organization-wide access", runtimeservice.ErrPermissionDenied)
				}
			case runtimeauth.PublishedRuntimeSubjectAccount:
				ok, err := s.runtimeGrantAccountInOrganization(ctx, organizationID, *grant.SubjectID)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("%w: runtime grant account is not in organization", runtimeservice.ErrInvalidInput)
				}
				isOrganizationAdmin, err := loadOrganizationAdmin()
				if err != nil {
					return err
				}
				if !isOrganizationAdmin {
					ok, err = s.runtimeGrantAccountVisibleToOperator(ctx, organizationID, accountID, *grant.SubjectID)
					if err != nil {
						return err
					}
					if !ok {
						return fmt.Errorf("%w: runtime grant account is not visible to current operator", runtimeservice.ErrPermissionDenied)
					}
				}
			case runtimeauth.PublishedRuntimeSubjectDepartment:
				ok, err := s.runtimeGrantDepartmentInOrganization(ctx, organizationID, *grant.SubjectID)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("%w: runtime grant department is not in organization", runtimeservice.ErrInvalidInput)
				}
				isOrganizationAdmin, err := loadOrganizationAdmin()
				if err != nil {
					return err
				}
				if !isOrganizationAdmin {
					ok, err = s.runtimeGrantDepartmentVisibleToOperator(ctx, organizationID, accountID, *grant.SubjectID)
					if err != nil {
						return err
					}
					if !ok {
						return fmt.Errorf("%w: runtime grant department is not visible to current operator", runtimeservice.ErrPermissionDenied)
					}
				}
			case runtimeauth.PublishedRuntimeSubjectWorkspace:
				ok, err := s.runtimeGrantWorkspaceInOrganization(ctx, organizationID, *grant.SubjectID)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("%w: runtime grant workspace is not in organization", runtimeservice.ErrInvalidInput)
				}
				isOrganizationAdmin, err := loadOrganizationAdmin()
				if err != nil {
					return err
				}
				if !isOrganizationAdmin {
					ok, err = s.runtimeGrantWorkspaceVisibleToOperator(ctx, organizationID, accountID, *grant.SubjectID)
					if err != nil {
						return err
					}
					if !ok {
						return fmt.Errorf("%w: runtime grant workspace is not visible to current operator", runtimeservice.ErrPermissionDenied)
					}
				}
			}
		}
	}
	return nil
}

func (s *agentsService) runtimeGrantOperatorIsOrganizationAdmin(ctx context.Context, organizationID uuid.UUID, accountID string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database is required")
	}
	accountID = strings.TrimSpace(accountID)
	if organizationID == uuid.Nil || accountID == "" {
		return false, runtimeservice.ErrUnauthorized
	}

	var member workspacemodel.OrganizationMember
	err := s.db.WithContext(ctx).
		Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID.String(), accountID, workspacemodel.OrganizationMemberStatusActive).
		Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("%w: runtime grant operator is not in organization", runtimeservice.ErrPermissionDenied)
	}
	if err != nil {
		return false, fmt.Errorf("failed to load runtime grant operator organization role: %w", err)
	}
	return member.Role == workspacemodel.OrganizationRoleOwner || member.Role == workspacemodel.OrganizationRoleAdmin, nil
}

func (s *agentsService) runtimeGrantAccountInOrganization(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID.String(), accountID.String(), workspacemodel.OrganizationMemberStatusActive).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant account: %w", err)
	}
	return count > 0, nil
}

func (s *agentsService) runtimeGrantAccountVisibleToOperator(ctx context.Context, organizationID uuid.UUID, operatorAccountID string, targetAccountID uuid.UUID) (bool, error) {
	if strings.EqualFold(strings.TrimSpace(operatorAccountID), targetAccountID.String()) {
		return true, nil
	}

	departmentIDs, err := s.runtimeGrantVisibleDepartmentIDs(ctx, organizationID, operatorAccountID)
	if err != nil {
		return false, err
	}
	if len(departmentIDs) > 0 {
		var count int64
		if err := s.db.WithContext(ctx).
			Model(&workspacemodel.DepartmentMember{}).
			Where("account_id = ? AND department_id IN ?", targetAccountID.String(), uuidStrings(departmentIDs)).
			Count(&count).Error; err != nil {
			return false, fmt.Errorf("failed to validate runtime grant account department visibility: %w", err)
		}
		if count > 0 {
			return true, nil
		}
	}

	workspaceIDs, err := s.runtimeGrantVisibleWorkspaceIDs(ctx, organizationID, operatorAccountID)
	if err != nil {
		return false, err
	}
	if len(workspaceIDs) == 0 {
		return false, nil
	}

	var count int64
	if err := s.db.WithContext(ctx).
		Table("workspace_members").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.account_id = ?", targetAccountID.String()).
		Where("workspace_members.workspace_id IN ?", uuidStrings(workspaceIDs)).
		Where("workspaces.organization_id = ? AND workspaces.status = ?", organizationID.String(), workspacemodel.WorkspaceStatusNormal).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant account workspace visibility: %w", err)
	}
	return count > 0, nil
}

func (s *agentsService) runtimeGrantDepartmentInOrganization(ctx context.Context, organizationID, departmentID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.Department{}).
		Where("group_id = ? AND id = ? AND status = ?", organizationID.String(), departmentID.String(), workspacemodel.DepartmentStatusActive).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant department: %w", err)
	}
	return count > 0, nil
}

func (s *agentsService) runtimeGrantDepartmentVisibleToOperator(ctx context.Context, organizationID uuid.UUID, accountID string, departmentID uuid.UUID) (bool, error) {
	departmentIDs, err := s.runtimeGrantVisibleDepartmentIDs(ctx, organizationID, accountID)
	if err != nil {
		return false, err
	}
	for _, visibleDepartmentID := range departmentIDs {
		if visibleDepartmentID == departmentID {
			return true, nil
		}
	}
	return false, nil
}

func (s *agentsService) runtimeGrantWorkspaceInOrganization(ctx context.Context, organizationID, workspaceID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.Workspace{}).
		Where("organization_id = ? AND id = ? AND status = ?", organizationID.String(), workspaceID.String(), workspacemodel.WorkspaceStatusNormal).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant workspace: %w", err)
	}
	return count > 0, nil
}

func (s *agentsService) runtimeGrantWorkspaceVisibleToOperator(ctx context.Context, organizationID uuid.UUID, accountID string, workspaceID uuid.UUID) (bool, error) {
	workspaceIDs, err := s.runtimeGrantVisibleWorkspaceIDs(ctx, organizationID, accountID)
	if err != nil {
		return false, err
	}
	for _, visibleWorkspaceID := range workspaceIDs {
		if visibleWorkspaceID == workspaceID {
			return true, nil
		}
	}
	return false, nil
}

func (s *agentsService) runtimeGrantVisibleDepartmentIDs(ctx context.Context, organizationID uuid.UUID, accountID string) ([]uuid.UUID, error) {
	var rawDepartmentIDs []string
	if err := s.db.WithContext(ctx).Raw(`
WITH RECURSIVE visible_departments AS (
	SELECT departments.id
	FROM department_members
	JOIN departments ON departments.id = department_members.department_id
	WHERE department_members.account_id = ?
	  AND departments.group_id = ?
	  AND departments.status = ?
	UNION
	SELECT child.id
	FROM departments child
	JOIN visible_departments parent ON child.parent_id = parent.id
	WHERE child.group_id = ?
	  AND child.status = ?
)
SELECT id FROM visible_departments
`, strings.TrimSpace(accountID), organizationID.String(), workspacemodel.DepartmentStatusActive, organizationID.String(), workspacemodel.DepartmentStatusActive).
		Scan(&rawDepartmentIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load runtime grant visible departments: %w", err)
	}
	return parseUUIDStrings(rawDepartmentIDs, "runtime grant visible department id")
}

func (s *agentsService) runtimeGrantVisibleWorkspaceIDs(ctx context.Context, organizationID uuid.UUID, accountID string) ([]uuid.UUID, error) {
	type workspacePermissionRow struct {
		WorkspaceID      string
		Role             workspacemodel.WorkspaceMemberRole
		RoleID           *string
		Permissions      string
		PermissionSource workspacemodel.WorkspaceMemberPermissionSource
	}

	var rows []workspacePermissionRow
	if err := s.db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_members.workspace_id, workspace_members.role, workspace_members.role_id, COALESCE(workspace_members.permissions::text, '') AS permissions, workspace_members.permission_source").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.account_id = ?", strings.TrimSpace(accountID)).
		Where("workspaces.organization_id = ? AND workspaces.status = ?", organizationID.String(), workspacemodel.WorkspaceStatusNormal).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load runtime grant visible workspaces: %w", err)
	}

	workspaceIDs := make([]uuid.UUID, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if !runtimeGrantWorkspaceMemberAllowsPermission(
			row.Role,
			row.RoleID,
			runtimeGrantParseWorkspacePermissions(row.Permissions),
			row.PermissionSource,
			workspacemodel.WorkspacePermissionWorkspaceView,
		) {
			continue
		}
		if _, ok := seen[row.WorkspaceID]; ok {
			continue
		}
		seen[row.WorkspaceID] = struct{}{}
		workspaceID, err := uuid.Parse(strings.TrimSpace(row.WorkspaceID))
		if err != nil {
			return nil, fmt.Errorf("invalid runtime grant visible workspace id: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	return workspaceIDs, nil
}

func runtimeGrantParseWorkspacePermissions(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []string{}
	}
	var permissions []string
	if err := json.Unmarshal([]byte(raw), &permissions); err != nil {
		return []string{}
	}
	return permissions
}

func runtimeGrantWorkspaceMemberAllowsPermission(
	role workspacemodel.WorkspaceMemberRole,
	roleID *string,
	permissions []string,
	permissionSource workspacemodel.WorkspaceMemberPermissionSource,
	permission workspacemodel.WorkspacePermissionCode,
) bool {
	return workspacemodel.WorkspacePermissionStringsAllow(
		runtimeGrantWorkspaceMemberPermissions(role, roleID, permissions, permissionSource),
		permission,
	)
}

func runtimeGrantWorkspaceMemberPermissions(
	role workspacemodel.WorkspaceMemberRole,
	roleID *string,
	permissions []string,
	permissionSource workspacemodel.WorkspaceMemberPermissionSource,
) []string {
	return workspacemodel.EffectiveWorkspaceMemberPermissionStrings(role, roleID, permissions, permissionSource)
}

func agentRuntimeSurfaceGrantsFromRequest(surface runtimeauth.PublishedRuntimeSurface, organizationID, workspaceID uuid.UUID, surfaceEnabled bool, grants []dto.UpdateAgentRuntimeSurfaceGrant) ([]runtimeauth.SurfaceGrant, error) {
	if len(grants) == 0 {
		switch surface {
		case runtimeauth.PublishedRuntimeSurfaceWebApp:
			return []runtimeauth.SurfaceGrant{{
				SubjectType: runtimeauth.PublishedRuntimeSubjectPublic,
				Enabled:     surfaceEnabled,
			}}, nil
		case runtimeauth.PublishedRuntimeSurfaceAPI:
			return []runtimeauth.SurfaceGrant{{
				SubjectType: runtimeauth.PublishedRuntimeSubjectPublic,
				Enabled:     surfaceEnabled,
			}}, nil
		case runtimeauth.PublishedRuntimeSurfaceAppCenter:
			return []runtimeauth.SurfaceGrant{{
				SubjectType: runtimeauth.PublishedRuntimeSubjectWorkspace,
				SubjectID:   copyRuntimeUUIDPtr(workspaceID),
				Enabled:     surfaceEnabled,
			}}, nil
		case runtimeauth.PublishedRuntimeSurfaceInternal:
			return []runtimeauth.SurfaceGrant{{
				SubjectType: runtimeauth.PublishedRuntimeSubjectInternal,
				Enabled:     true,
			}}, nil
		case runtimeauth.PublishedRuntimeSurfaceBuiltinApp:
			return nil, fmt.Errorf("%w: builtin app is not an agent runtime surface", runtimeservice.ErrInvalidInput)
		}
	}

	out := make([]runtimeauth.SurfaceGrant, 0, len(grants))
	for _, item := range grants {
		subjectType := runtimeauth.NormalizeSubjectType(item.SubjectType)
		if !runtimeauth.IsKnownSubjectType(subjectType) {
			return nil, fmt.Errorf("%w: unknown runtime grant subject type", runtimeservice.ErrInvalidInput)
		}
		switch surface {
		case runtimeauth.PublishedRuntimeSurfaceWebApp:
			switch subjectType {
			case runtimeauth.PublishedRuntimeSubjectPublic,
				runtimeauth.PublishedRuntimeSubjectOrganization,
				runtimeauth.PublishedRuntimeSubjectAccount,
				runtimeauth.PublishedRuntimeSubjectDepartment,
				runtimeauth.PublishedRuntimeSubjectWorkspace:
			default:
				return nil, fmt.Errorf("%w: webapp runtime grants must target public, organization, account, department, or workspace", runtimeservice.ErrInvalidInput)
			}
		case runtimeauth.PublishedRuntimeSurfaceAPI:
			if subjectType != runtimeauth.PublishedRuntimeSubjectPublic {
				return nil, fmt.Errorf("%w: api runtime grants must use public subject", runtimeservice.ErrInvalidInput)
			}
		case runtimeauth.PublishedRuntimeSurfaceAppCenter:
			switch subjectType {
			case runtimeauth.PublishedRuntimeSubjectOrganization,
				runtimeauth.PublishedRuntimeSubjectAccount,
				runtimeauth.PublishedRuntimeSubjectDepartment,
				runtimeauth.PublishedRuntimeSubjectWorkspace:
			default:
				return nil, fmt.Errorf("%w: app center grants must target organization, account, department, or workspace", runtimeservice.ErrInvalidInput)
			}
		case runtimeauth.PublishedRuntimeSurfaceInternal:
			if subjectType != runtimeauth.PublishedRuntimeSubjectInternal {
				return nil, fmt.Errorf("%w: internal runtime grants must use internal subject", runtimeservice.ErrInvalidInput)
			}
			if item.Enabled != nil && !*item.Enabled {
				return nil, fmt.Errorf("%w: internal runtime surface cannot be disabled", runtimeservice.ErrInvalidInput)
			}
		case runtimeauth.PublishedRuntimeSurfaceBuiltinApp:
			return nil, fmt.Errorf("%w: builtin app is not an agent runtime surface", runtimeservice.ErrInvalidInput)
		}
		subjectID, err := runtimeGrantSubjectID(subjectType, item.SubjectID, organizationID)
		if err != nil {
			return nil, err
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		out = append(out, runtimeauth.SurfaceGrant{
			SubjectType: subjectType,
			SubjectID:   subjectID,
			Enabled:     enabled,
		})
	}
	return out, nil
}

func parseUUIDStrings(rawIDs []string, label string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", label, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func runtimeGrantSubjectID(subjectType runtimeauth.PublishedRuntimeSubjectType, rawID *string, organizationID uuid.UUID) (*uuid.UUID, error) {
	switch subjectType {
	case runtimeauth.PublishedRuntimeSubjectPublic, runtimeauth.PublishedRuntimeSubjectInternal:
		return nil, nil
	case runtimeauth.PublishedRuntimeSubjectOrganization:
		if rawID == nil || strings.TrimSpace(*rawID) == "" {
			return copyRuntimeUUIDPtr(organizationID), nil
		}
	case runtimeauth.PublishedRuntimeSubjectAccount, runtimeauth.PublishedRuntimeSubjectDepartment, runtimeauth.PublishedRuntimeSubjectWorkspace:
		if rawID == nil || strings.TrimSpace(*rawID) == "" {
			return nil, fmt.Errorf("%w: runtime grant subject id is required", runtimeservice.ErrInvalidInput)
		}
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*rawID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid runtime grant subject id", runtimeservice.ErrInvalidInput)
	}
	if subjectType == runtimeauth.PublishedRuntimeSubjectOrganization && parsed != organizationID {
		return nil, fmt.Errorf("%w: runtime grant organization is not current organization", runtimeservice.ErrInvalidInput)
	}
	return &parsed, nil
}

func copyRuntimeUUIDPtr(value uuid.UUID) *uuid.UUID {
	copied := value
	return &copied
}

func syncAgentRuntimeSurfaceLegacyFields(ctx context.Context, tx *gorm.DB, ag *Agent, actorAccountID string, updates agentRuntimeLegacyUpdates) error {
	if updates.webAppStatus == nil && updates.apiEnabled == nil {
		return nil
	}

	now := time.Now()
	values := map[string]interface{}{
		"updated_at": now,
	}
	if actorID, err := uuid.Parse(strings.TrimSpace(actorAccountID)); err == nil {
		values["updated_by"] = actorID
	}
	if updates.apiEnabled != nil {
		values["enable_api"] = *updates.apiEnabled
		ag.EnableAPI = *updates.apiEnabled
	}
	if updates.webAppStatus != nil {
		values["web_app_status"] = *updates.webAppStatus
		ag.WebAppStatus = *updates.webAppStatus
		if *updates.webAppStatus == AgentWebAppStatusActive {
			values["web_app_offlined_at"] = nil
			values["web_app_offlined_by"] = nil
			values["web_app_offline_reason"] = ""
		} else {
			values["web_app_offlined_at"] = now
			if actorID, err := uuid.Parse(strings.TrimSpace(actorAccountID)); err == nil {
				values["web_app_offlined_by"] = actorID
			}
			values["web_app_offline_reason"] = ""
		}
	}

	result := tx.WithContext(ctx).Model(&Agent{}).
		Where("id = ? AND deleted_at IS NULL", ag.ID).
		Updates(values)
	if result.Error != nil {
		return fmt.Errorf("failed to sync agent runtime surface legacy fields: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	return nil
}

func (s *agentsService) updateAgentWebAppStatusAndRuntimeSurface(ctx context.Context, ag *Agent, status AgentWebAppStatus, reason string, actorAccountID string) error {
	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return err
	}
	surfaceEnabled := NormalizeAgentWebAppStatus(status) == AgentWebAppStatusActive

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		updates := map[string]interface{}{
			"web_app_status": status,
			"updated_at":     now,
		}
		if actorID, err := uuid.Parse(strings.TrimSpace(actorAccountID)); err == nil {
			updates["updated_by"] = actorID
		}
		if status == AgentWebAppStatusInactive {
			updates["web_app_offlined_at"] = now
			updates["web_app_offline_reason"] = strings.TrimSpace(reason)
			if actorID, err := uuid.Parse(strings.TrimSpace(actorAccountID)); err == nil {
				updates["web_app_offlined_by"] = actorID
			}
		} else {
			updates["web_app_offlined_at"] = nil
			updates["web_app_offlined_by"] = nil
			updates["web_app_offline_reason"] = ""
		}

		result := tx.WithContext(ctx).Model(&Agent{}).
			Where("id = ? AND deleted_at IS NULL", ag.ID).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("failed to update web app status: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
		}

		workspaceID := ag.TenantID
		if err := runtimeauth.NewStore(tx).SaveResourceAuthorization(ctx, runtimeauth.ResourceAuthorization{
			ResourceType:   runtimeauth.PublishedRuntimeResourceAgent,
			ResourceID:     ag.ID,
			OrganizationID: organizationID,
			WorkspaceID:    &workspaceID,
			Surfaces: []runtimeauth.SurfaceAuthorization{{
				Surface:             runtimeauth.PublishedRuntimeSurfaceWebApp,
				Enabled:             surfaceEnabled,
				CompatibilitySource: runtimeauth.PublishedRuntimeSourceGrant,
				Grants: []runtimeauth.SurfaceGrant{{
					SubjectType: runtimeauth.PublishedRuntimeSubjectPublic,
					Enabled:     surfaceEnabled,
				}},
			}},
		}); err != nil {
			return err
		}

		ag.WebAppStatus = status
		return nil
	})
}

func (s *agentsService) organizationUUIDForAgentWorkspace(ctx context.Context, workspaceID string) (uuid.UUID, error) {
	organizationID := s.organizationIDForAgentWorkspace(ctx, workspaceID)
	parsed, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid organization id", runtimeservice.ErrInvalidInput)
	}
	return parsed, nil
}

func agentRuntimeSupportedSurfaces(surfaces []runtimeauth.SurfaceAuthorization, workspaceID uuid.UUID) []runtimeauth.SurfaceAuthorization {
	out := make([]runtimeauth.SurfaceAuthorization, 0, len(surfaces))
	for _, surface := range surfaces {
		switch surface.Surface {
		case runtimeauth.PublishedRuntimeSurfaceWebApp,
			runtimeauth.PublishedRuntimeSurfaceAPI,
			runtimeauth.PublishedRuntimeSurfaceAppCenter,
			runtimeauth.PublishedRuntimeSurfaceInternal:
			if surface.Surface == runtimeauth.PublishedRuntimeSurfaceAppCenter && len(surface.Grants) == 0 && workspaceID != uuid.Nil {
				surface.Grants = []runtimeauth.SurfaceGrant{{
					SubjectType: runtimeauth.PublishedRuntimeSubjectWorkspace,
					SubjectID:   copyRuntimeUUIDPtr(workspaceID),
					Enabled:     surface.Enabled,
				}}
			}
			out = append(out, surface)
		}
	}
	return out
}

func agentRuntimeSurfaceAuthorizationDTOs(surfaces []runtimeauth.SurfaceAuthorization) []dto.AgentRuntimeSurfaceAuthorization {
	out := make([]dto.AgentRuntimeSurfaceAuthorization, 0, len(surfaces))
	for _, surface := range surfaces {
		out = append(out, dto.AgentRuntimeSurfaceAuthorization{
			Surface:             string(surface.Surface),
			Enabled:             surface.Enabled,
			CompatibilitySource: surface.CompatibilitySource,
			Grants:              agentRuntimeSurfaceGrantDTOs(surface.Grants),
		})
	}
	return out
}

func agentRuntimeSurfaceGrantDTOs(grants []runtimeauth.SurfaceGrant) []dto.AgentRuntimeSurfaceGrant {
	out := make([]dto.AgentRuntimeSurfaceGrant, 0, len(grants))
	for _, grant := range grants {
		var subjectID *string
		if grant.SubjectID != nil {
			value := grant.SubjectID.String()
			subjectID = &value
		}
		out = append(out, dto.AgentRuntimeSurfaceGrant{
			SubjectType: string(grant.SubjectType),
			SubjectID:   subjectID,
			Enabled:     grant.Enabled,
		})
	}
	return out
}
