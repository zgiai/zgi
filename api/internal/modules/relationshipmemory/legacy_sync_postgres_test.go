package relationshipmemory

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestLegacyAgentMemorySyncKeepsTheNewerValueAndHonoursLegacyClears(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run legacy memory sync PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	// Legacy rows reference an Agent; this rolled-back transaction skips
	// foreign-key checks instead of building one.
	if err := tx.Exec("SET LOCAL session_replication_role = replica").Error; err != nil {
		t.Skipf("cannot relax foreign keys for the legacy fixture: %v", err)
	}
	ctx := t.Context()
	service := NewService(tx)
	workspaceID, agentID, subject := uuid.New(), uuid.New(), usersubject.AccountSubject(uuid.New())
	relationship, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{OrganizationID: uuid.New(), WorkspaceID: workspaceID, Subject: subject, ConsumerType: ConsumerAgent, ConsumerID: agentID})
	if err != nil {
		t.Fatal(err)
	}
	contract := ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", Definitions: []MemoryDefinition{
		{Key: "stale", Enabled: true, MaxChars: 2000}, {Key: "newer", Enabled: true, MaxChars: 2000},
		{Key: "cleared", Enabled: true, MaxChars: 2000}, {Key: "kept", Enabled: true, MaxChars: 2000}}}
	for key, content := range map[string]string{"stale": "Old copy.", "newer": "Written after the legacy value.", "cleared": "Cleared in the legacy store.", "kept": "Only in the relationship store."} {
		if _, err := service.PutItem(ctx, relationship.ID, PutItemRequest{Key: key, Content: content, Contract: contract,
			Metadata: MutationMetadata{SourceKind: SourceExplicit, EvidenceFromUserInput: true}}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	// Relationship values were written an hour ago, except "newer", written now.
	tx.Exec(`UPDATE user_memory_items SET updated_at = ? WHERE relationship_id = ? AND memory_key <> 'newer'`, now.Add(-time.Hour), relationship.ID)
	legacy := func(key, content string, updatedAt time.Time) {
		t.Helper()
		if err := tx.Exec(`INSERT INTO agent_memory_values (id, workspace_id, agent_id, slot_key, user_scope, user_id, content, revision, source_kind, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'account', ?, ?, 1, 'explicit', ?, ?)`, uuid.New(), workspaceID, agentID, key, subject.ID, content, updatedAt, updatedAt).Error; err != nil {
			t.Fatal(err)
		}
	}
	legacy("stale", "Updated in the legacy store.", now.Add(-time.Minute))
	legacy("newer", "Older legacy value.", now.Add(-2*time.Hour))
	legacy("legacy_only", "Never copied before.", now.Add(-time.Minute))
	// The legacy store cleared this user's memory half an hour ago; "cleared" and "kept" are both absent there.
	if err := tx.Exec(`INSERT INTO agent_memory_subject_states (id, workspace_id, agent_id, user_scope, user_id, memory_epoch, extraction_cutoff_at, created_at, updated_at)
		VALUES (?, ?, ?, 'account', ?, 3, ?, ?, ?)`, uuid.New(), workspaceID, agentID, subject.ID, now.Add(-30*time.Minute), now, now).Error; err != nil {
		t.Fatal(err)
	}
	tx.Exec(`UPDATE user_memory_items SET updated_at = ? WHERE relationship_id = ? AND memory_key = 'kept'`, now.Add(-10*time.Minute), relationship.ID)

	planned, err := SyncLegacyAgentMemory(ctx, tx, false)
	if err != nil {
		t.Fatal(err)
	}
	if planned.ValuesUpdated != 1 || planned.ValuesInserted != 1 || planned.ValuesDeleted != 1 {
		t.Fatalf("planned sync = %+v", planned)
	}
	var unchanged string
	tx.Raw(`SELECT content_payload FROM user_memory_items WHERE relationship_id = ? AND memory_key = 'stale'`, relationship.ID).Scan(&unchanged)
	if unchanged != "Old copy." {
		t.Fatalf("planning changed a value: %q", unchanged)
	}

	if _, err := SyncLegacyAgentMemory(ctx, tx, true); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	var rows []UserMemoryItem
	tx.Where("relationship_id = ?", relationship.ID).Find(&rows)
	for _, row := range rows {
		values[row.MemoryKey] = row.ContentPayload
	}
	want := map[string]string{"stale": "Updated in the legacy store.", "newer": "Written after the legacy value.", "legacy_only": "Never copied before.", "kept": "Only in the relationship store."}
	if len(values) != len(want) {
		t.Fatalf("values after sync = %v", values)
	}
	for key, content := range want {
		if values[key] != content {
			t.Fatalf("%s = %q, want %q (all: %v)", key, values[key], content, values)
		}
	}
	var epoch int64
	tx.Raw(`SELECT memory_epoch FROM user_memory_relationships WHERE id = ?`, relationship.ID).Scan(&epoch)
	if epoch < 3 {
		t.Fatalf("relationship epoch = %d, want the legacy epoch 3", epoch)
	}
	again, err := SyncLegacyAgentMemory(ctx, tx, true)
	if err != nil || again.ValuesInserted+again.ValuesUpdated+again.ValuesDeleted+again.RelationshipsFenced != 0 {
		t.Fatalf("second sync = %+v, %v", again, err)
	}
}
