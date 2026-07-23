package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	caseModeInputKey           = "__case_mode"
	expectedChecksInputKey     = "__expected_checks"
	turnExpectationInputKey    = "__turn_expectation"
	turnChecksInputKey         = "__turn_checks"
	conversationChecksInputKey = "__conversation_checks"
	taskCheckInferTimeout      = 60 * time.Second
)

type TaskExpectedChecks struct {
	MustVisitNodes []string                     `json:"must_visit_nodes,omitempty"`
	MustCallTools  []string                     `json:"must_call_tools,omitempty"`
	OutputContains []string                     `json:"output_contains,omitempty"`
	MaxLatencyMS   int                          `json:"max_latency_ms,omitempty"`
	Conditions     []TaskExpectedCheckCondition `json:"conditions,omitempty"`
}

type TaskExpectedCheckCondition struct {
	ID          string   `json:"id,omitempty"`
	Type        string   `json:"type,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	TargetID    string   `json:"target_id,omitempty"`
	TargetLabel string   `json:"target_label,omitempty"`
	TargetType  string   `json:"target_type,omitempty"`
	Values      []string `json:"values,omitempty"`
	MatchMode   string   `json:"match_mode,omitempty"`
	ValueMS     int      `json:"value_ms,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type TaskExpectedCheckInferInput struct {
	WorkflowContext string
	Content         string
	ExpectedResult  string
	Variables       JSONMap
	Attachments     []CaseAttachment
}

type LLMTaskExpectedCheckInferer struct {
	Client      llmclient.LLMClient
	WorkspaceID string
	AccountID   string
	AgentID     string
}

func (i *LLMTaskExpectedCheckInferer) Infer(ctx context.Context, input TaskExpectedCheckInferInput) (*TaskExpectedChecks, error) {
	if i == nil || i.Client == nil {
		return nil, fmt.Errorf("llm task expected check inferer is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, taskCheckInferTimeout)
	defer cancel()

	temperature := 0.1
	maxTokens := 1200
	req := &adapter.ChatRequest{
		Messages: []adapter.Message{
			{Role: "system", Content: "You generate advanced checks for task-workflow batch tests. Return JSON only. Do not include markdown or explanations."},
			{Role: "user", Content: buildTaskExpectedCheckPrompt(input)},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	}
	resp, err := i.Client.AppChat(timeoutCtx, &llmclient.AppContext{
		WorkspaceID:        i.WorkspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace,
		AppID:              i.AgentID,
		AppType:            "agent",
		AccountID:          i.AccountID,
	}, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm task expected check inferer returned empty response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("llm task expected check inferer returned empty content")
	}
	return parseTaskExpectedChecks(content)
}

func buildTaskExpectedCheckPrompt(input TaskExpectedCheckInferInput) string {
	variablesJSON, _ := json.Marshal(input.Variables)
	attachments := make([]string, 0, len(input.Attachments))
	for _, attachment := range input.Attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = strings.TrimSpace(attachment.UploadFileID)
		}
		if name == "" {
			name = strings.TrimSpace(attachment.URL)
		}
		if name != "" {
			attachments = append(attachments, name)
		}
	}
	attachmentsJSON, _ := json.Marshal(attachments)
	return buildTaskExpectedCheckPromptV2(input, string(variablesJSON), string(attachmentsJSON))
}

func buildTaskExpectedCheckPromptV2(input TaskExpectedCheckInferInput, variablesJSON, attachmentsJSON string) string {
	return fmt.Sprintf(`Generate optional advanced checks for this task-workflow test case.
Return JSON only. Prefer the new "conditions" array, and keep legacy fields only for compatibility.

Output shape:
{
  "conditions": [
    {"type":"node","operator":"input_contains","target_id":"node id or name","target_label":"node display name","target_type":"start","values":["expected input field"],"match_mode":"semantic","severity":"normal","source":"ai_generated"},
    {"type":"node","operator":"output_contains","target_id":"node id or name","target_label":"node display name","target_type":"end","values":["expected output field"],"match_mode":"semantic","severity":"critical","source":"ai_generated"},
    {"type":"capability","operator":"called","target_id":"tool or node id","target_label":"tool or capability name","severity":"normal","source":"ai_generated"},
    {"type":"output_contains","operator":"contains","values":["final output field"],"match_mode":"semantic","severity":"critical","source":"ai_generated"},
    {"type":"latency","operator":"lte","value_ms":30000,"severity":"hint","source":"ai_generated"}
  ],
  "must_visit_nodes":["legacy node names"],
  "must_call_tools":["legacy tool names"],
  "output_contains":["legacy final output fields"],
  "max_latency_ms":30000
}

Rules:
1. For start nodes, generate input checks only: input_contains or input_not_contains. Do not use visited as the main start-node check.
2. For end or answer nodes, prefer output checks: output_contains or output_not_contains.
3. For normal intermediate nodes, use visited/not_visited only when path verification matters; use input/output checks when node-level data matters.
4. Do not generate checks for note/comment/display-only nodes.
5. Do not invent node ids, node names, or tools. Use only names or ids visible in the workflow context. If uncertain, omit node/tool checks.
6. Task workflows are function-like single executions. Do not generate checks that require follow-up questions, user contact, human escalation, or hidden context unless the expected output schema explicitly contains those fields.
7. Output checks must be concrete business fields or facts that are present in the task input or attached files. For missing input fields, generate checks for explicit missing-field markers such as missing/unknown/to be supplied/placeholders, not checks that require invented dates, parties, amounts, or contacts.
8. Do not use meta labels as values, such as "product name is provided", "责任划分说明已包含", or "字段已提供". Use the concrete value when it exists, for example "智能手表"; if only the field is known to be absent, use a missing-field check such as "产品名称缺失".
9. For state conclusions, include the state value and concrete evidence when possible, for example "部分验收", "拒收30件", "剩余50件未交付".
10. Generate a small diagnostic set, usually 3-5 conditions. Do not turn the whole expected_result into many output_contains checks.
11. Use severity critical only for truly blocking failures: core objective not completed, fabricated key facts, technical parse failure, or a required key node/tool missing. Use normal for supplemental fields and path/capability checks; use hint for formatting, wording, latency, and nice-to-have completeness.
12. If the overall task direction is likely correct but a detail may be incomplete, prefer normal or hint. The batch scorer should be able to pass a case with only non-blocking detail gaps.

Workflow context:
%s

Task input:
%s

Task variables:
%s

Attachments:
%s

Expected result:
%s`, strings.TrimSpace(input.WorkflowContext), strings.TrimSpace(input.Content), variablesJSON, attachmentsJSON, strings.TrimSpace(input.ExpectedResult))
}

func parseTaskExpectedChecks(content string) (*TaskExpectedChecks, error) {
	raw := extractGeneratedCasesJSON(stripJSONCodeFence(strings.TrimSpace(content)))
	type rawChecks struct {
		MustVisitNodes json.RawMessage `json:"must_visit_nodes"`
		MustCallTools  json.RawMessage `json:"must_call_tools"`
		OutputContains json.RawMessage `json:"output_contains"`
		MaxLatencyMS   json.RawMessage `json:"max_latency_ms"`
		Conditions     json.RawMessage `json:"conditions"`
	}
	var parsed rawChecks
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse task expected checks JSON: %w", err)
	}
	checks := &TaskExpectedChecks{
		MustVisitNodes: fixtureStringListValue(parsed.MustVisitNodes),
		MustCallTools:  fixtureStringListValue(parsed.MustCallTools),
		OutputContains: fixtureStringListValue(parsed.OutputContains),
		MaxLatencyMS:   fixtureIntValue(parsed.MaxLatencyMS),
		Conditions:     expectedCheckConditionsValue(parsed.Conditions),
	}
	checks.Conditions = normalizeExpectedCheckConditions(checks.Conditions, *checks)
	return checks, nil
}

func fixtureIntValue(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, _ := strconv.Atoi(strings.TrimSpace(text))
		return value
	}
	return 0
}

func (c TaskExpectedChecks) Useful() bool {
	return len(c.Conditions) > 0 ||
		len(c.MustVisitNodes) > 0 ||
		len(c.MustCallTools) > 0 ||
		len(c.OutputContains) > 0 ||
		c.MaxLatencyMS > 0
}

func (c TaskExpectedChecks) JSONMap() JSONMap {
	result := JSONMap{}
	conditions := normalizeExpectedCheckConditions(c.Conditions, c)
	if len(conditions) > 0 {
		result["conditions"] = conditions
	}
	if len(c.MustVisitNodes) > 0 {
		result["must_visit_nodes"] = c.MustVisitNodes
	}
	if len(c.MustCallTools) > 0 {
		result["must_call_tools"] = c.MustCallTools
	}
	if len(c.OutputContains) > 0 {
		result["output_contains"] = c.OutputContains
	}
	if c.MaxLatencyMS > 0 {
		result["max_latency_ms"] = c.MaxLatencyMS
	}
	return result
}

func expectedChecksFromInput(value interface{}) TaskExpectedChecks {
	raw, ok := value.(map[string]interface{})
	if !ok {
		if mapped, ok := value.(JSONMap); ok {
			raw = map[string]interface{}(mapped)
		} else {
			return TaskExpectedChecks{}
		}
	}
	return TaskExpectedChecks{
		MustVisitNodes: stringListFromInterface(raw["must_visit_nodes"]),
		MustCallTools:  stringListFromInterface(raw["must_call_tools"]),
		OutputContains: stringListFromInterface(raw["output_contains"]),
		MaxLatencyMS:   intFromInterface(raw["max_latency_ms"]),
		Conditions:     expectedCheckConditionsFromInterface(raw["conditions"]),
	}
}

func stringListFromInterface(value interface{}) []string {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return fixtureStringListValue(data)
}

func intFromInterface(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func expectedCheckConditionsValue(raw json.RawMessage) []TaskExpectedCheckCondition {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var values []TaskExpectedCheckCondition
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}
	return nil
}

func expectedCheckConditionsFromInterface(value interface{}) []TaskExpectedCheckCondition {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return expectedCheckConditionsValue(data)
}

func normalizeExpectedCheckConditions(values []TaskExpectedCheckCondition, legacy TaskExpectedChecks) []TaskExpectedCheckCondition {
	conditions := make([]TaskExpectedCheckCondition, 0, len(values)+len(legacy.MustVisitNodes)+len(legacy.MustCallTools)+len(legacy.OutputContains)+1)
	seenIDs := map[string]int{}
	for _, condition := range values {
		condition.ID = strings.TrimSpace(condition.ID)
		condition.Type = strings.TrimSpace(condition.Type)
		condition.Operator = strings.TrimSpace(condition.Operator)
		condition.TargetID = strings.TrimSpace(condition.TargetID)
		condition.TargetLabel = strings.TrimSpace(condition.TargetLabel)
		condition.TargetType = strings.TrimSpace(condition.TargetType)
		condition.Values = normalizeFixtureStringList(condition.Values)
		condition.MatchMode = normalizeMatchMode(condition.MatchMode)
		condition.Severity = normalizeSeverity(condition.Severity)
		condition.Source = normalizeCheckSource(condition.Source)
		if condition.Type == "" {
			continue
		}
		if condition.Operator == "" {
			condition.Operator = defaultCheckOperator(condition.Type)
		}
		if condition.Type == "node" || condition.Type == "capability" {
			if condition.TargetID == "" && condition.TargetLabel == "" {
				continue
			}
		}
		if condition.Type == "output_contains" && len(condition.Values) == 0 {
			continue
		}
		if condition.Type == "latency" && condition.ValueMS <= 0 {
			continue
		}
		condition.ID = uniqueWorkflowTestCheckID(seenIDs, condition.ID, fmt.Sprintf("check_%s_%d", condition.Type, len(conditions)+1))
		conditions = append(conditions, condition)
	}
	if len(conditions) > 0 {
		return conditions
	}
	for _, node := range legacy.MustVisitNodes {
		id := uniqueWorkflowTestCheckID(seenIDs, "", fmt.Sprintf("check_node_%d", len(conditions)+1))
		conditions = append(conditions, TaskExpectedCheckCondition{
			ID:          id,
			Type:        "node",
			Operator:    "visited",
			TargetID:    node,
			TargetLabel: node,
			Severity:    "normal",
			Source:      "ai_generated",
		})
	}
	for _, tool := range legacy.MustCallTools {
		id := uniqueWorkflowTestCheckID(seenIDs, "", fmt.Sprintf("check_capability_%d", len(conditions)+1))
		conditions = append(conditions, TaskExpectedCheckCondition{
			ID:          id,
			Type:        "capability",
			Operator:    "called",
			TargetID:    tool,
			TargetLabel: tool,
			Severity:    "normal",
			Source:      "ai_generated",
		})
	}
	if len(legacy.OutputContains) > 0 {
		id := uniqueWorkflowTestCheckID(seenIDs, "", fmt.Sprintf("check_output_contains_%d", len(conditions)+1))
		conditions = append(conditions, TaskExpectedCheckCondition{
			ID:        id,
			Type:      "output_contains",
			Operator:  "contains",
			Values:    normalizeFixtureStringList(legacy.OutputContains),
			MatchMode: "semantic",
			Severity:  "critical",
			Source:    "ai_generated",
		})
	}
	if legacy.MaxLatencyMS > 0 {
		id := uniqueWorkflowTestCheckID(seenIDs, "", fmt.Sprintf("check_latency_%d", len(conditions)+1))
		conditions = append(conditions, TaskExpectedCheckCondition{
			ID:       id,
			Type:     "latency",
			Operator: "lte",
			ValueMS:  legacy.MaxLatencyMS,
			Severity: "hint",
			Source:   "ai_generated",
		})
	}
	return conditions
}

func defaultCheckOperator(checkType string) string {
	switch strings.TrimSpace(checkType) {
	case "node":
		return "visited"
	case "capability":
		return "called"
	case "latency":
		return "lte"
	default:
		return "contains"
	}
}

func normalizeMatchMode(value string) string {
	switch strings.TrimSpace(value) {
	case "keyword":
		return "keyword"
	default:
		return "semantic"
	}
}

func normalizeSeverity(value string) string {
	switch strings.TrimSpace(value) {
	case "critical", "hint":
		return value
	default:
		return "normal"
	}
}

func normalizeCheckSource(value string) string {
	switch strings.TrimSpace(value) {
	case "user_added", "system_default":
		return value
	default:
		return "ai_generated"
	}
}
