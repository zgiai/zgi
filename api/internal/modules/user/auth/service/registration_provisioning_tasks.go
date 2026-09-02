package service

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

const registrationProvisioningOutboxTaskType = "auth:registration_provisioning_outbox"

type RegistrationProvisioningOutboxTask struct{}

var _ pkgscheduler.ScheduledTask = (*RegistrationProvisioningOutboxTask)(nil)

func (*RegistrationProvisioningOutboxTask) TaskType() string {
	return registrationProvisioningOutboxTaskType
}
func (*RegistrationProvisioningOutboxTask) CronSpec() string        { return "" }
func (*RegistrationProvisioningOutboxTask) Interval() time.Duration { return 3 * time.Second }
func (*RegistrationProvisioningOutboxTask) Payload() []byte         { return nil }
func (*RegistrationProvisioningOutboxTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Minute),
		asynq.Unique(2 * time.Second),
	}
}

type RegistrationProvisioningOutboxHandler struct {
	processor *RegistrationProvisioningOutboxProcessor
	batchSize int
}

var _ pkgscheduler.TaskHandler = (*RegistrationProvisioningOutboxHandler)(nil)

func NewRegistrationProvisioningOutboxHandler(processor *RegistrationProvisioningOutboxProcessor, batchSize int) *RegistrationProvisioningOutboxHandler {
	if batchSize <= 0 {
		batchSize = 25
	}
	return &RegistrationProvisioningOutboxHandler{processor: processor, batchSize: batchSize}
}

func (h *RegistrationProvisioningOutboxHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.processor == nil {
		return nil
	}
	_, err := h.processor.ProcessPending(ctx, h.batchSize)
	return err
}
