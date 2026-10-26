package agentmemory_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Since ADR 0016 an Agent's blocks are stored only as memory block
// definitions: what is synced is what is listed, and a block that comes back
// keeps its ID.
func TestBlocksPostgresAreStoredAsDefinitions(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the memory block PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	ctx := t.Context()
	repo := agentmemory.NewRepository(tx)
	workspaceID, agentID := uuid.New(), uuid.New()

	sync := func(slots ...*agentmemory.AgentMemorySlot) []*agentmemory.AgentMemorySlot {
		t.Helper()
		if err := repo.SyncBlockDefinitions(ctx, agentID, slots); err != nil {
			t.Fatal(err)
		}
		listed, err := repo.ListSlots(ctx, workspaceID, agentID, false)
		if err != nil {
			t.Fatal(err)
		}
		return listed
	}
	first := sync(
		&agentmemory.AgentMemorySlot{Key: "profile", Name: "Profile", MaxChars: 800, Enabled: true, SortOrder: 1, ReadOnly: true},
		&agentmemory.AgentMemorySlot{Key: "preferences", MaxChars: 600, Enabled: true, SortOrder: 2, ManualOnly: true},
	)
	if len(first) != 2 || first[0].Key != "profile" || first[0].Name != "Profile" || first[0].MaxChars != 800 || !first[0].ReadOnly ||
		first[1].Key != "preferences" || !first[1].ManualOnly || first[1].WorkspaceID != workspaceID {
		t.Fatalf("listed blocks = %+v %+v", first[0], first[1])
	}
	profileID := first[0].ID

	if second := sync(&agentmemory.AgentMemorySlot{Key: "preferences", MaxChars: 600, Enabled: false}); len(second) != 1 || second[0].Enabled {
		t.Fatalf("after removing profile = %+v", second)
	}
	if enabled, err := repo.ListSlots(ctx, workspaceID, agentID, true); err != nil || len(enabled) != 0 {
		t.Fatalf("enabled blocks = %v, %v", enabled, err)
	}
	third := sync(&agentmemory.AgentMemorySlot{Key: "profile", MaxChars: 500, Enabled: true})
	if len(third) != 1 || third[0].ID != profileID || third[0].MaxChars != 500 || third[0].ReadOnly {
		t.Fatalf("restored profile = %+v, want ID %s", third, profileID)
	}
	var legacy int64
	tx.Table("agent_memory_slots").Where("agent_id = ?", agentID).Count(&legacy)
	if legacy != 0 {
		t.Fatalf("agent_memory_slots rows = %d, want none", legacy)
	}
}
