package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func TestChannelReceiverInboundSharesWebhookDedupeAndRevisionFence(t *testing.T) {
	service, store, runtime, adapter, scope, subjectID := newChannelTestService(t)
	installation, route := createChannelTestRoute(t, service, scope, subjectID, channelruntime.ConversationTypeGroup)
	message := &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-ws", ProviderMessageID: "message-ws",
		ExternalTenantID: installation.ExternalTenantID, ExternalConversationID: route.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", Text: "hello", WakeReason: channelruntime.WakeReasonMention,
		Mentions: []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}, OccurredAt: time.Now().UTC(),
	}
	for attempt := range 10 {
		result, err := service.HandleInboundMessage(t.Context(), installation.ID, installation.Revision, message, time.Now())
		if err != nil || !result.Accepted || result.Duplicate != (attempt > 0) {
			t.Fatalf("attempt %d: result=%#v err=%v", attempt, result, err)
		}
	}
	// A transport switch must not create a new invocation for the same event.
	adapter.message = message
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{}`), ReceivedAt: time.Now()})
	if err != nil || !result.Duplicate || runtime.calls != 1 || len(store.events) != 1 {
		t.Fatalf("cross-transport dedupe: result=%#v err=%v calls=%d", result, err, runtime.calls)
	}
	message.ProviderEventID, message.ProviderMessageID = "new-event", "new-message"
	store.installations[installation.ID].Revision++
	if _, err := service.HandleInboundMessage(t.Context(), installation.ID, installation.Revision, message, time.Now()); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("rotated receiver accepted stale event: %v", err)
	}
	store.installations[installation.ID].Status = channelruntime.InstallationStatusDisabled
	if _, err := service.HandleInboundMessage(t.Context(), installation.ID, installation.Revision+1, message, time.Now()); !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("disabled receiver accepted event: %v", err)
	}
	if runtime.calls != 1 || len(store.events) != 1 {
		t.Fatal("fenced events reached the runtime")
	}
}

type receiverViewCredentialStub struct {
	channelCredentialStub
	mode string
	err  error
}

func (stub receiverViewCredentialStub) ResolveCredential(context.Context, *channelruntime.Installation) (*channelruntime.ResolvedChannelCredential, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return &channelruntime.ResolvedChannelCredential{Values: map[string]string{"mode": stub.mode}}, nil
}

type receiverViewStatusStub struct{ status channelruntime.ReceiverStatus }

func (stub receiverViewStatusStub) Status(uuid.UUID) channelruntime.ReceiverStatus {
	return stub.status
}
func (receiverViewStatusStub) Retry(uuid.UUID) {}

func TestChannelReceiverViewUsesSavedModeAndCurrentRevision(t *testing.T) {
	service, store, _, adapter, scope, subjectID := newChannelTestService(t)
	installation, _ := createChannelTestRoute(t, service, scope, subjectID, channelruntime.ConversationTypeDirect)
	registry, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: adapter, Provisioner: channelProvisionerStub{}, Receiver: lifecycleReceiver{}})
	if err != nil {
		t.Fatal(err)
	}
	service.adapters = registry
	service.apiBaseURL = "https://example.test"
	service.credentials = receiverViewCredentialStub{mode: "websocket"}
	view := func() *ChannelInstallationView {
		t.Helper()
		items, err := service.ListInstallations(t.Context(), scope, subjectID)
		if err != nil || len(items) != 1 {
			t.Fatalf("views=%#v err=%v", items, err)
		}
		return items[0]
	}
	if actual := view(); actual.Receiver.Mode != "websocket" || actual.Receiver.State != channelruntime.ReceiverStatePending || actual.WebhookURL != "" {
		t.Fatalf("fresh replica mode=%#v webhook=%q", actual.Receiver, actual.WebhookURL)
	}
	service.receivers = receiverViewStatusStub{channelruntime.ReceiverStatus{Mode: "websocket", Revision: installation.Revision - 1, State: channelruntime.ReceiverStateConnected}}
	if view().Receiver.State != channelruntime.ReceiverStatePending {
		t.Fatal("stale connected status leaked")
	}
	service.receivers = receiverViewStatusStub{channelruntime.ReceiverStatus{Mode: "websocket", Revision: installation.Revision, State: channelruntime.ReceiverStateReconnecting}}
	if view().Receiver.State != channelruntime.ReceiverStateReconnecting {
		t.Fatal("live transport status missing")
	}
	store.installations[installation.ID].Status = channelruntime.InstallationStatusDisabled
	if actual := view(); actual.Receiver.State != channelruntime.ReceiverStateDisconnected || actual.WebhookURL != "" {
		t.Fatal("disabled WebSocket fell back to Webhook")
	}
	service.credentials = receiverViewCredentialStub{mode: "webhook"}
	if actual := view(); actual.Receiver.State != channelruntime.ReceiverStateNotApplicable || actual.WebhookURL == "" {
		t.Fatal("legacy Webhook lost its setup URL")
	}
	service.credentials = receiverViewCredentialStub{err: errors.New("credential decryption failed")}
	if actual := view(); actual.Receiver.Mode != "unknown" || actual.Receiver.ErrorCode != "credential_unavailable" || actual.WebhookURL != "" {
		t.Fatal("credential failure was misrepresented as a Webhook configuration problem")
	}
}
