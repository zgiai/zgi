package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// answeredWaitingRunStore lists runs still waiting although their message
// has ended, which happened when a question answer did not settle its run.
type answeredWaitingRunStore interface {
	ListAnsweredWaitingRuns(ctx context.Context, updatedBefore time.Time, limit int) ([]*Run, error)
}

// answeredWaitingSurfaces are settled here. Task and channel runs settle
// through their own paths.
var answeredWaitingSurfaces = []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI}

func (r *GormRepository) ListAnsweredWaitingRuns(ctx context.Context, updatedBefore time.Time, limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = defaultOutboxBatch
	}
	items := make([]*Run, 0, limit)
	ended := []string{runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusStopped, runtimemodel.MessageStatusError}
	if err := r.db.WithContext(ctx).
		Where("status = ? AND message_id IS NOT NULL AND updated_at < ? AND surface IN ?", RunStatusWaiting, updatedBefore, answeredWaitingSurfaces).
		Where("EXISTS (?)", r.db.Model(&runtimemodel.Message{}).Select("1").
			Where("chat_runtime_messages.id = digital_human_runs.message_id AND chat_runtime_messages.status IN ?", ended)).
		Order("updated_at ASC, id ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list answered waiting DigitalHuman runs: %w", err)
	}
	return items, nil
}

// settleAnsweredWaitingRuns settles runs whose conversation moved on while
// the run still waited, from the message that ended it.
func (s *RuntimeService) settleAnsweredWaitingRuns(ctx context.Context, updatedBefore time.Time, limit int) error {
	store, ok := s.store.(answeredWaitingRunStore)
	if !ok {
		return nil
	}
	runs, err := store.ListAnsweredWaitingRuns(ctx, updatedBefore, limit)
	if err != nil {
		return err
	}
	var combined error
	for _, run := range runs {
		message, _, loadErr := s.chat.GetMessageByCaller(ctx, runtimeScope(run), runConversationCaller(run), *run.MessageID)
		if loadErr != nil {
			combined = errors.Join(combined, loadErr)
			continue
		}
		if message == nil {
			continue
		}
		if err := s.resumeHeldWorkflowRun(ctx, run); err != nil {
			combined = errors.Join(combined, err)
			continue
		}
		s.settleWorkflowContinuationRun(ctx, run, message.Status, message.Answer, message.Metadata)
	}
	return combined
}
