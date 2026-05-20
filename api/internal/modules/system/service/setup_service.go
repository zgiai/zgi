package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	auth_repo "github.com/zgiai/ginext/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/internal/util"
	"gorm.io/gorm"

	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"

	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/system/model"
	"github.com/zgiai/ginext/internal/modules/system/repository"
)

const (
	defaultOrganizationName = "Default Group"
	defaultWorkspaceName    = "Default Workspace"
	bootstrapLockKey        = "cloud_setup"
)

type BootstrapSource string

const (
	BootstrapSourceSelfHostedHTTP BootstrapSource = "self_hosted_http"
	BootstrapSourceCloudEnv       BootstrapSource = "cloud_env"
)

// BootstrapParams describes first-time initialization input.
type BootstrapParams struct {
	AdminEmail    string
	AdminName     string
	AdminPassword string
	IPAddress     string
	Source        BootstrapSource
}

// BootstrapService owns first-time business initialization.
type BootstrapService struct {
	repo            repository.SetupRepository
	lockRepo        *repository.BootstrapLockRepository
	accountRepo     auth_repo.AccountRepository
	db              *gorm.DB
	tenantSvc       interfaces.WorkspaceManagementService
	groupSvc        interfaces.OrganizationManagementService
	systemConfigSvc SystemConfigService
}

// NewBootstrapService creates a bootstrap service with concrete dependencies.
func NewBootstrapService(
	repo repository.SetupRepository,
	lockRepo *repository.BootstrapLockRepository,
	accountRepo auth_repo.AccountRepository,
	db *gorm.DB,
	tenantSvc interfaces.WorkspaceManagementService,
	groupSvc interfaces.OrganizationManagementService,
	systemConfigSvc SystemConfigService,
) *BootstrapService {
	return &BootstrapService{
		repo:            repo,
		lockRepo:        lockRepo,
		accountRepo:     accountRepo,
		db:              db,
		tenantSvc:       tenantSvc,
		groupSvc:        groupSvc,
		systemConfigSvc: systemConfigSvc,
	}
}

// GetSetupStatus returns the persisted setup marker when bootstrap already finished.
func (s *BootstrapService) GetSetupStatus() (*model.Setup, error) {
	return s.repo.GetSetupStatus()
}

// GetTenantCount returns the number of workspaces used to detect partial initialization.
func (s *BootstrapService) GetTenantCount() (int64, error) {
	return s.repo.GetTenantCount()
}

// GetInitValidateStatus reports whether initialization prechecks have passed.
func (s *BootstrapService) GetInitValidateStatus() (bool, error) {
	return true, nil
}

// Setup adapts self-hosted HTTP input into the shared bootstrap flow.
func (s *BootstrapService) Setup(ctx context.Context, email, name, password, ipAddress string) error {
	return s.Bootstrap(ctx, BootstrapParams{
		AdminEmail:    email,
		AdminName:     name,
		AdminPassword: password,
		IPAddress:     ipAddress,
		Source:        BootstrapSourceSelfHostedHTTP,
	})
}

// Bootstrap performs first-time business initialization inside one transaction.
func (s *BootstrapService) Bootstrap(ctx context.Context, params BootstrapParams) error {
	if err := ValidatePassword(params.AdminPassword); err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txSetupRepo := repository.NewSetupRepository(tx)
		txLockRepo := s.lockRepo.WithTx(tx)
		txAccountRepo := s.accountRepo.WithTx(tx)
		txTenantSvc := s.tenantSvc.WithTx(tx)
		txGroupSvc := s.groupSvc.WithTx(tx)

		if err := txLockRepo.EnsureLockRow(ctx, bootstrapLockKey); err != nil {
			return fmt.Errorf("ensure bootstrap lock: %w", err)
		}
		if _, err := txLockRepo.LockForUpdate(ctx, bootstrapLockKey); err != nil {
			return fmt.Errorf("acquire bootstrap lock: %w", err)
		}

		if err := s.ensureBootstrapReady(txSetupRepo); err != nil {
			return err
		}

		account, err := s.createBootstrapAccount(ctx, txAccountRepo, params)
		if err != nil {
			return err
		}

		organization, err := txGroupSvc.CreateOrganization(ctx, defaultOrganizationName)
		if err != nil {
			return fmt.Errorf("create default organization: %w", err)
		}

		workspace, err := txTenantSvc.CreateWorkspace(ctx, defaultWorkspaceName, true)
		if err != nil {
			return fmt.Errorf("create default workspace: %w", err)
		}

		if err := txGroupSvc.AddWorkspace(ctx, organization.ID, workspace.ID); err != nil {
			return fmt.Errorf("link workspace to organization: %w", err)
		}

		if err := txTenantSvc.CreateWorkspaceMember(ctx, workspace.ID, account.ID, string(workspace_model.WorkspaceRoleOwner)); err != nil {
			return fmt.Errorf("create default workspace member: %w", err)
		}

		if err := txGroupSvc.UpsertOrganizationRole(ctx, organization.ID, account.ID, workspace_model.OrganizationRoleOwner); err != nil {
			return fmt.Errorf("upsert default organization role: %w", err)
		}

		if err := txAccountRepo.CreateAccountContext(ctx, &auth_model.AccountContext{
			AccountID:             account.ID,
			CurrentOrganizationID: &organization.ID,
			CurrentWorkspaceID:    &workspace.ID,
		}); err != nil {
			return fmt.Errorf("create account context: %w", err)
		}

		if s.systemConfigSvc != nil {
			if err := s.systemConfigSvc.ConfigDefaultPluginAndConfig(ctx, organization.ID, account); err != nil {
				return fmt.Errorf("configure default plugins: %w", err)
			}
		}

		if err := txSetupRepo.CreateSetup(); err != nil {
			return fmt.Errorf("create setup marker: %w", err)
		}

		return nil
	})
}

func (s *BootstrapService) ensureBootstrapReady(repo repository.SetupRepository) error {
	setupStatus, err := repo.GetSetupStatus()
	if err != nil {
		return fmt.Errorf("get setup status: %w", err)
	}
	if setupStatus != nil {
		return ErrAlreadySetup
	}

	tenantCount, err := repo.GetTenantCount()
	if err != nil {
		return fmt.Errorf("get tenant count: %w", err)
	}
	if tenantCount > 0 {
		return ErrAlreadySetup
	}

	initValidated, err := repo.GetInitValidateStatus()
	if err != nil {
		return fmt.Errorf("get init validate status: %w", err)
	}
	if !initValidated {
		return ErrNotInitValidated
	}

	return nil
}

func (s *BootstrapService) createBootstrapAccount(
	ctx context.Context,
	accountRepo auth_repo.AccountRepository,
	params BootstrapParams,
) (*auth_model.Account, error) {
	exists, err := accountRepo.ExistsByEmail(ctx, params.AdminEmail)
	if err != nil {
		return nil, fmt.Errorf("check bootstrap admin email: %w", err)
	}
	if exists {
		return nil, ErrBootstrapAdminEmailExists
	}

	hashedPassword, salt, err := util.HashPasswordPBKDF2(params.AdminPassword)
	if err != nil {
		return nil, fmt.Errorf("hash bootstrap admin password: %w", err)
	}

	now := time.Now()
	account := &auth_model.Account{
		Name:          strings.TrimSpace(params.AdminName),
		Email:         strings.TrimSpace(params.AdminEmail),
		Password:      &hashedPassword,
		PasswordSalt:  &salt,
		Status:        auth_model.AccountStatusPending,
		IsSuperAdmin:  true,
		InitializedAt: &now,
	}
	defaultLanguage := "zh-Hans"
	account.InterfaceLanguage = &defaultLanguage
	if params.IPAddress != "" {
		ipAddress := strings.TrimSpace(params.IPAddress)
		account.LastLoginIp = &ipAddress
	}

	if err := accountRepo.CreateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("create bootstrap admin account: %w", err)
	}

	return account, nil
}

// ValidatePassword validates password strength.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if !util.ContainsLetter(password) || !util.ContainsNumber(password) {
		return ErrPasswordTooSimple
	}
	return nil
}

// IsPasswordValidationError reports whether err comes from bootstrap password validation.
func IsPasswordValidationError(err error) bool {
	return errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooSimple)
}
