package intentrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	ToolRouteIntent = "route_intent"

	maxEvidenceItems     = 10
	maxMissingInfoItems  = 12
	maxAlternateIntents  = 5
	maxUploadedFileItems = 20
)

var intentIDPattern = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)+$`)

var allowedTaskTypes = map[string]struct{}{
	"general_qa":             {},
	"knowledge_retrieval":    {},
	"database_query":         {},
	"database_mutation":      {},
	"workflow_execution":     {},
	"file_generation":        {},
	"chart_generation":       {},
	"report_generation":      {},
	"schedule_planning":      {},
	"calculation":            {},
	"code_or_debugging":      {},
	"data_analysis":          {},
	"clarification_required": {},
	"unsupported":            {},
}

var allowedActions = map[string]struct{}{
	"answer_directly":    {},
	"call_skill":         {},
	"call_tool":          {},
	"run_workflow":       {},
	"query_database":     {},
	"mutate_database":    {},
	"retrieve_knowledge": {},
	"request_user_input": {},
	"reject_or_escalate": {},
}

var allowedRoutingHints = map[string]struct{}{
	"needs_context":             {},
	"uses_uploaded_files":       {},
	"requires_database":         {},
	"requires_knowledge_base":   {},
	"requires_workflow":         {},
	"requires_file_generation":  {},
	"requires_chart_generation": {},
	"requires_confirmation":     {},
	"is_high_impact":            {},
	"is_multi_intent":           {},
}

var knownSkillIDs = map[string]struct{}{
	"file-generator":        {},
	"chart-generator":       {},
	"work-report-generator": {},
	"schedule-planner":      {},
	"calculator":            {},
	"internal-knowledge":    {},
	"agent-knowledge":       {},
	"internal-database":     {},
	"agent-database":        {},
	"agent-workflow":        {},
}

// RouteIntentTool validates and normalizes an intent routing decision.
type RouteIntentTool struct {
	*builtin.BuiltinTool
}

// NewRouteIntentTool creates a route_intent tool.
func NewRouteIntentTool(tenantID string) *RouteIntentTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     ToolRouteIntent,
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US": "Route Intent",
			},
			Icon: "route",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US": "Validate and normalize a user intent routing result.",
			},
			LLM: "Validate and normalize a structured intent routing result. The model must classify the user's real intent first, then pass task_type, intent_id, confidence, recommended_action, evidence, missing_info, routing_hints, and normalized_request.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "user_input",
				Label:            tools.I18nText{"en_US": "User Input"},
				HumanDescription: tools.I18nText{"en_US": "The current user message being classified."},
				LLMDescription:   "The current user message being classified.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "intent_id",
				Label:            tools.I18nText{"en_US": "Intent ID"},
				HumanDescription: tools.I18nText{"en_US": "Stable dotted intent identifier."},
				LLMDescription:   "Stable dotted identifier such as file_generation.docx or database_query.filter_records.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "task_type",
				Label:            tools.I18nText{"en_US": "Task Type"},
				HumanDescription: tools.I18nText{"en_US": "Standard task type."},
				LLMDescription:   "Task type enum from the intent-router taxonomy.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "confidence",
				Label:            tools.I18nText{"en_US": "Confidence"},
				HumanDescription: tools.I18nText{"en_US": "Confidence from 0 to 1."},
				LLMDescription:   "Confidence from 0 to 1.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "recommended_action",
				Label:            tools.I18nText{"en_US": "Recommended Action"},
				HumanDescription: tools.I18nText{"en_US": "Recommended next action."},
				LLMDescription:   "Recommended action enum from the intent-router taxonomy.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "normalized_request",
				Label:            tools.I18nText{"en_US": "Normalized Request"},
				HumanDescription: tools.I18nText{"en_US": "Concise restatement of the user's real request."},
				LLMDescription:   "Concise restatement of what the user is actually asking.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "evidence",
				Label:            tools.I18nText{"en_US": "Evidence"},
				HumanDescription: tools.I18nText{"en_US": "Evidence supporting the classification."},
				LLMDescription:   "Array or JSON array string of short evidence strings grounded in the message or supplied context.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
			},
			{
				Name:             "missing_info",
				Label:            tools.I18nText{"en_US": "Missing Info"},
				HumanDescription: tools.I18nText{"en_US": "Missing fields that block reliable execution."},
				LLMDescription:   "Optional array or JSON array string. Each item requires field, reason, and question; options are concrete strings.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "routing_hints",
				Label:            tools.I18nText{"en_US": "Routing Hints"},
				HumanDescription: tools.I18nText{"en_US": "Boolean hints for downstream routing."},
				LLMDescription:   "Optional object or JSON object string of boolean routing hints.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
			{
				Name:             "uploaded_files",
				Label:            tools.I18nText{"en_US": "Uploaded Files"},
				HumanDescription: tools.I18nText{"en_US": "Uploaded file metadata relevant to routing."},
				LLMDescription:   "Optional array or JSON array string of file metadata objects. Do not include raw file content.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				SupportVariable:  true,
			},
		},
		OutputType: "json",
		Tags:       []string{"intent", "routing", "classification"},
	}
	return &RouteIntentTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *RouteIntentTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tenantID := t.GetTenantID()
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}
	fork := NewRouteIntentTool(tenantID)
	fork.BuiltinTool = fork.BuiltinTool.ForkToolRuntime(runtime)
	return fork
}

// Invoke validates and returns a stable intent routing payload.
func (t *RouteIntentTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID

	result, err := buildIntentRoute(toolParameters)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(result)}, nil
}

func buildIntentRoute(params map[string]interface{}) (map[string]interface{}, error) {
	userInput := requiredString(params, "user_input")
	if userInput == "" {
		return nil, fmt.Errorf("user_input is required")
	}

	intentID := normalizeToken(requiredString(params, "intent_id"))
	if intentID == "" {
		return nil, fmt.Errorf("intent_id is required")
	}
	if !intentIDPattern.MatchString(intentID) {
		return nil, fmt.Errorf("intent_id must be a dotted lowercase identifier")
	}

	taskType := normalizeToken(requiredString(params, "task_type"))
	if _, ok := allowedTaskTypes[taskType]; !ok {
		return nil, fmt.Errorf("unsupported task_type: %s", taskType)
	}

	confidence, err := requiredNumber(params, "confidence")
	if err != nil {
		return nil, err
	}
	if confidence < 0 || confidence > 1 {
		return nil, fmt.Errorf("confidence must be between 0 and 1")
	}

	action := normalizeToken(requiredString(params, "recommended_action"))
	if _, ok := allowedActions[action]; !ok {
		return nil, fmt.Errorf("unsupported recommended_action: %s", action)
	}

	normalizedRequest := requiredString(params, "normalized_request")
	if normalizedRequest == "" {
		return nil, fmt.Errorf("normalized_request is required")
	}

	evidence, err := requiredStringList(params, "evidence")
	if err != nil {
		return nil, err
	}
	if len(evidence) == 0 {
		return nil, fmt.Errorf("evidence must contain at least one item")
	}
	if len(evidence) > maxEvidenceItems {
		return nil, fmt.Errorf("evidence must contain no more than %d items", maxEvidenceItems)
	}

	missingInfo, err := optionalMissingInfo(params, "missing_info")
	if err != nil {
		return nil, err
	}
	if len(missingInfo) > maxMissingInfoItems {
		return nil, fmt.Errorf("missing_info must contain no more than %d items", maxMissingInfoItems)
	}
	if (action == "request_user_input" || taskType == "clarification_required") && len(missingInfo) == 0 {
		return nil, fmt.Errorf("missing_info is required when clarification is recommended")
	}

	routingHints, err := optionalRoutingHints(params, "routing_hints")
	if err != nil {
		return nil, err
	}

	recommendedSkillID := normalizeOptionalSkillID(optionalString(params, "recommended_skill_id"))
	if recommendedSkillID != "" {
		if _, ok := knownSkillIDs[recommendedSkillID]; !ok {
			return nil, fmt.Errorf("unsupported recommended_skill_id: %s", recommendedSkillID)
		}
	}
	if action == "call_skill" && recommendedSkillID == "" {
		return nil, fmt.Errorf("recommended_skill_id is required when recommended_action is call_skill")
	}

	recommendedToolName := normalizeToolName(optionalString(params, "recommended_tool_name"))
	if action == "call_tool" && recommendedToolName == "" {
		return nil, fmt.Errorf("recommended_tool_name is required when recommended_action is call_tool")
	}

	uploadedFiles, err := optionalUploadedFiles(params, "uploaded_files")
	if err != nil {
		return nil, err
	}
	if len(uploadedFiles) > maxUploadedFileItems {
		return nil, fmt.Errorf("uploaded_files must contain no more than %d items", maxUploadedFileItems)
	}

	alternateIntents, err := optionalAlternateIntents(params, "alternate_intents")
	if err != nil {
		return nil, err
	}
	if len(alternateIntents) > maxAlternateIntents {
		return nil, fmt.Errorf("alternate_intents must contain no more than %d items", maxAlternateIntents)
	}

	result := map[string]interface{}{
		"intent_id":             intentID,
		"task_type":             taskType,
		"subtype":               normalizeSubtype(optionalString(params, "subtype")),
		"confidence":            confidence,
		"recommended_action":    action,
		"recommended_skill_id":  recommendedSkillID,
		"recommended_tool_name": recommendedToolName,
		"routing_hints":         routingHints,
		"missing_info":          missingInfo,
		"evidence":              evidence,
		"normalized_request":    normalizedRequest,
		"uploaded_files":        uploadedFiles,
		"alternate_intents":     alternateIntents,
		"context_summary":       strings.TrimSpace(optionalString(params, "context_summary")),
	}

	copyOptionalString(result, params, "recommended_workflow_id")
	copyOptionalString(result, params, "recommended_database_id")
	if datasetIDs, err := optionalStringList(params, "recommended_dataset_ids"); err != nil {
		return nil, err
	} else {
		result["recommended_dataset_ids"] = datasetIDs
	}

	return result, nil
}

func requiredString(params map[string]interface{}, key string) string {
	return strings.TrimSpace(optionalString(params, key))
}

func optionalString(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	raw, ok := params[key]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func requiredNumber(params map[string]interface{}, key string) (float64, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch value := raw.(type) {
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}

func requiredStringList(params map[string]interface{}, key string) ([]string, error) {
	values, err := optionalStringList(params, key)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s is required", key)
	}
	return values, nil
}

func optionalStringList(params map[string]interface{}, key string) ([]string, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return []string{}, nil
	}
	items, err := rawList(raw, key)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for idx, item := range items {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text == "" {
			return nil, fmt.Errorf("%s[%d] must not be empty", key, idx)
		}
		out = append(out, text)
	}
	return out, nil
}

func optionalRoutingHints(params map[string]interface{}, key string) (map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return map[string]interface{}{}, nil
	}
	value, err := rawObject(raw, key)
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{}, len(value))
	for rawKey, rawValue := range value {
		hint := normalizeToken(rawKey)
		if _, ok := allowedRoutingHints[hint]; !ok {
			return nil, fmt.Errorf("unsupported routing_hints key: %s", rawKey)
		}
		boolValue, ok := rawValue.(bool)
		if !ok {
			return nil, fmt.Errorf("routing_hints.%s must be boolean", rawKey)
		}
		out[hint] = boolValue
	}
	return out, nil
}

func optionalMissingInfo(params map[string]interface{}, key string) ([]map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return []map[string]interface{}{}, nil
	}
	items, err := rawList(raw, key)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(items))
	for idx, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", key, idx)
		}
		field := normalizeToken(objectString(obj, "field"))
		reason := strings.TrimSpace(objectString(obj, "reason"))
		question := strings.TrimSpace(objectString(obj, "question"))
		if field == "" || reason == "" || question == "" {
			return nil, fmt.Errorf("%s[%d] requires field, reason, and question", key, idx)
		}
		normalized := map[string]interface{}{
			"field":    field,
			"reason":   reason,
			"question": question,
		}
		if _, ok := obj["options"]; ok {
			options, err := listFromObjectField(obj, "options", fmt.Sprintf("%s[%d].options", key, idx))
			if err != nil {
				return nil, err
			}
			if len(options) > 5 {
				return nil, fmt.Errorf("%s[%d].options must contain no more than 5 items", key, idx)
			}
			normalized["options"] = options
		}
		out = append(out, normalized)
	}
	return out, nil
}

func optionalUploadedFiles(params map[string]interface{}, key string) ([]map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return []map[string]interface{}{}, nil
	}
	items, err := rawList(raw, key)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(items))
	for idx, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", key, idx)
		}
		file := map[string]interface{}{}
		for _, field := range []string{"file_id", "filename", "mime_type", "format", "role", "summary"} {
			value := strings.TrimSpace(objectString(obj, field))
			if value != "" && value != "<nil>" {
				file[field] = value
			}
		}
		if len(file) == 0 {
			return nil, fmt.Errorf("%s[%d] must include file metadata", key, idx)
		}
		out = append(out, file)
	}
	return out, nil
}

func optionalAlternateIntents(params map[string]interface{}, key string) ([]map[string]interface{}, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return []map[string]interface{}{}, nil
	}
	items, err := rawList(raw, key)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(items))
	for idx, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", key, idx)
		}
		intentID := normalizeToken(objectString(obj, "intent_id"))
		taskType := normalizeToken(objectString(obj, "task_type"))
		if intentID == "" || !intentIDPattern.MatchString(intentID) {
			return nil, fmt.Errorf("%s[%d].intent_id must be a dotted lowercase identifier", key, idx)
		}
		if _, ok := allowedTaskTypes[taskType]; !ok {
			return nil, fmt.Errorf("%s[%d].task_type is unsupported", key, idx)
		}
		confidence, err := numberFromObjectField(obj, "confidence", fmt.Sprintf("%s[%d].confidence", key, idx))
		if err != nil {
			return nil, err
		}
		if confidence < 0 || confidence > 1 {
			return nil, fmt.Errorf("%s[%d].confidence must be between 0 and 1", key, idx)
		}
		normalized := map[string]interface{}{
			"intent_id":  intentID,
			"task_type":  taskType,
			"confidence": confidence,
		}
		action := normalizeToken(objectString(obj, "recommended_action"))
		if action != "" {
			if _, ok := allowedActions[action]; !ok {
				return nil, fmt.Errorf("%s[%d].recommended_action is unsupported", key, idx)
			}
			normalized["recommended_action"] = action
		}
		out = append(out, normalized)
	}
	return out, nil
}

func rawObject(raw interface{}, label string) (map[string]interface{}, error) {
	if value, ok := raw.(map[string]interface{}); ok {
		return value, nil
	}
	text, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be an object or JSON object string", label)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return map[string]interface{}{}, nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("%s must be an object or JSON object string", label)
	}
	return decoded, nil
}

func rawList(raw interface{}, label string) ([]interface{}, error) {
	switch value := raw.(type) {
	case []interface{}:
		return value, nil
	case []string:
		out := make([]interface{}, 0, len(value))
		for _, item := range value {
			out = append(out, item)
		}
		return out, nil
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return []interface{}{}, nil
		}
		var decoded []interface{}
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return nil, fmt.Errorf("%s must be an array or JSON array string", label)
		}
		return decoded, nil
	default:
		return nil, fmt.Errorf("%s must be an array or JSON array string", label)
	}
}

func listFromObjectField(obj map[string]interface{}, field string, label string) ([]string, error) {
	raw, ok := obj[field]
	if !ok || raw == nil {
		return []string{}, nil
	}
	items, err := rawList(raw, label)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for idx, item := range items {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text == "" {
			return nil, fmt.Errorf("%s[%d] must not be empty", label, idx)
		}
		out = append(out, text)
	}
	return out, nil
}

func numberFromObjectField(obj map[string]interface{}, field string, label string) (float64, error) {
	raw, ok := obj[field]
	if !ok || raw == nil {
		return 0, fmt.Errorf("%s is required", label)
	}
	switch value := raw.(type) {
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", label)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number", label)
	}
}

func objectString(obj map[string]interface{}, field string) string {
	raw, ok := obj[field]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func normalizeToken(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.ReplaceAll(text, "-", "_")
	text = strings.ReplaceAll(text, " ", "_")
	return text
}

func normalizeSubtype(value string) string {
	text := normalizeToken(value)
	if text == "" || text == "<nil>" {
		return "unknown"
	}
	return text
}

func normalizeOptionalSkillID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeToolName(value string) string {
	return normalizeToken(value)
}

func copyOptionalString(target map[string]interface{}, params map[string]interface{}, key string) {
	value := strings.TrimSpace(optionalString(params, key))
	if value != "" {
		target[key] = value
	}
}

var _ tools.Tool = (*RouteIntentTool)(nil)
