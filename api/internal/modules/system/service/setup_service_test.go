package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	systemmodel "github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/internal/modules/system/repository"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	authrepo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newSetupScopeTestService(t *testing.T) (*gorm.DB, *BootstrapService) {
	t.Helper()

	dsn := fmt.Sprintf("file:setup-scope-%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open setup scope test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get setup scope test sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	statements := []string{
		`CREATE TABLE zgi_setups (
			version TEXT PRIMARY KEY,
			setup_at DATETIME NOT NULL,
			organization_id TEXT NULL,
			workspace_id TEXT NULL
		)`,
		`CREATE TABLE accounts (
			id TEXT PRIMARY KEY,
			is_super_admin BOOLEAN NOT NULL DEFAULT FALSE,
			deleted_at DATETIME NULL
		)`,
		`CREATE TABLE account_contexts (
			account_id TEXT PRIMARY KEY,
			current_organization_id TEXT NULL,
			current_workspace_id TEXT NULL
		)`,
		`CREATE TABLE organizations (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			organization_id TEXT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create setup scope test schema: %v", err)
		}
	}

	return db, &BootstrapService{
		repo: repository.NewSetupRepository(db),
		db:   db,
	}
}

func seedLegacySetup(t *testing.T, db *gorm.DB, organizationID, workspaceID any) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO zgi_setups (version, setup_at, organization_id, workspace_id) VALUES (?, ?, ?, ?)`,
		"1.0",
		time.Now().UTC(),
		organizationID,
		workspaceID,
	).Error; err != nil {
		t.Fatalf("seed setup marker: %v", err)
	}
}

func seedUsableSetupScope(t *testing.T, db *gorm.DB, organizationID, workspaceID string) {
	t.Helper()
	seedSetupScopeAt(
		t,
		db,
		organizationID,
		workspaceID,
		"active",
		"normal",
		time.Now().UTC().Add(-time.Hour),
	)
}

func seedSetupScopeAt(
	t *testing.T,
	db *gorm.DB,
	organizationID, workspaceID, organizationStatus, workspaceStatus string,
	createdAt time.Time,
) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO organizations (id, status, created_at) VALUES (?, ?, ?)`,
		organizationID,
		organizationStatus,
		createdAt,
	).Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO workspaces (id, organization_id, status, created_at) VALUES (?, ?, ?, ?)`,
		workspaceID,
		organizationID,
		workspaceStatus,
		createdAt,
	).Error; err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
}

func seedSuperAdminContext(t *testing.T, db *gorm.DB, accountID, organizationID, workspaceID string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO accounts (id, is_super_admin) VALUES (?, TRUE)`,
		accountID,
	).Error; err != nil {
		t.Fatalf("seed super administrator: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO account_contexts (account_id, current_organization_id, current_workspace_id) VALUES (?, ?, ?)`,
		accountID,
		organizationID,
		workspaceID,
	).Error; err != nil {
		t.Fatalf("seed super administrator context: %v", err)
	}
}

func readPersistedSetupScope(t *testing.T, db *gorm.DB) (sql.NullString, sql.NullString) {
	t.Helper()
	var row struct {
		OrganizationID sql.NullString `gorm:"column:organization_id"`
		WorkspaceID    sql.NullString `gorm:"column:workspace_id"`
	}
	if err := db.Table("zgi_setups").
		Select("organization_id, workspace_id").
		Where("version = ?", "1.0").
		Take(&row).Error; err != nil {
		t.Fatalf("read persisted setup scope: %v", err)
	}
	return row.OrganizationID, row.WorkspaceID
}

func TestResolveDefaultScope_IgnoresMutableSuperAdminContextWhenScopeIsAmbiguous(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	seedUsableSetupScope(t, db, "org-2", "workspace-2")
	seedSuperAdminContext(t, db, "admin-1", "org-2", "workspace-2")

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeAmbiguous) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeAmbiguous", err)
	}

	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if persistedOrganizationID.Valid || persistedWorkspaceID.Valid {
		t.Fatalf("mutable administrator context changed setup scope: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestGetSetupStatus_RemainsReadOnlyWhileResolverBackfillsUniqueScope(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")

	setup, err := service.GetSetupStatus()
	if err != nil {
		t.Fatalf("GetSetupStatus() error = %v, want nil", err)
	}
	if setup == nil || setup.OrganizationID != nil || setup.WorkspaceID != nil {
		t.Fatalf("GetSetupStatus() = %+v, want unchanged legacy marker", setup)
	}

	organizationID, workspaceID, err := service.ResolveDefaultScope(context.Background())
	if err != nil {
		t.Fatalf("ResolveDefaultScope() error = %v, want nil", err)
	}
	if organizationID != "org-1" || workspaceID != "workspace-1" {
		t.Fatalf("ResolveDefaultScope() = %q, %q, want unique scope", organizationID, workspaceID)
	}

	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if !persistedOrganizationID.Valid || !persistedWorkspaceID.Valid {
		t.Fatalf("resolver did not backfill setup scope: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScopeInTx_ReusesCallerTransactionWithSingleConnection(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		organizationID, workspaceID, err := service.ResolveDefaultScopeInTx(ctx, tx)
		if err != nil {
			return err
		}
		if organizationID != "org-1" || workspaceID != "workspace-1" {
			return fmt.Errorf("resolved scope = %q, %q, want org-1, workspace-1", organizationID, workspaceID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ResolveDefaultScopeInTx() error = %v, want nil", err)
	}

	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if !persistedOrganizationID.Valid || persistedOrganizationID.String != "org-1" ||
		!persistedWorkspaceID.Valid || persistedWorkspaceID.String != "workspace-1" {
		t.Fatalf("persisted scope = %+v, %+v, want caller transaction backfill", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScopeInTx_BackfillRollsBackWithCallerTransaction(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	wantErr := errors.New("rollback registration")

	err := db.Transaction(func(tx *gorm.DB) error {
		if _, _, err := service.ResolveDefaultScopeInTx(context.Background(), tx); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("transaction error = %v, want %v", err, wantErr)
	}

	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if persistedOrganizationID.Valid || persistedWorkspaceID.Valid {
		t.Fatalf("rolled back transaction persisted scope: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScope_UsesPersistedConstraintForPartialLegacyScope(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, "org-1", nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	seedUsableSetupScope(t, db, "org-2", "workspace-2")

	organizationID, workspaceID, err := service.ResolveDefaultScope(context.Background())
	if err != nil {
		t.Fatalf("ResolveDefaultScope() error = %v, want nil", err)
	}
	if organizationID != "org-1" || workspaceID != "workspace-1" {
		t.Fatalf("ResolveDefaultScope() = %q, %q, want constrained legacy scope", organizationID, workspaceID)
	}
}

func TestResolveDefaultScope_RejectsAmbiguousSuperAdminContexts(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	seedUsableSetupScope(t, db, "org-2", "workspace-2")
	seedSuperAdminContext(t, db, "admin-1", "org-1", "workspace-1")
	seedSuperAdminContext(t, db, "admin-2", "org-2", "workspace-2")

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeAmbiguous) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeAmbiguous", err)
	}
	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if persistedOrganizationID.Valid || persistedWorkspaceID.Valid {
		t.Fatalf("ambiguous resolution changed setup scope: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScope_RejectsAmbiguousSetupEraScopes(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	seedUsableSetupScope(t, db, "org-2", "workspace-2")

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeAmbiguous) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeAmbiguous", err)
	}
	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if persistedOrganizationID.Valid || persistedWorkspaceID.Valid {
		t.Fatalf("ambiguous fallback changed setup scope: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScope_DoesNotDriftFromArchivedSetupScopeToLaterActiveScope(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedSetupScopeAt(
		t,
		db,
		"org-setup",
		"workspace-setup",
		"active",
		"archived",
		time.Now().UTC().Add(-time.Hour),
	)
	seedSetupScopeAt(
		t,
		db,
		"org-later",
		"workspace-later",
		"active",
		"normal",
		time.Now().UTC().Add(time.Hour),
	)

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeInvalid) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeInvalid", err)
	}
	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if persistedOrganizationID.Valid || persistedWorkspaceID.Valid {
		t.Fatalf("archived setup scope drifted to a later tenant: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScope_RejectsLaterScopeWithoutSetupEraCandidate(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedSetupScopeAt(
		t,
		db,
		"org-later",
		"workspace-later",
		"active",
		"normal",
		time.Now().UTC().Add(time.Hour),
	)

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeUnavailable) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeUnavailable", err)
	}
}

func TestResolveDefaultScope_RejectsInvalidPersistedPairWithoutDrifting(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, "org-1", "workspace-2")
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	seedUsableSetupScope(t, db, "org-2", "workspace-2")
	seedSuperAdminContext(t, db, "admin-1", "org-1", "workspace-1")

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeInvalid) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeInvalid", err)
	}
	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if !persistedOrganizationID.Valid || persistedOrganizationID.String != "org-1" ||
		!persistedWorkspaceID.Valid || persistedWorkspaceID.String != "workspace-2" {
		t.Fatalf("invalid persisted scope drifted: %+v, %+v", persistedOrganizationID, persistedWorkspaceID)
	}
}

func TestResolveDefaultScope_ReturnsUnavailableWithoutSetupMarker(t *testing.T) {
	_, service := newSetupScopeTestService(t)

	_, _, err := service.ResolveDefaultScope(context.Background())
	if !errors.Is(err, ErrSetupScopeUnavailable) {
		t.Fatalf("ResolveDefaultScope() error = %v, want ErrSetupScopeUnavailable", err)
	}
}

func TestResolveDefaultScope_RejectsCancelledContext(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := service.ResolveDefaultScope(ctx)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "context canceled") {
		t.Fatalf("ResolveDefaultScope() error = %v, want context cancellation", err)
	}
}

type bootstrapWorkspaceManagementFake struct {
	interfaces.WorkspaceManagementService
	db *gorm.DB
}

func (s *bootstrapWorkspaceManagementFake) WithTx(tx *gorm.DB) interfaces.WorkspaceManagementService {
	return &bootstrapWorkspaceManagementFake{db: tx}
}

func (s *bootstrapWorkspaceManagementFake) CreateWorkspace(ctx context.Context, name string, _ bool) (*workspacemodel.Workspace, error) {
	workspace := &workspacemodel.Workspace{
		ID: uuid.NewString(), Name: name, Plan: "basic", Status: workspacemodel.WorkspaceStatusNormal,
	}
	if err := s.db.WithContext(ctx).Create(workspace).Error; err != nil {
		return nil, err
	}
	return workspace, nil
}

func (s *bootstrapWorkspaceManagementFake) CreateWorkspaceMember(ctx context.Context, workspaceID, accountID, role string) error {
	return s.db.WithContext(ctx).Create(&workspacemodel.WorkspaceMember{
		ID: uuid.NewString(), WorkspaceID: workspaceID, AccountID: accountID,
		Role: workspacemodel.WorkspaceMemberRole(role), Permissions: []string{},
		PermissionSource: workspacemodel.WorkspaceMemberPermissionSourceOwner,
	}).Error
}

type bootstrapOrganizationManagementFake struct {
	interfaces.OrganizationManagementService
	db *gorm.DB
}

func (s *bootstrapOrganizationManagementFake) WithTx(tx *gorm.DB) interfaces.OrganizationManagementService {
	return &bootstrapOrganizationManagementFake{db: tx}
}

func (s *bootstrapOrganizationManagementFake) CreateOrganization(ctx context.Context, name string) (*workspacemodel.Organization, error) {
	organization := &workspacemodel.Organization{Name: name, Status: workspacemodel.OrganizationStatusActive}
	if err := s.db.WithContext(ctx).Create(organization).Error; err != nil {
		return nil, err
	}
	return organization, nil
}

func (s *bootstrapOrganizationManagementFake) AddWorkspace(ctx context.Context, organizationID, workspaceID string) error {
	return s.db.WithContext(ctx).Model(&workspacemodel.Workspace{}).
		Where("id = ?", workspaceID).Update("organization_id", organizationID).Error
}

func (s *bootstrapOrganizationManagementFake) UpsertOrganizationRole(ctx context.Context, organizationID, accountID string, role workspacemodel.OrganizationRole) error {
	return s.db.WithContext(ctx).Create(&workspacemodel.OrganizationMember{
		OrganizationID: organizationID, AccountID: accountID, Role: role,
		Status: workspacemodel.OrganizationMemberStatusActive,
	}).Error
}

type bootstrapOutboxRow struct {
	ID             string `gorm:"column:id"`
	OrganizationID string `gorm:"column:organization_id"`
	AccountID      string `gorm:"column:account_id"`
}

func (bootstrapOutboxRow) TableName() string { return "registration_provisioning_outbox" }

func newCloudBootstrapOutboxTestService(
	t *testing.T,
	enqueuer RegistrationProvisioningOutboxEnqueuer,
) (*gorm.DB, *BootstrapService) {
	t.Helper()
	dsn := fmt.Sprintf("file:bootstrap-outbox-%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open bootstrap outbox test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get bootstrap outbox test sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&systemmodel.Setup{},
		&systemmodel.BootstrapLock{},
		&authmodel.Account{},
		&authmodel.AccountContext{},
		&workspacemodel.Organization{},
		&workspacemodel.Workspace{},
		&workspacemodel.WorkspaceMember{},
		&workspacemodel.OrganizationMember{},
		&bootstrapOutboxRow{},
	); err != nil {
		t.Fatalf("migrate bootstrap outbox test schema: %v", err)
	}

	service := NewBootstrapService(
		repository.NewSetupRepository(db),
		repository.NewBootstrapLockRepository(db),
		authrepo.NewAccountRepository(db),
		db,
		&bootstrapWorkspaceManagementFake{db: db},
		&bootstrapOrganizationManagementFake{db: db},
		nil,
		enqueuer,
	)
	return db, service
}

func insertBootstrapOutbox(ctx context.Context, tx *gorm.DB, accountID, organizationID string) error {
	return tx.WithContext(ctx).Create(&bootstrapOutboxRow{
		ID: uuid.NewString(), OrganizationID: organizationID, AccountID: accountID,
	}).Error
}

func cloudBootstrapParams() BootstrapParams {
	return BootstrapParams{
		AdminEmail: "cloud-admin@example.com", AdminName: "Cloud Admin", AdminPassword: "Password123",
		Language: "en-US", Source: BootstrapSourceCloudEnv,
	}
}

func TestCloudBootstrapEnqueuesRegistrationProvisioningInTransaction(t *testing.T) {
	db, service := newCloudBootstrapOutboxTestService(t, insertBootstrapOutbox)

	if err := service.Bootstrap(t.Context(), cloudBootstrapParams()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	var outbox bootstrapOutboxRow
	if err := db.First(&outbox).Error; err != nil {
		t.Fatalf("load bootstrap outbox: %v", err)
	}
	var account authmodel.Account
	if err := db.First(&account, "id = ?", outbox.AccountID).Error; err != nil {
		t.Fatalf("load bootstrap account: %v", err)
	}
	var organization workspacemodel.Organization
	if err := db.First(&organization, "id = ?", outbox.OrganizationID).Error; err != nil {
		t.Fatalf("load bootstrap organization: %v", err)
	}
}

func TestCloudBootstrapRollsBackWhenRegistrationProvisioningEnqueueFails(t *testing.T) {
	wantErr := errors.New("outbox unavailable")
	db, service := newCloudBootstrapOutboxTestService(t, func(ctx context.Context, tx *gorm.DB, accountID, organizationID string) error {
		if err := insertBootstrapOutbox(ctx, tx, accountID, organizationID); err != nil {
			return err
		}
		return wantErr
	})

	err := service.Bootstrap(t.Context(), cloudBootstrapParams())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Bootstrap() error = %v, want %v", err, wantErr)
	}
	for table, target := range map[string]any{
		"accounts":                         &authmodel.Account{},
		"organizations":                    &workspacemodel.Organization{},
		"registration_provisioning_outbox": &bootstrapOutboxRow{},
	} {
		var count int64
		if err := db.Model(target).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want transaction rollback", table, count)
		}
	}
}

func TestCloudBootstrapFailsClosedWithoutRegistrationProvisioningEnqueuer(t *testing.T) {
	_, service := newCloudBootstrapOutboxTestService(t, nil)

	err := service.Bootstrap(t.Context(), cloudBootstrapParams())
	if err == nil || !strings.Contains(err.Error(), "requires registration provisioning outbox") {
		t.Fatalf("Bootstrap() error = %v, want missing outbox failure", err)
	}
}
