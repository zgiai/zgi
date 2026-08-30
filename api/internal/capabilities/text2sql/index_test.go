package text2sql

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestIndexRepositoryEnsureReusesExistingIdentity(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewIndexRepository(db)
	first := testSchemaIndex("first-id", IndexFailed)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "text2sql_schema_indexes"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "text2sql_schema_indexes"`)).WillReturnRows(schemaIndexRows(first))
	if err := repo.Ensure(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := testSchemaIndex("second-id", IndexPending)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "text2sql_schema_indexes"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "text2sql_schema_indexes"`)).WillReturnRows(schemaIndexRows(first))
	if err := repo.Ensure(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Status != IndexFailed {
		t.Fatalf("Ensure returned %#v, want existing id/status %q/%q", second, first.ID, IndexFailed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func schemaIndexRows(row *SchemaIndex) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "organization_id", "workspace_id", "data_source_id", "asset_table_id", "physical_table_id", "schema_name", "physical_table_name", "fingerprint", "embedding_provider", "embedding_model", "collection_name", "vector_object_id", "status", "last_error_code", "indexed_at", "created_at", "updated_at"}).
		AddRow(row.ID, row.OrganizationID, row.WorkspaceID, row.DataSourceID, row.AssetTableID, row.PhysicalTableID, row.SchemaName, row.PhysicalTableName, row.Fingerprint, row.EmbeddingProvider, row.EmbeddingModel, row.CollectionName, row.VectorObjectID, row.Status, nil, nil, now, now)
}

func testSchemaIndex(id string, status IndexStatus) *SchemaIndex {
	return &SchemaIndex{
		ID: id, OrganizationID: "org", WorkspaceID: "workspace", DataSourceID: "source", AssetTableID: "asset",
		PhysicalTableID: 1, SchemaName: "public", PhysicalTableName: "orders", Fingerprint: "fingerprint",
		EmbeddingProvider: "qwen", EmbeddingModel: "text-embedding-v3", CollectionName: "collection", VectorObjectID: id + "-vector", Status: status,
	}
}

type indexSQLBase struct {
	sql_base.SQLBase
	table *sql_base.Table
}

func (s indexSQLBase) GetTable(context.Context, int) (*sql_base.Table, error) {
	return s.table, nil
}

type indexEmbedder struct{}

func (indexEmbedder) Embed(context.Context, string, string, string) ([]float64, error) {
	return []float64{0.1}, nil
}

type lifecycleVectorDB struct {
	vectordb.MockVectorDB
	storeErr       error
	deletedVectors []string
}

func (v *lifecycleVectorDB) StoreVector(context.Context, string, string, map[string]interface{}, []float64) error {
	return v.storeErr
}

func (v *lifecycleVectorDB) DeleteVector(_ context.Context, id, _ string) error {
	v.deletedVectors = append(v.deletedVectors, id)
	return nil
}

func TestIndexerReplacesReadyFingerprintAfterNewVersionIsReady(t *testing.T) {
	db, mock := newIndexLifecycleTestDB(t)
	table := &sql_base.Table{ID: 1, Schema: "public", Name: "orders", Columns: []sql_base.Column{{Name: "id", OrdinalPosition: 1}}}
	old := testSchemaIndex("old-id", IndexReady)
	old.Fingerprint = "old-fingerprint"
	current := testSchemaIndex("current-id", IndexPending)
	current.Fingerprint = schemaFingerprint(table)
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "text2sql_schema_indexes"`).WillReturnRows(schemaIndexRows(old))
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	vectors := &lifecycleVectorDB{}
	indexer := NewIndexer(NewIndexRepository(db), indexSQLBase{table: table}, indexEmbedder{}, vectors)

	if err := indexer.Process(context.Background(), current); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(vectors.deletedVectors) != 1 || vectors.deletedVectors[0] != old.VectorObjectID {
		t.Fatalf("deleted vectors = %#v, want [%q]", vectors.deletedVectors, old.VectorObjectID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIndexerKeepsOldReadyVersionWhenNewVectorWriteFails(t *testing.T) {
	db, mock := newIndexLifecycleTestDB(t)
	table := &sql_base.Table{ID: 1, Schema: "public", Name: "orders", Columns: []sql_base.Column{{Name: "id", OrdinalPosition: 1}}}
	old := testSchemaIndex("old-id", IndexReady)
	old.Fingerprint = "old-fingerprint"
	current := testSchemaIndex("current-id", IndexPending)
	current.Fingerprint = schemaFingerprint(table)
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	vectors := &lifecycleVectorDB{storeErr: errors.New("store failed")}
	indexer := NewIndexer(NewIndexRepository(db), indexSQLBase{table: table}, indexEmbedder{}, vectors)

	if err := indexer.Process(context.Background(), current); err == nil {
		t.Fatal("Process() error = nil, want vector store error")
	}
	if len(vectors.deletedVectors) != 0 {
		t.Fatalf("deleted vectors = %#v, want none", vectors.deletedVectors)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIndexerRetriesDeletingSupersededVersionForReadyCurrent(t *testing.T) {
	db, mock := newIndexLifecycleTestDB(t)
	current := testSchemaIndex("current-id", IndexReady)
	old := testSchemaIndex("old-id", IndexDeleting)
	old.Fingerprint = "old-fingerprint"
	mock.ExpectQuery(`SELECT \* FROM "text2sql_schema_indexes"`).WillReturnRows(schemaIndexRows(old))
	mock.ExpectExec(`DELETE FROM "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	vectors := &lifecycleVectorDB{}
	indexer := NewIndexer(NewIndexRepository(db), indexSQLBase{}, indexEmbedder{}, vectors)

	if err := indexer.Process(context.Background(), current); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(vectors.deletedVectors) != 1 || vectors.deletedVectors[0] != old.VectorObjectID {
		t.Fatalf("deleted vectors = %#v, want [%q]", vectors.deletedVectors, old.VectorObjectID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newIndexLifecycleTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}
