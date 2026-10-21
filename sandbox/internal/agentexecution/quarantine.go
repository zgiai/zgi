package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const quarantineFailureReason = "job termination failed; compute quarantined, workspace retained"

// QuarantineJob fences further execution and pauses unsafe compute. It never
// deletes a workspace, checkpoint, or provider disk. Failed pauses are retried
// by the existing owned cleanup queue.
func (s *Service) QuarantineJob(ctx context.Context, job Job) error {
	binding, err := s.repository.Binding(ctx, job.BindingID)
	if err != nil {
		return err
	}
	if binding.BackendRef != job.BackendRef || binding.Generation != job.BindingGeneration {
		return ErrGenerationConflict
	}
	backend, ok := s.backends[binding.Backend]
	if !ok {
		return ErrBackendUnavailable
	}
	previous := binding.Generation
	binding.Status = BindingStatusFailed
	binding.FailureReason = quarantineFailureReason
	binding.Generation++
	binding.UpdatedAt = s.now().UTC()
	if err := s.repository.UpdateBinding(ctx, binding, previous); err != nil {
		return err
	}
	if err := backend.SuspendBinding(ctx, allocationForBinding(binding)); err != nil {
		id, idErr := s.newID("arc")
		if idErr != nil {
			return errors.Join(err, idErr)
		}
		now := s.now().UTC()
		queueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		queueErr := s.repository.EnqueueCleanup(queueCtx, CleanupTask{
			ID: id, ResourceType: "binding", ResourceID: binding.ID, BindingID: binding.ID,
			BindingGeneration: binding.Generation, Backend: binding.Backend, BackendRef: binding.BackendRef,
			WorkspaceRef: binding.WorkspaceRef, Action: "quarantine", Status: "pending",
			LastError: limitFailureReason(err.Error()), NotBefore: now, CreatedAt: now, UpdatedAt: now,
		})
		return errors.Join(fmt.Errorf("pause quarantined compute: %w", err), queueErr)
	}
	return nil
}
