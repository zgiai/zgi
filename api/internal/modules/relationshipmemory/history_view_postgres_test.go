package relationshipmemory

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMemoryHistoryPostgresShowsChangesAndRevertsTheLatest(t *testing.T) {
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
	relationship, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerAgent, ConsumerID: uuid.New(), ConsumerNameSnapshot: "Assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	contract := ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", RenderBudgetChars: 4000,
		Definitions: []MemoryDefinition{{Key: "profile", Enabled: true, MaxChars: 2000}, {Key: "preferences", Enabled: true, MaxChars: 2000}}}
	put := func(key, content string, expected int64, evidence, reason string) {
		t.Helper()
		if _, err := service.PutItem(ctx, relationship.ID, PutItemRequest{Key: key, Content: content, ExpectedRevision: expected, Contract: contract,
			Metadata: MutationMetadata{SourceKind: SourceExplicit, EvidenceFromUserInput: true, Evidence: evidence, Reason: reason}}); err != nil {
			t.Fatal(err)
		}
	}
	put("profile", "住在上海。", 0, "我住在上海", "the user asked to remember")
	put("preferences", "回答简短。", 0, "回答短一点", "the user asked to remember")
	put("profile", "住在杭州。", 1, "我搬到杭州了", "the user corrected it")

	page, err := service.History(ctx, relationship.ID, HistoryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Commits) != 3 || page.NextBefore != nil {
		t.Fatalf("history = %+v", page)
	}
	latest := page.Commits[0].Versions[0]
	if latest.Key != "profile" || latest.Before != "住在上海。" || latest.After != "住在杭州。" || latest.Evidence != "我搬到杭州了" || latest.Reason != "the user corrected it" || !latest.Revertible {
		t.Fatalf("latest change = %+v", latest)
	}
	first := page.Commits[2].Versions[0]
	if first.Revertible || first.Action != VersionCreate {
		t.Fatalf("an older change is revertible: %+v", first)
	}
	if _, err := service.RevertVersion(ctx, relationship.ID, first.ID, contract, MutationMetadata{ActorType: "user", SourceKind: SourceManager}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("reverting an older change = %v, want a conflict", err)
	}

	// Paging and the per-file filter.
	paged, err := service.History(ctx, relationship.ID, HistoryRequest{Limit: 2})
	if err != nil || len(paged.Commits) != 2 || paged.NextBefore == nil {
		t.Fatalf("first page = %+v, %v", paged, err)
	}
	rest, err := service.History(ctx, relationship.ID, HistoryRequest{Limit: 2, Before: paged.NextBefore})
	if err != nil || len(rest.Commits) != 1 {
		t.Fatalf("second page = %+v, %v", rest, err)
	}
	onlyPreferences, err := service.History(ctx, relationship.ID, HistoryRequest{Key: "preferences"})
	if err != nil || len(onlyPreferences.Commits) != 1 || onlyPreferences.Commits[0].Versions[0].Key != "preferences" {
		t.Fatalf("preferences history = %+v, %v", onlyPreferences, err)
	}

	// Reverting the correction brings back the earlier content as a new change.
	item, err := service.RevertVersion(ctx, relationship.ID, latest.ID, contract, MutationMetadata{ActorType: "user", SourceKind: SourceManager})
	if err != nil || item == nil || item.ContentPayload != "住在上海。" {
		t.Fatalf("revert = %+v, %v", item, err)
	}
	page, _ = service.History(ctx, relationship.ID, HistoryRequest{Key: "profile"})
	if len(page.Commits) != 3 || page.Commits[0].Versions[0].After != "住在上海。" || page.Commits[0].Versions[0].Reason == "" || page.Commits[1].Versions[0].Revertible {
		t.Fatalf("history after revert = %+v", page.Commits)
	}

	// Reverting a creation removes the file.
	preferences := onlyPreferences.Commits[0].Versions[0]
	if item, err := service.RevertVersion(ctx, relationship.ID, preferences.ID, contract, MutationMetadata{ActorType: "user", SourceKind: SourceManager}); err != nil || item != nil {
		t.Fatalf("revert of a creation = %+v, %v", item, err)
	}
	var count int64
	tx.Model(&UserMemoryItem{}).Where("relationship_id = ? AND memory_key = ?", relationship.ID, "preferences").Count(&count)
	if count != 0 {
		t.Fatal("reverting the creation kept the file")
	}

	// Clearing all memory purges history content, quotes included, and nothing is revertible.
	if err := service.ClearItems(ctx, relationship.ID); err != nil {
		t.Fatal(err)
	}
	page, _ = service.History(ctx, relationship.ID, HistoryRequest{})
	for _, commit := range page.Commits {
		for _, version := range commit.Versions {
			if !version.ContentPurged || version.Before != "" || version.After != "" || version.Evidence != "" || version.Revertible {
				t.Fatalf("version after clear = %+v", version)
			}
		}
	}
}
