package agentexecution

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const defaultIdleTTL = 30 * time.Minute
const defaultRetentionTTL = 7 * 24 * time.Hour
const allocationAttemptGrace = 2 * time.Hour

type CreateBindingRequest struct {
	Scope          Scope
	AgentID        string
	ConversationID string
	ProfileID      string
	HomeSnapshotID string
	Backend        string
	IdempotencyKey string
}

type ServiceOptions struct {
	Now         func() time.Time
	NewID       func(string) (string, error)
	IdleTTL     time.Duration
	Checkpoints WorkspaceCheckpointStore
	Jobs        ActiveJobReader
}

type Service struct {
	repository  Repository
	backends    map[string]Backend
	now         func() time.Time
	newID       func(string) (string, error)
	idleTTL     time.Duration
	checkpoints WorkspaceCheckpointStore
	jobs        ActiveJobReader
}

func NewService(repository Repository, backends map[string]Backend, options ServiceOptions) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidRequest)
	}
	if len(backends) == 0 {
		return nil, fmt.Errorf("%w: at least one backend is required", ErrInvalidRequest)
	}
	if options.Checkpoints != nil && options.Jobs == nil {
		return nil, fmt.Errorf("%w: active job reader is required with workspace checkpoints", ErrInvalidRequest)
	}
	backendCopy := make(map[string]Backend, len(backends))
	for name, backend := range backends {
		name = strings.TrimSpace(name)
		if name == "" || backend == nil {
			return nil, fmt.Errorf("%w: backend name and implementation are required", ErrInvalidRequest)
		}
		backendCopy[name] = backend
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.IdleTTL <= 0 {
		options.IdleTTL = defaultIdleTTL
	}
	return &Service{
		repository:  repository,
		backends:    backendCopy,
		now:         options.Now,
		newID:       options.NewID,
		idleTTL:     options.IdleTTL,
		checkpoints: options.Checkpoints,
		jobs:        options.Jobs,
	}, nil
}

func (s *Service) CreateBinding(ctx context.Context, request CreateBindingRequest) (Binding, error) {
	request = normalizeCreateRequest(request)
	if err := validateCreateRequest(request); err != nil {
		return Binding{}, err
	}
	backend, ok := s.backends[request.Backend]
	if !ok {
		return Binding{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, request.Backend)
	}
	bindingID, err := s.newID("arb")
	if err != nil {
		return Binding{}, fmt.Errorf("create binding id: %w", err)
	}
	workspaceID, err := s.newID("arw")
	if err != nil {
		return Binding{}, fmt.Errorf("create workspace id: %w", err)
	}
	now := s.now().UTC()
	binding := Binding{
		ID:                 bindingID,
		OrganizationID:     request.Scope.OrganizationID,
		SubjectID:          request.Scope.SubjectID,
		AgentID:            request.AgentID,
		ConversationID:     request.ConversationID,
		ProfileID:          request.ProfileID,
		HomeSnapshotID:     request.HomeSnapshotID,
		WorkspaceID:        workspaceID,
		Backend:            request.Backend,
		IdempotencyKey:     request.IdempotencyKey,
		RequestHash:        requestFingerprint(request),
		Status:             BindingStatusAllocating,
		Generation:         1,
		CreatedAt:          now,
		UpdatedAt:          now,
		LastActivityAt:     now,
		ExpiresAt:          now.Add(s.idleTTL),
		RetentionExpiresAt: now.Add(defaultRetentionTTL),
	}
	binding, created, err := s.repository.ReserveBinding(ctx, binding)
	if err != nil {
		return Binding{}, fmt.Errorf("reserve agent runtime binding: %w", err)
	}
	if !created {
		if binding.RequestHash != requestFingerprint(request) {
			return Binding{}, ErrIdempotencyConflict
		}
		return binding, nil
	}

	attemptID, err := s.scheduleAllocationCheck(ctx, binding, backend)
	if err != nil {
		return Binding{}, err
	}
	createContext, cancelCreate := context.WithTimeout(ctx, 90*time.Minute)
	defer cancelCreate()
	allocation, err := backend.CreateBinding(createContext, CreateSpec{
		BindingID:           binding.ID,
		AllocationAttemptID: attemptID,
		WorkspaceID:         binding.WorkspaceID,
		OrganizationID:      binding.OrganizationID,
		SubjectID:           binding.SubjectID,
		ProfileID:           binding.ProfileID,
		HomeSnapshotID:      binding.HomeSnapshotID,
	})
	if err != nil {
		cleanupErr := s.enqueueDestroy(ctx, binding, allocation, err)
		binding.Status = BindingStatusFailed
		binding.FailureReason = limitFailureReason(err.Error())
		binding.UpdatedAt = s.now().UTC()
		binding.Generation++
		if updateErr := s.repository.UpdateBinding(ctx, binding, binding.Generation-1); updateErr != nil {
			return Binding{}, errors.Join(fmt.Errorf("create backend binding: %w", err), fmt.Errorf("record failed binding: %w", updateErr), cleanupErr)
		}
		return binding, errors.Join(fmt.Errorf("create backend binding: %w", err), cleanupErr)
	}

	binding.BackendRef = allocation.BindingRef
	binding.WorkspaceRef = allocation.WorkspaceRef
	binding.Status = BindingStatusActive
	binding.FailureReason = ""
	binding.UpdatedAt = s.now().UTC()
	binding.LastActivityAt = binding.UpdatedAt
	binding.ExpiresAt = binding.UpdatedAt.Add(s.idleTTL)
	binding.RetentionExpiresAt = binding.UpdatedAt.Add(defaultRetentionTTL)
	binding.Generation++
	if err := s.repository.UpdateBinding(ctx, binding, binding.Generation-1); err != nil {
		cleanupErr := s.enqueueDestroy(ctx, binding, allocation, err)
		return Binding{}, errors.Join(fmt.Errorf("activate agent runtime binding: %w", err), cleanupErr)
	}
	return binding, nil
}

func (s *Service) GetBinding(ctx context.Context, scope Scope, bindingID string) (Binding, error) {
	binding, err := s.repository.Binding(ctx, strings.TrimSpace(bindingID))
	if err != nil {
		return Binding{}, err
	}
	if !owns(scope, binding) {
		return Binding{}, ErrNotFound
	}
	return binding, nil
}

func (s *Service) SuspendBinding(ctx context.Context, scope Scope, bindingID string, generation int64) (Binding, error) {
	binding, backend, err := s.mutableBinding(ctx, scope, bindingID, generation)
	if err != nil {
		return Binding{}, err
	}
	if binding.Status == BindingStatusSuspended {
		return binding, nil
	}
	if binding.Status != BindingStatusActive && binding.Status != BindingStatusSuspending {
		return Binding{}, fmt.Errorf("%w: cannot suspend %s", ErrInvalidTransition, binding.Status)
	}
	if s.checkpoints == nil && (binding.CheckpointKey != "" || binding.PendingCheckpointKey != "") {
		return Binding{}, fmt.Errorf("%w: workspace checkpoint storage is not configured", ErrBackendUnavailable)
	}
	previousGeneration := binding.Generation
	binding.Status = BindingStatusSuspending
	binding.Generation++
	if s.checkpoints != nil && binding.PendingCheckpointKey == "" {
		binding.PendingCheckpointKey = workspaceCheckpointKey(binding)
	}
	binding.UpdatedAt = s.now().UTC()
	if err := s.repository.UpdateBinding(ctx, binding, previousGeneration); err != nil {
		return Binding{}, err
	}
	allocation := Allocation{BindingRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef}
	if s.checkpoints != nil {
		if err := s.captureWorkspace(ctx, binding, backend, allocation); err != nil {
			return Binding{}, fmt.Errorf("checkpoint workspace before suspension: %w", err)
		}
	}
	if err := backend.SuspendBinding(ctx, allocation); err != nil {
		return Binding{}, fmt.Errorf("suspend backend binding: %w", err)
	}
	previousGeneration = binding.Generation
	if s.checkpoints != nil {
		binding.CheckpointKey = binding.PendingCheckpointKey
		binding.PendingCheckpointKey = ""
	}
	binding.Status = BindingStatusSuspended
	binding.Generation++
	binding.UpdatedAt = s.now().UTC()
	if err := s.repository.UpdateBinding(ctx, binding, previousGeneration); err != nil {
		return Binding{}, fmt.Errorf("record suspended binding: %w", err)
	}
	return binding, nil
}

func (s *Service) ResumeBinding(ctx context.Context, scope Scope, bindingID string, generation int64) (Binding, error) {
	binding, backend, err := s.mutableBinding(ctx, scope, bindingID, generation)
	if err != nil {
		return Binding{}, err
	}
	if binding.Status == BindingStatusActive {
		return binding, nil
	}
	if binding.Status != BindingStatusSuspended && binding.Status != BindingStatusResuming {
		return Binding{}, fmt.Errorf("%w: cannot resume %s", ErrInvalidTransition, binding.Status)
	}
	if !binding.RetentionExpiresAt.IsZero() && !s.now().UTC().Before(binding.RetentionExpiresAt) {
		return Binding{}, fmt.Errorf("%w: workspace retention expired", ErrInvalidTransition)
	}
	previousGeneration := binding.Generation
	binding.Status = BindingStatusResuming
	binding.Generation++
	binding.UpdatedAt = s.now().UTC()
	if err := s.repository.UpdateBinding(ctx, binding, previousGeneration); err != nil {
		return Binding{}, err
	}

	spec := CreateSpec{
		BindingID: binding.ID, WorkspaceID: binding.WorkspaceID,
		OrganizationID: binding.OrganizationID, SubjectID: binding.SubjectID,
		ProfileID: binding.ProfileID, HomeSnapshotID: binding.HomeSnapshotID,
	}
	allocation, err := backend.ResumeBinding(ctx, spec, Allocation{
		BindingRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef,
	})
	if errors.Is(err, ErrNotFound) && s.checkpoints != nil && binding.CheckpointKey != "" {
		var attemptID string
		attemptID, err = s.scheduleAllocationCheck(ctx, binding, backend)
		if err == nil {
			spec.AllocationAttemptID = attemptID
			rebuildContext, cancelRebuild := context.WithTimeout(ctx, 90*time.Minute)
			allocation, err = s.rebuildWorkspace(rebuildContext, binding, backend, spec)
			cancelRebuild()
		}
	}
	if err != nil {
		return Binding{}, fmt.Errorf("resume backend binding: %w", err)
	}
	previousGeneration = binding.Generation
	binding.BackendRef = allocation.BindingRef
	binding.WorkspaceRef = allocation.WorkspaceRef
	binding.Status = BindingStatusActive
	binding.FailureReason = ""
	binding.Generation++
	binding.UpdatedAt = s.now().UTC()
	binding.LastActivityAt = binding.UpdatedAt
	binding.ExpiresAt = binding.UpdatedAt.Add(s.idleTTL)
	binding.RetentionExpiresAt = binding.UpdatedAt.Add(defaultRetentionTTL)
	if err := s.repository.UpdateBinding(ctx, binding, previousGeneration); err != nil {
		cleanupErr := s.enqueueDestroy(ctx, binding, allocation, err)
		return Binding{}, errors.Join(fmt.Errorf("record resumed binding: %w", err), cleanupErr)
	}
	return binding, nil
}

func (s *Service) DestroyBinding(ctx context.Context, scope Scope, bindingID string, generation int64) (Binding, error) {
	binding, err := s.GetBinding(ctx, scope, bindingID)
	if err != nil {
		return Binding{}, err
	}
	if binding.Status == BindingStatusDeleted {
		return binding, nil
	}
	if s.checkpoints == nil && (binding.CheckpointKey != "" || binding.PendingCheckpointKey != "") {
		return Binding{}, fmt.Errorf("%w: workspace checkpoint storage is not configured", ErrBackendUnavailable)
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return Binding{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	if generation != binding.Generation {
		return Binding{}, ErrGenerationConflict
	}
	previousGeneration := binding.Generation
	binding.Status = BindingStatusDeleting
	binding.Generation++
	binding.UpdatedAt = s.now().UTC()
	if err := s.repository.UpdateBinding(ctx, binding, previousGeneration); err != nil {
		return Binding{}, err
	}
	allocation := Allocation{BindingRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef}
	if err := backend.DestroyBinding(ctx, allocation); err != nil {
		cleanupErr := s.enqueueDestroy(ctx, binding, allocation, err)
		return Binding{}, errors.Join(fmt.Errorf("destroy backend binding: %w", err), cleanupErr)
	}
	if s.checkpoints != nil {
		for _, key := range []string{binding.CheckpointKey, binding.PendingCheckpointKey} {
			if key == "" {
				continue
			}
			if err := s.checkpoints.Delete(ctx, key); err != nil {
				return Binding{}, fmt.Errorf("delete workspace checkpoint: %w", err)
			}
		}
	}
	previousGeneration = binding.Generation
	binding.Status = BindingStatusDeleted
	binding.CheckpointKey = ""
	binding.PendingCheckpointKey = ""
	binding.Generation++
	binding.UpdatedAt = s.now().UTC()
	if err := s.repository.UpdateBinding(ctx, binding, previousGeneration); err != nil {
		cleanupErr := s.enqueueDestroy(ctx, binding, allocation, err)
		return Binding{}, errors.Join(fmt.Errorf("record deleted binding: %w", err), cleanupErr)
	}
	return binding, nil
}

func (s *Service) mutableBinding(ctx context.Context, scope Scope, bindingID string, generation int64) (Binding, Backend, error) {
	binding, err := s.GetBinding(ctx, scope, bindingID)
	if err != nil {
		return Binding{}, nil, err
	}
	if generation != binding.Generation {
		return Binding{}, nil, ErrGenerationConflict
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return Binding{}, nil, fmt.Errorf("%w: %s", ErrBackendUnavailable, binding.Backend)
	}
	return binding, backend, nil
}

func (s *Service) captureWorkspace(ctx context.Context, binding Binding, backend Backend, allocation Allocation) error {
	active, err := s.jobs.HasActiveJobs(ctx, binding.ID)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: active jobs must finish before a workspace checkpoint", ErrInvalidTransition)
	}
	key := binding.PendingCheckpointKey
	existing, err := s.checkpoints.Open(ctx, key)
	if err == nil {
		_, verifyErr := io.Copy(io.Discard, existing)
		return errors.Join(verifyErr, existing.Close())
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	archiver, ok := backend.(WorkspaceArchiver)
	if !ok {
		return ErrBackendUnavailable
	}
	archive, err := archiver.ExportWorkspace(ctx, allocation)
	if err != nil {
		return err
	}
	saveErr := s.checkpoints.Save(ctx, key, archive)
	return errors.Join(saveErr, archive.Close())
}

func (s *Service) rebuildWorkspace(ctx context.Context, binding Binding, backend Backend, spec CreateSpec) (Allocation, error) {
	archive, err := s.checkpoints.Open(ctx, binding.CheckpointKey)
	if err != nil {
		return Allocation{}, fmt.Errorf("open workspace checkpoint: %w", err)
	}
	archiver, ok := backend.(WorkspaceArchiver)
	if !ok {
		_ = archive.Close()
		return Allocation{}, ErrBackendUnavailable
	}
	allocation, err := backend.CreateBinding(ctx, spec)
	if err != nil {
		_ = archive.Close()
		if allocation.BindingRef != "" || allocation.WorkspaceRef != "" {
			cleanupErr := s.enqueueDestroy(context.WithoutCancel(ctx), binding, allocation, err)
			return allocation, errors.Join(fmt.Errorf("rebuild runtime environment: %w", err), cleanupErr)
		}
		return allocation, fmt.Errorf("rebuild runtime environment: %w", err)
	}
	importErr := archiver.ImportWorkspace(ctx, allocation, archive)
	importErr = errors.Join(importErr, archive.Close())
	if importErr != nil {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		cleanupErr := backend.DestroyBinding(cleanupContext, allocation)
		cancel()
		if cleanupErr != nil {
			cleanupErr = errors.Join(cleanupErr, s.enqueueDestroy(context.WithoutCancel(ctx), binding, allocation, importErr))
		}
		return Allocation{}, errors.Join(fmt.Errorf("restore workspace checkpoint: %w", importErr), cleanupErr)
	}
	return allocation, nil
}

func workspaceCheckpointKey(binding Binding) string {
	value := strings.Join([]string{
		binding.OrganizationID, binding.SubjectID, binding.WorkspaceID,
		fmt.Sprint(binding.Generation),
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) enqueueDestroy(ctx context.Context, binding Binding, allocation Allocation, cause error) error {
	cleanupID, err := s.newID("arc")
	if err != nil {
		return fmt.Errorf("create cleanup id: %w", err)
	}
	now := s.now().UTC()
	err = s.repository.EnqueueCleanup(ctx, CleanupTask{
		ID:           cleanupID,
		ResourceType: "binding",
		ResourceID:   binding.ID,
		BindingID:    binding.ID,
		Backend:      binding.Backend,
		BackendRef:   allocation.BindingRef,
		WorkspaceRef: allocation.WorkspaceRef,
		Action:       "destroy",
		Status:       "pending",
		LastError:    limitFailureReason(cause.Error()),
		NotBefore:    now,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return fmt.Errorf("enqueue agent runtime cleanup: %w", err)
	}
	return nil
}

func (s *Service) scheduleAllocationCheck(ctx context.Context, binding Binding, backend Backend) (string, error) {
	if _, discoverable := backend.(AllocationDiscoverer); !discoverable {
		return "", nil
	}
	attemptID, err := s.newID("ara")
	if err != nil {
		return "", fmt.Errorf("create allocation attempt id: %w", err)
	}
	now := s.now().UTC()
	if err := s.repository.EnqueueCleanup(ctx, CleanupTask{
		ID: attemptID, ResourceType: "allocation_attempt", ResourceID: attemptID,
		BindingID: binding.ID, Backend: binding.Backend, Action: "reconcile",
		Status: "pending", NotBefore: now.Add(allocationAttemptGrace),
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return "", fmt.Errorf("record allocation attempt before provider creation: %w", err)
	}
	return attemptID, nil
}

func normalizeCreateRequest(request CreateBindingRequest) CreateBindingRequest {
	request.Scope.OrganizationID = strings.TrimSpace(request.Scope.OrganizationID)
	request.Scope.SubjectID = strings.TrimSpace(request.Scope.SubjectID)
	request.AgentID = strings.TrimSpace(request.AgentID)
	request.ConversationID = strings.TrimSpace(request.ConversationID)
	request.ProfileID = strings.TrimSpace(request.ProfileID)
	request.HomeSnapshotID = strings.TrimSpace(request.HomeSnapshotID)
	request.Backend = strings.TrimSpace(request.Backend)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	return request
}

func validateCreateRequest(request CreateBindingRequest) error {
	if request.Scope.OrganizationID == "" || request.Scope.SubjectID == "" || request.AgentID == "" || request.ConversationID == "" || request.ProfileID == "" || request.Backend == "" || request.IdempotencyKey == "" {
		return fmt.Errorf("%w: organization, subject, agent, conversation, profile, backend, and idempotency key are required", ErrInvalidRequest)
	}
	return nil
}

func requestFingerprint(request CreateBindingRequest) string {
	value := strings.Join([]string{
		request.Scope.OrganizationID,
		request.Scope.SubjectID,
		request.AgentID,
		request.ConversationID,
		request.ProfileID,
		request.HomeSnapshotID,
		request.Backend,
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func owns(scope Scope, binding Binding) bool {
	return strings.TrimSpace(scope.OrganizationID) != "" && strings.TrimSpace(scope.SubjectID) != "" &&
		strings.TrimSpace(scope.OrganizationID) == binding.OrganizationID && strings.TrimSpace(scope.SubjectID) == binding.SubjectID
}

func randomID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(value[:]), nil
}

func limitFailureReason(value string) string {
	const limit = 2048
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
