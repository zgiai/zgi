package agentmemory

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
	workspaceID uuid.UUID
	slots       map[uuid.UUID]*AgentMemorySlot
	values      map[string]*AgentMemoryValue
	events      []*AgentMemoryEvent
	undos       map[uuid.UUID]*AgentMemoryUndoRecord
	jobs        map[uuid.UUID]*AgentMemoryExtractionJob
	subjects    map[string]*AgentMemorySubjectState
}

func newFakeStore(workspaceID uuid.UUID) *fakeStore {
	return &fakeStore{
		workspaceID: workspaceID,
		slots:       map[uuid.UUID]*AgentMemorySlot{},
		values:      map[string]*AgentMemoryValue{},
		undos:       map[uuid.UUID]*AgentMemoryUndoRecord{},
		jobs:        map[uuid.UUID]*AgentMemoryExtractionJob{},
		subjects:    map[string]*AgentMemorySubjectState{},
	}
}

func (f *fakeStore) WithTransaction(ctx context.Context, fn func(store) error) error {
	tx := f.clone()
	if err := fn(tx); err != nil {
		return err
	}
	*f = *tx
	return nil
}

func (f *fakeStore) clone() *fakeStore {
	cloned := newFakeStore(f.workspaceID)
	for id, slot := range f.slots {
		value := *slot
		cloned.slots[id] = &value
	}
	for key, stored := range f.values {
		value := *stored
		cloned.values[key] = &value
	}
	for _, event := range f.events {
		value := *event
		cloned.events = append(cloned.events, &value)
	}
	for id, record := range f.undos {
		value := *record
		cloned.undos[id] = &value
	}
	for id, job := range f.jobs {
		value := *job
		cloned.jobs[id] = &value
	}
	for key, state := range f.subjects {
		value := *state
		cloned.subjects[key] = &value
	}
	return cloned
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
	if v, ok := values["name"].(string); ok {
		slot.Name = v
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

func (f *fakeStore) DeleteSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID) error {
	slot := f.slots[slotID]
	if slot == nil || slot.WorkspaceID != workspaceID || slot.AgentID != agentID {
		return gorm.ErrRecordNotFound
	}
	delete(f.slots, slotID)
	return nil
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

func (f *fakeStore) ListValuesForAgent(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*AgentMemoryValue, error) {
	out := []*AgentMemoryValue{}
	for _, value := range f.values {
		if value.WorkspaceID == workspaceID && value.AgentID == agentID {
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

func (f *fakeStore) GetValueScopedForUpdate(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error) {
	return f.GetValueScoped(ctx, workspaceID, agentID, slotKey, userScope, userID)
}

func (f *fakeStore) UpsertValue(ctx context.Context, value *AgentMemoryValue) error {
	if value.ID == uuid.Nil {
		value.ID = uuid.New()
	}
	copy := *value
	f.values[valueKey(value.WorkspaceID, value.AgentID, value.SlotKey, value.UserScope, value.UserID)] = &copy
	return nil
}

func (f *fakeStore) CreateValue(ctx context.Context, value *AgentMemoryValue) error {
	return f.UpsertValue(ctx, value)
}

func (f *fakeStore) UpdateValueCAS(ctx context.Context, value *AgentMemoryValue, expectedRevision int64) error {
	key := valueKey(value.WorkspaceID, value.AgentID, value.SlotKey, value.UserScope, value.UserID)
	current := f.values[key]
	if current == nil || current.Revision != expectedRevision {
		return ErrConflict
	}
	copy := *value
	f.values[key] = &copy
	return nil
}

func (f *fakeStore) DeleteValueCAS(ctx context.Context, value *AgentMemoryValue, expectedRevision int64) error {
	key := valueKey(value.WorkspaceID, value.AgentID, value.SlotKey, value.UserScope, value.UserID)
	current := f.values[key]
	if current == nil || current.Revision != expectedRevision {
		return ErrConflict
	}
	delete(f.values, key)
	return nil
}

func (f *fakeStore) DeleteValuesForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error {
	for key, value := range f.values {
		if value.WorkspaceID == workspaceID && value.AgentID == agentID && value.UserScope == userScope && value.UserID == userID {
			delete(f.values, key)
		}
	}
	return nil
}

func (f *fakeStore) DeleteUndoForSlot(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, slotKey string) error {
	for id, record := range f.undos {
		if record.WorkspaceID == workspaceID && record.AgentID == agentID && record.UserScope == userScope && record.UserID == userID && record.SlotKey == slotKey {
			delete(f.undos, id)
		}
	}
	return nil
}

func (f *fakeStore) DeleteUndoForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error {
	for id, record := range f.undos {
		if record.WorkspaceID == workspaceID && record.AgentID == agentID && record.UserScope == userScope && record.UserID == userID {
			delete(f.undos, id)
		}
	}
	return nil
}

func (f *fakeStore) CreateUndoRecord(ctx context.Context, record *AgentMemoryUndoRecord) error {
	copy := *record
	f.undos[record.OperationID] = &copy
	return nil
}
func (f *fakeStore) GetUndoRecordForUpdate(ctx context.Context, operationID, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentMemoryUndoRecord, error) {
	record := f.undos[operationID]
	if record == nil || record.WorkspaceID != workspaceID || record.AgentID != agentID || record.UserScope != userScope || record.UserID != userID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *record
	return &copy, nil
}
func (f *fakeStore) DeleteUndoRecord(ctx context.Context, operationID uuid.UUID) error {
	delete(f.undos, operationID)
	return nil
}
func (f *fakeStore) FindUndoExpiry(ctx context.Context, operationID uuid.UUID) (*time.Time, error) {
	record := f.undos[operationID]
	if record == nil {
		return nil, gorm.ErrRecordNotFound
	}
	expires := record.ExpiresAt
	return &expires, nil
}

func subjectKey(workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) string {
	return workspaceID.String() + ":" + agentID.String() + ":" + userScope + ":" + userID.String()
}
func (f *fakeStore) LockSubjectState(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentMemorySubjectState, error) {
	key := subjectKey(workspaceID, agentID, userScope, userID)
	state := f.subjects[key]
	if state == nil {
		state = &AgentMemorySubjectState{ID: uuid.New(), WorkspaceID: workspaceID, AgentID: agentID, UserScope: userScope, UserID: userID}
		f.subjects[key] = state
	}
	copy := *state
	return &copy, nil
}
func (f *fakeStore) ListSubjectStatesForAgentForUpdate(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*AgentMemorySubjectState, error) {
	out := make([]*AgentMemorySubjectState, 0)
	for _, state := range f.subjects {
		if state.WorkspaceID == workspaceID && state.AgentID == agentID {
			copy := *state
			out = append(out, &copy)
		}
	}
	return out, nil
}
func (f *fakeStore) UpdateSubjectEpoch(ctx context.Context, state *AgentMemorySubjectState, epoch int64) error {
	key := subjectKey(state.WorkspaceID, state.AgentID, state.UserScope, state.UserID)
	stored := f.subjects[key]
	stored.MemoryEpoch = epoch
	if state.ExtractionCutoffAt == nil {
		stored.ExtractionCutoffAt = nil
	} else {
		cutoff := *state.ExtractionCutoffAt
		stored.ExtractionCutoffAt = &cutoff
	}
	return nil
}
func (f *fakeStore) CancelPendingJobsForSubject(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) error {
	for _, job := range f.jobs {
		if job.WorkspaceID == workspaceID && job.AgentID == agentID && job.UserScope == userScope && job.UserID == userID && (job.Status == ExtractionJobPending || job.Status == ExtractionJobQueued || job.Status == ExtractionJobFailed) {
			job.Status = ExtractionJobCancelled
		}
	}
	return nil
}
func (f *fakeStore) CreateExtractionJob(ctx context.Context, job *AgentMemoryExtractionJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	copy := *job
	f.jobs[job.ID] = &copy
	return nil
}
func (f *fakeStore) GetExtractionJob(ctx context.Context, id uuid.UUID) (*AgentMemoryExtractionJob, error) {
	job := f.jobs[id]
	if job == nil {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *job
	return &copy, nil
}
func (f *fakeStore) GetExtractionJobByIdempotency(ctx context.Context, key string) (*AgentMemoryExtractionJob, error) {
	for _, job := range f.jobs {
		if job.IdempotencyKey == key {
			copy := *job
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeStore) SupersedeConversationJobs(ctx context.Context, current *AgentMemoryExtractionJob) error {
	for _, job := range f.jobs {
		if job.ID != current.ID && job.ConversationID == current.ConversationID && (job.Status == ExtractionJobPending || job.Status == ExtractionJobQueued || job.Status == ExtractionJobFailed) {
			job.Status = ExtractionJobCancelled
		}
	}
	return nil
}
func (f *fakeStore) EarliestConversationForceAt(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID, conversationID uuid.UUID) (*time.Time, error) {
	var earliest *time.Time
	for _, job := range f.jobs {
		if job.WorkspaceID == workspaceID && job.AgentID == agentID && job.UserScope == userScope && job.UserID == userID && job.ConversationID == conversationID && (job.Status == ExtractionJobPending || job.Status == ExtractionJobQueued || job.Status == ExtractionJobFailed) {
			if earliest == nil || job.ForceAt.Before(*earliest) {
				value := job.ForceAt
				earliest = &value
			}
		}
	}
	if earliest == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return earliest, nil
}
func (f *fakeStore) ClaimExtractionJob(ctx context.Context, id uuid.UUID, epoch int64) (*AgentMemoryExtractionJob, error) {
	job := f.jobs[id]
	if job == nil || job.MemoryEpoch != epoch {
		return nil, ErrConflict
	}
	job.Status = ExtractionJobRunning
	job.AttemptCount++
	copy := *job
	return &copy, nil
}
func (f *fakeStore) FinishExtractionJob(ctx context.Context, id uuid.UUID, status, errorCode string) error {
	job := f.jobs[id]
	if job == nil {
		return gorm.ErrRecordNotFound
	}
	job.Status = status
	job.ErrorCode = errorCode
	return nil
}
func (f *fakeStore) RescheduleExtractionJob(ctx context.Context, id uuid.UUID, errorCode string, scheduledAt time.Time) error {
	job := f.jobs[id]
	if job == nil {
		return gorm.ErrRecordNotFound
	}
	job.Status = ExtractionJobFailed
	job.ErrorCode = errorCode
	job.ScheduledAt = scheduledAt
	return nil
}
func (f *fakeStore) ListDueExtractionJobs(ctx context.Context, limit int) ([]*AgentMemoryExtractionJob, error) {
	out := []*AgentMemoryExtractionJob{}
	now := time.Now()
	for _, job := range f.jobs {
		if (job.Status == ExtractionJobPending || job.Status == ExtractionJobFailed) && !job.ScheduledAt.After(now) {
			copy := *job
			out = append(out, &copy)
		}
	}
	return out, nil
}
func (f *fakeStore) DeleteTerminalExtractionJobs(ctx context.Context, finishedBefore time.Time, limit int) (int64, error) {
	var deleted int64
	for id, job := range f.jobs {
		if limit > 0 && deleted >= int64(limit) {
			break
		}
		terminal := job.Status == ExtractionJobCompleted || job.Status == ExtractionJobCancelled || job.Status == ExtractionJobExhausted
		if terminal && job.FinishedAt != nil && job.FinishedAt.Before(finishedBefore) {
			delete(f.jobs, id)
			deleted++
		}
	}
	return deleted, nil
}

func (f *fakeStore) CreateEvent(ctx context.Context, event *AgentMemoryEvent) error {
	if event.OperationID != nil {
		for _, existing := range f.events {
			if existing.OperationID != nil && *existing.OperationID == *event.OperationID {
				return gorm.ErrDuplicatedKey
			}
		}
	}
	copy := *event
	f.events = append(f.events, &copy)
	return nil
}

func (f *fakeStore) GetEventByOperationID(ctx context.Context, operationID uuid.UUID) (*AgentMemoryEvent, error) {
	for _, event := range f.events {
		if event.OperationID != nil && *event.OperationID == operationID {
			copy := *event
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func valueKey(workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) string {
	return workspaceID.String() + ":" + agentID.String() + ":" + slotKey + ":" + userScope + ":" + userID.String()
}

func TestMutateValuesAppliesAtomicMultiSlotBatch(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID, messageID := uuid.New(), uuid.New(), uuid.New()
	slots := []RuntimeSlot{
		{Key: "profile", Enabled: true, MaxChars: 500},
		{Key: "preferences", Enabled: true, MaxChars: 500},
	}
	response, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, MutateValuesRequest{Operations: []ValueMutation{
		{Action: MutationActionUpsert, Key: "profile", Content: "Name is Ada.", Mode: MutationModeExplicit, OperationID: uuid.New()},
		{Action: MutationActionUpsert, Key: "preferences", Content: "Prefers concise replies.", Mode: MutationModeExplicit, OperationID: uuid.New()},
	}}, MutationMetadata{ActorType: EventActorModel, Source: EventSourceAgent, SourceMessageID: &messageID})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Operations) != 2 || len(store.values) != 2 || len(store.events) != 2 {
		t.Fatalf("response=%#v values=%d events=%d", response, len(store.values), len(store.events))
	}
}

func TestMutateValuesConflictLeavesEntireBatchUnchanged(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID := uuid.New(), uuid.New()
	slots := []RuntimeSlot{{Key: "profile", Enabled: true, MaxChars: 500}, {Key: "preferences", Enabled: true, MaxChars: 500}}
	for _, key := range []string{"profile", "preferences"} {
		_ = store.CreateValue(context.Background(), &AgentMemoryValue{WorkspaceID: store.workspaceID, AgentID: agentID, SlotKey: key, UserScope: UserScopeAccount, UserID: userID, Content: "before-" + key, Revision: 1})
	}
	_, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, MutateValuesRequest{Operations: []ValueMutation{
		{Action: MutationActionUpsert, Key: "profile", Content: "after-profile", Mode: MutationModeExplicit, ExpectedRevision: 1, OperationID: uuid.New()},
		{Action: MutationActionUpsert, Key: "preferences", Content: "after-preferences", Mode: MutationModeExplicit, ExpectedRevision: 0, OperationID: uuid.New()},
	}}, MutationMetadata{})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	for _, key := range []string{"profile", "preferences"} {
		value := store.values[valueKey(store.workspaceID, agentID, key, UserScopeAccount, userID)]
		if value.Content != "before-"+key || value.Revision != 1 {
			t.Fatalf("%s changed after rollback: %#v", key, value)
		}
	}
	if len(store.events) != 0 {
		t.Fatalf("events = %d after rollback", len(store.events))
	}
}

func TestMutateValuesSameContentIsIdempotentWithoutRevision(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID, operationID := uuid.New(), uuid.New(), uuid.New()
	slots := []RuntimeSlot{{Key: "profile", Enabled: true, MaxChars: 500}}
	_ = store.CreateValue(context.Background(), &AgentMemoryValue{WorkspaceID: store.workspaceID, AgentID: agentID, SlotKey: "profile", UserScope: UserScopeAccount, UserID: userID, Content: "same", Revision: 4})
	response, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, MutateValuesRequest{Operations: []ValueMutation{{
		Action: MutationActionUpsert, Key: "profile", Content: "same", Mode: MutationModeExplicit, ExpectedRevision: 4, OperationID: operationID,
	}}}, MutationMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Operations[0].Status != MutationStatusUnchanged || response.Operations[0].Revision != 4 {
		t.Fatalf("response = %#v", response)
	}
	replayed, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, MutateValuesRequest{Operations: []ValueMutation{{
		Action: MutationActionUpsert, Key: "profile", Content: "same", Mode: MutationModeExplicit, ExpectedRevision: 4, OperationID: operationID,
	}}}, MutationMetadata{})
	if err != nil || replayed.Operations[0].Status != MutationStatusUnchanged || len(store.events) != 1 {
		t.Fatalf("replay=%#v err=%v events=%d", replayed, err, len(store.events))
	}
}

func TestMutateValuesClearMissingAdvancesCutoffOnce(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID := uuid.New(), uuid.New()
	staleUndoID := uuid.New()
	store.undos[staleUndoID] = &AgentMemoryUndoRecord{
		OperationID: staleUndoID, WorkspaceID: store.workspaceID, AgentID: agentID,
		UserScope: UserScopeAccount, UserID: userID, SlotKey: "profile",
	}
	cutoff := time.Now().Add(-time.Minute)
	slots := []RuntimeSlot{{Key: "profile", Enabled: true, MaxChars: 500}, {Key: "preferences", Enabled: true, MaxChars: 500}}
	response, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, MutateValuesRequest{Operations: []ValueMutation{
		{Action: MutationActionClear, Key: "profile", Mode: MutationModeExplicit, OperationID: uuid.New()},
		{Action: MutationActionClear, Key: "preferences", Mode: MutationModeExplicit, OperationID: uuid.New()},
	}}, MutationMetadata{SourceCompletedAt: &cutoff})
	if err != nil {
		t.Fatal(err)
	}
	state := store.subjects[subjectKey(store.workspaceID, agentID, UserScopeAccount, userID)]
	if state.MemoryEpoch != 1 || state.ExtractionCutoffAt == nil || !state.ExtractionCutoffAt.Equal(cutoff) {
		t.Fatalf("subject state = %#v", state)
	}
	if len(response.Operations) != 2 || response.Operations[0].Status != MutationStatusUnchanged || response.Operations[1].Status != MutationStatusUnchanged {
		t.Fatalf("response = %#v", response)
	}
	if store.undos[staleUndoID] != nil {
		t.Fatal("missing-value clear did not remove stale undo record")
	}
}

func TestMutateValuesProactiveCreatesUndoReceipt(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID, operationID := uuid.New(), uuid.New(), uuid.New()
	epoch := int64(0)
	response, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, []RuntimeSlot{{Key: "profile", Enabled: true, MaxChars: 500}}, UserScopeAccount, userID, MutateValuesRequest{Operations: []ValueMutation{{
		Action: MutationActionUpsert, Key: "profile", Content: "Prefers diagrams.", Mode: MutationModeProactive, OperationID: operationID,
	}}}, MutationMetadata{MemoryEpoch: &epoch})
	if err != nil {
		t.Fatal(err)
	}
	if response.Operations[0].SourceKind != SourceKindAutomatic || response.Operations[0].UndoableUntil == nil || store.undos[operationID] == nil {
		t.Fatalf("response=%#v undo=%#v", response, store.undos[operationID])
	}
}

func TestMutateValuesProactiveRequiresCurrentEpoch(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID := uuid.New(), uuid.New()
	slots := []RuntimeSlot{{Key: "profile", Enabled: true, MaxChars: 500}}
	request := MutateValuesRequest{Operations: []ValueMutation{{
		Action: MutationActionUpsert, Key: "profile", Content: "Prefers diagrams.", Mode: MutationModeProactive, OperationID: uuid.New(),
	}}}
	if _, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, request, MutationMetadata{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing epoch error = %v, want ErrInvalidInput", err)
	}
	epoch, err := svc.ReadSubjectEpoch(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, request, MutationMetadata{MemoryEpoch: &epoch}); err != nil {
		t.Fatalf("initial proactive mutation error = %v", err)
	}
	if err := svc.ClearAllValues(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID, MutationMetadata{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MutateValues(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, request, MutationMetadata{MemoryEpoch: &epoch}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale epoch error = %v, want ErrConflict", err)
	}
	if len(store.values) != 0 {
		t.Fatalf("stale proactive mutation recreated values: %#v", store.values)
	}
}

func TestReplaceSlotsInvalidatesSubjectsWhenSlotIsRemoved(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID := uuid.New(), uuid.New()
	actorID := uuid.New()
	if _, err := svc.ReplaceSlots(context.Background(), agentID, actorID, ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}, {Key: "preferences"}}}); err != nil {
		t.Fatal(err)
	}
	state, err := store.LockSubjectState(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID)
	if err != nil {
		t.Fatal(err)
	}
	jobID := uuid.New()
	store.jobs[jobID] = &AgentMemoryExtractionJob{ID: jobID, WorkspaceID: store.workspaceID, AgentID: agentID, UserScope: UserScopeAccount, UserID: userID, Status: ExtractionJobPending, MemoryEpoch: state.MemoryEpoch}
	profileID := uuid.Nil
	for _, slot := range store.slots {
		if slot.AgentID == agentID && slot.Key == "profile" {
			profileID = slot.ID
			break
		}
	}
	if profileID == uuid.Nil {
		t.Fatal("profile slot was not created")
	}
	if _, err := svc.ReplaceSlots(context.Background(), agentID, actorID, ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{ID: profileID.String(), Key: "profile"}}}); err != nil {
		t.Fatal(err)
	}
	updatedState := store.subjects[subjectKey(store.workspaceID, agentID, UserScopeAccount, userID)]
	if updatedState.MemoryEpoch != state.MemoryEpoch+1 || store.jobs[jobID].Status != ExtractionJobCancelled {
		t.Fatalf("subject/job = %#v/%#v, want invalidated and cancelled", updatedState, store.jobs[jobID])
	}
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

func TestReplaceSlotsRejectsKeyRenameByID(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	slots, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots initial error = %v", err)
	}

	_, err = svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{
		ID:  slots[0].ID,
		Key: "profile_new",
	}}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ReplaceSlots rename error = %v, want ErrInvalidInput", err)
	}
}

func TestReplaceSlotsDeletesOmittedSlots(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	_, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}, {Key: "preference"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots initial error = %v", err)
	}

	slots, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots delete error = %v", err)
	}
	if len(slots) != 1 || slots[0].Key != "profile" {
		t.Fatalf("slots = %#v, want only profile", slots)
	}
	if _, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "preference"}}}); err != nil {
		t.Fatalf("ReplaceSlots recreate deleted key error = %v", err)
	}
}

func TestReplaceSlotsLimitsSlotCountAndDescriptionLength(t *testing.T) {
	svc := &Service{repo: newFakeStore(uuid.New())}
	agentID := uuid.New()
	_, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{
		{Key: "memory_1"}, {Key: "memory_2"}, {Key: "memory_3"}, {Key: "memory_4"}, {Key: "memory_5"}, {Key: "memory_6"},
	}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ReplaceSlots too many error = %v, want ErrInvalidInput", err)
	}
	_, err = svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{
		Key:         "profile",
		Description: strings.Repeat("x", 201),
	}}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ReplaceSlots long description error = %v, want ErrInvalidInput", err)
	}
	_, err = svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{
		Key:  "profile",
		Name: strings.Repeat("名", 81),
	}}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ReplaceSlots long name error = %v, want ErrInvalidInput", err)
	}
}

func TestReplaceSlotsPersistsOptionalName(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()

	slots, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{
		Key:  "profile",
		Name: "用户资料",
	}}})
	if err != nil {
		t.Fatalf("ReplaceSlots create error = %v", err)
	}
	if len(slots) != 1 || slots[0].Name != "用户资料" {
		t.Fatalf("created slots = %#v, want localized name", slots)
	}

	slots, err = svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{
		ID:   slots[0].ID,
		Key:  "profile",
		Name: "User profile",
	}}})
	if err != nil {
		t.Fatalf("ReplaceSlots update error = %v", err)
	}
	if len(slots) != 1 || slots[0].Name != "User profile" {
		t.Fatalf("updated slots = %#v, want edited name", slots)
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
	_, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots error = %v", err)
	}

	_, err = svc.UpdateValue(context.Background(), store.workspaceID, agentID, []RuntimeSlot{{Key: "profile", MaxChars: 5, Enabled: true}}, UserScopeAccount, userID, UpdateValueRequest{
		Key:     "profile",
		Content: strings.Repeat("x", 2001),
	}, MutationMetadata{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateValue error = %v, want ErrInvalidInput", err)
	}
}

func TestClearValueDoesNotPersistContentInAuditEvent(t *testing.T) {
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
	if len(event.BeforeSnapshot) != 0 || len(event.AfterSnapshot) != 0 {
		t.Fatalf("audit event persisted memory content snapshots")
	}
}

func TestClearMissingValueRemovesUndoAndAdvancesCutoff(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID, operationID := uuid.New(), uuid.New(), uuid.New()
	store.undos[operationID] = &AgentMemoryUndoRecord{
		OperationID: operationID, WorkspaceID: store.workspaceID, AgentID: agentID,
		UserScope: UserScopeAccount, UserID: userID, SlotKey: "profile",
	}

	_, err := svc.ClearValue(
		context.Background(), store.workspaceID, agentID,
		[]RuntimeSlot{{Key: "profile", Enabled: true, MaxChars: 500}},
		UserScopeAccount, userID, "profile", MutationMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if store.undos[operationID] != nil {
		t.Fatal("missing-value clear did not remove stale undo record")
	}
	state := store.subjects[subjectKey(store.workspaceID, agentID, UserScopeAccount, userID)]
	if state == nil || state.MemoryEpoch != 1 || state.ExtractionCutoffAt == nil {
		t.Fatalf("subject state = %#v", state)
	}
}

func TestClearValuesNotInKeysClearsRemovedMemoryValues(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID := uuid.New()
	userID := uuid.New()
	_, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}, {Key: "preference"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots error = %v", err)
	}
	store.values[valueKey(store.workspaceID, agentID, "profile", UserScopeAccount, userID)] = &AgentMemoryValue{
		ID: uuid.New(), WorkspaceID: store.workspaceID, AgentID: agentID, SlotKey: "profile", UserScope: UserScopeAccount, UserID: userID, Content: "keep me",
	}
	store.values[valueKey(store.workspaceID, agentID, "preference", UserScopeAccount, userID)] = &AgentMemoryValue{
		ID: uuid.New(), WorkspaceID: store.workspaceID, AgentID: agentID, SlotKey: "preference", UserScope: UserScopeAccount, UserID: userID, Content: "clear me",
	}

	state, err := store.LockSubjectState(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID)
	if err != nil {
		t.Fatal(err)
	}
	jobID := uuid.New()
	store.jobs[jobID] = &AgentMemoryExtractionJob{ID: jobID, WorkspaceID: store.workspaceID, AgentID: agentID, UserScope: UserScopeAccount, UserID: userID, Status: ExtractionJobPending, MemoryEpoch: state.MemoryEpoch}

	if err := svc.ClearValuesNotInKeys(context.Background(), agentID, []string{"profile"}, true); err != nil {
		t.Fatalf("ClearValuesNotInKeys error = %v", err)
	}
	if got := store.values[valueKey(store.workspaceID, agentID, "profile", UserScopeAccount, userID)].Content; got != "keep me" {
		t.Fatalf("profile content = %q, want keep me", got)
	}
	if _, ok := store.values[valueKey(store.workspaceID, agentID, "preference", UserScopeAccount, userID)]; ok {
		t.Fatal("preference value still exists after permanent clear")
	}
	updatedState := store.subjects[subjectKey(store.workspaceID, agentID, UserScopeAccount, userID)]
	if updatedState.MemoryEpoch != state.MemoryEpoch+1 || store.jobs[jobID].Status != ExtractionJobCancelled {
		t.Fatalf("subject/job = %#v/%#v, want invalidated and cancelled", updatedState, store.jobs[jobID])
	}
}

func TestAutomaticUpdateHonorsEnabledSlotsRevisionAndSourceTime(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID := uuid.New(), uuid.New()
	runtimeSlots := []RuntimeSlot{
		{Key: "profile", MaxChars: 2000, Enabled: true},
		{Key: "standing_instructions", MaxChars: 2000, Enabled: true},
	}
	expectedZero := int64(0)
	epoch := int64(0)
	if _, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, runtimeSlots, UserScopeAccount, userID, UpdateValueRequest{
		Key: "standing_instructions", Content: "always lead with the conclusion", ExpectedRevision: &expectedZero,
	}, MutationMetadata{SourceKind: SourceKindAutomatic, MemoryEpoch: &epoch}); err != nil {
		t.Fatalf("automatic standing-instructions update error = %v", err)
	}

	managerValue, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, runtimeSlots, UserScopeAccount, userID, UpdateValueRequest{
		Key: "profile", Content: "manager correction", ExpectedRevision: &expectedZero,
	}, MutationMetadata{SourceKind: SourceKindManager})
	if err != nil {
		t.Fatal(err)
	}
	stored := store.values[valueKey(store.workspaceID, agentID, "profile", UserScopeAccount, userID)]
	stored.UpdatedAt = time.Now()
	olderSource := stored.UpdatedAt.Add(-time.Minute)
	expected := managerValue.Revision
	if _, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, runtimeSlots, UserScopeAccount, userID, UpdateValueRequest{
		Key: "profile", Content: "stale automatic value", ExpectedRevision: &expected,
	}, MutationMetadata{SourceKind: SourceKindAutomatic, SourceCompletedAt: &olderSource, MemoryEpoch: &epoch}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale automatic update error = %v, want ErrConflict", err)
	}
	wrongRevision := expected + 1
	if _, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, runtimeSlots, UserScopeAccount, userID, UpdateValueRequest{
		Key: "profile", Content: "wrong revision", ExpectedRevision: &wrongRevision,
	}, MutationMetadata{SourceKind: SourceKindExplicit}); !errors.Is(err, ErrConflict) {
		t.Fatalf("CAS update error = %v, want ErrConflict", err)
	}
}

func TestAutomaticUpdateCanBeUndoneOnlyAtItsResultingRevision(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID := uuid.New(), uuid.New()
	slots := []RuntimeSlot{{Key: "profile", MaxChars: 2000, Enabled: true}}
	expectedZero := int64(0)
	base, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, UpdateValueRequest{
		Key: "profile", Content: "original", ExpectedRevision: &expectedZero,
	}, MutationMetadata{SourceKind: SourceKindManager})
	if err != nil {
		t.Fatal(err)
	}
	operationID := uuid.New()
	epoch := int64(0)
	expected := base.Revision
	automatic, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, slots, UserScopeAccount, userID, UpdateValueRequest{
		Key: "profile", Content: "automatic", ExpectedRevision: &expected,
	}, MutationMetadata{SourceKind: SourceKindAutomatic, OperationID: &operationID, MemoryEpoch: &epoch})
	if err != nil {
		t.Fatal(err)
	}
	if automatic.LastOperationID != operationID.String() || len(store.undos) != 1 {
		t.Fatalf("automatic value/undo = %#v / %#v", automatic, store.undos)
	}
	undone, err := svc.UndoAutomaticOperation(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID, operationID, slots)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Value == nil || undone.Value.Content != "original" || undone.Value.Revision != automatic.Revision+1 {
		t.Fatalf("undo response = %#v", undone)
	}
	if _, err := svc.UndoAutomaticOperation(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID, operationID, slots); err == nil {
		t.Fatal("second undo unexpectedly succeeded")
	}
}

func TestClearAllAdvancesEpochAndCancelsPendingJobs(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	agentID, userID, conversationID := uuid.New(), uuid.New(), uuid.New()
	job, err := svc.ScheduleExtraction(context.Background(), ScheduleExtractionRequest{
		WorkspaceID: store.workspaceID.String(), AgentID: agentID.String(), UserScope: UserScopeAccount,
		UserID: userID.String(), ConversationID: conversationID.String(), MessageWatermarkID: uuid.New().String(), ExtractorVersion: "test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearAllValues(context.Background(), store.workspaceID, agentID, UserScopeAccount, userID, MutationMetadata{SourceKind: SourceKindManager}); err != nil {
		t.Fatal(err)
	}
	if store.jobs[job.ID].Status != ExtractionJobCancelled {
		t.Fatalf("job status = %q, want cancelled", store.jobs[job.ID].Status)
	}
	state := store.subjects[subjectKey(store.workspaceID, agentID, UserScopeAccount, userID)]
	if state == nil || state.MemoryEpoch != 1 || state.ExtractionCutoffAt == nil {
		t.Fatalf("subject state = %#v, want epoch 1 with extraction cutoff", state)
	}
	if _, err := svc.ClaimExtractionJob(context.Background(), job.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("claim old job error = %v, want ErrConflict", err)
	}
	staleEpoch, expectedZero := int64(0), int64(0)
	if _, err := svc.UpdateValue(context.Background(), store.workspaceID, agentID, []RuntimeSlot{{
		Key: "profile", MaxChars: 2000, Enabled: true,
	}}, UserScopeAccount, userID, UpdateValueRequest{Key: "profile", Content: "must not return", ExpectedRevision: &expectedZero}, MutationMetadata{
		SourceKind: SourceKindAutomatic, MemoryEpoch: &staleEpoch,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale running worker write error = %v, want ErrConflict", err)
	}
	if _, exists := store.values[valueKey(store.workspaceID, agentID, "profile", UserScopeAccount, userID)]; exists {
		t.Fatal("stale running worker recreated memory after delete-all")
	}
}

func TestScheduleExtractionIsIdempotentAndKeepsOriginalForceDeadline(t *testing.T) {
	store := newFakeStore(uuid.New())
	svc := &Service{repo: store}
	request := ScheduleExtractionRequest{
		WorkspaceID: store.workspaceID.String(), AgentID: uuid.New().String(), UserScope: UserScopeAccount,
		UserID: uuid.New().String(), ConversationID: uuid.New().String(), MessageWatermarkID: uuid.New().String(), ExtractorVersion: "test-v1",
	}
	first, err := svc.ScheduleExtraction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := svc.ScheduleExtraction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != first.ID || len(store.jobs) != 1 {
		t.Fatalf("duplicate job = %s, first = %s, count = %d", duplicate.ID, first.ID, len(store.jobs))
	}
	request.MessageWatermarkID = uuid.New().String()
	next, err := svc.ScheduleExtraction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !next.ForceAt.Equal(first.ForceAt) {
		t.Fatalf("next force_at = %v, want original %v", next.ForceAt, first.ForceAt)
	}
	if store.jobs[first.ID].Status != ExtractionJobCancelled {
		t.Fatalf("superseded status = %q, want cancelled", store.jobs[first.ID].Status)
	}
}
