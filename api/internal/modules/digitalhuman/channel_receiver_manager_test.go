package digitalhuman

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

// TestChannelReceiverManagerReconnectsAfterPersonalConnectionRotation covers a
// Connector-owned credential rotating without an installation revision change:
// the old transport must stop and the replacement must open with the new lease.
func TestChannelReceiverManagerReconnectsAfterPersonalConnectionRotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		installation := &channelruntime.Installation{
			ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
			Provider: "stream-test", ConnectionID: uuid.New(), CredentialSource: channelruntime.CredentialSourcePersonalConnection,
			Status: channelruntime.InstallationStatusActive, Revision: 1,
		}
		store := &receiverStoreStub{installation: installation}
		credentials := &rotatingReceiverCredentialStub{revision: 1, version: 1}
		var mu sync.Mutex
		started := make([]int, 0, 2)
		registry, err := channelruntime.NewRegistry(channelruntime.Provider{
			Adapter: &receiverAdapterStub{}, Provisioner: &receiverAdapterStub{},
			Receiver: lifecycleReceiver{run: func(ctx context.Context, request channelruntime.ReceiverRequest) error {
				mu.Lock()
				started = append(started, request.Credential.Version)
				mu.Unlock()
				request.OnConnected()
				<-ctx.Done()
				return ctx.Err()
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		manager := NewChannelReceiverManager(store, registry, credentials, &receiverSinkStub{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		manager.reconcile(ctx)
		synctest.Wait()
		credentials.set(2, 2)
		manager.reconcile(ctx)
		synctest.Wait()
		manager.reconcile(ctx)
		synctest.Wait()

		mu.Lock()
		versions := append([]int(nil), started...)
		mu.Unlock()
		if len(versions) != 2 || versions[0] != 1 || versions[1] != 2 {
			t.Fatalf("receiver credential versions = %#v, want [1 2]", versions)
		}
		if status := manager.Status(installation.ID); status.State != channelruntime.ReceiverStateConnected || status.Revision != 1 {
			t.Fatalf("receiver status = %#v", status)
		}
	})
}

// TestChannelReceiverManagerStopsWhenPersonalConnectionIsRevoked covers a
// deleted or unreadable owner-scoped connection: the transport must stop and
// the failure must stay credential-free.
func TestChannelReceiverManagerStopsWhenPersonalConnectionIsRevoked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		installation := &channelruntime.Installation{
			ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
			Provider: "stream-test", ConnectionID: uuid.New(), CredentialSource: channelruntime.CredentialSourcePersonalConnection,
			Status: channelruntime.InstallationStatusActive, Revision: 1,
		}
		store := &receiverStoreStub{installation: installation}
		credentials := &rotatingReceiverCredentialStub{revision: 1, version: 1}
		var active atomic.Int32
		registry, err := channelruntime.NewRegistry(channelruntime.Provider{
			Adapter: &receiverAdapterStub{}, Provisioner: &receiverAdapterStub{},
			Receiver: lifecycleReceiver{run: func(ctx context.Context, request channelruntime.ReceiverRequest) error {
				active.Add(1)
				defer active.Add(-1)
				request.OnConnected()
				<-ctx.Done()
				return ctx.Err()
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		manager := NewChannelReceiverManager(store, registry, credentials, &receiverSinkStub{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		manager.reconcile(ctx)
		synctest.Wait()
		if active.Load() != 1 {
			t.Fatal("receiver did not start for a live connection")
		}
		credentials.revoke(ErrChannelNotReady)
		manager.reconcile(ctx)
		synctest.Wait()
		if active.Load() != 0 {
			t.Fatal("revoked connection still has an active receiver")
		}
		status := manager.Status(installation.ID)
		if status.State != channelruntime.ReceiverStateError || status.ErrorCode != "credential_unavailable" {
			t.Fatalf("receiver status = %#v", status)
		}
	})
}

// TestChannelReceiverManagerLeaseKeyTracksCredentialSource pins the ownership
// key: one owner-scoped PersonalConnection is leased once across installations,
// while an embedded installation keeps its installation ID.
func TestChannelReceiverManagerLeaseKeyTracksCredentialSource(t *testing.T) {
	for _, test := range []struct {
		name     string
		personal bool
	}{
		{name: "owner scoped personal connection", personal: true},
		{name: "embedded installation", personal: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			installation := &channelruntime.Installation{
				ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
				Provider: "stream-test", Status: channelruntime.InstallationStatusActive, Revision: 1,
			}
			wantKey := installation.ID
			if test.personal {
				installation.ConnectionID = uuid.New()
				installation.CredentialSource = channelruntime.CredentialSourcePersonalConnection
				wantKey = installation.ConnectionID
			} else {
				installation.ConnectionID = installation.ID
				installation.CredentialSource = channelruntime.CredentialSourceEmbedded
			}
			store := &receiverStoreStub{installation: installation}
			registry, err := channelruntime.NewRegistry(channelruntime.Provider{
				Adapter: &receiverAdapterStub{}, Provisioner: &receiverAdapterStub{}, Receiver: receiverStub{},
			})
			if err != nil {
				t.Fatal(err)
			}
			manager := NewChannelReceiverManager(store, registry, receiverCredentialStub{}, &receiverSinkStub{received: make(chan *channelruntime.InboundMessage, 1)})
			ctx, cancel := context.WithCancel(context.Background())
			manager.reconcile(ctx)
			cancel()

			store.mu.Lock()
			keys := append([]uuid.UUID(nil), store.leaseKeys...)
			store.mu.Unlock()
			if len(keys) != 1 || keys[0] != wantKey {
				t.Fatalf("receiver lease keys = %#v, want [%s]", keys, wantKey)
			}
		})
	}
}

type rotatingReceiverCredentialStub struct {
	mu       sync.Mutex
	revision int
	version  int
	err      error
}

func (stub *rotatingReceiverCredentialStub) set(revision, version int) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.revision, stub.version = revision, version
}

func (stub *rotatingReceiverCredentialStub) revoke(err error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.err = err
}

func (stub *rotatingReceiverCredentialStub) ResolveCredential(context.Context, *channelruntime.Installation) (*channelruntime.ResolvedChannelCredential, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.err != nil {
		return nil, stub.err
	}
	return &channelruntime.ResolvedChannelCredential{
		Values: map[string]string{"mode": "stream"}, Revision: stub.revision, Version: stub.version,
	}, nil
}

func (stub *rotatingReceiverCredentialStub) ResolveIdentity(context.Context, *channelruntime.Installation) (string, string, error) {
	return "tenant", "bot", nil
}

func (stub *rotatingReceiverCredentialStub) ContextReadiness(context.Context, *channelruntime.Installation) ChannelContextReadiness {
	return ChannelContextReadiness{Status: ChannelContextReadinessUnavailable}
}

type receiverStoreStub struct {
	installation *channelruntime.Installation
	mu           sync.Mutex
	leased       bool
	leaseKeys    []uuid.UUID
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

func (store *receiverStoreStub) TryAcquireChannelReceiverLease(_ context.Context, receiverKey uuid.UUID) (*ChannelReceiverLease, bool, error) {
	store.mu.Lock()
	store.leaseKeys = append(store.leaseKeys, receiverKey)
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
