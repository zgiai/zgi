package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type fakeStore struct {
	settings map[uuid.UUID]*AccountMemorySetting
	entries  map[uuid.UUID]*AccountMemoryEntry
	events   []*AccountMemoryEvent

	failCreateEvent bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		settings: map[uuid.UUID]*AccountMemorySetting{},
		entries:  map[uuid.UUID]*AccountMemoryEntry{},
		events:   []*AccountMemoryEvent{},
	}
}

func newTestService() (*Service, *fakeStore) {
	store := newFakeStore()
	return &Service{repo: store}, store
}

func (f *fakeStore) WithTransaction(ctx context.Context, fn func(store) error) error {
	settings := cloneSettings(f.settings)
	entries := cloneEntries(f.entries)
	events := append([]*AccountMemoryEvent(nil), f.events...)
	if err := fn(f); err != nil {
		f.settings = settings
		f.entries = entries
		f.events = events
		return err
	}
	return nil
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

func (f *fakeStore) LockAccount(ctx context.Context, accountID uuid.UUID) error {
	if _, ok := f.settings[accountID]; !ok {
		now := time.Now()
		f.settings[accountID] = &AccountMemorySetting{
			AccountID: accountID,
			Enabled:   false,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
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

func (f *fakeStore) GetEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) (*AccountMemoryEntry, error) {
	entry, ok := f.entries[entryID]
	if !ok || entry.AccountID != accountID {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *entry
	return &cp, nil
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
	if memoryType, ok := values["memory_type"].(string); ok {
		entry.MemoryType = memoryType
	}
	if _, ok := values["expires_at"]; ok {
		expiresAt, _ := values["expires_at"].(*time.Time)
		entry.ExpiresAt = expiresAt
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

func (f *fakeStore) CreateEvent(ctx context.Context, event *AccountMemoryEvent) error {
	if f.failCreateEvent {
		return fmt.Errorf("forced event failure")
	}
	cp := *event
	if cp.ID == uuid.Nil {
		cp.ID = uuid.New()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	f.events = append(f.events, &cp)
	return nil
}

func cloneSettings(input map[uuid.UUID]*AccountMemorySetting) map[uuid.UUID]*AccountMemorySetting {
	out := make(map[uuid.UUID]*AccountMemorySetting, len(input))
	for id, setting := range input {
		if setting == nil {
			continue
		}
		cp := *setting
		out[id] = &cp
	}
	return out
}

func cloneEntries(input map[uuid.UUID]*AccountMemoryEntry) map[uuid.UUID]*AccountMemoryEntry {
	out := make(map[uuid.UUID]*AccountMemoryEntry, len(input))
	for id, entry := range input {
		if entry == nil {
			continue
		}
		cp := *entry
		out[id] = &cp
	}
	return out
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
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

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

func TestRenderContextIncludesMemoryPolicyWithoutEntries(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	rendered, err := svc.RenderContext(ctx, accountID, 2000)
	if err != nil {
		t.Fatalf("RenderContext() error = %v", err)
	}
	for _, want := range []string{
		"User memory is enabled",
		"Consider saving",
		"read_user_memory",
		"conflicts with existing memory",
		"ask the user to confirm",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered context = %q, want %q", rendered, want)
		}
	}
}

func TestCreateEntryMergesDuplicateMemory(t *testing.T) {
	ctx := context.Background()
	svc, store := newTestService()
	accountID := uuid.New()

	first, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Call the user Captain.",
		Category: CategoryPreference,
	})
	if err != nil {
		t.Fatalf("CreateEntry(first) error = %v", err)
	}
	second, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Call the user Captain!",
		Category: CategoryOther,
	})
	if err != nil {
		t.Fatalf("CreateEntry(second) error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %s, want merged id %s", second.ID, first.ID)
	}
	state, err := svc.GetMe(ctx, accountID)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(state.Entries))
	}
	if len(store.events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(store.events))
	}
	if got := store.events[1].Action; got != EventActionUpdate {
		t.Fatalf("second event action = %s, want update", got)
	}
}

func TestResolveCategoryUsesSpecificPolicyWhenModelIsBroad(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()

	entry, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "The user's birthday is May 24.",
		Category: CategoryFact,
	})
	if err != nil {
		t.Fatalf("CreateEntry() error = %v", err)
	}
	if entry.Category != CategoryProfile {
		t.Fatalf("entry.Category = %s, want profile", entry.Category)
	}
}

func TestRenderContextGroupsByCategoryPolicy(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	_, _ = svc.CreateEntry(ctx, accountID, CreateEntryRequest{Content: "Stable project fact.", Category: CategoryFact})
	_, _ = svc.CreateEntry(ctx, accountID, CreateEntryRequest{Content: "Always answer in Chinese.", Category: CategoryInstruction})
	_, _ = svc.CreateEntry(ctx, accountID, CreateEntryRequest{Content: "Prefers concise answers.", Category: CategoryPreference})

	rendered, err := svc.RenderContext(ctx, accountID, 1000)
	if err != nil {
		t.Fatalf("RenderContext() error = %v", err)
	}
	instructionIndex := strings.Index(rendered, "Standing instructions:")
	preferenceIndex := strings.Index(rendered, "Preferences:")
	factIndex := strings.Index(rendered, "Stable facts:")
	if instructionIndex < 0 || preferenceIndex < 0 || factIndex < 0 {
		t.Fatalf("rendered context missing sections: %q", rendered)
	}
	if !(instructionIndex < preferenceIndex && preferenceIndex < factIndex) {
		t.Fatalf("rendered section order = %q", rendered)
	}
}

func TestTemporaryMemoryRequiresExpiresAt(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()

	_, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:    "The user needs to pack a briefcase for tomorrow's trip.",
		Category:   CategoryOther,
		MemoryType: MemoryTypeTemporary,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateEntry() error = %v, want ErrInvalidInput", err)
	}
}

func TestRenderContextIncludesOnlyActiveTemporaryMemory(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	active, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:    "The user needs to pack a briefcase for the 2026-05-24 trip.",
		Category:   CategoryOther,
		MemoryType: MemoryTypeTemporary,
		ExpiresAt:  future,
	})
	if err != nil {
		t.Fatalf("CreateEntry(active temporary) error = %v", err)
	}
	expired, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:    "The user needed to submit last week's report.",
		Category:   CategoryOther,
		MemoryType: MemoryTypeTemporary,
		ExpiresAt:  past,
	})
	if err != nil {
		t.Fatalf("CreateEntry(expired temporary) error = %v", err)
	}

	rendered, err := svc.RenderContext(ctx, accountID, 1000)
	if err != nil {
		t.Fatalf("RenderContext() error = %v", err)
	}
	if !strings.Contains(rendered, "Temporary memory:") || !strings.Contains(rendered, active.Content) {
		t.Fatalf("rendered context = %q, want active temporary memory", rendered)
	}
	if strings.Contains(rendered, expired.Content) {
		t.Fatalf("rendered context = %q, want expired temporary memory omitted", rendered)
	}
}

func TestRenderContextReturnsEmptyWhenMemoryDisabled(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if _, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Call the user Captain.",
		Category: CategoryPreference,
	}); err != nil {
		t.Fatalf("CreateEntry() error = %v", err)
	}

	rendered, err := svc.RenderContext(ctx, accountID, 1000)
	if err != nil {
		t.Fatalf("RenderContext() error = %v", err)
	}
	if rendered != "" {
		t.Fatalf("RenderContext() = %q, want empty when disabled", rendered)
	}
}

func TestListTemporaryEntriesCanReturnExpiredHistory(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	expired, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:    "The user needed to call Alice yesterday.",
		MemoryType: MemoryTypeTemporary,
		ExpiresAt:  past,
	})
	if err != nil {
		t.Fatalf("CreateEntry(expired) error = %v", err)
	}
	entries, err := svc.ListTemporaryEntries(ctx, accountID, memoryStatusExpired, 10)
	if err != nil {
		t.Fatalf("ListTemporaryEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].ID != expired.ID || entries[0].Status != memoryStatusExpired {
		t.Fatalf("entry = %#v, want expired entry %s", entries[0], expired.ID)
	}
}

func TestCreateEntryRejectsWhenAccountLimitReached(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()

	for i := 0; i < maxMemoryEntriesPerAccount; i++ {
		_, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
			Content:  "Unique memory " + uuid.NewString(),
			Category: CategoryFact,
		})
		if err != nil {
			t.Fatalf("CreateEntry(%d) error = %v", i, err)
		}
	}
	_, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "One memory too many " + uuid.NewString(),
		Category: CategoryFact,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateEntry(over limit) error = %v, want ErrInvalidInput", err)
	}
}

func TestMemoryMutationsRecordAuditEvents(t *testing.T) {
	ctx := context.Background()
	svc, store := newTestService()
	accountID := uuid.New()

	entry, err := svc.CreateEntryWithMetadata(ctx, accountID, CreateEntryRequest{
		Content:  "Prefers short answers.",
		Category: CategoryPreference,
	}, MutationMetadata{ActorType: EventActorModel, Source: EventSourceAIChat})
	if err != nil {
		t.Fatalf("CreateEntryWithMetadata() error = %v", err)
	}
	entryID, _ := uuid.Parse(entry.ID)
	enabled := false
	if _, err := svc.UpdateEntry(ctx, accountID, entryID, UpdateEntryRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("UpdateEntry() error = %v", err)
	}
	if err := svc.DeleteEntry(ctx, accountID, entryID); err != nil {
		t.Fatalf("DeleteEntry() error = %v", err)
	}
	if len(store.events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(store.events))
	}
	if store.events[0].ActorType != EventActorModel || store.events[0].Source != EventSourceAIChat {
		t.Fatalf("first event actor/source = %s/%s", store.events[0].ActorType, store.events[0].Source)
	}
	if store.events[1].Action != EventActionDisable {
		t.Fatalf("second event action = %s, want disable", store.events[1].Action)
	}
	if store.events[2].Action != EventActionDelete {
		t.Fatalf("third event action = %s, want delete", store.events[2].Action)
	}
}

func TestCreateEntryRollsBackWhenAuditEventFails(t *testing.T) {
	ctx := context.Background()
	svc, store := newTestService()
	accountID := uuid.New()
	store.failCreateEvent = true

	_, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Prefers short answers.",
		Category: CategoryPreference,
	})
	if err == nil {
		t.Fatal("CreateEntry() error = nil, want forced audit failure")
	}
	if len(store.entries) != 0 {
		t.Fatalf("len(entries) = %d, want rollback to 0", len(store.entries))
	}
	if len(store.events) != 0 {
		t.Fatalf("len(events) = %d, want rollback to 0", len(store.events))
	}
}

func stringPtr(value string) *string {
	return &value
}
