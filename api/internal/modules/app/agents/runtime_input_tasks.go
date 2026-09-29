package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	runtimeInputFollowUpTaskType    = "agent:runtime_input_follow_up"
	runtimeInputFollowUpQueue       = "default"
	runtimeInputFollowUpTaskTimeout = 8 * time.Minute
	runtimeInputRecoveryTaskType    = "agent:runtime_input_recovery"
)

type runtimeInputFollowUpTaskPayload struct {
	BatchID string `json:"batch_id"`
}

type RuntimeInputFollowUpDispatcher struct {
	tasks *queue.TaskManager
}

func (*RuntimeInputFollowUpDispatcher) OwnsCallerType(callerType string) bool {
	return callerType == runtimemodel.ConversationCallerAgent
}

func NewRuntimeInputFollowUpDispatcher(tasks *queue.TaskManager) *RuntimeInputFollowUpDispatcher {
	if tasks == nil {
		return nil
	}
	return &RuntimeInputFollowUpDispatcher{tasks: tasks}
}

func (d *RuntimeInputFollowUpDispatcher) EnqueueRuntimeInputFollowUp(_ context.Context, batchID uuid.UUID) error {
	if d == nil || d.tasks == nil || batchID == uuid.Nil {
		return runtimeservice.ErrInvalidInput
	}
	payload, err := json.Marshal(runtimeInputFollowUpTaskPayload{BatchID: batchID.String()})
	if err != nil {
		return fmt.Errorf("encode Agent runtime input follow-up task: %w", err)
	}
	taskID := runtimeInputFollowUpTaskID(batchID)
	newTask := func() *asynq.Task {
		return asynq.NewTask(
			d.tasks.GetTaskTypeWithPrefix(runtimeInputFollowUpTaskType), payload,
			asynq.Queue(runtimeInputFollowUpQueue), asynq.MaxRetry(5),
			asynq.Timeout(runtimeInputFollowUpTaskTimeout), asynq.TaskID(taskID),
		)
	}
	if _, err := d.tasks.EnqueueTask(newTask()); err != nil {
		if !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err
		}
		reset, resetErr := d.tasks.ResetArchivedTask(runtimeInputFollowUpQueue, taskID)
		if resetErr != nil || !reset {
			return resetErr
		}
		_, err = d.tasks.EnqueueTask(newTask())
		return err
	}
	return nil
}

func runtimeInputFollowUpTaskID(batchID uuid.UUID) string {
	return "agent-runtime-input-follow-up-" + batchID.String()
}

func RegisterRuntimeInputFollowUpTaskHandler(registry interface {
	Register(string, func(context.Context, *asynq.Task) error) bool
}, prefixer interface{ GetTaskTypeWithPrefix(string) string }, handler *AgentsHandler) {
	if registry == nil || prefixer == nil || handler == nil {
		return
	}
	taskType := prefixer.GetTaskTypeWithPrefix(runtimeInputFollowUpTaskType)
	registry.Register(taskType, func(ctx context.Context, task *asynq.Task) error {
		var payload runtimeInputFollowUpTaskPayload
		if task == nil || json.Unmarshal(task.Payload(), &payload) != nil {
			return fmt.Errorf("invalid Agent runtime input follow-up task: %w", asynq.SkipRetry)
		}
		batchID, err := uuid.Parse(payload.BatchID)
		if err != nil || batchID == uuid.Nil {
			return fmt.Errorf("invalid Agent runtime input follow-up batch: %w", asynq.SkipRetry)
		}
		err = handler.ExecuteRuntimeInputFollowUp(ctx, batchID)
		if err != nil && runtimeInputFollowUpAttemptExhausted(ctx) {
			_ = handler.chatRuntimeService.FailRuntimeInputFollowUp(context.WithoutCancel(ctx), batchID, err)
		}
		return err
	})
}

func runtimeInputFollowUpAttemptExhausted(ctx context.Context) bool {
	retryCount, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxOK := asynq.GetMaxRetry(ctx)
	return retryOK && maxOK && retryCount >= maxRetry
}

func (h *AgentsHandler) ExecuteRuntimeInputFollowUp(ctx context.Context, batchID uuid.UUID) error {
	info, err := h.chatRuntimeService.GetRuntimeInputFollowUpContext(ctx, batchID)
	if err != nil {
		if errors.Is(err, runtimeservice.ErrNotFound) {
			return nil
		}
		return err
	}
	runConfig, err := h.runtimeInputFollowUpRunConfig(ctx, info)
	if err != nil {
		return err
	}
	prepared, err := h.chatRuntimeService.PrepareRuntimeInputFollowUp(ctx, runConfig, batchID)
	if err != nil || prepared == nil {
		return err
	}
	_, err = h.chatRuntimeService.RunPreparedStream(ctx, prepared, nil)
	if err != nil {
		_ = h.chatRuntimeService.FailRuntimeInputFollowUp(context.WithoutCancel(ctx), batchID, err)
	}
	return err
}

func (h *AgentsHandler) runtimeInputFollowUpRunConfig(ctx context.Context, info *runtimeservice.RuntimeInputFollowUpContext) (runtimeservice.RunConfig, error) {
	if info == nil || info.CallerType != runtimemodel.ConversationCallerAgent || info.CallerID == uuid.Nil || info.WorkspaceID == nil {
		return runtimeservice.RunConfig{}, runtimeservice.ErrInvalidInput
	}
	scope := runtimeservice.Scope{
		OrganizationID: info.OrganizationID, WorkspaceID: info.WorkspaceID, AccountID: info.AccountID,
		SkipAccessCheck: info.Source == runtimemodel.ConversationSourceWebApp,
	}
	requestCtx := context.WithValue(ctx, "account_id", info.AccountID.String())
	requestCtx = context.WithValue(requestCtx, "tenant_id", info.OrganizationID.String())
	var runConfig runtimeservice.RunConfig
	var err error
	switch info.Source {
	case runtimemodel.ConversationSourceConsole:
		draft, loadErr := h.appService.GetAgentDraftRuntimeConfig(requestCtx, info.CallerID.String(), info.AccountID.String())
		if loadErr != nil {
			return runtimeservice.RunConfig{}, loadErr
		}
		runConfig, err = h.agentRunConfig(requestCtx, scope, info.CallerID.String(), "agent.draft", draft.Config, agentmemory.UserScopeAccount)
		if err == nil {
			err = h.attachConsoleAgentExecutionTools(&runConfig, scope, info.CallerID, draft.Config)
		}
	case runtimemodel.ConversationSourceWebApp:
		if info.SourceWebAppID == nil || *info.SourceWebAppID == uuid.Nil {
			return runtimeservice.RunConfig{}, runtimeservice.ErrInvalidInput
		}
		published, loadErr := h.appService.GetPublishedAgentWebAppConfig(requestCtx, info.SourceWebAppID.String())
		if loadErr != nil {
			return runtimeservice.RunConfig{}, loadErr
		}
		// Memory-enabled WebApps require authentication, so follow-up runs use
		// the same account subject as the original WebApp conversation.
		runConfig, err = h.agentRunConfig(requestCtx, scope, info.CallerID.String(), "agent.published."+published.Version, published.Config, agentmemory.UserScopeAccount)
	default:
		return runtimeservice.RunConfig{}, runtimeservice.ErrRuntimeInputDisabled
	}
	if err != nil {
		return runtimeservice.RunConfig{}, err
	}
	runConfig.RuntimeInputPolicy = runtimeservice.RuntimeInputPolicyCheckpointAndFollowUp
	return runConfig, nil
}

type RuntimeInputRecoveryTask struct{}

func (*RuntimeInputRecoveryTask) TaskType() string        { return runtimeInputRecoveryTaskType }
func (*RuntimeInputRecoveryTask) CronSpec() string        { return "" }
func (*RuntimeInputRecoveryTask) Interval() time.Duration { return 30 * time.Second }
func (*RuntimeInputRecoveryTask) Payload() []byte         { return nil }
func (*RuntimeInputRecoveryTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(time.Minute), asynq.Unique(20 * time.Second)}
}

type RuntimeInputRecoveryHandler struct{ runtime runtimeservice.Service }

func NewRuntimeInputRecoveryHandler(runtime runtimeservice.Service) *RuntimeInputRecoveryHandler {
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
