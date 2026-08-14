package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"gorm.io/gorm"
)

const (
	defaultOutboxBatchSize = 100
	outboxLeaseDuration    = time.Minute
	outboxRetryDelay       = 5 * time.Second
	datasetPurgeRetryDelay = 5 * time.Minute
	maxOutboxAttempts      = 20
)

type OutboxEventProcessor interface {
	Process(context.Context, *model.GraphOutboxEvent) error
}

type OutboxTerminalFailureHandler interface {
	HandleTerminalFailure(context.Context, *model.GraphOutboxEvent, error) error
}

type OutboxReconciler struct {
	repository *repository.GraphOutboxRepository
	run        OutboxEventProcessor
	visibility OutboxEventProcessor
	purge      OutboxEventProcessor
	now        func() time.Time
}

func NewOutboxReconciler(
	db *gorm.DB,
	run OutboxEventProcessor,
	visibility OutboxEventProcessor,
	purge OutboxEventProcessor,
) *OutboxReconciler {
	return &OutboxReconciler{
		repository: repository.NewGraphOutboxRepository(db),
		run:        run,
		visibility: visibility,
		purge:      purge,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (r *OutboxReconciler) RunOnce(ctx context.Context) error {
	if r == nil || r.repository == nil {
		return fmt.Errorf("graph outbox reconciler is not configured")
	}
	now := r.now()
	if _, err := r.repository.RequeueStale(ctx, now.Add(-outboxLeaseDuration), now); err != nil {
		return err
	}
	events, err := r.repository.ClaimBatch(ctx, defaultOutboxBatchSize, now)
	if err != nil {
		return err
	}
	for _, event := range events {
		processor := r.processor(event.EventType)
		if processor == nil {
			err = fmt.Errorf("no processor is registered for graph outbox event type %q", event.EventType)
		} else {
			err = processor.Process(ctx, event)
		}
		if err != nil {
			message := err.Error()
			if event.AttemptCount >= maxOutboxAttempts {
				if event.EventType == model.GraphOutboxEventDatasetPurge {
					if retryErr := r.repository.Retry(ctx, event.ID, now.Add(datasetPurgeRetryDelay), message); retryErr != nil {
						return retryErr
					}
					continue
				}
				if handler, ok := processor.(OutboxTerminalFailureHandler); ok {
					if handleErr := handler.HandleTerminalFailure(ctx, event, err); handleErr != nil {
						return handleErr
					}
				}
				if failErr := r.repository.MarkFailed(ctx, event.ID, message); failErr != nil {
					return failErr
				}
				continue
			}
			if retryErr := r.repository.Retry(ctx, event.ID, now.Add(outboxRetryDelay), message); retryErr != nil {
				return retryErr
			}
			continue
		}
		if err := r.repository.Confirm(ctx, event.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *OutboxReconciler) processor(eventType string) OutboxEventProcessor {
	switch eventType {
	case model.GraphOutboxEventRun:
		return r.run
	case model.GraphOutboxEventVisibility:
		return r.visibility
	case model.GraphOutboxEventDatasetPurge:
		return r.purge
	default:
		return nil
	}
}
