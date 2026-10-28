package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

// DigitalHumanRunRequest is the narrow Automation-to-DigitalHuman invocation contract.
type DigitalHumanRunRequest struct {
	ActionRunID           string
	OrganizationID        string
	WorkspaceID           string
	DigitalHumanID        string
	OwnerKind             string
	OwnerSubjectKind      string
	OwnerSubjectID        string
	OwnerConversationID   string
	OriginSurface         string
	ChannelProvider       string
	ChannelInstallationID string
	ChannelConversationID string
	TaskID                string
	TaskRunID             string
	CreatorAccountID      string
	Name                  string
	Goal                  string
	SuccessCriteria       []string
	DeliveryMode          string
	TimeoutSeconds        int
	MaxTaskTokens         int
	ReplyTo               *invocation.ReplyAddress
	// ReworkFeedback is the requester's feedback when this run redoes a task
	// whose report they sent back for rework.
	ReworkFeedback string
	// Trigger says what started the task run: the scheduler, a person, or a
	// retry. People see it as the run's source.
	Trigger string
	// ServerEvent describes the alert when a server watch started the run
	// (ADR 0031).
	ServerEvent string
}

type DigitalHumanRunResult struct{ RunID string }

type DigitalHumanRunState struct {
	Status       string
	Result       string
	ErrorMessage string
}

// AutomationDigitalHumanRunner submits one durable DigitalHuman invocation.
type AutomationDigitalHumanRunner interface {
	RunDigitalHuman(context.Context, DigitalHumanRunRequest) (*DigitalHumanRunResult, error)
	GetDigitalHumanRun(context.Context, string, string, string, string) (*DigitalHumanRunState, error)
	CancelDigitalHumanRun(context.Context, string, string, string, string) error
}

type DigitalHumanRunnerProvider func() AutomationDigitalHumanRunner

// RunDigitalHumanExecutor starts a subject run and deliberately leaves the
// Automation action running until the terminal reconciler observes its result.
type RunDigitalHumanExecutor struct{ provider DigitalHumanRunnerProvider }

func NewRunDigitalHumanExecutor(provider DigitalHumanRunnerProvider) *RunDigitalHumanExecutor {
	return &RunDigitalHumanExecutor{provider: provider}
}

func (*RunDigitalHumanExecutor) ActionType() automationmodel.AutomationActionType {
	return automationmodel.AutomationActionTypeRunDigitalHuman
}

func (e *RunDigitalHumanExecutor) ExecuteAction(ctx context.Context, req ActionExecutionRequest) (*ActionExecutionResult, error) {
	if req.Task == nil || req.TaskRun == nil || req.ActionRun == nil || req.Task.SubjectID == nil || strings.TrimSpace(*req.Task.SubjectID) == "" {
		return nil, fmt.Errorf("digital human automation action is incomplete")
	}
	if e == nil || e.provider == nil {
		return nil, fmt.Errorf("digital human runner is not available")
	}
	runner := e.provider()
	if runner == nil {
		return nil, fmt.Errorf("digital human runner is not available")
	}
	request := DigitalHumanRunRequest{
		ActionRunID: req.ActionRun.ID, OrganizationID: req.Task.OrganizationID, WorkspaceID: req.Task.WorkspaceID,
		DigitalHumanID: *req.Task.SubjectID, TaskID: req.Task.ID, TaskRunID: req.TaskRun.ID, CreatorAccountID: req.Task.CreatedBy,
		OwnerKind: stringValue(req.Task.OwnerKind), OwnerSubjectKind: stringValue(req.Task.OwnerSubjectKind),
		OwnerSubjectID: stringValue(req.Task.OwnerSubjectID), OwnerConversationID: stringValue(req.Task.OwnerConversationID),
		OriginSurface:         stringValue(req.Task.OriginSurface),
		ChannelProvider:       strings.TrimPrefix(stringValue(req.Task.SourceRef), "channel:"),
		ChannelInstallationID: mapStringValue(req.Task.SourceSnapshot, "installation_id"),
		ChannelConversationID: mapStringValue(req.Task.SourceSnapshot, "conversation_id"),
		Name:                  req.Task.Name, Goal: stringValue(req.Task.Goal), SuccessCriteria: append([]string(nil), req.Task.SuccessCriteria...),
		DeliveryMode:   mapStringValue(req.Task.DeliveryConfig, "mode"),
		TimeoutSeconds: req.Task.ExecutionTimeoutSeconds, ReplyTo: replyAddress(req.Task.DeliveryConfig),
		ReworkFeedback: strings.TrimSpace(mapStringValue(req.TaskRun.RuntimeContext, "rework_feedback")),
		Trigger:        string(req.TaskRun.TriggerSource),
		ServerEvent:    serverEventSummary(req.TaskRun.RuntimeContext["server_event"]),
	}
	if stringValue(req.Task.SourceRef) == "social_operations" {
		data, marshalErr := json.Marshal(req.Task.SourceSnapshot["social_operations"])
		var snapshot struct {
			Budget struct {
				MaxTokens int `json:"max_tokens"`
			} `json:"budget"`
		}
		if marshalErr != nil || json.Unmarshal(data, &snapshot) != nil || snapshot.Budget.MaxTokens < 1 || snapshot.Budget.MaxTokens > 100000 {
			return nil, fmt.Errorf("social operations token budget unavailable")
		}
		request.MaxTaskTokens = snapshot.Budget.MaxTokens
	}
	result, err := runner.RunDigitalHuman(ctx, request)
	payload := map[string]any{"task_id": request.TaskID, "task_run_id": request.TaskRunID, "digital_human_id": request.DigitalHumanID}
	if err != nil {
		return &ActionExecutionResult{RequestPayload: payload}, fmt.Errorf("submit digital human task run: %w", err)
	}
	if result == nil || strings.TrimSpace(result.RunID) == "" {
		return &ActionExecutionResult{RequestPayload: payload}, fmt.Errorf("digital human runner returned no run id")
	}
	return &ActionExecutionResult{RequestPayload: payload, ResponsePayload: map[string]any{"digital_human_run_id": result.RunID}, Pending: true, ExternalRunType: "digital_human", ExternalRunID: result.RunID}, nil
}

func replyAddress(config map[string]any) *invocation.ReplyAddress {
	if config == nil || config["mode"] != "source_conversation" {
		return nil
	}
	raw, ok := config["reply_to"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var reply invocation.ReplyAddress
	if json.Unmarshal(encoded, &reply) != nil || strings.TrimSpace(reply.Provider) == "" || strings.TrimSpace(reply.ConversationID) == "" {
		return nil
	}
	return &reply
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func mapStringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

// serverEventSummary renders a server watch alert for the run's instruction.
func serverEventSummary(raw any) string {
	event, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	targets, _ := event["targets"].([]any)
	lines := make([]string, 0, len(targets)+1)
	for _, item := range targets {
		target, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := target["name"].(string)
		observed, _ := target["observed"].(string)
		failures := fmt.Sprint(target["failures"])
		if name == "@host" {
			lines = append(lines, fmt.Sprintf("- the server could not be reached or logged in to (%s) in %s checks in a row", observed, failures))
			continue
		}
		lines = append(lines, fmt.Sprintf("- service %s is %s (%s checks in a row)", name, observed, failures))
	}
	if len(lines) == 0 {
		return ""
	}
	if at, _ := event["detected_at"].(string); at != "" {
		lines = append(lines, "Detected at "+at+".")
	}
	return strings.Join(lines, "\n")
}
