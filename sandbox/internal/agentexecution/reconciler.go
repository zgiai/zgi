package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultCleanupLease = time.Minute
	defaultRetryDelay   = 15 * time.Second
)

type ReconcilerOptions struct {
	OwnerID       string
	Now           func() time.Time
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	Checkpoints   WorkspaceCheckpointStore
}

type Reconciler struct {
	repository    CleanupRepository
	backends      map[string]Backend
	ownerID       string
	now           func() time.Time
	leaseDuration time.Duration
	retryDelay    time.Duration
	checkpoints   WorkspaceCheckpointStore
}

func NewReconciler(repository CleanupRepository, backends map[string]Backend, options ReconcilerOptions) (*Reconciler, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: cleanup repository is required", ErrInvalidRequest)
	}
	options.OwnerID = strings.TrimSpace(options.OwnerID)
	if options.OwnerID == "" {
		return nil, fmt.Errorf("%w: cleanup owner is required", ErrInvalidRequest)
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = defaultCleanupLease
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = defaultRetryDelay
	}
	return &Reconciler{
		repository:    repository,
		backends:      copyBackends(backends),
		ownerID:       options.OwnerID,
		now:           options.Now,
		leaseDuration: options.LeaseDuration,
		retryDelay:    options.RetryDelay,
		checkpoints:   options.Checkpoints,
	}, nil
}

// ReconcileOne claims and processes at most one cleanup task. The boolean is
// false when no due work exists. Backend deletion is intentionally repeatable
// so an expired claim can safely be recovered after a worker crash.
func (r *Reconciler) ReconcileOne(ctx context.Context) (bool, error) {
	now := r.now().UTC()
	task, claimed, err := r.repository.ClaimCleanup(ctx, r.ownerID, now, now.Add(r.leaseDuration))
	if err != nil || !claimed {
		return claimed, err
	}
	if task.ResourceType == "checkpoint" && task.Action == "delete" {
		if r.checkpoints == nil {
			return true, r.retryCleanup(ctx, task, ErrBackendUnavailable)
		}
		if err := r.checkpoints.Delete(ctx, task.ResourceID); err != nil {
			return true, r.retryCleanup(ctx, task, err)
		}
		finishedAt := r.now().UTC()
		return true, r.repository.CompleteCleanup(ctx, task.ID, r.ownerID, true, "", finishedAt, finishedAt)
	}
	if task.ResourceType == "allocation_attempt" && task.Action == "reconcile" {
		return true, r.reconcileAllocationAttempt(ctx, task)
	}
	backend, ok := r.backends[task.Backend]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrBackendUnavailable, task.Backend)
		return true, r.retryCleanup(ctx, task, err)
	}
	if task.Action != "destroy" || task.ResourceType != "binding" {
		err := fmt.Errorf("%w: unsupported cleanup %s/%s", ErrInvalidRequest, task.ResourceType, task.Action)
		return true, r.retryCleanup(ctx, task, err)
	}
	if err := backend.DestroyBinding(ctx, Allocation{BindingRef: task.BackendRef, WorkspaceRef: task.WorkspaceRef}); err != nil {
		return true, r.retryCleanup(ctx, task, err)
	}
	finishedAt := r.now().UTC()
	if err := r.recordCleanedBinding(ctx, task, finishedAt); err != nil {
		return true, r.retryCleanup(ctx, task, err)
	}
	if err := r.repository.CompleteCleanup(ctx, task.ID, r.ownerID, true, "", finishedAt, finishedAt); err != nil {
		return true, err
	}
	return true, nil
}

func (r *Reconciler) reconcileAllocationAttempt(ctx context.Context, task CleanupTask) error {
	backend, ok := r.backends[task.Backend]
	if !ok {
		return r.retryCleanup(ctx, task, ErrBackendUnavailable)
	}
	discoverer, ok := backend.(AllocationDiscoverer)
	if !ok || task.BindingID == "" {
		return r.retryCleanup(ctx, task, ErrInvalidRequest)
	}
	allocations, err := discoverer.FindAllocations(ctx, task.ResourceID)
	if err != nil {
		return r.retryCleanup(ctx, task, err)
	}
	binding, err := r.repository.Binding(ctx, task.BindingID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return r.retryCleanup(ctx, task, err)
	}
	for _, allocation := range allocations {
		if err == nil && binding.Backend == task.Backend && binding.BackendRef == allocation.BindingRef &&
			binding.Status != BindingStatusDeleted && binding.Status != BindingStatusDeleting && binding.Status != BindingStatusFailed {
			continue
		}
		if destroyErr := backend.DestroyBinding(ctx, allocation); destroyErr != nil {
			return r.retryCleanup(ctx, task, destroyErr)
		}
	}
	finishedAt := r.now().UTC()
	return r.repository.CompleteCleanup(ctx, task.ID, r.ownerID, true, "", finishedAt, finishedAt)
}

func (r *Reconciler) recordCleanedBinding(ctx context.Context, task CleanupTask, now time.Time) error {
	binding, err := r.repository.Binding(ctx, task.ResourceID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	previousGeneration := binding.Generation
	switch binding.Status {
	case BindingStatusDeleted:
		return nil
	case BindingStatusDeleting:
		if binding.CheckpointKey != "" || binding.PendingCheckpointKey != "" {
			// Backend deletion is complete, but a retryable checkpoint deletion
			// must finish before the Binding can be marked deleted.
			return nil
		}
		binding.Status = BindingStatusDeleted
	case BindingStatusAllocating, BindingStatusFailed:
		binding.Status = BindingStatusFailed
		binding.FailureReason = "backend allocation was removed by cleanup reconciliation"
	default:
		return fmt.Errorf("%w: refusing to clean binding in %s", ErrInvalidTransition, binding.Status)
	}
	binding.Generation++
	binding.UpdatedAt = now
	return r.repository.UpdateBinding(ctx, binding, previousGeneration)
}

func (r *Reconciler) retryCleanup(ctx context.Context, task CleanupTask, cause error) error {
	now := r.now().UTC()
	completeErr := r.repository.CompleteCleanup(ctx, task.ID, r.ownerID, false, limitFailureReason(cause.Error()), now.Add(r.retryDelay), now)
	return errors.Join(cause, completeErr)
}

type LeaseManagerOptions struct {
	Now   func() time.Time
	NewID func(string) (string, error)
	TTL   time.Duration
}

type LeaseManager struct {
	repository LeaseRepository
	now        func() time.Time
	newID      func(string) (string, error)
	ttl        time.Duration
}

func NewLeaseManager(repository LeaseRepository, options LeaseManagerOptions) (*LeaseManager, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: lease repository is required", ErrInvalidRequest)
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.TTL <= 0 {
		options.TTL = time.Minute
	}
	return &LeaseManager{repository: repository, now: options.Now, newID: options.NewID, ttl: options.TTL}, nil
}

func (m *LeaseManager) Acquire(ctx context.Context, bindingID string, ownerID string) (AdmissionLease, error) {
	bindingID = strings.TrimSpace(bindingID)
	ownerID = strings.TrimSpace(ownerID)
	if bindingID == "" || ownerID == "" {
		return AdmissionLease{}, fmt.Errorf("%w: binding and lease owner are required", ErrInvalidRequest)
	}
	id, err := m.newID("arl")
	if err != nil {
		return AdmissionLease{}, fmt.Errorf("create lease id: %w", err)
	}
	now := m.now().UTC()
	return m.repository.AcquireLease(ctx, AdmissionLease{
		ID: id, BindingID: bindingID, OwnerID: ownerID, Generation: 1,
		ExpiresAt: now.Add(m.ttl), CreatedAt: now, UpdatedAt: now,
	}, now)
}

func (m *LeaseManager) Renew(ctx context.Context, lease AdmissionLease) (AdmissionLease, error) {
	now := m.now().UTC()
	return m.repository.RenewLease(ctx, lease.ID, lease.OwnerID, lease.Generation, now, now.Add(m.ttl))
}

func (m *LeaseManager) Release(ctx context.Context, lease AdmissionLease) error {
	return m.repository.ReleaseLease(ctx, lease.ID, lease.OwnerID, lease.Generation)
}

func copyBackends(backends map[string]Backend) map[string]Backend {
	result := make(map[string]Backend, len(backends))
	for name, backend := range backends {
		if strings.TrimSpace(name) != "" && backend != nil {
			result[strings.TrimSpace(name)] = backend
		}
	}
	return result
}
