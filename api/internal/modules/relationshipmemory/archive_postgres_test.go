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

func TestMemoryArchivePostgresKeepsSearchesAndClearsWithTheUser(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run memory archive PostgreSQL integration")
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
	service := NewService(tx)
	relationship, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerAgent, ConsumerID: uuid.New(), ConsumerNameSnapshot: "Assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	meta := MutationMetadata{ActorType: "model", SourceKind: SourceAutomatic, Reason: "the profile file was full"}
	trip, err := service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: "2025 年春天去过京都赏樱，住在祇园附近。", Tags: "profile", SourceKey: "profile", Metadata: meta})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: "Prefers aisle seats on long flights.", Tags: "preferences", Metadata: meta}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: "API key sk-abcdefghijklmnopqrstuv", Metadata: meta}); err == nil {
		t.Fatal("a secret was archived")
	}

	// A question finds entries that share its words, CJK included.
	found, err := service.SearchArchive(ctx, relationship.ID, "我之前去京都是什么时候？", 5)
	if err != nil || len(found) != 1 || found[0].ID != trip.ID || !strings.Contains(found[0].ContentPayload, "京都") {
		t.Fatalf("CJK search = %+v, %v", found, err)
	}
	if found, _ := service.SearchArchive(ctx, relationship.ID, "which seat on flights", 5); len(found) != 1 || !strings.Contains(found[0].ContentPayload, "aisle") {
		t.Fatalf("English search = %+v", found)
	}

	if _, err := service.UpdateArchiveEntry(ctx, relationship.ID, trip.ID, "2025 年春天去过京都和奈良。", MutationMetadata{ActorType: "user", SourceKind: SourceManager}); err != nil {
		t.Fatal(err)
	}
	export, err := service.Export(ctx, relationship.ID)
	if err != nil || len(export.Archive) != 2 {
		t.Fatalf("export archive = %+v, %v", export, err)
	}
	page, _ := service.History(ctx, relationship.ID, HistoryRequest{Key: ArchiveKey(trip.ID)})
	if len(page.Commits) != 2 || page.Commits[0].Versions[0].After != "2025 年春天去过京都和奈良。" || page.Commits[1].Versions[0].Reason != "the profile file was full" {
		t.Fatalf("archive history = %+v", page.Commits)
	}

	if err := service.DeleteArchiveEntry(ctx, relationship.ID, trip.ID, MutationMetadata{ActorType: "user", SourceKind: SourceManager}); err != nil {
		t.Fatal(err)
	}
	if left, _ := service.ArchiveEntries(ctx, relationship.ID, 0); len(left) != 1 {
		t.Fatalf("after delete = %d entries", len(left))
	}
	// Clearing all memory clears the archive and its history content.
	if err := service.ClearItems(ctx, relationship.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := service.ArchiveEntries(ctx, relationship.ID, 0); len(left) != 0 {
		t.Fatalf("after clear = %d entries", len(left))
	}
	var unpurged int64
	tx.Model(&MemoryFileVersion{}).Where("relationship_id = ? AND content_purged_at IS NULL", relationship.ID).Count(&unpurged)
	if unpurged != 0 {
		t.Fatalf("%d versions kept their content after clear", unpurged)
	}
}

// Review of #2893: a stale writer cannot archive after memory was cleared or
// turned off, and an archive change is never offered as a file revert.
func TestMemoryArchivePostgresFencesStaleWritersAndIsNotRevertedAsAFile(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run memory archive PostgreSQL integration")
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
	service := NewService(tx)
	relationship, err := service.EnsureRelationship(ctx, EnsureRelationshipRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(uuid.New()),
		ConsumerType: ConsumerAgent, ConsumerID: uuid.New(), ConsumerNameSnapshot: "Assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	staleEpoch := relationship.MemoryEpoch
	stale := MutationMetadata{ActorType: "model", SourceKind: SourceAutomatic, ExpectedMemoryEpoch: &staleEpoch}
	if err := service.ClearItems(ctx, relationship.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: "Lives in Hangzhou.", Metadata: stale}); err == nil {
		t.Fatal("a writer from before the clear archived its text")
	}

	var current UserMemoryRelationship
	if err := tx.First(&current, "id = ?", relationship.ID).Error; err != nil {
		t.Fatal(err)
	}
	fresh := current.MemoryEpoch
	off := false
	if _, err := service.UpdateSettings(ctx, relationship.ID, UpdateSettingsRequest{MemoryEnabled: &off, ExpectedRevision: current.SettingsRevision, ActorType: "user", Source: "profile"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: "Lives in Hangzhou.", Metadata: MutationMetadata{ActorType: "model", SourceKind: SourceAutomatic, ExpectedMemoryEpoch: &fresh}}); err == nil {
		t.Fatal("a writer archived after memory was turned off")
	}
	if entries, _ := service.ArchiveEntries(ctx, relationship.ID, 0); len(entries) != 0 {
		t.Fatalf("archive holds %d entries after fenced writes", len(entries))
	}

	on := true
	if err := tx.First(&current, "id = ?", relationship.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateSettings(ctx, relationship.ID, UpdateSettingsRequest{MemoryEnabled: &on, ExpectedRevision: current.SettingsRevision, ActorType: "user", Source: "profile"}); err != nil {
		t.Fatal(err)
	}
	manager := MutationMetadata{ActorType: "user", SourceKind: SourceManager}
	entry, err := service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: "Prefers aisle seats.", Metadata: manager})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteArchiveEntry(ctx, relationship.ID, entry.ID, manager); err != nil {
		t.Fatal(err)
	}
	page, err := service.History(ctx, relationship.ID, HistoryRequest{Key: ArchiveKey(entry.ID)})
	if err != nil || len(page.Commits) == 0 {
		t.Fatalf("archive history = %+v, %v", page, err)
	}
	deletion := page.Commits[0].Versions[0]
	if deletion.Revertible {
		t.Fatal("an archive deletion is offered as a file revert")
	}
	if _, err := service.RevertVersion(ctx, relationship.ID, deletion.ID, ConsumerMemoryContract{}, manager); !errors.Is(err, ErrUndoUnavailable) {
		t.Fatalf("RevertVersion(archive deletion) = %v, want ErrUndoUnavailable", err)
	}
}
