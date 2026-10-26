package relationshipmemory

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/memoryblock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMemoryHistoryPostgresRestoresDeletionsPurgesReasonsAndHonorsBlockLimits(t *testing.T) {
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
	service := NewService(tx)
	explicit := MutationMetadata{SourceKind: SourceExplicit, EvidenceFromUserInput: true, Reason: "Remember that the user lives in Hangzhou"}

	// A deleted Digital Human memory comes back with its category and name.
	human, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerDigitalHuman, ConsumerID: uuid.New(), ConsumerNameSnapshot: "Helper",
	})
	if err != nil {
		t.Fatal(err)
	}
	dynamic := ConsumerMemoryContract{Enabled: true, DefinitionMode: "dynamic", RenderBudgetChars: 4000}
	item, err := service.PutItem(ctx, human.ID, PutItemRequest{Key: "home", DisplayName: "Home city", Category: "profile", Content: "Lives in Hangzhou",
		Contract: dynamic, Metadata: explicit})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteItem(ctx, human.ID, "home", item.Revision, MutationMetadata{SourceKind: SourceExplicit}); err != nil {
		t.Fatal(err)
	}
	page, err := service.History(ctx, human.ID, HistoryRequest{Key: "home"})
	if err != nil || len(page.Commits) == 0 || len(page.Commits[0].Versions) != 1 || !page.Commits[0].Versions[0].Revertible {
		t.Fatalf("history after deletion = %+v, %v", page, err)
	}
	restored, err := service.RevertVersion(ctx, human.ID, page.Commits[0].Versions[0].ID, dynamic, MutationMetadata{SourceKind: SourceExplicit})
	if err != nil || restored == nil || restored.Category != "profile" || restored.DisplayName != "Home city" || restored.ContentPayload != "Lives in Hangzhou" {
		t.Fatalf("restored deletion = %+v, %v", restored, err)
	}

	// Clearing memory also clears the reasons, which can restate the facts.
	if err := service.ClearItems(ctx, human.ID, MutationMetadata{SourceKind: SourceExplicit}); err != nil {
		t.Fatal(err)
	}
	page, err = service.History(ctx, human.ID, HistoryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, commit := range page.Commits {
		for _, version := range commit.Versions {
			if strings.Contains(version.Reason, "Hangzhou") || !version.ContentPurged {
				t.Fatalf("cleared history still holds %+v", version)
			}
		}
	}

	// An Agent block configured for 5000 characters accepts content past 2000.
	agentID := uuid.New()
	if err := memoryblock.SyncAgentSlots(ctx, tx, agentID, []memoryblock.AgentSlot{{Key: "notes", Name: "Notes", MaxChars: 5000, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	agent, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: "Assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	slots := ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", RenderBudgetChars: 8000,
		Definitions: []MemoryDefinition{{Key: "notes", Enabled: true, MaxChars: 5000}}}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "notes", Content: strings.Repeat("a", 2500), Contract: slots, Metadata: explicit}); err != nil {
		t.Fatalf("2500 characters in a 5000-character block: %v", err)
	}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "notes", Content: strings.Repeat("a", 5001), ExpectedRevision: 1, Contract: slots, Metadata: explicit}); err == nil {
		t.Fatal("content past the block limit was saved")
	}
	// Digital Human items keep the fixed limit.
	if _, err := service.PutItem(ctx, human.ID, PutItemRequest{Key: "long", Category: "profile", Content: strings.Repeat("a", 2500), Contract: dynamic, Metadata: explicit}); err == nil {
		t.Fatal("a Digital Human item past 2000 characters was saved")
	}
}
