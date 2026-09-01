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
	"github.com/zgiai/zgi/api/internal/modules/system/repository"
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
			status TEXT NOT NULL
		)`,
		`CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			organization_id TEXT NULL,
			status TEXT NOT NULL
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
	if err := db.Exec(
		`INSERT INTO organizations (id, status) VALUES (?, ?)`,
		organizationID,
		"active",
	).Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO workspaces (id, organization_id, status) VALUES (?, ?, ?)`,
		workspaceID,
		organizationID,
		"normal",
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

func TestResolveDefaultScope_PrefersSuperAdminContextAndBackfills(t *testing.T) {
	db, service := newSetupScopeTestService(t)
	seedLegacySetup(t, db, nil, nil)
	seedUsableSetupScope(t, db, "org-1", "workspace-1")
	seedUsableSetupScope(t, db, "org-2", "workspace-2")
	seedSuperAdminContext(t, db, "admin-1", "org-2", "workspace-2")

	organizationID, workspaceID, err := service.ResolveDefaultScope(context.Background())
	if err != nil {
		t.Fatalf("ResolveDefaultScope() error = %v, want nil", err)
	}
	if organizationID != "org-2" || workspaceID != "workspace-2" {
		t.Fatalf("ResolveDefaultScope() = %q, %q, want org-2, workspace-2", organizationID, workspaceID)
	}

	persistedOrganizationID, persistedWorkspaceID := readPersistedSetupScope(t, db)
	if !persistedOrganizationID.Valid || persistedOrganizationID.String != organizationID ||
		!persistedWorkspaceID.Valid || persistedWorkspaceID.String != workspaceID {
		t.Fatalf("persisted scope = %+v, %+v, want resolved scope", persistedOrganizationID, persistedWorkspaceID)
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

func TestResolveDefaultScope_RejectsAmbiguousUniqueFallback(t *testing.T) {
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
