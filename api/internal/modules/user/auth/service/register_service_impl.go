package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/config"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	"github.com/zgiai/zgi/api/pkg/email"
	"github.com/zgiai/zgi/api/pkg/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/shared/workspacebootstrap"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

type RegisterServiceImpl struct {
	db                        *gorm.DB
	accountRepo               auth_repo.AccountRepository
	invitationRepo            auth_repo.InvitationRepository
	tenantService             interfaces.WorkspaceManagementService
	enterpriseGroupService    interfaces.OrganizationManagementService
	tokenMgr                  *util.TokenManager
	billingService            interfaces.BillingService
	officialRouteBootstrapper interfaces.OfficialRouteBootstrapper
}

func NewRegisterService(
	db *gorm.DB,
	accountRepo auth_repo.AccountRepository,
	invitationRepo auth_repo.InvitationRepository,
	tenantService interfaces.WorkspaceManagementService,
	enterpriseGroupService interfaces.OrganizationManagementService,
	tokenMgr *util.TokenManager,
	billingService interfaces.BillingService,
) *RegisterServiceImpl {
	return &RegisterServiceImpl{
		db:                     db,
		accountRepo:            accountRepo,
		invitationRepo:         invitationRepo,
		tenantService:          tenantService,
		enterpriseGroupService: enterpriseGroupService,
		tokenMgr:               tokenMgr,
		billingService:         billingService,
	}
}

func (s *RegisterServiceImpl) ActivateCheck(ctx context.Context, workspaceID, email, token string) (map[string]interface{}, bool) {
	// Get invitation data from Redis
	invitationData, err := s.tokenMgr.GetInvitationByToken(token, workspaceID, email)
	if err != nil || invitationData == nil {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	// Validate tenant status
	tenant, err := s.tenantService.GetWorkspaceByID(ctx, invitationData.WorkspaceID)
	if err != nil || tenant == nil || tenant.Status != workspace_model.WorkspaceStatusNormal {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	// Validate account and tenant association (using existing method)
	invitationDataMap := map[string]string{
		"email": invitationData.Email,
	}
	tenantAccount, err := s.accountRepo.SelectAccountAndTenantAccountJoin(ctx, invitationDataMap, *tenant)
	if err != nil || tenantAccount == nil {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	// Validate account ID match
	if invitationData.AccountID != tenantAccount.Account.ID {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	return map[string]interface{}{
		"is_valid": true,
		"data": map[string]interface{}{
			"workspace_name": tenant.Name,
			"workspace_id":   tenant.ID,
			"email":          invitationData.Email,
		},
	}, true
}

func (s *RegisterServiceImpl) Activate(ctx context.Context, workspaceID, email, token, name, password, lang, timezone string) (interface{}, error) {
	// Validate invitation token
	invitationData, err := s.tokenMgr.GetInvitationByToken(token, workspaceID, email)
	if err != nil || invitationData == nil {
		return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
	}

	// Validate tenant status
	tenant, err := s.tenantService.GetWorkspaceByID(ctx, invitationData.WorkspaceID)
	if err != nil || tenant == nil || tenant.Status != workspace_model.WorkspaceStatusNormal {
		return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
	}

	// Validate account and tenant association (using existing method)
	invitationDataMap := map[string]string{
		"email": invitationData.Email,
	}
	tenantAccount, err := s.accountRepo.SelectAccountAndTenantAccountJoin(ctx, invitationDataMap, *tenant)
	if err != nil || tenantAccount == nil {
		return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
	}

	account := &tenantAccount.Account

	// Validate account ID match
	if invitationData.AccountID != account.ID {
		return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
	}

	if err := s.tokenMgr.RevokeInvitationToken(workspaceID, email, token); err != nil {
		return nil, fmt.Errorf("failed to revoke token: %w", err)
	}

	account.Name = name
	if lang != "" {
		account.InterfaceLanguage = &lang
	}
	if timezone != "" {
		account.Timezone = &timezone
	}
	theme := "light"
	account.InterfaceTheme = &theme
	account.Status = auth_model.AccountStatusActive

	// Set initialization time
	now := time.Now().UTC()
	account.InitializedAt = &now

	// Update account
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to update account: %w", err)
	}

	// For public deployment, create user's own group and related data (similar to registration)
	if s.isPublicDeployment() {
		if err := s.initializeUserOwnGroup(ctx, account); err != nil {
			// Log warning but don't fail activation
			logger.Warn("Failed to initialize user's own group during activation: %v", err)
		}
	}

	return account, nil
}

// isPublicDeployment checks if public deployment mode is enabled
func (s *RegisterServiceImpl) isPublicDeployment() bool {
	return config.Current().Feature.PublicDeploymentEnabled
}

// initializeUserOwnGroup creates user's own group and related data for public deployment
func (s *RegisterServiceImpl) initializeUserOwnGroup(ctx context.Context, account *auth_model.Account) error {
	var defaultOrganizationID string
	var defaultWorkspaceID string

	// Use transaction for atomicity
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		groupService := s.enterpriseGroupService.WithTx(tx)
		tenantService := s.tenantService.WithTx(tx)

		// Create user's own group
		groupName, err := uniqueOwnedOrganizationName(ctx, groupService, account.Name, account.InterfaceLanguage)
		if err != nil {
			return fmt.Errorf("failed to prepare group name: %w", err)
		}
		group, err := groupService.CreateOrganization(ctx, groupName)
		if err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}

		// Set user as group owner
		if err := groupService.UpsertOrganizationRole(ctx, group.ID, account.ID, workspace_model.OrganizationRoleOwner); err != nil {
			return fmt.Errorf("failed to upsert group role: %w", err)
		}

		// Create default workspace
		defaultTenant, err := tenantService.CreateWorkspace(ctx, fmt.Sprintf("%s's Workspace", account.Name), true)
		if err != nil {
			return fmt.Errorf("failed to create default tenant: %w", err)
		}

		// Add tenant to group
		if err := groupService.AddWorkspace(ctx, group.ID, defaultTenant.ID); err != nil {
			return fmt.Errorf("failed to add tenant to group: %w", err)
		}

		if err := workspacebootstrap.EnsureOwnerWorkspaceMember(ctx, tenantService, account.ID, defaultTenant.ID); err != nil {
			return fmt.Errorf("failed to initialize default workspace state: %w", err)
		}
		defaultOrganizationID = group.ID
		defaultWorkspaceID = defaultTenant.ID

		logger.Info("Initialized user's own group for activated user",
			"account_id", account.ID,
			"group_id", group.ID)

		return nil
	}); err != nil {
		return err
	}

	if err := s.initializeAccountWorkspaceContext(ctx, account.ID, defaultOrganizationID, defaultWorkspaceID); err != nil {
		return err
	}

	s.bootstrapOfficialRoute(ctx, defaultOrganizationID)
	return nil
}

func (s *RegisterServiceImpl) bootstrapOfficialRoute(ctx context.Context, organizationID string) {
	if s.officialRouteBootstrapper == nil || organizationID == "" {
		return
	}

	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil {
		logger.Warn("Failed to parse organization ID for official route bootstrap: %v", err)
		return
	}

	if err := s.officialRouteBootstrapper.InitOfficialChannel(ctx, organizationUUID); err != nil {
		logger.Warn("Failed to bootstrap official route after activation: %v", err)
	}
}

func (s *RegisterServiceImpl) initializeAccountWorkspaceContext(ctx context.Context, accountID, organizationID, workspaceID string) error {
	if organizationID == "" || workspaceID == "" {
		return nil
	}

	ctxModel, err := s.accountRepo.GetAccountContextByAccountID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("failed to get account context: %w", err)
	}

	now := time.Now()
	if ctxModel == nil {
		ctxModel = &auth_model.AccountContext{
			AccountID:             accountID,
			CurrentOrganizationID: &organizationID,
			CurrentWorkspaceID:    &workspaceID,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := s.accountRepo.CreateAccountContext(ctx, ctxModel); err != nil {
			return fmt.Errorf("failed to create account context: %w", err)
		}
		return nil
	}

	ctxModel.CurrentOrganizationID = &organizationID
	ctxModel.CurrentWorkspaceID = &workspaceID
	ctxModel.UpdatedAt = now

	if err := s.accountRepo.UpdateAccountContext(ctx, ctxModel); err != nil {
		return fmt.Errorf("failed to update account context: %w", err)
	}

	return nil
}

func (s *RegisterServiceImpl) SetOfficialRouteBootstrapper(bootstrapper interfaces.OfficialRouteBootstrapper) {
	s.officialRouteBootstrapper = bootstrapper
}

func (s *RegisterServiceImpl) CheckRegisterValidity(ctx context.Context, email, code, token string) (bool, error) {
	exists, err := s.accountRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}
	if exists {
		return false, errors.New("email already exists")
	}

	// Verify code from token
	if token != "" && code != "" {
		tokenData, err := s.tokenMgr.GetTokenData(token, "register")
		if err == nil && tokenData != nil && tokenData.Extra != nil {
			if storedCode, ok := tokenData.Extra["code"].(string); ok {
				// Master verification code for testing/development
				masterCode := config.Current().Auth.MasterVerificationCode
				if storedCode != code && (masterCode == "" || code != masterCode) {
					return false, errors.New("invalid verification code")
				}
			}
		}
	}

	return true, nil
}

func (s *RegisterServiceImpl) RegisterFinish(ctx context.Context, email, name, password string) (*auth_model.Account, error) {
	exists, err := s.accountRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to check email existence: %w", err)
	}
	if exists {
		return nil, errors.New("email already exists")
	}

	account := &auth_model.Account{
		ID:     uuid.New().String(),
		Email:  email,
		Name:   name,
		Status: auth_model.AccountStatusActive,
	}

	if password != "" {
		// hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		// if err != nil {
		//     return nil, fmt.Errorf("failed to hash password: %w", err)
		// }
		// account.Password = string(hashedPassword)
	}

	now := time.Now().UTC()
	account.InitializedAt = &now

	if err := s.accountRepo.CreateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}

func (s *RegisterServiceImpl) Register(ctx context.Context, email string, password string, name string) (interface{}, error) {

	panic("implement me")

}

func (s *RegisterServiceImpl) InviteMember(ctx context.Context, tenantID, inviterID, email string, role workspace_model.WorkspaceMemberRole, language string) (string, error) {
	return s.InviteMemberEx(ctx, tenantID, inviterID, email, role, language, "", "", "", "", true)
}

func (s *RegisterServiceImpl) InviteMemberEx(ctx context.Context, tenantID, inviterID, email string, role workspace_model.WorkspaceMemberRole, language string, name, mobile, gender, position string, sendEmail bool) (string, error) {

	// Begin transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return "", tx.Error
	}

	var committed bool
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
		if !committed {
			tx.Rollback()
		}
	}()

	tenant, err := s.tenantService.GetWorkspaceByID(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("tenant not found: %w", err)
	}

	inviter, err := s.accountRepo.GetAccount(ctx, inviterID)
	if err != nil {
		return "", fmt.Errorf("inviter not found: %w", err)
	}

	account, err := s.accountRepo.GetAccountByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("failed to check account: %w", err)
	}

	var ta *workspace_model.WorkspaceMember = nil

	if account == nil {
		// Create new account with extended info
		account = &auth_model.Account{
			ID:                uuid.New().String(),
			Email:             email,
			Name:              name,
			Status:            auth_model.AccountStatusPending,
			InterfaceLanguage: &language,
			Extensions:        make(auth_model.JSONMap),
		}

		if mobile != "" {
			account.Extensions["mobile"] = mobile
		}
		if gender != "" {
			account.Extensions["gender"] = gender
		}

		if err := tx.Create(account).Error; err != nil {
			return "", fmt.Errorf("failed to create account: %w", err)
		}
	} else {
		if err := s.tenantService.CheckMemberPermission(ctx, tenant, inviter, account, "add"); err != nil {
			return "", err
		}
		ta, _ = s.tenantService.GetByWorkspaceAndMember(ctx, tenantID, account.ID)
		if ta != nil && account.Status != auth_model.AccountStatusPending {
			// User already exists in tenant
			return "", usererrors.ErrAccountAlreadyInWorkspace
		}
	}

	if ta == nil {
		// Prepare extensions
		extMap := make(map[string]interface{})
		if position != "" {
			extMap["position"] = position
		}

		// Create tenant member relationship
		tenantMember := &workspace_model.WorkspaceMember{
			ID:          uuid.New().String(),
			WorkspaceID: tenantID,
			AccountID:   account.ID,
			Role:        role,
			InvitedBy:   &inviterID,
			Current:     false, // Set to false initially for pending invitations
			Extensions:  extMap,
		}
		workspace_model.ApplyWorkspaceMemberDefaults(tenantMember)

		if err := tx.Create(tenantMember).Error; err != nil {
			return "", fmt.Errorf("failed to create tenant member: %w", err)
		}

		// === Add member to enterprise group ===
		// 1. Get the enterprise group associated with this tenant
		var currentTenant workspace_model.Workspace
		if err := tx.Select("organization_id").Where("id = ?", tenantID).First(&currentTenant).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return "", fmt.Errorf("failed to get tenant info: %w", err)
			}
		} else if currentTenant.OrganizationID != nil && *currentTenant.OrganizationID != "" {
			groupID := *currentTenant.OrganizationID

			// 2. Create group member using transaction
			txGroupSvc := s.enterpriseGroupService.WithTx(tx)
			txTenantSvc := s.tenantService.WithTx(tx)

			// 3. Add enterprise group member role (default: normal, idempotent operation)
			if err := txGroupSvc.UpsertOrganizationRole(ctx, groupID, account.ID, workspace_model.OrganizationRoleNormal); err != nil {
				return "", fmt.Errorf("failed to add member to enterprise group: %w", err)
			}

			// 4. Add to shadowTenant (shadowTenant.ID = groupID)
			// Check if already exists to avoid duplicate creation (user may have joined via another tenant)
			existingJoin, _ := txTenantSvc.GetByWorkspaceAndMember(ctx, groupID, account.ID)
			if existingJoin == nil {
				if err := txTenantSvc.CreateWorkspaceMember(ctx, groupID, account.ID, string(workspace_model.WorkspaceRoleNormal)); err != nil {
					return "", fmt.Errorf("failed to add member to shadow tenant: %w", err)
				}
			}
		}
		// === End of enterprise group member addition ===

		// Commit transaction
		if err := tx.Commit().Error; err != nil {
			return "", fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	// Generate invitation token and store in Redis
	inviteToken := s.generateInviteToken(tenant, account)

	committed = true

	// Send email if requested
	if sendEmail {
		inviterName := inviter.Name
		workspaceName := tenant.Name
		if workspaceName == "" {
			workspaceName = "ZGIAI Workspace"
		}

		// Send email asynchronously
		go func() {
			if err := s.sendInviteMemberMail(language, email, inviteToken, inviterName, workspaceName); err != nil {
				logger.WarnContext(ctx, "failed to send invite email", "email", email, err)
			}
		}()
	}

	return inviteToken, nil
}

func (s *RegisterServiceImpl) JoinByInvitation(ctx context.Context, accountID, inviteToken string) (*workspace_model.Workspace, error) {
	// Begin transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	var committed bool
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
		if !committed {
			tx.Rollback()
		}
	}()

	invitation, err := s.getInvitationByToken(inviteToken)
	if err != nil {
		return nil, fmt.Errorf("invalid invitation code: %w", err)
	}

	_, err = s.accountRepo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	tenant, err := s.tenantService.GetWorkspaceByID(ctx, invitation.TenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	// Check if tenant member relationship exists
	tenantMember, err := s.tenantService.GetByWorkspaceAndMember(ctx, invitation.TenantID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check tenant member: %w", err)
	}

	if tenantMember != nil {
		// Update current status
		tenantMember.Current = true
		if err := tx.Save(tenantMember).Error; err != nil {
			return nil, fmt.Errorf("failed to update tenant member: %w", err)
		}
	}

	// Revoke invitation token
	s.revokeToken(inviteToken)

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	committed = true
	return tenant, nil
}

func (s *RegisterServiceImpl) getInvitationByToken(token string) (*auth_model.Invitation, error) {
	// Should actually get from Redis or database
	// Simple simulation here
	return &auth_model.Invitation{
		TenantID: "tenant-id",
		Role:     workspace_model.WorkspaceRoleOwner,
	}, nil
}

func (s *RegisterServiceImpl) generateInviteToken(tenant *workspace_model.Workspace, account *auth_model.Account) string {
	token := uuid.New().String()

	expiryHours := 72
	err := s.tokenMgr.StoreInvitationToken(tenant.ID, account.Email, account.ID, token, expiryHours)
	if err != nil {
		// Fallback to simple token generation
		return fmt.Sprintf("%s-%s-%d", tenant.ID, account.ID, time.Now().Unix())
	}

	return token
}

func (s *RegisterServiceImpl) revokeToken(token string) error {
	return nil
}

func (s *RegisterServiceImpl) sendInviteMemberMail(language, to, token, inviterName, workspaceName string) error {
	return email.SendInviteMemberMailTask(language, to, token, inviterName, workspaceName)
}

func (s *RegisterServiceImpl) ValidateInvitationCode(ctx context.Context, code string) (bool, error) {
	_, err := s.GetInvitationData(ctx, code)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *RegisterServiceImpl) GetInvitationData(ctx context.Context, token string) (map[string]interface{}, error) {
	invitationData, err := s.tokenMgr.GetInvitationByToken(token, "", "")
	if err != nil {
		return nil, fmt.Errorf("invalid invitation token: %w", err)
	}

	result := make(map[string]interface{})
	data, _ := json.Marshal(&invitationData)
	_ = json.Unmarshal(data, &result)
	return result, nil
}

func (s *RegisterServiceImpl) GetInvitationIfTokenValid(workspace_id *string, email, token string) map[string]interface{} {
	workspaceID := ""
	if workspace_id != nil {
		workspaceID = *workspace_id
	}

	invitationData, err := s.tokenMgr.GetInvitationByToken(token, workspaceID, email)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	invitedEmail := invitationData.Email
	if invitedEmail == "" || invitedEmail != email {
		return map[string]interface{}{
			"error": "email does not match invitation",
		}
	}

	return map[string]interface{}{
		"account_id":   invitationData.AccountID,
		"email":        invitationData.Email,
		"workspace_id": invitationData.WorkspaceID,
	}
}
