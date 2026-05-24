package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
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
		AccountID        string     `gorm:"column:id"`
		AccountName      string     `gorm:"column:account_name"`
		Email            string     `gorm:"column:email"`
		Avatar           *string    `gorm:"column:avatar"`
		Status           string     `gorm:"column:status"`
		LastLoginAt      *time.Time `gorm:"column:last_login_at"`
		LastActiveAt     *time.Time `gorm:"column:last_active_at"`
		AccountCreatedAt time.Time  `gorm:"column:account_created_at"`
		MobileE164       *string    `gorm:"column:mobile_e164"`
		Role             string     `gorm:"column:role"`
		RoleID           *string    `gorm:"column:role_id"`
		MemberName       *string    `gorm:"column:member_name"`
	}

	var results []Result
	var total int64

	query := s.db.WithContext(ctx).Table("workspace_members wm").
		Joins("JOIN accounts a ON wm.account_id = a.id").
		Where("wm.workspace_id = ?", workspaceID)

	fields := []string{
		"a.id", "a.name as account_name", "a.email", "a.avatar", "a.status",
		"a.last_login_at", "a.last_active_at", "a.created_at as account_created_at",
		"a.mobile_e164", "wm.role", "wm.role_id",
	}

	if orgID != "" {
		fields = append(fields, "om.name as member_name")
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

	if err := query.Order("wm.created_at DESC").Offset(offset).Limit(limit).Scan(&results).Error; err != nil {
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

		members = append(members, &interfaces.AccountWithRole{
			ID:           r.AccountID,
			Name:         displayName,
			AccountName:  r.AccountName,
			MemberName:   r.MemberName,
			Avatar:       avatar,
			AvatarURL:    avatar,
			Email:        r.Email,
			LastLoginAt:  lastLoginAt,
			LastActiveAt: lastActiveAt,
			CreatedAt:    r.AccountCreatedAt.Unix(),
			Role:         r.Role,
			RoleID:       r.RoleID,
			Status:       r.Status,
			HasMobile:    r.MobileE164 != nil && strings.TrimSpace(*r.MobileE164) != "",
		})
	}

	return members, total, nil
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
			Account:   account,
			Role:      join.Role,
			JoinedAt:  join.CreatedAt,
			Extension: join.Extensions,
		}

		// If extension info exists, set position and permissions
		if join.Extensions != nil {
			if pos, ok := join.Extensions["position"].(string); ok {
				member.Position = pos
			}
			member.Permissions = []string{} // Permissions are handled by Role
		}

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
	isGroupAdmin, err := s.accountService.CheckOrganizationpAdminByWorkspace(ctx, accountID, workspaceID)

	// If operator not in workspace but is system admin or enterprise group admin, allow operation
	if isGroupAdmin {
		return true
	}

	join, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil || join == nil {
		return false
	}

	// Owner has all permissions
	if join.Role == model.WorkspaceRoleOwner {
		return true
	}

	// Admin has all permissions
	if join.Role == model.WorkspaceRoleAdmin {
		return true
	}

	// Viewer only has read permissions
	if join.Role == model.WorkspaceRoleNormal {
		return false
	}

	return false
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

// TransferOwner transfers workspace ownership from current owner to another member
func (s *WorkspaceManagementServiceImpl) TransferOwner(ctx context.Context, workspaceID, currentOwnerID, newOwnerID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := s.workspaceMemberRepo.WithTx(tx)

		// 1. Verify current owner
		currentJoin, err := repo.GetByWorkspaceAndMember(ctx, workspaceID, currentOwnerID)
		if err != nil {
			return err
		}
		if currentJoin == nil {
			return usererrors.NewMemberNotInWorkspaceError("Current owner not found in workspace")
		}
		if currentJoin.Role != model.WorkspaceRoleOwner {
			return usererrors.NewNoPermissionError("Operator is not the owner of this workspace")
		}

		// 2. Verify new owner (target member)
		targetJoin, err := repo.GetByWorkspaceAndMember(ctx, workspaceID, newOwnerID)
		if err != nil {
			return err
		}
		if targetJoin == nil {
			return usererrors.ErrMemberNotInWorkspace
		}
		if targetJoin.Role == model.WorkspaceRoleOwner {
			return nil // Already owner
		}

		// 3. Downgrade current owner Member
		currentJoin.Role = model.WorkspaceRoleNormal
		adminRoleID := model.WorkspaceBuiltinRoleMemberID
		currentJoin.RoleID = &adminRoleID

		if err := repo.Update(ctx, currentJoin); err != nil {
			return fmt.Errorf("failed to downgrade current owner: %w", err)
		}

		// 4. Upgrade target member to Owner
		targetJoin.Role = model.WorkspaceRoleOwner
		ownerRoleID := model.WorkspaceBuiltinRoleOwnerID
		targetJoin.RoleID = &ownerRoleID

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
	// Permission mapping
	perms := map[string][]model.WorkspaceMemberRole{
		"add":    {model.WorkspaceRoleOwner, model.WorkspaceRoleAdmin},
		"remove": {model.WorkspaceRoleOwner, model.WorkspaceRoleAdmin},
		"update": {model.WorkspaceRoleOwner, model.WorkspaceRoleAdmin},
	}

	// Check if action is valid
	allowedRoles, validAction := perms[action]
	if !validAction {
		return usererrors.NewInvalidActionError("Invalid action")
	}

	// Get operator's role in workspace
	operatorJoin, _ := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspace.ID, operator.ID)

	isGroupAdmin, err := s.accountService.CheckOrganizationpAdminByWorkspace(ctx, operator.ID, workspace.ID)

	// Check if operator is trying to operate on themselves
	if member != nil && operator.ID == member.ID {
		// Group admin can operate on themselves (e.g. join a workspace)
		if !isGroupAdmin {
			return usererrors.NewCannotOperateSelfError("Cannot operate on self")
		}
	}

	if (err != nil || operatorJoin == nil) && !isGroupAdmin {
		return usererrors.NewNoPermissionError(fmt.Sprintf("No permission to %s", action))
	}

	if isGroupAdmin {
		return nil
	}

	// Check if operator role has permission to execute this action
	if operatorJoin != nil {
		if slices.Contains(allowedRoles, operatorJoin.Role) {
			return nil
		}
	}

	return usererrors.NewNoPermissionError(fmt.Sprintf("No permission to %s", action))
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
		Account:   account,
		Role:      join.Role,
		JoinedAt:  join.CreatedAt,
		Extension: join.Extensions,
	}

	// Set position and permissions
	if join.Extensions != nil {
		if pos, ok := join.Extensions["position"].(string); ok {
			member.Position = pos
		}
		member.Permissions = []string{} // Permissions are handled by Role
	}

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
	// 1. Permission validation
	// 2. Check member permission
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "update"); err != nil {
		return err
	}

	// 3. Get target member's workspace association record
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

	// 4. Update role
	if newRole != nil {
		// Validate role
		role := model.WorkspaceMemberRole(*newRole)
		if !role.IsValidRole() {
			return fmt.Errorf("invalid role: %s", *newRole)
		}

		// Handle owner role special logic
		if *newRole == "owner" {
			// Find current owner and change their role to admin
			currentOwnerJoin, err := s.workspaceMemberRepo.GetOwnerByWorkspaceID(ctx, workspace.ID)
			if err == nil && currentOwnerJoin != nil {
				currentOwnerJoin.Role = model.WorkspaceRoleAdmin
				if err := s.workspaceMemberRepo.Update(ctx, currentOwnerJoin); err != nil {
					return fmt.Errorf("failed to update current owner role to admin: %w", err)
				}
			}
		}

		// Update target member's role
		targetMemberJoin.Role = model.WorkspaceMemberRole(*newRole)
		if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
			return fmt.Errorf("failed to update workspace account join: %w", err)
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

	// Permissions are no longer handled here (newPermissions ignored)

	return nil
}

// UpdateMemberRoleWithPermissionCheck Update member role with permission check
func (s *WorkspaceManagementServiceImpl) UpdateMemberRoleWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, newRole string, operator *auth_model.Account) error {
	// 1. Check permissions
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "update"); err != nil {
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

	// 3. Check if role is already the same
	if string(targetMemberJoin.Role) == newRole {
		return usererrors.NewRoleAlreadyAssignedError("The provided role is already assigned to the member")
	}

	// 4. Handle owner role special logic
	if newRole == "owner" {
		// Find current owner and change their role to admin
		currentOwnerJoin, err := s.workspaceMemberRepo.GetOwnerByWorkspaceID(ctx, workspace.ID)
		if err == nil && currentOwnerJoin != nil {
			currentOwnerJoin.Role = model.WorkspaceRoleAdmin
			adminRoleID := model.WorkspaceBuiltinRoleAdminID
			currentOwnerJoin.RoleID = &adminRoleID
			if err := s.workspaceMemberRepo.Update(ctx, currentOwnerJoin); err != nil {
				return fmt.Errorf("failed to update current owner role to admin: %w", err)
			}
		}
	}

	// 5. Update target member's role
	targetMemberJoin.Role = model.WorkspaceMemberRole(newRole)
	var roleID *string
	switch newRole {
	case string(model.WorkspaceRoleOwner):
		id := model.WorkspaceBuiltinRoleOwnerID
		roleID = &id
	case string(model.WorkspaceRoleAdmin):
		id := model.WorkspaceBuiltinRoleAdminID
		roleID = &id
	case string(model.WorkspaceRoleMember):
		id := model.WorkspaceBuiltinRoleMemberID
		roleID = &id
	case string(model.WorkspaceRoleViewer):
		id := model.WorkspaceBuiltinRoleViewerID
		roleID = &id
	default:
		roleID = nil
	}
	targetMemberJoin.RoleID = roleID
	if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
		return fmt.Errorf("failed to update workspace account join: %w", err)
	}

	return nil
}

func (s *WorkspaceManagementServiceImpl) UpdateMemberRoleAndRoleIDWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, newRole string, roleID *string, operator *auth_model.Account) error {
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "update"); err != nil {
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

	if string(targetMemberJoin.Role) == newRole {
		if roleID == nil && targetMemberJoin.RoleID == nil {
			return nil
		}
		if roleID != nil && targetMemberJoin.RoleID != nil && *targetMemberJoin.RoleID == *roleID {
			return nil
		}
	}

	if newRole == "owner" {
		currentOwnerJoin, err := s.workspaceMemberRepo.GetOwnerByWorkspaceID(ctx, workspace.ID)
		if err == nil && currentOwnerJoin != nil {
			currentOwnerJoin.Role = model.WorkspaceRoleAdmin
			if err := s.workspaceMemberRepo.Update(ctx, currentOwnerJoin); err != nil {
				return fmt.Errorf("failed to update current owner role to admin: %w", err)
			}
		}
	}

	targetMemberJoin.Role = model.WorkspaceMemberRole(newRole)
	targetMemberJoin.RoleID = roleID
	if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
		return fmt.Errorf("failed to update workspace account join: %w", err)
	}

	return nil
}

func (s *WorkspaceManagementServiceImpl) UpdateMemberCustomRoleWithPermissionCheck(ctx context.Context, workspace *model.Workspace, member *auth_model.Account, roleID string, operator *auth_model.Account) error {
	if err := s.CheckMemberPermission(ctx, workspace, operator, member, "update"); err != nil {
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

	if targetMemberJoin.RoleID != nil && *targetMemberJoin.RoleID == roleID {
		return usererrors.NewRoleAlreadyAssignedError("The provided role is already assigned to the member")
	}

	targetMemberJoin.RoleID = &roleID
	if err := s.workspaceMemberRepo.Update(ctx, targetMemberJoin); err != nil {
		return fmt.Errorf("failed to update workspace account join: %w", err)
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

// GetAccessibleWorkspaceIDs returns all workspace IDs the user can access, including direct and group-based permissions.
func (s *WorkspaceManagementServiceImpl) GetAccessibleWorkspaceIDs(ctx context.Context, accountID string) ([]string, error) {
	// 1. Get the workspaces the user joined directly
	joins, err := s.workspaceMemberRepo.GetJoinsByMemberID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get direct workspace joins: %w", err)
	}

	// Collect the directly joined workspace IDs
	workspaceIDSet := make(map[string]bool)
	for _, join := range joins {
		workspaceIDSet[join.WorkspaceID] = true
	}

	// 2. Get workspaces the user can access through group membership
	// Use batch queries to avoid N+1 query problems
	if s.accountService != nil {
		// Use the batch query method to get group permissions
		groupWorkspaceIDs, err := s.getGroupAdminWorkspaceIDs(ctx, accountID)
		if err != nil {
			// Log the error but do not stop execution
			logger.WarnContext(ctx, "failed to get group admin workspace ids", "account_id", accountID, err)
		} else {
			// Add the workspaces granted by group permissions
			for _, workspaceID := range groupWorkspaceIDs {
				workspaceIDSet[workspaceID] = true
			}
		}
	}

	// 3. Convert the set to a slice and return it
	result := make([]string, 0, len(workspaceIDSet))
	for workspaceID := range workspaceIDSet {
		result = append(result, workspaceID)
	}

	return result, nil
}

// getGroupAdminWorkspaceIDs batch-fetches the workspace IDs the user can access through group admin permissions.
func (s *WorkspaceManagementServiceImpl) getGroupAdminWorkspaceIDs(ctx context.Context, accountID string) ([]string, error) {
	// Query the database directly to avoid N+1 query problems
	// Use a JOIN to fetch all workspaces the user can access in one pass
	return s.workspaceRepo.GetWorkspaceIDsByGroupAdmin(ctx, accountID)
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
