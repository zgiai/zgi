package text2sql

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type recordingCatalogSQLBase struct {
	sql_base.SQLBase
	requestedID int
	table       *sql_base.Table
}

func (f *recordingCatalogSQLBase) GetTable(_ context.Context, id int) (*sql_base.Table, error) {
	f.requestedID = id
	return f.table, nil
}

func TestSQLCatalogLoaderUsesServerMetadataForMappedTable(t *testing.T) {
	db, mock, closeDB := newCatalogTestDB(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "data_sources"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ds-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, description FROM "data_source_tables"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "table_id", "description"}).AddRow("table-1", 7, "营销域campaigns业务表"))
	expectCatalogValueSamples(mock)
	sqlClient := &recordingCatalogSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "customers"}}
	loader := NewSQLCatalogLoader(db, sqlClient)

	catalog, err := loader.Load(context.Background(), CatalogRequest{
		Scope: Scope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}, DataSourceID: "ds-1",
		Tables: []TableRef{{AssetID: "table-1", PhysicalID: 7, Schema: "forged", Name: "forged"}},
	})
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if sqlClient.requestedID != 7 {
		t.Fatalf("GetTable id = %d, want server-mapped id 7", sqlClient.requestedID)
	}
	if len(catalog.Tables) != 1 || catalog.Tables[0].Ref.PhysicalID != 7 || catalog.Tables[0].Ref.Name != "customers" {
		t.Fatalf("catalog = %#v", catalog)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLCatalogLoaderSupportsLegacyPhysicalIDWithinDataSource(t *testing.T) {
	db, mock, closeDB := newCatalogTestDB(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "data_sources"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ds-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, description FROM "data_source_tables"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "table_id", "description"}).AddRow("table-1", 7, "营销域campaigns业务表"))
	expectCatalogValueSamples(mock)
	sqlClient := &recordingCatalogSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "customers"}}
	loader := NewSQLCatalogLoader(db, sqlClient)

	catalog, err := loader.Load(context.Background(), CatalogRequest{
		Scope: Scope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}, DataSourceID: "ds-1",
		Tables: []TableRef{{PhysicalID: 7, Name: "customers"}},
	})
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if len(catalog.Tables) != 1 || catalog.Tables[0].Ref.AssetID != "table-1" {
		t.Fatalf("catalog = %#v, want server business table id", catalog)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLCatalogLoaderRejectsTableOutsideDataSource(t *testing.T) {
	db, mock, closeDB := newCatalogTestDB(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "data_sources"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ds-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, description FROM "data_source_tables"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "table_id", "description"}))
	loader := NewSQLCatalogLoader(db, &recordingCatalogSQLBase{})

	_, err := loader.Load(context.Background(), CatalogRequest{
		Scope: Scope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}, DataSourceID: "ds-1",
		Tables: []TableRef{{AssetID: "foreign-table", PhysicalID: 7}},
	})
	if !errors.Is(err, ErrTableOutOfScope) {
		t.Fatalf("Load error = %v, want ErrTableOutOfScope", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLCatalogLoaderPrefersBusinessIDWhenPhysicalIDIsStale(t *testing.T) {
	db, mock, closeDB := newCatalogTestDB(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "data_sources"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ds-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, description FROM "data_source_tables"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "table_id", "description"}).AddRow("table-1", 7, "营销域campaigns业务表"))
	expectCatalogValueSamples(mock)
	sqlClient := &recordingCatalogSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "customers"}}
	loader := NewSQLCatalogLoader(db, sqlClient)

	catalog, err := loader.Load(context.Background(), CatalogRequest{
		Scope: Scope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}, DataSourceID: "ds-1",
		Tables: []TableRef{{AssetID: "table-1", PhysicalID: 999}},
	})
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if sqlClient.requestedID != 7 || len(catalog.Tables) != 1 || catalog.Tables[0].Ref.PhysicalID != 7 {
		t.Fatalf("catalog = %#v, requested physical id = %d; want current server mapping 7", catalog, sqlClient.requestedID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLCatalogLoaderCarriesBusinessDescription(t *testing.T) {
	db, mock, closeDB := newCatalogTestDB(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "data_sources"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ds-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, description FROM "data_source_tables"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "table_id", "description"}).
			AddRow("table-1", 7, "营销域campaigns业务表").
			AddRow("table-2", 8, nil))
	expectCatalogValueSamples(mock)
	comment := "Table created for organization org, data source ds"
	sqlClient := &catalogStubSQLBase{tables: map[int]*sql_base.Table{
		7: {ID: 7, Schema: "public", Name: "marketing_campaigns"},
		8: {ID: 8, Schema: "public", Name: "orders", Comment: &comment},
	}}
	loader := NewSQLCatalogLoader(db, sqlClient)

	catalog, err := loader.Load(context.Background(), CatalogRequest{
		Scope: Scope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}, DataSourceID: "ds-1",
		Tables: []TableRef{{AssetID: "table-1", PhysicalID: 7}, {AssetID: "table-2", PhysicalID: 8}},
	})
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if got := catalog.Tables[0].Description; got != "营销域campaigns业务表" {
		t.Fatalf("Description = %q, want the operator description", got)
	}
	if got := catalog.Tables[0].BusinessDescription(); got != "营销域campaigns业务表" {
		t.Fatalf("BusinessDescription() = %q, want the operator description", got)
	}
	// A nil description must fall back to the physical comment rather than
	// rendering an empty label.
	if got := catalog.Tables[1].BusinessDescription(); got != comment {
		t.Fatalf("BusinessDescription() = %q, want physical comment fallback", got)
	}
}

func TestSQLCatalogLoaderLoadsValueSamples(t *testing.T) {
	db, mock, closeDB := newCatalogTestDB(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "data_sources"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ds-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, description FROM "data_source_tables"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "table_id", "description"}).AddRow("table-1", 7, nil))
	mock.ExpectQuery(`SELECT .*idx\.asset_table_id, idx\.value_samples FROM text2sql_schema_indexes AS idx JOIN database_schema_index_settings AS settings`).
		WillReturnRows(sqlmock.NewRows([]string{"asset_table_id", "value_samples"}).AddRow("table-1", datatypes.JSON([]byte(`{"v":1,"columns":{"campaign_status":["active","completed"]}}`))))
	loader := NewSQLCatalogLoader(db, &recordingCatalogSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "campaigns"}})

	catalog, err := loader.Load(context.Background(), CatalogRequest{
		Scope: Scope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}, DataSourceID: "ds-1",
		Tables: []TableRef{{AssetID: "table-1", PhysicalID: 7}},
	})
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if got := catalog.Tables[0].ValueSamples["campaign_status"]; len(got) != 2 || got[1] != "completed" {
		t.Fatalf("ValueSamples = %#v, want campaign status values", catalog.Tables[0].ValueSamples)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type catalogStubSQLBase struct {
	sql_base.SQLBase
	tables map[int]*sql_base.Table
}

func (f *catalogStubSQLBase) GetTable(_ context.Context, id int) (*sql_base.Table, error) {
	return f.tables[id], nil
}

func newCatalogTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		sqlDB.Close()
		t.Fatal(err)
	}
	return db, mock, func() { _ = sqlDB.Close() }
}

func expectCatalogValueSamples(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .*idx\.asset_table_id, idx\.value_samples FROM text2sql_schema_indexes AS idx JOIN database_schema_index_settings AS settings`).
		WillReturnRows(sqlmock.NewRows([]string{"asset_table_id", "value_samples"}))
}
