package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
)

func TestSQLAuditContextFallsBackToOrganizationWorkspace(t *testing.T) {
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
	}
	table := &model.Table{
		ID:   "table-1",
		Name: "Users",
	}

	auditCtx := sqlAuditContext("organization-1", dataSource, table, "account-1", "query")
	if auditCtx == nil {
		t.Fatal("audit context is nil")
	}
	if auditCtx.OrganizationID != "organization-1" {
		t.Fatalf("organization_id = %s, want organization-1", auditCtx.OrganizationID)
	}
	if auditCtx.WorkspaceID != "organization-1" {
		t.Fatalf("workspace_id = %s, want organization-1", auditCtx.WorkspaceID)
	}
	if auditCtx.TableID != "table-1" {
		t.Fatalf("table_id = %s, want table-1", auditCtx.TableID)
	}
}

func TestSQLAuditContextUsesDataSourceWorkspace(t *testing.T) {
	workspaceID := "workspace-1"
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		WorkspaceID:    &workspaceID,
		Name:           "main",
	}

	auditCtx := sqlAuditContext("organization-1", dataSource, nil, "account-1", "query")
	if auditCtx == nil {
		t.Fatal("audit context is nil")
	}
	if auditCtx.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %s, want workspace-1", auditCtx.WorkspaceID)
	}
}
