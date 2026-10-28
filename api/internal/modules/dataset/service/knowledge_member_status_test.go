package service

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// The disabled-member predicate is part of both workspace access queries, so
// a disabled organization member's surviving workspace rows grant nothing.
func TestKnowledgeWorkspaceAccessExcludesDisabledOrganizationMembers(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}
	disabledPredicate := regexp.QuoteMeta(`NOT EXISTS (SELECT 1 FROM members AS disabled_org_members`)

	mock.ExpectQuery(`FROM "workspace_members" WHERE workspace_id = \$1 AND account_id = \$2 AND `+disabledPredicate+
		`.*disabled_org_workspaces\.id = workspace_members\.workspace_id AND disabled_org_members\.account_id = workspace_members\.account_id AND disabled_org_members\.status = 'inactive'`).
		WithArgs("workspace-1", "account-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"role", "role_id", "permissions", "permission_source"}))
	allowed, err := svc.canAccessKnowledgeWorkspace(context.Background(), "workspace-1", "account-1")
	if err != nil {
		t.Fatalf("canAccessKnowledgeWorkspace() error = %v", err)
	}
	if allowed {
		t.Fatal("a member filtered out by the disabled predicate must not access the workspace")
	}

	mock.ExpectQuery(`WHERE workspaces\.organization_id = \$1 AND workspace_members\.account_id = \$2 AND ` + disabledPredicate +
		`.*disabled_org_workspaces\.id = workspaces\.id AND disabled_org_members\.account_id = workspace_members\.account_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "role_id", "permissions", "permission_source"}))
	workspaceIDs, err := svc.accessibleKnowledgeWorkspaceIDs(context.Background(), "org-1", "", "account-1")
	if err != nil {
		t.Fatalf("accessibleKnowledgeWorkspaceIDs() error = %v", err)
	}
	if len(workspaceIDs) != 0 {
		t.Fatalf("workspaceIDs = %v, want none", workspaceIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
