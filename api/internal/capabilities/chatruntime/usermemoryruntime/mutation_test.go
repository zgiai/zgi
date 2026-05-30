package usermemoryruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/memory"
)

type fakeMemoryService struct {
	created       int
	updated       int
	deleted       int
	entry         memory.MemoryEntryResponse
	lastCreateReq memory.CreateEntryRequest
	lastUpdateReq memory.UpdateEntryRequest
	lastAccountID uuid.UUID
}

func (f *fakeMemoryService) IsEnabled(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func (f *fakeMemoryService) GetModelState(context.Context, uuid.UUID) (*memory.MemoryMeResponse, error) {
	return &memory.MemoryMeResponse{Enabled: true, Entries: []memory.MemoryEntryResponse{f.entry}}, nil
}

func (f *fakeMemoryService) CreateEntryWithMetadata(_ context.Context, accountID uuid.UUID, req memory.CreateEntryRequest, _ memory.MutationMetadata) (*memory.MemoryEntryResponse, error) {
	f.created++
	f.lastAccountID = accountID
	f.lastCreateReq = req
	return &f.entry, nil
}

func (f *fakeMemoryService) UpdateEntryWithMetadata(_ context.Context, accountID uuid.UUID, _ uuid.UUID, req memory.UpdateEntryRequest, _ memory.MutationMetadata) (*memory.MemoryEntryResponse, error) {
	f.updated++
	f.lastAccountID = accountID
	f.lastUpdateReq = req
	return &f.entry, nil
}

func (f *fakeMemoryService) DeleteEntryWithMetadata(_ context.Context, accountID uuid.UUID, _ uuid.UUID, _ memory.MutationMetadata) error {
	f.deleted++
	f.lastAccountID = accountID
	return nil
}

func TestValidateDecisionAskConfirmationDoesNotMutate(t *testing.T) {
	_, _, err := ValidateDecision(Decision{Action: ActionAskConfirmation, Content: "Call me Captain"}, &State{})
	if err == nil {
		t.Fatal("ValidateDecision() error = nil, want no-mutation error")
	}
}

func TestApplyDecisionCreatesUserMemory(t *testing.T) {
	service := &fakeMemoryService{entry: memory.MemoryEntryResponse{
		ID:         uuid.NewString(),
		Content:    "Call the user Captain.",
		Category:   memory.CategoryPreference,
		MemoryType: memory.MemoryTypeLongTerm,
	}}
	result, trace, err := ApplyDecision(context.Background(), PreflightRequest{
		AccountID:     uuid.New(),
		MemoryService: service,
		State:         &State{},
	}, Decision{
		Action:     ActionCreate,
		Content:    service.entry.Content,
		Category:   memory.CategoryPreference,
		MemoryType: memory.MemoryTypeLongTerm,
	})
	if err != nil {
		t.Fatalf("ApplyDecision() error = %v", err)
	}
	if service.created != 1 {
		t.Fatalf("created = %d, want 1", service.created)
	}
	if trace.SkillID != "" {
		t.Fatalf("trace.SkillID = %q, want empty non-skill trace", trace.SkillID)
	}
	if result["action"] != ActionCreate {
		t.Fatalf("result action = %#v, want create", result["action"])
	}
}

func TestApplyDecisionLongTermIgnoresNullishExpiresAt(t *testing.T) {
	service := &fakeMemoryService{entry: memory.MemoryEntryResponse{
		ID:         uuid.NewString(),
		Content:    "User prefers concise answers.",
		Category:   memory.CategoryPreference,
		MemoryType: memory.MemoryTypeLongTerm,
	}}
	_, _, err := ApplyDecision(context.Background(), PreflightRequest{
		AccountID:     uuid.New(),
		MemoryService: service,
		State:         &State{},
	}, Decision{
		Action:     ActionCreate,
		Content:    service.entry.Content,
		Category:   memory.CategoryPreference,
		MemoryType: memory.MemoryTypeLongTerm,
		ExpiresAt:  "null",
	})
	if err != nil {
		t.Fatalf("ApplyDecision() error = %v", err)
	}
	if service.lastCreateReq.MemoryType != memory.MemoryTypeLongTerm {
		t.Fatalf("memory_type = %q, want long_term", service.lastCreateReq.MemoryType)
	}
	if service.lastCreateReq.ExpiresAt != "" {
		t.Fatalf("expires_at = %q, want empty", service.lastCreateReq.ExpiresAt)
	}
}

func TestApplyDecisionUsesStateAccountIDFallback(t *testing.T) {
	accountID := uuid.New()
	service := &fakeMemoryService{entry: memory.MemoryEntryResponse{
		ID:         uuid.NewString(),
		Content:    "User prefers risk-first reviews.",
		Category:   memory.CategoryPreference,
		MemoryType: memory.MemoryTypeLongTerm,
	}}
	_, _, err := ApplyDecision(context.Background(), PreflightRequest{
		MemoryService: service,
		State:         &State{AccountID: accountID},
	}, Decision{
		Action:     ActionCreate,
		Content:    service.entry.Content,
		Category:   memory.CategoryPreference,
		MemoryType: memory.MemoryTypeLongTerm,
	})
	if err != nil {
		t.Fatalf("ApplyDecision() error = %v", err)
	}
	if service.lastAccountID != accountID {
		t.Fatalf("accountID = %s, want state fallback %s", service.lastAccountID, accountID)
	}
}

func TestApplyDecisionDeletesExistingUserMemory(t *testing.T) {
	entryID := uuid.NewString()
	service := &fakeMemoryService{entry: memory.MemoryEntryResponse{ID: entryID}}
	_, _, err := ApplyDecision(context.Background(), PreflightRequest{
		AccountID:     uuid.New(),
		MemoryService: service,
		State: &State{Entries: []memory.MemoryEntryResponse{
			{ID: entryID},
		}},
	}, Decision{
		Action:  ActionDelete,
		EntryID: entryID,
	})
	if err != nil {
		t.Fatalf("ApplyDecision() error = %v", err)
	}
	if service.deleted != 1 {
		t.Fatalf("deleted = %d, want 1", service.deleted)
	}
}
