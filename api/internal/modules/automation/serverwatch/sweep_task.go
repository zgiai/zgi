package serverwatch

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const TaskTypeSweepServerWatches = "automation:server-watch:sweep"

// sweepLimit bounds the watches one sweep checks.
const sweepLimit = 50

// SweepTask checks due server watches every minute.
type SweepTask struct{}

func (*SweepTask) TaskType() string        { return TaskTypeSweepServerWatches }
func (*SweepTask) CronSpec() string        { return "" }
func (*SweepTask) Interval() time.Duration { return time.Minute }
func (*SweepTask) Payload() []byte         { return nil }
func (*SweepTask) Options() []asynq.Option {
	return []asynq.Option{asynq.Queue("scheduler"), asynq.MaxRetry(0), asynq.Timeout(5 * time.Minute), asynq.Unique(50 * time.Second)}
}

// SweepHandler runs the sweep.
type SweepHandler struct{ service *Service }

func NewSweepHandler(service *Service) *SweepHandler { return &SweepHandler{service: service} }

func (h *SweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.service == nil {
		return nil
	}
	_, err := h.service.Sweep(ctx, sweepLimit)
	return err
}
