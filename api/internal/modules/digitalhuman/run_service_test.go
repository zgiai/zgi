package digitalhuman

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/datatypes"
)

func TestRuntimeServiceSubmitIsDurableAndIdempotent(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	request := ManualInvocationRequest{RequestID: uuid.New(), Query: "research the issue"}

	first, created, err := service.SubmitManual(t.Context(), scope, digitalHumanID, request)
	if err != nil || !created {
		t.Fatalf("SubmitManual(first) = created %v, err %v", created, err)
	}
	second, created, err := service.SubmitManual(t.Context(), scope, digitalHumanID, request)
	if err != nil || created {
		t.Fatalf("SubmitManual(duplicate) = created %v, err %v", created, err)
	}
	if second.ID != first.ID || len(store.runs) != 1 || len(store.outbox) != 1 {
		t.Fatalf("duplicate created extra durable state: runs=%d outbox=%d", len(store.runs), len(store.outbox))
	}
	if len(dispatcher.enqueued) != 1 || dispatcher.enqueued[0] != first.ID {
		t.Fatalf("queue delivery = %v, want [%s]", dispatcher.enqueued, first.ID)
	}
	if got := store.outbox[0].Status; got != RunOutboxConfirmed {
		t.Fatalf("outbox status = %s, want confirmed", got)
	}
}

func TestRuntimeServiceReconcilesFailedQueueDelivery(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{fail: true}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	service.now = advancingClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))

	run, created, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "retry me"})
	if err != nil || !created {
		t.Fatalf("SubmitManual() = created %v, err %v", created, err)
	}
	if store.outbox[0].Status != RunOutboxPending {
		t.Fatalf("outbox status after failure = %s", store.outbox[0].Status)
	}
	dispatcher.fail = false
	store.outbox[0].AvailableAt = service.now().Add(-time.Second)
	if err := service.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if store.outbox[0].Status != RunOutboxConfirmed || dispatcher.enqueued[len(dispatcher.enqueued)-1] != run.ID {
		t.Fatalf("reconciled outbox = %#v, enqueued=%v", store.outbox[0], dispatcher.enqueued)
	}
}

func TestRuntimeServiceExecuteUsesDigitalHumanCallerAndNoLegacyMemory(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	run, _, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{RequestID: uuid.New(), Query: "invoke bound skill"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.caller.Type != runtimemodel.ConversationCallerDigitalHuman || chat.caller.ID == nil || *chat.caller.ID != digitalHumanID {
		t.Fatalf("caller = %#v", chat.caller)
	}
	if chat.request.UseMemory || chat.config.UseMemory || chat.config.AgentMemoryEnabled || chat.config.AgentMemoryAutoExtractionEnabled {
		t.Fatal("DigitalHuman execution enabled a legacy memory path")
	}
	if chat.request.SourceMessageID == nil || *chat.request.SourceMessageID != run.ID {
		t.Fatalf("source message id = %v, want run id %s", chat.request.SourceMessageID, run.ID)
	}
	stored := store.runs[run.ID]
	if stored.Status != RunStatusSucceeded || stored.ResultSummary != "completed answer" || stored.MessageID == nil {
		t.Fatalf("completed run = %#v", stored)
	}
	if !bytes.Contains(stored.ExecutionTrace, []byte(`"skill_id":"calculator"`)) ||
		bytes.Contains(stored.ExecutionTrace, []byte("must-not-persist")) ||
		bytes.Contains(stored.ExecutionTrace, []byte("arguments")) {
		t.Fatalf("execution trace was not safely projected: %s", stored.ExecutionTrace)
	}
}

func TestRuntimeServiceAddsSubjectMemoryWithoutEnablingLegacyMemory(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	now := time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)
	memory := &fakeMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		SystemPromptAddendum: "Authorized active long-term memory:\n- launch is September 8",
		Tools:                []skillloop.RuntimeTool{{Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: digitalHumanMemoryToolName}}}},
	}}
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, dispatcher, WithMemoryRuntime(memory))
	service.now = func() time.Time { return now }
	run, _, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "use our launch plan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	if memory.calls != 1 || memory.finalizeCalls != 1 || !strings.Contains(chat.config.SystemPrompt, "launch is September 8") || len(chat.config.RuntimeTools) != 1 {
		t.Fatalf("memory preparation calls=%d finalization calls=%d config=%#v", memory.calls, memory.finalizeCalls, chat.config)
	}
	if chat.config.UseMemory || chat.config.AgentMemoryEnabled || chat.config.AgentMemoryAutoExtractionEnabled {
		t.Fatal("subject memory enabled a legacy memory path")
	}
}

func TestRuntimeServiceCancelUsesDatabaseStateAsAuthority(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	run, _, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{RequestID: uuid.New(), Query: "cancel me"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	canceled, err := service.CancelRun(t.Context(), scope, digitalHumanID, run.ID)
	if err != nil || canceled.Status != RunStatusCanceled {
		t.Fatalf("CancelRun(queued) = status %s, err %v", canceled.Status, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil || chat.prepareCalls != 0 {
		t.Fatalf("canceled run executed: err=%v prepares=%d", err, chat.prepareCalls)
	}
	if len(dispatcher.canceled) != 1 || dispatcher.canceled[0] != run.ID {
		t.Fatalf("queue cancellation = %v", dispatcher.canceled)
	}
}

func TestRuntimeServiceRejectsTamperedSnapshotBeforeChatRuntime(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	run, _, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "do not run tampered state"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	store.runs[run.ID].ExecutionSnapshotRevision = "sha256:tampered"
	if err := service.Execute(t.Context(), run.ID); !errors.Is(err, ErrRunConflict) {
		t.Fatalf("Execute() error = %v, want run conflict", err)
	}
	if chat.prepareCalls != 0 || store.runs[run.ID].Status != RunStatusFailed {
		t.Fatalf("tampered run reached runtime or was not failed: prepares=%d status=%s", chat.prepareCalls, store.runs[run.ID].Status)
	}
}

func TestRuntimeServiceListsOnlySafeDigitalHumanSkills(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	chat := &fakeDigitalHumanChat{skills: []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillTime, Name: "Current time", Description: "Read the current time", Enabled: true, Status: skills.SkillStatusActive},
		{ID: skills.SkillAgentMemory, Name: "Agent memory", Enabled: true, Status: skills.SkillStatusActive},
		{ID: "broken", Name: "Broken", Enabled: true, Status: skills.SkillStatusInvalid},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	result, err := service.ListResourceCandidates(t.Context(), testRunScope(), uuid.New(), ResourceCandidateQuery{
		Type: resourcebindings.BindingTypeSkill, Search: "time",
	})
	if err != nil {
		t.Fatalf("ListResourceCandidates() error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ResourceID != skills.SkillTime ||
		result.Items[0].ResourceRevision == "" || !result.Items[0].Available {
		t.Fatalf("skill candidates = %#v, want one safe time skill", result)
	}
}

type fakeRunStore struct {
	mu         sync.Mutex
	runs       map[uuid.UUID]*Run
	outbox     []*RunOutbox
	candidates []ResourceCandidate
}

func newFakeRunStore() *fakeRunStore { return &fakeRunStore{runs: make(map[uuid.UUID]*Run)} }

func (s *fakeRunStore) CreateRunWithOutbox(_ context.Context, run *Run, outbox *RunOutbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.OrganizationID == run.OrganizationID && existing.DigitalHumanID == run.DigitalHumanID && existing.IdempotencyKey == run.IdempotencyKey {
			return ErrRunConflict
		}
	}
	s.runs[run.ID], s.outbox = run, append(s.outbox, outbox)
	return nil
}

func (s *fakeRunStore) RunByID(_ context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[runID]
	if run == nil || run.OrganizationID != scope.OrganizationID || run.WorkspaceID != scope.WorkspaceID || run.DigitalHumanID != digitalHumanID {
		return nil, ErrRunNotFound
	}
	return run, nil
}

func (s *fakeRunStore) RunByIdempotency(_ context.Context, organizationID, digitalHumanID uuid.UUID, key string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.OrganizationID == organizationID && run.DigitalHumanID == digitalHumanID && run.IdempotencyKey == key {
			return run, nil
		}
	}
	return nil, ErrRunNotFound
}

func (s *fakeRunStore) ListRuns(_ context.Context, scope Scope, digitalHumanID uuid.UUID, _ ListQuery) ([]*Run, int64, error) {
	items := make([]*Run, 0)
	for _, run := range s.runs {
		if run.OrganizationID == scope.OrganizationID && run.WorkspaceID == scope.WorkspaceID && run.DigitalHumanID == digitalHumanID {
			items = append(items, run)
		}
	}
	return items, int64(len(items)), nil
}

func (s *fakeRunStore) ListResourceCandidates(context.Context, Scope, uuid.UUID, ResourceCandidateQuery) ([]ResourceCandidate, error) {
	return append([]ResourceCandidate(nil), s.candidates...), nil
}

func (s *fakeRunStore) ClaimRun(_ context.Context, runID uuid.UUID, now time.Time) (*Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[runID]
	if run == nil {
		return nil, false, ErrRunNotFound
	}
	if run.Status != RunStatusQueued {
		return run, false, nil
	}
	run.Status, run.StartedAt, run.UpdatedAt = RunStatusRunning, &now, now
	return run, true, nil
}

func (s *fakeRunStore) LinkRunMessage(_ context.Context, runID, conversationID, messageID uuid.UUID, now time.Time) error {
	run := s.runs[runID]
	if run == nil || (run.Status != RunStatusRunning && run.Status != RunStatusCanceling) {
		return ErrRunConflict
	}
	run.ConversationID, run.MessageID, run.UpdatedAt = &conversationID, &messageID, now
	return nil
}

func (s *fakeRunStore) CompleteRun(_ context.Context, runID uuid.UUID, summary string, trace datatypes.JSON, now time.Time) error {
	run := s.runs[runID]
	if run == nil {
		return ErrRunNotFound
	}
	if run.Status == RunStatusCanceling || run.CancelRequestedAt != nil {
		run.Status = RunStatusCanceled
	} else if run.Status == RunStatusRunning {
		run.Status = RunStatusSucceeded
	} else {
		return ErrRunConflict
	}
	run.ResultSummary, run.ExecutionTrace, run.CompletedAt, run.UpdatedAt = summary, trace, &now, now
	return nil
}

func (s *fakeRunStore) FailRun(_ context.Context, runID uuid.UUID, code, message string, now time.Time) error {
	run := s.runs[runID]
	if run == nil {
		return ErrRunNotFound
	}
	if run.CancelRequestedAt != nil {
		run.Status = RunStatusCanceled
	} else {
		run.Status = RunStatusFailed
	}
	run.ErrorCode, run.ErrorMessage, run.CompletedAt, run.UpdatedAt = code, message, &now, now
	return nil
}

func (s *fakeRunStore) RequestRunCancel(_ context.Context, scope Scope, digitalHumanID, runID uuid.UUID, now time.Time) (*Run, error) {
	run, err := s.RunByID(context.Background(), scope, digitalHumanID, runID)
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case RunStatusQueued:
		run.Status, run.CompletedAt = RunStatusCanceled, &now
	case RunStatusRunning:
		run.Status = RunStatusCanceling
	case RunStatusCanceling, RunStatusCanceled:
	default:
		return nil, ErrRunConflict
	}
	run.CancelRequestedAt, run.UpdatedAt = &now, now
	return run, nil
}

func (s *fakeRunStore) ClaimRunOutboxBatch(_ context.Context, limit int, now time.Time) ([]*RunOutbox, error) {
	items := make([]*RunOutbox, 0, limit)
	for _, item := range s.outbox {
		if len(items) == limit {
			break
		}
		if item.Status == RunOutboxPending && !item.AvailableAt.After(now) {
			item.Status, item.ClaimedAt = RunOutboxProcessing, &now
			item.AttemptCount++
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *fakeRunStore) ConfirmRunOutbox(_ context.Context, id uuid.UUID, now time.Time) error {
	for _, item := range s.outbox {
		if item.ID == id {
			item.Status, item.ConfirmedAt = RunOutboxConfirmed, &now
			return nil
		}
	}
	return errors.New("outbox not found")
}

func (s *fakeRunStore) RetryRunOutbox(_ context.Context, id uuid.UUID, availableAt time.Time, message string) error {
	for _, item := range s.outbox {
		if item.ID == id {
			item.Status, item.AvailableAt, item.ErrorMessage, item.ClaimedAt = RunOutboxPending, availableAt, message, nil
			return nil
		}
	}
	return errors.New("outbox not found")
}

func (s *fakeRunStore) RequeueStaleRunOutbox(_ context.Context, claimedBefore, now time.Time) error {
	for _, item := range s.outbox {
		if item.Status == RunOutboxProcessing && item.ClaimedAt != nil && item.ClaimedAt.Before(claimedBefore) {
			item.Status, item.AvailableAt, item.ClaimedAt = RunOutboxPending, now, nil
		}
	}
	return nil
}

func (s *fakeRunStore) MarkStaleRunsUnknown(_ context.Context, updatedBefore, now time.Time) error {
	for _, run := range s.runs {
		if (run.Status == RunStatusRunning || run.Status == RunStatusCanceling) && run.UpdatedAt.Before(updatedBefore) {
			run.Status, run.CompletedAt = RunStatusUnknown, &now
		}
	}
	return nil
}

type fakeRunDispatcher struct {
	fail     bool
	enqueued []uuid.UUID
	canceled []uuid.UUID
}

func (d *fakeRunDispatcher) EnqueueRun(_ context.Context, id uuid.UUID) error {
	d.enqueued = append(d.enqueued, id)
	if d.fail {
		return errors.New("queue unavailable")
	}
	return nil
}

func (d *fakeRunDispatcher) CancelRun(_ context.Context, id uuid.UUID) error {
	d.canceled = append(d.canceled, id)
	return nil
}

type fakeDigitalHumanChat struct {
	caller       chatruntime.Caller
	config       chatruntime.RunConfig
	request      runtimedto.ChatRequest
	prepareCalls int
	skills       []skills.SkillDiscoveryMetadata
}

func (c *fakeDigitalHumanChat) PrepareConfiguredChat(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, config chatruntime.RunConfig, request runtimedto.ChatRequest) (*chatruntime.PreparedChat, error) {
	c.prepareCalls++
	c.caller, c.config, c.request = caller, config, request
	return &chatruntime.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
	}, nil
}

func (c *fakeDigitalHumanChat) RunPreparedStream(context.Context, *chatruntime.PreparedChat, func(string) error, ...func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error) {
	return &chatruntime.ChatResult{
		Answer: "completed answer", Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"runtime_id": "skill:1", "skill_id": "calculator", "tool_name": "calculate", "status": "succeeded",
				"arguments": map[string]interface{}{"api_key": "must-not-persist"}, "result": "must-not-persist",
			}},
		},
	}, nil
}

func (c *fakeDigitalHumanChat) StopMessage(context.Context, chatruntime.Scope, uuid.UUID) (*runtimemodel.Message, error) {
	return &runtimemodel.Message{}, nil
}

func (c *fakeDigitalHumanChat) ListSkills(context.Context, chatruntime.Scope) ([]skills.SkillDiscoveryMetadata, error) {
	return append([]skills.SkillDiscoveryMetadata(nil), c.skills...), nil
}

type fakeExecutionResolver struct{ now time.Time }

func (r fakeExecutionResolver) Resolve(_ context.Context, event invocation.Event, _ *uuid.UUID) (*ResolvedExecution, error) {
	snapshot := ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Subject: event.Target, DigitalHumanVersion: uuid.New(),
		Name: "Atlas", Persona: "Researcher", Rules: []string{"cite evidence"}, Model: "test-model",
		UseUserMemory: false, UseAgentMemory: false, ResolvedAt: r.now,
	}
	var err error
	snapshot.Revision, err = executionSnapshotRevision(snapshot)
	if err != nil {
		return nil, err
	}
	spec, err := runspec.Seal(runspec.Spec{
		InvocationID: event.ID, Subject: event.Target,
		Identity: runspec.VersionRef{Kind: "identity", ID: snapshot.DigitalHumanVersion.String(), Revision: snapshot.Revision},
		Model:    runspec.VersionRef{Kind: "model", ID: snapshot.Model, Revision: snapshot.Revision},
		Policies: []runspec.VersionRef{{Kind: "execution_snapshot", ID: snapshot.DigitalHumanVersion.String(), Revision: snapshot.Revision}},
		Budget:   runspec.Budget{MaxDurationSeconds: 300}, ResolvedAt: r.now,
	})
	return &ResolvedExecution{Spec: spec, Snapshot: snapshot}, err
}

type fakeBrainAdapter struct{}

func (fakeBrainAdapter) RunConfig(resolved *ResolvedExecution) (chatruntime.RunConfig, error) {
	return chatruntime.RunConfig{
		SystemPrompt: resolved.Snapshot.Persona, Model: resolved.Snapshot.Model,
		BillingAppID: resolved.Snapshot.Subject.ID.String(), BillingAppType: runtimemodel.ConversationCallerDigitalHuman,
		UseMemory: false, AgentMemoryEnabled: false, AgentMemoryAutoExtractionEnabled: false,
	}, nil
}

type fakeMemoryRuntimeProvider struct {
	preparation   *MemoryRuntimePreparation
	calls         int
	finalizeCalls int
}

func (p *fakeMemoryRuntimeProvider) Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (*MemoryRuntimePreparation, error) {
	p.calls++
	return p.preparation, nil
}

func (p *fakeMemoryRuntimeProvider) Finalize(context.Context, *Run, *chatruntime.PreparedChat, *chatruntime.ChatResult) error {
	p.finalizeCalls++
	return nil
}

func newTestRuntimeService(t *testing.T, store *fakeRunStore, dispatcher *fakeRunDispatcher, chat *fakeDigitalHumanChat) *RuntimeService {
	t.Helper()
	now := time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, dispatcher)
	service.now = func() time.Time { return now }
	return service
}

func testRunScope() Scope {
	return Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
}

func advancingClock(start time.Time) func() time.Time {
	current := start
	return func() time.Time {
		current = current.Add(time.Second)
		return current
	}
}
