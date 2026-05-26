package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

const workflowRecognitionContextMaxChars = 8000

type WorkflowContextProvider interface {
	WorkflowRecognitionContext(ctx context.Context, agentID string) string
}

type WorkflowServiceContextProvider struct {
	WorkflowService interfaces.WorkflowService
}

func (p WorkflowServiceContextProvider) WorkflowRecognitionContext(ctx context.Context, agentID string) string {
	if p.WorkflowService == nil {
		return ""
	}
	draft, err := p.WorkflowService.GetDraftWorkflow(ctx, agentID, true)
	if err != nil || draft == nil {
		return ""
	}
	return buildWorkflowRecognitionContext(draft)
}

func buildWorkflowRecognitionContext(draft any) string {
	workflow := normalizeWorkflowDraftMap(draft)
	graph := graphMapFromValue(workflow["graph"])
	nodes := anySlice(graph["nodes"])
	edges := anySlice(graph["edges"])
	if len(nodes) == 0 {
		return ""
	}

	lines := []string{"Workflow structure summary:"}
	for index, rawNode := range nodes {
		node := mapValue(rawNode)
		data := mapValue(node["data"])
		id := stringValue(node["id"])
		nodeType := firstNonEmptyString(stringValue(data["type"]), stringValue(node["type"]))
		title := firstNonEmptyString(stringValue(data["title"]), id)
		desc := stringValue(data["desc"])
		if desc == "" {
			desc = stringValue(data["description"])
		}
		header := fmt.Sprintf("%d. [%s] %s", index+1, nodeType, title)
		if id != "" {
			header += fmt.Sprintf(" (id: %s)", id)
		}
		lines = append(lines, header)
		if desc != "" {
			lines = append(lines, "   description: "+truncateForPrompt(desc, 500))
		}
		lines = append(lines, summarizeNodeData(nodeType, data)...)
	}

	edgeLines := summarizeEdges(edges)
	if len(edgeLines) > 0 {
		lines = append(lines, "Edges:")
		lines = append(lines, edgeLines...)
	}

	return truncateForPrompt(strings.Join(lines, "\n"), workflowRecognitionContextMaxChars)
}

func normalizeWorkflowDraftMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if mapped := mapValue(value); len(mapped) > 0 {
		return mapped
	}
	if graphProvider, ok := value.(interface{ GetGraphDict() map[string]interface{} }); ok {
		return map[string]any{"graph": graphProvider.GetGraphDict()}
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() == reflect.Ptr {
		if reflected.IsNil() {
			return nil
		}
		reflected = reflected.Elem()
	}
	if reflected.Kind() != reflect.Struct {
		return nil
	}
	field := reflected.FieldByName("Graph")
	if !field.IsValid() {
		return nil
	}
	graphText := ""
	switch field.Kind() {
	case reflect.String:
		graphText = field.String()
	case reflect.Ptr:
		if !field.IsNil() && field.Elem().Kind() == reflect.String {
			graphText = field.Elem().String()
		}
	}
	if strings.TrimSpace(graphText) == "" {
		return nil
	}
	var graph map[string]any
	if err := json.Unmarshal([]byte(graphText), &graph); err != nil {
		return nil
	}
	return map[string]any{"graph": graph}
}

func graphMapFromValue(value any) map[string]any {
	if text := stringValue(value); text != "" {
		var graph map[string]any
		if err := json.Unmarshal([]byte(text), &graph); err == nil {
			return graph
		}
	}
	return mapValue(value)
}

func summarizeNodeData(nodeType string, data map[string]any) []string {
	switch nodeType {
	case "start":
		return summarizeStartNode(data)
	case "llm":
		return summarizeLLMNode(data)
	case "if-else":
		return summarizeIfElseNode(data)
	case "question-answer":
		return summarizeQuestionAnswerNode(data)
	case "answer":
		return summarizeTextField(data, "answer", "answer")
	case "knowledge-retrieval":
		return summarizeTextField(data, "query_variable_selector", "query selector")
	case "http-request":
		return summarizeHTTPRequestNode(data)
	case "tool":
		return summarizeToolNode(data)
	case "call-database", "sql-generator":
		return summarizeDatabaseNode(data)
	default:
		return summarizeGenericNode(data)
	}
}

func summarizeStartNode(data map[string]any) []string {
	var lines []string
	for _, raw := range anySlice(data["variables"]) {
		item := mapValue(raw)
		name := firstNonEmptyString(stringValue(item["variable"]), stringValue(item["name"]))
		if name == "" {
			continue
		}
		parts := []string{name}
		if typ := stringValue(item["type"]); typ != "" {
			parts = append(parts, "type="+typ)
		}
		if label := stringValue(item["label"]); label != "" {
			parts = append(parts, "label="+label)
		}
		if desc := stringValue(item["description"]); desc != "" {
			parts = append(parts, "description="+desc)
		}
		lines = append(lines, "   input: "+truncateForPrompt(strings.Join(parts, ", "), 500))
	}
	return lines
}

func summarizeLLMNode(data map[string]any) []string {
	var lines []string
	for _, raw := range anySlice(data["prompt_template"]) {
		item := mapValue(raw)
		role := firstNonEmptyString(stringValue(item["role"]), "prompt")
		text := stringValue(item["text"])
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("   %s prompt: %s", role, truncateForPrompt(text, 1000)))
	}
	ref := mapValue(data["prompt_reference"])
	if name := stringValue(ref["prompt_name"]); name != "" {
		lines = append(lines, "   managed prompt: "+name)
	}
	return lines
}

func summarizeIfElseNode(data map[string]any) []string {
	var lines []string
	for index, rawCase := range anySlice(data["cases"]) {
		item := mapValue(rawCase)
		caseID := firstNonEmptyString(stringValue(item["case_id"]), fmt.Sprintf("case-%d", index+1))
		operator := firstNonEmptyString(stringValue(item["logical_operator"]), "and")
		conditions := summarizeConditions(anySlice(item["conditions"]))
		if len(conditions) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("   branch %s (%s): %s", caseID, operator, strings.Join(conditions, "; ")))
	}
	return lines
}

func summarizeQuestionAnswerNode(data map[string]any) []string {
	var lines []string
	if question := stringValue(data["question"]); question != "" {
		lines = append(lines, "   question: "+truncateForPrompt(question, 800))
	}
	if instruction := stringValue(data["completion_instruction"]); instruction != "" {
		lines = append(lines, "   completion instruction: "+truncateForPrompt(instruction, 800))
	}
	for _, raw := range anySlice(data["choices"]) {
		item := mapValue(raw)
		label := firstNonEmptyString(stringValue(item["label"]), stringValue(item["value"]))
		if label != "" {
			lines = append(lines, "   choice: "+truncateForPrompt(label, 300))
		}
	}
	for _, raw := range anySlice(data["extraction_fields"]) {
		item := mapValue(raw)
		name := stringValue(item["name"])
		if name == "" {
			continue
		}
		desc := stringValue(item["description"])
		if desc != "" {
			lines = append(lines, fmt.Sprintf("   extraction: %s (%s)", name, truncateForPrompt(desc, 300)))
		} else {
			lines = append(lines, "   extraction: "+name)
		}
	}
	return lines
}

func summarizeHTTPRequestNode(data map[string]any) []string {
	var lines []string
	method := stringValue(data["method"])
	url := sanitizeURLForPrompt(stringValue(data["url"]))
	if method != "" || url != "" {
		lines = append(lines, "   request: "+strings.TrimSpace(method+" "+url))
	}
	return lines
}

func summarizeToolNode(data map[string]any) []string {
	var lines []string
	for _, key := range []string{"provider_name", "tool_name", "tool_label"} {
		if value := stringValue(data[key]); value != "" {
			lines = append(lines, fmt.Sprintf("   %s: %s", key, truncateForPrompt(value, 300)))
		}
	}
	return lines
}

func summarizeDatabaseNode(data map[string]any) []string {
	lines := summarizeTextField(data, "prompt", "prompt")
	for _, key := range []string{"sql", "query"} {
		if value := stringValue(data[key]); value != "" {
			lines = append(lines, fmt.Sprintf("   %s: %s", key, truncateForPrompt(value, 800)))
		}
	}
	return lines
}

func summarizeGenericNode(data map[string]any) []string {
	keys := []string{"instruction", "prompt", "query", "question", "content", "template"}
	var lines []string
	for _, key := range keys {
		if value := stringValue(data[key]); value != "" {
			lines = append(lines, fmt.Sprintf("   %s: %s", key, truncateForPrompt(value, 500)))
		}
	}
	return lines
}

func summarizeTextField(data map[string]any, key, label string) []string {
	if value := stringValue(data[key]); value != "" {
		return []string{fmt.Sprintf("   %s: %s", label, truncateForPrompt(value, 800))}
	}
	return nil
}

func summarizeConditions(conditions []any) []string {
	items := make([]string, 0, len(conditions))
	for _, raw := range conditions {
		condition := mapValue(raw)
		selector := stringSlice(condition["variable_selector"])
		key := firstNonEmptyString(strings.Join(selector, "."), stringValue(condition["key"]))
		operator := stringValue(condition["comparison_operator"])
		value := conditionValueString(condition["value"])
		text := strings.TrimSpace(strings.Join([]string{key, operator, value}, " "))
		if text != "" {
			items = append(items, truncateForPrompt(text, 300))
		}
	}
	return items
}

func summarizeEdges(edges []any) []string {
	lines := make([]string, 0, len(edges))
	for _, raw := range edges {
		edge := mapValue(raw)
		source := stringValue(edge["source"])
		target := stringValue(edge["target"])
		if source == "" || target == "" {
			continue
		}
		handle := firstNonEmptyString(stringValue(edge["sourceHandle"]), stringValue(edge["source_handle"]))
		if handle != "" {
			lines = append(lines, fmt.Sprintf("   %s[%s] -> %s", source, handle, target))
		} else {
			lines = append(lines, fmt.Sprintf("   %s -> %s", source, target))
		}
	}
	return lines
}

func mapValue(value any, keys ...string) map[string]any {
	if len(keys) > 0 {
		current := value
		for _, key := range keys {
			current = mapValue(current)[key]
		}
		return mapValue(current)
	}
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func anySlice(value any) []any {
	if value == nil {
		return nil
	}
	if items, ok := value.([]any); ok {
		return items
	}
	if items, ok := value.([]interface{}); ok {
		return items
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return nil
	}
	out := make([]any, 0, reflected.Len())
	for i := 0; i < reflected.Len(); i++ {
		out = append(out, reflected.Index(i).Interface())
	}
	return out
}

func stringSlice(value any) []string {
	items := anySlice(value)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func conditionValueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				values = append(values, text)
			}
		}
		return strings.Join(values, ",")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateForPrompt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func sanitizeURLForPrompt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if index := strings.Index(value, "?"); index >= 0 {
		value = value[:index] + "?..."
	}
	return truncateForPrompt(value, 500)
}
