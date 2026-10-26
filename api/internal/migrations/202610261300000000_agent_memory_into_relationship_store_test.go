package migrations

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAgentMemoryIntoRelationshipStoreKeepsTheNewerValueAndHonoursLegacyClears(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Agent memory move PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunWithDB(db); err != nil {
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
	service := relationshipmemory.NewService(tx)
	workspaceID, agentID, subject := uuid.New(), uuid.New(), usersubject.AccountSubject(uuid.New())
	relationship, err := service.EnsureRelationship(ctx, relationshipmemory.EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: workspaceID, Subject: subject,
		ConsumerType: relationshipmemory.ConsumerAgent, ConsumerID: agentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	contract := relationshipmemory.ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", Definitions: []relationshipmemory.MemoryDefinition{
		{Key: "stale", Enabled: true, MaxChars: 2000}, {Key: "newer", Enabled: true, MaxChars: 2000},
		{Key: "cleared", Enabled: true, MaxChars: 2000}, {Key: "kept", Enabled: true, MaxChars: 2000}}}
	for key, content := range map[string]string{"stale": "Old copy.", "newer": "Written after the legacy value.", "cleared": "Cleared in the legacy store.", "kept": "Only in the relationship store."} {
		if _, err := service.PutItem(ctx, relationship.ID, relationshipmemory.PutItemRequest{Key: key, Content: content, Contract: contract,
			Metadata: relationshipmemory.MutationMetadata{SourceKind: relationshipmemory.SourceExplicit, EvidenceFromUserInput: true}}); err != nil {
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

	if err := upAgentMemoryIntoRelationshipStore(mschema.New(tx)); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	type item struct {
		MemoryKey      string
		ContentPayload string
		Revision       int64
	}
	read := func() map[string]item {
		t.Helper()
		var rows []item
		if err := tx.Raw(`SELECT memory_key, content_payload, revision FROM user_memory_items WHERE relationship_id = ?`, relationship.ID).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		out := map[string]item{}
		for _, row := range rows {
			out[row.MemoryKey] = row
		}
		return out
	}
	values := read()
	want := map[string]string{"stale": "Updated in the legacy store.", "newer": "Written after the legacy value.", "legacy_only": "Never copied before.", "kept": "Only in the relationship store."}
	if len(values) != len(want) {
		t.Fatalf("values after the move = %v", values)
	}
	for key, content := range want {
		if values[key].ContentPayload != content {
			t.Fatalf("%s = %q, want %q (all: %v)", key, values[key].ContentPayload, content, values)
		}
	}
	var epoch int64
	tx.Raw(`SELECT memory_epoch FROM user_memory_relationships WHERE id = ?`, relationship.ID).Scan(&epoch)
	if epoch < 3 {
		t.Fatalf("relationship epoch = %d, want the legacy epoch 3", epoch)
	}

	// A second run changes nothing.
	if err := upAgentMemoryIntoRelationshipStore(mschema.New(tx)); err != nil {
		t.Fatalf("second run: %v", err)
	}
	again := read()
	if len(again) != len(values) {
		t.Fatalf("second run changed the items: %v", again)
	}
	for key, before := range values {
		if again[key] != before {
			t.Fatalf("second run changed %s: %+v -> %+v", key, before, again[key])
		}
	}

	// A legacy value of a user with no relationship yet gets one.
	newcomer := uuid.New()
	if err := tx.Exec(`INSERT INTO agent_memory_values (id, workspace_id, agent_id, slot_key, user_scope, user_id, content, revision, source_kind, created_at, updated_at)
		VALUES (?, ?, ?, 'profile', 'account', ?, 'Prefers short answers.', 1, 'explicit', ?, ?)`, uuid.New(), workspaceID, agentID, newcomer, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Legacy memory') ON CONFLICT (id) DO NOTHING`, workspaceID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := upAgentMemoryIntoRelationshipStore(mschema.New(tx)); err != nil {
		t.Fatalf("third run: %v", err)
	}
	var moved string
	tx.Raw(`SELECT item.content_payload FROM user_memory_items item JOIN user_memory_relationships relationship ON relationship.id = item.relationship_id
		WHERE relationship.subject_id = ? AND relationship.consumer_id = ? AND item.memory_key = 'profile'`, newcomer, agentID).Scan(&moved)
	if moved != "Prefers short answers." {
		t.Fatalf("the newcomer's legacy value was not moved: %q", moved)
	}
}
