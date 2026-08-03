package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFindPendingVectorSyncIncludesOnlyGraphProjectedEntities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:entity-vector-sync?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TABLE kb_entities (
			id text PRIMARY KEY,
			kb_id text NOT NULL,
			tenant_id text NOT NULL,
			name text NOT NULL,
			canonical_name text NOT NULL,
			type text NOT NULL,
			vector_state text NOT NULL,
			graph_state text NOT NULL,
			is_deleted numeric NOT NULL DEFAULT false,
			created_at datetime,
			updated_at datetime
		)
	`).Error; err != nil {
		t.Fatal(err)
	}

	kbID := uuid.New()
	tenantID := uuid.New()
	tests := []struct {
		name        string
		vectorState string
		graphState  string
		deleted     bool
		want        bool
	}{
		{name: "pending vector after graph sync", vectorState: "pending", graphState: "synced", want: true},
		{name: "failed vector after graph sync", vectorState: "failed", graphState: "synced", want: true},
		{name: "already vector synced", vectorState: "synced", graphState: "synced"},
		{name: "graph projection pending", vectorState: "pending", graphState: "pending"},
		{name: "graph projection failed", vectorState: "failed", graphState: "failed"},
		{name: "deleted entity", vectorState: "pending", graphState: "synced", deleted: true},
	}
	wanted := make(map[string]bool)
	for _, tt := range tests {
		if err := db.Exec(
			"INSERT INTO kb_entities (id, kb_id, tenant_id, name, canonical_name, type, vector_state, graph_state, is_deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			uuid.New(), kbID, tenantID, tt.name, tt.name, "test", tt.vectorState, tt.graphState, tt.deleted,
		).Error; err != nil {
			t.Fatal(err)
		}
		if tt.want {
			wanted[tt.name] = true
		}
	}

	results, err := NewEntityRepository(db).FindPendingVectorSync(context.Background(), kbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(wanted) {
		t.Fatalf("entities requiring sync = %d, want %d", len(results), len(wanted))
	}
	for _, entity := range results {
		if !wanted[entity.Name] {
			t.Fatalf("unexpected entity selected for vector sync: %q", entity.Name)
		}
	}
}
