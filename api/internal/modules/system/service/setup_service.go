package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/internal/modules/system/repository"
)

const (
	defaultOrganizationNameZHHans = "默认组织"
	defaultOrganizationNameEnUS   = "Default Organization"
	defaultWorkspaceName          = "Default Workspace"
	bootstrapLockKey              = "cloud_setup"
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
	Language      string
	IPAddress     string
	Source        BootstrapSource
}

// RegistrationProvisioningOutboxEnqueuer records cloud bootstrap provisioning
// in the bootstrap transaction without coupling the system module to auth.
type RegistrationProvisioningOutboxEnqueuer func(ctx context.Context, tx *gorm.DB, accountID, organizationID string) error

// BootstrapService owns first-time business initialization.
type BootstrapService struct {
	repo                                   repository.SetupRepository
	lockRepo                               *repository.BootstrapLockRepository
	accountRepo                            auth_repo.AccountRepository
	db                                     *gorm.DB
	tenantSvc                              interfaces.WorkspaceManagementService
	groupSvc                               interfaces.OrganizationManagementService
	systemConfigSvc                        SystemConfigService
	registrationProvisioningOutboxEnqueuer RegistrationProvisioningOutboxEnqueuer
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
	registrationProvisioningOutboxEnqueuer RegistrationProvisioningOutboxEnqueuer,
) *BootstrapService {
	return &BootstrapService{
		repo:                                   repo,
		lockRepo:                               lockRepo,
		accountRepo:                            accountRepo,
		db:                                     db,
		tenantSvc:                              tenantSvc,
		groupSvc:                               groupSvc,
		systemConfigSvc:                        systemConfigSvc,
		registrationProvisioningOutboxEnqueuer: registrationProvisioningOutboxEnqueuer,
	}
}

// GetSetupStatus returns the persisted setup marker when bootstrap already finished.
func (s *BootstrapService) GetSetupStatus() (*model.Setup, error) {
	return s.repo.GetSetupStatus()
}

// ResolveDefaultScope returns the default organization and workspace recorded by setup.
// Legacy setup markers are resolved and backfilled before the scope is returned.
func (s *BootstrapService) ResolveDefaultScope(ctx context.Context) (string, string, error) {
	if s.db == nil {
		return "", "", fmt.Errorf("resolve setup scope: database is required")
	}

	var setupStatus *model.Setup
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resolveErr error
		setupStatus, resolveErr = s.resolveAndBackfillSetupScope(ctx, tx)
		return resolveErr
	})
	if err != nil {
		return "", "", err
	}
	return resolvedSetupScopeIDs(setupStatus)
}

// ResolveDefaultScopeInTx resolves the setup scope using the caller's
// transaction. Registration uses this path so it does not acquire a second
// database connection while its account transaction is still open.
func (s *BootstrapService) ResolveDefaultScopeInTx(ctx context.Context, tx *gorm.DB) (string, string, error) {
	if tx == nil {
		return "", "", fmt.Errorf("resolve setup scope: transaction is required")
	}

	setupStatus, err := s.resolveAndBackfillSetupScope(ctx, tx)
	if err != nil {
		return "", "", err
	}
	return resolvedSetupScopeIDs(setupStatus)
}

func resolvedSetupScopeIDs(setupStatus *model.Setup) (string, string, error) {
	if setupStatus == nil {
		return "", "", ErrSetupScopeUnavailable
	}

	organizationID := normalizedStringPointer(setupStatus.OrganizationID)
	workspaceID := normalizedStringPointer(setupStatus.WorkspaceID)
	if organizationID == "" || workspaceID == "" {
		return "", "", ErrSetupScopeUnavailable
	}
	return organizationID, workspaceID, nil
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
func (s *BootstrapService) Setup(ctx context.Context, email, name, password, language, ipAddress string) error {
	return s.Bootstrap(ctx, BootstrapParams{
		AdminEmail:    email,
		AdminName:     name,
		AdminPassword: password,
		Language:      language,
		IPAddress:     ipAddress,
		Source:        BootstrapSourceSelfHostedHTTP,
	})
}

// Bootstrap performs first-time business initialization inside one transaction.
func (s *BootstrapService) Bootstrap(ctx context.Context, params BootstrapParams) error {
	if err := ValidatePassword(params.AdminPassword); err != nil {
		return err
	}
	if params.Source == BootstrapSourceCloudEnv && s.registrationProvisioningOutboxEnqueuer == nil {
		return fmt.Errorf("cloud bootstrap requires registration provisioning outbox")
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

		organization, err := txGroupSvc.CreateOrganization(ctx, defaultOrganizationNameForLanguage(params.Language))
		if err != nil {
			return fmt.Errorf("create default organization: %w", err)
		}
		if params.Source == BootstrapSourceCloudEnv {
			if err := s.registrationProvisioningOutboxEnqueuer(ctx, tx, account.ID, organization.ID); err != nil {
				return fmt.Errorf("enqueue cloud bootstrap provisioning: %w", err)
			}
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

		if err := txSetupRepo.CreateSetup(organization.ID, workspace.ID); err != nil {
			return fmt.Errorf("create setup marker: %w", err)
		}

		return nil
	})
}

type setupScope struct {
	OrganizationID string `gorm:"column:organization_id"`
	WorkspaceID    string `gorm:"column:workspace_id"`
}

func (s *BootstrapService) resolveAndBackfillSetupScope(ctx context.Context, tx *gorm.DB) (*model.Setup, error) {
	txSetupRepo := repository.NewSetupRepository(tx.WithContext(ctx))
	setupStatus, err := txSetupRepo.GetSetupStatus()
	if err != nil {
		return nil, fmt.Errorf("get setup status for scope resolution: %w", err)
	}
	if setupStatus == nil {
		return nil, nil
	}

	// Complete setup markers are immutable and only need validation. Legacy
	// markers require a row lock so only one transaction may choose and persist
	// their default scope.
	if normalizedStringPointer(setupStatus.OrganizationID) == "" ||
		normalizedStringPointer(setupStatus.WorkspaceID) == "" {
		version := setupStatus.Version
		setupStatus, err = txSetupRepo.GetSetupStatusForUpdate()
		if err != nil {
			return nil, fmt.Errorf("lock setup status for scope resolution: %w", err)
		}
		if setupStatus == nil {
			return nil, nil
		}
		if setupStatus.Version != version {
			return nil, fmt.Errorf("setup marker changed during scope resolution")
		}
	}

	scope, needsBackfill, err := resolveSetupScope(ctx, tx, setupStatus)
	if err != nil {
		return nil, err
	}
	if needsBackfill {
		if err := txSetupRepo.UpdateSetupScope(setupStatus.Version, scope.OrganizationID, scope.WorkspaceID); err != nil {
			return nil, fmt.Errorf("backfill setup scope: %w", err)
		}
	}

	setupStatus.OrganizationID = stringPointer(scope.OrganizationID)
	setupStatus.WorkspaceID = stringPointer(scope.WorkspaceID)
	return setupStatus, nil
}

func resolveSetupScope(ctx context.Context, tx *gorm.DB, setupStatus *model.Setup) (setupScope, bool, error) {
	if setupStatus == nil {
		return setupScope{}, false, ErrSetupScopeUnavailable
	}

	organizationID := normalizedStringPointer(setupStatus.OrganizationID)
	workspaceID := normalizedStringPointer(setupStatus.WorkspaceID)
	if organizationID != "" && workspaceID != "" {
		scope := setupScope{OrganizationID: organizationID, WorkspaceID: workspaceID}
		if err := validateSetupScope(ctx, tx, scope); err != nil {
			return setupScope{}, false, err
		}
		return scope, false, nil
	}

	constraints := setupScope{OrganizationID: organizationID, WorkspaceID: workspaceID}
	// A legacy marker's missing IDs must not be inferred from account_contexts or
	// from the currently active tenant set: both can change long after setup. The
	// setup timestamp is immutable and bootstrap refused pre-existing workspaces,
	// so only a unique scope created no later than setup is safe to backfill.
	scope, found, err := resolveSetupEraScope(ctx, tx, setupStatus.SetupAt, constraints)
	if err != nil {
		return setupScope{}, false, err
	}
	if !found {
		return setupScope{}, false, ErrSetupScopeUnavailable
	}
	if err := validateSetupScope(ctx, tx, scope); err != nil {
		return setupScope{}, false, err
	}
	return scope, true, nil
}

func resolveSetupEraScope(ctx context.Context, tx *gorm.DB, setupAt time.Time, constraints setupScope) (setupScope, bool, error) {
	if setupAt.IsZero() {
		return setupScope{}, false, nil
	}

	query := tx.WithContext(ctx).
		Table("workspaces AS w").
		Distinct("w.organization_id AS organization_id, w.id AS workspace_id").
		Joins("JOIN organizations AS o ON o.id = w.organization_id").
		Where("o.created_at <= ?", setupAt).
		Where("w.created_at <= ?", setupAt).
		Where("w.organization_id IS NOT NULL")
	query = constrainSetupScopeQuery(query, "w.organization_id", "w.id", constraints)

	scopes, err := loadSetupScopeCandidates(query)
	if err != nil {
		return setupScope{}, false, fmt.Errorf("resolve setup-era scope: %w", err)
	}
	switch len(scopes) {
	case 0:
		return setupScope{}, false, nil
	case 1:
		return scopes[0], true, nil
	default:
		return setupScope{}, false, fmt.Errorf("%w: multiple organization/workspace pairs exist", ErrSetupScopeAmbiguous)
	}
}

func constrainSetupScopeQuery(query *gorm.DB, organizationColumn, workspaceColumn string, constraints setupScope) *gorm.DB {
	if constraints.OrganizationID != "" {
		query = query.Where(organizationColumn+" = ?", constraints.OrganizationID)
	}
	if constraints.WorkspaceID != "" {
		query = query.Where(workspaceColumn+" = ?", constraints.WorkspaceID)
	}
	return query
}

func loadSetupScopeCandidates(query *gorm.DB) ([]setupScope, error) {
	var scopes []setupScope
	err := query.
		Order("organization_id ASC, workspace_id ASC").
		Limit(2).
		Scan(&scopes).Error
	return scopes, err
}

func validateSetupScope(ctx context.Context, tx *gorm.DB, scope setupScope) error {
	var count int64
	err := tx.WithContext(ctx).
		Table("workspaces AS w").
		Joins("JOIN organizations AS o ON o.id = w.organization_id").
		Where("w.id = ? AND w.organization_id = ?", scope.WorkspaceID, scope.OrganizationID).
		Where("o.status = ?", workspace_model.OrganizationStatusActive).
		Where("w.status = ?", workspace_model.WorkspaceStatusNormal).
		Count(&count).Error
	if err != nil {
		return fmt.Errorf("validate setup scope: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("%w: organization %q and workspace %q are not an active setup scope", ErrSetupScopeInvalid, scope.OrganizationID, scope.WorkspaceID)
	}
	return nil
}

func normalizedStringPointer(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
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
	defaultLanguage := normalizeBootstrapLanguage(params.Language)
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

func defaultOrganizationNameForLanguage(language string) string {
	if isChineseBootstrapLanguage(language) {
		return defaultOrganizationNameZHHans
	}
	return defaultOrganizationNameEnUS
}

func normalizeBootstrapLanguage(language string) string {
	if isChineseBootstrapLanguage(language) {
		return "zh-Hans"
	}
	return "en-US"
}

func isChineseBootstrapLanguage(language string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh")
}

// IsPasswordValidationError reports whether err comes from bootstrap password validation.
func IsPasswordValidationError(err error) bool {
	return errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooSimple)
}
