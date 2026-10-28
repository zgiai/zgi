package runtimeworkspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/workspace/memberstatus"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

type sourceWorkspaceAccess struct {
	Workspaces        []SourceWorkspace
	AllowedIDs        map[uuid.UUID]struct{}
	OrganizationAdmin bool
	CrossEnabled      bool
}

func (s *Service) sourceWorkspaceAccess(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, accountID uuid.UUID) (*sourceWorkspaceAccess, error) {
	if runtimeWorkspace == nil || runtimeWorkspace.ResourceWorkspaceID() == uuid.Nil || runtimeWorkspace.OrganizationID == uuid.Nil || accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	organizationAdmin, err := s.isOrganizationAdminOrOwner(ctx, runtimeWorkspace.OrganizationID, accountID)
	if err != nil {
		return nil, err
	}
	type workspaceRow struct {
		ID   uuid.UUID
		Name string
	}
	var rows []workspaceRow
	query := s.db.WithContext(ctx).Table("workspaces AS workspaces").
		Select("workspaces.id, workspaces.name").
		Where("workspaces.organization_id = ? AND workspaces.status = ?", runtimeWorkspace.OrganizationID, workspace_model.WorkspaceStatusNormal)
	if !organizationAdmin {
		query = query.Joins("JOIN workspace_members AS members ON members.workspace_id = workspaces.id").
			Where("members.account_id = ? AND members.role IN ?", accountID, []workspace_model.WorkspaceMemberRole{workspace_model.WorkspaceRoleOwner, workspace_model.WorkspaceRoleAdmin}).
			// Workspace rows survive a disabled organization membership.
			Where(memberstatus.NotDisabledInWorkspaceOrganizationSQL("workspaces.id", "members.account_id"))
	}
	if err := query.Order("workspaces.name ASC").Order("workspaces.id ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list manageable Runtime source Workspaces: %w", err)
	}
	allowedIDs := make(map[uuid.UUID]struct{}, len(rows))
	workspaces := make([]SourceWorkspace, 0, len(rows))
	hostGovernance := organizationAdmin
	for _, row := range rows {
		allowedIDs[row.ID] = struct{}{}
		isCurrent := row.ID == runtimeWorkspace.ResourceWorkspaceID()
		if isCurrent {
			hostGovernance = true
		}
		workspaces = append(workspaces, SourceWorkspace{ID: row.ID.String(), Name: row.Name, IsCurrent: isCurrent})
	}
	crossEnabled := organizationAdmin
	if !crossEnabled && hostGovernance {
		for workspaceID := range allowedIDs {
			if workspaceID != runtimeWorkspace.ResourceWorkspaceID() {
				crossEnabled = true
				break
			}
		}
	}
	return &sourceWorkspaceAccess{Workspaces: workspaces, AllowedIDs: allowedIDs, OrganizationAdmin: organizationAdmin, CrossEnabled: crossEnabled}, nil
}

func (s *Service) isOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error) {
	var member struct {
		Role workspace_model.OrganizationRole
	}
	if err := s.db.WithContext(ctx).Table("members").Select("role").
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID, accountID, workspace_model.OrganizationMemberStatusActive).Take(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("check Runtime source Organization role: %w", err)
	}
	return member.Role == workspace_model.OrganizationRoleOwner || member.Role == workspace_model.OrganizationRoleAdmin, nil
}

func (access *sourceWorkspaceAccess) canReference(hostWorkspaceID, sourceWorkspaceID uuid.UUID) bool {
	if sourceWorkspaceID == hostWorkspaceID {
		return true
	}
	if access == nil || !access.CrossEnabled {
		return false
	}
	_, allowed := access.AllowedIDs[sourceWorkspaceID]
	return allowed
}
