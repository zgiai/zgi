package action

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

const (
	WorkflowVersionStrategyLatestPublished = "latest_published"
	WorkflowVersionStrategyPinned          = "pinned"

	defaultWorkflowTimeoutSeconds = 600
	minWorkflowTimeoutSeconds     = 30
	maxWorkflowTimeoutSeconds     = 1800
)

// AutomationWorkflowRunner is implemented by the workflow module for automation-triggered runs.
type AutomationWorkflowRunner interface {
	RunAutomationWorkflow(ctx context.Context, req WorkflowRunRequest) (*WorkflowRunResult, error)
}

// WorkflowRunRequest is the normalized request passed from automation to workflow.
type WorkflowRunRequest struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
	TaskID         string
	TaskRunID      string
	ActionID       string
	ScheduledFor   time.Time
	WorkflowRef    WorkflowRef
	Inputs         map[string]interface{}
	Timeout        time.Duration
}

// WorkflowRef identifies the target workflow and version strategy.
type WorkflowRef struct {
	AgentID         string
	WorkflowID      string
	VersionStrategy string
	VersionUUID     string
}

// WorkflowRunResult captures the workflow execution summary needed by automation.
type WorkflowRunResult struct {
	WorkflowRunID string
	WorkflowID    string
	AgentID       string
	Version       string
	Status        string
	Outputs       map[string]interface{}
	ElapsedTime   float64
}

type workflowRunnerProvider func() AutomationWorkflowRunner

// RunWorkflowExecutor executes run_workflow automation actions.
type RunWorkflowExecutor struct {
	runnerProvider workflowRunnerProvider
}

// NewRunWorkflowExecutor creates a run_workflow executor with a concrete runner.
func NewRunWorkflowExecutor(runner AutomationWorkflowRunner) *RunWorkflowExecutor {
	return NewRunWorkflowExecutorWithProvider(func() AutomationWorkflowRunner {
		return runner
	})
}

// NewRunWorkflowExecutorWithProvider creates a run_workflow executor with a lazy runner provider.
func NewRunWorkflowExecutorWithProvider(provider func() AutomationWorkflowRunner) *RunWorkflowExecutor {
	return &RunWorkflowExecutor{runnerProvider: provider}
}

// ActionType returns the automation action type handled by this executor.
func (e *RunWorkflowExecutor) ActionType() automationmodel.AutomationActionType {
	return automationmodel.AutomationActionTypeRunWorkflow
}

// ExecuteAction validates the action config and invokes the target workflow runner.
func (e *RunWorkflowExecutor) ExecuteAction(ctx context.Context, req ActionExecutionRequest) (*ActionExecutionResult, error) {
	if req.Task == nil {
		return nil, fmt.Errorf("automation task is required for run_workflow action")
	}
	if req.TaskRun == nil {
		return nil, fmt.Errorf("automation task run is required for run_workflow action")
	}
	if req.Action == nil {
		return nil, fmt.Errorf("automation action is required")
	}
	if req.Action.ActionType != automationmodel.AutomationActionTypeRunWorkflow {
		return nil, fmt.Errorf("unsupported automation action type: %s", req.Action.ActionType)
	}

	config, err := ParseWorkflowRunConfig(req.Action.Config)
	if err != nil {
		return nil, err
	}

	runner := e.runner()
	if runner == nil {
		return nil, fmt.Errorf("automation workflow runner is not configured")
	}

	runReq := WorkflowRunRequest{
		OrganizationID: req.Task.OrganizationID,
		WorkspaceID:    req.Task.WorkspaceID,
		AccountID:      req.Task.CreatedBy,
		TaskID:         req.Task.ID,
		TaskRunID:      req.TaskRun.ID,
		ActionID:       req.Action.ID,
		ScheduledFor:   req.TaskRun.ScheduledFor,
		WorkflowRef:    config.WorkflowRef,
		Inputs:         config.Inputs,
		Timeout:        time.Duration(config.TimeoutSeconds) * time.Second,
	}

	runCtx, cancel := context.WithTimeout(ctx, runReq.Timeout)
	defer cancel()

	result, err := runner.RunAutomationWorkflow(runCtx, runReq)
	if err != nil {
		if result != nil {
			return workflowActionExecutionResult(config, result), fmt.Errorf("run automation workflow: %w", err)
		}
		return nil, fmt.Errorf("run automation workflow: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("run automation workflow returned empty result")
	}

	return workflowActionExecutionResult(config, result), nil
}

func workflowActionExecutionResult(config *WorkflowRunConfig, result *WorkflowRunResult) *ActionExecutionResult {
	return &ActionExecutionResult{
		RequestPayload: map[string]interface{}{
			"workflow_ref": map[string]interface{}{
				"agent_id":         config.WorkflowRef.AgentID,
				"workflow_id":      config.WorkflowRef.WorkflowID,
				"version_strategy": config.WorkflowRef.VersionStrategy,
				"version_uuid":     config.WorkflowRef.VersionUUID,
			},
			"inputs":          config.Inputs,
			"timeout_seconds": config.TimeoutSeconds,
		},
		ResponsePayload: map[string]interface{}{
			"workflow_run_id":  result.WorkflowRunID,
			"workflow_id":      result.WorkflowID,
			"agent_id":         result.AgentID,
			"version":          result.Version,
			"version_strategy": config.WorkflowRef.VersionStrategy,
			"status":           result.Status,
			"elapsed_time":     result.ElapsedTime,
			"outputs_summary":  summarizeWorkflowOutputs(result.Outputs),
		},
	}
}

func (e *RunWorkflowExecutor) runner() AutomationWorkflowRunner {
	if e == nil || e.runnerProvider == nil {
		return nil
	}
	return e.runnerProvider()
}

// WorkflowRunConfig is the parsed run_workflow action configuration.
type WorkflowRunConfig struct {
	WorkflowRef    WorkflowRef
	Inputs         map[string]interface{}
	TimeoutSeconds int
}

// ValidateRunWorkflowConfig validates a run_workflow action config without executing it.
func ValidateRunWorkflowConfig(config map[string]interface{}) error {
	_, err := ParseWorkflowRunConfig(config)
	return err
}

// ParseWorkflowRunConfig parses and validates a run_workflow action config.
func ParseWorkflowRunConfig(config map[string]interface{}) (*WorkflowRunConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("run_workflow config is required")
	}

	workflowRefConfig, err := requiredMap(config, "workflow_ref")
	if err != nil {
		return nil, err
	}

	agentID := strings.TrimSpace(optionalString(workflowRefConfig, "agent_id"))
	if agentID == "" {
		return nil, fmt.Errorf("workflow_ref.agent_id is required")
	}

	versionStrategy := strings.TrimSpace(optionalString(workflowRefConfig, "version_strategy"))
	if versionStrategy == "" {
		versionStrategy = WorkflowVersionStrategyLatestPublished
	}
	if versionStrategy != WorkflowVersionStrategyLatestPublished && versionStrategy != WorkflowVersionStrategyPinned {
		return nil, fmt.Errorf("workflow_ref.version_strategy %q is not supported", versionStrategy)
	}

	versionUUID := strings.TrimSpace(optionalString(workflowRefConfig, "version_uuid"))
	if versionStrategy == WorkflowVersionStrategyPinned && versionUUID == "" {
		return nil, fmt.Errorf("workflow_ref.version_uuid is required when version_strategy is pinned")
	}

	inputs, err := optionalMap(config, "inputs")
	if err != nil {
		return nil, err
	}
	if inputs == nil {
		inputs = map[string]interface{}{}
	}
	if key := findSensitiveInputKey(inputs); key != "" {
		return nil, fmt.Errorf("run_workflow inputs must not contain sensitive key %q", key)
	}

	timeoutSeconds, err := parseWorkflowTimeoutSeconds(config)
	if err != nil {
		return nil, err
	}

	return &WorkflowRunConfig{
		WorkflowRef: WorkflowRef{
			AgentID:         agentID,
			WorkflowID:      strings.TrimSpace(optionalString(workflowRefConfig, "workflow_id")),
			VersionStrategy: versionStrategy,
			VersionUUID:     versionUUID,
		},
		Inputs:         inputs,
		TimeoutSeconds: timeoutSeconds,
	}, nil
}

func parseWorkflowTimeoutSeconds(config map[string]interface{}) (int, error) {
	executionConfig, err := optionalMap(config, "execution")
	if err != nil {
		return 0, err
	}
	if executionConfig == nil {
		return defaultWorkflowTimeoutSeconds, nil
	}

	raw, ok := executionConfig["timeout_seconds"]
	if !ok || raw == nil {
		return defaultWorkflowTimeoutSeconds, nil
	}

	timeoutSeconds, ok := intFromValue(raw)
	if !ok {
		return 0, fmt.Errorf("execution.timeout_seconds must be an integer")
	}
	if timeoutSeconds < minWorkflowTimeoutSeconds || timeoutSeconds > maxWorkflowTimeoutSeconds {
		return 0, fmt.Errorf("execution.timeout_seconds must be between %d and %d", minWorkflowTimeoutSeconds, maxWorkflowTimeoutSeconds)
	}
	return timeoutSeconds, nil
}

func requiredMap(config map[string]interface{}, key string) (map[string]interface{}, error) {
	value, err := optionalMap(config, key)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalMap(config map[string]interface{}, key string) (map[string]interface{}, error) {
	value, ok := config[key]
	if !ok || value == nil {
		return nil, nil
	}
	typed, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return typed, nil
}

func optionalString(config map[string]interface{}, key string) string {
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

func intFromValue(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}

func findSensitiveInputKey(inputs map[string]interface{}) string {
	for key, value := range inputs {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "api_key" || normalized == "password" || normalized == "secret" || normalized == "token" {
			return key
		}
		child, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		if found := findSensitiveInputKey(child); found != "" {
			return found
		}
	}
	return ""
}

func summarizeWorkflowOutputs(outputs map[string]interface{}) map[string]interface{} {
	keys := make([]string, 0, len(outputs))
	for key := range outputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sizeBytes := 0
	if payload, err := json.Marshal(outputs); err == nil {
		sizeBytes = len(payload)
	}

	return map[string]interface{}{
		"keys":       keys,
		"size_bytes": sizeBytes,
	}
}
