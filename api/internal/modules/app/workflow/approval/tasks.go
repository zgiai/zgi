package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	TypeApprovalResume      = "workflow:approval:resume"
	TypeApprovalTimeoutScan = "workflow:approval:timeout_scan"
)

type ResumeTaskPayload struct {
	FormID string `json:"form_id"`
}

func NewResumeTask(formID string, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if formID == "" {
		return nil, fmt.Errorf("approval form id is empty")
	}
	payload, err := json.Marshal(ResumeTaskPayload{FormID: formID})
	if err != nil {
		return nil, fmt.Errorf("marshal approval resume task payload: %w", err)
	}
	taskType := TypeApprovalResume
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return asynq.NewTask(taskType, payload,
		asynq.Queue("critical"),
		asynq.Timeout(30*time.Minute),
		asynq.MaxRetry(3),
	), nil
}

func EnqueueResumeTask(ctx context.Context, taskManager *queue.TaskManager, formID string) error {
	if taskManager == nil {
		return fmt.Errorf("task manager is not configured")
	}
	task, err := NewResumeTask(formID, taskManager)
	if err != nil {
		return err
	}
	if _, err := taskManager.EnqueueTask(task, asynq.Queue("critical")); err != nil {
		return fmt.Errorf("enqueue approval resume task: %w", err)
	}
	logger.InfoContext(ctx, "approval resume task enqueued", "form_id", formID)
	return nil
}

func NewResumeTaskHandler(service *Service, onSubmit ResumeCallback) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if service == nil {
			return fmt.Errorf("approval service is not configured: %w", asynq.SkipRetry)
		}
		var payload ResumeTaskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal approval resume task payload: %v: %w", err, asynq.SkipRetry)
		}
		if payload.FormID == "" {
			return fmt.Errorf("approval resume task payload missing form_id: %w", asynq.SkipRetry)
		}
		form, err := service.GetFormByID(ctx, payload.FormID)
		if err != nil {
			return fmt.Errorf("load approval form for resume: %w", err)
		}
		if onSubmit != nil {
			if err := onSubmit(ctx, form); err != nil {
				return err
			}
		}
		return nil
	}
}

type TimeoutScanTask struct {
	interval time.Duration
}

func NewTimeoutScanTask(interval time.Duration) *TimeoutScanTask {
	if interval <= 0 {
		interval = time.Minute
	}
	return &TimeoutScanTask{interval: interval}
}

func (t *TimeoutScanTask) TaskType() string {
	return TypeApprovalTimeoutScan
}

func (t *TimeoutScanTask) CronSpec() string {
	return ""
}

func (t *TimeoutScanTask) Interval() time.Duration {
	return t.interval
}

func (t *TimeoutScanTask) Payload() []byte {
	return nil
}

func (t *TimeoutScanTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(1),
		asynq.Timeout(5 * time.Minute),
		asynq.Unique(55 * time.Second),
	}
}

type TimeoutScanHandler struct {
	service   *Service
	onTimeout ResumeCallback
	batchSize int
}

func NewTimeoutScanHandler(service *Service, onTimeout ResumeCallback, batchSize int) *TimeoutScanHandler {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &TimeoutScanHandler{
		service:   service,
		onTimeout: onTimeout,
		batchSize: batchSize,
	}
}

func (h *TimeoutScanHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.service == nil {
		return nil
	}
	forms, err := h.service.TimeoutExpiredForms(ctx, h.batchSize)
	if err != nil {
		return err
	}
	for _, form := range forms {
		if h.onTimeout != nil {
			if err := h.onTimeout(ctx, form); err != nil {
				return err
			}
		}
	}
	if len(forms) > 0 {
		logger.InfoContext(ctx, "approval timeout scan completed", "timed_out_count", len(forms))
	}
	return nil
}
