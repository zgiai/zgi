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
)

func (s *organizationService) InviteCurrentOrganizationMember(ctx context.Context, req *shared_dto.InviteCurrentOrganizationMemberRequest) (*shared_dto.InviteCurrentOrganizationMemberResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("invite request is required")
	}

	organizationID := strings.TrimSpace(req.OrganizationID)
	operatorAccountID := strings.TrimSpace(req.OperatorAccountID)
	email := normalizeAccountEmail(req.Email)
	name := strings.TrimSpace(req.Name)
	if organizationID == "" || operatorAccountID == "" || email == "" || name == "" {
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

		if createdAccount {
			if err := createAccountContextTx(ctx, tx, account.ID, organizationID); err != nil {
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

func createAccountContextTx(ctx context.Context, tx *gorm.DB, accountID, organizationID string) error {
	now := time.Now()
	ctxModel := &auth_model.AccountContext{
		AccountID:             accountID,
		CurrentOrganizationID: &organizationID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := tx.WithContext(ctx).Create(ctxModel).Error; err != nil {
		return fmt.Errorf("failed to create account context: %w", err)
	}
	return nil
}
