package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/errors/failureprojection"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	"github.com/zgiai/zgi/api/internal/modules/tools/workflowevents"
)

const (
	ProviderID               = "workflow"
	ToolListAgentWorkflows   = "list_agent_workflows"
	ToolRunAgentWorkflow     = "run_agent_workflow"
	ToolGetWorkflowRunStatus = "get_workflow_run_status"

	defaultTimeoutSeconds = 600
	minTimeoutSeconds     = 30
	maxTimeoutSeconds     = 1800
	defaultInputKey       = "query"

	agentRuntimeSourceParameter = "agent_runtime_source"
	workflowApprovalUIAllowed   = "ui_approval_allowed"
	publishedWorkflowError      = "workflow run failed"
	workflowErrorVisibilityKey  = "error_visibility"
	workflowErrorGeneric        = "generic"
)

type RunnerProvider func() automationaction.AutomationWorkflowRunner

type Provider struct {
	*builtin.BuiltinProvider
	runnerProvider RunnerProvider
}

func NewProvider(runnerProvider RunnerProvider) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Workflow Tools",
			"zh_Hans": "Workflow Tools",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for running Agent-bound workflows.",
			"zh_Hans": "Built-in tools for running Agent-bound workflows.",
		},
		Icon: "workflow",
		Tags: []string{"workflow", "system"},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
		runnerProvider:  runnerProvider,
	}
	for _, name := range []string{ToolListAgentWorkflows, ToolRunAgentWorkflow, ToolGetWorkflowRunStatus} {
		provider.RegisterTool(newWorkflowTool(runnerProvider, name))
	}
	return provider
}

type workflowTool struct {
	*builtin.BuiltinTool
	runnerProvider RunnerProvider
	kind           string
}

func newWorkflowTool(runnerProvider RunnerProvider, kind string) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     kind,
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": workflowToolLabel(kind), "zh_Hans": workflowToolLabel(kind)},
			Icon:     "workflow",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": workflowToolDescription(kind), "zh_Hans": workflowToolDescription(kind)},
			LLM:   workflowToolDescription(kind),
		},
		Parameters: workflowToolParameters(kind),
		OutputType: "json",
		Tags:       []string{"workflow", "system"},
	}
	return &workflowTool{
		BuiltinTool:    builtin.NewBuiltinTool(entity, ""),
		runnerProvider: runnerProvider,
		kind:           kind,
	}
}

func (t *workflowTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = appID
	runtime := t.Runtime()
	if runtime == nil || runtime.InvokeFrom != tools.ToolInvokeFromAgent {
		return nil, fmt.Errorf("%s is only available to Agent skill runtimes", t.kind)
	}
	scope, err := workflowScopeFromRuntime(runtime, userID)
	if err != nil {
		return nil, err
	}
	bindings, err := workflowBindingsFromRuntime(runtime)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case ToolListAgentWorkflows:
		return jsonMessages(map[string]interface{}{
			"status":    "succeeded",
			"workflows": workflowBindingList(bindings),
		})
	case ToolRunAgentWorkflow:
		return t.runWorkflow(ctx, scope, params, bindings, stringPointerValue(conversationID), stringPointerValue(messageID))
	case ToolGetWorkflowRunStatus:
		return t.getWorkflowRunStatus(ctx, scope, params, bindings)
	default:
		return nil, fmt.Errorf("unknown workflow tool %s", t.kind)
	}
}

func (t *workflowTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &workflowTool{
		BuiltinTool:    t.BuiltinTool.ForkToolRuntime(runtime),
		runnerProvider: t.runnerProvider,
		kind:           t.kind,
	}
}

func (t *workflowTool) runner() automationaction.AutomationWorkflowRunner {
	if t == nil || t.runnerProvider == nil {
		return nil
	}
	return t.runnerProvider()
}

func (t *workflowTool) runWorkflow(ctx context.Context, scope workflowScope, params map[string]interface{}, bindings []workflowBinding, conversationID, messageID string) ([]tools.ToolInvokeMessage, error) {
	bindingID := strings.TrimSpace(stringValue(params, "binding_id"))
	if bindingID == "" {
		return nil, fmt.Errorf("binding_id is required")
	}
	binding, ok := findWorkflowBinding(bindings, bindingID)
	if !ok {
		return nil, fmt.Errorf("unknown workflow binding_id %s", bindingID)
	}
	if binding.VersionStrategy == automationaction.WorkflowVersionStrategyPinned && strings.TrimSpace(binding.VersionUUID) == "" {
		return nil, fmt.Errorf("workflow binding %s requires version_uuid for pinned strategy", binding.BindingID)
	}
	scope = workflowScopeForBinding(t.Runtime(), scope, binding)
	runner := t.runner()
	if runner == nil {
		return nil, fmt.Errorf("automation workflow runner is not configured")
	}
	inputs, err := inputMap(params, "inputs")
	if err != nil {
		return nil, err
	}
	inputs, err = normalizeWorkflowInputs(inputs, binding)
	if err != nil {
		return nil, err
	}
	invocation := workflowInvocationContext(t.Runtime(), binding, conversationID, messageID)
	if invocation.Mode == automationaction.WorkflowInvocationModeAgentDelegate {
		injectWorkflowContext(inputs, t.Runtime())
	}
	timeout := time.Duration(normalizeTimeoutSeconds(binding.TimeoutSeconds)) * time.Second
	runReq := automationaction.WorkflowRunRequest{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		AccountID:      scope.AccountID,
		ScheduledFor:   time.Now(),
		WorkflowRef: automationaction.WorkflowRef{
			AgentID:         binding.AgentID,
			WorkflowID:      binding.WorkflowID,
			VersionStrategy: binding.VersionStrategy,
			VersionUUID:     binding.VersionUUID,
		},
		Inputs:     inputs,
		Timeout:    timeout,
		Invocation: &invocation,
	}
	if emitter := workflowevents.FromContext(ctx); emitter != nil {
		runReq.EventSink = func(event automationaction.WorkflowRunEvent) {
			if suppressAgentWorkflowAnswerTransport(invocation.Mode, event.Type) {
				return
			}
			payload := make(map[string]interface{}, len(event.Payload)+3)
			for key, value := range event.Payload {
				payload[key] = value
			}
			payload["invocation_id"] = invocation.InvocationID
			payload["invocation_mode"] = invocation.Mode
			payload["invocation_protocol_version"] = invocation.ProtocolVersion
			applyPublishedWorkflowEventFailureExposure(t.Runtime(), event.Type, payload)
			annotateWorkflowApprovalUIAccess(t.Runtime(), event.Type, payload)
			emitter(workflowevents.Event{
				Type:            event.Type,
				Payload:         payload,
				Sequence:        event.Sequence,
				SchemaVersion:   event.SchemaVersion,
				PayloadVersion:  event.PayloadVersion,
				ExecutionID:     event.ExecutionID,
				PauseID:         event.PauseID,
				PauseGeneration: event.PauseGeneration,
			})
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, runErr := runner.RunAutomationWorkflow(runCtx, runReq)
	if result == nil {
		if runErr != nil {
			payload := failedWorkflowPayload("", "", "", runErr)
			applyPublishedWorkflowResultFailureExposure(t.Runtime(), payload)
			return jsonMessages(payload)
		}
		payload := failedWorkflowPayload("", "", "", fmt.Errorf("workflow returned empty result"))
		applyPublishedWorkflowResultFailureExposure(t.Runtime(), payload)
		return jsonMessages(payload)
	}
	// The invocation identity belongs to the parent Agent turn. Keep it intact
	// even when a legacy/custom runner has not echoed the new result fields yet.
	if strings.TrimSpace(result.InvocationID) == "" {
		result.InvocationID = invocation.InvocationID
	}
	if strings.TrimSpace(result.InvocationMode) == "" {
		result.InvocationMode = invocation.Mode
	}
	payload := workflowResultPayload(result, runErr, binding, t.Runtime())
	return jsonMessages(payload)
}

func (t *workflowTool) getWorkflowRunStatus(ctx context.Context, scope workflowScope, params map[string]interface{}, bindings []workflowBinding) ([]tools.ToolInvokeMessage, error) {
	workflowRunID := strings.TrimSpace(stringValue(params, "workflow_run_id"))
	if workflowRunID == "" {
		return nil, fmt.Errorf("workflow_run_id is required")
	}
	runner := t.runner()
	if runner == nil {
		return nil, fmt.Errorf("automation workflow runner is not configured")
	}
	reader, ok := runner.(automationaction.AutomationWorkflowRunStatusReader)
	if !ok {
		return nil, fmt.Errorf("workflow run status reader is not configured")
	}
	var lastErr error
	for _, candidateScope := range workflowScopesForBindings(t.Runtime(), scope, bindings) {
		result, err := reader.GetAutomationWorkflowRunStatus(ctx, automationaction.WorkflowRunStatusRequest{
			OrganizationID: candidateScope.OrganizationID,
			WorkspaceID:    candidateScope.WorkspaceID,
			AccountID:      candidateScope.AccountID,
			WorkflowRunID:  workflowRunID,
		})
		if err != nil {
			lastErr = err
			continue
		}
		if result == nil {
			lastErr = fmt.Errorf("workflow run status is empty")
			continue
		}
		targetBinding, allowed := workflowBindingForRun(result, bindings)
		if !allowed {
			lastErr = fmt.Errorf("workflow_run_id %s is not part of the current Agent workflow bindings", workflowRunID)
			continue
		}
		targetScope := workflowScopeForBinding(t.Runtime(), scope, targetBinding)
		if targetScope.AccountID != candidateScope.AccountID {
			continue
		}
		return jsonMessages(workflowStatusPayload(result, t.Runtime()))
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("workflow run status is empty")
}

func workflowResultPayload(result *automationaction.WorkflowRunResult, runErr error, binding workflowBinding, runtime *tools.ToolRuntime) map[string]interface{} {
	outputs := safeOutputs(result.Outputs)
	status := normalizeWorkflowStatus(result.Status, outputs)
	payload := map[string]interface{}{
		"status":          status,
		"workflow_run_id": result.WorkflowRunID,
		"workflow_id":     result.WorkflowID,
		"agent_id":        result.AgentID,
		"agent_type":      strings.TrimSpace(binding.AgentType),
		"binding_id":      strings.TrimSpace(binding.BindingID),
		"version":         result.Version,
		"outputs":         outputs,
		"primary_output":  primaryWorkflowOutput(outputs),
		"output_keys":     workflowOutputKeys(outputs),
		"elapsed_time":    result.ElapsedTime,
		"invocation_id":   result.InvocationID,
		"invocation_mode": result.InvocationMode,
	}
	if status == "pending_approval" {
		mergeApprovalFields(payload, result.Outputs)
		applyWorkflowApprovalExposure(runtime, payload)
	}
	if status == "pending_question" {
		mergeQuestionAnswerFields(payload, result.Outputs)
	}
	if runErr != nil {
		payload["status"] = "failed"
		payload["error"] = strings.TrimSpace(runErr.Error())
	}
	applyPublishedWorkflowResultFailureExposure(runtime, payload)
	return payload
}

func workflowInvocationContext(runtime *tools.ToolRuntime, binding workflowBinding, conversationID, messageID string) automationaction.WorkflowInvocationContext {
	conversationID = firstNonEmptyWorkflowString(conversationID, runtimeStringParameter(runtime, "workflow_parent_conversation_id"))
	messageID = firstNonEmptyWorkflowString(messageID, runtimeStringParameter(runtime, "workflow_parent_message_id"))
	toolCallID := runtimeStringParameter(runtime, "workflow_parent_tool_call_id")
	invocationSeed := strings.Join([]string{messageID, toolCallID, strings.TrimSpace(binding.BindingID)}, ":")
	if strings.Trim(invocationSeed, ":") == "" {
		invocationSeed = strings.Join([]string{conversationID, strings.TrimSpace(binding.BindingID), time.Now().UTC().Format(time.RFC3339Nano)}, ":")
	}
	invocationID := fmt.Sprintf("%x", sha256.Sum256([]byte(invocationSeed)))
	mode := automationaction.WorkflowInvocationModeAgentTaskTool
	var contextSnapshot map[string]interface{}
	if strings.EqualFold(strings.TrimSpace(binding.AgentType), "CONVERSATIONAL_WORKFLOW") {
		mode = automationaction.WorkflowInvocationModeAgentDelegate
		contextSnapshot = workflowRuntimeContextSnapshot(runtime)
	}
	return automationaction.WorkflowInvocationContext{
		InvocationID:         invocationID,
		ProtocolVersion:      1,
		Mode:                 mode,
		ParentConversationID: conversationID,
		ParentMessageID:      messageID,
		BindingID:            strings.TrimSpace(binding.BindingID),
		ContextDigest:        workflowContextDigest(contextSnapshot),
		ContextSnapshot:      contextSnapshot,
	}
}

func isWorkflowAnswerTransportEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "message", "text_chunk", "text_replace", "message_end":
		return true
	default:
		return false
	}
}

func suppressAgentWorkflowAnswerTransport(invocationMode string, eventType string) bool {
	if !isWorkflowAnswerTransportEvent(eventType) {
		return false
	}

	switch strings.TrimSpace(invocationMode) {
	case automationaction.WorkflowInvocationModeAgentTaskTool:
		return true
	case automationaction.WorkflowInvocationModeAgentDelegate:
		// The parent Agent owns message_end and replacement semantics, but ordered
		// conversation message chunks must cross this boundary immediately so the
		// delegated workflow can take over the visible answer stream.
		return strings.TrimSpace(eventType) != "message"
	default:
		return false
	}
}

func workflowRuntimeContextSnapshot(runtime *tools.ToolRuntime) map[string]interface{} {
	if runtime == nil || runtime.RuntimeParameters == nil {
		return nil
	}
	contextMap, ok := runtime.RuntimeParameters["workflow_context"].(map[string]interface{})
	if !ok || len(contextMap) == 0 {
		return nil
	}
	payload, err := json.Marshal(contextMap)
	if err != nil {
		return nil
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil
	}
	return snapshot
}

func workflowContextDigest(snapshot map[string]interface{}) string {
	if len(snapshot) == 0 {
		return ""
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func runtimeStringParameter(runtime *tools.ToolRuntime, key string) string {
	if runtime == nil || runtime.RuntimeParameters == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(runtime.RuntimeParameters[key]))
}

func firstNonEmptyWorkflowString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" && trimmed != "<nil>" {
			return trimmed
		}
	}
	return ""
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func workflowStatusPayload(result *automationaction.WorkflowRunStatusResult, runtime *tools.ToolRuntime) map[string]interface{} {
	outputs := safeOutputs(result.Outputs)
	status := normalizeWorkflowStatus(result.Status, outputs)
	payload := map[string]interface{}{
		"status":           status,
		"workflow_run_id":  result.WorkflowRunID,
		"workflow_id":      result.WorkflowID,
		"agent_id":         result.AgentID,
		"version":          result.Version,
		"outputs":          outputs,
		"primary_output":   primaryWorkflowOutput(outputs),
		"output_keys":      workflowOutputKeys(outputs),
		"elapsed_time":     result.ElapsedTime,
		"created_at_unix":  result.CreatedAtUnix,
		"finished_at_unix": result.FinishedAtUnix,
	}
	if status == "pending_approval" {
		mergeApprovalFields(payload, result.Outputs)
		applyWorkflowApprovalExposure(runtime, payload)
	}
	if status == "pending_question" {
		mergeQuestionAnswerFields(payload, result.Outputs)
	}
	if strings.TrimSpace(result.Error) != "" {
		payload["error"] = strings.TrimSpace(result.Error)
	}
	applyPublishedWorkflowResultFailureExposure(runtime, payload)
	return payload
}

func failedWorkflowPayload(workflowRunID, workflowID, agentID string, err error) map[string]interface{} {
	payload := map[string]interface{}{
		"status":          "failed",
		"workflow_run_id": strings.TrimSpace(workflowRunID),
		"workflow_id":     strings.TrimSpace(workflowID),
		"agent_id":        strings.TrimSpace(agentID),
		"outputs":         map[string]interface{}{},
		"primary_output":  "",
		"output_keys":     []string{},
	}
	if err != nil {
		payload["error"] = strings.TrimSpace(err.Error())
	}
	return payload
}

func applyPublishedWorkflowResultFailureExposure(runtime *tools.ToolRuntime, payload map[string]interface{}) {
	if !publishedWorkflowFailureDetailsHidden(runtime) || payload == nil {
		return
	}
	if !failureprojection.IsFailureStatus(stringValue(payload, "status")) {
		return
	}
	projected := failureprojection.ProjectPublicPayload(payload, publishedWorkflowError, true)
	for key := range payload {
		delete(payload, key)
	}
	for key, value := range projected {
		payload[key] = value
	}
	payload[workflowErrorVisibilityKey] = workflowErrorGeneric
}

func applyPublishedWorkflowEventFailureExposure(runtime *tools.ToolRuntime, eventType string, payload map[string]interface{}) {
	if !publishedWorkflowFailureDetailsHidden(runtime) || payload == nil {
		return
	}
	if !workflowEventReportsFailure(eventType, payload) {
		return
	}
	projected := failureprojection.ProjectPublicPayload(payload, publishedWorkflowError, true)
	for key := range payload {
		delete(payload, key)
	}
	for key, value := range projected {
		payload[key] = value
	}
	payload[workflowErrorVisibilityKey] = workflowErrorGeneric
}

func publishedWorkflowFailureDetailsHidden(runtime *tools.ToolRuntime) bool {
	switch strings.ToLower(runtimeStringParameter(runtime, agentRuntimeSourceParameter)) {
	case "webapp", "external-api":
		return true
	default:
		return false
	}
}

func workflowEventReportsFailure(eventType string, payload map[string]interface{}) bool {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if eventType == "error" || strings.HasSuffix(eventType, "_failed") {
		return true
	}
	return failureprojection.IsFailureStatus(stringValue(payload, "status"))
}

func normalizeWorkflowStatus(status string, outputs map[string]interface{}) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paused":
		if hasQuestionAnswerFields(outputs) {
			return "pending_question"
		}
		return "pending_approval"
	case "":
		return "unknown"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func hasQuestionAnswerFields(outputs map[string]interface{}) bool {
	fields := findQuestionAnswerFields(outputs)
	return cleanOutputText(fields["question"]) != ""
}

func mergeQuestionAnswerFields(payload map[string]interface{}, outputs map[string]interface{}) {
	if len(outputs) == 0 {
		return
	}
	fields := findQuestionAnswerFields(outputs)
	for key, value := range fields {
		payload[key] = value
	}
}

func findQuestionAnswerFields(value interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	var copyFields func(map[string]interface{})
	copyFields = func(source map[string]interface{}) {
		copyQuestionAnswerField(out, "node_id", source, "node_id")
		copyQuestionAnswerField(out, "node_title", source, "node_title")
		copyQuestionAnswerField(out, "question", source, "question")
		copyQuestionAnswerField(out, "round", source, "round")
		copyQuestionAnswerField(out, "choices", source, "choices")
		copyQuestionAnswerField(out, "answer", source, "answer")
		copyQuestionAnswerField(out, "choice_id", source, "choice_id")
		copyQuestionAnswerField(out, "choice_label", source, "choice_label")
		copyQuestionAnswerField(out, "choice_value", source, "choice_value")
	}
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			if qa, ok := typed["__question_answer"]; ok && qa != nil {
				if record, ok := qa.(map[string]interface{}); ok {
					copyFields(record)
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func copyQuestionAnswerField(out map[string]interface{}, target string, source map[string]interface{}, keys ...string) {
	if _, exists := out[target]; exists {
		return
	}
	for _, key := range keys {
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		out[target] = value
		return
	}
}

func mergeApprovalFields(payload map[string]interface{}, outputs map[string]interface{}) {
	if len(outputs) == 0 {
		return
	}
	fields := findApprovalFields(outputs)
	for key, value := range fields {
		payload[key] = value
	}
}

func annotateWorkflowApprovalUIAccess(runtime *tools.ToolRuntime, eventType string, payload map[string]interface{}) {
	if len(payload) == 0 {
		return
	}
	switch strings.TrimSpace(eventType) {
	case "approval_requested":
		applyWorkflowApprovalExposure(runtime, payload)
	}
}

func applyWorkflowApprovalExposure(runtime *tools.ToolRuntime, payload map[string]interface{}) {
	allowed := workflowApprovalUIAccessAllowed(runtime, payload)
	payload[workflowApprovalUIAllowed] = allowed
	if allowed {
		return
	}
	redacted, ok := redactWorkflowApprovalCredentials(payload, false).(map[string]interface{})
	if !ok {
		return
	}
	for key := range payload {
		delete(payload, key)
	}
	for key, value := range redacted {
		payload[key] = value
	}
}

func redactWorkflowApprovalCredentials(value interface{}, inApprovalForm bool) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if normalizedKey == "approval_token" || normalizedKey == "__approval_token" {
				continue
			}
			childIsApprovalForm := inApprovalForm || normalizedKey == "approval_form" || normalizedKey == "__approval_form"
			if childIsApprovalForm && normalizedKey == "token" {
				continue
			}
			redacted[key] = redactWorkflowApprovalCredentials(child, childIsApprovalForm)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, len(typed))
		for index, child := range typed {
			redacted[index] = redactWorkflowApprovalCredentials(child, inApprovalForm)
		}
		return redacted
	default:
		return value
	}
}

func workflowApprovalUIAccessAllowed(runtime *tools.ToolRuntime, payload map[string]interface{}) bool {
	switch strings.ToLower(runtimeStringParameter(runtime, agentRuntimeSourceParameter)) {
	case "console":
		// Draft preview is an operator/debug surface. It deliberately provides
		// an inline escape hatch regardless of the workflow delivery channels.
		return true
	case "webapp", "external-api":
		// Published applications must honor the workflow's configured channel.
		return workflowApprovalWebAppEnabled(payload)
	default:
		return false
	}
}

func workflowApprovalWebAppEnabled(payload map[string]interface{}) bool {
	methods, ok := workflowApprovalRecord(payload["submit_methods"])
	if !ok {
		if form, formOK := workflowApprovalRecord(payload["approval_form"]); formOK {
			methods, ok = workflowApprovalRecord(form["submit_methods"])
		}
	}
	if !ok {
		return false
	}
	webApp, ok := workflowApprovalRecord(methods["webapp"])
	if !ok {
		return false
	}
	enabled, exists := webApp["enabled"]
	if !exists || enabled == nil {
		// The workflow approval contract defaults an omitted webapp.enabled to
		// true for legacy definitions.
		return true
	}
	value, ok := enabled.(bool)
	return ok && value
}

func workflowApprovalRecord(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	if record, ok := value.(map[string]interface{}); ok {
		return record, true
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var record map[string]interface{}
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, false
	}
	return record, true
}

func findApprovalFields(value interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			copyApprovalField(out, "approval_form_id", typed, "__approval_form_id", "approval_form_id", "form_id")
			copyApprovalField(out, "approval_token", typed, "__approval_token", "approval_token", "token")
			copyApprovalField(out, "approval_url", typed, "approval_url", "url")
			if form, ok := typed["__approval_form"]; ok && form != nil {
				out["approval_form"] = form
				copyApprovalFormFields(out, form)
				walk(form)
			}
			if form, ok := typed["approval_form"]; ok && form != nil {
				out["approval_form"] = form
				copyApprovalFormFields(out, form)
				walk(form)
			}
			for _, child := range typed {
				walk(child)
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func copyApprovalFormFields(out map[string]interface{}, form interface{}) {
	formMap, ok := form.(map[string]interface{})
	if !ok {
		return
	}
	copyApprovalField(out, "approval_form_id", formMap, "id", "form_id")
	copyApprovalField(out, "approval_token", formMap, "token")
	copyApprovalField(out, "approval_url", formMap, "url", "approval_url")
}

func copyApprovalField(out map[string]interface{}, target string, source map[string]interface{}, keys ...string) {
	if _, exists := out[target]; exists {
		return
	}
	for _, key := range keys {
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		if str := strings.TrimSpace(fmt.Sprint(value)); str != "" {
			out[target] = str
			return
		}
	}
}

func safeOutputs(outputs map[string]interface{}) map[string]interface{} {
	if outputs == nil {
		return map[string]interface{}{}
	}
	return outputs
}

func primaryWorkflowOutput(outputs map[string]interface{}) string {
	if len(outputs) == 0 {
		return ""
	}
	if answer := cleanOutputText(outputs["answer"]); answer != "" {
		return answer
	}
	var found string
	var walk func(interface{})
	walk = func(value interface{}) {
		if found != "" || value == nil {
			return
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			if answer := cleanOutputText(typed["answer"]); answer != "" {
				found = answer
				return
			}
			for _, child := range typed {
				walk(child)
				if found != "" {
					return
				}
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
				if found != "" {
					return
				}
			}
		}
	}
	walk(outputs)
	return found
}

func cleanOutputText(value interface{}) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func workflowOutputKeys(outputs map[string]interface{}) []string {
	if len(outputs) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(outputs))
	for key := range outputs {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func workflowRunAllowed(result *automationaction.WorkflowRunStatusResult, bindings []workflowBinding) bool {
	_, allowed := workflowBindingForRun(result, bindings)
	return allowed
}

func workflowBindingForRun(result *automationaction.WorkflowRunStatusResult, bindings []workflowBinding) (workflowBinding, bool) {
	if result == nil || strings.TrimSpace(result.WorkflowID) == "" || strings.TrimSpace(result.AgentID) == "" {
		return workflowBinding{}, false
	}
	for _, binding := range bindings {
		if strings.TrimSpace(binding.AgentID) != strings.TrimSpace(result.AgentID) {
			continue
		}
		if strings.TrimSpace(binding.WorkflowID) == strings.TrimSpace(result.WorkflowID) {
			return binding, true
		}
	}
	return workflowBinding{}, false
}

func jsonMessages(payload map[string]interface{}) ([]tools.ToolInvokeMessage, error) {
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

type workflowScope struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
}

func workflowScopeFromRuntime(runtime *tools.ToolRuntime, userID string) (workflowScope, error) {
	organizationID := strings.TrimSpace(stringValue(runtime.RuntimeParameters, "organization_id"))
	workspaceID := strings.TrimSpace(stringValue(runtime.RuntimeParameters, "workspace_id"))
	accountID := strings.TrimSpace(userID)
	if boundBy := strings.TrimSpace(stringValue(runtime.RuntimeParameters, "workflow_bound_by_account_id")); boundBy != "" {
		accountID = boundBy
	}
	if accountID == "" {
		return workflowScope{}, fmt.Errorf("account_id is required")
	}
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(runtime.TenantID)
	}
	if organizationID == "" {
		organizationID = workspaceID
	}
	if workspaceID == "" {
		return workflowScope{}, fmt.Errorf("workspace_id is required")
	}
	return workflowScope{OrganizationID: organizationID, WorkspaceID: workspaceID, AccountID: accountID}, nil
}

func workflowScopeForBinding(runtime *tools.ToolRuntime, scope workflowScope, binding workflowBinding) workflowScope {
	if runtime == nil || runtime.InvokeFrom != tools.ToolInvokeFromAgent {
		return scope
	}
	if authorization, ok := tools.AgentBindingAuthorizationFor(
		runtime.RuntimeParameters,
		"workflow",
		strings.TrimSpace(binding.AgentID),
		strings.TrimSpace(binding.BindingID),
		"execute",
	); ok {
		scope.AccountID = authorization.BoundByAccountID
	}
	return scope
}

func workflowScopesForBindings(runtime *tools.ToolRuntime, fallback workflowScope, bindings []workflowBinding) []workflowScope {
	result := make([]workflowScope, 0, len(bindings)+1)
	seen := make(map[string]struct{}, len(bindings)+1)
	for _, binding := range bindings {
		scope := workflowScopeForBinding(runtime, fallback, binding)
		if strings.TrimSpace(scope.AccountID) == "" {
			continue
		}
		if _, ok := seen[scope.AccountID]; ok {
			continue
		}
		seen[scope.AccountID] = struct{}{}
		result = append(result, scope)
	}
	if len(result) == 0 {
		result = append(result, fallback)
	}
	return result
}

type workflowBinding struct {
	BindingID       string               `json:"binding_id"`
	Label           string               `json:"label"`
	Description     string               `json:"description,omitempty"`
	AgentID         string               `json:"agent_id"`
	WorkflowID      string               `json:"workflow_id"`
	AgentType       string               `json:"agent_type,omitempty"`
	VersionStrategy string               `json:"version_strategy"`
	VersionUUID     string               `json:"version_uuid,omitempty"`
	TimeoutSeconds  int                  `json:"timeout_seconds,omitempty"`
	StartInputs     []workflowStartInput `json:"start_inputs,omitempty"`
	RequiredInputs  []string             `json:"required_inputs,omitempty"`
	DefaultInputKey string               `json:"default_input_key,omitempty"`
}

type workflowStartInput struct {
	Variable            string      `json:"variable"`
	Label               string      `json:"label,omitempty"`
	Type                string      `json:"type,omitempty"`
	Required            bool        `json:"required,omitempty"`
	Default             interface{} `json:"default,omitempty"`
	DefaultDateTimeMode string      `json:"default_datetime_mode,omitempty"`
}

func workflowBindingsFromRuntime(runtime *tools.ToolRuntime) ([]workflowBinding, error) {
	raw, ok := runtime.RuntimeParameters["workflow_bindings"]
	if !ok || raw == nil {
		return []workflowBinding{}, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow bindings: %w", err)
	}
	var parsed []workflowBinding
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("invalid workflow bindings: %w", err)
	}
	out := make([]workflowBinding, 0, len(parsed))
	seen := map[string]struct{}{}
	for _, binding := range parsed {
		binding.BindingID = strings.TrimSpace(binding.BindingID)
		binding.AgentID = strings.TrimSpace(binding.AgentID)
		binding.WorkflowID = strings.TrimSpace(binding.WorkflowID)
		binding.AgentType = strings.TrimSpace(binding.AgentType)
		binding.VersionStrategy = strings.TrimSpace(binding.VersionStrategy)
		if binding.VersionStrategy == "" {
			binding.VersionStrategy = automationaction.WorkflowVersionStrategyLatestPublished
		}
		binding.VersionUUID = strings.TrimSpace(binding.VersionUUID)
		binding.StartInputs = normalizeWorkflowStartInputs(binding.StartInputs)
		binding.RequiredInputs = normalizeWorkflowRequiredInputs(binding.RequiredInputs, binding.StartInputs)
		binding.DefaultInputKey = normalizeWorkflowDefaultInputKey(binding.DefaultInputKey, binding.StartInputs)
		if binding.BindingID == "" || binding.AgentID == "" || binding.WorkflowID == "" {
			continue
		}
		if _, ok := seen[binding.BindingID]; ok {
			continue
		}
		seen[binding.BindingID] = struct{}{}
		out = append(out, binding)
	}
	return out, nil
}

func workflowBindingList(bindings []workflowBinding) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(bindings))
	for _, binding := range bindings {
		defaultInputKey := bindingDefaultInputKey(binding)
		requiredInputs := bindingRequiredInputs(binding)
		item := map[string]interface{}{
			"binding_id":       binding.BindingID,
			"label":            binding.Label,
			"description":      binding.Description,
			"agent_type":       binding.AgentType,
			"version_strategy": binding.VersionStrategy,
			"timeout_seconds":  normalizeTimeoutSeconds(binding.TimeoutSeconds),
			"input_schema":     workflowInputSchema(binding),
			"required_inputs":  requiredInputs,
			"start_inputs":     binding.StartInputs,
		}
		if defaultInputKey != "" {
			item["default_input_key"] = defaultInputKey
		}
		out = append(out, item)
	}
	return out
}

func findWorkflowBinding(bindings []workflowBinding, bindingID string) (workflowBinding, bool) {
	bindingID = strings.TrimSpace(bindingID)
	for _, binding := range bindings {
		if strings.TrimSpace(binding.BindingID) == bindingID {
			return binding, true
		}
	}
	return workflowBinding{}, false
}

func inputMap(params map[string]interface{}, key string) (map[string]interface{}, error) {
	value, ok := params[key]
	if !ok || value == nil {
		return map[string]interface{}{}, nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for itemKey, itemValue := range typed {
			out[itemKey] = itemValue
		}
		return out, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return map[string]interface{}{}, nil
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
			return nil, fmt.Errorf("%s must be an object or JSON object string", key)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an object", key)
	}
}

func normalizeWorkflowInputs(inputs map[string]interface{}, binding workflowBinding) (map[string]interface{}, error) {
	if inputs == nil {
		inputs = map[string]interface{}{}
	}
	normalized := make(map[string]interface{}, len(inputs)+2)
	for key, value := range inputs {
		normalized[key] = value
	}
	if isConversationalWorkflowBinding(binding) {
		query := cleanOutputText(inputs[defaultInputKey])
		if query == "" {
			query = cleanOutputText(inputs["sys.query"])
		}
		if query == "" {
			return nil, fmt.Errorf("workflow inputs.%s is required; retry with inputs.%s set to the user's current request", defaultInputKey, defaultInputKey)
		}
		normalized[defaultInputKey] = query
		normalized["sys.query"] = query
		return normalized, nil
	}

	// query is a conversational-workflow transport input, not an implicit task
	// workflow start variable. A task workflow may still declare a normal start
	// variable named query; otherwise discard legacy callers' transport fields.
	if !workflowStartInputExists(binding.StartInputs, defaultInputKey) {
		delete(normalized, defaultInputKey)
	}
	delete(normalized, "sys.query")
	missing := missingWorkflowInputs(normalized, bindingRequiredInputs(binding))
	if len(missing) > 0 {
		if len(missing) == 1 {
			return nil, fmt.Errorf("workflow inputs.%s is required; retry with inputs.%s set to the user's current task input", missing[0], missing[0])
		}
		return nil, fmt.Errorf("workflow start inputs are missing required fields: %s; retry with inputs matching the binding's required_inputs from available_workflows or list_agent_workflows", strings.Join(missing, ", "))
	}
	return normalized, nil
}

func injectWorkflowContext(inputs map[string]interface{}, runtime *tools.ToolRuntime) {
	if inputs == nil || runtime == nil || runtime.RuntimeParameters == nil {
		return
	}
	if _, exists := inputs["sys.conversation_history"]; exists {
		return
	}
	contextMap, ok := runtime.RuntimeParameters["workflow_context"].(map[string]interface{})
	if !ok {
		return
	}
	history, ok := contextMap["conversation_history"]
	if !ok || history == nil {
		return
	}
	inputs["sys.conversation_history"] = history
}

func normalizeWorkflowStartInputs(inputs []workflowStartInput) []workflowStartInput {
	out := make([]workflowStartInput, 0, len(inputs))
	seen := map[string]struct{}{}
	for _, input := range inputs {
		variable := strings.TrimSpace(input.Variable)
		if variable == "" {
			continue
		}
		if _, exists := seen[variable]; exists {
			continue
		}
		seen[variable] = struct{}{}
		out = append(out, workflowStartInput{
			Variable:            variable,
			Label:               strings.TrimSpace(input.Label),
			Type:                strings.TrimSpace(input.Type),
			Required:            input.Required,
			Default:             input.Default,
			DefaultDateTimeMode: strings.TrimSpace(input.DefaultDateTimeMode),
		})
	}
	return out
}

func normalizeWorkflowRequiredInputs(required []string, startInputs []workflowStartInput) []string {
	if len(required) == 0 {
		out := make([]string, 0, len(startInputs))
		for _, input := range startInputs {
			if input.Required && strings.TrimSpace(input.Variable) != "" {
				out = append(out, strings.TrimSpace(input.Variable))
			}
		}
		return out
	}
	allowed := map[string]struct{}{}
	for _, input := range startInputs {
		if variable := strings.TrimSpace(input.Variable); variable != "" {
			allowed[variable] = struct{}{}
		}
	}
	out := make([]string, 0, len(required))
	seen := map[string]struct{}{}
	for _, item := range required {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[item]; !ok {
				continue
			}
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeWorkflowDefaultInputKey(key string, startInputs []workflowStartInput) string {
	key = strings.TrimSpace(key)
	if key != "" && workflowStartInputExists(startInputs, key) {
		return key
	}
	required := normalizeWorkflowRequiredInputs(nil, startInputs)
	if len(required) == 1 {
		return required[0]
	}
	if workflowStartInputExists(startInputs, defaultInputKey) {
		return defaultInputKey
	}
	if len(startInputs) == 1 {
		return strings.TrimSpace(startInputs[0].Variable)
	}
	return ""
}

func bindingRequiredInputs(binding workflowBinding) []string {
	if isConversationalWorkflowBinding(binding) {
		return []string{defaultInputKey}
	}
	required := normalizeWorkflowRequiredInputs(binding.RequiredInputs, binding.StartInputs)
	if len(required) > 0 {
		return required
	}
	return []string{}
}

func bindingDefaultInputKey(binding workflowBinding) string {
	if isConversationalWorkflowBinding(binding) {
		return defaultInputKey
	}
	key := normalizeWorkflowDefaultInputKey(binding.DefaultInputKey, binding.StartInputs)
	return key
}

func isConversationalWorkflowBinding(binding workflowBinding) bool {
	return strings.EqualFold(strings.TrimSpace(binding.AgentType), "CONVERSATIONAL_WORKFLOW")
}

func workflowStartInputExists(inputs []workflowStartInput, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, input := range inputs {
		if strings.TrimSpace(input.Variable) == key {
			return true
		}
	}
	return false
}

func missingWorkflowInputs(inputs map[string]interface{}, required []string) []string {
	missing := make([]string, 0, len(required))
	for _, key := range required {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if cleanOutputText(inputs[key]) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func workflowJSONSchemaType(inputType string) string {
	switch strings.ToLower(strings.TrimSpace(inputType)) {
	case "datetime", "date-time":
		return "string"
	case "number", "integer":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "object":
		return "object"
	case "array":
		return "array"
	default:
		return "string"
	}
}

func workflowInputSchema(binding workflowBinding) map[string]interface{} {
	if isConversationalWorkflowBinding(binding) {
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				defaultInputKey: map[string]interface{}{
					"type":        "string",
					"description": "The user's current request to pass into the conversational workflow.",
				},
			},
			"required":             []string{defaultInputKey},
			"additionalProperties": true,
		}
	}
	startInputs := binding.StartInputs
	if len(startInputs) > 0 {
		properties := map[string]interface{}{}
		for _, input := range startInputs {
			variable := strings.TrimSpace(input.Variable)
			if variable == "" {
				continue
			}
			description := strings.TrimSpace(input.Label)
			if description == "" {
				description = "Workflow start input."
			}
			properties[variable] = map[string]interface{}{
				"type":        workflowJSONSchemaType(input.Type),
				"description": description,
			}
		}
		return map[string]interface{}{
			"type":                 "object",
			"properties":           properties,
			"required":             bindingRequiredInputs(binding),
			"additionalProperties": true,
		}
	}
	return map[string]interface{}{
		"type":                 "object",
		"properties":           map[string]interface{}{},
		"required":             []string{},
		"additionalProperties": true,
	}
}

func normalizeTimeoutSeconds(value int) int {
	if value <= 0 {
		return defaultTimeoutSeconds
	}
	if value < minTimeoutSeconds {
		return minTimeoutSeconds
	}
	if value > maxTimeoutSeconds {
		return maxTimeoutSeconds
	}
	return value
}

func workflowToolParameters(kind string) []tools.ToolParameter {
	bindingID := stringParam("binding_id", "Binding ID", "Workflow binding ID from injected available_workflows, or from list_agent_workflows if the injected list is missing or ambiguous.", true)
	inputs := jsonParam("inputs", "Inputs", "Optional workflow input object. For task workflows, pass only the start variables declared by input_schema and required_inputs; omit this field when none are declared. For conversational workflows, pass the user's current request in inputs.query.", false)
	workflowRunID := stringParam("workflow_run_id", "Workflow run ID", "Workflow run ID returned by run_agent_workflow.", true)
	switch kind {
	case ToolListAgentWorkflows:
		return nil
	case ToolRunAgentWorkflow:
		return []tools.ToolParameter{bindingID, inputs}
	case ToolGetWorkflowRunStatus:
		return []tools.ToolParameter{workflowRunID}
	default:
		return nil
	}
}

func stringParam(name, label, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func jsonParam(name, label, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func workflowToolLabel(kind string) string {
	switch kind {
	case ToolListAgentWorkflows:
		return "List agent workflows"
	case ToolRunAgentWorkflow:
		return "Run agent workflow"
	case ToolGetWorkflowRunStatus:
		return "Get workflow run status"
	default:
		return kind
	}
}

func workflowToolDescription(kind string) string {
	switch kind {
	case ToolListAgentWorkflows:
		return "List workflows bound to the current Agent. Does not expose arbitrary workflow lookup."
	case ToolRunAgentWorkflow:
		return "Run a workflow bound to the current Agent by binding_id. For task workflows, follow the binding's input_schema exactly and use an empty inputs object when it declares no start inputs. For conversational workflows, pass the user's current request in inputs.query. Returns structured status, outputs, primary_output, workflow_run_id, and output_keys. After a succeeded run, the final answer must be based on primary_output or outputs; do not claim workflow output that is not present. If succeeded with no primary_output or outputs, say the workflow ran but returned no displayable output and include workflow_run_id. If a failed result has error_visibility=generic, tell the user only that the workflow run failed; do not include identifiers and do not state or infer a specific reason."
	case ToolGetWorkflowRunStatus:
		return "Query a previously started Agent-bound workflow run by workflow_run_id."
	default:
		return kind
	}
}

var _ tools.ToolProvider = (*Provider)(nil)
var _ tools.Tool = (*workflowTool)(nil)
