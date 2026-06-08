package repository

import (
	"context"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetPaginatedByTenantIDsWithPermissionsFiltersByDatasetPermission(t *testing.T) {
	db := newDatasetPermissionTestDB(t)
	repo := NewDatasetRepository(db)
	ctx := context.Background()

	insertWorkspaceMember(t, db, "workspace-1", "member-1", workspace_model.WorkspaceRoleNormal)
	insertDatasetPermissionRow(t, db, "private-other", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionOnlyMe), time.Now().Add(-4*time.Minute))
	insertDatasetPermissionRow(t, db, "private-own", "org-1", "workspace-1", "member-1", string(model.DatasetPermissionOnlyMe), time.Now().Add(-3*time.Minute))
	insertDatasetPermissionRow(t, db, "team", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionAllTeam), time.Now().Add(-2*time.Minute))
	insertDatasetPermissionRow(t, db, "legacy-team", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionAllTeamMembers), time.Now().Add(-time.Minute))
	insertDatasetPermissionRow(t, db, "unknown", "org-1", "workspace-1", "owner-1", "unexpected", time.Now())

	datasets, total, err := repo.GetPaginatedByTenantIDsWithPermissions(ctx, []string{"workspace-1"}, "member-1", false, nil, 1, 20, "", "DESC")
	if err != nil {
		t.Fatalf("get datasets: %v", err)
	}

	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	assertDatasetIDs(t, datasets, []string{"legacy-team", "team", "private-own"})
}

func TestGetDatasetsInFolderByIDWithPaginationWithPermissionsFiltersByDatasetPermission(t *testing.T) {
	db := newDatasetPermissionTestDB(t)
	repo := NewDatasetFolderRepository(db)
	ctx := context.Background()

	insertWorkspaceMember(t, db, "workspace-1", "member-1", workspace_model.WorkspaceRoleNormal)
	insertDatasetPermissionRow(t, db, "private-other", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionOnlyMe), time.Now().Add(-4*time.Minute))
	insertDatasetPermissionRow(t, db, "private-own", "org-1", "workspace-1", "member-1", string(model.DatasetPermissionOnlyMe), time.Now().Add(-3*time.Minute))
	insertDatasetPermissionRow(t, db, "team", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionAllTeam), time.Now().Add(-2*time.Minute))
	insertDatasetPermissionRow(t, db, "legacy-team", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionAllTeamMembers), time.Now().Add(-time.Minute))
	insertDatasetPermissionRow(t, db, "unknown", "org-1", "workspace-1", "owner-1", "unexpected", time.Now())

	datasets, total, err := repo.GetDatasetsInFolderByIDWithPaginationWithPermissions(ctx, "", "org-1", []string{"workspace-1"}, "member-1", false, nil, 1, 20, "")
	if err != nil {
		t.Fatalf("get folder datasets: %v", err)
	}

	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	assertDatasetIDs(t, datasets, []string{"legacy-team", "team", "private-own"})
}

func TestGetPaginatedByTenantIDsWithPermissionsAllowsWorkspaceAdmin(t *testing.T) {
	db := newDatasetPermissionTestDB(t)
	repo := NewDatasetRepository(db)
	ctx := context.Background()

	insertWorkspaceMember(t, db, "workspace-1", "admin-1", workspace_model.WorkspaceRoleAdmin)
	insertDatasetPermissionRow(t, db, "private-other", "org-1", "workspace-1", "owner-1", string(model.DatasetPermissionOnlyMe), time.Now())

	datasets, total, err := repo.GetPaginatedByTenantIDsWithPermissions(ctx, []string{"workspace-1"}, "admin-1", false, nil, 1, 20, "", "DESC")
	if err != nil {
		t.Fatalf("get datasets: %v", err)
	}

	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	assertDatasetIDs(t, datasets, []string{"private-other"})
}

func newDatasetPermissionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	statements := []string{
		`CREATE TABLE datasets (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text NOT NULL,
			name text NOT NULL,
			description text,
			provider text NOT NULL,
			permission text NOT NULL,
			enable_graph_flow boolean,
			created_by text NOT NULL,
			created_at datetime,
			updated_by text,
			updated_at datetime,
			owner text,
			embedding_model text,
			embedding_model_provider text,
			entity_model text,
			entity_model_provider text,
			collection_binding_id text,
			retrieval_config text,
			icon_type text,
			icon text,
			icon_background text,
			process_rule text
		)`,
		`CREATE TABLE workspace_members (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			account_id text NOT NULL,
			role text NOT NULL,
			role_id text,
			current boolean,
			created_at datetime,
			updated_at datetime,
			invited_by text,
			extensions text
		)`,
		`CREATE TABLE dataset_folder_joins (
			id text PRIMARY KEY,
			dataset_id text NOT NULL,
			folder_id text NOT NULL,
			created_by text,
			created_at datetime
		)`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}

	return db
}

func insertDatasetPermissionRow(t *testing.T, db *gorm.DB, id, organizationID, workspaceID, createdBy, permission string, createdAt time.Time) {
	t.Helper()

	dataset := &model.Dataset{
		ID:             id,
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Name:           id,
		Provider:       "vendor",
		Permission:     permission,
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if err := db.Create(dataset).Error; err != nil {
		t.Fatalf("insert dataset %s: %v", id, err)
	}
}

func insertWorkspaceMember(t *testing.T, db *gorm.DB, workspaceID, accountID string, role workspace_model.WorkspaceMemberRole) {
	t.Helper()

	member := &workspace_model.WorkspaceMember{
		ID:          workspaceID + "-" + accountID,
		WorkspaceID: workspaceID,
		AccountID:   accountID,
		Role:        role,
	}
	if err := db.Create(member).Error; err != nil {
		t.Fatalf("insert workspace member %s/%s: %v", workspaceID, accountID, err)
	}
}

func assertDatasetIDs(t *testing.T, datasets []*model.Dataset, want []string) {
	t.Helper()

	got := make([]string, 0, len(datasets))
	for _, dataset := range datasets {
		got = append(got, dataset.ID)
	}

	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}
