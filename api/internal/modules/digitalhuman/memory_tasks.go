package digitalhuman

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const MemoryExtractionSweepTaskType = "system:digital_human:memory_extraction_sweep"

// MemoryExtractionSweepTask periodically claims due reference-only jobs.
type MemoryExtractionSweepTask struct{}

func (*MemoryExtractionSweepTask) TaskType() string        { return MemoryExtractionSweepTaskType }
func (*MemoryExtractionSweepTask) CronSpec() string        { return "" }
func (*MemoryExtractionSweepTask) Interval() time.Duration { return time.Minute }
func (*MemoryExtractionSweepTask) Payload() []byte         { return nil }
func (*MemoryExtractionSweepTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(50 * time.Second), asynq.Unique(45 * time.Second)}
}

// MemoryExtractionSweepHandler runs one bounded extraction sweep.
type MemoryExtractionSweepHandler struct {
	runner *MemoryExtractionRunner
}

func NewMemoryExtractionSweepHandler(runner *MemoryExtractionRunner) *MemoryExtractionSweepHandler {
	if runner == nil {
		panic("DigitalHuman memory extraction handler requires runner")
	}
	return &MemoryExtractionSweepHandler{runner: runner}
}

func (h *MemoryExtractionSweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	return h.runner.ProcessDue(ctx, defaultMemoryExtractionBatch)
}
