package graphflow

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGraphRepositoriesIsolateSameEntityIdentityByDataset(t *testing.T) {
	dsn := fmt.Sprintf("file:graph-tenant-isolation-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE kb_entities (
		id text PRIMARY KEY,
		kb_id text NOT NULL,
		tenant_id text NOT NULL,
		name text NOT NULL,
		canonical_name text NOT NULL,
		type text NOT NULL,
		is_deleted boolean NOT NULL DEFAULT false
	)`).Error; err != nil {
		t.Fatal(err)
	}
	firstDataset := uuid.New()
	secondDataset := uuid.New()
	first := &model.Entity{ID: uuid.New(), KBID: firstDataset, TenantID: uuid.New(), Name: "Shared", CanonicalName: "shared", Type: "Concept"}
	second := &model.Entity{ID: uuid.New(), KBID: secondDataset, TenantID: uuid.New(), Name: "Shared", CanonicalName: "shared", Type: "Concept"}
	if err := db.Exec(
		"INSERT INTO kb_entities (id, kb_id, tenant_id, name, canonical_name, type, is_deleted) VALUES (?, ?, ?, ?, ?, ?, false), (?, ?, ?, ?, ?, ?, false)",
		first.ID, first.KBID, first.TenantID, first.Name, first.CanonicalName, first.Type,
		second.ID, second.KBID, second.TenantID, second.Name, second.CanonicalName, second.Type,
	).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewEntityRepository(db)
	result, err := repo.FindByCanonicalName(t.Context(), firstDataset, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.ID != first.ID || result.KBID != firstDataset {
		t.Fatalf("dataset-scoped lookup returned %#v", result)
	}
}

func TestGraphProjectionSourcesKeepEveryTraversalAndVectorClassDatasetScoped(t *testing.T) {
	assertSourceContains(t, "graph/smart_expansion.go", []string{
		"start.kb_id = $kb_id",
		"m.kb_id = $kb_id",
		"p.kb_id = $kb_id",
		"coalesce(r.active_weight, 0) > 0",
	})
	assertSourceExcludes(t, "graph/smart_expansion.go", []string{
		"Trying global search",
		"m.kb_id = $kb_id OR",
	})
	assertSourceContains(t, "retrieval/entity_retrieval.go", []string{
		`fmt.Sprintf("Entity_%s", kbID.String())`,
	})
	assertSourceContains(t, "sync/vector_sync.go", []string{
		`fmt.Sprintf("Entity_%s", kbID.String())`,
	})
	assertSourceContains(t, "worker/cleanup_handler.go", []string{
		`"kb_id = ? AND is_deleted = ?`,
		`relationship.kb_id = kb_entities.kb_id`,
	})
}

func assertSourceContains(t *testing.T, path string, required []string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, value := range required {
		if !strings.Contains(text, value) {
			t.Fatalf("%s does not contain tenant guard %q", path, value)
		}
	}
}

func assertSourceExcludes(t *testing.T, path string, forbidden []string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, value := range forbidden {
		if strings.Contains(text, value) {
			t.Fatalf("%s contains cross-tenant pattern %q", path, value)
		}
	}
}
