package relationshipmemory

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
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

	// Agent memory files (ADR 0017) take any valid name and are bounded
	// only by the user's total.
	agentID := uuid.New()
	agent, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: "Assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	files := ConsumerMemoryContract{Enabled: true, DefinitionMode: "files", RenderBudgetChars: 8000}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "notes", Content: strings.Repeat("a", 6000), Contract: files, Metadata: explicit}); err != nil {
		t.Fatalf("6000 characters in one file: %v", err)
	}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "travel_plans", Content: strings.Repeat("b", 3000), Contract: files, Metadata: explicit}); err != nil {
		t.Fatalf("a second file within the total: %v", err)
	}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "hobbies", Content: strings.Repeat("c", 1500), Contract: files, Metadata: explicit}); !errors.Is(err, ErrCapacityExceeded) {
		t.Fatalf("a file past the total = %v, want ErrCapacityExceeded", err)
	}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "notes", Content: strings.Repeat("a", 7500), ExpectedRevision: 1, Contract: files, Metadata: explicit}); err == nil {
		t.Fatal("rewriting a file past the total was saved")
	}
	if _, err := service.PutItem(ctx, agent.ID, PutItemRequest{Key: "Bad Name", Content: "x", Contract: files, Metadata: explicit}); err == nil {
		t.Fatal("an invalid file name was saved")
	}
	// Digital Human items keep the fixed limit.
	if _, err := service.PutItem(ctx, human.ID, PutItemRequest{Key: "long", Category: "profile", Content: strings.Repeat("a", 2500), Contract: dynamic, Metadata: explicit}); err == nil {
		t.Fatal("a Digital Human item past 2000 characters was saved")
	}
}
