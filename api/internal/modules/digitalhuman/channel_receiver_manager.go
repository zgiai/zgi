package digitalhuman

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

const channelReceiverReconcileInterval = 5 * time.Second

// ChannelReceiverManager reconciles long-lived provider receivers against the
// durable installation catalog. It owns no provider protocol details.
type ChannelReceiverManager struct {
	store       ChannelReceiverStore
	providers   *channelruntime.Registry
	credentials ChannelCredentialResolver
	channels    ChannelInboundMessageService

	mu       sync.RWMutex
	running  map[uuid.UUID]*channelReceiverRun
	observed map[uuid.UUID]int64
	statuses map[uuid.UUID]channelruntime.ReceiverStatus
	cancel   context.CancelFunc
	done     chan struct{}
	workers  sync.WaitGroup
}

type ChannelInboundMessageService interface {
	HandleInboundMessage(context.Context, uuid.UUID, *channelruntime.InboundMessage, time.Time) (*ChannelWebhookResult, error)
}

type channelReceiverRun struct {
	revision int64
	cancel   context.CancelFunc
}

func NewChannelReceiverManager(
	store ChannelReceiverStore,
	providers *channelruntime.Registry,
	credentials ChannelCredentialResolver,
	channels ChannelInboundMessageService,
) *ChannelReceiverManager {
	if store == nil || providers == nil || credentials == nil || channels == nil {
		panic("channel receiver manager requires store, providers, credentials, and channel service")
	}
	return &ChannelReceiverManager{
		store: store, providers: providers, credentials: credentials, channels: channels,
		running: make(map[uuid.UUID]*channelReceiverRun), observed: make(map[uuid.UUID]int64),
		statuses: make(map[uuid.UUID]channelruntime.ReceiverStatus),
	}
}

// Start begins reconciliation and returns without waiting for external
// providers so API startup is never blocked by one channel connection.
func (manager *ChannelReceiverManager) Start(context.Context) error {
	manager.mu.Lock()
	if manager.cancel != nil {
		manager.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.cancel = cancel
	manager.done = make(chan struct{})
	done := manager.done
	manager.mu.Unlock()
	go func() {
		defer close(done)
		manager.run(ctx)
	}()
	return nil
}

func (manager *ChannelReceiverManager) Stop(ctx context.Context) error {
	manager.mu.Lock()
	cancel, done := manager.cancel, manager.done
	manager.cancel, manager.done = nil, nil
	for _, receiver := range manager.running {
		receiver.cancel()
	}
	manager.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	stopped := make(chan struct{})
	go func() {
		<-done
		manager.workers.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (manager *ChannelReceiverManager) Status(installationID uuid.UUID) channelruntime.ReceiverStatus {
	manager.mu.RLock()
	status, ok := manager.statuses[installationID]
	manager.mu.RUnlock()
	if !ok {
		return channelruntime.ReceiverStatus{Mode: "webhook", State: channelruntime.ReceiverStateNotApplicable}
	}
	return status
}

func (manager *ChannelReceiverManager) run(ctx context.Context) {
	manager.reconcile(ctx)
	ticker := time.NewTicker(channelReceiverReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			manager.reconcile(ctx)
		}
	}
}

func (manager *ChannelReceiverManager) reconcile(ctx context.Context) {
	installations, err := manager.store.ListActiveChannelInstallations(ctx)
	if err != nil {
		return
	}
	active := make(map[uuid.UUID]struct{}, len(installations))
	for _, installation := range installations {
		if installation == nil || installation.ID == uuid.Nil {
			continue
		}
		active[installation.ID] = struct{}{}
		registered, resolveErr := manager.providers.ResolveProvider(installation.Provider)
		if resolveErr != nil || registered.Receiver == nil {
			continue
		}
		manager.ensureReceiver(ctx, installation, registered.Receiver)
	}

	manager.mu.Lock()
	for installationID, receiver := range manager.running {
		if _, ok := active[installationID]; !ok {
			receiver.cancel()
			delete(manager.running, installationID)
			manager.statuses[installationID] = channelruntime.ReceiverStatus{
				Mode: "stream", State: channelruntime.ReceiverStateDisconnected, UpdatedAt: time.Now().UTC(),
			}
		}
	}
	manager.mu.Unlock()
}

func (manager *ChannelReceiverManager) ensureReceiver(
	ctx context.Context,
	installation *channelruntime.Installation,
	receiver channelruntime.Receiver,
) {
	manager.mu.Lock()
	if current := manager.running[installation.ID]; current != nil {
		if current.revision == installation.Revision {
			manager.mu.Unlock()
			return
		}
		current.cancel()
		delete(manager.running, installation.ID)
	}
	if manager.observed[installation.ID] == installation.Revision && manager.statuses[installation.ID].Mode == "webhook" {
		manager.mu.Unlock()
		return
	}
	manager.mu.Unlock()

	release, acquired, err := manager.store.TryAcquireChannelReceiverLease(ctx, installation.ID)
	if err != nil || !acquired {
		return
	}
	credential, err := manager.credentials.ResolveCredential(ctx, installation)
	if err != nil {
		release()
		manager.setStatus(installation.ID, channelruntime.ReceiverStatus{
			Mode: "stream", State: channelruntime.ReceiverStateError, ErrorCode: "credential_unavailable", UpdatedAt: time.Now().UTC(),
		})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(receiver.Mode(*credential)))
	if mode != "stream" {
		credential.Destroy()
		release()
		manager.mu.Lock()
		manager.observed[installation.ID] = installation.Revision
		manager.statuses[installation.ID] = channelruntime.ReceiverStatus{
			Mode: "webhook", State: channelruntime.ReceiverStateNotApplicable, UpdatedAt: time.Now().UTC(),
		}
		manager.mu.Unlock()
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	entry := &channelReceiverRun{revision: installation.Revision, cancel: cancel}
	manager.mu.Lock()
	manager.running[installation.ID] = entry
	manager.observed[installation.ID] = installation.Revision
	manager.statuses[installation.ID] = channelruntime.ReceiverStatus{
		Mode: "stream", State: channelruntime.ReceiverStateConnecting, UpdatedAt: time.Now().UTC(),
	}
	manager.mu.Unlock()

	manager.workers.Add(1)
	go func() {
		defer manager.workers.Done()
		manager.runReceiver(runCtx, installation, receiver, credential, release, entry)
	}()
}

func (manager *ChannelReceiverManager) runReceiver(
	ctx context.Context,
	installation *channelruntime.Installation,
	receiver channelruntime.Receiver,
	credential *channelruntime.ResolvedChannelCredential,
	release func(),
	entry *channelReceiverRun,
) {
	defer credential.Destroy()
	defer release()
	request := channelruntime.ReceiverRequest{
		InstallationID: installation.ID,
		Identity: channelruntime.InstallationIdentity{
			ExternalTenantID: installation.ExternalTenantID,
			ExternalSelfID:   installation.ExternalBotID,
		},
		Credential: *credential,
		OnConnected: func() {
			manager.setStatus(installation.ID, channelruntime.ReceiverStatus{
				Mode: "stream", State: channelruntime.ReceiverStateConnected, UpdatedAt: time.Now().UTC(),
			})
		},
	}
	err := receiver.Run(ctx, request, func(messageCtx context.Context, message *channelruntime.InboundMessage) error {
		_, handleErr := manager.channels.HandleInboundMessage(messageCtx, installation.ID, message, time.Now().UTC())
		return handleErr
	})

	manager.mu.Lock()
	if manager.running[installation.ID] == entry {
		delete(manager.running, installation.ID)
		status := channelruntime.ReceiverStatus{
			Mode: "stream", State: channelruntime.ReceiverStateDisconnected, UpdatedAt: time.Now().UTC(),
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			status.State = channelruntime.ReceiverStateError
			status.ErrorCode = "connection_failed"
		}
		manager.statuses[installation.ID] = status
	}
	manager.mu.Unlock()
}

func (manager *ChannelReceiverManager) setStatus(installationID uuid.UUID, status channelruntime.ReceiverStatus) {
	manager.mu.Lock()
	manager.statuses[installationID] = status
	manager.mu.Unlock()
}
