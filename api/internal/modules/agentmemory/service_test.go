package agentmemory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type fakeStore struct {
	workspaceID uuid.UUID
	slots       map[uuid.UUID]*AgentMemorySlot
	values      map[string]*AgentMemoryValue
	events      []*AgentMemoryEvent
}

func newFakeStore(workspaceID uuid.UUID) *fakeStore {
	return &fakeStore{
		workspaceID: workspaceID,
		slots:       map[uuid.UUID]*AgentMemorySlot{},
		values:      map[string]*AgentMemoryValue{},
	}
}

func (f *fakeStore) WithTransaction(ctx context.Context, fn func(store) error) error {
	return fn(f)
}

func (f *fakeStore) ResolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	if agentID == uuid.Nil {
		return uuid.Nil, gorm.ErrRecordNotFound
	}
	return f.workspaceID, nil
}

func (f *fakeStore) LockAgent(ctx context.Context, agentID uuid.UUID) error {
	if agentID == uuid.Nil {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (f *fakeStore) ListSlots(ctx context.Context, workspaceID, agentID uuid.UUID, enabledOnly bool) ([]*AgentMemorySlot, error) {
	out := []*AgentMemorySlot{}
	for _, slot := range f.slots {
		if slot.WorkspaceID == workspaceID && slot.AgentID == agentID && (!enabledOnly || slot.Enabled) {
			copy := *slot
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (f *fakeStore) CreateSlot(ctx context.Context, slot *AgentMemorySlot) error {
	if slot.ID == uuid.Nil {
		slot.ID = uuid.New()
	}
	copy := *slot
	f.slots[slot.ID] = &copy
	return nil
}

func (f *fakeStore) UpdateSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID, values map[string]interface{}) (*AgentMemorySlot, error) {
	slot := f.slots[slotID]
	if slot == nil || slot.WorkspaceID != workspaceID || slot.AgentID != agentID {
		return nil, gorm.ErrRecordNotFound
	}
	if v, ok := values["description"].(string); ok {
		slot.Description = v
	}
	if v, ok := values["max_chars"].(int); ok {
		slot.MaxChars = v
	}
	if v, ok := values["enabled"].(bool); ok {
		slot.Enabled = v
	}
	if v, ok := values["sort_order"].(int); ok {
		slot.SortOrder = v
	}
	copy := *slot
	return &copy, nil
}

func (f *fakeStore) ListValuesForUser(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]*AgentMemoryValue, error) {
	out := []*AgentMemoryValue{}
	for _, value := range f.values {
		if value.WorkspaceID == workspaceID && value.AgentID == agentID && value.UserScope == userScope && value.UserID == userID {
			copy := *value
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (f *fakeStore) GetValueScoped(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error) {
	value := f.values[valueKey(workspaceID, agentID, slotKey, userScope, userID)]
	if value == nil {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *value
	return &copy, nil
}

func (f *fakeStore) UpsertValue(ctx context.Context, value *AgentMemoryValue) error {
	if value.ID == uuid.Nil {
		value.ID = uuid.New()
	}
	copy := *value
	f.values[valueKey(value.WorkspaceID, value.AgentID, value.SlotKey, value.UserScope, value.UserID)] = &copy
	return nil
}

func (f *fakeStore) CreateEvent(ctx context.Context, event *AgentMemoryEvent) error {
	f.events = append(f.events, event)
	return nil
}

func valueKey(workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) string {
	return workspaceID.String() + ":" + agentID.String() + ":" + slotKey + ":" + userScope + ":" + userID.String()
}

func TestReplaceSlotsValidatesDuplicateKeys(t *testing.T) {
	svc := &Service{repo: newFakeStore(uuid.New())}
	agentID := uuid.New()
	_, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{
		{Key: "profile"},
		{Key: "profile"},
	}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ReplaceSlots error = %v, want ErrInvalidInput", err)
	}
}

func TestReplaceSlotsRejectsInvalidRowsWithoutDisablingExistingSlots(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	slots, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots initial error = %v", err)
	}
	if len(slots) != 1 || !slots[0].Enabled {
		t.Fatalf("initial slots = %#v, want one enabled slot", slots)
	}

	_, err = svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: ""}}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ReplaceSlots invalid error = %v, want ErrInvalidInput", err)
	}
	remaining, err := svc.ListSlots(context.Background(), agentID)
	if err != nil {
		t.Fatalf("ListSlots error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Key != "profile" || !remaining[0].Enabled {
		t.Fatalf("remaining slots = %#v, want profile still enabled", remaining)
	}
}

func TestUpdateValueRequiresExistingEnabledSlot(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	userID := uuid.New()

	_, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, nil, UserScopeAccount, userID, UpdateValueRequest{
		Key:     "profile",
		Content: "likes concise answers",
	}, MutationMetadata{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateValue error = %v, want ErrInvalidInput", err)
	}
}

func TestReadUserMemoryIsolatedByAgentAndUser(t *testing.T) {
	workspaceID := uuid.New()
	store := newFakeStore(workspaceID)
	svc := &Service{repo: store}
	agentA := uuid.New()
	agentB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()

	slots, err := svc.ReplaceSlots(context.Background(), agentA, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots error = %v", err)
	}
	store.slots[uuid.New()] = &AgentMemorySlot{ID: uuid.New(), WorkspaceID: workspaceID, AgentID: agentB, Key: "profile", MaxChars: 1000, Enabled: true}
	store.values[valueKey(workspaceID, agentA, "profile", UserScopeAccount, userA)] = &AgentMemoryValue{
		ID: uuid.New(), WorkspaceID: workspaceID, AgentID: agentA, SlotKey: "profile", UserScope: UserScopeAccount, UserID: userA, Content: "agent A user A",
	}
	store.values[valueKey(workspaceID, agentA, "profile", UserScopeAccount, userB)] = &AgentMemoryValue{
		ID: uuid.New(), WorkspaceID: workspaceID, AgentID: agentA, SlotKey: "profile", UserScope: UserScopeAccount, UserID: userB, Content: "agent A user B",
	}

	entries, err := svc.ReadUserMemory(context.Background(), workspaceID, agentA, []RuntimeSlot{{Key: slots[0].Key, MaxChars: slots[0].MaxChars, Enabled: true}}, UserScopeAccount, userA)
	if err != nil {
		t.Fatalf("ReadUserMemory error = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "agent A user A" {
		t.Fatalf("entries = %#v, want only agent A user A memory", entries)
	}
}

func TestUpdateValueRejectsContentOverSlotLimit(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	userID := uuid.New()
	_, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile", MaxChars: 5}}})
	if err != nil {
		t.Fatalf("ReplaceSlots error = %v", err)
	}

	_, err = svc.UpdateValue(context.Background(), store.workspaceID, agentID, []RuntimeSlot{{Key: "profile", MaxChars: 5, Enabled: true}}, UserScopeAccount, userID, UpdateValueRequest{
		Key:     "profile",
		Content: "too long",
	}, MutationMetadata{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateValue error = %v, want ErrInvalidInput", err)
	}
}

func TestClearValueRedactsClearedContentInEventSnapshots(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	userID := uuid.New()
	slots, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots error = %v", err)
	}
	runtimeSlots := []RuntimeSlot{{Key: slots[0].Key, MaxChars: slots[0].MaxChars, Enabled: true}}
	_, err = svc.UpdateValue(context.Background(), store.workspaceID, agentID, runtimeSlots, UserScopeAccount, userID, UpdateValueRequest{
		Key:     "profile",
		Content: "likes concise answers",
	}, MutationMetadata{})
	if err != nil {
		t.Fatalf("UpdateValue error = %v", err)
	}

	_, err = svc.ClearValue(context.Background(), store.workspaceID, agentID, runtimeSlots, UserScopeAccount, userID, "profile", MutationMetadata{})
	if err != nil {
		t.Fatalf("ClearValue error = %v", err)
	}
	if len(store.events) == 0 {
		t.Fatal("expected clear event")
	}
	event := store.events[len(store.events)-1]
	if event.Action != EventActionValueClear {
		t.Fatalf("last event action = %s, want %s", event.Action, EventActionValueClear)
	}
	var before map[string]interface{}
	if err := json.Unmarshal(event.BeforeSnapshot, &before); err != nil {
		t.Fatalf("unmarshal before snapshot: %v", err)
	}
	if _, ok := before["content"]; ok {
		t.Fatalf("before snapshot leaks cleared content: %#v", before)
	}
	if before["content_redacted"] != true {
		t.Fatalf("before snapshot content_redacted = %#v, want true", before["content_redacted"])
	}
	if before["content_length"] == nil {
		t.Fatalf("before snapshot missing content_length: %#v", before)
	}
}
