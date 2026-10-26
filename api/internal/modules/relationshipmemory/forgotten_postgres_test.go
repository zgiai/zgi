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

func TestForgottenLinesPostgresListWhatTheUserRemoved(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run forgotten memory PostgreSQL integration")
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
	contract := ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", RenderBudgetChars: 4000,
		Definitions: []MemoryDefinition{{Key: "profile", Enabled: true, MaxChars: 2000}}}
	put := func(content string, expected int64, kind string) {
		t.Helper()
		if _, err := service.PutItem(ctx, relationship.ID, PutItemRequest{Key: "profile", Content: content, ExpectedRevision: expected, Contract: contract,
			Metadata: MutationMetadata{SourceKind: kind, EvidenceFromUserInput: true}}); err != nil {
			t.Fatal(err)
		}
	}
	put("在深圳工作。\n开一辆蓝色特斯拉。\n养了一只柯基。", 0, SourceExplicit)
	put("- 在深圳工作。\n- 养了一只柯基。", 1, SourceAutomatic)
	put("在深圳工作。\n养了一只柯基。", 2, SourceExplicit)
	if lines, err := service.ForgottenLines(ctx, relationship.ID); err != nil || len(lines) != 0 {
		t.Fatalf("automatic tidying or reformatting counted as forgetting: %v, %v", lines, err)
	}
	put("在深圳工作。\n开一辆蓝色特斯拉。\n养了一只柯基。", 3, SourceExplicit)
	put("在深圳工作。\n养了一只柯基。", 4, SourceExplicit)
	lines, err := service.ForgottenLines(ctx, relationship.ID)
	if err != nil || len(lines) != 1 || lines[0] != "开一辆蓝色特斯拉。" {
		t.Fatalf("forgotten lines = %v, %v", lines, err)
	}
}
