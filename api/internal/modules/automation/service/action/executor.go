package action

import (
	"context"
	"fmt"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
)

// ActionExecutionRequest contains the task, run, and action snapshots needed to execute one action.
type ActionExecutionRequest struct {
	Task    *automationmodel.AutomationTask
	TaskRun *automationmodel.AutomationTaskRun
	Action  *automationmodel.AutomationTaskAction
}

// ActionExecutionResult stores normalized payloads for automation action run records.
type ActionExecutionResult struct {
	RequestPayload  map[string]interface{}
	ResponsePayload map[string]interface{}
	ChannelType     *automationmodel.NotificationChannelType
}

// ActionExecutor executes one supported automation action type.
type ActionExecutor interface {
	ActionType() automationmodel.AutomationActionType
	ExecuteAction(ctx context.Context, req ActionExecutionRequest) (*ActionExecutionResult, error)
}

// ExecutorRegistry maps action types to their executors.
type ExecutorRegistry struct {
	executors map[automationmodel.AutomationActionType]ActionExecutor
}

// NewExecutorRegistry creates an executor registry from non-nil executors.
func NewExecutorRegistry(executors ...ActionExecutor) *ExecutorRegistry {
	registry := &ExecutorRegistry{
		executors: make(map[automationmodel.AutomationActionType]ActionExecutor),
	}
	for _, executor := range executors {
		if executor == nil {
			continue
		}
		registry.executors[executor.ActionType()] = executor
	}
	return registry
}

// Get returns the executor registered for actionType.
func (r *ExecutorRegistry) Get(actionType automationmodel.AutomationActionType) (ActionExecutor, error) {
	if r == nil {
		return nil, fmt.Errorf("automation action executor registry is not configured")
	}
	executor, ok := r.executors[actionType]
	if !ok {
		return nil, fmt.Errorf("unsupported automation action type: %s", actionType)
	}
	return executor, nil
}
