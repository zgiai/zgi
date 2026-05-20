package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do/v2"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/tables"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/service"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

const tableNamePrefix = "zgi_base_"

func TestTableLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Hour)
	defer cancel()

	cfg := driver.Config{
		DBHost: "localhost",
		DBPort: "5432",
		DBUser: "postgres",
		DBPass: "Abc1234",
		DBName: "postgres",
	}

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

	schemaSvc := do.MustInvoke[service.SchemasService](injector)
	tableSvc := do.MustInvoke[service.TablesService](injector)

	testSchema := "sqlmeta_table_schema_" + uuid.NewString()[:8]
	schemaInput := types.SchemaCreateInput{Name: testSchema, Owner: "postgres"}
	createdSchema, err := schemaSvc.Create(ctx, schemaInput)
	if err != nil {
		t.Fatalf("create schema for table test: %v", err)
	}
	t.Cleanup(func() {
		_, _ = schemaSvc.Delete(context.Background(), createdSchema.ID, types.SchemaDropOptions{Cascade: true})
	})

	baseTableName := "sqlmeta_table_" + uuid.NewString()[:8]
	tableInput := types.TableCreateInput{Name: baseTableName, Schema: testSchema}
	createdTable, err := tableSvc.Create(ctx, tableInput)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	expectedName := tableNamePrefix + baseTableName
	if createdTable == nil || createdTable.Name != expectedName {
		t.Fatalf("unexpected create result: %#v", createdTable)
	}

	listOpts := types.TableListOptions{
		IncludedSchemas: []string{testSchema},
		IncludeColumns:  true,
	}
	tablesList, err := tableSvc.List(ctx, listOpts)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	found := false
	for _, tbl := range tablesList {
		if tbl.Name == expectedName && tbl.Schema == testSchema {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created table %s not found in list", baseTableName)
	}

	fetchedByName, err := tableSvc.Retrieve(ctx, types.TableIdentifier{Schema: testSchema, Name: baseTableName}, true)
	if err != nil {
		t.Fatalf("retrieve table by name: %v", err)
	}
	if fetchedByName.ID != createdTable.ID {
		t.Fatalf("expected same ID, got %d vs %d", fetchedByName.ID, createdTable.ID)
	}

	newComment := "updated comment"
	newNameBase := baseTableName + "_renamed"
	updateInput := types.TableUpdateInput{Name: &newNameBase, Comment: &newComment}
	updatedTable, err := tableSvc.Update(ctx, createdTable.ID, updateInput)
	if err != nil {
		t.Fatalf("update table: %v", err)
	}
	expectedRenamed := tableNamePrefix + newNameBase
	if updatedTable.Name != expectedRenamed {
		t.Fatalf("expected renamed table %s, got %s", expectedRenamed, updatedTable.Name)
	}
	if updatedTable.Comment == nil || *updatedTable.Comment != newComment {
		t.Fatalf("table comment not updated")
	}

	deleted, err := tableSvc.Delete(ctx, createdTable.ID, types.TableDeleteOptions{Cascade: false})
	if err != nil {
		t.Fatalf("delete table: %v", err)
	}
	if deleted.Name != expectedRenamed {
		t.Fatalf("delete returned unexpected table name: %s", deleted.Name)
	}

	if _, err := tableSvc.Retrieve(ctx, types.TableIdentifier{ID: createdTable.ID}, false); err == nil {
		t.Fatalf("expected error retrieving deleted table")
	}
}
