package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	runtimeInputFollowUpTaskType = "digital_human:runtime_input_follow_up"
	runtimeInputRecoveryTaskType = "digital_human:runtime_input_recovery"
)

type runtimeInputFollowUpTaskPayload struct {
	BatchID string `json:"batch_id"`
}

type RuntimeInputFollowUpDispatcher struct{ tasks *queue.TaskManager }

func (*RuntimeInputFollowUpDispatcher) OwnsCallerType(callerType string) bool {
	return callerType == runtimemodel.ConversationCallerDigitalHuman
}

func NewRuntimeInputFollowUpDispatcher(tasks *queue.TaskManager) *RuntimeInputFollowUpDispatcher {
	if tasks == nil {
		return nil
	}
	return &RuntimeInputFollowUpDispatcher{tasks: tasks}
}

func (d *RuntimeInputFollowUpDispatcher) EnqueueRuntimeInputFollowUp(_ context.Context, batchID uuid.UUID) error {
	if d == nil || d.tasks == nil || batchID == uuid.Nil {
		return chatruntime.ErrInvalidInput
	}
	payload, err := json.Marshal(runtimeInputFollowUpTaskPayload{BatchID: batchID.String()})
	if err != nil {
		return fmt.Errorf("encode DigitalHuman runtime input follow-up task: %w", err)
	}
	taskID := "digital-human-runtime-input-follow-up-" + batchID.String()
	newTask := func() *asynq.Task {
		return asynq.NewTask(
			d.tasks.GetTaskTypeWithPrefix(runtimeInputFollowUpTaskType), payload,
			asynq.Queue(runQueueName), asynq.MaxRetry(5), asynq.Timeout(runTaskTimeout), asynq.TaskID(taskID),
		)
	}
	if _, err := d.tasks.EnqueueTask(newTask()); err != nil {
		if !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err
		}
		reset, resetErr := d.tasks.ResetArchivedTask(runQueueName, taskID)
		if resetErr != nil || !reset {
			return resetErr
		}
		_, err = d.tasks.EnqueueTask(newTask())
		return err
	}
	return nil
}

func RegisterRuntimeInputFollowUpTaskHandler(registry RunTaskRegistry, prefixer interface{ GetTaskTypeWithPrefix(string) string }, runtime *RuntimeService) {
	if registry == nil || prefixer == nil || runtime == nil {
		panic("DigitalHuman runtime input follow-up task requires registry, prefixer, and runtime")
	}
	taskType := prefixer.GetTaskTypeWithPrefix(runtimeInputFollowUpTaskType)
	if !registry.Register(taskType, func(ctx context.Context, task *asynq.Task) error {
		var payload runtimeInputFollowUpTaskPayload
		if task == nil || json.Unmarshal(task.Payload(), &payload) != nil {
			return fmt.Errorf("invalid DigitalHuman runtime input follow-up task: %w", asynq.SkipRetry)
		}
		batchID, err := uuid.Parse(payload.BatchID)
		if err != nil || batchID == uuid.Nil {
			return fmt.Errorf("invalid DigitalHuman runtime input follow-up batch: %w", asynq.SkipRetry)
		}
		_, _, err = runtime.SubmitRuntimeInputFollowUp(ctx, batchID)
		if err != nil && runtimeInputFollowUpAttemptExhausted(ctx) {
			if followUpRuntime, ok := runtime.chat.(runtimeInputChatRuntime); ok {
				_ = followUpRuntime.FailRuntimeInputFollowUp(context.WithoutCancel(ctx), batchID, err)
			}
		}
		return err
	}) {
		panic("DigitalHuman runtime input follow-up task is already registered")
	}
}

func runtimeInputFollowUpAttemptExhausted(ctx context.Context) bool {
	retryCount, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxOK := asynq.GetMaxRetry(ctx)
	return retryOK && maxOK && retryCount >= maxRetry
}

type RuntimeInputRecoveryTask struct{}

func (*RuntimeInputRecoveryTask) TaskType() string        { return runtimeInputRecoveryTaskType }
func (*RuntimeInputRecoveryTask) CronSpec() string        { return "" }
func (*RuntimeInputRecoveryTask) Interval() time.Duration { return 30 * time.Second }
func (*RuntimeInputRecoveryTask) Payload() []byte         { return nil }
func (*RuntimeInputRecoveryTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(time.Minute), asynq.Unique(20 * time.Second)}
}

type RuntimeInputRecoveryHandler struct{ runtime chatruntime.Service }

func NewRuntimeInputRecoveryHandler(runtime chatruntime.Service) *RuntimeInputRecoveryHandler {
	return &RuntimeInputRecoveryHandler{runtime: runtime}
}

func (h *RuntimeInputRecoveryHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.runtime == nil {
		return nil
	}
	if _, err := h.runtime.RecoverStaleRuntimeInputs(ctx); err != nil {
		return err
	}
	_, err := h.runtime.EnqueuePendingRuntimeInputFollowUps(ctx)
	return err
}
