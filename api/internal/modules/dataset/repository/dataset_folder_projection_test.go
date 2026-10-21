package repository

import (
	"context"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The runtime projection schema is hand-written because the models carry
// PostgreSQL-only defaults (uuid_generate_v4, CURRENT_TIMESTAMP(0)) that SQLite
// rejects during AutoMigrate.
const folderProjectionTestSchema = `
CREATE TABLE datasets (
	id text PRIMARY KEY,
	organization_id text NOT NULL,
	workspace_id text,
	name text NOT NULL,
	description text,
	provider text NOT NULL DEFAULT 'vendor',
	permission text NOT NULL DEFAULT 'all_team',
	enable_graph_flow numeric DEFAULT false,
	graph_status text NOT NULL DEFAULT 'disabled',
	graph_revision integer NOT NULL DEFAULT 0,
	graph_available_revision integer,
	graph_projected_revision integer NOT NULL DEFAULT 0,
	graph_visibility_revision integer NOT NULL DEFAULT 0,
	graph_projected_visibility_revision integer NOT NULL DEFAULT 0,
	graph_current_run_id text,
	graph_progress integer NOT NULL DEFAULT 0,
	graph_error_code text,
	graph_error_message text,
	graph_ready_at datetime,
	graph_updated_at datetime,
	created_by text NOT NULL,
	created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_by text,
	updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
);
CREATE TABLE dataset_folder_joins (
	id text PRIMARY KEY,
	dataset_id text NOT NULL,
	folder_id text NOT NULL,
	created_by text NOT NULL,
	created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

// The hidden DigitalHuman memory projection is runtime infrastructure. It must
// never be listed as a user knowledge base, so every folder listing path that
// feeds the knowledge base page has to exclude its provider.
func TestFolderListingsHideRuntimeDigitalHumanProjection(t *testing.T) {
	ctx := context.Background()

	t.Run("root listings", func(t *testing.T) {
		db := newFolderProjectionTestDB(t)
		repo := NewDatasetFolderRepository(db)
		listings := map[string]func() ([]*model.Dataset, error){
			"GetDatasetsInFolderByID": func() ([]*model.Dataset, error) {
				return repo.GetDatasetsInFolderByID(ctx, "", []string{"workspace-1"})
			},
			"GetDatasetsInFolderByIDWithPagination": func() ([]*model.Dataset, error) {
				items, _, err := repo.GetDatasetsInFolderByIDWithPagination(ctx, "", []string{"workspace-1"}, 1, 20, "")
				return items, err
			},
			"GetDatasetsInFolderByIDWithPaginationWithPermissions": func() ([]*model.Dataset, error) {
				items, _, err := repo.GetDatasetsInFolderByIDWithPaginationWithPermissions(ctx, "", "org-1", []string{"workspace-1"}, "member-1", false, nil, 1, 20, "")
				return items, err
			},
		}
		for name, list := range listings {
			t.Run(name, func(t *testing.T) {
				assertOnlyVisibleDataset(t, list)
			})
		}
	})

	t.Run("folder listing", func(t *testing.T) {
		db := newFolderProjectionTestDB(t)
		repo := NewDatasetFolderRepository(db)
		for _, datasetID := range []string{"visible", "projection"} {
			join := &model.DatasetFolderJoins{ID: datasetID + "-join", DatasetID: datasetID, FolderID: "folder-1", CreatedBy: "member-1", CreatedAt: time.Now()}
			if err := db.Create(join).Error; err != nil {
				t.Fatalf("insert folder join: %v", err)
			}
		}
		assertOnlyVisibleDataset(t, func() ([]*model.Dataset, error) {
			items, _, err := repo.GetDatasetsInFolder(ctx, "folder-1", 1, 20)
			return items, err
		})
	})
}

func newFolderProjectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(folderProjectionTestSchema).Error; err != nil {
		t.Fatalf("create dataset schema: %v", err)
	}
	now := time.Now()
	rows := []*model.Dataset{
		{ID: "visible", OrganizationID: "org-1", WorkspaceID: "workspace-1", Name: "visible", Provider: "vendor", Permission: string(model.DatasetPermissionAllTeam), CreatedBy: "member-1", CreatedAt: now, UpdatedAt: now},
		{ID: "projection", OrganizationID: "org-1", WorkspaceID: "workspace-1", Name: "DigitalHuman memory projection", Provider: model.ProviderDigitalHumanMemory, Permission: string(model.DatasetPermissionOnlyMe), CreatedBy: "member-1", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("insert datasets: %v", err)
	}
	return db
}

func assertOnlyVisibleDataset(t *testing.T, list func() ([]*model.Dataset, error)) {
	t.Helper()

	items, err := list()
	if err != nil {
		t.Fatalf("list folder datasets: %v", err)
	}
	if len(items) != 1 || items[0].ID != "visible" {
		t.Fatalf("datasets = %#v, want only the user-visible knowledge base", items)
	}
}
