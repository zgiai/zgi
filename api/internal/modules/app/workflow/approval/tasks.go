package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	TypeApprovalResume      = "workflow:approval:resume"
	TypeQuestionResume      = "workflow:question:resume"
	TypeApprovalTimeoutScan = "workflow:approval:timeout_scan"
	TypeRuntimeOutboxScan   = "workflow:runtime:outbox_scan"
	TypeExecutionLeaseScan  = "workflow:runtime:execution_lease_scan"
)

type ResumeTaskPayload struct {
	FormID string `json:"form_id"`
}

type QuestionResumeTaskPayload struct {
	WorkflowRunID string                 `json:"workflow_run_id"`
	Inputs        map[string]interface{} `json:"inputs"`
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
		asynq.MaxRetry(10),
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

func NewQuestionResumeTask(payload QuestionResumeTaskPayload, taskManager *queue.TaskManager) (*asynq.Task, error) {
	if payload.WorkflowRunID == "" {
		return nil, fmt.Errorf("workflow run id is empty")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal question resume task payload: %w", err)
	}
	taskType := TypeQuestionResume
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}
	return asynq.NewTask(taskType, raw,
		asynq.Queue("critical"),
		asynq.Timeout(30*time.Minute),
		asynq.MaxRetry(10),
	), nil
}

func DispatchResumeOutbox(ctx context.Context, service *Service, taskManager *queue.TaskManager, ref *workflowRuntimeOutboxRef) error {
	if service == nil || taskManager == nil || ref == nil {
		return fmt.Errorf("workflow resume outbox dispatcher is not configured")
	}
	var payload workflowpause.RuntimeOutboxPayload
	if err := json.Unmarshal([]byte(ref.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode workflow resume outbox: %w", err)
	}
	var task *asynq.Task
	var err error
	if payload.InteractionType == workflowpause.ReasonTypeQuestionAnswerRequired {
		task, err = NewQuestionResumeTask(QuestionResumeTaskPayload{
			WorkflowRunID: payload.WorkflowRunID,
			Inputs:        questionResumeInputs(payload.ResumeInputs),
		}, taskManager)
	} else {
		task, err = NewResumeTask(payload.TriggerID, taskManager)
	}
	if err != nil {
		return err
	}
	task = asynq.NewTask(task.Type(), task.Payload(),
		asynq.Queue("critical"),
		asynq.Timeout(30*time.Minute),
		asynq.MaxRetry(10),
		asynq.TaskID(ref.IdempotencyKey),
	)
	if _, err := taskManager.EnqueueTask(task, asynq.Queue("critical")); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		_ = service.markRuntimeOutboxRetry(ctx, ref.ID, err)
		return fmt.Errorf("enqueue workflow resume outbox: %w", err)
	}
	if err := service.markRuntimeOutboxPublished(ctx, ref.ID); err != nil {
		return err
	}
	return nil
}

func questionResumeInputs(eventData map[string]interface{}) map[string]interface{} {
	inputs := make(map[string]interface{}, len(eventData)+3)
	for key, value := range eventData {
		inputs[key] = value
	}
	if answer, ok := eventData["answer"].(string); ok && answer != "" {
		inputs["query"] = answer
		inputs["sys.query"] = answer
	}
	if choiceID, ok := eventData["choice_id"].(string); ok && choiceID != "" {
		inputs["question_answer_option_id"] = choiceID
	}
	return inputs
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
				if errors.Is(err, workflowpause.ErrPauseNotFound) {
					return nil
				}
				return err
			}
		}
		return nil
	}
}

type QuestionResumeCallback func(context.Context, string, map[string]interface{}) error

func NewQuestionResumeTaskHandler(onResume QuestionResumeCallback) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if onResume == nil {
			return fmt.Errorf("question resume callback is not configured: %w", asynq.SkipRetry)
		}
		var payload QuestionResumeTaskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal question resume task payload: %v: %w", err, asynq.SkipRetry)
		}
		if payload.WorkflowRunID == "" {
			return fmt.Errorf("question resume task payload missing workflow_run_id: %w", asynq.SkipRetry)
		}
		if err := onResume(ctx, payload.WorkflowRunID, payload.Inputs); err != nil {
			if errors.Is(err, workflowpause.ErrPauseNotFound) {
				return nil
			}
			return err
		}
		return nil
	}
}

type TimeoutScanTask struct {
	interval time.Duration
}

type RuntimeOutboxScanTask struct {
	interval time.Duration
}

type ExecutionLeaseScanTask struct {
	interval time.Duration
}

func NewExecutionLeaseScanTask(interval time.Duration) *ExecutionLeaseScanTask {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &ExecutionLeaseScanTask{interval: interval}
}

func (t *ExecutionLeaseScanTask) TaskType() string        { return TypeExecutionLeaseScan }
func (t *ExecutionLeaseScanTask) CronSpec() string        { return "" }
func (t *ExecutionLeaseScanTask) Interval() time.Duration { return t.interval }
func (t *ExecutionLeaseScanTask) Payload() []byte         { return nil }
func (t *ExecutionLeaseScanTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(1),
		asynq.Timeout(time.Minute),
		asynq.Unique(25 * time.Second),
	}
}

type ExecutionLeaseScanHandler struct {
	service   *Service
	batchSize int
}

func NewExecutionLeaseScanHandler(service *Service, batchSize int) *ExecutionLeaseScanHandler {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &ExecutionLeaseScanHandler{service: service, batchSize: batchSize}
}

func (h *ExecutionLeaseScanHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.service == nil {
		return nil
	}
	finalized, err := h.service.finalizeExpiredWorkflowExecutions(ctx, time.Now().Add(-15*time.Second), h.batchSize)
	if err != nil {
		return err
	}
	for _, workflowRunID := range finalized {
		logger.WarnContext(ctx, "orphaned workflow execution finalized", "workflow_run_id", workflowRunID, "error_code", "workflow_execution_interrupted")
	}
	if err := h.service.observeActiveV1WorkflowRuns(ctx); err != nil {
		logger.WarnContext(ctx, "failed to observe active V1 workflow runs", err)
	}
	return nil
}

func NewRuntimeOutboxScanTask(interval time.Duration) *RuntimeOutboxScanTask {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &RuntimeOutboxScanTask{interval: interval}
}

func (t *RuntimeOutboxScanTask) TaskType() string        { return TypeRuntimeOutboxScan }
func (t *RuntimeOutboxScanTask) CronSpec() string        { return "" }
func (t *RuntimeOutboxScanTask) Interval() time.Duration { return t.interval }
func (t *RuntimeOutboxScanTask) Payload() []byte         { return nil }
func (t *RuntimeOutboxScanTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.Queue("scheduler"),
		asynq.MaxRetry(1),
		asynq.Timeout(time.Minute),
		asynq.Unique(9 * time.Second),
	}
}

type RuntimeOutboxScanHandler struct {
	service     *Service
	taskManager *queue.TaskManager
	batchSize   int
}

func NewRuntimeOutboxScanHandler(service *Service, taskManager *queue.TaskManager, batchSize int) *RuntimeOutboxScanHandler {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &RuntimeOutboxScanHandler{service: service, taskManager: taskManager, batchSize: batchSize}
}

func (h *RuntimeOutboxScanHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.service == nil || h.taskManager == nil {
		return nil
	}
	items, err := h.service.pendingRuntimeOutbox(ctx, h.batchSize)
	if err != nil {
		return err
	}
	for _, item := range items {
		ref := &workflowRuntimeOutboxRef{ID: item.ID, IdempotencyKey: item.IdempotencyKey, PayloadJSON: item.PayloadJSON}
		if err := DispatchResumeOutbox(ctx, h.service, h.taskManager, ref); err != nil {
			logger.WarnContext(ctx, "workflow runtime outbox dispatch failed", "outbox_id", item.ID, err)
		}
	}
	return nil
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
