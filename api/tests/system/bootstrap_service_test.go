package system_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/config"
	system_model "github.com/zgiai/ginext/internal/modules/system/model"
	system_repo "github.com/zgiai/ginext/internal/modules/system/repository"
	system_service "github.com/zgiai/ginext/internal/modules/system/service"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/ginext/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/ginext/internal/modules/workspace/repository"
	workspace_service "github.com/zgiai/ginext/internal/modules/workspace/service"
)

func TestBootstrapServiceBootstrapCreatesInitialAdminAndWorkspace(t *testing.T) {
	db := newBootstrapTestDB(t)
	service := newBootstrapServiceForTest(db)

	err := service.Bootstrap(context.Background(), system_service.BootstrapParams{
		AdminEmail:    "bootstrap@example.com",
		AdminName:     "Bootstrap Admin",
		AdminPassword: "secret1234",
		IPAddress:     "127.0.0.1",
		Source:        system_service.BootstrapSourceSelfHostedHTTP,
	})
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	account := loadBootstrapAccount(t, db, "bootstrap@example.com")
	assertSetupMarker(t, service)
	assertBootstrapAccount(t, account, "127.0.0.1")

	organization := loadDefaultOrganization(t, db)
	workspace := loadDefaultWorkspace(t, db)
	assertWorkspaceLinkedToOrganization(t, workspace, organization)
	assertOrganizationOwner(t, db, organization.ID, account.ID)
	assertWorkspaceOwner(t, db, workspace.ID, account.ID)
	assertAccountContext(t, db, account.ID, organization.ID, workspace.ID)
}

func TestBootstrapServiceBootstrapRejectsDuplicateSetup(t *testing.T) {
	db := newBootstrapTestDB(t)
	service := newBootstrapServiceForTest(db)
	params := system_service.BootstrapParams{
		AdminEmail:    "bootstrap@example.com",
		AdminName:     "Bootstrap Admin",
		AdminPassword: "secret1234",
		Source:        system_service.BootstrapSourceCloudEnv,
	}

	if err := service.Bootstrap(context.Background(), params); err != nil {
		t.Fatalf("first Bootstrap() error = %v, want nil", err)
	}

	err := service.Bootstrap(context.Background(), params)
	if !errors.Is(err, system_service.ErrAlreadySetup) {
		t.Fatalf("second Bootstrap() error = %v, want %v", err, system_service.ErrAlreadySetup)
	}
}

func TestCloudBootstrapRunnerUsesConfigAndSkipsOnceSetupExists(t *testing.T) {
	db := newBootstrapTestDB(t)
	service := newBootstrapServiceForTest(db)

	runner := system_service.NewCloudBootstrapRunner(newCloudConfig("cloud@example.com", "Cloud Admin", "secret1234"), service)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() error = %v, want nil", err)
	}

	var account auth_model.Account
	if err := db.Where("email = ?", "cloud@example.com").First(&account).Error; err != nil {
		t.Fatalf("load cloud bootstrap account error = %v, want nil", err)
	}

	skipRunner := system_service.NewCloudBootstrapRunner(newCloudConfig("", "", ""), service)
	if err := skipRunner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() after setup error = %v, want nil", err)
	}
}

func TestCloudBootstrapRunnerFailsWhenCloudConfigMissingAndSetupNotDone(t *testing.T) {
	db := newBootstrapTestDB(t)
	service := newBootstrapServiceForTest(db)

	runner := system_service.NewCloudBootstrapRunner(newCloudConfig("", "", ""), service)
	err := runner.Run(context.Background())
	if !errors.Is(err, system_service.ErrCloudBootstrapConfig) {
		t.Fatalf("runner.Run() error = %v, want %v", err, system_service.ErrCloudBootstrapConfig)
	}
}

func newBootstrapTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "bootstrap.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db error = %v, want nil", err)
	}

	err = db.AutoMigrate(
		&auth_model.Account{},
		&auth_model.AccountContext{},
		&workspace_model.Organization{},
		&workspace_model.OrganizationMember{},
		&workspace_model.Workspace{},
		&workspace_model.WorkspaceMember{},
		&system_model.Setup{},
		&system_model.BootstrapLock{},
	)
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v, want nil", err)
	}

	return db
}

func newBootstrapServiceForTest(db *gorm.DB) *system_service.BootstrapService {
	workspaceRepository := workspace_repo.NewWorkspaceRepository(db)
	workspaceMemberRepository := workspace_repo.NewWorkspaceMemberRepository(db)
	tenantService := workspace_service.NewWorkspaceManagementService(
		db,
		workspaceRepository,
		workspaceMemberRepository,
		nil,
		nil,
		nil,
	)

	return system_service.NewBootstrapService(
		system_repo.NewSetupRepository(db),
		system_repo.NewBootstrapLockRepository(db),
		auth_repo.NewAccountRepository(db),
		db,
		tenantService,
		workspace_service.NewOrganizationManagementService(db, nil),
		nil,
	)
}

func newCloudConfig(email, name, password string) *config.Config {
	return &config.Config{
		Platform: config.PlatformConfig{
			Edition: "CLOUD",
			CloudBootstrap: config.CloudBootstrapConfig{
				AdminEmail:    email,
				AdminName:     name,
				AdminPassword: password,
			},
		},
	}
}

func assertSetupMarker(t *testing.T, service *system_service.BootstrapService) {
	t.Helper()

	setupStatus, err := service.GetSetupStatus()
	if err != nil {
		t.Fatalf("GetSetupStatus() error = %v, want nil", err)
	}
	if setupStatus == nil {
		t.Fatal("GetSetupStatus() = nil, want non-nil setup marker")
	}
}

func loadBootstrapAccount(t *testing.T, db *gorm.DB, email string) auth_model.Account {
	t.Helper()

	var account auth_model.Account
	if err := db.Where("email = ?", email).First(&account).Error; err != nil {
		t.Fatalf("load bootstrap account error = %v, want nil", err)
	}

	return account
}

func assertBootstrapAccount(t *testing.T, account auth_model.Account, expectedIP string) {
	t.Helper()

	if !account.IsSuperAdmin {
		t.Fatal("account.IsSuperAdmin = false, want true")
	}
	if account.InitializedAt == nil {
		t.Fatal("account.InitializedAt = nil, want non-nil")
	}
	if account.LastLoginIp == nil || *account.LastLoginIp != expectedIP {
		t.Fatalf("account.LastLoginIp = %v, want %s", account.LastLoginIp, expectedIP)
	}
}

func loadDefaultOrganization(t *testing.T, db *gorm.DB) workspace_model.Organization {
	t.Helper()

	var organization workspace_model.Organization
	if err := db.Where("name = ?", "Default Group").First(&organization).Error; err != nil {
		t.Fatalf("load default organization error = %v, want nil", err)
	}

	return organization
}

func loadDefaultWorkspace(t *testing.T, db *gorm.DB) workspace_model.Workspace {
	t.Helper()

	var workspace workspace_model.Workspace
	if err := db.Where("name = ?", "Default Workspace").First(&workspace).Error; err != nil {
		t.Fatalf("load default workspace error = %v, want nil", err)
	}

	return workspace
}

func assertWorkspaceLinkedToOrganization(t *testing.T, workspace workspace_model.Workspace, organization workspace_model.Organization) {
	t.Helper()

	if workspace.OrganizationID == nil || *workspace.OrganizationID != organization.ID {
		t.Fatalf("workspace.OrganizationID = %v, want %s", workspace.OrganizationID, organization.ID)
	}
}

func assertOrganizationOwner(t *testing.T, db *gorm.DB, organizationID, accountID string) {
	t.Helper()

	var organizationMember workspace_model.OrganizationMember
	if err := db.Where("organization_id = ? AND account_id = ?", organizationID, accountID).First(&organizationMember).Error; err != nil {
		t.Fatalf("load organization member error = %v, want nil", err)
	}
	if organizationMember.Role != workspace_model.OrganizationRoleOwner {
		t.Fatalf("organization member role = %s, want %s", organizationMember.Role, workspace_model.OrganizationRoleOwner)
	}
}

func assertWorkspaceOwner(t *testing.T, db *gorm.DB, workspaceID, accountID string) {
	t.Helper()

	var workspaceMember workspace_model.WorkspaceMember
	if err := db.Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).First(&workspaceMember).Error; err != nil {
		t.Fatalf("load workspace member error = %v, want nil", err)
	}
	if workspaceMember.Role != workspace_model.WorkspaceRoleOwner {
		t.Fatalf("workspace member role = %s, want %s", workspaceMember.Role, workspace_model.WorkspaceRoleOwner)
	}
}

func assertAccountContext(t *testing.T, db *gorm.DB, accountID, organizationID, workspaceID string) {
	t.Helper()

	var accountContext auth_model.AccountContext
	if err := db.Where("account_id = ?", accountID).First(&accountContext).Error; err != nil {
		t.Fatalf("load account context error = %v, want nil", err)
	}
	if accountContext.CurrentOrganizationID == nil || *accountContext.CurrentOrganizationID != organizationID {
		t.Fatalf("account context organization = %v, want %s", accountContext.CurrentOrganizationID, organizationID)
	}
	if accountContext.CurrentWorkspaceID == nil || *accountContext.CurrentWorkspaceID != workspaceID {
		t.Fatalf("account context workspace = %v, want %s", accountContext.CurrentWorkspaceID, workspaceID)
	}
}
