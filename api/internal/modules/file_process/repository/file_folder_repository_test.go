package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

func TestListAllFilesWithFiltersAndTenantFiltersByCurrentAssetProductStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileFolderRepositoryTables(t, db)

	insertUploadFile(t, db, "file-failed", "Failed", "2026-06-18 10:00:00")
	insertUploadFile(t, db, "file-confirming", "Confirming", "2026-06-18 09:00:00")
	insertUploadFile(t, db, "file-ready", "Ready", "2026-06-18 08:00:00")
	insertUploadFile(t, db, "file-old-failed-now-ready", "Old failed now ready", "2026-06-18 07:00:00")
	insertUploadFile(t, db, "file-no-asset", "No asset", "2026-06-18 06:00:00")

	insertDocumentAsset(t, db, "asset-failed", "file-failed", "parse_failed", "2026-06-18 10:10:00")
	insertDocumentAsset(t, db, "asset-confirming", "file-confirming", "confirming", "2026-06-18 09:10:00")
	insertDocumentAsset(t, db, "asset-ready", "file-ready", "ready", "2026-06-18 08:10:00")
	insertDocumentAsset(t, db, "asset-old-failed", "file-old-failed-now-ready", "parse_failed", "2026-06-18 07:10:00")
	insertDocumentAsset(t, db, "asset-new-ready", "file-old-failed-now-ready", "ready", "2026-06-18 07:20:00")

	repo := NewFileFolderRepository(db)
	files, total, err := repo.ListAllFilesWithFiltersAndTenant(
		context.Background(),
		1,
		20,
		"",
		"created_at_desc",
		"",
		"parse_failed",
		nil,
		nil,
		"org-1",
		"account-1",
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}

	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].ID != "file-failed" {
		t.Fatalf("expected current parse-failed file only, got %q", files[0].ID)
	}
}

func TestCountAllFilesByCurrentAssetProductStatusUsesLatestAsset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileFolderRepositoryTables(t, db)

	insertUploadFile(t, db, "file-failed", "Failed", "2026-06-18 10:00:00")
	insertUploadFile(t, db, "file-ready", "Ready", "2026-06-18 09:00:00")
	insertUploadFile(t, db, "file-old-failed-now-ready", "Old failed now ready", "2026-06-18 08:00:00")
	insertUploadFile(t, db, "file-no-asset", "No asset", "2026-06-18 07:00:00")

	insertDocumentAsset(t, db, "asset-failed", "file-failed", "parse_failed", "2026-06-18 10:10:00")
	insertDocumentAsset(t, db, "asset-ready", "file-ready", "ready", "2026-06-18 09:10:00")
	insertDocumentAsset(t, db, "asset-old-failed", "file-old-failed-now-ready", "parse_failed", "2026-06-18 08:10:00")
	insertDocumentAsset(t, db, "asset-new-ready", "file-old-failed-now-ready", "ready", "2026-06-18 08:20:00")

	repo := NewFileFolderRepository(db)
	counts, err := repo.CountAllFilesByCurrentAssetProductStatus(
		context.Background(),
		"",
		"",
		nil,
		nil,
		"org-1",
		"account-1",
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("count files by status: %v", err)
	}

	if got, want := counts["all"], int64(4); got != want {
		t.Fatalf("all count = %d, want %d", got, want)
	}
	if got, want := counts["parse_failed"], int64(1); got != want {
		t.Fatalf("parse_failed count = %d, want %d", got, want)
	}
	if got, want := counts["ready"], int64(2); got != want {
		t.Fatalf("ready count = %d, want %d", got, want)
	}
	if got := counts["stored_only"]; got != 0 {
		t.Fatalf("stored_only count = %d, want 0", got)
	}
}

func TestListFilesInFolderWithFiltersAndTenantFiltersByCurrentAssetProductStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileFolderRepositoryTables(t, db)

	insertUploadFile(t, db, "file-ready", "Ready", "2026-06-18 10:00:00")
	insertUploadFile(t, db, "file-stored", "Stored", "2026-06-18 09:00:00")
	insertUploadFile(t, db, "file-other-folder-ready", "Other folder ready", "2026-06-18 08:00:00")
	insertFileFolderJoin(t, db, "file-ready", "folder-1")
	insertFileFolderJoin(t, db, "file-stored", "folder-1")
	insertFileFolderJoin(t, db, "file-other-folder-ready", "folder-2")
	insertDocumentAsset(t, db, "asset-ready", "file-ready", "ready", "2026-06-18 10:10:00")
	insertDocumentAsset(t, db, "asset-stored", "file-stored", "stored_only", "2026-06-18 09:10:00")
	insertDocumentAsset(t, db, "asset-other-ready", "file-other-folder-ready", "ready", "2026-06-18 08:10:00")

	repo := NewFileFolderRepository(db)
	files, total, err := repo.ListFilesInFolderWithFiltersAndTenant(
		context.Background(),
		"folder-1",
		1,
		20,
		"",
		"created_at_desc",
		"",
		"ready",
		nil,
		nil,
		"org-1",
		[]string{"workspace-1"},
	)
	if err != nil {
		t.Fatalf("list folder files: %v", err)
	}

	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].ID != "file-ready" {
		t.Fatalf("expected folder ready file only, got %q", files[0].ID)
	}
}

func TestApplyWorkspaceIDsFilterUsesTextComparisonForPostgres(t *testing.T) {
	db := openFileFolderPostgresMockDB(t)

	query := db.Session(&gorm.Session{DryRun: true}).Model(&file_model.UploadFile{})
	query = applyWorkspaceIDsFilter(query, []string{"a94d589e-9b61-4927-b965-5894426dc1f5"}, "workspace_id")
	query.Find(&[]file_model.UploadFile{})

	sql := query.Statement.SQL.String()
	if !strings.Contains(sql, "workspace_id::text = ANY(string_to_array(") {
		t.Fatalf("expected postgres text workspace filter, got SQL: %s", sql)
	}
	if strings.Contains(sql, "workspace_id IN") {
		t.Fatalf("postgres workspace filter must not use IN with uuid-like params, got SQL: %s", sql)
	}
}

func TestApplyCurrentAssetProductStatusFilterUsesTextComparisonForPostgres(t *testing.T) {
	db := openFileFolderPostgresMockDB(t)

	query := db.Session(&gorm.Session{DryRun: true}).Model(&file_model.UploadFile{})
	query = applyCurrentAssetProductStatusFilter(query, []string{"parse_failed"})
	query.Find(&[]file_model.UploadFile{})

	sql := query.Statement.SQL.String()
	for _, want := range []string{
		"dla.organization_id = upload_files.organization_id::text",
		"dla.source_file_id = upload_files.id::text",
		"latest_dla.organization_id = upload_files.organization_id::text",
		"latest_dla.source_file_id = upload_files.id::text",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected postgres asset status filter to contain %q, got SQL: %s", want, sql)
		}
	}
	if strings.Contains(sql, "source_file_id = upload_files.id\n") {
		t.Fatalf("postgres asset status filter must not compare varchar to uuid directly, got SQL: %s", sql)
	}
}

func TestFileFolderRepositoryFolderNameExistsScopesToSiblingDirectory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileFolderRepositoryTables(t, db)

	insertFileFolder(t, db, "root-a", "org-1", "workspace-1", "", "Reports")
	insertFileFolder(t, db, "root-b", "org-1", "workspace-1", "", "Archive")
	insertFileFolder(t, db, "child-a", "org-1", "workspace-1", "root-b", "Reports")

	repo := NewFileFolderRepository(db)
	workspaceID := "workspace-1"

	exists, err := repo.FolderNameExists(context.Background(), "org-1", &workspaceID, nil, "reports", nil)
	if err != nil {
		t.Fatalf("check root duplicate: %v", err)
	}
	if !exists {
		t.Fatalf("expected case-insensitive duplicate in same root directory")
	}

	parentID := "root-b"
	exists, err = repo.FolderNameExists(context.Background(), "org-1", &workspaceID, &parentID, "Reports", nil)
	if err != nil {
		t.Fatalf("check child duplicate: %v", err)
	}
	if !exists {
		t.Fatalf("expected duplicate under same parent")
	}

	otherParentID := "root-a"
	exists, err = repo.FolderNameExists(context.Background(), "org-1", &workspaceID, &otherParentID, "Reports", nil)
	if err != nil {
		t.Fatalf("check different parent: %v", err)
	}
	if exists {
		t.Fatalf("did not expect duplicate across different parent directories")
	}

	excludeID := "root-a"
	exists, err = repo.FolderNameExists(context.Background(), "org-1", &workspaceID, nil, "Reports", &excludeID)
	if err != nil {
		t.Fatalf("check self exclusion: %v", err)
	}
	if exists {
		t.Fatalf("did not expect self to count as duplicate")
	}
}

func TestFileFolderRepositoryListsRecursiveFolderDeleteScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileFolderRepositoryTables(t, db)

	insertFileFolder(t, db, "root", "org-1", "workspace-1", "", "Root")
	insertFileFolder(t, db, "child", "org-1", "workspace-1", "root", "Child")
	insertFileFolder(t, db, "grandchild", "org-1", "workspace-1", "child", "Grandchild")
	insertFileFolder(t, db, "other-workspace-child", "org-1", "workspace-2", "root", "Other Workspace Child")
	insertFileFolder(t, db, "other-org-child", "org-2", "workspace-1", "root", "Other Org Child")

	insertFileFolderJoin(t, db, "file-root", "root")
	insertFileFolderJoin(t, db, "file-child", "child")
	insertFileFolderJoin(t, db, "file-grandchild", "grandchild")
	insertFileFolderJoin(t, db, "file-other-workspace", "other-workspace-child")
	insertFileFolderJoin(t, db, "file-other-org", "other-org-child")

	repo := NewFileFolderRepository(db)
	folderIDs, err := repo.ListDescendantFolderIDs(context.Background(), "root")
	if err != nil {
		t.Fatalf("list descendant folder ids: %v", err)
	}
	assertStringSet(t, folderIDs, []string{"root", "child", "grandchild"})

	fileIDs, err := repo.ListFileIDsInFolders(context.Background(), folderIDs)
	if err != nil {
		t.Fatalf("list file ids in folders: %v", err)
	}
	assertStringSet(t, fileIDs, []string{"file-root", "file-child", "file-grandchild"})
}

func openFileFolderPostgresMockDB(t *testing.T) *gorm.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres mock: %v", err)
	}
	return db
}

func createFileFolderRepositoryTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE upload_files (
			id text primary key,
			organization_id text,
			workspace_id text,
			is_temporary boolean,
			is_archived boolean,
			archived_at datetime,
			archived_by text,
			storage_type text,
			key text,
			name text,
			size integer,
			extension text,
			mime_type text,
			created_by_role text,
			created_by text,
			created_at datetime,
			used boolean,
			used_by text,
			used_at datetime,
			hash text,
			source_url text,
			content_text text
		)`,
		`CREATE TABLE data_library_document_assets (
			id text primary key,
			organization_id text,
			source_file_id text,
			product_status text,
			updated_at datetime,
			deleted_at datetime
		)`,
		`CREATE TABLE file_folder_joins (
			file_id text,
			folder_id text,
			created_by text
		)`,
		`CREATE TABLE file_folders (
			id text primary key,
			organization_id text,
			workspace_id text,
			name text,
			description text,
			parent_id text,
			created_by text,
			created_at datetime,
			updated_by text,
			updated_at datetime,
			icon_type text,
			icon text,
			icon_background text,
			position integer,
			permission text
		)`,
	}
	for _, statement := range statements {
		execFileFolderRepositorySQL(t, db, statement)
	}
}

func insertUploadFile(t *testing.T, db *gorm.DB, id string, name string, createdAt string) {
	t.Helper()
	parsedCreatedAt, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		t.Fatalf("parse created at: %v", err)
	}
	execFileFolderRepositorySQL(
		t,
		db,
		`INSERT INTO upload_files (id, organization_id, workspace_id, is_archived, storage_type, key, name, size, extension, mime_type, created_by_role, created_by, created_at, used, hash, source_url)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		"org-1",
		"workspace-1",
		false,
		"local",
		id,
		name,
		100,
		"pdf",
		"application/pdf",
		"account",
		"account-1",
		parsedCreatedAt,
		false,
		id+"-hash",
		"",
	)
}

func insertDocumentAsset(t *testing.T, db *gorm.DB, id string, sourceFileID string, status string, updatedAt string) {
	t.Helper()
	parsedUpdatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAt)
	if err != nil {
		t.Fatalf("parse updated at: %v", err)
	}
	execFileFolderRepositorySQL(
		t,
		db,
		`INSERT INTO data_library_document_assets (id, organization_id, source_file_id, product_status, updated_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, NULL)`,
		id,
		"org-1",
		sourceFileID,
		status,
		parsedUpdatedAt,
	)
}

func insertFileFolderJoin(t *testing.T, db *gorm.DB, fileID string, folderID string) {
	t.Helper()
	execFileFolderRepositorySQL(
		t,
		db,
		`INSERT INTO file_folder_joins (file_id, folder_id, created_by) VALUES (?, ?, ?)`,
		fileID,
		folderID,
		"account-1",
	)
}

func insertFileFolder(t *testing.T, db *gorm.DB, id string, organizationID string, workspaceID string, parentID string, name string) {
	t.Helper()
	var parentValue any
	if parentID != "" {
		parentValue = parentID
	}

	execFileFolderRepositorySQL(
		t,
		db,
		`INSERT INTO file_folders (id, organization_id, workspace_id, name, parent_id, created_by, created_at, updated_at, position, permission)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?)`,
		id,
		organizationID,
		workspaceID,
		name,
		parentValue,
		"account-1",
		0,
		"only_me",
	)
}

func execFileFolderRepositorySQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}

func assertStringSet(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	seen := make(map[string]struct{}, len(got))
	for _, value := range got {
		seen[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := seen[value]; !ok {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
