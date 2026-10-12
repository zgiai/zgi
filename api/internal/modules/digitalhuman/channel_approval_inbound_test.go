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
