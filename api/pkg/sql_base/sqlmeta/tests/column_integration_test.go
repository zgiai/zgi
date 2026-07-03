package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do/v2"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/columns"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/tables"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/service"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

func TestColumnLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Hour)
	defer cancel()

	cfg := testDBConfig()

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, schemas.ProvideRepository)
	do.Provide(injector, service.ProvideSchemasService)
	do.Provide(injector, tables.ProvideRepository)
	do.Provide(injector, service.ProvideTablesService)
	do.Provide(injector, columns.ProvideRepository)
	do.Provide(injector, service.ProvideColumnsService)

	schemaSvc := do.MustInvoke[service.SchemasService](injector)
	tableSvc := do.MustInvoke[service.TablesService](injector)
	columnSvc := do.MustInvoke[service.ColumnsService](injector)

	testSchema := "sqlmeta_column_schema_" + uuid.NewString()[:8]
	schemaInput := types.SchemaCreateInput{Name: testSchema, Owner: "postgres"}
	createdSchema, err := schemaSvc.Create(ctx, schemaInput)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = schemaSvc.Delete(context.Background(), createdSchema.ID, types.SchemaDropOptions{Cascade: true})
	})

	baseTableName := "sqlmeta_column_table_" + uuid.NewString()[:8]
	tableInput := types.TableCreateInput{Name: baseTableName, Schema: testSchema}
	createdTable, err := tableSvc.Create(ctx, tableInput)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	nullable := true
	colInput := types.ColumnCreateInput{
		TableID:            createdTable.ID,
		Name:               "notes",
		Type:               "text",
		DefaultValue:       "initial",
		DefaultValueSet:    true,
		DefaultValueFormat: "literal",
		IsNullable:         &nullable,
	}

	createdColumn, err := columnSvc.CreateColumn(ctx, colInput)
	if err != nil {
		t.Fatalf("create column: %v", err)
	}
	if createdColumn.Name != colInput.Name {
		t.Fatalf("expected column name %s, got %s", colInput.Name, createdColumn.Name)
	}
	if createdColumn.Schema != testSchema {
		t.Fatalf("expected column schema %s, got %s", testSchema, createdColumn.Schema)
	}

	listOpts := types.ColumnListOptions{
		TableID: createdTable.ID,
		Pagination: types.Pagination{
			Limit:  10,
			Offset: 0,
		},
	}
	listedColumns, err := columnSvc.ListColumns(ctx, listOpts)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	if len(listedColumns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(listedColumns))
	}

	fetchedByID, err := columnSvc.GetColumn(ctx, types.ColumnIdentifier{ID: createdColumn.ID})
	if err != nil {
		t.Fatalf("retrieve column by id: %v", err)
	}
	if fetchedByID.ID != createdColumn.ID {
		t.Fatalf("expected same column id, got %s vs %s", fetchedByID.ID, createdColumn.ID)
	}

	updatedName := "notes_updated"
	nullable = false
	unique := true
	comment := "important field"
	updateInput := types.ColumnUpdateInput{
		Name:                  &updatedName,
		DefaultValue:          "updated",
		DefaultValueSet:       true,
		DefaultValueFormat:    "literal",
		DefaultValueFormatSet: true,
		IsNullable:            &nullable,
		IsUnique:              &unique,
		Comment:               &comment,
		CommentSet:            true,
	}

	updatedColumn, err := columnSvc.UpdateColumn(ctx, createdColumn.ID, updateInput)
	if err != nil {
		t.Fatalf("update column: %v", err)
	}
	if updatedColumn.Name != updatedName {
		t.Fatalf("expected column renamed to %s, got %s", updatedName, updatedColumn.Name)
	}
	if updatedColumn.IsNullable != false {
		t.Fatalf("expected column to be not null")
	}
	if !updatedColumn.IsUnique {
		t.Fatalf("expected column to be unique")
	}
	if updatedColumn.Comment == nil || *updatedColumn.Comment != comment {
		t.Fatalf("column comment not updated")
	}
	if updatedColumn.DefaultValue != "'updated'::text" {
		t.Fatalf("expected default expression %q, got %#v", "'updated'::text", updatedColumn.DefaultValue)
	}

	fetchedByName, err := columnSvc.GetColumn(ctx, types.ColumnIdentifier{
		Schema: testSchema,
		Table:  createdTable.Name,
		Name:   updatedName,
	})
	if err != nil {
		t.Fatalf("retrieve column by name: %v", err)
	}
	if fetchedByName.ID != createdColumn.ID {
		t.Fatalf("expected same column id when fetching by name")
	}

	deletedColumn, err := columnSvc.DeleteColumn(ctx, createdColumn.ID, false)
	if err != nil {
		t.Fatalf("delete column: %v", err)
	}
	if deletedColumn.Name != updatedName {
		t.Fatalf("expected deleted column name %s, got %s", updatedName, deletedColumn.Name)
	}

	if _, err := columnSvc.GetColumn(ctx, types.ColumnIdentifier{ID: createdColumn.ID}); err == nil {
		t.Fatalf("expected error retrieving deleted column")
	}

	remaining, err := columnSvc.ListColumns(ctx, listOpts)
	if err != nil {
		t.Fatalf("list columns after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no columns after delete, found %d", len(remaining))
	}
}

func TestColumnTypeMappings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Hour)
	defer cancel()

	cfg := testDBConfig()

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, schemas.ProvideRepository)
	do.Provide(injector, service.ProvideSchemasService)
	do.Provide(injector, tables.ProvideRepository)
	do.Provide(injector, service.ProvideTablesService)
	do.Provide(injector, columns.ProvideRepository)
	do.Provide(injector, service.ProvideColumnsService)

	schemaSvc := do.MustInvoke[service.SchemasService](injector)
	tableSvc := do.MustInvoke[service.TablesService](injector)
	columnSvc := do.MustInvoke[service.ColumnsService](injector)

	testSchema := "sqlmeta_column_type_schema_" + uuid.NewString()[:8]
	schemaInput := types.SchemaCreateInput{Name: testSchema, Owner: "postgres"}
	createdSchema, err := schemaSvc.Create(ctx, schemaInput)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Log(createdSchema)
	//t.Cleanup(func() {
	//	_, _ = schemaSvc.Delete(context.Background(), createdSchema.ID, types.SchemaDropOptions{Cascade: true})
	//})

	baseTableName := "sqlmeta_column_type_table_" + uuid.NewString()[:8]
	tableInput := types.TableCreateInput{Name: baseTableName, Schema: testSchema}
	createdTable, err := tableSvc.Create(ctx, tableInput)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	type typeExpectation struct {
		Name           string
		ColType        string
		ExpectedData   string
		ExpectedFormat string
	}

	typeCases := []typeExpectation{
		{Name: "col_bigint", ColType: "bigint", ExpectedData: "bigint", ExpectedFormat: "int8"},
		{Name: "col_uuid", ColType: "uuid", ExpectedData: "uuid", ExpectedFormat: "uuid"},
		{Name: "col_timestamp", ColType: "timestamp", ExpectedData: "timestamp without time zone", ExpectedFormat: "timestamp"},
		{Name: "col_boolean", ColType: "boolean", ExpectedData: "boolean", ExpectedFormat: "bool"},
		{Name: "col_text", ColType: "text", ExpectedData: "text", ExpectedFormat: "text"},
		{Name: "col_numeric", ColType: "numeric", ExpectedData: "numeric", ExpectedFormat: "numeric"},
		{Name: "col_integer", ColType: "integer", ExpectedData: "integer", ExpectedFormat: "int4"},
	}

	for _, tc := range typeCases {
		createdTypedColumn, err := columnSvc.CreateColumn(ctx, types.ColumnCreateInput{
			TableID: createdTable.ID,
			Name:    tc.Name,
			Type:    tc.ColType,
		})
		if err != nil {
			t.Fatalf("create column %s of type %s: %v", tc.Name, tc.ColType, err)
		}
		if createdTypedColumn.DataType != tc.ExpectedData {
			t.Fatalf("expected data type %s for column %s, got %s", tc.ExpectedData, tc.Name, createdTypedColumn.DataType)
		}
		if createdTypedColumn.Format != tc.ExpectedFormat {
			t.Fatalf("expected format %s for column %s, got %s", tc.ExpectedFormat, tc.Name, createdTypedColumn.Format)
		}
	}
}

func TestListColumnsByTableName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Hour)
	defer cancel()

	cfg := testDBConfig()

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, schemas.ProvideRepository)
	do.Provide(injector, service.ProvideSchemasService)
	do.Provide(injector, tables.ProvideRepository)
	do.Provide(injector, service.ProvideTablesService)
	do.Provide(injector, columns.ProvideRepository)
	do.Provide(injector, service.ProvideColumnsService)

	schemaSvc := do.MustInvoke[service.SchemasService](injector)
	tableSvc := do.MustInvoke[service.TablesService](injector)
	columnSvc := do.MustInvoke[service.ColumnsService](injector)

	testSchema := "sqlmeta_column_list_schema_" + uuid.NewString()[:8]
	schemaInput := types.SchemaCreateInput{Name: testSchema, Owner: "postgres"}
	createdSchema, err := schemaSvc.Create(ctx, schemaInput)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = schemaSvc.Delete(context.Background(), createdSchema.ID, types.SchemaDropOptions{Cascade: true})
	})

	baseTableName := "sqlmeta_column_list_table_" + uuid.NewString()[:8]
	tableInput := types.TableCreateInput{Name: baseTableName, Schema: testSchema}
	createdTable, err := tableSvc.Create(ctx, tableInput)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	type typeCase struct {
		Name string
		Type string
	}

	columnsToCreate := []typeCase{
		{Name: "col_bigint", Type: "bigint"},
		{Name: "col_uuid", Type: "uuid"},
		{Name: "col_timestamp", Type: "timestamp"},
		{Name: "col_boolean", Type: "boolean"},
		{Name: "col_text", Type: "text"},
		{Name: "col_numeric", Type: "numeric"},
		{Name: "col_integer", Type: "integer"},
	}

	for _, col := range columnsToCreate {
		if _, err := columnSvc.CreateColumn(ctx, types.ColumnCreateInput{
			TableID: createdTable.ID,
			Name:    col.Name,
			Type:    col.Type,
		}); err != nil {
			t.Fatalf("create column %s: %v", col.Name, err)
		}
	}

	tableMeta, err := tableSvc.Retrieve(ctx, types.TableIdentifier{Schema: testSchema, Name: baseTableName}, false)
	if err != nil {
		t.Fatalf("retrieve table by name: %v", err)
	}

	listOpts := types.ColumnListOptions{TableID: tableMeta.ID}
	columnsList, err := columnSvc.ListColumns(ctx, listOpts)
	if err != nil {
		t.Fatalf("list columns by table: %v", err)
	}

	if len(columnsList) != len(columnsToCreate) {
		t.Fatalf("expected %d columns, got %d", len(columnsToCreate), len(columnsList))
	}

	seen := make(map[string]struct{}, len(columnsList))
	for _, col := range columnsList {
		seen[col.Name] = struct{}{}
	}
	for _, col := range columnsToCreate {
		if _, ok := seen[col.Name]; !ok {
			t.Fatalf("expected column %s to be present in listing", col.Name)
		}
	}
}
