package relationshipmemory

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInstanceNotesPostgresListOnlyThePersonsActiveNotes(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run instance notes PostgreSQL integration")
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := tx.Exec(sql, args...).Error; err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	organizationID, workspaceID, digitalHumanID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "Instance notes")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Instance notes")
	exec("INSERT INTO digital_humans (id, organization_id, workspace_id, name) VALUES (?, ?, ?, ?)", digitalHumanID, organizationID, workspaceID, "Notes")
	person, other := uuid.New(), uuid.New()
	service := NewService(tx)
	relationships := map[uuid.UUID]uuid.UUID{}
	for _, subjectID := range []uuid.UUID{person, other} {
		exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id) VALUES (?, 'api_end_user', 'digital_human_api', ?)", subjectID, digitalHumanID)
		issuerID := digitalHumanID
		relationship, err := service.EnsureRelationship(t.Context(), EnsureRelationshipRequest{
			OrganizationID: organizationID, WorkspaceID: workspaceID, Subject: usersubject.Ref{Kind: usersubject.APIEndUser, ID: subjectID},
			IssuerType: "digital_human_api", IssuerID: &issuerID, ConsumerType: ConsumerDigitalHuman, ConsumerID: digitalHumanID,
		})
		if err != nil {
			t.Fatal(err)
		}
		relationships[subjectID] = relationship.ID
	}
	contract := ConsumerMemoryContract{Enabled: true, DefinitionMode: "dynamic", AllowedCategories: []string{"preference"}}
	put := func(subjectID uuid.UUID, key, category, content string, age int) {
		t.Helper()
		if _, err := service.PutItem(t.Context(), relationships[subjectID], PutItemRequest{Key: key, Category: category, Content: content, Contract: contract,
			Metadata: MutationMetadata{ActorType: "operator", SourceKind: SourceManager}}); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
		exec("UPDATE user_memory_items SET updated_at = now() - make_interval(mins => ?) WHERE relationship_id = ? AND memory_key = ?", age, relationships[subjectID], key)
	}
	put(person, NoteKey("quote_due"), "open_loop", "Promised a quote by Friday.", 10)
	put(person, NoteKey("plan_enterprise"), "fact", "Evaluating the Enterprise plan.", 1)
	put(person, "prefers_email", "preference", "Prefers email.", 1)
	put(other, NoteKey("someone_else"), "fact", "Another person's note.", 1)
	if _, err := service.PutItem(t.Context(), relationships[person], PutItemRequest{Key: NoteKey("bad"), Category: "preference", Content: "x", Contract: contract,
		Metadata: MutationMetadata{ActorType: "operator", SourceKind: SourceManager}}); err == nil {
		t.Fatal("a note was saved with a category of the person's own memory")
	}

	notes, err := NewService(tx).InstanceNotes(t.Context(), workspaceID, digitalHumanID, person)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Path != "/user/notes/plan_enterprise.md" || notes[1].Path != "/user/notes/quote_due.md" || notes[1].Tags != "open_loop" || notes[1].Key != "quote_due" {
		t.Fatalf("notes = %+v", notes)
	}
	if none, err := NewService(tx).InstanceNotes(t.Context(), workspaceID, uuid.New(), person); err != nil || len(none) != 0 {
		t.Fatalf("another DigitalHuman's notes = %+v, %v", none, err)
	}
}
