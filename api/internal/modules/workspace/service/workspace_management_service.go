package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/gorm"
)

type WorkspaceManagementServiceImpl struct {
	db                  *gorm.DB
	workspaceRepo       repository.WorkspaceRepository
	workspaceMemberRepo repository.WorkspaceMemberRepository
	accountService      interfaces.AccountService
	quotaService        interfaces.QuotaService
	organizationService interfaces.OrganizationService
}

func NewWorkspaceManagementService(
	db *gorm.DB,
	workspaceRepo repository.WorkspaceRepository,
	workspaceMemberRepo repository.WorkspaceMemberRepository,
	accountService interfaces.AccountService,
	quotaService interfaces.QuotaService,
	organizationService interfaces.OrganizationService,
) interfaces.WorkspaceManagementService {
	return &WorkspaceManagementServiceImpl{
		db:                  db,
		workspaceRepo:       workspaceRepo,
		workspaceMemberRepo: workspaceMemberRepo,
		accountService:      accountService,
		quotaService:        quotaService,
		organizationService: organizationService,
	}
}

func (s *WorkspaceManagementServiceImpl) GetByWorkspaceAndMember(ctx context.Context, workspaceID, accountID string) (*model.WorkspaceMember, error) {
	return s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
}

func (s *WorkspaceManagementServiceImpl) SetAccountService(accountService interfaces.AccountService) {
	s.accountService = accountService
}

func (s *WorkspaceManagementServiceImpl) SetOrganizationService(organizationService interfaces.OrganizationService) {
	s.organizationService = organizationService
}

func (s *WorkspaceManagementServiceImpl) WithTx(tx *gorm.DB) interfaces.WorkspaceManagementService {
	return NewWorkspaceManagementService(
		tx,
		s.workspaceRepo.WithTx(tx),
		s.workspaceMemberRepo.WithTx(tx),
		// Fix: Use accountService instead of accountRepo
		s.accountService, // AccountService doesn't need WithTx as it's an interface
		s.quotaService,
		s.organizationService,
	)
}

func (s *WorkspaceManagementServiceImpl) applyWorkspaceMemberPermissionTemplate(ctx context.Context, join *model.WorkspaceMember) error {
	if join == nil {
		return nil
	}

	model.ApplyWorkspaceMemberDefaults(join)
	if join.Role == model.WorkspaceRoleOwner {
		return nil
	}

	if join.PermissionSource == model.WorkspaceMemberPermissionSourceDirect {
		join.Permissions = model.CanonicalAssignableWorkspacePermissionSnapshotStrings(join.Permissions)
		return nil
	}

	roleID := ""
	if join.RoleID != nil {
		roleID = strings.TrimSpace(*join.RoleID)
	}
	if roleID == "" {
		defaultRoleID, err := s.defaultWorkspaceRoleTemplateID(ctx, join.WorkspaceID, join.Role)
		if err != nil {
			return err
		}
		roleID = defaultRoleID
	}
	if roleID == "" {
		join.PermissionSource = model.WorkspaceMemberPermissionSourceDirect
		join.PermissionTemplateRoleID = nil
		join.Permissions = []string{}
		return nil
	}

	join.RoleID = &roleID
	join.PermissionTemplateRoleID = &roleID
	join.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate

	if model.IsBuiltinRole(roleID) {
		join.Permissions = model.CanonicalWorkspacePermissionSnapshotStrings(
			model.DefaultWorkspaceMemberPermissionStrings(join.Role, &roleID),
		)
		return nil
	}

	permissions, err := s.getActiveCustomWorkspaceRolePermissions(ctx, join.WorkspaceID, roleID)
	if err != nil {
		return err
	}
	join.Permissions = model.CanonicalAssignableWorkspacePermissionSnapshotStrings(permissions)
	return nil
}

func applyWorkspaceMemberDirectPermissionSnapshot(join *model.WorkspaceMember, permissions []string) {
	if join == nil || model.WorkspaceMemberRoleHasGovernanceAuthority(join.Role) {
		return
	}

	join.PermissionSource = model.WorkspaceMemberPermissionSourceDirect
	join.Permissions = model.CanonicalAssignableWorkspacePermissionSnapshotStrings(permissions)
}

func (s *WorkspaceManagementServiceImpl) defaultWorkspaceRoleTemplateID(ctx context.Context, workspaceID string, role model.WorkspaceMemberRole) (string, error) {
	switch role {
	case model.WorkspaceRoleNormal, model.WorkspaceRoleMember, model.WorkspaceRoleEditor:
		roleID, err := s.findDefaultWorkspaceRoleTemplateID(ctx, workspaceID, model.WorkspaceDefaultRoleTemplateBasicKey)
		if err != nil {
			return "", err
		}
		if roleID != "" {
			return roleID, nil
		}
	}
	return model.DefaultWorkspaceRoleID(role), nil
}

func (s *WorkspaceManagementServiceImpl) findDefaultWorkspaceRoleTemplateID(ctx context.Context, workspaceID, systemKey string) (string, error) {
	if s == nil || s.db == nil || s.workspaceRepo == nil {
		return "", nil
	}

	organizationID, err := s.workspaceRepo.GetWorkspaceOrganizationID(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve workspace organization: %w", err)
	}
	if strings.TrimSpace(organizationID) == "" {
		return "", nil
	}

	var role model.WorkspaceCustomRole
	if err := s.db.WithContext(ctx).
		Where("group_id = ? AND system_key = ? AND status = ?", organizationID, systemKey, model.WorkspaceCustomRoleStatusActive).
		First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get default workspace role template: %w", err)
	}
	return role.ID, nil
}

func (s *WorkspaceManagementServiceImpl) getActiveCustomWorkspaceRolePermissions(ctx context.Context, workspaceID, roleID string) ([]string, error) {
	organizationID, err := s.workspaceRepo.GetWorkspaceOrganizationID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workspace organization: %w", err)
	}
	if strings.TrimSpace(organizationID) == "" {
		return []string{}, nil
	}

	var role model.WorkspaceCustomRole
	if err := s.db.WithContext(ctx).
		Where("id = ? AND group_id = ? AND status = ?", roleID, organizationID, model.WorkspaceCustomRoleStatusActive).
		First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workspace role not found")
		}
		return nil, fmt.Errorf("failed to get workspace role: %w", err)
	}
	return model.CanonicalAssignableWorkspacePermissionSnapshotStrings(role.Permissions), nil
}

// CreateWorkspace Create workspace
func (s *WorkspaceManagementServiceImpl) CreateWorkspace(ctx context.Context, name string, isFromDashboard bool) (*model.Workspace, error) {
	workspace := &model.Workspace{
		ID:        uuid.New().String(), // Generate new UUID
		Name:      name,
		Status:    model.WorkspaceStatusNormal,
		Plan:      "basic", // Set default plan
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.workspaceRepo.Create(ctx, workspace); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	return workspace, nil
}

// Get organization ID from workspace and parse it as UUID for legacy quota records.
func (s *WorkspaceManagementServiceImpl) getWorkspaceOrganizationUUID(ctx context.Context, workspaceID string) *uuid.UUID {
	if s.organizationService == nil {
		return nil
	}

	organization, err := s.organizationService.GetOrganizationByWorkspaceID(ctx, workspaceID)
	if err != nil || organization == nil {
		return nil
	}

	organizationUUID, parseErr := uuid.Parse(organization.ID)
	if parseErr != nil {
		return nil
	}

	return &organizationUUID
}

func (s *WorkspaceManagementServiceImpl) ensureSeatQuotaAvailable(ctx context.Context, organizationUUID *uuid.UUID) error {
	if organizationUUID == nil || s.quotaService == nil {
		return nil
	}

	canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, *organizationUUID, quota_model.ResourceTypeSeats, 1)
	if err != nil {
		return fmt.Errorf("failed to check seat quota: %w", err)
	}
	if !canProceed {
		return fmt.Errorf("seats quota exceeded: current=%d limit=%d attempted=1", currentUsage, limit)
	}

	return nil
}

// CreateWorkspaceMember Create workspace member
func (s *WorkspaceManagementServiceImpl) CreateWorkspaceMember(ctx context.Context, workspaceID string, accountID string, role string) error {
	// Step 1: Resolve the organization UUID from the workspace for legacy quota records.
	organizationUUID := s.getWorkspaceOrganizationUUID(ctx, workspaceID)

	if err := s.ensureSeatQuotaAvailable(ctx, organizationUUID); err != nil {
		return err
	}

	// Step 4: Get account information for metadata
	var account *auth_model.Account
	if s.accountService != nil {
		account, _ = s.accountService.GetAccountByID(ctx, accountID)
	}

	// Step 5: Create member in transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Create workspace member
		join := &model.WorkspaceMember{
			WorkspaceID: workspaceID,
			AccountID:   accountID,
			Role:        model.WorkspaceMemberRole(role),
			Current:     false,
		}
		if err := s.applyWorkspaceMemberPermissionTemplate(ctx, join); err != nil {
			return err
		}
		if err := s.workspaceMemberRepo.WithTx(tx).Create(ctx, join); err != nil {
			return fmt.Errorf("failed to create workspace member: %w", err)
		}

		// Step 6: Record quota usage if the organization UUID exists.
		if organizationUUID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse workspaceID to UUID
			workspaceUUID, err := uuid.Parse(workspaceID)
			if err != nil {
				return fmt.Errorf("failed to parse workspace ID: %w", err)
			}

			// Prepare metadata
			metadata := quota_model.JSONMap{
				"member_id": accountID,
				"role":      role,
			}
			if account != nil {
				metadata["member_name"] = account.Name
				metadata["member_email"] = account.Email
			}

			// Create usage history record
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeSeats,
				Delta:        1,
				ResourceID:   &accountID,
				Metadata:     &metadata,
			}

			// Set resource name
			if account != nil {
				usageRecord.ResourceName = &account.Name
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record quota usage: %w", err)
			}
		}

		return nil
	})
}

// GetWorkspaceByID Get workspace by ID
func (s *WorkspaceManagementServiceImpl) GetWorkspaceByID(ctx context.Context, id string) (*model.Workspace, error) {
	return s.workspaceRepo.GetByID(ctx, id)
}

// UpdateWorkspace Update workspace information
func (s *WorkspaceManagementServiceImpl) UpdateWorkspace(ctx context.Context, id string, req *interfaces.UpdateWorkspaceRequest) error {
	workspace, err := s.workspaceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	// Update fields
	if req.Name != "" {
		workspace.Name = req.Name
	}
	if req.Plan != "" {
		workspace.Plan = req.Plan
	}

	return s.workspaceRepo.Update(ctx, workspace)
}

// DeleteWorkspace Delete workspace
func (s *WorkspaceManagementServiceImpl) DeleteWorkspace(ctx context.Context, id string) error {
	return s.workspaceRepo.Delete(ctx, id)
}

// AddMember Add member
func (s *WorkspaceManagementServiceImpl) AddMember(ctx context.Context, req *interfaces.AddMemberRequest) error {
	// Validate workspace exists
	_, err := s.workspaceRepo.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	// Check if already a member
	existingJoin, _ := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, req.WorkspaceID, req.AccountID)
	if existingJoin != nil {
		return errors.New("account is already a member of this workspace")
	}

	organizationUUID := s.getWorkspaceOrganizationUUID(ctx, req.WorkspaceID)

	if err := s.ensureSeatQuotaAvailable(ctx, organizationUUID); err != nil {
		return err
	}

	// Get account information for metadata
	var account *auth_model.Account
	if s.accountService != nil {
		account, _ = s.accountService.GetAccountByID(ctx, req.AccountID)
	}

	// Create association in transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Create association
		join := &model.WorkspaceMember{
			WorkspaceID: req.WorkspaceID,
			AccountID:   req.AccountID,
			Role:        req.Role,
			RoleID:      req.RoleID,
		}
		if err := s.applyWorkspaceMemberPermissionTemplate(ctx, join); err != nil {
			return err
		}
		if req.Permissions != nil {
			return usererrors.NewNoPermissionError("direct permissions cannot be assigned while adding workspace member")
		}

		if err := s.workspaceMemberRepo.WithTx(tx).Create(ctx, join); err != nil {
			return fmt.Errorf("failed to create workspace member: %w", err)
		}

		// Record quota usage if the organization UUID exists.
		if organizationUUID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(req.AccountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse workspaceID to UUID
			workspaceUUID, err := uuid.Parse(req.WorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to parse workspace ID: %w", err)
			}

			// Prepare metadata
			metadata := quota_model.JSONMap{
				"member_id": req.AccountID,
				"role":      string(req.Role),
			}
			if req.RoleID != nil {
				metadata["role_id"] = *req.RoleID
			}
			if account != nil {
				metadata["member_name"] = account.Name
				metadata["member_email"] = account.Email
			}

			// Create usage history record
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeSeats,
				Delta:        1,
				ResourceID:   &req.AccountID,
				Metadata:     &metadata,
			}

			// Set resource name
			if account != nil {
				usageRecord.ResourceName = &account.Name
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record quota usage: %w", err)
			}
		}

		return nil
	})
}

// RemoveMember Remove member
func (s *WorkspaceManagementServiceImpl) RemoveMember(ctx context.Context, workspaceID, accountID string) error {
	return s.removeMemberInternal(ctx, workspaceID, accountID, false)
}

// LeaveWorkspace allows a member to leave a workspace, including owners
func (s *WorkspaceManagementServiceImpl) LeaveWorkspace(ctx context.Context, workspaceID, accountID string) error {
	// First verify that the user is actually in the workspace
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	if join == nil {
		return fmt.Errorf("member not found in workspace")
	}

	// Allow the member to leave regardless of their role (including owner)
	// This is a special case where users can leave their own workspace
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete workspace member
		if err := s.workspaceMemberRepo.WithTx(tx).Delete(ctx, join.ID); err != nil {
			return fmt.Errorf("failed to remove member from workspace: %w", err)
		}

		organizationUUID := s.getWorkspaceOrganizationUUID(ctx, workspaceID)

		// Get account information for metadata
		var account *auth_model.Account
		if s.accountService != nil {
			account, _ = s.accountService.GetAccountByID(ctx, accountID)
		}

		// Record quota usage decrease if the organization UUID exists.
		if organizationUUID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse workspaceID to UUID
			workspaceUUID, err := uuid.Parse(workspaceID)
			if err != nil {
				return fmt.Errorf("failed to parse workspace ID: %w", err)
			}

			// Prepare metadata
			metadata := quota_model.JSONMap{
				"member_id": accountID,
				"action":    "left",
			}
			if account != nil {
				metadata["member_name"] = account.Name
				metadata["member_email"] = account.Email
			}

			// Create usage history record with negative delta
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeSeats,
				Delta:        -1, // Negative delta for decrease
				ResourceID:   &accountID,
				Metadata:     &metadata,
			}

			// Set resource name
			if account != nil {
				usageRecord.ResourceName = &account.Name
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record quota usage: %w", err)
			}
		}

		return nil
	})
}

func (s *WorkspaceManagementServiceImpl) removeMemberInternal(ctx context.Context, workspaceID, accountID string, force bool) error {
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Check if join is nil
	if join == nil {
		return fmt.Errorf("member not found in workspace")
	}

	// Cannot remove owner unless forced
	if !force && join.Role == model.WorkspaceRoleOwner {
		return errors.New("cannot remove workspace owner")
	}

	organizationUUID := s.getWorkspaceOrganizationUUID(ctx, workspaceID)

	// Get account information for metadata
	var account *auth_model.Account
	if s.accountService != nil {
		account, _ = s.accountService.GetAccountByID(ctx, accountID)
	}

	// Delete member in transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete workspace member
		if err := s.workspaceMemberRepo.WithTx(tx).Delete(ctx, join.ID); err != nil {
			return fmt.Errorf("failed to delete workspace member: %w", err)
		}

		// Record quota usage decrease if the organization UUID exists.
		if organizationUUID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse workspaceID to UUID
			workspaceUUID, err := uuid.Parse(workspaceID)
			if err != nil {
				return fmt.Errorf("failed to parse workspace ID: %w", err)
			}

			// Prepare metadata
			metadata := quota_model.JSONMap{
				"member_id": accountID,
				"action":    "removed",
			}
			if account != nil {
				metadata["member_name"] = account.Name
				metadata["member_email"] = account.Email
			}

			// Create usage history record with negative delta
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeSeats,
				Delta:        -1, // Negative delta for decrease
				ResourceID:   &accountID,
				Metadata:     &metadata,
			}

			// Set resource name
			if account != nil {
				usageRecord.ResourceName = &account.Name
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record quota usage: %w", err)
			}
		}

		return nil
	})
}

// DeleteWorkspaceWithMembers Delete workspace and all its members (including owner)
func (s *WorkspaceManagementServiceImpl) DeleteWorkspaceWithMembers(ctx context.Context, id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		txSvc := s.WithTx(tx).(*WorkspaceManagementServiceImpl)

		// Get all members
		joins, err := txSvc.workspaceMemberRepo.GetJoinsByWorkspaceID(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get workspace members: %w", err)
		}

		// Remove all members (force=true to allow removing owner)
		for _, join := range joins {
			if err := txSvc.removeMemberInternal(ctx, id, join.AccountID, true); err != nil {
				return fmt.Errorf("failed to remove member %s: %w", join.AccountID, err)
			}
		}

		// Delete workspace
		return txSvc.DeleteWorkspace(ctx, id)
	})
}

// UpdateMemberRole Update member role
func (s *WorkspaceManagementServiceImpl) UpdateMemberRole(ctx context.Context, req *interfaces.UpdateMemberRoleRequest) error {
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, req.WorkspaceID, req.AccountID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Check if join is nil
	if join == nil {
		return fmt.Errorf("member not found in workspace")
	}

	// Cannot modify owner role
	if join.Role == model.WorkspaceRoleOwner {
		return errors.New("cannot change owner role")
	}

	join.Role = req.Role
	if roleID := workspaceRoleIDForRoleChange(req.Role); roleID != "" {
		join.RoleID = &roleID
	} else {
		join.RoleID = nil
	}
	join.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate
	join.PermissionTemplateRoleID = join.RoleID
	join.Permissions = nil
	if err := s.applyWorkspaceMemberPermissionTemplate(ctx, join); err != nil {
		return err
	}
	return s.workspaceMemberRepo.Update(ctx, join)
}

// GetWorkspaceMembers Get workspace members list
func (s *WorkspaceManagementServiceImpl) GetWorkspaceMembers(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error) {
	joins, err := s.workspaceMemberRepo.GetJoinsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace members: %w", err)
	}

	// Get organization member names
	memberNames := make(map[string]string)
	if s.organizationService != nil {
		org, err := s.organizationService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err == nil && org != nil {
			var accountIDs []string
			for _, join := range joins {
				accountIDs = append(accountIDs, join.AccountID)
			}

			if len(accountIDs) > 0 {
				type MemberInfo struct {
					AccountID string
					Name      string
				}
				var results []MemberInfo
				if err := s.db.WithContext(ctx).Table("members").
					Select("account_id, name").
					Where("organization_id = ? AND account_id IN ?", org.ID, accountIDs).
					Scan(&results).Error; err == nil {
					for _, r := range results {
						memberNames[r.AccountID] = r.Name
					}
				}
			}
		}
	}

	var members []*interfaces.AccountWithRole
	for _, join := range joins {
		account, err := s.accountService.GetAccountByID(ctx, join.AccountID)
		if err != nil {
			continue // Skip failed account retrieval
		}

		// Handle Avatar field (pointer to string)
		var avatar, avatarURL string
		if account.Avatar != nil {
			avatar = *account.Avatar
			avatarURL = *account.Avatar // In Go version, we temporarily use avatar as avatarURL
		}

		// Handle time field (*time.Time to *int64)
		var lastLoginAt, lastActiveAt *int64
		if account.LastLoginAt != nil {
			timestamp := account.LastLoginAt.Unix()
			lastLoginAt = &timestamp
		}
		if account.LastActiveAt != nil {
			timestamp := account.LastActiveAt.Unix()
			lastActiveAt = &timestamp
		}

		// Determine display name logic
		accountName := account.Name
		var memberNamePtr *string
		displayName := accountName

		if name, ok := memberNames[account.ID]; ok && name != "" {
			displayName = name
			memberNamePtr = &name
		}

		members = append(members, &interfaces.AccountWithRole{
			ID:           account.ID,
			Name:         displayName,
			AccountName:  accountName,
			MemberName:   memberNamePtr,
			Avatar:       avatar,
			AvatarURL:    avatarURL,
			Email:        account.Email,
			LastLoginAt:  lastLoginAt,
			LastActiveAt: lastActiveAt,
			CreatedAt:    account.CreatedAt.Unix(),
			Role:         string(join.Role),
			RoleID:       join.RoleID,
			Status:       string(account.Status),
			HasMobile:    account.MobileE164 != nil && strings.TrimSpace(*account.MobileE164) != "",
		})
	}

	return members, nil
}

func (s *WorkspaceManagementServiceImpl) GetWorkspaceMembersPaginated(ctx context.Context, workspaceID string, page, limit int, keyword, roleFilter string) ([]*interfaces.AccountWithRole, int64, error) {
	offset := (page - 1) * limit

	// Check if workspace belongs to an organization to join members table
	var orgID string
	if s.organizationService != nil {
		org, err := s.organizationService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err == nil && org != nil {
			orgID = org.ID
		}
	}

	type Result struct {
		AccountID                string                                `gorm:"column:id"`
		AccountName              string                                `gorm:"column:account_name"`
		Email                    string                                `gorm:"column:email"`
		Avatar                   *string                               `gorm:"column:avatar"`
		Status                   string                                `gorm:"column:status"`
		LastLoginAt              *time.Time                            `gorm:"column:last_login_at"`
		LastActiveAt             *time.Time                            `gorm:"column:last_active_at"`
		AccountCreatedAt         time.Time                             `gorm:"column:account_created_at"`
		MobileE164               *string                               `gorm:"column:mobile_e164"`
		Role                     string                                `gorm:"column:role"`
		RoleID                   *string                               `gorm:"column:role_id"`
		Permissions              string                                `gorm:"column:permissions"`
		PermissionSource         model.WorkspaceMemberPermissionSource `gorm:"column:permission_source"`
		PermissionTemplateRoleID *string                               `gorm:"column:permission_template_role_id"`
		MemberName               *string                               `gorm:"column:member_name"`
		OrganizationRole         *string                               `gorm:"column:organization_role"`
	}

	var results []Result
	var total int64

	query := s.db.WithContext(ctx).Table("workspace_members wm").
		Joins("JOIN accounts a ON wm.account_id = a.id").
		Where("wm.workspace_id = ?", workspaceID)

	fields := []string{
		"a.id", "a.name as account_name", "a.email", "a.avatar", "a.status",
		"a.last_login_at", "a.last_active_at", "a.created_at as account_created_at",
		"a.mobile_e164", "wm.role", "wm.role_id", "wm.permissions", "wm.permission_source",
		"wm.permission_template_role_id",
	}

	if orgID != "" {
		fields = append(fields, "om.name as member_name", "om.role as organization_role")
		query = query.Joins("LEFT JOIN members om ON om.account_id = a.id AND om.organization_id = ?", orgID)
	}

	query = query.Select(strings.Join(fields, ", "))

	if keyword != "" {
		kw := "%" + strings.ToLower(keyword) + "%"
		if orgID != "" {
			query = query.Where("(LOWER(a.name) LIKE ? OR LOWER(a.email) LIKE ? OR LOWER(om.name) LIKE ?)", kw, kw, kw)
		} else {
			query = query.Where("(LOWER(a.name) LIKE ? OR LOWER(a.email) LIKE ?)", kw, kw)
		}
	}

	if roleFilter != "" {
		query = query.Where("wm.role = ?", roleFilter)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.
		Order("CASE wm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END ASC").
		Order("wm.created_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&results).Error; err != nil {
		return nil, 0, err
	}

	accountIDs := make([]string, 0, len(results))
	for _, r := range results {
		accountIDs = append(accountIDs, r.AccountID)
	}
	departmentsByAccount, err := s.getOrganizationDepartmentDisplayByAccountIDs(ctx, orgID, accountIDs)
	if err != nil {
		return nil, 0, err
	}

	var members []*interfaces.AccountWithRole
	for _, r := range results {
		displayName := r.AccountName
		if r.MemberName != nil && *r.MemberName != "" {
			displayName = *r.MemberName
		}

		var lastLoginAt, lastActiveAt *int64
		if r.LastLoginAt != nil {
			ts := r.LastLoginAt.Unix()
			lastLoginAt = &ts
		}
		if r.LastActiveAt != nil {
			ts := r.LastActiveAt.Unix()
			lastActiveAt = &ts
		}

		avatar := ""
		if r.Avatar != nil {
			avatar = *r.Avatar
		}
		organizationRole := ""
		if r.OrganizationRole != nil {
			organizationRole = strings.TrimSpace(*r.OrganizationRole)
		}

		members = append(members, &interfaces.AccountWithRole{
			ID:                       r.AccountID,
			Name:                     displayName,
			AccountName:              r.AccountName,
			MemberName:               r.MemberName,
			Avatar:                   avatar,
			AvatarURL:                avatar,
			Email:                    r.Email,
			LastLoginAt:              lastLoginAt,
			LastActiveAt:             lastActiveAt,
			CreatedAt:                r.AccountCreatedAt.Unix(),
			Role:                     r.Role,
			RoleID:                   r.RoleID,
			Permissions:              expandWorkspaceMemberStoredPermissions(r.Role, r.RoleID, r.Permissions, r.PermissionSource),
			PermissionSource:         r.PermissionSource,
			PermissionTemplateRoleID: r.PermissionTemplateRoleID,
			Status:                   r.Status,
			HasMobile:                r.MobileE164 != nil && strings.TrimSpace(*r.MobileE164) != "",
			DepartmentID:             departmentsByAccount[r.AccountID].ID,
			DepartmentName:           departmentsByAccount[r.AccountID].Name,
			OrganizationRole:         organizationRole,
		})
	}

	return members, total, nil
}

type organizationDepartmentDisplay struct {
	ID   *string
	Name *string
}

func (s *WorkspaceManagementServiceImpl) getOrganizationDepartmentDisplayByAccountIDs(ctx context.Context, organizationID string, accountIDs []string) (map[string]organizationDepartmentDisplay, error) {
	result := make(map[string]organizationDepartmentDisplay, len(accountIDs))
	if organizationID == "" || len(accountIDs) == 0 {
		return result, nil
	}

	type departmentRow struct {
		AccountID      string `gorm:"column:account_id"`
		DepartmentID   string `gorm:"column:department_id"`
		DepartmentName string `gorm:"column:department_name"`
	}

	var rows []departmentRow
	if err := s.db.WithContext(ctx).
		Table("department_members dm").
		Select("dm.account_id, d.id AS department_id, d.name AS department_name").
		Joins("JOIN departments d ON d.id = dm.department_id").
		Where("dm.account_id IN ? AND d.group_id = ? AND d.status = ?", accountIDs, organizationID, model.DepartmentStatusActive).
		Order("d.sort_order ASC, d.created_at ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load workspace member departments: %w", err)
	}

	namesByAccount := make(map[string][]string)
	for _, row := range rows {
		if _, exists := result[row.AccountID]; !exists {
			departmentID := row.DepartmentID
			result[row.AccountID] = organizationDepartmentDisplay{ID: &departmentID}
		}
		if row.DepartmentName != "" {
			namesByAccount[row.AccountID] = append(namesByAccount[row.AccountID], row.DepartmentName)
		}
	}

	for accountID, names := range namesByAccount {
		display := result[accountID]
		name := strings.Join(names, ", ")
		display.Name = &name
		result[accountID] = display
	}

	return result, nil
}

func expandWorkspaceMemberStoredPermissions(role string, roleID *string, rawPermissions string, source model.WorkspaceMemberPermissionSource) []string {
	permissions := parseStoredWorkspacePermissions(rawPermissions)
	return workspaceMemberDisplayPermissionStrings(model.WorkspaceMemberRole(role), roleID, permissions, source)
}

func workspaceMemberResponsePermissions(join *model.WorkspaceMember) []string {
	if join == nil {
		return []string{}
	}
	return workspaceMemberDisplayPermissionStrings(
		join.Role,
		join.RoleID,
		join.Permissions,
		join.PermissionSource,
	)
}

func workspaceMemberDisplayPermissionStrings(
	role model.WorkspaceMemberRole,
	roleID *string,
	permissions []string,
	permissionSource model.WorkspaceMemberPermissionSource,
) []string {
	if role == model.WorkspaceRoleOwner || role == model.WorkspaceRoleAdmin {
		roleID := model.DefaultWorkspaceRoleID(role)
		return model.CanonicalAssignableWorkspacePermissionSnapshotStrings(
			model.DefaultWorkspaceMemberPermissionStrings(role, &roleID),
		)
	}

	effective := model.EffectiveWorkspaceMemberPermissionStrings(role, roleID, permissions, permissionSource)
	return model.CanonicalAssignableWorkspacePermissionSnapshotStrings(effective)
}

func parseStoredWorkspacePermissions(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []string{}
	}

	var permissions []string
	if err := json.Unmarshal([]byte(raw), &permissions); err != nil {
		return []string{}
	}
	return model.NormalizeWorkspacePermissionStrings(permissions)
}

func workspaceMemberActionPermission(action string) (model.WorkspacePermissionCode, bool) {
	switch action {
	case "add", "remove", "update":
		return model.WorkspacePermissionWorkspaceMemberManage, true
	case "permission":
		return model.WorkspacePermissionWorkspacePermissionManage, true
	default:
		return "", false
	}
}

func workspaceMemberJoinAllowsPermission(join *model.WorkspaceMember, permissionCode model.WorkspacePermissionCode) bool {
	if join == nil {
		return false
	}
	return model.WorkspaceMemberAllowsPermission(
		join.Role,
		join.RoleID,
		join.Permissions,
		join.PermissionSource,
		permissionCode,
	)
}

// GetWorkspaceMembersWithExtensions Get workspace members list with extensions
func (s *WorkspaceManagementServiceImpl) GetWorkspaceMembersWithExtensions(ctx context.Context, workspaceID string) ([]*interfaces.WorkspaceMemberWithExtensionResponse, error) {
	joins, err := s.workspaceMemberRepo.GetJoinsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace members: %w", err)
	}

	var members []*interfaces.WorkspaceMemberWithExtensionResponse
	for _, join := range joins {
		// Get account information by AccountID from join
		// Fix: Use accountService.GetAccountByID instead of accountRepo.GetAccountById
		account, err := s.accountService.GetAccountByID(ctx, join.AccountID)
		if err != nil {
			continue // Skip failed account retrieval
		}

		member := &interfaces.WorkspaceMemberWithExtensionResponse{
			Account:                  account,
			Role:                     join.Role,
			RoleID:                   join.RoleID,
			JoinedAt:                 join.CreatedAt,
			Extension:                join.Extensions,
			PermissionSource:         join.PermissionSource,
			PermissionTemplateRoleID: join.PermissionTemplateRoleID,
		}

		// If extension info exists, set position and permissions
		if join.Extensions != nil {
			if pos, ok := join.Extensions["position"].(string); ok {
				member.Position = pos
			}
		}
		member.Permissions = workspaceMemberResponsePermissions(join)

		members = append(members, member)
	}

	return members, nil
}

// GetAccountWorkspaces Get account's workspace list
func (s *WorkspaceManagementServiceImpl) GetAccountWorkspaces(ctx context.Context, accountID string) ([]*model.Workspace, error) {
	// Get workspace-account joins by account ID
	joins, err := s.workspaceMemberRepo.GetJoinsByMemberID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account workspaces: %w", err)
	}

	var workspaces []*model.Workspace
	for _, join := range joins {
		// Get workspace information by TenantID from join
		workspace, err := s.workspaceRepo.GetByID(ctx, join.WorkspaceID)
		if err != nil {
			continue // Skip failed workspace retrieval
		}
		workspaces = append(workspaces, workspace)
	}

	return workspaces, nil
}

// GetDatasetOperatorMembers Get dataset operator members list
func (s *WorkspaceManagementServiceImpl) GetDatasetOperatorMembers(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error) {
	joins, err := s.workspaceMemberRepo.GetJoinsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace members: %w", err)
	}

	var members []*interfaces.AccountWithRole
	for _, join := range joins {
		// Get account information
		// Fix: Use accountService.GetAccountByID instead of accountRepo.GetAccountById
		account, err := s.accountService.GetAccountByID(ctx, join.AccountID)
		if err != nil {
			continue // Skip failed account retrieval
		}

		// Handle Avatar field (pointer to string)
		var avatar, avatarURL string
		if account.Avatar != nil {
			avatar = *account.Avatar
			avatarURL = *account.Avatar // In Go version, we temporarily use avatar as avatarURL
		}

		// Handle time field (*time.Time to *int64)
		var lastLoginAt, lastActiveAt *int64
		if account.LastLoginAt != nil {
			timestamp := account.LastLoginAt.Unix()
			lastLoginAt = &timestamp
		}
		if account.LastActiveAt != nil {
			timestamp := account.LastActiveAt.Unix()
			lastActiveAt = &timestamp
		}

		members = append(members, &interfaces.AccountWithRole{
			ID:           account.ID,
			Name:         account.Name,
			Avatar:       avatar,
			AvatarURL:    avatarURL,
			Email:        account.Email,
			LastLoginAt:  lastLoginAt,
			LastActiveAt: lastActiveAt,
			CreatedAt:    account.CreatedAt.Unix(),
			Role:         string(join.Role), // Ensure role is dataset_operator
			RoleID:       join.RoleID,
			Status:       string(account.Status),
			HasMobile:    account.MobileE164 != nil && strings.TrimSpace(*account.MobileE164) != "",
		})
	}

	return members, nil
}

// CheckPermission Check permission
func (s *WorkspaceManagementServiceImpl) CheckPermission(ctx context.Context, workspaceID, accountID string) bool {
	allowed, err := s.isWorkspaceOrganizationAdminOrOwner(ctx, workspaceID, accountID)
	if err == nil && allowed {
		return true
	}

	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil || join == nil {
		return false
	}

	return workspaceMemberJoinAllowsPermission(join, model.WorkspacePermissionWorkspaceManage)
}

// CreateMemberExtension Create member extension information
func (s *WorkspaceManagementServiceImpl) CreateMemberExtension(ctx context.Context, req *interfaces.CreateMemberExtensionRequest) error {
	// Check if member exists
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, req.WorkspaceID, req.AccountID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}
	if join == nil {
		return fmt.Errorf("member not found in workspace")
	}

	// Update extension info
	if join.Extensions == nil {
		join.Extensions = make(map[string]interface{})
	}

	if req.Position != "" {
		join.Extensions["position"] = req.Position
	}

	return s.workspaceMemberRepo.Update(ctx, join)
}

// UpdateMemberExtension Update member extension information
func (s *WorkspaceManagementServiceImpl) UpdateMemberExtension(ctx context.Context, req *interfaces.UpdateMemberExtensionRequest) error {
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, req.WorkspaceID, req.AccountID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}
	if join == nil {
		return fmt.Errorf("member not found in workspace")
	}

	if join.Extensions == nil {
		join.Extensions = make(map[string]interface{})
	}

	if req.Position != nil {
		join.Extensions["position"] = *req.Position
	}

	return s.workspaceMemberRepo.Update(ctx, join)
}

func (s *WorkspaceManagementServiceImpl) GetJoinWorkspaces(ctx context.Context, account *auth_model.Account) ([]*model.Workspace, error) {
	joins, err := s.workspaceMemberRepo.GetJoinsByMemberID(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get join workspaces: %w", err)
	}

	workspaceIDs := make([]string, 0, len(joins))
	for _, join := range joins {
		workspaceIDs = append(workspaceIDs, join.WorkspaceID)
	}

	if len(workspaceIDs) == 0 {
		return []*model.Workspace{}, nil
	}

	workspaces, err := s.workspaceRepo.GetByIDs(ctx, workspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces by IDs: %w", err)
	}

	return workspaces, nil
}

// GetCurrentWorkspace Get account's current workspace
func (s *WorkspaceManagementServiceImpl) GetCurrentWorkspace(ctx context.Context, accountID string) (*model.WorkspaceMember, error) {
	return s.workspaceMemberRepo.GetCurrentWorkspace(ctx, accountID)
}

// GetCurrentOrganization Get account's current organization
func (s *WorkspaceManagementServiceImpl) GetCurrentOrganization(ctx context.Context, accountID string) (*model.OrganizationMember, error) {
	return s.workspaceMemberRepo.GetCurrentOrganization(ctx, accountID)
}

// TransferOwner transfers workspace ownership to another workspace member.
func (s *WorkspaceManagementServiceImpl) TransferOwner(ctx context.Context, workspaceID, operatorID, newOwnerID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := s.workspaceMemberRepo.WithTx(tx)

		// 1. Verify operator authority. Workspace owners and organization admins can transfer ownership.
		operatorJoin, err := repo.GetByWorkspaceAndMember(ctx, workspaceID, operatorID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		operatorIsWorkspaceOwner := operatorJoin != nil && operatorJoin.Role == model.WorkspaceRoleOwner
		if !operatorIsWorkspaceOwner {
			allowed, err := s.isWorkspaceOrganizationAdminOrOwner(ctx, workspaceID, operatorID)
			if err != nil {
				return err
			}
			if !allowed {
				return usererrors.NewNoPermissionError("Operator is not allowed to transfer this workspace")
			}
		}

		// 2. Verify new owner (target member)
		targetJoin, err := repo.GetByWorkspaceAndMember(ctx, workspaceID, newOwnerID)
		if err != nil {
			return err
		}
		if targetJoin == nil {
			return usererrors.ErrMemberNotInWorkspace
		}

		// 3. Normalize historical owner rows. Organization admins may manage a workspace
		// without being its owner or even a workspace member, so we always demote every
		// existing owner except the target member.
		joins, err := repo.GetJoinsByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return err
		}

		for _, join := range joins {
			if join == nil || join.Role != model.WorkspaceRoleOwner || join.AccountID == targetJoin.AccountID {
				continue
			}

			join.Role = model.WorkspaceRoleAdmin
			adminRoleID := model.WorkspaceBuiltinRoleAdminID
			join.RoleID = &adminRoleID
			join.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate
			join.PermissionTemplateRoleID = &adminRoleID
			join.Permissions = nil
			if err := s.applyWorkspaceMemberPermissionTemplate(ctx, join); err != nil {
				return err
			}

			if err := repo.Update(ctx, join); err != nil {
				return fmt.Errorf("failed to downgrade current owner: %w", err)
			}
		}

		// 4. Upgrade target member to Owner
		targetJoin.Role = model.WorkspaceRoleOwner
		ownerRoleID := model.WorkspaceBuiltinRoleOwnerID
		targetJoin.RoleID = &ownerRoleID
		targetJoin.PermissionSource = model.WorkspaceMemberPermissionSourceOwner
		targetJoin.PermissionTemplateRoleID = &ownerRoleID
		targetJoin.Permissions = nil
		if err := s.applyWorkspaceMemberPermissionTemplate(ctx, targetJoin); err != nil {
			return err
		}

		if err := repo.Update(ctx, targetJoin); err != nil {
			return fmt.Errorf("failed to upgrade target member to owner: %w", err)
		}

		return nil
	})
}

// CheckMemberPermission Check member permission
func (s *WorkspaceManagementServiceImpl) CheckMemberPermission(ctx context.Context, workspace *model.Workspace, operator *auth_model.Account, member *auth_model.Account, action string) error {
	if workspace == nil {
		return usererrors.NewNoPermissionError("workspace is nil in CheckMemberPermission")
	}
	if operator == nil {
		return usererrors.NewNoPermissionError("operator is nil in CheckMemberPermission")
	}

	requiredPermission, validAction := workspaceMemberActionPermission(action)
	if !validAction {
		return usererrors.NewInvalidActionError("invalid action")
	}

	if member != nil && operator.ID == member.ID {
		return usererrors.NewCannotOperateSelfError("cannot operate on self")
	}

	organizationAdmin, err := s.isWorkspaceOrganizationAdminOrOwner(ctx, workspace.ID, operator.ID)
	if err != nil {
		return err
	}
	if organizationAdmin {
		return nil
	}

	operatorJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, operator.ID)
	if err != nil {
		return fmt.Errorf("failed to get operator workspace membership: %w", err)
	}
	if operatorJoin == nil {
		return usererrors.NewNoPermissionError(fmt.Sprintf("no permission to %s", action))
	}
	if workspaceMemberJoinAllowsPermission(operatorJoin, requiredPermission) {
		return nil
	}

	return usererrors.NewNoPermissionError(fmt.Sprintf("no permission to %s", action))
}

func (s *WorkspaceManagementServiceImpl) isWorkspaceOrganizationAdminOrOwner(ctx context.Context, workspaceID, accountID string) (bool, error) {
	if s.organizationService == nil || s.workspaceRepo == nil {
		return false, nil
	}
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, usererrors.NewMemberNotInWorkspaceError("workspace not found")
		}
		return false, fmt.Errorf("failed to get workspace: %w", err)
	}
	if workspace == nil || workspace.OrganizationID == nil || strings.TrimSpace(*workspace.OrganizationID) == "" {
		return false, nil
	}
	allowed, err := s.organizationService.IsOrganizationAdminOrOwner(ctx, strings.TrimSpace(*workspace.OrganizationID), accountID)
	if err != nil {
		return false, fmt.Errorf("failed to check organization admin permission: %w", err)
	}
	return allowed, nil
}

// RemoveMemberFromWorkspace Remove member from workspace
func (s *WorkspaceManagementServiceImpl) RemoveMemberFromWorkspace(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, operator *auth_model.Account) error {
	// First check permissions
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "remove"); err != nil {
		return err
	}

	// Find member's workspace association record
	memberJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, member.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || memberJoin == nil {
			return usererrors.NewNoPermissionError("member is not a member of this workspace")
		}
		return fmt.Errorf("failed to find member in workspace: %w", err)
	}

	if memberJoin == nil {
		return usererrors.ErrMemberNotInWorkspace
	}
	if memberJoin.Role == model.WorkspaceRoleOwner {
		return usererrors.NewNoPermissionError("workspace owner must be transferred before removal")
	}

	organizationUUID := s.getWorkspaceOrganizationUUID(ctx, workspace.ID)

	// Delete member in transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete workspace member association record
		if err := s.workspaceMemberRepo.WithTx(tx).Delete(ctx, memberJoin.ID); err != nil {
			return fmt.Errorf("failed to remove member from workspace: %w", err)
		}

		// Record quota usage decrease if the organization UUID exists.
		if organizationUUID != nil && s.quotaService != nil {
			// Parse member ID to UUID
			memberUUID, err := uuid.Parse(member.ID)
			if err != nil {
				return fmt.Errorf("failed to parse member ID: %w", err)
			}

			// Parse workspaceID to UUID
			workspaceUUID, err := uuid.Parse(workspace.ID)
			if err != nil {
				return fmt.Errorf("failed to parse workspace ID: %w", err)
			}

			// Prepare metadata
			metadata := quota_model.JSONMap{
				"member_id": member.ID,
				"action":    "removed",
			}
			if member != nil {
				metadata["member_name"] = member.Name
				metadata["member_email"] = member.Email
			}

			// Create usage history record with negative delta
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *organizationUUID,
				AccountID:    memberUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeSeats,
				Delta:        -1, // Negative delta for decrease
				ResourceID:   &member.ID,
				Metadata:     &metadata,
			}

			// Set resource name
			if member != nil {
				usageRecord.ResourceName = &member.Name
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record quota usage: %w", err)
			}
		}

		return nil
	})
}

func (s *WorkspaceManagementServiceImpl) GetWorkspaceMemberWithExtensionsById(ctx context.Context, workspaceID, memberID string) (*interfaces.WorkspaceMemberWithExtensionResponse, error) {
	// Get workspace account association information
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, memberID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Member not in workspace
		}
		return nil, fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if join == nil {
		return nil, nil // Member not in workspace
	}

	// Get account information
	// Fix: Use accountService.GetAccountByID instead of accountRepo.GetAccountById
	account, err := s.accountService.GetAccountByID(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	member := &interfaces.WorkspaceMemberWithExtensionResponse{
		Account:                  account,
		Role:                     join.Role,
		RoleID:                   join.RoleID,
		JoinedAt:                 join.CreatedAt,
		Extension:                join.Extensions,
		PermissionSource:         join.PermissionSource,
		PermissionTemplateRoleID: join.PermissionTemplateRoleID,
	}

	// Set position and permissions
	if join.Extensions != nil {
		if pos, ok := join.Extensions["position"].(string); ok {
			member.Position = pos
		}
	}
	member.Permissions = workspaceMemberResponsePermissions(join)

	// Set GroupRole
	// Need to get GroupRole through AccountService
	// Due to service layer dependencies, need to set GroupRole in handler layer

	return member, nil
}

// ChangeWorkspaceWithJoin Change workspace with join
func (s *WorkspaceManagementServiceImpl) ChangeWorkspaceWithJoin(ctx context.Context, member *auth_model.Account, fromWorkspaceID, toWorkspaceID string, operator *auth_model.Account) error {
	// Check if member exists
	memberJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, fromWorkspaceID, member.ID)
	if err != nil {
		return fmt.Errorf("member not found in from workspace: %w", err)
	}

	// Check if memberJoin is nil
	if memberJoin == nil {
		return fmt.Errorf("member not found in from workspace")
	}

	// Check if target workspace exists
	_, err = s.workspaceRepo.GetByID(ctx, toWorkspaceID)
	if err != nil {
		return fmt.Errorf("target workspace not found: %w", err)
	}

	fromWorkspace, err := s.workspaceRepo.GetByID(ctx, fromWorkspaceID)
	if err != nil {
		return fmt.Errorf("from workspace not found: %w", err)
	}

	// Check permissions
	if err := s.CheckMemberPermission(ctx, fromWorkspace, operator, member, "remove"); err != nil {
		return err
	}

	// Remove member
	if err := s.RemoveMember(ctx, fromWorkspaceID, member.ID); err != nil {
		return fmt.Errorf("failed to remove member from from workspace: %w", err)
	}

	// Add member
	if err := s.AddMember(ctx, &interfaces.AddMemberRequest{
		WorkspaceID: toWorkspaceID,
		AccountID:   member.ID,
		Role:        model.WorkspaceRoleNormal,
	}); err != nil {
		return fmt.Errorf("failed to add member to to workspace: %w", err)
	}

	return nil
}

// UpdateMemberRoleExtensions Update member role extensions
func (s *WorkspaceManagementServiceImpl) UpdateMemberRoleExtensions(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, newRole, newPosition *string, newPermissions []string, operator *auth_model.Account) error {
	if newRole != nil || newPermissions != nil {
		if err := s.CheckMemberPermission(ctx, workspace, operator, member, "permission"); err != nil {
			return err
		}
	} else if newPosition != nil {
		if err := s.CheckMemberPermission(ctx, workspace, operator, member, "update"); err != nil {
			return err
		}
	}

	targetMemberJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, member.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
		}
		return fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if targetMemberJoin == nil {
		return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
	}
	if newPermissions != nil && model.WorkspaceMemberRoleHasGovernanceAuthority(targetMemberJoin.Role) {
		return usererrors.NewNoPermissionError("governance role permissions are managed by workspace role")
	}
	if newRole != nil && targetMemberJoin.Role == model.WorkspaceRoleOwner && *newRole != string(model.WorkspaceRoleOwner) {
		return usererrors.NewNoPermissionError("owner role can only be changed through ownership transfer")
	}

	// 4. Update role
	if newRole != nil {
		// Validate role
		role := model.WorkspaceMemberRole(*newRole)
		if !role.IsValidRole() {
			return fmt.Errorf("invalid role: %s", *newRole)
		}

		if *newRole == string(model.WorkspaceRoleOwner) {
			if err := s.TransferOwner(ctx, workspace.ID, operator.ID, member.ID); err != nil {
				return err
			}
			targetMemberJoin, err = s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, member.ID)
			if err != nil {
				return fmt.Errorf("failed to reload workspace account join: %w", err)
			}
			if targetMemberJoin == nil {
				return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
			}
		} else {
			// Update target member's role
			targetMemberJoin.Role = model.WorkspaceMemberRole(*newRole)
			if roleID := workspaceRoleIDForRoleChange(targetMemberJoin.Role); roleID != "" {
				targetMemberJoin.RoleID = &roleID
			} else {
				targetMemberJoin.RoleID = nil
			}
			targetMemberJoin.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate
			targetMemberJoin.PermissionTemplateRoleID = targetMemberJoin.RoleID
			targetMemberJoin.Permissions = nil
			if err := s.applyWorkspaceMemberPermissionTemplate(ctx, targetMemberJoin); err != nil {
				return err
			}
			if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
				return fmt.Errorf("failed to update workspace account join: %w", err)
			}
		}
	}

	// 5. Handle extension information update
	if newPosition != nil {
		if targetMemberJoin.Extensions == nil {
			targetMemberJoin.Extensions = make(map[string]interface{})
		}

		targetMemberJoin.Extensions["position"] = *newPosition

		if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
			return fmt.Errorf("failed to update workspace account join extensions: %w", err)
		}
	}

	if newPermissions != nil {
		applyWorkspaceMemberDirectPermissionSnapshot(targetMemberJoin, newPermissions)
		if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
			return fmt.Errorf("failed to update workspace account join permissions: %w", err)
		}
	}

	return nil
}

// UpdateMemberRoleWithPermissionCheck Update member role with permission check
func (s *WorkspaceManagementServiceImpl) UpdateMemberRoleWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, newRole string, operator *auth_model.Account) error {
	// 1. Check permissions
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "permission"); err != nil {
		return err
	}

	// 2. Get target member's current role
	targetMemberJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, member.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
		}
		return fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if targetMemberJoin == nil {
		return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
	}
	if targetMemberJoin.Role == model.WorkspaceRoleOwner && newRole != string(model.WorkspaceRoleOwner) {
		return usererrors.NewNoPermissionError("owner role can only be changed through ownership transfer")
	}

	if targetMemberJoin.Role == model.WorkspaceRoleOwner && newRole == string(model.WorkspaceRoleOwner) {
		return nil
	}

	if newRole == string(model.WorkspaceRoleOwner) {
		return s.TransferOwner(ctx, workspace.ID, operator.ID, member.ID)
	}

	// 3. Update target member's role. Reapplying the same role is intentional: role templates
	// are copied into member permission snapshots, so users need a way to refresh that snapshot.
	targetMemberJoin.Role = model.WorkspaceMemberRole(newRole)
	var roleID *string
	switch newRole {
	case string(model.WorkspaceRoleOwner):
		id := model.WorkspaceBuiltinRoleOwnerID
		roleID = &id
	case string(model.WorkspaceRoleAdmin):
		id := model.WorkspaceBuiltinRoleAdminID
		roleID = &id
	case string(model.WorkspaceRoleViewer):
		id := model.WorkspaceBuiltinRoleViewerID
		roleID = &id
	default:
		roleID = nil
	}
	targetMemberJoin.RoleID = roleID
	targetMemberJoin.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate
	targetMemberJoin.PermissionTemplateRoleID = roleID
	targetMemberJoin.Permissions = nil
	if err := s.applyWorkspaceMemberPermissionTemplate(ctx, targetMemberJoin); err != nil {
		return err
	}
	if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
		return fmt.Errorf("failed to update workspace account join: %w", err)
	}

	return nil
}

func (s *WorkspaceManagementServiceImpl) UpdateMemberRoleAndRoleIDWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, newRole string, roleID *string, operator *auth_model.Account) error {
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "permission"); err != nil {
		return err
	}

	targetMemberJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, member.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
		}
		return fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if targetMemberJoin == nil {
		return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
	}
	if targetMemberJoin.Role == model.WorkspaceRoleOwner && newRole != string(model.WorkspaceRoleOwner) {
		return usererrors.NewNoPermissionError("owner role can only be changed through ownership transfer")
	}

	if targetMemberJoin.Role == model.WorkspaceRoleOwner && newRole == string(model.WorkspaceRoleOwner) {
		return nil
	}

	if newRole == string(model.WorkspaceRoleOwner) {
		return s.TransferOwner(ctx, workspace.ID, operator.ID, member.ID)
	}

	targetMemberJoin.Role = model.WorkspaceMemberRole(newRole)
	targetMemberJoin.RoleID = roleID
	targetMemberJoin.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate
	targetMemberJoin.PermissionTemplateRoleID = roleID
	targetMemberJoin.Permissions = nil
	if err := s.applyWorkspaceMemberPermissionTemplate(ctx, targetMemberJoin); err != nil {
		return err
	}
	if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
		return fmt.Errorf("failed to update workspace account join: %w", err)
	}

	return nil
}

func (s *WorkspaceManagementServiceImpl) UpdateMemberCustomRoleWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, roleID string, operator *auth_model.Account) error {
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "permission"); err != nil {
		return err
	}

	targetMemberJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, member.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
		}
		return fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if targetMemberJoin == nil {
		return usererrors.NewMemberNotInWorkspaceError("Member not in workspace")
	}
	if targetMemberJoin.Role == model.WorkspaceRoleOwner {
		return usererrors.NewNoPermissionError("owner role can only be changed through ownership transfer")
	}
	if targetMemberJoin.Role == model.WorkspaceRoleAdmin {
		targetMemberJoin.Role = model.WorkspaceRoleNormal
	}

	targetMemberJoin.RoleID = &roleID
	targetMemberJoin.PermissionSource = model.WorkspaceMemberPermissionSourceRoleTemplate
	targetMemberJoin.PermissionTemplateRoleID = &roleID
	targetMemberJoin.Permissions = nil
	if err := s.applyWorkspaceMemberPermissionTemplate(ctx, targetMemberJoin); err != nil {
		return err
	}
	if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
		return fmt.Errorf("failed to update workspace account join: %w", err)
	}

	return nil
}

func workspaceRoleIDForRoleChange(role model.WorkspaceMemberRole) string {
	switch role {
	case model.WorkspaceRoleOwner:
		return model.WorkspaceBuiltinRoleOwnerID
	case model.WorkspaceRoleAdmin:
		return model.WorkspaceBuiltinRoleAdminID
	case model.WorkspaceRoleViewer:
		return model.WorkspaceBuiltinRoleViewerID
	default:
		return ""
	}
}

func (s *WorkspaceManagementServiceImpl) UpdateMemberDirectPermissions(ctx context.Context, workspaceID, accountID string, permissions []string) error {
	if workspaceID == "" || accountID == "" {
		return fmt.Errorf("invalid parameters")
	}

	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usererrors.NewMemberNotInWorkspaceError("member not in workspace")
		}
		return fmt.Errorf("failed to get workspace account join: %w", err)
	}
	if join == nil {
		return usererrors.NewMemberNotInWorkspaceError("member not in workspace")
	}
	if join.Role == model.WorkspaceRoleOwner {
		return usererrors.NewNoPermissionError("owner permissions are managed by ownership")
	}
	if join.Role == model.WorkspaceRoleAdmin {
		return usererrors.NewNoPermissionError("admin permissions are managed by workspace role")
	}

	applyWorkspaceMemberDirectPermissionSnapshot(join, permissions)
	if err := s.workspaceMemberRepo.Update(ctx, join); err != nil {
		return fmt.Errorf("failed to update workspace member permissions: %w", err)
	}

	return nil
}

// GetAccountWorkspaceJoins Get account workspace joins
func (s *WorkspaceManagementServiceImpl) GetAccountWorkspaceJoins(ctx context.Context, accountID string) ([]*model.WorkspaceMember, error) {
	return s.workspaceMemberRepo.GetJoinsByMemberID(ctx, accountID)
}

func (s *WorkspaceManagementServiceImpl) GetWorkspaceAccountJoins(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error) {
	return s.workspaceMemberRepo.GetJoinsByWorkspaceID(ctx, workspaceID)
}

func (s *WorkspaceManagementServiceImpl) GetWorkspacesByIDs(ctx context.Context, workspaceIDs []string) ([]*model.Workspace, error) {
	return s.workspaceRepo.GetByIDs(ctx, workspaceIDs)
}

func (s *WorkspaceManagementServiceImpl) SwitchWorkspace(ctx context.Context, accountID, workspaceID string) error {
	// 1. Update AccountContext (New Logic)
	// Get workspace to find organization ID
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if s.accountService != nil {
		// Update AccountContext with current organization and workspace
		if _, err := s.accountService.UpdateAccountContext(ctx, accountID, workspace.OrganizationID, &workspaceID); err != nil {
			return fmt.Errorf("failed to update account context: %w", err)
		}
	}

	// 2. Update WorkspaceMember (Legacy Logic)
	// Set current workspace to non-current
	if err := s.workspaceMemberRepo.ClearCurrentWorkspace(ctx, accountID); err != nil {
		return fmt.Errorf("failed to clear current workspace: %w", err)
	}

	// Set new current workspace
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		return fmt.Errorf("failed to find workspace join: %w", err)
	}

	// Check if join is nil
	if join == nil {
		// If user is not a member (e.g. Org Admin), we can't update join table.
		// But since we updated AccountContext, we consider the switch successful.
		return nil
	}

	join.Current = true
	if err := s.workspaceMemberRepo.Update(ctx, join); err != nil {
		return fmt.Errorf("failed to set current workspace: %w", err)
	}

	return nil
}

// GetUserRole Get user's role in workspace
func (s *WorkspaceManagementServiceImpl) GetUserRole(ctx context.Context, accountID, workspaceID string) (*model.WorkspaceMemberRole, error) {
	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // User not in workspace, return nil
		}
		return nil, fmt.Errorf("failed to get user role: %w", err)
	}

	// Check if join is nil
	if join == nil {
		return nil, nil
	}

	return &join.Role, nil
}

// GetAccessibleWorkspaceIDs returns workspace IDs the user joined directly.
func (s *WorkspaceManagementServiceImpl) GetAccessibleWorkspaceIDs(ctx context.Context, accountID string) ([]string, error) {
	joins, err := s.workspaceMemberRepo.GetJoinsByMemberID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get direct workspace joins: %w", err)
	}

	workspaceIDSet := make(map[string]bool)
	for _, join := range joins {
		workspaceIDSet[join.WorkspaceID] = true
	}

	result := make([]string, 0, len(workspaceIDSet))
	for workspaceID := range workspaceIDSet {
		result = append(result, workspaceID)
	}

	return result, nil
}

// GetWorkspaceIDsByOrganizationID gets all workspace IDs that belong to a specific organization.
func (s *WorkspaceManagementServiceImpl) GetWorkspaceIDsByOrganizationID(ctx context.Context, organizationID string) ([]string, error) {
	return s.workspaceRepo.GetWorkspaceIDsByOrganizationID(ctx, organizationID)
}

// GetUserWorkspaceMemberships Get user's department memberships (workspaces where current=false)
func (s *WorkspaceManagementServiceImpl) GetUserWorkspaceMemberships(ctx context.Context, accountID string) ([]interfaces.WorkspaceMembership, error) {
	// Query workspace_account_joins WHERE account_id = ? AND current = false
	joins, err := s.workspaceMemberRepo.GetJoinsByMemberID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user department memberships: %w", err)
	}

	// Filter for departments (current = false) and build result
	var departments []interfaces.WorkspaceMembership
	for _, join := range joins {
		if !join.Current {
			departments = append(departments, interfaces.WorkspaceMembership{
				WorkspaceID: join.WorkspaceID,
				Role:        join.Role,
			})
		}
	}

	return departments, nil
}
