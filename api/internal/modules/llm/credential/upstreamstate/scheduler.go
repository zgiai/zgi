package upstreamstate

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

const pollingTaskType = "llm:upstream_credential_poll"

type PollingTask struct{}

var _ pkgscheduler.ScheduledTask = (*PollingTask)(nil)

func NewPollingTask() *PollingTask { return &PollingTask{} }

func (*PollingTask) TaskType() string        { return pollingTaskType }
func (*PollingTask) CronSpec() string        { return "" }
func (*PollingTask) Interval() time.Duration { return time.Minute }
func (*PollingTask) Payload() []byte         { return nil }
func (*PollingTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(0),
		asynq.Timeout(20 * time.Minute),
		asynq.Unique(50 * time.Second),
	}
}

type PollingHandler struct {
	service *Service
}

var _ pkgscheduler.TaskHandler = (*PollingHandler)(nil)

func NewPollingHandler(service *Service) *PollingHandler {
	return &PollingHandler{service: service}
}

func (h *PollingHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	checked, err := h.service.RunDueChecks(ctx)
	if err != nil {
		return err
	}
	logger.DebugContext(ctx, "LLM upstream credential polling completed", "checked", checked)
	return nil
}
