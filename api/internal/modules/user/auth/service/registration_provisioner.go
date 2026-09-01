package service

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	registrationRunModeCloud      = "CLOUD"
	registrationRunModeSelfHosted = "SELF_HOSTED"
)

// RegistrationSetupScopeResolver exposes only the setup state needed to place
// self-hosted registrations into the shared default organization and workspace.
type RegistrationSetupScopeResolver interface {
	ResolveDefaultScopeInTx(ctx context.Context, tx *gorm.DB) (organizationID, workspaceID string, err error)
}

// RegistrationProvisioningResult describes the account scope established by a
// registration. CreatedWorkspace is set only for a new personal cloud scope.
type RegistrationProvisioningResult struct {
	OrganizationID      string
	WorkspaceID         string
	CreatedOrganization bool
	RequiresCloudOutbox bool
	CreatedWorkspace    *workspace_model.Workspace
}

type registrationAccountProvisioner interface {
	Provision(
		ctx context.Context,
		tx *gorm.DB,
		account *auth_model.Account,
		createWorkspaceRequired *bool,
	) (*RegistrationProvisioningResult, error)
}

type registrationOrganizationService interface {
	CreateOrganization(ctx context.Context, name string) (*workspace_model.Organization, error)
	CheckOrganizationNameExists(ctx context.Context, name string) (bool, error)
	UpsertOrganizationRole(ctx context.Context, organizationID string, accountID string, role workspace_model.OrganizationRole) error
	AddWorkspace(ctx context.Context, organizationID string, workspaceID string) error
}

type registrationWorkspaceService interface {
	CreateWorkspace(ctx context.Context, name string, isFromDashboard bool) (*workspace_model.Workspace, error)
}

type registrationWorkspaceMemberTxCreator interface {
	CreateRegistrationWorkspaceMember(
		ctx context.Context,
		tx *gorm.DB,
		workspaceID string,
		accountID string,
		role string,
	) error
}

// RegistrationProvisioner owns the run-mode-specific organization/workspace
// affiliation created during account registration.
type RegistrationProvisioner struct {
	organizationServiceForTx func(tx *gorm.DB) registrationOrganizationService
	workspaceServiceForTx    func(tx *gorm.DB) registrationWorkspaceService
	setupScopeResolver       RegistrationSetupScopeResolver
	runMode                  func() string
}

func NewRegistrationProvisioner(
	organizationService interfaces.OrganizationManagementService,
	workspaceService interfaces.WorkspaceManagementService,
	setupScopeResolver RegistrationSetupScopeResolver,
) *RegistrationProvisioner {
	return &RegistrationProvisioner{
		organizationServiceForTx: func(tx *gorm.DB) registrationOrganizationService {
			if organizationService == nil {
				return nil
			}
			return organizationService.WithTx(tx)
		},
		workspaceServiceForTx: func(tx *gorm.DB) registrationWorkspaceService {
			if workspaceService == nil {
				return nil
			}
			return workspaceService.WithTx(tx)
		},
		setupScopeResolver: setupScopeResolver,
		runMode: func() string {
			return config.Current().Platform.Edition
		},
	}
}

func (p *RegistrationProvisioner) Provision(
	ctx context.Context,
	tx *gorm.DB,
	account *auth_model.Account,
	createWorkspaceRequired *bool,
) (*RegistrationProvisioningResult, error) {
	if createWorkspaceRequired != nil && !*createWorkspaceRequired {
		return &RegistrationProvisioningResult{}, nil
	}
	if tx == nil {
		return nil, fmt.Errorf("registration provisioning transaction is required")
	}
	if account == nil || strings.TrimSpace(account.ID) == "" {
		return nil, fmt.Errorf("registration provisioning account is required")
	}

	runMode := registrationRunModeSelfHosted
	if p != nil && p.runMode != nil {
		runMode = normalizeRegistrationRunMode(p.runMode())
	}

	switch runMode {
	case registrationRunModeCloud:
		return p.provisionPersonalScope(ctx, tx, account)
	case registrationRunModeSelfHosted:
		return p.provisionSharedSetupScope(ctx, tx, account)
	default:
		return nil, fmt.Errorf("unsupported registration run mode %q", runMode)
	}
}

func normalizeRegistrationRunMode(mode string) string {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	return strings.ReplaceAll(mode, "-", "_")
}

func (p *RegistrationProvisioner) provisionPersonalScope(
	ctx context.Context,
	tx *gorm.DB,
	account *auth_model.Account,
) (*RegistrationProvisioningResult, error) {
	organizationService, workspaceService, err := p.transactionServices(tx)
	if err != nil {
		return nil, err
	}

	organizationName, err := uniqueOwnedOrganizationName(ctx, organizationService, account.Name, account.InterfaceLanguage)
	if err != nil {
		return nil, fmt.Errorf("prepare personal organization name: %w", err)
	}
	organization, err := organizationService.CreateOrganization(ctx, organizationName)
	if err != nil {
		return nil, fmt.Errorf("create personal organization: %w", err)
	}
	if err := organizationService.UpsertOrganizationRole(
		ctx,
		organization.ID,
		account.ID,
		workspace_model.OrganizationRoleOwner,
	); err != nil {
		return nil, fmt.Errorf("create personal organization owner: %w", err)
	}

	workspace, err := workspaceService.CreateWorkspace(ctx, fmt.Sprintf("%s's Workspace", account.Name), true)
	if err != nil {
		return nil, fmt.Errorf("create personal workspace: %w", err)
	}
	if err := organizationService.AddWorkspace(ctx, organization.ID, workspace.ID); err != nil {
		return nil, fmt.Errorf("link personal workspace: %w", err)
	}
	if err := createRegistrationWorkspaceMember(
		ctx,
		tx,
		workspaceService,
		workspace.ID,
		account.ID,
		string(workspace_model.WorkspaceRoleOwner),
	); err != nil {
		return nil, fmt.Errorf("create personal workspace owner: %w", err)
	}
	if err := createRegistrationAccountContext(ctx, tx, account.ID, organization.ID, workspace.ID); err != nil {
		return nil, err
	}

	return &RegistrationProvisioningResult{
		OrganizationID:      organization.ID,
		WorkspaceID:         workspace.ID,
		CreatedOrganization: true,
		RequiresCloudOutbox: true,
		CreatedWorkspace:    workspace,
	}, nil
}

func (p *RegistrationProvisioner) provisionSharedSetupScope(
	ctx context.Context,
	tx *gorm.DB,
	account *auth_model.Account,
) (*RegistrationProvisioningResult, error) {
	if p == nil || p.setupScopeResolver == nil {
		return nil, fmt.Errorf("self-hosted registration setup scope resolver is not configured")
	}
	organizationID, workspaceID, err := p.setupScopeResolver.ResolveDefaultScopeInTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("resolve self-hosted registration scope: %w", err)
	}
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if organizationID == "" || workspaceID == "" {
		return nil, fmt.Errorf("self-hosted registration setup scope is incomplete")
	}
	if err := validateSharedRegistrationScope(ctx, tx, organizationID, workspaceID); err != nil {
		return nil, err
	}

	organizationService, workspaceService, err := p.transactionServices(tx)
	if err != nil {
		return nil, err
	}
	if err := organizationService.UpsertOrganizationRole(
		ctx,
		organizationID,
		account.ID,
		workspace_model.OrganizationRoleNormal,
	); err != nil {
		return nil, fmt.Errorf("join self-hosted default organization: %w", err)
	}
	if err := createRegistrationWorkspaceMember(
		ctx,
		tx,
		workspaceService,
		workspaceID,
		account.ID,
		string(workspace_model.WorkspaceRoleMember),
	); err != nil {
		return nil, fmt.Errorf("join self-hosted default workspace: %w", err)
	}
	if err := createRegistrationAccountContext(ctx, tx, account.ID, organizationID, workspaceID); err != nil {
		return nil, err
	}

	return &RegistrationProvisioningResult{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
	}, nil
}

func createRegistrationWorkspaceMember(
	ctx context.Context,
	tx *gorm.DB,
	workspaceService registrationWorkspaceService,
	workspaceID string,
	accountID string,
	role string,
) error {
	if txCreator, ok := workspaceService.(registrationWorkspaceMemberTxCreator); ok {
		return txCreator.CreateRegistrationWorkspaceMember(ctx, tx, workspaceID, accountID, role)
	}
	return fmt.Errorf("transaction-aware registration workspace member service is unavailable")
}

func (p *RegistrationProvisioner) transactionServices(
	tx *gorm.DB,
) (registrationOrganizationService, registrationWorkspaceService, error) {
	if p == nil || p.organizationServiceForTx == nil || p.workspaceServiceForTx == nil {
		return nil, nil, fmt.Errorf("registration provisioning services are not configured")
	}
	organizationService := p.organizationServiceForTx(tx)
	workspaceService := p.workspaceServiceForTx(tx)
	if organizationService == nil || workspaceService == nil {
		return nil, nil, fmt.Errorf("registration provisioning services are not configured")
	}
	return organizationService, workspaceService, nil
}

func validateSharedRegistrationScope(ctx context.Context, tx *gorm.DB, organizationID, workspaceID string) error {
	var count int64
	if err := tx.WithContext(ctx).
		Table("workspaces AS workspace").
		Joins("JOIN organizations AS organization ON organization.id = workspace.organization_id").
		Where("workspace.id = ? AND workspace.organization_id = ?", workspaceID, organizationID).
		Where("workspace.status = ?", workspace_model.WorkspaceStatusNormal).
		Where("organization.status = ?", workspace_model.OrganizationStatusActive).
		Count(&count).Error; err != nil {
		return fmt.Errorf("validate self-hosted registration scope: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("self-hosted registration setup scope is unavailable")
	}
	return nil
}

func createRegistrationAccountContext(
	ctx context.Context,
	tx *gorm.DB,
	accountID, organizationID, workspaceID string,
) error {
	accountContext := &auth_model.AccountContext{
		AccountID:             accountID,
		CurrentOrganizationID: &organizationID,
		CurrentWorkspaceID:    &workspaceID,
	}
	if err := tx.WithContext(ctx).Create(accountContext).Error; err != nil {
		return fmt.Errorf("create registration account context: %w", err)
	}
	return nil
}
