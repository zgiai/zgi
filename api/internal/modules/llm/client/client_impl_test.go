package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetOrganizationIDFromApp_AgentUsesWorkspaceOrganization(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm with sqlmock: %v", err)
	}

	mock.ExpectQuery(`SELECT .*w\.organization_id.*FROM .*agents AS a.*JOIN workspaces w ON w\.id = a\.tenant_id.*WHERE a\.id = .*a\.deleted_at IS NULL.*`).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-1"))

	c := &llmClientImpl{db: db}
	orgID, err := c.getOrganizationIDFromApp(context.Background(), "agent-1", "agent")
	if err != nil {
		t.Fatalf("getOrganizationIDFromApp returned error: %v", err)
	}
	if orgID != "org-1" {
		t.Fatalf("organizationID = %q, want %q", orgID, "org-1")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestResolveOrganizationID_PrefersWorkspaceOrganization(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm with sqlmock: %v", err)
	}

	mock.ExpectQuery(`SELECT .*organization_id.*FROM .*workspaces.*WHERE id = .*`).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-from-workspace"))

	c := &llmClientImpl{db: db}
	orgID, err := c.resolveOrganizationID(context.Background(), &AppContext{
		AppID:       "agent-1",
		AppType:     "agent",
		AccountID:   "acc-1",
		WorkspaceID: "ws-1",
	})
	if err != nil {
		t.Fatalf("resolveOrganizationID returned error: %v", err)
	}
	if orgID != "org-from-workspace" {
		t.Fatalf("organizationID = %q, want %q", orgID, "org-from-workspace")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestGetOrganizationIDFromWorkspace_DoesNotRequireDeletedAtColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY, organization_id TEXT)`).Error; err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, organization_id) VALUES (?, ?)`, "ws-1", "org-1").Error; err != nil {
		t.Fatalf("insert workspace: %v", err)
	}

	c := &llmClientImpl{db: db}
	orgID, err := c.getOrganizationIDFromWorkspace(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("getOrganizationIDFromWorkspace returned error: %v", err)
	}
	if orgID != "org-1" {
		t.Fatalf("organizationID = %q, want %q", orgID, "org-1")
	}
}

func TestResolveOrganizationID_ZeroOrganizationFallsBackToCallerOrganization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE account_contexts (account_id TEXT PRIMARY KEY, current_organization_id TEXT, current_workspace_id TEXT)`).Error; err != nil {
		t.Fatalf("create account_contexts table: %v", err)
	}
	if err := db.Exec(`INSERT INTO account_contexts (account_id, current_organization_id, current_workspace_id) VALUES (?, ?, ?)`, "acc-1", "org-caller", "ws-1").Error; err != nil {
		t.Fatalf("insert account context: %v", err)
	}

	c := &llmClientImpl{db: db}
	orgID, err := c.resolveOrganizationID(context.Background(), &AppContext{
		AppID:          "agent-1",
		AppType:        "agent",
		AccountID:      "acc-1",
		OrganizationID: "00000000-0000-0000-0000-000000000000",
		WorkspaceID:    "00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("resolveOrganizationID returned error: %v", err)
	}
	if orgID != "org-caller" {
		t.Fatalf("organizationID = %q, want %q", orgID, "org-caller")
	}
}

func TestResolveOrganizationID_MissingWorkspaceFallsBackToCallerOrganization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY, organization_id TEXT)`).Error; err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE account_contexts (account_id TEXT PRIMARY KEY, current_organization_id TEXT, current_workspace_id TEXT)`).Error; err != nil {
		t.Fatalf("create account_contexts table: %v", err)
	}
	if err := db.Exec(`INSERT INTO account_contexts (account_id, current_organization_id, current_workspace_id) VALUES (?, ?, ?)`, "acc-1", "org-caller", "ws-current").Error; err != nil {
		t.Fatalf("insert account context: %v", err)
	}

	c := &llmClientImpl{db: db}
	orgID, err := c.resolveOrganizationID(context.Background(), &AppContext{
		AppID:       "agent-1",
		AppType:     "agent",
		AccountID:   "acc-1",
		WorkspaceID: "ws-missing",
	})
	if err != nil {
		t.Fatalf("resolveOrganizationID returned error: %v", err)
	}
	if orgID != "org-caller" {
		t.Fatalf("organizationID = %q, want %q", orgID, "org-caller")
	}
}
