package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/samber/do/v2"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/indexes"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/tables"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/service"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

func TestIndexesListAndRetrieve(t *testing.T) {
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
	do.Provide(injector, indexes.ProvideRepository)
	do.Provide(injector, service.ProvideIndexesService)

	schemaSvc := do.MustInvoke[service.SchemasService](injector)
	tableSvc := do.MustInvoke[service.TablesService](injector)
	indexSvc := do.MustInvoke[service.IndexesService](injector)

	testSchema := "sqlmeta_index_schema_" + uuid.NewString()[:8]
	schemaInput := types.SchemaCreateInput{Name: testSchema, Owner: "postgres"}
	createdSchema, err := schemaSvc.Create(ctx, schemaInput)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = schemaSvc.Delete(context.Background(), createdSchema.ID, types.SchemaDropOptions{Cascade: true})
	})

	baseTableName := "sqlmeta_index_table_" + uuid.NewString()[:8]
	tableInput := types.TableCreateInput{Name: baseTableName, Schema: testSchema}
	createdTable, err := tableSvc.Create(ctx, tableInput)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	rawPool := pool.Raw()
	tableIdent := pgx.Identifier{testSchema, createdTable.Name}.Sanitize()
	if _, err := rawPool.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN created_at timestamptz DEFAULT now();", tableIdent)); err != nil {
		t.Fatalf("add column for index: %v", err)
	}

	indexName := "sqlmeta_idx_" + uuid.NewString()[:8]
	createIndexSQL := fmt.Sprintf("CREATE INDEX %s ON %s (created_at);", pgx.Identifier{indexName}.Sanitize(), tableIdent)
	if _, err := rawPool.Exec(ctx, createIndexSQL); err != nil {
		t.Fatalf("create index: %v", err)
	}

	listOpts := types.IndexListOptions{IncludedSchemas: []string{testSchema}}
	items, err := indexSvc.List(ctx, listOpts)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}

	var found *types.Index
	for i := range items {
		idx := items[i]
		if strings.Contains(idx.IndexDefinition, indexName) {
			found = &idx
			break
		}
	}
	if found == nil {
		t.Fatalf("expected to find index %s in schema %s", indexName, testSchema)
	}

	if found.Schema != testSchema {
		t.Fatalf("expected index schema %s, got %s", testSchema, found.Schema)
	}
	if found.NumberOfAttributes != 1 {
		t.Fatalf("expected 1 attribute, got %d", found.NumberOfAttributes)
	}
	if len(found.IndexAttributes) != 1 || found.IndexAttributes[0].AttributeName != "created_at" {
		t.Fatalf("unexpected index attributes: %+v", found.IndexAttributes)
	}

	retrieved, err := indexSvc.Retrieve(ctx, found.ID)
	if err != nil {
		t.Fatalf("retrieve index: %v", err)
	}
	if retrieved == nil {
		t.Fatalf("retrieve index returned nil result")
	}
	if retrieved.ID != found.ID {
		t.Fatalf("expected retrieved index id %d, got %d", found.ID, retrieved.ID)
	}
	if retrieved.IndexDefinition != found.IndexDefinition {
		t.Fatalf("expected matching index definition, got %q vs %q", found.IndexDefinition, retrieved.IndexDefinition)
	}

	excluded, err := indexSvc.List(ctx, types.IndexListOptions{ExcludedSchemas: []string{testSchema}, IncludeSystemSchemas: true})
	if err != nil {
		t.Fatalf("list indexes with exclusion: %v", err)
	}
	for _, idx := range excluded {
		if strings.Contains(idx.IndexDefinition, indexName) {
			t.Fatalf("expected index %s to be excluded", indexName)
		}
	}
}
