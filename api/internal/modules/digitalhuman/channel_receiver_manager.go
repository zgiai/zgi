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

// ChannelReceiverManager owns provider connection lifetimes and their leases.
// Neither protocol details nor model execution belong in this manager.
type ChannelReceiverManager struct {
	store       ChannelReceiverStore
	providers   *channelruntime.Registry
	credentials ChannelCredentialResolver
	channels    ChannelInboundMessageService

	mu         sync.RWMutex
	running    map[uuid.UUID]*channelReceiverRun
	observed   map[uuid.UUID]int64
	blocked    map[uuid.UUID]int64
	retryAfter map[uuid.UUID]time.Time
	statuses   map[uuid.UUID]channelruntime.ReceiverStatus
	cancel     context.CancelFunc
	done       chan struct{}
	workers    sync.WaitGroup
}

type ChannelInboundMessageService interface {
	HandleInboundMessage(context.Context, uuid.UUID, int64, *channelruntime.InboundMessage, time.Time) (*ChannelWebhookResult, error)
}

type channelReceiverRun struct {
	revision int64
	mode     string
	cancel   context.CancelCauseFunc
	lease    *ChannelReceiverLease
}

func NewChannelReceiverManager(store ChannelReceiverStore, providers *channelruntime.Registry, credentials ChannelCredentialResolver, channels ChannelInboundMessageService) *ChannelReceiverManager {
	if store == nil || providers == nil || credentials == nil || channels == nil {
		panic("channel receiver manager requires store, providers, credentials, and channel service")
	}
	return &ChannelReceiverManager{
		store: store, providers: providers, credentials: credentials, channels: channels,
		running: make(map[uuid.UUID]*channelReceiverRun), observed: make(map[uuid.UUID]int64),
		blocked: make(map[uuid.UUID]int64), retryAfter: make(map[uuid.UUID]time.Time),
		statuses: make(map[uuid.UUID]channelruntime.ReceiverStatus),
	}
}

// Start is non-blocking; an unavailable provider never blocks API startup.
func (manager *ChannelReceiverManager) Start(context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.cancel = cancel
	manager.done = make(chan struct{})
	done := manager.done
	go func() {
		defer close(done)
		manager.run(ctx)
		manager.workers.Wait()
	}()
	return nil
}

func (manager *ChannelReceiverManager) Stop(ctx context.Context) error {
	manager.mu.Lock()
	cancel, done := manager.cancel, manager.done
	manager.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		manager.mu.Lock()
		if manager.done == done {
			manager.cancel, manager.done = nil, nil
		}
		manager.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (manager *ChannelReceiverManager) Status(id uuid.UUID) channelruntime.ReceiverStatus {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.statuses[id]
}

// Retry releases a permanent failure after an explicit connection test.
// It does not interrupt a healthy transport or create a second connection.
func (manager *ChannelReceiverManager) Retry(id uuid.UUID) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	delete(manager.blocked, id)
	delete(manager.retryAfter, id)
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
		if installation == nil || installation.ID == uuid.Nil || ctx.Err() != nil {
			continue
		}
		active[installation.ID] = struct{}{}
		registered, err := manager.providers.ResolveProvider(installation.Provider)
		if err == nil && registered.Receiver != nil {
			manager.ensureReceiver(ctx, installation, registered.Receiver)
		}
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for id, entry := range manager.running {
		if _, ok := active[id]; !ok {
			// Keep the entry until Run returns and releases its lease. Rapid
			// disable/re-enable must not overlap the previous connection.
			entry.cancel(context.Canceled)
		}
	}
	for id := range manager.observed {
		if _, ok := active[id]; !ok {
			delete(manager.observed, id)
			delete(manager.blocked, id)
			delete(manager.retryAfter, id)
		}
	}
}

func (manager *ChannelReceiverManager) ensureReceiver(ctx context.Context, installation *channelruntime.Installation, receiver channelruntime.Receiver) {
	manager.mu.Lock()
	if current := manager.running[installation.ID]; current != nil {
		if current.revision != installation.Revision {
			current.cancel(context.Canceled)
		}
		manager.mu.Unlock()
		return
	}
	if manager.observed[installation.ID] == installation.Revision &&
		(manager.statuses[installation.ID].Mode == "webhook" || manager.blocked[installation.ID] == installation.Revision ||
			time.Now().Before(manager.retryAfter[installation.ID])) {
		manager.mu.Unlock()
		return
	}
	manager.mu.Unlock()

	credential, err := manager.credentials.ResolveCredential(ctx, installation)
	if err != nil {
		manager.recordFailure(installation, "", &channelruntime.ReceiverError{Code: "credential_unavailable", Retryable: true})
		return
	}
	mode := strings.TrimSpace(receiver.Mode(*credential))
	if mode == "webhook" {
		credential.Destroy()
		manager.mu.Lock()
		manager.observed[installation.ID] = installation.Revision
		manager.statuses[installation.ID] = channelruntime.ReceiverStatus{Mode: mode, Revision: installation.Revision, State: channelruntime.ReceiverStateNotApplicable}
		manager.mu.Unlock()
		return
	}
	leaseID := installation.ID
	if scoped, ok := receiver.(channelruntime.ReceiverLeaseScope); ok {
		leaseID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(scoped.LeaseScope(*credential)))
	}
	lease, acquired, err := manager.store.TryAcquireChannelReceiverLease(ctx, leaseID)
	if err != nil || !acquired {
		credential.Destroy()
		code := "connection_in_use"
		if err != nil {
			code = "lease_unavailable"
		}
		manager.recordFailure(installation, mode, &channelruntime.ReceiverError{Code: code, Retryable: true})
		return
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	entry := &channelReceiverRun{revision: installation.Revision, mode: mode, cancel: cancel, lease: lease}
	manager.mu.Lock()
	manager.running[installation.ID] = entry
	manager.observed[installation.ID] = installation.Revision
	delete(manager.blocked, installation.ID)
	manager.statuses[installation.ID] = channelruntime.ReceiverStatus{
		Mode: mode, Revision: installation.Revision, State: channelruntime.ReceiverStateConnecting, UpdatedAt: time.Now().UTC(),
	}
	manager.mu.Unlock()
	manager.workers.Go(func() { manager.runReceiver(runCtx, installation, receiver, credential, entry) })
}

func (manager *ChannelReceiverManager) runReceiver(ctx context.Context, installation *channelruntime.Installation, receiver channelruntime.Receiver, credential *channelruntime.ResolvedChannelCredential, entry *channelReceiverRun) {
	defer credential.Destroy()
	defer entry.lease.Release()
	defer entry.cancel(context.Canceled)
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		manager.watchLease(watchCtx, entry)
	}()
	state := func(next channelruntime.ReceiverConnectionState) {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if manager.running[installation.ID] == entry && ctx.Err() == nil {
			manager.statuses[installation.ID] = channelruntime.ReceiverStatus{
				Mode: entry.mode, Revision: entry.revision, State: next, UpdatedAt: time.Now().UTC(),
			}
		}
	}
	err := receiver.Run(ctx, channelruntime.ReceiverRequest{
		InstallationID: installation.ID, Credential: *credential,
		Identity:    channelruntime.InstallationIdentity{ExternalTenantID: installation.ExternalTenantID, ExternalSelfID: installation.ExternalBotID},
		OnConnected: func() { state(channelruntime.ReceiverStateConnected) }, OnStateChange: state,
	}, func(messageCtx context.Context, message *channelruntime.InboundMessage) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := manager.channels.HandleInboundMessage(messageCtx, installation.ID, entry.revision, message, time.Now().UTC())
		return err
	})
	stopWatch()
	<-watchDone
	if cause := context.Cause(ctx); cause != nil {
		err = cause
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.running[installation.ID] != entry {
		return
	}
	delete(manager.running, installation.ID)
	if err != nil && !errors.Is(err, context.Canceled) {
		manager.recordFailureLocked(installation, entry.mode, err)
		return
	}
	manager.statuses[installation.ID] = channelruntime.ReceiverStatus{
		Mode: entry.mode, Revision: entry.revision, State: channelruntime.ReceiverStateDisconnected, UpdatedAt: time.Now().UTC(),
	}
}

func (manager *ChannelReceiverManager) watchLease(ctx context.Context, entry *channelReceiverRun) {
	if entry.lease.Check == nil {
		return
	}
	ticker := time.NewTicker(channelReceiverReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := entry.lease.Check(checkCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				entry.cancel(&channelruntime.ReceiverError{Code: "lease_unavailable", Retryable: true, Cause: err})
				return
			}
		}
	}
}

func (manager *ChannelReceiverManager) recordFailure(installation *channelruntime.Installation, mode string, err error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.recordFailureLocked(installation, mode, err)
}

func (manager *ChannelReceiverManager) recordFailureLocked(installation *channelruntime.Installation, mode string, err error) {
	failure := &channelruntime.ReceiverError{Code: "connection_failed", Retryable: true}
	var classified *channelruntime.ReceiverError
	if errors.As(err, &classified) {
		failure = classified
	}
	manager.observed[installation.ID] = installation.Revision
	manager.statuses[installation.ID] = channelruntime.ReceiverStatus{
		Mode: mode, Revision: installation.Revision, State: channelruntime.ReceiverStateError, ErrorCode: failure.Code, UpdatedAt: time.Now().UTC(),
	}
	if !failure.Retryable {
		manager.blocked[installation.ID] = installation.Revision
	} else {
		manager.retryAfter[installation.ID] = time.Now().Add(15 * time.Second)
	}
}
