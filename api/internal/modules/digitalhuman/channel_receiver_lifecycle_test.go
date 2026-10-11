package digitalhuman

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

type lifecycleReceiver struct {
	run func(context.Context, channelruntime.ReceiverRequest) error
}

func (lifecycleReceiver) Mode(credential channelruntime.ResolvedChannelCredential) string {
	return credential.Values["mode"]
}
func (receiver lifecycleReceiver) Run(ctx context.Context, request channelruntime.ReceiverRequest, _ channelruntime.InboundMessageHandler) error {
	return receiver.run(ctx, request)
}

func lifecycleManager(t *testing.T, receiver lifecycleReceiver) (*ChannelReceiverManager, *receiverStoreStub) {
	t.Helper()
	store := &receiverStoreStub{installation: &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Provider: "stream-test",
		ConnectionID: uuid.New(), Status: channelruntime.InstallationStatusActive, Revision: 1,
	}}
	registry, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: &receiverAdapterStub{}, Provisioner: &receiverAdapterStub{}, Receiver: receiver})
	if err != nil {
		t.Fatal(err)
	}
	return NewChannelReceiverManager(store, registry, receiverCredentialStub{mode: "websocket"}, &receiverSinkStub{}), store
}

func TestChannelReceiverPermanentFailureRequiresExplicitRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		manager, store := lifecycleManager(t, lifecycleReceiver{run: func(context.Context, channelruntime.ReceiverRequest) error {
			calls.Add(1)
			return &channelruntime.ReceiverError{Code: "credentials_invalid"}
		}})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		manager.reconcile(ctx)
		synctest.Wait()
		for range 3 {
			time.Sleep(time.Minute)
			manager.reconcile(ctx)
			synctest.Wait()
		}
		if calls.Load() != 1 || manager.Status(store.installation.ID).ErrorCode != "credentials_invalid" {
			t.Fatal("permanent configuration failure retried or hidden")
		}
		manager.Retry(store.installation.ID)
		manager.reconcile(ctx)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatalf("explicit retry calls = %d", calls.Load())
		}
	})
}

func TestChannelReceiverRotationDisableAndWebhookDoNotOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls, active, maximum atomic.Int32
		var previousState func(channelruntime.ReceiverConnectionState)
		manager, store := lifecycleManager(t, lifecycleReceiver{run: func(ctx context.Context, request channelruntime.ReceiverRequest) error {
			calls.Add(1)
			n := active.Add(1)
			defer active.Add(-1)
			if n > maximum.Load() {
				maximum.Store(n)
			}
			previousState = request.OnStateChange
			request.OnConnected()
			<-ctx.Done()
			return ctx.Err()
		}})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		manager.reconcile(ctx)
		synctest.Wait()
		oldState := previousState
		store.installation.Revision++
		manager.reconcile(ctx)
		synctest.Wait()
		manager.reconcile(ctx)
		synctest.Wait()
		oldState(channelruntime.ReceiverStateError)
		if status := manager.Status(store.installation.ID); status.State != channelruntime.ReceiverStateConnected || status.Revision != 2 {
			t.Fatalf("stale state callback won: %#v", status)
		}
		store.installation.Status = channelruntime.InstallationStatusDisabled
		manager.reconcile(ctx)
		synctest.Wait()
		if active.Load() != 0 {
			t.Fatal("disabled connection still active")
		}
		store.installation.Status = channelruntime.InstallationStatusActive
		store.installation.Revision++
		manager.credentials = receiverCredentialStub{mode: "webhook"}
		manager.reconcile(ctx)
		synctest.Wait()
		if calls.Load() != 2 || maximum.Load() != 1 || manager.Status(store.installation.ID).Mode != "webhook" {
			t.Fatalf("calls=%d maxActive=%d", calls.Load(), maximum.Load())
		}
	})
}

func TestChannelReceiverLeaseLossStopsTransportAndRestartRecovers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var lost atomic.Bool
		manager, store := lifecycleManager(t, lifecycleReceiver{run: func(ctx context.Context, request channelruntime.ReceiverRequest) error {
			request.OnConnected()
			<-ctx.Done()
			return ctx.Err()
		}})
		store.check = func(context.Context) error {
			if lost.Load() {
				return errors.New("database disconnected")
			}
			return nil
		}
		if err := manager.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		lost.Store(true)
		time.Sleep(6 * time.Second)
		synctest.Wait()
		if status := manager.Status(store.installation.ID); status.ErrorCode != "lease_unavailable" {
			t.Fatalf("lease loss = %#v", status)
		}
		if store.leased {
			t.Fatal("lost lease was not released")
		}
		if err := manager.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		lost.Store(false)
		manager.Retry(store.installation.ID)
		if err := manager.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if manager.Status(store.installation.ID).State != channelruntime.ReceiverStateConnected {
			t.Fatal("restart did not recover")
		}
		if err := manager.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestChannelReceiverSharedLeaseAllowsOnlyOneManager(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		receiver := lifecycleReceiver{run: func(ctx context.Context, request channelruntime.ReceiverRequest) error {
			calls.Add(1)
			request.OnConnected()
			<-ctx.Done()
			return ctx.Err()
		}}
		first, store := lifecycleManager(t, receiver)
		second := NewChannelReceiverManager(store, first.providers, first.credentials, first.channels)
		ctx, cancel := context.WithCancel(t.Context())
		first.reconcile(ctx)
		synctest.Wait()
		second.reconcile(ctx)
		synctest.Wait()
		if calls.Load() != 1 || second.Status(store.installation.ID).ErrorCode != "connection_in_use" {
			t.Fatal("competing managers opened duplicate connections")
		}
		cancel()
		synctest.Wait()
		second.Retry(store.installation.ID)
		recoveryCtx, recoveryCancel := context.WithCancel(t.Context())
		second.reconcile(recoveryCtx)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatal("standby failed to acquire released lease")
		}
		recoveryCancel()
		synctest.Wait()
	})
}
