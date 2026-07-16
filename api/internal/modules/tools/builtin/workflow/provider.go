package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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
	_ = conversationID
	_ = appID
	_ = messageID
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
		return t.runWorkflow(ctx, scope, params, bindings)
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

func (t *workflowTool) runWorkflow(ctx context.Context, scope workflowScope, params map[string]interface{}, bindings []workflowBinding) ([]tools.ToolInvokeMessage, error) {
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
	injectWorkflowContext(inputs, t.Runtime())
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
		Inputs:  inputs,
		Timeout: timeout,
	}
	if emitter := workflowevents.FromContext(ctx); emitter != nil {
		runReq.EventSink = func(event automationaction.WorkflowRunEvent) {
			emitter(workflowevents.Event{
				Type:    event.Type,
				Payload: event.Payload,
			})
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, runErr := runner.RunAutomationWorkflow(runCtx, runReq)
	if result == nil {
		if runErr != nil {
			return jsonMessages(failedWorkflowPayload("", "", "", runErr))
		}
		return jsonMessages(failedWorkflowPayload("", "", "", fmt.Errorf("workflow returned empty result")))
	}
	payload := workflowResultPayload(result, runErr, binding)
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
		return jsonMessages(workflowStatusPayload(result))
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("workflow run status is empty")
}

func workflowResultPayload(result *automationaction.WorkflowRunResult, runErr error, binding workflowBinding) map[string]interface{} {
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
	}
	if status == "pending_approval" {
		mergeApprovalFields(payload, result.Outputs)
	}
	if status == "pending_question" {
		mergeQuestionAnswerFields(payload, result.Outputs)
	}
	if runErr != nil {
		payload["status"] = "failed"
		payload["error"] = strings.TrimSpace(runErr.Error())
	}
	return payload
}

func workflowStatusPayload(result *automationaction.WorkflowRunStatusResult) map[string]interface{} {
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
	}
	if status == "pending_question" {
		mergeQuestionAnswerFields(payload, result.Outputs)
	}
	if strings.TrimSpace(result.Error) != "" {
		payload["error"] = strings.TrimSpace(result.Error)
	}
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
		out = append(out, map[string]interface{}{
			"binding_id":        binding.BindingID,
			"label":             binding.Label,
			"description":       binding.Description,
			"agent_type":        binding.AgentType,
			"version_strategy":  binding.VersionStrategy,
			"timeout_seconds":   normalizeTimeoutSeconds(binding.TimeoutSeconds),
			"input_schema":      workflowInputSchema(binding),
			"required_inputs":   requiredInputs,
			"default_input_key": defaultInputKey,
			"start_inputs":      binding.StartInputs,
		})
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
	query := cleanOutputText(inputs[defaultInputKey])
	if query == "" {
		query = cleanOutputText(inputs["sys.query"])
	}
	normalized := make(map[string]interface{}, len(inputs)+2)
	for key, value := range inputs {
		normalized[key] = value
	}
	startInputs := binding.StartInputs
	if len(startInputs) == 0 || strings.EqualFold(strings.TrimSpace(binding.AgentType), "CONVERSATIONAL_WORKFLOW") {
		if query == "" {
			return nil, fmt.Errorf("workflow inputs.%s is required; retry with inputs.%s set to the user's current request", defaultInputKey, defaultInputKey)
		}
		normalized[defaultInputKey] = query
		normalized["sys.query"] = query
		return normalized, nil
	}
	if query != "" {
		normalized["sys.query"] = query
		if !workflowStartInputExists(startInputs, defaultInputKey) {
			delete(normalized, defaultInputKey)
		}
	}
	defaultKey := bindingDefaultInputKey(binding)
	if defaultKey != "" && cleanOutputText(normalized[defaultKey]) == "" && query != "" {
		normalized[defaultKey] = query
	}
	missing := missingWorkflowInputs(normalized, bindingRequiredInputs(binding))
	if len(missing) > 0 {
		if query == "" && len(missing) == 1 {
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
	if len(startInputs) == 0 {
		return defaultInputKey
	}
	return ""
}

func bindingRequiredInputs(binding workflowBinding) []string {
	required := normalizeWorkflowRequiredInputs(binding.RequiredInputs, binding.StartInputs)
	if len(required) > 0 {
		return required
	}
	if len(binding.StartInputs) == 0 {
		return []string{defaultInputKey}
	}
	return []string{}
}

func bindingDefaultInputKey(binding workflowBinding) string {
	key := normalizeWorkflowDefaultInputKey(binding.DefaultInputKey, binding.StartInputs)
	if key != "" {
		return key
	}
	return defaultInputKey
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
		"type": "object",
		"properties": map[string]interface{}{
			defaultInputKey: map[string]interface{}{
				"type":        "string",
				"description": "The user's current request or instruction to pass into the workflow.",
			},
		},
		"required":             []string{defaultInputKey},
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
	inputs := jsonParam("inputs", "Inputs", "Workflow input object. Use the binding's input_schema and required_inputs from available_workflows or list_agent_workflows. For single-input workflows, inputs.query may be used as the user's current request and will be mapped to the start input and sys.query.", true)
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
		return "Run a workflow bound to the current Agent by binding_id. Pass the user's current request in inputs.query. Returns structured status, outputs, primary_output, workflow_run_id, and output_keys. After a succeeded run, the final answer must be based on primary_output or outputs; do not claim workflow output that is not present. If succeeded with no primary_output or outputs, say the workflow ran but returned no displayable output and include workflow_run_id."
	case ToolGetWorkflowRunStatus:
		return "Query a previously started Agent-bound workflow run by workflow_run_id."
	default:
		return kind
	}
}

var _ tools.ToolProvider = (*Provider)(nil)
var _ tools.Tool = (*workflowTool)(nil)
