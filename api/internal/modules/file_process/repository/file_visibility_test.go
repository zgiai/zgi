package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	file_model "github.com/zgiai/ginext/internal/modules/file_process/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFileRepository_ListByTenantID_RestrictsToVisibleWorkspaceIDs(t *testing.T) {
	t.Parallel()

	db := newFileVisibilityTestDB(t)
	repo := NewFileRepository(db)

	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-alpha",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		StorageType:    "local",
		Key:            "alpha",
		Name:           "alpha.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-beta",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-beta"),
		StorageType:    "local",
		Key:            "beta",
		Name:           "beta.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)

	files, total, err := repo.ListByTenantIDs(t.Context(), "org-1", "account-1", false, []string{"ws-alpha"}, 1, 20, "", "", "", nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, files, 1)
	require.Equal(t, "file-alpha", files[0].ID)
}

func TestFileFolderRepository_ListFoldersWithPermissionFilter_RestrictsToVisibleWorkspaceIDs(t *testing.T) {
	t.Parallel()

	db := newFileVisibilityTestDB(t)
	repo := NewFileFolderRepository(db)

	require.NoError(t, db.Create(&file_model.FileFolder{
		ID:             "folder-alpha",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		Name:           "Alpha",
		Permission:     string(file_model.FileFolderPermissionAllTeam),
		CreatedBy:      "owner-1",
		CreatedAt:      time.Unix(1710000000, 0),
		UpdatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolder{
		ID:             "folder-beta",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-beta"),
		Name:           "Beta",
		Permission:     string(file_model.FileFolderPermissionAllTeam),
		CreatedBy:      "owner-1",
		CreatedAt:      time.Unix(1710000000, 0),
		UpdatedAt:      time.Unix(1710000000, 0),
	}).Error)

	folders, total, err := repo.ListFoldersWithPermissionFilter(t.Context(), "org-1", "account-1", nil, 1, 20, "", "", "", []string{"ws-alpha"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, folders, 1)
	require.Equal(t, "folder-alpha", folders[0].ID)
}

func TestFileFolderRepository_ListFoldersWithPermissionFilter_RespectsFolderVisibility(t *testing.T) {
	t.Parallel()

	db := newFileVisibilityTestDB(t)
	repo := NewFileFolderRepository(db)

	require.NoError(t, db.Exec(
		"INSERT INTO workspaces (id, organization_id, status) VALUES (?, ?, ?)",
		"ws-alpha",
		"org-1",
		"normal",
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO workspace_members (id, workspace_id, account_id, role, current, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"wm-1",
		"ws-alpha",
		"account-1",
		"normal",
		false,
		time.Unix(1710000000, 0),
		time.Unix(1710000000, 0),
	).Error)

	require.NoError(t, db.Create(&file_model.FileFolder{
		ID:             "folder-private",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		Name:           "Private",
		Permission:     string(file_model.FileFolderPermissionOnlyMe),
		CreatedBy:      "owner-2",
		CreatedAt:      time.Unix(1710000000, 0),
		UpdatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolder{
		ID:             "folder-partial",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		Name:           "Partial",
		Permission:     string(file_model.FileFolderPermissionPartialTeam),
		CreatedBy:      "owner-2",
		CreatedAt:      time.Unix(1710000000, 0),
		UpdatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolderPermission{
		ID:          "ffp-1",
		FolderID:    "folder-partial",
		WorkspaceID: "ws-alpha",
		CreatedBy:   "owner-2",
		CreatedAt:   time.Unix(1710000000, 0),
	}).Error)

	folders, total, err := repo.ListFoldersWithPermissionFilter(t.Context(), "org-1", "account-1", nil, 1, 20, "", "", "", []string{"ws-alpha"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, folders, 1)
	require.Equal(t, "folder-partial", folders[0].ID)
}

func TestFileFolderRepository_ListAllFilesWithFiltersAndTenant_HidesFilesInInvisibleFolders(t *testing.T) {
	t.Parallel()

	db := newFileVisibilityTestDB(t)
	repo := NewFileFolderRepository(db)

	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-root",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		StorageType:    "local",
		Key:            "root",
		Name:           "root.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-private",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		StorageType:    "local",
		Key:            "private",
		Name:           "private.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolder{
		ID:             "folder-private",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		Name:           "Private",
		Permission:     string(file_model.FileFolderPermissionOnlyMe),
		CreatedBy:      "owner-2",
		CreatedAt:      time.Unix(1710000000, 0),
		UpdatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolderJoins{
		ID:        "join-1",
		FileID:    "file-private",
		FolderID:  "folder-private",
		CreatedBy: "owner-2",
		CreatedAt: time.Unix(1710000000, 0),
	}).Error)

	files, total, err := repo.ListAllFilesWithFiltersAndTenant(t.Context(), 1, 20, "", "", "", nil, nil, "org-1", "account-1", false, []string{"ws-alpha"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, files, 1)
	require.Equal(t, "file-root", files[0].ID)
}

func TestFileRepository_ListByTenantIDs_HidesFilesInInvisibleFolders(t *testing.T) {
	t.Parallel()

	db := newFileVisibilityTestDB(t)
	repo := NewFileRepository(db)

	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-visible",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		StorageType:    "local",
		Key:            "visible",
		Name:           "visible.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-hidden",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		StorageType:    "local",
		Key:            "hidden",
		Name:           "hidden.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolder{
		ID:             "folder-private",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		Name:           "Private",
		Permission:     string(file_model.FileFolderPermissionOnlyMe),
		CreatedBy:      "owner-2",
		CreatedAt:      time.Unix(1710000000, 0),
		UpdatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.FileFolderJoins{
		ID:        "join-1",
		FileID:    "file-hidden",
		FolderID:  "folder-private",
		CreatedBy: "owner-2",
		CreatedAt: time.Unix(1710000000, 0),
	}).Error)

	files, total, err := repo.ListByTenantIDs(t.Context(), "org-1", "account-1", false, []string{"ws-alpha"}, 1, 20, "", "", "", nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, files, 1)
	require.Equal(t, "file-visible", files[0].ID)
}

func TestFileFolderRepository_ListAllFilesWithFiltersAndTenant_RestrictsToVisibleWorkspaceIDs(t *testing.T) {
	t.Parallel()

	db := newFileVisibilityTestDB(t)
	repo := NewFileFolderRepository(db)

	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-alpha",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-alpha"),
		StorageType:    "local",
		Key:            "alpha",
		Name:           "alpha.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)
	require.NoError(t, db.Create(&file_model.UploadFile{
		ID:             "file-beta",
		OrganizationID: "org-1",
		WorkspaceID:    ptr("ws-beta"),
		StorageType:    "local",
		Key:            "beta",
		Name:           "beta.txt",
		Size:           1,
		Extension:      "txt",
		CreatedByRole:  file_model.CreatedByRoleAccount,
		CreatedBy:      "account-1",
		CreatedAt:      time.Unix(1710000000, 0),
	}).Error)

	files, total, err := repo.ListAllFilesWithFiltersAndTenant(t.Context(), 1, 20, "", "", "", nil, nil, "org-1", "account-1", false, []string{"ws-alpha"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, files, 1)
	require.Equal(t, "file-alpha", files[0].ID)
}

func newFileVisibilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE upload_files (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			workspace_id TEXT,
			is_temporary BOOLEAN DEFAULT FALSE,
			storage_type TEXT NOT NULL,
			key TEXT NOT NULL,
			name TEXT NOT NULL,
			size INTEGER NOT NULL,
			extension TEXT NOT NULL,
			mime_type TEXT,
			created_by_role TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			used BOOLEAN DEFAULT FALSE,
			used_by TEXT,
			used_at DATETIME,
			hash TEXT,
			source_url TEXT,
			content_text TEXT,
			is_archived BOOLEAN DEFAULT FALSE,
			archived_at DATETIME,
			archived_by TEXT
		)`,
		`CREATE TABLE file_folders (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			workspace_id TEXT,
			name TEXT NOT NULL,
			description TEXT,
			parent_id TEXT,
			created_by TEXT NOT NULL,
			updated_by TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			icon_type TEXT,
			icon TEXT,
			icon_background TEXT,
			position INTEGER DEFAULT 0,
			permission TEXT NOT NULL
		)`,
		`CREATE TABLE file_folder_permissions (
			id TEXT PRIMARY KEY,
			folder_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE file_folder_joins (
			id TEXT PRIMARY KEY,
			file_id TEXT NOT NULL,
			folder_id TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE workspace_members (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			role TEXT NOT NULL,
			current BOOLEAN DEFAULT FALSE,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			status TEXT NOT NULL
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}

	return db
}

func ptr[T any](v T) *T {
	return &v
}
