package text2sql

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/pkg/sql_base"
)

type snapshotTestSQL struct {
	sql_base.SQLBase
	table *sql_base.Table
}

func (s snapshotTestSQL) GetTable(context.Context, int) (*sql_base.Table, error) {
	return s.table, nil
}

func TestTableMetadataChangesSchemaFingerprint(t *testing.T) {
	physical := &sql_base.Table{ID: 1, Schema: "public", Name: "physical_table"}
	sql := snapshotTestSQL{table: physical}
	before, err := loadIndexedTable(context.Background(), sql, indexedTableAsset{TableID: 1, Name: "Orders", Description: "Customer orders"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := loadIndexedTable(context.Background(), sql, indexedTableAsset{TableID: 1, Name: "Archived orders", Description: "Historical customer orders"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaFingerprint(before) == schemaFingerprint(after) {
		t.Fatal("table name and description did not change the schema fingerprint")
	}
	if physical.Comment != nil {
		t.Fatal("index snapshot mutated the physical table")
	}
}

func TestCompleteIndexesRejectsPartialAndHistoricalCatalogs(t *testing.T) {
	tables := []IndexedTable{{"a", &sql_base.Table{ID: 1, Name: "one"}}, {"b", &sql_base.Table{ID: 2, Name: "two"}}}
	a := NewSchemaIndex("org", "ws", "ds", "a", tables[0].Table, "p", "m")
	a.Status = IndexReady
	b := NewSchemaIndex("org", "ws", "ds", "b", tables[1].Table, "p", "m")
	b.Status = IndexReady
	if _, ok := CompleteIndexes(tables, []SchemaIndex{*a}); ok {
		t.Fatal("partial catalog accepted")
	}
	if rows, ok := CompleteIndexes(tables, []SchemaIndex{*b}); ok || len(rows) != 1 {
		t.Fatal("partial catalog must count ready tables after a missing table")
	}
	b.Fingerprint = "old"
	if _, ok := CompleteIndexes(tables, []SchemaIndex{*a, *b}); ok {
		t.Fatal("historical schema accepted")
	}
	b.Fingerprint = schemaFingerprint(tables[1].Table)
	if rows, ok := CompleteIndexes(tables, []SchemaIndex{*a, *a, *b}); !ok || len(rows) != 2 {
		t.Fatal("current catalog rejected or duplicated")
	}
	if _, ok := CompleteIndexes(nil, nil); ok {
		t.Fatal("empty catalog accepted")
	}
}

func TestIndexFailurePreservesOriginalCauseWhenPersistenceFails(t *testing.T) {
	db, mock := newIndexLifecycleTestDB(t)
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnError(errors.New("database unavailable"))
	cause := errors.New("original embedding failure")
	indexer := NewIndexer(NewIndexRepository(db), nil, nil, nil)
	err := indexer.fail(context.Background(), testSchemaIndex("id", IndexIndexing), "embedding_failed", cause)
	if !errors.Is(err, cause) {
		t.Fatalf("original cause lost: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIndexFailurePersistsAfterRequestCancellation(t *testing.T) {
	db, mock := newIndexLifecycleTestDB(t)
	mock.ExpectExec(`UPDATE "text2sql_schema_indexes"`).WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewIndexer(NewIndexRepository(db), nil, nil, nil).fail(ctx, testSchemaIndex("id", IndexIndexing), "embedding_failed", context.DeadlineExceeded)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
