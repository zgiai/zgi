package relationshipmemory

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/memoryblock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMemoryHistoryPostgresRecordsEveryChangeAndFollowsTheRelationship(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run memory history PostgreSQL integration")
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
	ctx := t.Context()
	agentID := uuid.New()

	// Definitions mirror the slots and keep their IDs when a slot leaves and returns.
	slots := []memoryblock.AgentSlot{{Key: "profile", Name: "Profile", MaxChars: 2000, Enabled: true}, {Key: "preferences", Name: "Preferences", MaxChars: 2000, Enabled: true, SortOrder: 1}}
	if err := memoryblock.SyncAgentSlots(ctx, tx, agentID, slots); err != nil {
		t.Fatal(err)
	}
	profileDefinition, err := memoryblock.DefinitionID(ctx, tx, memoryblock.ConsumerAgent, agentID, "profile")
	if err != nil || profileDefinition == nil {
		t.Fatalf("profile definition = %v, %v", profileDefinition, err)
	}
	preferencesDefinition, _ := memoryblock.DefinitionID(ctx, tx, memoryblock.ConsumerAgent, agentID, "preferences")
	if err := memoryblock.SyncAgentSlots(ctx, tx, agentID, slots[:1]); err != nil {
		t.Fatal(err)
	}
	var removed memoryblock.Definition
	if err := tx.First(&removed, "id = ?", *preferencesDefinition).Error; err != nil || removed.DeletedAt == nil {
		t.Fatalf("removed slot's definition = %+v, %v", removed, err)
	}
	if err := memoryblock.SyncAgentSlots(ctx, tx, agentID, slots); err != nil {
		t.Fatal(err)
	}
	returned, _ := memoryblock.DefinitionID(ctx, tx, memoryblock.ConsumerAgent, agentID, "preferences")
	if returned == nil || *returned != *preferencesDefinition {
		t.Fatalf("returning slot got definition %v, want %v", returned, *preferencesDefinition)
	}

	service := NewService(tx)
	relationship, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: "Assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	contract := ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", RenderBudgetChars: 4000,
		Definitions: []MemoryDefinition{{Key: "profile", Enabled: true, MaxChars: 2000}, {Key: "preferences", Enabled: true, MaxChars: 2000}}}
	put := func(ctx2 any, key, content string, expected int64) *UserMemoryItem {
		t.Helper()
		item, err := service.PutItem(ctx, relationship.ID, PutItemRequest{Key: key, Content: content, ExpectedRevision: expected, Contract: contract,
			Metadata: MutationMetadata{SourceKind: SourceExplicit, EvidenceFromUserInput: true}})
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	versions := func() []MemoryFileVersion {
		t.Helper()
		var rows []MemoryFileVersion
		if err := tx.Where("relationship_id = ?", relationship.ID).Order("created_at, id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		return rows
	}

	// Create and update: the value points at its definition, and each change keeps both sides.
	created := put(nil, "profile", "Lives in Hangzhou.", 0)
	if created.DefinitionID == nil || *created.DefinitionID != *profileDefinition {
		t.Fatalf("created value definition = %v, want %v", created.DefinitionID, *profileDefinition)
	}
	put(nil, "profile", "Lives in Suzhou.", created.Revision)
	rows := versions()
	if len(rows) != 2 || rows[0].Action != VersionCreate || rows[1].Action != VersionUpdate ||
		rows[1].BeforePayload != "Lives in Hangzhou." || rows[1].AfterPayload != "Lives in Suzhou." || rows[1].DefinitionID == nil {
		t.Fatalf("versions after create and update = %+v", rows)
	}

	// A scoped batch is one commit.
	scoped := WithCommitScope(ctx)
	for _, key := range []string{"preferences", "profile"} {
		current := int64(0)
		var item UserMemoryItem
		if tx.Where("relationship_id = ? AND memory_key = ?", relationship.ID, key).First(&item).Error == nil {
			current = item.Revision
		}
		if _, err := service.PutItem(scoped, relationship.ID, PutItemRequest{Key: key, Content: "Batch value for " + key, ExpectedRevision: current, Contract: contract,
			Metadata: MutationMetadata{SourceKind: SourceAutomatic, ActorType: "model", EvidenceFromUserInput: true}}); err != nil {
			t.Fatal(err)
		}
	}
	rows = versions()
	if len(rows) != 4 || rows[2].CommitID != rows[3].CommitID || rows[1].CommitID == rows[2].CommitID {
		t.Fatalf("batch versions do not share one commit: %+v", rows)
	}

	// Delete, then undo the delete: the restored value gets its definition back.
	var profile UserMemoryItem
	if err := tx.First(&profile, "relationship_id = ? AND memory_key = ?", relationship.ID, "profile").Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := service.DeleteItem(ctx, relationship.ID, "profile", profile.Revision, MutationMetadata{SourceKind: SourceExplicit})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.Undo(ctx, relationship.ID, deleted.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&profile, "id = ?", restored.ID).Error; err != nil || profile.DefinitionID == nil || *profile.DefinitionID != *profileDefinition {
		t.Fatalf("restored value = %+v, %v", profile, err)
	}
	rows = versions()
	if last := rows[len(rows)-1]; len(rows) != 6 || rows[4].Action != VersionDelete || rows[4].BeforePayload != "Batch value for profile" ||
		last.Action != VersionUndo || last.AfterPayload != "Batch value for profile" {
		t.Fatalf("versions after delete and undo = %+v", rows)
	}

	// Clearing all memory records the clear and drops every stored value from history at once.
	if err := service.ClearItems(ctx, relationship.ID); err != nil {
		t.Fatal(err)
	}
	rows = versions()
	for _, row := range rows {
		if row.ContentPurgedAt == nil || row.BeforePayload != "" || row.AfterPayload != "" {
			t.Fatalf("history content survived a clear: %+v", row)
		}
	}
	if len(rows) != 8 || rows[6].Action != VersionClear || rows[7].Action != VersionClear {
		t.Fatalf("versions after clear = %d rows: %+v", len(rows), rows)
	}

	// Retention: content goes after 30 days, the records after 180.
	put(nil, "profile", "Written again.", 0)
	aged := NewService(tx)
	aged.now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	if err := aged.CleanupExpiredHistory(ctx); err != nil {
		t.Fatal(err)
	}
	var unpurged int64
	tx.Model(&MemoryFileVersion{}).Where("relationship_id = ? AND content_purged_at IS NULL", relationship.ID).Count(&unpurged)
	if unpurged != 0 {
		t.Fatalf("%d versions kept content past 30 days", unpurged)
	}
	aged.now = func() time.Time { return time.Now().Add(181 * 24 * time.Hour) }
	if err := aged.CleanupExpiredHistory(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int64
	tx.Model(&MemoryFileVersion{}).Where("relationship_id = ?", relationship.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("%d versions outlived the audit window", remaining)
	}

	// Permanent deletion removes the relationship's whole history.
	put(nil, "preferences", "Last value.", 0)
	if err := service.PermanentlyDeleteRelationship(ctx, relationship.ID); err != nil {
		t.Fatal(err)
	}
	var commits int64
	tx.Model(&MemoryCommit{}).Where("relationship_id = ?", relationship.ID).Count(&commits)
	if commits != 0 {
		t.Fatalf("%d commits survived permanent deletion", commits)
	}
}
