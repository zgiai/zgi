package digitalhuman

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPersonNotesPostgresLiveInTheRelationshipMemory(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the person notes PostgreSQL integration")
	}
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED", "true")
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_MAX_ENTRIES", "2")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Notes')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Notes workspace')", workspaceID, organizationID)
	service := NewService(NewRepository(tx))
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: accountID}
	detail, err := service.Create(ctx, scope, CreateRequest{Name: "Note keeper", Persona: "Helps.", Rules: []string{"Be brief."}, DefaultModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(ctx, scope, detail.Definition.ID, PublishRequest{ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID
	state, err := service.UpdateMemoryState(ctx, scope, digitalHumanID, UpdateMemoryStateRequest{ExpectedMemoryEpoch: 0, AutoExtractionEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	person := func() (uuid.UUID, uuid.UUID) {
		t.Helper()
		subjectID, instanceID := uuid.New(), uuid.New()
		exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id) VALUES (?, 'api_end_user', 'digital_human_api', ?)", subjectID, digitalHumanID)
		exec(`INSERT INTO digital_human_end_user_instances (id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, issuer_id, status, quota_policy, first_seen_at, last_active_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'api_end_user', ?, 'digital_human_api', ?, 'active', '{}', now(), now(), now(), now())`, instanceID, organizationID, workspaceID, digitalHumanID, subjectID, digitalHumanID)
		return subjectID, instanceID
	}
	subject, instance := person()
	_, otherInstance := person()
	note := func(instanceID uuid.UUID, key, content string, expected int64, action memoryruntime.MutationAction) (*MemoryMutationResult, error) {
		t.Helper()
		return service.ApplyAutonomousMemoryProposal(ctx, scope, digitalHumanID, MemoryProposalRequest{
			OperationID: uuid.New(), Key: key, Action: action, MemoryClass: memoryruntime.MemoryClassBusiness,
			Scope:      &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeInstance, ID: instanceID.String()},
			MemoryType: memoryruntime.MemoryTypeOpenLoop, Content: content, ExpectedRevision: expected,
			ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "note"}},
			Confidence: 1, OccurredAt: time.Now().UTC(), SourceContext: MemorySourceContext{SourceTrust: SourceTrustExternalEndUser},
		})
	}

	// A note is written to the person's relationship memory, not the
	// DigitalHuman's own memory.
	created, err := note(instance, "quote_due", "Promised the quote by Friday.", 0, memoryruntime.MutationActionUpsert)
	if err != nil || created.Memory.Revision != 1 || created.Memory.ScopeType != memoryruntime.MemoryScopeInstance {
		t.Fatalf("note = %#v, %v", created, err)
	}
	var ownRows int64
	tx.Raw("SELECT count(*) FROM digital_human_memories WHERE digital_human_id = ? AND scope_type = 'instance'", digitalHumanID).Scan(&ownRows)
	if ownRows != 0 {
		t.Fatalf("a note was written to the DigitalHuman's own memory: %d rows", ownRows)
	}
	relationships := relationshipmemory.NewService(tx)
	notes, err := relationships.InstanceNotes(ctx, workspaceID, digitalHumanID, subject)
	if err != nil || len(notes) != 1 || notes[0].Path != "/user/notes/quote_due.md" || notes[0].Tags != "open_loop" {
		t.Fatalf("the person's notes = %+v, %v", notes, err)
	}

	// The end user's run reads it; another person's run does not.
	read := func(instanceID uuid.UUID) []*DigitalHumanMemory {
		t.Helper()
		items, err := service.repo.ActiveMemories(ctx, scope, digitalHumanID, 50, &memoryReadScope{external: true, instanceID: instanceID.String()})
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	if items := read(instance); len(items) != 1 || items[0].MemoryKey != "quote_due" || items[0].Content != "Promised the quote by Friday." {
		t.Fatalf("own notes = %#v", items)
	}
	if items := read(otherInstance); len(items) != 0 {
		t.Fatalf("another person read the notes: %#v", items)
	}

	// Updates follow the item revision; the capacity limits new notes only.
	if updated, err := note(instance, "quote_due", "Promised the quote by Tuesday.", 1, memoryruntime.MutationActionUpsert); err != nil || updated.Memory.Revision != 2 {
		t.Fatalf("update = %#v, %v", updated, err)
	}
	if _, err := note(instance, "quote_due", "stale", 1, memoryruntime.MutationActionUpsert); !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	if _, err := note(instance, "sso_guide", "Send the SSO guide.", 0, memoryruntime.MutationActionUpsert); err != nil {
		t.Fatal(err)
	}
	if _, err := note(instance, "third", "One too many.", 0, memoryruntime.MutationActionUpsert); err == nil {
		t.Fatal("a note beyond the capacity was written")
	}
	if _, err := note(instance, "sso_guide", "", 1, memoryruntime.MutationActionClear); err != nil {
		t.Fatalf("clear note: %v", err)
	}

	// An expired note moves into the person's archive.
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_TTL", "1h")
	exec("UPDATE user_memory_items SET updated_at = now() - interval '2 hours' WHERE memory_key = ?", relationshipmemory.NoteKey("quote_due"))
	if moved, err := service.ArchiveExpiredInstanceMemories(ctx); err != nil || moved != 1 {
		t.Fatalf("expired notes moved = %d, %v", moved, err)
	}
	if items := read(instance); len(items) != 0 {
		t.Fatalf("an expired note is still read: %#v", items)
	}
	relationship, err := relationships.FindRelationship(ctx, workspaceID, subject, relationshipmemory.ConsumerDigitalHuman, digitalHumanID)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := relationships.SearchArchive(ctx, relationship.ID, "quote Tuesday", 5)
	if err != nil || len(archived) != 1 || archived[0].SourceKey != relationshipmemory.NoteKey("quote_due") {
		t.Fatalf("archive = %+v, %v", archived, err)
	}
}
