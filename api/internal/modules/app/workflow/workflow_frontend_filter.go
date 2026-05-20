package workflow

import (
	"fmt"
	"reflect"
	"strings"
)

// ---------------------------------------------------------------------------
// Frontend display filter for workflow node inputs / outputs.
//
// Design principles (inspired by Coze workflow UX):
//   - Show only business-relevant data; hide engine internals.
//   - Use a blacklist (remove known noise) instead of a whitelist so that
//     newly added fields are visible by default – safer & easier to maintain.
//   - NEVER mutate the original map; always return a new copy.
//   - Database records and in-memory engine state remain UNAFFECTED.
// ---------------------------------------------------------------------------

// frontendSystemInputBlacklist contains sys.* keys that should be removed from
// ALL node types by default.  Keys listed here are engine-internal identifiers
// that provide no value to end users in the execution detail panel.
var frontendSystemInputBlacklist = map[string]bool{
	"sys.user_id":              true,
	"sys.app_id":               true,
	"sys.agent_id":             true,
	"sys.workflow_id":          true,
	"sys.workspace_id":         true,
	"sys.tenant_id":            true,
	"sys.organization_id":      true,
	"sys.billing_subject_type": true,
	"sys.workflow_run_id":      true,
	"sys.workflow_type":        true,
	"sys.parent_message_id":    true,
	"sys.files":                true,
	// Non-prefixed versions for safety
	"organization_id":      true,
	"user_id":              true,
	"app_id":               true,
	"agent_id":             true,
	"workflow_id":          true,
	"workspace_id":         true,
	"tenant_id":            true,
	"billing_subject_type": true,
	"workflow_run_id":      true,
	"workflow_type":        true,
	"conversation_id":      true,
	"dialogue_count":       true,
	"metadata":             true,
	// model_config is an internal override mechanism
	"model_config": true,
	// conversation_params is internal invocation metadata
	"conversation_params": true,
	// Internal memory-variable reference format
	"#sys.query#":  true,
	"__is_success": true,
	"__reason":     true,
	"__usage":      true,
	"created_by":   true,
}

// frontendLLMKeepSysKeys are sys.* keys that should be RETAINED in LLM nodes
// because they give users meaningful context about the model invocation.
var frontendLLMKeepSysKeys = map[string]bool{
	"sys.query":                true,
	"sys.conversation_id":      true,
	"sys.dialogue_count":       true,
	"sys.conversation_history": true,
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// FilterFrontendInputs returns a filtered copy of inputs suitable for frontend
// display.  The original map is never modified.
func FilterFrontendInputs(nodeType string, inputs map[string]interface{}) map[string]interface{} {
	if len(inputs) == 0 {
		return inputs
	}

	filtered := make(map[string]interface{}, len(inputs))

	// Specialized logic for Document Extractor:
	// Only keep inputs that are actual file objects or file lists.
	// This hides other variables that might be in the pool but aren't being extracted.
	if nodeType == "document-extractor" {
		for key, value := range inputs {
			if isFileOrFileArray(value) {
				filtered[key] = value
			}
		}
		return filtered
	}

	// Specialized logic for if-else condition branch:
	// Only keep the cases field (user-configured branch rules).
	// Hide sys vars, pool vars, conditions, and logical_operator.
	if nodeType == "if-else" {
		if cases, exists := inputs["cases"]; exists {
			filtered["cases"] = cases
		}
		return filtered
	}

	if nodeType == "answer" {
		return filterConfiguredInputs(inputs, mapKeys("answer", "template", "content", "streaming"))
	}

	if nodeType == "iteration" {
		return filterIterationInputs(inputs)
	}

	if keepKeys, ok := frontendNodeInputKeepKeys[nodeType]; ok {
		return filterInputsByKeys(inputs, keepKeys)
	}

	if frontendConfiguredInputNodeTypes[nodeType] {
		return filterConfiguredInputs(inputs, nil)
	}

	for key, value := range inputs {
		// 1. Remove double-underscore internal keys (e.g. __edge_source_handle__)
		if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			continue
		}

		// 2. Per-node-type sys.* handling
		if isSysKey(key) {
			if shouldKeepSysKeyForNode(nodeType, key) {
				filtered[key] = filterSysValue(key, value)
			}
			continue
		}

		// 3. Global blacklist
		if frontendSystemInputBlacklist[key] {
			continue
		}

		filtered[frontendVariableDisplayKey(key)] = value
	}

	return filtered
}

var frontendConfiguredInputNodeTypes = map[string]bool{
	"llm":                 true,
	"end":                 true,
	"parameter-extractor": true,
	"knowledge-retrieval": true,
	"json-parser":         true,
	"variable-aggregator": true,
	"code":                true,
}

var frontendNodeInputKeepKeys = map[string]map[string]bool{
	"http-request":  mapKeys("url", "method", "header", "param", "auth"),
	"call-database": mapKeys("sql"),
	"sql-generator": mapKeys("prompt"),
	"image-gen":     mapKeys("prompt", "prompt_variables"),
	"loop": mapKeys(
		"loop_variables",
		"loop_count",
		"break_conditions",
	),
	"assigner": mapKeys("items"),
	"iteration": mapKeys(
		"iterator_selector",
		"iterator_value",
		"output_selector",
	),
}

func mapKeys(keys ...string) map[string]bool {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = true
	}
	return result
}

func filterInputsByKeys(inputs map[string]interface{}, keepKeys map[string]bool) map[string]interface{} {
	filtered := make(map[string]interface{}, len(keepKeys))
	for key, value := range inputs {
		if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			continue
		}
		if isSysKey(key) || frontendSystemInputBlacklist[key] {
			continue
		}
		if keepKeys[key] {
			filtered[key] = value
		}
	}
	return filtered
}

func filterIterationInputs(inputs map[string]interface{}) map[string]interface{} {
	rejectKeys := mapKeys(
		"iterator_selector",
		"iterator_value",
		"input_selector",
		"output_selector",
		"parallel_nums",
		"is_parallel",
		"start_node_id",
		"error_handle_mode",
	)
	filtered := make(map[string]interface{}, len(inputs))
	for key, value := range inputs {
		if rejectKeys[key] {
			continue
		}
		filtered[frontendVariableDisplayKey(key)] = value
	}
	return filtered
}

func filterConfiguredInputs(inputs map[string]interface{}, rejectKeys map[string]bool) map[string]interface{} {
	filtered := make(map[string]interface{}, len(inputs))
	for key, value := range inputs {
		if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			continue
		}
		if rejectKeys[key] {
			continue
		}
		filtered[frontendVariableDisplayKey(key)] = value
	}
	return filtered
}

func frontendVariableDisplayKey(key string) string {
	if strings.HasPrefix(key, "sys.") || !strings.Contains(key, ".") {
		return key
	}
	parts := strings.Split(key, ".")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return key
	}
	return last
}

// FilterFrontendOutputs returns a filtered copy of outputs suitable for
// frontend display.  The original map is never modified.
func FilterFrontendOutputs(nodeType string, outputs map[string]interface{}) map[string]interface{} {
	if len(outputs) == 0 {
		return outputs
	}

	if nodeType == "image-gen" {
		return filterOutputsByKeys(outputs, mapKeys("urls"))
	}

	if nodeType == "knowledge-retrieval" {
		return filterKnowledgeRetrievalOutputs(outputs)
	}

	filtered := make(map[string]interface{}, len(outputs))

	for key, value := range outputs {
		// 1. Remove double-underscore internal keys
		if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			continue
		}

		// 2. sys.* handling for outputs:
		// ONLY keep specific sys keys for the "start" node as requested.
		// For all other nodes, sys keys are removed from outputs.
		if isSysKey(key) {
			if nodeType == "start" && shouldKeepSysKeyForNode("start", key) {
				filtered[key] = filterSysValue(key, value)
			}
			continue
		}

		// 3. Global blacklist
		if frontendSystemInputBlacklist[key] {
			continue
		}
		// Parameter Extractor: Filter out internal status/usage fields from outputs
		if nodeType == "parameter-extractor" {
			if key == "__is_success" || key == "__reason" || key == "__usage" {
				continue
			}
		}

		// 4. Specialized logic for LLM node: filter finish_reason
		if nodeType == "llm" && key == "finish_reason" {
			continue
		}

		filtered[frontendVariableDisplayKey(key)] = value
	}

	return filtered
}

func filterOutputsByKeys(outputs map[string]interface{}, keepKeys map[string]bool) map[string]interface{} {
	filtered := make(map[string]interface{}, len(keepKeys))
	for key, value := range outputs {
		if keepKeys[key] {
			filtered[key] = value
		}
	}
	return filtered
}

func filterKnowledgeRetrievalOutputs(outputs map[string]interface{}) map[string]interface{} {
	filtered := make(map[string]interface{}, len(outputs))
	for key, value := range outputs {
		if key == "retriever_resources" {
			filtered[key] = filterRetrieverResources(value)
			continue
		}
		if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			continue
		}
		if isSysKey(key) || frontendSystemInputBlacklist[key] {
			continue
		}
		filtered[frontendVariableDisplayKey(key)] = value
	}
	return filtered
}

func filterRetrieverResources(value interface{}) interface{} {
	switch typed := value.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, filterRetrieverResource(item))
		}
		return result
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
			result := make([]interface{}, 0, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				result = append(result, filterRetrieverResource(rv.Index(i).Interface()))
			}
			return result
		}
		return value
	}
}

func filterRetrieverResource(value interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for _, key := range []string{"document_id", "document_name", "content", "score"} {
		if item, ok := getResourceField(value, key); ok {
			result[key] = item
		}
	}
	return result
}

func getResourceField(value interface{}, jsonKey string) (interface{}, bool) {
	if value == nil {
		return nil, false
	}
	if m, ok := value.(map[string]interface{}); ok {
		item, exists := m[jsonKey]
		return item, exists
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, false
	}
	fieldName := retrieverResourceFieldName(jsonKey)
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, false
	}
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return nil, false
		}
		return field.Elem().Interface(), true
	}
	return field.Interface(), true
}

func retrieverResourceFieldName(jsonKey string) string {
	switch jsonKey {
	case "document_id":
		return "DocumentID"
	case "document_name":
		return "DocumentName"
	case "content":
		return "Content"
	case "score":
		return "Score"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func isSysKey(key string) bool {
	return strings.HasPrefix(key, "sys.") || strings.HasPrefix(key, "#sys.")
}

// shouldKeepSysKeyForNode decides whether a sys.* key should be kept for a
// given node type.  The default is to REMOVE all sys.* keys unless the node
// type explicitly needs them.
func shouldKeepSysKeyForNode(nodeType, key string) bool {
	switch nodeType {
	case "llm":
		return frontendLLMKeepSysKeys[key]
	default:
		return false
	}
}

// filterSysValue applies special transformations on retained sys.* values.
// Currently it replaces large conversation_history blobs with a compact summary.
func filterSysValue(key string, value interface{}) interface{} {
	if key == "sys.conversation_history" {
		count := conversationHistoryCount(value)
		if count > 0 {
			return fmt.Sprintf("[%d messages]", count)
		}
		return "[0 messages]"
	}
	return value
}

// isFileOrFileArray checks if a value looks like a workflow File object
// or a slice of File objects.
func isFileOrFileArray(v interface{}) bool {
	if v == nil {
		return false
	}

	rv := reflect.ValueOf(v)
	// 1. Handle pointer
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}

	// 2. Handle slice/array
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		if rv.Len() == 0 {
			return false
		}
		// Check the first element
		return isFileOrFileArray(rv.Index(0).Interface())
	}

	// 3. Handle map
	if m, ok := v.(map[string]interface{}); ok {
		return isFileMap(m)
	}

	// 4. Handle struct (via reflection)
	if rv.Kind() == reflect.Struct {
		// Most workflow file structs have an ID or Filename field
		if f := rv.FieldByName("ID"); f.IsValid() {
			return true
		}
		if f := rv.FieldByName("Filename"); f.IsValid() {
			return true
		}
		// Check for zgi identity field if it's a struct with tags
		if f := rv.FieldByName("ZgiModelIdentity"); f.IsValid() {
			return true
		}
	}

	return false
}

func isFileMap(m map[string]interface{}) bool {
	// 1. Official ZGI file identity check
	if identity, ok := m["zgi_model_identity"].(string); ok {
		if identity == "__zgi__file__" {
			return true
		}
	}

	// 2. Fallback heuristic: A workflow file object usually has "id" or "upload_file_id"
	// and either "filename" or "mime_type" as a heuristic.
	_, hasID := m["id"]
	_, hasUploadID := m["upload_file_id"]
	_, hasName := m["filename"]
	_, hasMime := m["mime_type"]
	return (hasID || hasUploadID) && (hasName || hasMime || hasUploadID)
}
