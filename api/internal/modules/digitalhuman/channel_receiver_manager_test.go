package digitalhuman

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func TestChannelReceiverManagerStartsStreamAndRoutesNormalizedMessage(t *testing.T) {
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
		Provider: "stream-test", ConnectionID: uuid.New(), Status: channelruntime.InstallationStatusActive,
		ExternalTenantID: "tenant", ExternalBotID: "bot", Revision: 1,
	}
	store := &receiverStoreStub{installation: installation}
	credentials := receiverCredentialStub{}
	sink := &receiverSinkStub{received: make(chan *channelruntime.InboundMessage, 1)}
	registry, err := channelruntime.NewRegistry(channelruntime.Provider{
		Adapter: &receiverAdapterStub{}, Provisioner: &receiverAdapterStub{}, Receiver: receiverStub{},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewChannelReceiverManager(store, registry, credentials, sink)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Stop(stopCtx)
	})

	select {
	case message := <-sink.received:
		if message.Text != "hello from stream" || message.ExternalConversationID != "direct:member" {
			t.Fatalf("received message = %#v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream message was not routed")
	}
	if status := manager.Status(installation.ID); status.Mode != "stream" || status.State != channelruntime.ReceiverStateConnected {
		t.Fatalf("receiver status = %#v", status)
	}
}

type receiverStoreStub struct {
	installation *channelruntime.Installation
	mu           sync.Mutex
	leased       bool
	check        func(context.Context) error
}

func (store *receiverStoreStub) ListActiveChannelInstallations(context.Context) ([]*channelruntime.Installation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.installation == nil || store.installation.Status != channelruntime.InstallationStatusActive {
		return nil, nil
	}
	return []*channelruntime.Installation{cloneInstallation(store.installation)}, nil
}

func (store *receiverStoreStub) TryAcquireChannelReceiverLease(context.Context, uuid.UUID) (*ChannelReceiverLease, bool, error) {
	store.mu.Lock()
	if store.leased {
		store.mu.Unlock()
		return nil, false, nil
	}
	store.leased = true
	store.mu.Unlock()
	return &ChannelReceiverLease{Check: store.check, Release: func() {
		store.mu.Lock()
		store.leased = false
		store.mu.Unlock()
	}}, true, nil
}

type receiverCredentialStub struct{ mode string }

func (stub receiverCredentialStub) ResolveCredential(context.Context, *channelruntime.Installation) (*channelruntime.ResolvedChannelCredential, error) {
	mode := stub.mode
	if mode == "" {
		mode = "stream"
	}
	return &channelruntime.ResolvedChannelCredential{Values: map[string]string{"mode": mode}}, nil
}

func (receiverCredentialStub) ResolveIdentity(context.Context, *channelruntime.Installation) (string, string, error) {
	return "tenant", "bot", nil
}

func (receiverCredentialStub) ContextReadiness(context.Context, *channelruntime.Installation) ChannelContextReadiness {
	return ChannelContextReadiness{Status: ChannelContextReadinessUnavailable}
}

type receiverStub struct{}

func (receiverStub) Mode(channelruntime.ResolvedChannelCredential) string { return "stream" }

func (receiverStub) Run(ctx context.Context, request channelruntime.ReceiverRequest, handle channelruntime.InboundMessageHandler) error {
	if request.OnConnected != nil {
		request.OnConnected()
	}
	if err := handle(ctx, &channelruntime.InboundMessage{
		Provider: "stream-test", ProviderEventID: "event-1", ProviderMessageID: "message-1",
		ExternalTenantID: "tenant", ExternalConversationID: "direct:member",
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "member",
		Text: "hello from stream", WakeReason: channelruntime.WakeReasonDirectMessage, OccurredAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

type receiverSinkStub struct {
	received chan *channelruntime.InboundMessage
}

func (sink *receiverSinkStub) HandleInboundMessage(
	_ context.Context,
	_ uuid.UUID,
	_ int64,
	message *channelruntime.InboundMessage,
	_ time.Time,
) (*ChannelWebhookResult, error) {
	sink.received <- message
	return &ChannelWebhookResult{}, nil
}

type receiverAdapterStub struct{}

func (*receiverAdapterStub) Provider() string { return "stream-test" }
func (*receiverAdapterStub) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{}
}
func (*receiverAdapterStub) Verify(context.Context, channelruntime.WebhookRequest, channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*receiverAdapterStub) Normalize(context.Context, *channelruntime.VerifiedWebhook, channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*receiverAdapterStub) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{StatusCode: http.StatusOK}
}
func (*receiverAdapterStub) Deliver(context.Context, channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*receiverAdapterStub) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*receiverAdapterStub) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}
func (*receiverAdapterStub) PrepareCredentials(_, updates map[string]string) (map[string]string, error) {
	return updates, nil
}
func (*receiverAdapterStub) ResolveIdentity(context.Context, channelruntime.ResolvedChannelCredential) (channelruntime.InstallationIdentity, error) {
	return channelruntime.InstallationIdentity{ExternalTenantID: "tenant", ExternalSelfID: "bot"}, nil
}
func (*receiverAdapterStub) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{}
}
