package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type fakeStore struct {
	settings map[uuid.UUID]*AccountMemorySetting
	entries  map[uuid.UUID]*AccountMemoryEntry
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		settings: map[uuid.UUID]*AccountMemorySetting{},
		entries:  map[uuid.UUID]*AccountMemoryEntry{},
	}
}

func newTestService() (*Service, *fakeStore) {
	store := newFakeStore()
	return &Service{repo: store}, store
}

func (f *fakeStore) GetSetting(ctx context.Context, accountID uuid.UUID) (*AccountMemorySetting, error) {
	setting, ok := f.settings[accountID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *setting
	return &cp, nil
}

func (f *fakeStore) UpsertSetting(ctx context.Context, setting *AccountMemorySetting) error {
	cp := *setting
	f.settings[setting.AccountID] = &cp
	return nil
}

func (f *fakeStore) ListEntries(ctx context.Context, accountID uuid.UUID, enabledOnly bool) ([]*AccountMemoryEntry, error) {
	out := []*AccountMemoryEntry{}
	for _, entry := range f.entries {
		if entry.AccountID != accountID {
			continue
		}
		if enabledOnly && !entry.Enabled {
			continue
		}
		cp := *entry
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeStore) CreateEntry(ctx context.Context, entry *AccountMemoryEntry) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now
	cp := *entry
	f.entries[entry.ID] = &cp
	return nil
}

func (f *fakeStore) UpdateEntryScoped(ctx context.Context, accountID, entryID uuid.UUID, values map[string]interface{}) (*AccountMemoryEntry, error) {
	entry, ok := f.entries[entryID]
	if !ok || entry.AccountID != accountID {
		return nil, gorm.ErrRecordNotFound
	}
	if content, ok := values["content"].(string); ok {
		entry.Content = content
	}
	if category, ok := values["category"].(string); ok {
		entry.Category = category
	}
	if enabled, ok := values["enabled"].(bool); ok {
		entry.Enabled = enabled
	}
	if updatedAt, ok := values["updated_at"].(time.Time); ok {
		entry.UpdatedAt = updatedAt
	}
	cp := *entry
	return &cp, nil
}

func (f *fakeStore) DeleteEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) error {
	entry, ok := f.entries[entryID]
	if !ok || entry.AccountID != accountID {
		return gorm.ErrRecordNotFound
	}
	delete(f.entries, entryID)
	return nil
}

func TestServiceScopesEntriesToCurrentAccount(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	ownerID := uuid.New()
	otherID := uuid.New()

	entry, err := svc.CreateEntry(ctx, ownerID, CreateEntryRequest{
		Content:  "Call the user Captain.",
		Category: CategoryPreference,
	})
	if err != nil {
		t.Fatalf("CreateEntry() error = %v", err)
	}
	entryID, err := uuid.Parse(entry.ID)
	if err != nil {
		t.Fatalf("parse entry id: %v", err)
	}

	_, err = svc.UpdateEntry(ctx, otherID, entryID, UpdateEntryRequest{Content: stringPtr("Nope")})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateEntry() error = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteEntry(ctx, otherID, entryID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteEntry() error = %v, want ErrNotFound", err)
	}

	state, err := svc.GetMe(ctx, ownerID)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(state.Entries))
	}
	if state.Entries[0].Content != "Call the user Captain." {
		t.Fatalf("entry content = %q", state.Entries[0].Content)
	}
}

func TestRenderContextUsesEnabledEntriesAndBudget(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()

	first, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Prefers concise answers.",
		Category: CategoryPreference,
	})
	if err != nil {
		t.Fatalf("CreateEntry(first) error = %v", err)
	}
	second, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "This disabled item should not render.",
		Category: CategoryFact,
	})
	if err != nil {
		t.Fatalf("CreateEntry(second) error = %v", err)
	}
	secondID, _ := uuid.Parse(second.ID)
	disabled := false
	if _, err := svc.UpdateEntry(ctx, accountID, secondID, UpdateEntryRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable second entry: %v", err)
	}

	rendered, err := svc.RenderContext(ctx, accountID, 1000)
	if err != nil {
		t.Fatalf("RenderContext() error = %v", err)
	}
	if !strings.Contains(rendered, first.Content) {
		t.Fatalf("rendered context %q does not contain enabled entry %q", rendered, first.Content)
	}
	if strings.Contains(rendered, second.Content) {
		t.Fatalf("rendered context %q contains disabled entry %q", rendered, second.Content)
	}

	short, err := svc.RenderContext(ctx, accountID, len("User memory"))
	if err != nil {
		t.Fatalf("RenderContext(short) error = %v", err)
	}
	if strings.Contains(short, first.Content) {
		t.Fatalf("short rendered context = %q, want budget to omit entry", short)
	}
}

func stringPtr(value string) *string {
	return &value
}
