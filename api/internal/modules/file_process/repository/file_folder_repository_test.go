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
			is_archived boolean,
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
			hash text,
			source_url text
		)`,
		`CREATE TABLE data_library_document_assets (
			id text primary key,
			organization_id text,
			source_file_id text,
			product_status text,
			updated_at datetime,
			deleted_at datetime
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

func execFileFolderRepositorySQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}
