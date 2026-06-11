package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

var (
	ErrOrganizationInvitePermissionDenied = errors.New("organization invite permission denied")
	ErrOrganizationMemberNotFound         = errors.New("organization member not found")
	ErrOrganizationOwnerPasswordReset     = errors.New("organization owner password reset denied")
	ErrSuperAdminPasswordReset            = errors.New("super admin password reset denied")
	ErrOrganizationInviteWorkspaceInvalid = errors.New("organization invite workspace invalid")
)

func (s *organizationService) InviteCurrentOrganizationMember(ctx context.Context, req *shared_dto.InviteCurrentOrganizationMemberRequest) (*shared_dto.InviteCurrentOrganizationMemberResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("invite request is required")
	}

	organizationID := strings.TrimSpace(req.OrganizationID)
	operatorAccountID := strings.TrimSpace(req.OperatorAccountID)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	departmentID := trimOptionalInviteString(req.DepartmentID)
	email := normalizeAccountEmail(req.Email)
	name := strings.TrimSpace(req.Name)
	if organizationID == "" || operatorAccountID == "" || workspaceID == "" || email == "" || name == "" {
		return nil, fmt.Errorf("invalid invite request")
	}

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash invite password: %w", err)
	}

	var resp *shared_dto.InviteCurrentOrganizationMemberResponse
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operatorRole, err := getOrganizationMemberRoleTx(ctx, tx, organizationID, operatorAccountID)
		if err != nil {
			return err
		}
		if !isOrganizationAdminRole(operatorRole) {
			return ErrOrganizationInvitePermissionDenied
		}

		workspace, err := getInviteWorkspaceTx(ctx, tx, organizationID, workspaceID)
		if err != nil {
			return err
		}

		var department *model.Department
		if departmentID != "" {
			department, err = getInviteDepartmentTx(ctx, tx, organizationID, departmentID)
			if err != nil {
				return err
			}
		}

		account, createdAccount, err := getOrCreateInviteAccountTx(ctx, tx, email, name, hashedPassword, salt)
		if err != nil {
			return err
		}

		alreadyMember, err := organizationMemberExistsTx(ctx, tx, organizationID, account.ID)
		if err != nil {
			return err
		}
		if !alreadyMember {
			memberName := name
			join := &model.OrganizationMember{
				OrganizationID: organizationID,
				AccountID:      account.ID,
				Role:           model.OrganizationRoleNormal,
				Name:           &memberName,
			}
			if err := tx.WithContext(ctx).Create(join).Error; err != nil {
				return fmt.Errorf("failed to add organization member: %w", err)
			}
		}

		alreadyWorkspaceMember, err := workspaceMemberExistsTx(ctx, tx, workspaceID, account.ID)
		if err != nil {
			return err
		}
		if !alreadyWorkspaceMember {
			workspaceJoin := newInviteWorkspaceMemberJoin(workspaceID, account.ID, false)
			if err := tx.WithContext(ctx).Create(workspaceJoin).Error; err != nil {
				return fmt.Errorf("failed to add workspace member: %w", err)
			}
		}

		targetWorkspaceSelected, err := ensureInviteAccountContextTx(ctx, tx, account.ID, organizationID, workspaceID)
		if err != nil {
			return err
		}
		if targetWorkspaceSelected {
			if err := setInviteCurrentWorkspaceTx(ctx, tx, account.ID, workspaceID); err != nil {
				return err
			}
		}

		if department != nil {
			if err := addInviteDepartmentMemberTx(ctx, tx, organizationID, department.ID, account.ID); err != nil {
				return err
			}
		}

		resp = &shared_dto.InviteCurrentOrganizationMemberResponse{
			AccountID:       account.ID,
			Email:           account.Email,
			Name:            account.Name,
			OrganizationID:  organizationID,
			Role:            model.OrganizationRoleNormal,
			CreatedAccount:  createdAccount,
			AlreadyMember:   alreadyMember,
			PasswordApplied: createdAccount,
			Workspace: &shared_dto.MemberWorkspaceInfo{
				ID:   workspace.ID,
				Name: workspace.Name,
			},
		}
		if department != nil {
			resp.Department = &shared_dto.MemberDepartmentInfo{
				ID:   department.ID,
				Name: department.Name,
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *organizationService) ResetCurrentOrganizationMemberPassword(ctx context.Context, req *shared_dto.ResetCurrentOrganizationMemberPasswordRequest) (*shared_dto.ResetCurrentOrganizationMemberPasswordResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("reset password request is required")
	}

	organizationID := strings.TrimSpace(req.OrganizationID)
	operatorAccountID := strings.TrimSpace(req.OperatorAccountID)
	email := normalizeAccountEmail(req.Email)
	if organizationID == "" || operatorAccountID == "" || email == "" {
		return nil, fmt.Errorf("invalid reset password request")
	}

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash reset password: %w", err)
	}

	var resp *shared_dto.ResetCurrentOrganizationMemberPasswordResponse
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operatorRole, err := getOrganizationMemberRoleTx(ctx, tx, organizationID, operatorAccountID)
		if err != nil {
			return err
		}
		if !isOrganizationAdminRole(operatorRole) {
			return ErrOrganizationInvitePermissionDenied
		}

		account, err := getAccountByEmailTx(ctx, tx, email)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrganizationMemberNotFound
			}
			return err
		}
		if account.IsSuperAdmin {
			return ErrSuperAdminPasswordReset
		}

		targetRole, err := getOrganizationMemberRoleTx(ctx, tx, organizationID, account.ID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrganizationMemberNotFound
			}
			return err
		}
		if targetRole == model.OrganizationRoleOwner && operatorRole != model.OrganizationRoleOwner {
			return ErrOrganizationOwnerPasswordReset
		}

		account.Password = &hashedPassword
		account.PasswordSalt = &salt
		if err := tx.WithContext(ctx).Save(account).Error; err != nil {
			return fmt.Errorf("failed to reset organization member password: %w", err)
		}

		resp = &shared_dto.ResetCurrentOrganizationMemberPasswordResponse{
			AccountID:     account.ID,
			Email:         account.Email,
			PasswordReset: true,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func normalizeAccountEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isOrganizationAdminRole(role model.OrganizationRole) bool {
	return role == model.OrganizationRoleAdmin || role == model.OrganizationRoleOwner
}

func getOrganizationMemberRoleTx(ctx context.Context, tx *gorm.DB, organizationID, accountID string) (model.OrganizationRole, error) {
	var member model.OrganizationMember
	err := tx.WithContext(ctx).
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		First(&member).Error
	if err != nil {
		return "", err
	}
	return member.Role, nil
}

func organizationMemberExistsTx(ctx context.Context, tx *gorm.DB, organizationID, accountID string) (bool, error) {
	var count int64
	if err := tx.WithContext(ctx).
		Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check organization membership: %w", err)
	}
	return count > 0, nil
}

func workspaceMemberExistsTx(ctx context.Context, tx *gorm.DB, workspaceID, accountID string) (bool, error) {
	var count int64
	if err := tx.WithContext(ctx).
		Model(&model.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check workspace membership: %w", err)
	}
	return count > 0, nil
}

func getInviteWorkspaceTx(ctx context.Context, tx *gorm.DB, organizationID, workspaceID string) (*model.Workspace, error) {
	var workspace model.Workspace
	err := tx.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND status = ?", workspaceID, organizationID, model.WorkspaceStatusNormal).
		First(&workspace).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationInviteWorkspaceInvalid
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	return &workspace, nil
}

func getInviteDepartmentTx(ctx context.Context, tx *gorm.DB, organizationID, departmentID string) (*model.Department, error) {
	var department model.Department
	err := tx.WithContext(ctx).
		Where("id = ? AND group_id = ? AND status = ?", departmentID, organizationID, model.DepartmentStatusActive).
		First(&department).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, fmt.Errorf("failed to get department: %w", err)
	}
	return &department, nil
}

func addInviteDepartmentMemberTx(ctx context.Context, tx *gorm.DB, organizationID, departmentID, accountID string) error {
	var existing model.DepartmentMember
	err := tx.WithContext(ctx).
		Joins("JOIN departments ON departments.id = department_members.department_id").
		Where("departments.group_id = ? AND department_members.account_id = ?", organizationID, accountID).
		First(&existing).Error
	if err == nil {
		return ErrMemberAlreadyInDept
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing department membership: %w", err)
	}

	member := &model.DepartmentMember{
		DepartmentID: departmentID,
		AccountID:    accountID,
	}
	if err := tx.WithContext(ctx).Create(member).Error; err != nil {
		return fmt.Errorf("failed to add department member: %w", err)
	}
	return nil
}

func getAccountByEmailTx(ctx context.Context, tx *gorm.DB, email string) (*auth_model.Account, error) {
	var account auth_model.Account
	err := tx.WithContext(ctx).
		Where("LOWER(email) = ?", email).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func getOrCreateInviteAccountTx(ctx context.Context, tx *gorm.DB, email, name, hashedPassword, salt string) (*auth_model.Account, bool, error) {
	account, err := getAccountByEmailTx(ctx, tx, email)
	if err == nil {
		return account, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("failed to query account by email: %w", err)
	}

	now := time.Now()
	account = &auth_model.Account{
		ID:            uuid.New().String(),
		Name:          name,
		Email:         email,
		Password:      &hashedPassword,
		PasswordSalt:  &salt,
		Status:        auth_model.AccountStatusActive,
		InitializedAt: &now,
		LastActiveAt:  &now,
	}
	if err := tx.WithContext(ctx).Create(account).Error; err != nil {
		return nil, false, fmt.Errorf("failed to create invite account: %w", err)
	}
	return account, true, nil
}

func newInviteWorkspaceMemberJoin(workspaceID, accountID string, current bool) *model.WorkspaceMember {
	join := &model.WorkspaceMember{
		WorkspaceID: workspaceID,
		AccountID:   accountID,
		Role:        model.WorkspaceRoleNormal,
		Current:     current,
	}
	model.ApplyWorkspaceMemberDefaults(join)
	return join
}

func newInviteAccountContext(accountID, organizationID, workspaceID string) *auth_model.AccountContext {
	now := time.Now()
	return &auth_model.AccountContext{
		AccountID:             accountID,
		CurrentOrganizationID: &organizationID,
		CurrentWorkspaceID:    &workspaceID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

func ensureInviteAccountContextTx(ctx context.Context, tx *gorm.DB, accountID, organizationID, workspaceID string) (bool, error) {
	var ctxModel auth_model.AccountContext
	err := tx.WithContext(ctx).Where("account_id = ?", accountID).First(&ctxModel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := createAccountContextTx(ctx, tx, accountID, organizationID, workspaceID); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get account context: %w", err)
	}

	valid, resolvedOrganizationID, err := isInviteAccountContextWorkspaceValidTx(ctx, tx, accountID, ctxModel.CurrentOrganizationID, ctxModel.CurrentWorkspaceID)
	if err != nil {
		return false, err
	}
	if valid {
		if ctxModel.CurrentOrganizationID == nil || strings.TrimSpace(*ctxModel.CurrentOrganizationID) == "" {
			ctxModel.CurrentOrganizationID = &resolvedOrganizationID
			if err := tx.WithContext(ctx).Save(&ctxModel).Error; err != nil {
				return false, fmt.Errorf("failed to update account context organization: %w", err)
			}
		}
		return ptrValueEquals(ctxModel.CurrentWorkspaceID, workspaceID), nil
	}

	ctxModel.CurrentOrganizationID = &organizationID
	ctxModel.CurrentWorkspaceID = &workspaceID
	if err := tx.WithContext(ctx).Save(&ctxModel).Error; err != nil {
		return false, fmt.Errorf("failed to update account context: %w", err)
	}
	return true, nil
}

func isInviteAccountContextWorkspaceValidTx(ctx context.Context, tx *gorm.DB, accountID string, organizationID, workspaceID *string) (bool, string, error) {
	currentWorkspaceID := trimOptionalInviteString(workspaceID)
	if currentWorkspaceID == "" {
		return false, "", nil
	}

	var workspace model.Workspace
	err := tx.WithContext(ctx).
		Where("id = ? AND status = ?", currentWorkspaceID, model.WorkspaceStatusNormal).
		First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to get current workspace: %w", err)
	}
	if workspace.OrganizationID == nil || strings.TrimSpace(*workspace.OrganizationID) == "" {
		return false, "", nil
	}

	resolvedOrganizationID := strings.TrimSpace(*workspace.OrganizationID)
	currentOrganizationID := trimOptionalInviteString(organizationID)
	if currentOrganizationID != "" && currentOrganizationID != resolvedOrganizationID {
		return false, "", nil
	}

	var member model.OrganizationMember
	err = tx.WithContext(ctx).
		Where("organization_id = ? AND account_id = ?", resolvedOrganizationID, accountID).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check current organization membership: %w", err)
	}
	if isOrganizationAdminRole(member.Role) {
		return true, resolvedOrganizationID, nil
	}

	workspaceMember, err := workspaceMemberExistsTx(ctx, tx, currentWorkspaceID, accountID)
	if err != nil {
		return false, "", err
	}
	return workspaceMember, resolvedOrganizationID, nil
}

func setInviteCurrentWorkspaceTx(ctx context.Context, tx *gorm.DB, accountID, workspaceID string) error {
	if err := tx.WithContext(ctx).
		Model(&model.WorkspaceMember{}).
		Where("account_id = ?", accountID).
		Update("current", false).Error; err != nil {
		return fmt.Errorf("failed to clear current workspace: %w", err)
	}
	if err := tx.WithContext(ctx).
		Model(&model.WorkspaceMember{}).
		Where("account_id = ? AND workspace_id = ?", accountID, workspaceID).
		Update("current", true).Error; err != nil {
		return fmt.Errorf("failed to set current workspace: %w", err)
	}
	return nil
}

func createAccountContextTx(ctx context.Context, tx *gorm.DB, accountID, organizationID, workspaceID string) error {
	ctxModel := newInviteAccountContext(accountID, organizationID, workspaceID)
	if err := tx.WithContext(ctx).Create(ctxModel).Error; err != nil {
		return fmt.Errorf("failed to create account context: %w", err)
	}
	return nil
}

func trimOptionalInviteString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func ptrValueEquals(value *string, expected string) bool {
	return trimOptionalInviteString(value) == strings.TrimSpace(expected)
}
