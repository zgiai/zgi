package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/user/auth/statuscache"
	workspacecache "github.com/zgiai/zgi/api/internal/modules/workspace/cache"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *organizationService) ValidateInviteLinkForRegistration(ctx context.Context, token string) (*model.OrganizationInviteLink, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("organization invite service is unavailable")
	}
	var link model.OrganizationInviteLink
	if err := s.db.WithContext(ctx).Where("token = ?", strings.TrimSpace(token)).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("invalid invite token")
		}
		return nil, fmt.Errorf("get invite link: %w", err)
	}
	workspaceID, err := validateOrganizationInviteTargetTx(ctx, s.db, &link)
	if err != nil {
		return nil, err
	}
	link.WorkspaceID = workspaceID
	return &link, nil
}

func (s *organizationService) acceptInviteByTokenTransaction(ctx context.Context, token, accountID string, name *string) (*model.OrganizationJoinRequest, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("organization invite service is unavailable")
	}
	token = strings.TrimSpace(token)
	accountID = strings.TrimSpace(accountID)
	if token == "" || accountID == "" {
		return nil, fmt.Errorf("invite token and account are required")
	}

	var accepted *model.OrganizationJoinRequest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var link model.OrganizationInviteLink
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token = ?", token).
			First(&link).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("invalid invite token")
			}
			return fmt.Errorf("get invite link: %w", err)
		}
		workspaceID, err := validateOrganizationInviteTargetTx(ctx, tx, &link)
		if err != nil {
			return err
		}
		if err := lockInvitedAccountTx(ctx, tx, accountID); err != nil {
			return err
		}

		member, err := organizationMemberExistsTx(ctx, tx, link.OrganizationID, accountID)
		if err != nil {
			return err
		}
		// An organization-only invitation grants nothing beyond an existing
		// membership, so it can safely act as a context switch. Department and
		// workspace invitations must still follow their approval policy and apply
		// the target membership effects below.
		if member && link.DepartmentID == nil && workspaceID == nil {
			if err := setAcceptedInviteContextTx(ctx, tx, accountID, link.OrganizationID, workspaceID); err != nil {
				return err
			}
			accepted = newOrganizationInviteJoinRequest(&link, accountID, name, workspaceID, model.OrganizationJoinRequestStatusApproved)
			return nil
		}

		var existing model.OrganizationJoinRequest
		existingErr := tx.WithContext(ctx).
			Where("group_id = ? AND invite_link_id = ? AND account_id = ?", link.OrganizationID, link.ID, accountID).
			Order("created_at DESC").
			First(&existing).Error
		if existingErr == nil {
			switch existing.Status {
			case model.OrganizationJoinRequestStatusPending:
				accepted = &existing
				return nil
			case model.OrganizationJoinRequestStatusApproved:
				if err := s.applyApprovedInviteEffectsTx(ctx, tx, &existing); err != nil {
					return err
				}
				accepted = &existing
				return nil
			}
		}
		if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get existing join request: %w", existingErr)
		}

		status := model.OrganizationJoinRequestStatusPending
		if !link.RequireApproval {
			status = model.OrganizationJoinRequestStatusApproved
		}
		accepted = newOrganizationInviteJoinRequest(&link, accountID, name, workspaceID, status)
		if err := tx.WithContext(ctx).Create(accepted).Error; err != nil {
			return fmt.Errorf("create organization join request: %w", err)
		}
		if status == model.OrganizationJoinRequestStatusApproved {
			if err := s.applyApprovedInviteEffectsTx(ctx, tx, accepted); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	statuscache.InvalidateAccountStatus(ctx, accountID)
	workspacecache.InvalidateAccount(ctx, accountID)
	if s.accountService != nil {
		s.accountService.InvalidateAccountProfileCache(accountID)
	}
	return accepted, nil
}

func (s *organizationService) approveInviteRequestTransaction(ctx context.Context, organizationID, joinRequestID, reviewerAccountID string) (*model.OrganizationJoinRequest, error) {
	var approved *model.OrganizationJoinRequest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var req model.OrganizationJoinRequest
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", joinRequestID).
			First(&req).Error; err != nil {
			return err
		}
		if req.OrganizationID != organizationID {
			return fmt.Errorf("invalid organization id")
		}
		if req.Status != model.OrganizationJoinRequestStatusPending {
			return fmt.Errorf("request is not pending")
		}
		if err := validateOrganizationJoinRequestTargetTx(ctx, tx, &req); err != nil {
			return err
		}
		if err := lockInvitedAccountTx(ctx, tx, req.AccountID); err != nil {
			return err
		}
		now := time.Now()
		req.Status = model.OrganizationJoinRequestStatusApproved
		req.ReviewerID = &reviewerAccountID
		req.ReviewedAt = &now
		if err := tx.WithContext(ctx).Save(&req).Error; err != nil {
			return fmt.Errorf("approve organization join request: %w", err)
		}
		if err := s.applyApprovedInviteEffectsTx(ctx, tx, &req); err != nil {
			return err
		}
		approved = &req
		return nil
	})
	if err != nil {
		return nil, err
	}
	statuscache.InvalidateAccountStatus(ctx, approved.AccountID)
	workspacecache.InvalidateAccount(ctx, approved.AccountID)
	if s.accountService != nil {
		s.accountService.InvalidateAccountProfileCache(approved.AccountID)
	}
	return approved, nil
}

func validateOrganizationJoinRequestTargetTx(ctx context.Context, tx *gorm.DB, req *model.OrganizationJoinRequest) error {
	if req == nil || req.InviteLinkID == nil || strings.TrimSpace(*req.InviteLinkID) == "" {
		return fmt.Errorf("invite link is unavailable")
	}

	var link model.OrganizationInviteLink
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("id = ?", strings.TrimSpace(*req.InviteLinkID)).
		First(&link).Error; err != nil {
		return fmt.Errorf("invite link is unavailable: %w", err)
	}
	if link.OrganizationID != req.OrganizationID ||
		trimOptionalInviteString(link.DepartmentID) != trimOptionalInviteString(req.DepartmentID) {
		return fmt.Errorf("invite target no longer matches the pending request")
	}

	workspaceID, err := validateOrganizationInviteTargetTx(ctx, tx, &link)
	if err != nil {
		return err
	}
	if trimOptionalInviteString(workspaceID) != trimOptionalInviteString(req.WorkspaceID) {
		return fmt.Errorf("invite target no longer matches the pending request")
	}
	return nil
}

func validateOrganizationInviteTargetTx(ctx context.Context, tx *gorm.DB, link *model.OrganizationInviteLink) (*string, error) {
	if link == nil || strings.TrimSpace(link.OrganizationID) == "" {
		return nil, fmt.Errorf("invite link organization is required")
	}
	if link.Status != "active" {
		return nil, fmt.Errorf("invite link is not active")
	}
	if link.ExpiresAt != nil && link.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("invite link expired")
	}
	var organization model.Organization
	if err := tx.WithContext(ctx).
		// Serialize invite effects within an organization. Besides keeping the
		// target active for the transaction, this makes the member-name check and
		// insert one organization-scoped critical section for different accounts.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", link.OrganizationID).
		First(&organization).Error; err != nil {
		return nil, fmt.Errorf("invited organization is unavailable: %w", err)
	}
	if !organization.IsActive() {
		return nil, fmt.Errorf("invited organization is unavailable")
	}
	if link.DepartmentID != nil && strings.TrimSpace(*link.DepartmentID) != "" {
		var department model.Department
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id = ? AND group_id = ? AND status = ?", strings.TrimSpace(*link.DepartmentID), link.OrganizationID, model.DepartmentStatusActive).
			First(&department).Error; err != nil {
			return nil, fmt.Errorf("invited department is unavailable: %w", err)
		}
	}

	if link.WorkspaceID != nil && strings.TrimSpace(*link.WorkspaceID) != "" {
		workspaceID := strings.TrimSpace(*link.WorkspaceID)
		var workspace model.Workspace
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id = ? AND organization_id = ? AND status = ?", workspaceID, link.OrganizationID, model.WorkspaceStatusNormal).
			First(&workspace).Error; err != nil {
			return nil, fmt.Errorf("invited workspace is unavailable: %w", err)
		}
		return &workspaceID, nil
	}

	// A nil WorkspaceID is an organization-only invitation. Do not infer a
	// workspace: doing so would grant access beyond the link's authority.
	return nil, nil
}

func lockInvitedAccountTx(ctx context.Context, tx *gorm.DB, accountID string) error {
	var account auth_model.Account
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		Where("id = ?", strings.TrimSpace(accountID)).
		First(&account).Error; err != nil {
		return fmt.Errorf("invited account is unavailable: %w", err)
	}
	return nil
}

func newOrganizationInviteJoinRequest(link *model.OrganizationInviteLink, accountID string, name *string, workspaceID *string, status model.OrganizationJoinRequestStatus) *model.OrganizationJoinRequest {
	return &model.OrganizationJoinRequest{
		OrganizationID:          link.OrganizationID,
		InviteLinkID:            &link.ID,
		AccountID:               accountID,
		DepartmentID:            link.DepartmentID,
		WorkspaceID:             workspaceID,
		DefaultOrganizationRole: link.DefaultOrganizationRole,
		DefaultWorkspaceRole:    link.DefaultWorkspaceRole,
		Status:                  status,
		Name:                    name,
	}
}

func (s *organizationService) applyApprovedInviteEffectsTx(ctx context.Context, tx *gorm.DB, req *model.OrganizationJoinRequest) error {
	member, err := organizationMemberExistsTx(ctx, tx, req.OrganizationID, req.AccountID)
	if err != nil {
		return err
	}
	if !member {
		if err := ensureInviteMemberNameAvailableTx(ctx, tx, req.OrganizationID, req.AccountID, req.Name); err != nil {
			return err
		}
		role := model.OrganizationRole(req.DefaultOrganizationRole)
		if role != model.OrganizationRoleAdmin && role != model.OrganizationRoleNormal {
			role = model.OrganizationRoleNormal
		}
		join := &model.OrganizationMember{
			OrganizationID: req.OrganizationID,
			AccountID:      req.AccountID,
			Role:           role,
			Name:           req.Name,
		}
		if err := tx.WithContext(ctx).Create(join).Error; err != nil {
			return fmt.Errorf("add invited organization member: %w", err)
		}
	}
	if req.DepartmentID != nil {
		departmentID := strings.TrimSpace(*req.DepartmentID)
		var targetDepartmentMemberships int64
		if err := tx.WithContext(ctx).
			Model(&model.DepartmentMember{}).
			Where("department_id = ? AND account_id = ?", departmentID, req.AccountID).
			Count(&targetDepartmentMemberships).Error; err != nil {
			return fmt.Errorf("check invited department membership: %w", err)
		}
		if targetDepartmentMemberships == 0 {
			if err := addInviteDepartmentMemberTx(ctx, tx, req.OrganizationID, departmentID, req.AccountID); err != nil {
				return err
			}
		}
	}
	if req.WorkspaceID != nil && strings.TrimSpace(*req.WorkspaceID) != "" {
		workspaceID := strings.TrimSpace(*req.WorkspaceID)
		workspaceMember, err := workspaceMemberExistsTx(ctx, tx, workspaceID, req.AccountID)
		if err != nil {
			return err
		}
		if !workspaceMember {
			if s.workspaceManagementService == nil {
				return fmt.Errorf("workspace management service is required to accept invite")
			}
			role := model.WorkspaceMemberRole(req.DefaultWorkspaceRole)
			if model.DefaultWorkspaceRoleID(role) == "" || role == model.WorkspaceRoleOwner {
				role = model.WorkspaceRoleNormal
			}
			if err := s.workspaceManagementService.WithTx(tx).AddMember(ctx, &interfaces.AddMemberRequest{
				WorkspaceID: workspaceID,
				AccountID:   req.AccountID,
				Role:        role,
			}); err != nil && !strings.Contains(err.Error(), "already a member") {
				return fmt.Errorf("add invited workspace member: %w", err)
			}
		}
	}
	return setAcceptedInviteContextTx(ctx, tx, req.AccountID, req.OrganizationID, req.WorkspaceID)
}

func ensureInviteMemberNameAvailableTx(ctx context.Context, tx *gorm.DB, organizationID, accountID string, name *string) error {
	if name == nil || strings.TrimSpace(*name) == "" {
		return nil
	}

	exists, err := organizationMemberNameExistsTx(ctx, tx, organizationID, strings.TrimSpace(*name), accountID)
	if err != nil {
		return fmt.Errorf("check invited member name: %w", err)
	}
	if exists {
		return ErrMemberNameExists
	}
	return nil
}

func setAcceptedInviteContextTx(ctx context.Context, tx *gorm.DB, accountID, organizationID string, workspaceID *string) error {
	workspace := ""
	if workspaceID != nil {
		workspace = strings.TrimSpace(*workspaceID)
	}
	if workspace == "" {
		if err := tx.WithContext(ctx).
			Model(&model.WorkspaceMember{}).
			Where("account_id = ?", accountID).
			Update("current", false).Error; err != nil {
			return fmt.Errorf("failed to clear current workspace: %w", err)
		}
	} else if err := setInviteCurrentWorkspaceTx(ctx, tx, accountID, workspace); err != nil {
		return err
	}

	var ctxModel auth_model.AccountContext
	err := tx.WithContext(ctx).Where("account_id = ?", accountID).First(&ctxModel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := createAccountContextTx(ctx, tx, accountID, organizationID, workspace); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("get invited account context: %w", err)
	} else {
		ctxModel.CurrentOrganizationID = &organizationID
		if workspace == "" {
			ctxModel.CurrentWorkspaceID = nil
		} else {
			ctxModel.CurrentWorkspaceID = &workspace
		}
		ctxModel.UpdatedAt = time.Now()
		if err := tx.WithContext(ctx).Save(&ctxModel).Error; err != nil {
			return fmt.Errorf("update invited account context: %w", err)
		}
	}
	return nil
}
