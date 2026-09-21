package digitalhuman

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const ChannelDeliverySweepTaskType = "digital_human:channel_delivery_sweep"

// ChannelDeliverySweepTask schedules bounded, lease-protected delivery work.
type ChannelDeliverySweepTask struct{}

func (*ChannelDeliverySweepTask) TaskType() string        { return ChannelDeliverySweepTaskType }
func (*ChannelDeliverySweepTask) CronSpec() string        { return "" }
func (*ChannelDeliverySweepTask) Interval() time.Duration { return 5 * time.Second }
func (*ChannelDeliverySweepTask) Payload() []byte         { return nil }
func (*ChannelDeliverySweepTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(time.Minute), asynq.Unique(4 * time.Second)}
}

type ChannelDeliverySweepHandler struct{ service *ChannelService }

func NewChannelDeliverySweepHandler(service *ChannelService) *ChannelDeliverySweepHandler {
	if service == nil {
		panic("channel delivery sweep handler requires service")
	}
	return &ChannelDeliverySweepHandler{service: service}
}

func (handler *ChannelDeliverySweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	return handler.service.ReconcileDeliveries(ctx)
}
