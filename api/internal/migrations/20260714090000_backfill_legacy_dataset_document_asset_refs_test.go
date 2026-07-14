package migrations

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBackfillLegacyDatasetDocumentAssetRefsMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectBegin()
	for range 7 {
		mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery(regexp.QuoteMeta(strings.TrimSpace(countUnlinkedLegacyDatasetDocumentsSQL))).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectCommit()

	if err := upBackfillLegacyDatasetDocumentAssetRefs(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillLegacyDatasetDocumentAssetRefsRollsBackWhenDocumentsRemainUnlinked(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectBegin()
	for range 7 {
		mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery(regexp.QuoteMeta(strings.TrimSpace(countUnlinkedLegacyDatasetDocumentsSQL))).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	err := upBackfillLegacyDatasetDocumentAssetRefs(mschema.New(db))
	if err == nil || !strings.Contains(err.Error(), "left 1 documents without exactly one active ref") {
		t.Fatalf("expected verification failure, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillLegacyDatasetDocumentAssetRefsIsInsertOnlyForPersistentBusinessTables(t *testing.T) {
	allSQL := strings.ToUpper(strings.Join([]string{
		createLegacyDatasetDocumentBackfillPlanSQL,
		createLegacyDatasetDocumentBackfillPlanIndexSQL,
		insertLegacyDatasetDocumentCanonicalAssetsSQL,
		createLegacyDatasetDocumentResolvedPlanSQL,
		createLegacyDatasetDocumentResolvedPlanIndexSQL,
		insertLegacyDatasetDocumentPlaceholderAssetsSQL,
		insertLegacyDatasetDocumentAssetRefsSQL,
	}, "\n"))

	for _, forbidden := range []string{
		"UPDATE PUBLIC.DOCUMENTS",
		"DELETE FROM PUBLIC.DOCUMENTS",
		"UPDATE PUBLIC.DOCUMENT_SEGMENTS",
		"DELETE FROM PUBLIC.DOCUMENT_SEGMENTS",
		"TRUNCATE",
	} {
		if strings.Contains(allSQL, forbidden) {
			t.Fatalf("migration must not contain destructive legacy-table operation %q", forbidden)
		}
	}
	for _, required := range []string{
		"INSERT INTO PUBLIC.DATA_LIBRARY_DOCUMENT_ASSETS",
		"INSERT INTO PUBLIC.DATA_LIBRARY_KNOWLEDGE_BASE_ASSET_REFS",
		"ON CONFLICT DO NOTHING",
		"'SYNCED'",
		"'LEGACY_PLACEHOLDER', TRUE",
		"DOC_METADATA ->> 'SOURCE_FILE_ID'",
		`"UPLOAD_FILE_ID"`,
	} {
		if !strings.Contains(allSQL, required) {
			t.Fatalf("migration is missing safety or compatibility marker %q", required)
		}
	}
}

func TestBackfillLegacyDatasetDocumentAssetRefsAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL migration integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err := RunWithDB(db); err != nil {
		t.Fatalf("prepare migrated schema: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	const (
		accountID      = "10000000-0000-0000-0000-000000000001"
		orgID          = "10000000-0000-0000-0000-000000000002"
		workspaceID    = "10000000-0000-0000-0000-000000000003"
		datasetID      = "10000000-0000-0000-0000-000000000004"
		sourceFileID   = "10000000-0000-0000-0000-000000000005"
		linkedDocID    = "10000000-0000-0000-0000-000000000006"
		duplicateDocID = "10000000-0000-0000-0000-000000000007"
		missingDocID   = "10000000-0000-0000-0000-000000000008"
		missingFileID  = "10000000-0000-0000-0000-000000000009"
	)

	fixtures := []struct {
		sql  string
		args []any
	}{
		{
			`INSERT INTO public.accounts (id, name, email)
			 VALUES (?, 'Migration Test Account', 'migration-test@example.invalid')`,
			[]any{accountID},
		},
		{
			`INSERT INTO public.organizations (id, name)
			 VALUES (?, 'Migration Test Organization')`,
			[]any{orgID},
		},
		{
			`INSERT INTO public.workspaces (id, name, organization_id)
			 VALUES (?, 'Migration Test Workspace', ?)`,
			[]any{workspaceID, orgID},
		},
		{
			`INSERT INTO public.datasets (id, workspace_id, organization_id, name, created_by)
			 VALUES (?, ?, ?, 'Migration Test Dataset', ?)`,
			[]any{datasetID, workspaceID, orgID, accountID},
		},
		{
			`INSERT INTO public.upload_files (
				id, organization_id, workspace_id, storage_type, key, name, size, extension, created_by, hash
			 ) VALUES (?, ?, ?, 'local', 'migration/source.pdf', 'source.pdf', 10, 'pdf', ?, 'source-hash')`,
			[]any{sourceFileID, orgID, workspaceID, accountID},
		},
		{
			`INSERT INTO public.documents (
				id, organization_id, dataset_id, position, data_source_type, batch, name,
				created_from, created_by, file_id, indexing_status, completed_at
			 ) VALUES
				(?, ?, ?, 1, 'upload_file', 'migration', 'source.pdf', 'api', ?, ?, 'completed', now()),
				(?, ?, ?, 2, 'upload_file', 'migration', 'source-copy.pdf', 'api', ?, ?, 'completed', now()),
				(?, ?, ?, 3, 'upload_file', 'migration', 'missing.pdf', 'api', ?, ?, 'completed', now())`,
			[]any{
				linkedDocID, orgID, datasetID, accountID, sourceFileID,
				duplicateDocID, orgID, datasetID, accountID, sourceFileID,
				missingDocID, orgID, datasetID, accountID, missingFileID,
			},
		},
	}
	for _, fixture := range fixtures {
		if err := tx.Exec(fixture.sql, fixture.args...).Error; err != nil {
			t.Fatalf("insert migration fixture: %v", err)
		}
	}

	if err := upBackfillLegacyDatasetDocumentAssetRefs(mschema.New(tx)); err != nil {
		t.Fatalf("run migration against fixtures: %v", err)
	}

	var refCount int64
	if err := tx.Table("data_library_knowledge_base_asset_refs").
		Where("dataset_id = ? AND deleted_at IS NULL", datasetID).
		Count(&refCount).Error; err != nil {
		t.Fatalf("count restored refs: %v", err)
	}
	if refCount != 3 {
		t.Fatalf("expected 3 restored refs, got %d", refCount)
	}

	type restoredRef struct {
		DocumentID     string
		SourceFileID   string
		Placeholder    bool
		PlaceholderWhy string
	}
	var restored []restoredRef
	if err := tx.Raw(`
		SELECT
			refs.dataset_document_id::text AS document_id,
			assets.source_file_id,
			COALESCE((assets.metadata_json ->> 'legacy_placeholder')::boolean, false) AS placeholder,
			COALESCE(assets.metadata_json ->> 'placeholder_reason', '') AS placeholder_why
		FROM public.data_library_knowledge_base_asset_refs AS refs
		JOIN public.data_library_document_assets AS assets ON assets.id = refs.asset_id
		WHERE refs.dataset_id = ?
		ORDER BY refs.dataset_document_id
	`, datasetID).Scan(&restored).Error; err != nil {
		t.Fatalf("read restored refs: %v", err)
	}

	byDocument := make(map[string]restoredRef, len(restored))
	for _, item := range restored {
		byDocument[item.DocumentID] = item
	}
	if got := byDocument[linkedDocID]; got.SourceFileID != sourceFileID || got.Placeholder {
		t.Fatalf("source-backed document was not linked to its real file asset: %+v", got)
	}
	if got := byDocument[duplicateDocID]; !got.Placeholder || got.PlaceholderWhy != "duplicate_dataset_file" {
		t.Fatalf("duplicate source document did not receive a safe placeholder: %+v", got)
	}
	if got := byDocument[missingDocID]; !got.Placeholder || got.PlaceholderWhy != "missing_source_file" {
		t.Fatalf("missing source document did not receive a placeholder: %+v", got)
	}

	// The test wraps both runs in one outer transaction so fixtures can be rolled
	// back. In production ON COMMIT DROP removes these tables after the first run.
	for _, table := range []string{
		"legacy_dataset_document_asset_ref_resolved",
		"legacy_dataset_document_asset_ref_plan",
	} {
		if err := tx.Exec("DROP TABLE " + table).Error; err != nil {
			t.Fatalf("reset transaction-local migration plan: %v", err)
		}
	}
	if err := upBackfillLegacyDatasetDocumentAssetRefs(mschema.New(tx)); err != nil {
		t.Fatalf("rerun idempotent migration: %v", err)
	}
	var rerunRefCount int64
	if err := tx.Table("data_library_knowledge_base_asset_refs").
		Where("dataset_id = ? AND deleted_at IS NULL", datasetID).
		Count(&rerunRefCount).Error; err != nil {
		t.Fatalf("count refs after rerun: %v", err)
	}
	if rerunRefCount != refCount {
		t.Fatalf("idempotent rerun changed ref count from %d to %d", refCount, rerunRefCount)
	}
}
