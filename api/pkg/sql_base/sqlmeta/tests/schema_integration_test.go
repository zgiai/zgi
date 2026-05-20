package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do/v2"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/service"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/types"
)

func TestSchemaLifecycle(t *testing.T) {
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

	svc := do.MustInvoke[service.SchemasService](injector)

	baseName := "sqlmeta_test_" + uuid.NewString()[:8]
	createInput := types.SchemaCreateInput{
		Name:  baseName,
		Owner: "postgres",
	}

	created, err := svc.Create(ctx, createInput)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if created == nil || created.Name != baseName {
		t.Fatalf("unexpected create result: %#v", created)
	}

	t.Cleanup(func() {
		_, _ = svc.Delete(context.Background(), created.ID, types.SchemaDropOptions{Cascade: true})
	})

	fetchedByName, err := svc.Retrieve(ctx, types.SchemaIdentifier{Name: baseName})
	if err != nil {
		t.Fatalf("retrieve by name: %v", err)
	}
	if fetchedByName.ID != created.ID {
		t.Fatalf("expected same ID, got %d vs %d", fetchedByName.ID, created.ID)
	}

	list, err := svc.List(ctx, types.SchemaListOptions{IncludedSchemas: []string{baseName}})
	if err != nil {
		t.Fatalf("list schemas: %v", err)
	}
	found := false
	for _, s := range list {
		if s.Name == baseName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created schema %s not found in list", baseName)
	}

	newName := baseName + "_renamed"
	updateInput := types.SchemaUpdateInput{Name: &newName}
	updated, err := svc.Update(ctx, created.ID, updateInput)
	if err != nil {
		t.Fatalf("update schema: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("expected name %s after update, got %s", newName, updated.Name)
	}

	if _, err = svc.Delete(ctx, created.ID, types.SchemaDropOptions{Cascade: false}); err != nil {
		t.Fatalf("delete schema: %v", err)
	}

	if _, err = svc.Retrieve(ctx, types.SchemaIdentifier{ID: created.ID}); err == nil {
		t.Fatalf("expected error retrieving deleted schema")
	}
}
