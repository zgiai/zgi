package music

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	ReconcileTaskType     = "music:reconcile_tasks"
	generationRecoveryAge = generationTaskTimeout + time.Minute
)

type ReconcileTask struct{}

func (*ReconcileTask) TaskType() string        { return ReconcileTaskType }
func (*ReconcileTask) CronSpec() string        { return "" }
func (*ReconcileTask) Interval() time.Duration { return time.Minute }
func (*ReconcileTask) Payload() []byte         { return nil }
func (*ReconcileTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Timeout(time.Minute),
		asynq.Unique(45 * time.Second),
	}
}

type ReconcileHandler struct {
	repo       Repository
	dispatcher Dispatcher
}

func NewReconcileHandler(repo Repository, dispatcher Dispatcher) *ReconcileHandler {
	if repo == nil || dispatcher == nil {
		panic("music reconciler requires repository and dispatcher")
	}
	return &ReconcileHandler{repo: repo, dispatcher: dispatcher}
}

func (h *ReconcileHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	now := time.Now().UTC()
	return errors.Join(
		h.requeue(ctx, StatusQueued, now, h.dispatcher.EnqueueGeneration),
		h.requeue(ctx, StatusGeneratingLyrics, now.Add(-generationRecoveryAge), h.dispatcher.EnqueueGeneration),
		h.requeue(ctx, StatusCompensationPending, now, h.dispatcher.EnqueueCompensation),
		h.recoverStaleGeneration(ctx, now.Add(-generationRecoveryAge)),
	)
}

func (h *ReconcileHandler) requeue(
	ctx context.Context,
	status Status,
	updatedBefore time.Time,
	enqueue func(context.Context, uuid.UUID) error,
) error {
	ids, err := h.repo.ListIDsByStatus(ctx, status, updatedBefore, 100)
	if err != nil {
		return err
	}
	var reconcileErrors []error
	for _, id := range ids {
		if err := enqueue(ctx, id); err != nil {
			reconcileErrors = append(reconcileErrors, err)
			continue
		}
		if err := h.repo.TouchStatus(ctx, id, status); err != nil && !errors.Is(err, ErrInvalidTransition) {
			reconcileErrors = append(reconcileErrors, err)
		}
	}
	return errors.Join(reconcileErrors...)
}

func (h *ReconcileHandler) recoverStaleGeneration(ctx context.Context, updatedBefore time.Time) error {
	ids, err := h.repo.ListIDsByStatus(ctx, StatusGenerating, updatedBefore, 100)
	if err != nil {
		return err
	}
	var reconcileErrors []error
	for _, id := range ids {
		if err := h.repo.Transition(ctx, id, StatusGenerating, StatusCompensationPending, TaskUpdate{
			ErrorCode:    ErrorCodeDeliveryUnknown,
			ErrorMessage: messageTaskInterrupted,
		}); err != nil {
			if !errors.Is(err, ErrInvalidTransition) {
				reconcileErrors = append(reconcileErrors, err)
			}
			continue
		}
		if err := h.dispatcher.EnqueueCompensation(ctx, id); err != nil {
			reconcileErrors = append(reconcileErrors, err)
		}
	}
	return errors.Join(reconcileErrors...)
}
