package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

// channelApprovalRuntimeStub adds the pending-approval entry point to the
// channel runtime stub, as the production RuntimeService provides it.
type channelApprovalRuntimeStub struct {
	*channelRuntimeStub
	handled        bool
	err            error
	approvalCalls  int
	lastApprovalID uuid.UUID
	lastApproval   string
}

// HasPendingChannelApproval mirrors the handled flag: a stub that would
// handle the reply also reports a pending approval.
func (runtime *channelApprovalRuntimeStub) HasPendingChannelApproval(context.Context, Scope, uuid.UUID, uuid.UUID) (bool, error) {
	return runtime.handled, nil
}

func (runtime *channelApprovalRuntimeStub) ContinueChannelTaskApproval(_ context.Context, _ Scope, _ uuid.UUID, conversationID uuid.UUID, text string) (bool, error) {
	runtime.approvalCalls++
	runtime.lastApprovalID = conversationID
	runtime.lastApproval = text
	return runtime.handled, runtime.err
}

func newChannelApprovalTestMessage(binding *ChannelRoute, installation *channelruntime.Installation, eventID, text string) *channelruntime.InboundMessage {
	return &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: eventID, ProviderMessageID: "message-" + eventID,
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", SenderDisplayName: "康源",
		Text: text, Mentions: []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}, OccurredAt: time.Now().UTC(),
	}
}

func TestChannelConfirmationResumesPendingApprovalInsteadOfStartingRun(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	runtime.runtimeInputAccepted = true
	approval := &channelApprovalRuntimeStub{channelRuntimeStub: runtime, handled: true}
	service.runtime = approval
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = newChannelApprovalTestMessage(binding, installation, "event-approve", "确认")

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)})
	if err != nil || result == nil || !result.Accepted || result.RunID != nil {
		t.Fatalf("HandleWebhook() = %#v, %v", result, err)
	}
	if approval.approvalCalls != 1 || approval.lastApproval != "确认" || approval.lastApprovalID == uuid.Nil {
		t.Fatalf("approval calls = %d, text = %q, conversation = %s", approval.approvalCalls, approval.lastApproval, approval.lastApprovalID)
	}
	if runtime.runtimeInputCalls != 0 || runtime.calls != 0 {
		t.Fatalf("runtime input calls = %d, invocation calls = %d; a confirmation must resume the pending message", runtime.runtimeInputCalls, runtime.calls)
	}
	event := store.events[installation.ID.String()+":"+adapter.message.ProviderEventID]
	if event == nil || event.Status != channelruntime.InboundEventAccepted || event.ReasonCode != channelTaskApprovalReason {
		t.Fatalf("approval inbound event = %#v", event)
	}
	if replay, replayErr := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); replayErr != nil || !replay.Duplicate {
		t.Fatalf("replayed approval = %#v, %v", replay, replayErr)
	}
	if approval.approvalCalls != 1 {
		t.Fatalf("approval calls after replay = %d, want 1", approval.approvalCalls)
	}
}

func TestChannelMessageWithoutPendingApprovalKeepsNormalAdmission(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	runtime.runtimeInputAccepted = true
	approval := &channelApprovalRuntimeStub{channelRuntimeStub: runtime}
	service.runtime = approval
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = newChannelApprovalTestMessage(binding, installation, "event-normal", "再补充一个要求")

	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if approval.approvalCalls != 1 || runtime.runtimeInputCalls != 1 {
		t.Fatalf("approval calls = %d, runtime input calls = %d", approval.approvalCalls, runtime.runtimeInputCalls)
	}
}

func TestChannelApprovalFailureMarksInboundEventFailed(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	approval := &channelApprovalRuntimeStub{channelRuntimeStub: runtime, err: errors.New("approval unavailable")}
	service.runtime = approval
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = newChannelApprovalTestMessage(binding, installation, "event-fail", "确认")

	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); err == nil {
		t.Fatal("HandleWebhook() succeeded after an approval failure")
	}
	event := store.events[installation.ID.String()+":"+adapter.message.ProviderEventID]
	if event == nil || event.Status != channelruntime.InboundEventFailed || runtime.runtimeInputCalls != 0 || runtime.calls != 0 {
		t.Fatalf("event = %#v, runtime input calls = %d, invocation calls = %d", event, runtime.runtimeInputCalls, runtime.calls)
	}
}

// channelApprovalJobStoreStub is an in-memory ChannelApprovalJobStore with the
// same claim, retry and finish semantics as the repository.
type channelApprovalJobStoreStub struct {
	jobs map[uuid.UUID]*ChannelApprovalJob
}

func newChannelApprovalJobStoreStub() *channelApprovalJobStoreStub {
	return &channelApprovalJobStoreStub{jobs: make(map[uuid.UUID]*ChannelApprovalJob)}
}

func (store *channelApprovalJobStoreStub) CreateChannelApprovalJob(_ context.Context, job *ChannelApprovalJob) (bool, error) {
	if _, exists := store.jobs[job.ID]; exists {
		return false, nil
	}
	copy := *job
	store.jobs[job.ID] = &copy
	return true, nil
}

func (store *channelApprovalJobStoreStub) ClaimChannelApprovalJobs(_ context.Context, limit int, now time.Time) ([]*ChannelApprovalJob, error) {
	claimed := []*ChannelApprovalJob{}
	for _, job := range store.jobs {
		if len(claimed) >= limit || job.AvailableAt.After(now) ||
			(job.Status != ChannelApprovalJobPending && job.Status != ChannelApprovalJobRunning) {
			continue
		}
		job.Status, job.AttemptCount, job.ClaimedAt, job.AvailableAt = ChannelApprovalJobRunning, job.AttemptCount+1, &now, now.Add(channelApprovalLease)
		copy := *job
		claimed = append(claimed, &copy)
	}
	return claimed, nil
}

func (store *channelApprovalJobStoreStub) ListDueChannelApprovalJobs(_ context.Context, _ int, now time.Time) ([]*ChannelApprovalJob, error) {
	due := []*ChannelApprovalJob{}
	for _, job := range store.jobs {
		if !job.AvailableAt.After(now) && (job.Status == ChannelApprovalJobPending || job.Status == ChannelApprovalJobRunning) {
			copy := *job
			due = append(due, &copy)
		}
	}
	return due, nil
}

func (store *channelApprovalJobStoreStub) FinishChannelApprovalJob(_ context.Context, id uuid.UUID, status ChannelApprovalJobStatus, code string, now time.Time) error {
	job := store.jobs[id]
	if job == nil || job.Status != ChannelApprovalJobRunning {
		return ErrChannelDeliveryConflict
	}
	job.Status, job.ErrorCode, job.CompletedAt, job.ClaimedAt = status, code, &now, nil
	return nil
}

func (store *channelApprovalJobStoreStub) RetryChannelApprovalJob(_ context.Context, id uuid.UUID, availableAt time.Time, code string, _ time.Time) error {
	job := store.jobs[id]
	if job == nil || job.Status != ChannelApprovalJobRunning {
		return ErrChannelDeliveryConflict
	}
	job.Status, job.AvailableAt, job.ErrorCode, job.ClaimedAt = ChannelApprovalJobPending, availableAt, code, nil
	return nil
}

// channelApprovalWakeStub records approval wake-ups next to the transient
// dispatcher stub so tests never start the in-process fallback worker.
type channelApprovalWakeStub struct {
	*channelTransientDispatcherStub
	wakes []string
}

func (dispatcher *channelApprovalWakeStub) EnqueueChannelApproval(_ context.Context, id uuid.UUID, attempt int) error {
	dispatcher.wakes = append(dispatcher.wakes, id.String()+":"+string(rune('0'+attempt)))
	return nil
}

func newDurableChannelApprovalService(t *testing.T, handled bool) (*ChannelService, *channelStoreStub, *channelRuntimeStub, *channelApprovalRuntimeStub, *channelApprovalJobStoreStub, *channelApprovalWakeStub, *channelruntime.Installation, *ChannelRoute, *channelAdapterStub) {
	t.Helper()
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	approval := &channelApprovalRuntimeStub{channelRuntimeStub: runtime, handled: handled}
	service.runtime = approval
	jobs := newChannelApprovalJobStoreStub()
	service.approvals = jobs
	wake := &channelApprovalWakeStub{channelTransientDispatcherStub: service.transient.(*channelTransientDispatcherStub)}
	service.transient = wake
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	return service, store, runtime, approval, jobs, wake, installation, binding, adapter
}

func TestChannelApprovalIsQueuedWithinAcknowledgementAndExecutedByWorker(t *testing.T) {
	service, store, runtime, approval, jobs, wake, installation, binding, adapter := newDurableChannelApprovalService(t, true)
	adapter.message = newChannelApprovalTestMessage(binding, installation, "event-queued", "确认")

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)})
	if err != nil || result == nil || !result.Accepted || result.RunID != nil {
		t.Fatalf("HandleWebhook() = %#v, %v", result, err)
	}
	if approval.approvalCalls != 0 {
		t.Fatalf("approval ran %d times inside the acknowledgement window, want 0", approval.approvalCalls)
	}
	if runtime.runtimeInputCalls != 0 || runtime.calls != 0 {
		t.Fatalf("runtime input calls = %d, invocation calls = %d", runtime.runtimeInputCalls, runtime.calls)
	}
	event := store.events[installation.ID.String()+":"+adapter.message.ProviderEventID]
	if event == nil || event.Status != channelruntime.InboundEventAccepted || event.ReasonCode != channelTaskApprovalReason {
		t.Fatalf("inbound event = %#v", event)
	}
	job := jobs.jobs[event.ID]
	if job == nil || job.Status != ChannelApprovalJobPending || job.Text != "确认" || len(wake.wakes) != 1 {
		t.Fatalf("queued job = %#v, wakes = %v", job, wake.wakes)
	}
	if replay, replayErr := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); replayErr != nil || !replay.Duplicate {
		t.Fatalf("replayed approval = %#v, %v", replay, replayErr)
	}
	if len(jobs.jobs) != 1 || len(wake.wakes) != 1 {
		t.Fatalf("jobs = %d, wakes = %v after provider retry", len(jobs.jobs), wake.wakes)
	}

	if err := service.DispatchChannelApprovals(t.Context(), 1); err != nil {
		t.Fatalf("DispatchChannelApprovals() error = %v", err)
	}
	if approval.approvalCalls != 1 || approval.lastApproval != "确认" || job.Status != ChannelApprovalJobSucceeded {
		t.Fatalf("approval calls = %d, text = %q, job = %#v", approval.approvalCalls, approval.lastApproval, job)
	}
}

func TestChannelApprovalWorkerRetriesThenFails(t *testing.T) {
	service, _, _, approval, jobs, _, installation, binding, adapter := newDurableChannelApprovalService(t, true)
	adapter.message = newChannelApprovalTestMessage(binding, installation, "event-retry", "确认")
	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); err != nil {
		t.Fatal(err)
	}
	approval.err = errors.New("workflow unavailable")
	now := time.Now().UTC()
	for attempt := 1; attempt <= channelApprovalMaxAttempts; attempt++ {
		service.now = func() time.Time { return now }
		if err := service.DispatchChannelApprovals(t.Context(), 1); err != nil {
			t.Fatalf("DispatchChannelApprovals(attempt %d) error = %v", attempt, err)
		}
		now = now.Add(time.Hour)
	}
	var job *ChannelApprovalJob
	for _, candidate := range jobs.jobs {
		job = candidate
	}
	if approval.approvalCalls != channelApprovalMaxAttempts || job.Status != ChannelApprovalJobFailed || job.ErrorCode != "continuation_failed" {
		t.Fatalf("approval calls = %d, job = %#v", approval.approvalCalls, job)
	}
}

func TestChannelApprovalSweepRewakesDueJobs(t *testing.T) {
	service, _, _, _, jobs, wake, installation, binding, adapter := newDurableChannelApprovalService(t, true)
	adapter.message = newChannelApprovalTestMessage(binding, installation, "event-sweep", "确认")
	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); err != nil {
		t.Fatal(err)
	}
	if len(wake.wakes) != 1 || len(jobs.jobs) != 1 {
		t.Fatalf("wakes = %v", wake.wakes)
	}
	if err := service.wakeDueChannelApprovals(t.Context(), time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(wake.wakes) != 2 {
		t.Fatalf("wakes after sweep = %v, want the lost wake-up repeated", wake.wakes)
	}
}
