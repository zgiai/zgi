package digitalhuman

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// recordingExtractor returns fixed proposals and keeps the input it saw.
type recordingExtractor struct {
	input     MemoryExtractionInput
	proposals []MemoryExtractionDraft
}

func (e *recordingExtractor) Extract(_ context.Context, input MemoryExtractionInput) ([]MemoryExtractionDraft, error) {
	e.input = input
	return e.proposals, nil
}

func TestDigitalHumanMemoryInstructionsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman memory instructions PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	organizationID, workspaceID, actorID := uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO organizations (id, name) VALUES (?, ?)", []any{organizationID, "Memory instructions"}},
		{"INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", []any{workspaceID, organizationID, "Memory instructions"}},
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{actorID, "Memory operator", "memory-instructions-" + actorID.String() + "@example.invalid"}},
	} {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(NewRepository(tx))
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	const saved = "Keep customer complaints and promised fixes; ignore small talk."
	create := func(instructions string) (*Detail, error) {
		return service.Create(t.Context(), scope, CreateRequest{
			Name: "Support desk", Persona: "Answers customers.", DefaultModel: "test-model",
			MemoryPolicy: map[string]any{"enabled": true, "instructions": instructions}, TriggerPolicy: map[string]any{"manual": true},
		})
	}
	if _, err := create(strings.Repeat("x", maxMemoryInstructionsRunes+1)); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("over-long instructions error = %v", err)
	}
	detail, err := create(saved)
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID

	// A run reads the instructions of the version it uses.
	run := &Run{ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: digitalHumanID, DigitalHumanVersionID: detail.Published.ID}
	if got := service.runMemoryInstructions(t.Context(), run); got != saved {
		t.Fatalf("run instructions = %q", got)
	}
	run.DigitalHumanVersionID = uuid.New()
	if got := service.runMemoryInstructions(t.Context(), run); got != "" {
		t.Fatalf("an unknown version gave instructions %q", got)
	}

	// The preview proposes with the draft's instructions, or the given ones,
	// and says which proposals the memory policy would refuse.
	turns := []MemoryPreviewTurn{{User: "The export button failed again, please fix it by Friday.", Assistant: "Sorry, I will escalate it."}}
	if _, err := service.PreviewMemory(t.Context(), scope, digitalHumanID, MemoryPreviewRequest{Turns: turns}); err == nil {
		t.Fatal("the preview ran without an extractor")
	}
	extractor := &recordingExtractor{proposals: []MemoryExtractionDraft{
		{Key: "export_complaint", MemoryType: memoryruntime.MemoryTypeOpenLoop, Content: "Customer reported the export button failing; fix promised by Friday.", Confidence: 0.9},
		{Key: "persona", MemoryType: memoryruntime.MemoryTypeIdentity, Content: "I am a billing bot.", Confidence: 0.9},
	}}
	service.SetMemoryPreviewExtractor(extractor)
	preview, err := service.PreviewMemory(t.Context(), scope, digitalHumanID, MemoryPreviewRequest{Turns: turns})
	if err != nil {
		t.Fatal(err)
	}
	if extractor.input.Instructions != saved || extractor.input.Source.ModelName != "test-model" ||
		!strings.Contains(extractor.input.Source.Query, "export button") || preview.Instructions != saved {
		t.Fatalf("extractor input = %#v", extractor.input)
	}
	if len(preview.Proposals) != 2 || !preview.Proposals[0].Accepted || preview.Proposals[1].Accepted || preview.Proposals[1].Reason == "" {
		t.Fatalf("preview proposals = %#v", preview.Proposals)
	}
	edited := "Only remember billing questions."
	if _, err := service.PreviewMemory(t.Context(), scope, digitalHumanID, MemoryPreviewRequest{Instructions: &edited, Turns: turns}); err != nil || extractor.input.Instructions != edited {
		t.Fatalf("given instructions were not used: %q, %v", extractor.input.Instructions, err)
	}
	for name, request := range map[string]MemoryPreviewRequest{
		"no turns":   {},
		"empty user": {Turns: []MemoryPreviewTurn{{User: " "}}},
	} {
		if _, err := service.PreviewMemory(t.Context(), scope, digitalHumanID, request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	var events int64
	tx.Model(&MemoryEvent{}).Where("digital_human_id = ?", digitalHumanID).Count(&events)
	if events != 0 {
		t.Fatalf("the preview wrote %d memory events", events)
	}
}
