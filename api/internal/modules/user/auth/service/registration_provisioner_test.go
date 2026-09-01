package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	quota_repo "github.com/zgiai/zgi/api/internal/modules/quota/repository"
	quota_service "github.com/zgiai/zgi/api/internal/modules/quota/service"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
)

func TestRegistrationProvisionerCloudCreatesPersonalOwnerScope(t *testing.T) {
	db := newRegistrationProvisionerTestDB(t)
	organization := &registrationOrganizationRecorder{
		organization: &workspace_model.Organization{ID: "organization-personal"},
	}
	workspace := &registrationWorkspaceRecorder{
		workspace: &workspace_model.Workspace{ID: "workspace-personal"},
	}
	provisioner := newTestRegistrationProvisioner(registrationRunModeCloud, organization, workspace, nil)
	account := &auth_model.Account{ID: "account-cloud", Name: "Cloud User"}

	result, err := provisioner.Provision(t.Context(), db, account, nil)

	require.NoError(t, err)
	require.Equal(t, "organization-personal", result.OrganizationID)
	require.Equal(t, "workspace-personal", result.WorkspaceID)
	require.True(t, result.CreatedOrganization)
	require.Same(t, workspace.workspace, result.CreatedWorkspace)
	require.Equal(t, "Cloud User's Organization", organization.createdName)
	require.Equal(t, workspace_model.OrganizationRoleOwner, organization.role)
	require.Equal(t, "organization-personal", organization.linkedOrganizationID)
	require.Equal(t, "workspace-personal", organization.linkedWorkspaceID)
	require.Equal(t, "Cloud User's Workspace", workspace.createdName)
	require.Equal(t, workspace_model.WorkspaceRoleOwner, workspace.memberRole)
	require.Equal(t, "account-cloud", workspace.memberAccountID)
	require.Same(t, db, workspace.memberTx)

	assertRegistrationAccountContext(t, db, account.ID, result.OrganizationID, result.WorkspaceID)
}

func TestRegistrationProvisionerSelfHostedJoinsSetupScopeAsMember(t *testing.T) {
	db := newRegistrationProvisionerTestDB(t)
	organizationID := "organization-shared"
	workspaceID := "workspace-shared"
	require.NoError(t, db.Create(&workspace_model.Organization{
		ID:     organizationID,
		Name:   "Default Organization",
		Status: workspace_model.OrganizationStatusActive,
	}).Error)
	require.NoError(t, db.Create(&workspace_model.Workspace{
		ID:             workspaceID,
		Name:           "Default Workspace",
		Status:         workspace_model.WorkspaceStatusNormal,
		OrganizationID: &organizationID,
	}).Error)

	organization := &registrationOrganizationRecorder{}
	workspace := &registrationWorkspaceRecorder{}
	resolver := &registrationSetupScopeStub{
		organizationID: organizationID,
		workspaceID:    workspaceID,
	}
	provisioner := newTestRegistrationProvisioner(registrationRunModeSelfHosted, organization, workspace, resolver)
	account := &auth_model.Account{ID: "account-self-hosted", Name: "Shared User"}
	required := true

	result, err := provisioner.Provision(t.Context(), db, account, &required)

	require.NoError(t, err)
	require.Equal(t, organizationID, result.OrganizationID)
	require.Equal(t, workspaceID, result.WorkspaceID)
	require.False(t, result.CreatedOrganization)
	require.Nil(t, result.CreatedWorkspace)
	require.Empty(t, organization.createdName)
	require.Equal(t, workspace_model.OrganizationRoleNormal, organization.role)
	require.Equal(t, organizationID, organization.roleOrganizationID)
	require.Equal(t, workspace_model.WorkspaceRoleMember, workspace.memberRole)
	require.Equal(t, workspaceID, workspace.memberWorkspaceID)
	require.Equal(t, account.ID, workspace.memberAccountID)
	require.Same(t, db, workspace.memberTx)
	require.Same(t, db, resolver.tx)

	assertRegistrationAccountContext(t, db, account.ID, organizationID, workspaceID)
}

func TestRegistrationProvisionerExplicitFalseCreatesNoScope(t *testing.T) {
	required := false
	provisioner := &RegistrationProvisioner{
		organizationServiceForTx: func(*gorm.DB) registrationOrganizationService {
			panic("organization service must not be called")
		},
		workspaceServiceForTx: func(*gorm.DB) registrationWorkspaceService {
			panic("workspace service must not be called")
		},
		setupScopeResolver: &registrationSetupScopeStub{err: errors.New("resolver must not be called")},
		runMode:            func() string { panic("run mode must not be read") },
	}

	result, err := provisioner.Provision(t.Context(), nil, nil, &required)

	require.NoError(t, err)
	require.Equal(t, &RegistrationProvisioningResult{}, result)
}

func TestRegistrationProvisionerUsesCallerTransactionWithSingleConnection(t *testing.T) {
	tests := []struct {
		name    string
		runMode string
		role    workspace_model.WorkspaceMemberRole
	}{
		{name: "cloud creates personal scope", runMode: registrationRunModeCloud, role: workspace_model.WorkspaceRoleOwner},
		{name: "self-hosted joins setup scope", runMode: registrationRunModeSelfHosted, role: workspace_model.WorkspaceRoleMember},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newRegistrationTransactionTestDB(t)
			organizationService := workspace_service.NewOrganizationManagementService(db, nil)
			quotaService := quota_service.NewQuotaService(quota_repo.NewQuotaRepository(db), db)
			workspaceRepository := workspace_repo.NewWorkspaceRepository(db)
			baseOrganizationService := workspace_service.NewOrganizationService(
				workspace_repo.NewOrganizationRepository(db),
				nil,
				workspaceRepository,
				nil,
				nil,
				nil,
				nil,
				db,
				nil,
				nil,
			)
			workspaceService := workspace_service.NewWorkspaceManagementService(
				db,
				workspaceRepository,
				workspace_repo.NewWorkspaceMemberRepository(db),
				nil,
				quotaService,
				baseOrganizationService,
			)

			var resolver RegistrationSetupScopeResolver
			if tt.runMode == registrationRunModeSelfHosted {
				organizationID := uuid.NewString()
				workspaceID := uuid.NewString()
				require.NoError(t, db.Create(&workspace_model.Organization{
					ID:     organizationID,
					Name:   "Setup Organization",
					Status: workspace_model.OrganizationStatusActive,
				}).Error)
				require.NoError(t, db.Create(&workspace_model.Workspace{
					ID:             workspaceID,
					Name:           "Setup Workspace",
					Status:         workspace_model.WorkspaceStatusNormal,
					OrganizationID: &organizationID,
				}).Error)
				resolver = &registrationSetupScopeStub{organizationID: organizationID, workspaceID: workspaceID}
			}

			provisioner := NewRegistrationProvisioner(organizationService, workspaceService, resolver)
			provisioner.runMode = func() string { return tt.runMode }
			account := &auth_model.Account{ID: uuid.NewString(), Name: "Single Connection User", Email: uuid.NewString() + "@example.com"}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			var result *RegistrationProvisioningResult
			err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(account).Error; err != nil {
					return err
				}
				var provisionErr error
				result, provisionErr = provisioner.Provision(ctx, tx, account, nil)
				return provisionErr
			})

			require.NoError(t, err)
			require.NotNil(t, result)
			var member workspace_model.WorkspaceMember
			require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", result.WorkspaceID, account.ID).Take(&member).Error)
			require.Equal(t, tt.role, member.Role)
			require.True(t, member.Current, "registration membership must match the current account context")
			assertRegistrationAccountContext(t, db, account.ID, result.OrganizationID, result.WorkspaceID)
			var seatUsageCount int64
			require.NoError(t, db.Model(&quota_model.QuotaUsageHistory{}).
				Where("group_id = ? AND account_id = ? AND resource_type = ?", result.OrganizationID, account.ID, quota_model.ResourceTypeSeats).
				Count(&seatUsageCount).Error)
			require.EqualValues(t, 1, seatUsageCount)
		})
	}
}

func TestRegisterExRollsBackAccountWhenProvisioningFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&auth_model.Account{}, &registrationProvisioningAudit{}))

	service := &AccountService{
		accountRepo:             auth_repo.NewAccountRepository(db),
		registrationProvisioner: &failingRegistrationAccountProvisioner{},
	}
	required := true

	account, err := service.RegisterEx(
		t.Context(),
		"rollback@example.com",
		"Rollback User",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&required,
	)

	require.ErrorContains(t, err, "forced provisioning failure")
	require.Nil(t, account)
	var accountCount int64
	require.NoError(t, db.Model(&auth_model.Account{}).Count(&accountCount).Error)
	require.Zero(t, accountCount)
	var auditCount int64
	require.NoError(t, db.Model(&registrationProvisioningAudit{}).Count(&auditCount).Error)
	require.Zero(t, auditCount)
}

func TestRegisterExBootstrapsOfficialRouteOnlyForNewPersonalOrganization(t *testing.T) {
	tests := []struct {
		name                string
		createdOrganization bool
		wantBootstrapCalls  int
	}{
		{name: "cloud personal scope", createdOrganization: true, wantBootstrapCalls: 1},
		{name: "self-hosted shared scope", createdOrganization: false, wantBootstrapCalls: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&auth_model.Account{}))

			organizationID := uuid.NewString()
			bootstrapper := &registrationOfficialRouteBootstrapper{db: db}
			service := &AccountService{
				accountRepo: auth_repo.NewAccountRepository(db),
				registrationProvisioner: &staticRegistrationAccountProvisioner{result: &RegistrationProvisioningResult{
					OrganizationID:      organizationID,
					WorkspaceID:         uuid.NewString(),
					CreatedOrganization: tt.createdOrganization,
				}},
				officialRouteBootstrapper: bootstrapper,
			}
			required := true

			account, err := service.RegisterEx(
				t.Context(),
				fmt.Sprintf("%s@example.com", uuid.NewString()),
				"Registration User",
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				&required,
			)

			require.NoError(t, err)
			require.NotNil(t, account)
			require.Len(t, bootstrapper.organizationIDs, tt.wantBootstrapCalls)
			if tt.wantBootstrapCalls == 1 {
				require.Equal(t, organizationID, bootstrapper.organizationIDs[0].String())
				require.EqualValues(t, 1, bootstrapper.accountCountAtCall, "official route bootstrap must run after account commit")
			}
		})
	}
}

func newRegistrationProvisionerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.AccountContext{},
		&workspace_model.Organization{},
		&workspace_model.Workspace{},
	))
	return db
}

func newRegistrationTransactionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&workspace_model.Organization{},
		&workspace_model.OrganizationMember{},
		&workspace_model.Workspace{},
		&workspace_model.WorkspaceMember{},
		&quota_model.QuotaUsageHistory{},
	))
	require.NoError(t, db.Exec(`CREATE TABLE roles (
		id text PRIMARY KEY,
		group_id text NOT NULL,
		name text NOT NULL,
		name_i18n text NOT NULL DEFAULT '{}',
		name_customized boolean NOT NULL DEFAULT false,
		description text,
		description_i18n text NOT NULL DEFAULT '{}',
		description_customized boolean NOT NULL DEFAULT false,
		status text NOT NULL DEFAULT 'active',
		permissions text NOT NULL DEFAULT '[]',
		system_key text,
		template_origin text NOT NULL DEFAULT 'custom',
		created_by text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func newTestRegistrationProvisioner(
	runMode string,
	organization registrationOrganizationService,
	workspace registrationWorkspaceService,
	resolver RegistrationSetupScopeResolver,
) *RegistrationProvisioner {
	return &RegistrationProvisioner{
		organizationServiceForTx: func(*gorm.DB) registrationOrganizationService { return organization },
		workspaceServiceForTx:    func(*gorm.DB) registrationWorkspaceService { return workspace },
		setupScopeResolver:       resolver,
		runMode:                  func() string { return runMode },
	}
}

func assertRegistrationAccountContext(
	t *testing.T,
	db *gorm.DB,
	accountID, organizationID, workspaceID string,
) {
	t.Helper()
	var accountContext auth_model.AccountContext
	require.NoError(t, db.First(&accountContext, "account_id = ?", accountID).Error)
	require.NotNil(t, accountContext.CurrentOrganizationID)
	require.Equal(t, organizationID, *accountContext.CurrentOrganizationID)
	require.NotNil(t, accountContext.CurrentWorkspaceID)
	require.Equal(t, workspaceID, *accountContext.CurrentWorkspaceID)
}

type registrationOrganizationRecorder struct {
	organization         *workspace_model.Organization
	createdName          string
	roleOrganizationID   string
	roleAccountID        string
	role                 workspace_model.OrganizationRole
	linkedOrganizationID string
	linkedWorkspaceID    string
}

func (r *registrationOrganizationRecorder) CreateOrganization(_ context.Context, name string) (*workspace_model.Organization, error) {
	r.createdName = name
	return r.organization, nil
}

func (r *registrationOrganizationRecorder) CheckOrganizationNameExists(context.Context, string) (bool, error) {
	return false, nil
}

func (r *registrationOrganizationRecorder) UpsertOrganizationRole(
	_ context.Context,
	organizationID string,
	accountID string,
	role workspace_model.OrganizationRole,
) error {
	r.roleOrganizationID = organizationID
	r.roleAccountID = accountID
	r.role = role
	return nil
}

func (r *registrationOrganizationRecorder) AddWorkspace(_ context.Context, organizationID string, workspaceID string) error {
	r.linkedOrganizationID = organizationID
	r.linkedWorkspaceID = workspaceID
	return nil
}

type registrationWorkspaceRecorder struct {
	workspace         *workspace_model.Workspace
	createdName       string
	memberWorkspaceID string
	memberAccountID   string
	memberRole        workspace_model.WorkspaceMemberRole
	memberTx          *gorm.DB
}

func (r *registrationWorkspaceRecorder) CreateWorkspace(_ context.Context, name string, _ bool) (*workspace_model.Workspace, error) {
	r.createdName = name
	return r.workspace, nil
}

func (r *registrationWorkspaceRecorder) CreateWorkspaceMember(
	_ context.Context,
	workspaceID string,
	accountID string,
	role string,
) error {
	r.memberWorkspaceID = workspaceID
	r.memberAccountID = accountID
	r.memberRole = workspace_model.WorkspaceMemberRole(role)
	return nil
}

func (r *registrationWorkspaceRecorder) CreateRegistrationWorkspaceMember(
	ctx context.Context,
	tx *gorm.DB,
	workspaceID string,
	accountID string,
	role string,
) error {
	r.memberTx = tx
	return r.CreateWorkspaceMember(ctx, workspaceID, accountID, role)
}

func (r *registrationWorkspaceRecorder) SwitchWorkspace(context.Context, string, string) error {
	return nil
}

type registrationSetupScopeStub struct {
	organizationID string
	workspaceID    string
	err            error
	tx             *gorm.DB
}

func (r *registrationSetupScopeStub) ResolveDefaultScopeInTx(_ context.Context, tx *gorm.DB) (string, string, error) {
	r.tx = tx
	return r.organizationID, r.workspaceID, r.err
}

type registrationProvisioningAudit struct {
	ID string `gorm:"primaryKey"`
}

func (registrationProvisioningAudit) TableName() string {
	return "registration_provisioning_audits"
}

type failingRegistrationAccountProvisioner struct{}

func (*failingRegistrationAccountProvisioner) Provision(
	ctx context.Context,
	tx *gorm.DB,
	_ *auth_model.Account,
	_ *bool,
) (*RegistrationProvisioningResult, error) {
	if err := tx.WithContext(ctx).Create(&registrationProvisioningAudit{ID: "audit-1"}).Error; err != nil {
		return nil, err
	}
	return nil, errors.New("forced provisioning failure")
}

type staticRegistrationAccountProvisioner struct {
	result *RegistrationProvisioningResult
}

func (p *staticRegistrationAccountProvisioner) Provision(
	context.Context,
	*gorm.DB,
	*auth_model.Account,
	*bool,
) (*RegistrationProvisioningResult, error) {
	return p.result, nil
}

type registrationOfficialRouteBootstrapper struct {
	db                 *gorm.DB
	organizationIDs    []uuid.UUID
	accountCountAtCall int64
}

func (b *registrationOfficialRouteBootstrapper) InitOfficialChannel(_ context.Context, organizationID uuid.UUID) error {
	b.organizationIDs = append(b.organizationIDs, organizationID)
	return b.db.Model(&auth_model.Account{}).Count(&b.accountCountAtCall).Error
}
