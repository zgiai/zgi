package agentmemoryworker

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const ExtractionSweepTaskType = "system:agent_memory:extraction_sweep"

type ExtractionSweepTask struct{}

func NewExtractionSweepTask() *ExtractionSweepTask   { return &ExtractionSweepTask{} }
func (*ExtractionSweepTask) TaskType() string        { return ExtractionSweepTaskType }
func (*ExtractionSweepTask) CronSpec() string        { return "" }
func (*ExtractionSweepTask) Interval() time.Duration { return time.Minute }
func (*ExtractionSweepTask) Payload() []byte         { return nil }
func (*ExtractionSweepTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(50 * time.Second), asynq.Unique(45 * time.Second)}
}

type ExtractionSweepHandler struct{ runner *Runner }

func NewExtractionSweepHandler(runner *Runner) *ExtractionSweepHandler {
	return &ExtractionSweepHandler{runner: runner}
}
func (h *ExtractionSweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.runner == nil {
		return nil
	}
	return h.runner.ProcessDue(ctx, 100)
}
