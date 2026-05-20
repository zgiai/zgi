package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newDatasetFolderRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	mustExecDatasetFolderTestSQL(t, db, `
		CREATE TABLE dataset_folders (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			parent_id TEXT,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_by TEXT,
			updated_at DATETIME NOT NULL,
			icon_type TEXT,
			icon TEXT,
			icon_background TEXT,
			position INTEGER NOT NULL DEFAULT 0,
			permission TEXT NOT NULL DEFAULT 'only_me'
		)
	`)

	return db
}

func mustExecDatasetFolderTestSQL(t *testing.T, db *gorm.DB, sql string, args ...interface{}) {
	t.Helper()

	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec failed: %v\nsql=%s", err, sql)
	}
}

func TestGetFoldersByParentWithPaginationWithPermissions_FiltersByWorkspaceIDs(t *testing.T) {
	db := newDatasetFolderRepositoryTestDB(t)
	repo := NewDatasetFolderRepository(db)
	now := time.Now().UTC().Truncate(time.Second)

	folders := []*model.DatasetFolder{
		{
			ID:             "folder-a",
			OrganizationID: "org-1",
			WorkspaceID:    "ws-a",
			Name:           "Folder A",
			CreatedBy:      "user-1",
			CreatedAt:      now,
			UpdatedAt:      now,
			Position:       0,
			Permission:     "only_me",
		},
		{
			ID:             "folder-b",
			OrganizationID: "org-1",
			WorkspaceID:    "ws-b",
			Name:           "Folder B",
			CreatedBy:      "user-1",
			CreatedAt:      now.Add(time.Second),
			UpdatedAt:      now.Add(time.Second),
			Position:       0,
			Permission:     "only_me",
		},
		{
			ID:             "folder-c",
			OrganizationID: "org-2",
			WorkspaceID:    "ws-c",
			Name:           "Folder C",
			CreatedBy:      "user-1",
			CreatedAt:      now.Add(2 * time.Second),
			UpdatedAt:      now.Add(2 * time.Second),
			Position:       0,
			Permission:     "only_me",
		},
	}

	if err := db.Create(&folders).Error; err != nil {
		t.Fatalf("seed folders: %v", err)
	}

	items, total, err := repo.GetFoldersByParentWithPaginationWithPermissions(
		context.Background(),
		nil,
		"org-1",
		[]string{"ws-a"},
		"user-1",
		true,
		[]string{"ws-a", "ws-b"},
		1,
		20,
		"",
	)
	if err != nil {
		t.Fatalf("query folders: %v", err)
	}

	if total != 1 {
		t.Fatalf("expected total 1 for workspace ws-a, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 folder for workspace ws-a, got %d", len(items))
	}
	if items[0].WorkspaceID != "ws-a" {
		t.Fatalf("expected workspace_id ws-a, got %s", items[0].WorkspaceID)
	}
}
