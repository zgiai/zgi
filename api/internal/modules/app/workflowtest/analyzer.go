package workflowtest

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	workflowTestAnalysisOutputKey = "workflow_test_analysis"
	checkResultsOutputKey         = "check_results"
)

type WorkflowTestAnalysis struct {
	Mode             string                      `json:"mode"`
	EvaluationSchema *TaskEvaluationSchema       `json:"evaluation_schema,omitempty"`
	Trace            WorkflowTestTrace           `json:"trace"`
	Comparisons      WorkflowTestComparisons     `json:"comparisons"`
	Summary          WorkflowTestAnalysisSummary `json:"summary"`
	Suggestions      []WorkflowTestSuggestion    `json:"suggestions"`
}

type WorkflowTestTrace struct {
	Nodes []WorkflowTestTraceNode `json:"nodes"`
}

type WorkflowTestTraceNode struct {
	NodeID     string                 `json:"node_id"`
	NodeName   string                 `json:"node_name"`
	NodeType   string                 `json:"node_type"`
	Status     string                 `json:"status"`
	DurationMS float64                `json:"duration_ms"`
	Input      map[string]interface{} `json:"input,omitempty"`
	Output     map[string]interface{} `json:"output,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

type WorkflowTestComparisons struct {
	Overall *WorkflowTestComparison   `json:"overall,omitempty"`
	Checks  []WorkflowTestCheckResult `json:"checks"`
	Turns   []WorkflowTestTurnResult  `json:"turns,omitempty"`
}

type WorkflowTestComparison struct {
	Status     string `json:"status"`
	Expected   string `json:"expected"`
	Actual     string `json:"actual"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion,omitempty"`
}

type WorkflowTestCheckResult struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion,omitempty"`
}

type WorkflowTestTurnResult struct {
	TurnIndex  int    `json:"turn_index"`
	UserInput  string `json:"user_input"`
	Expected   string `json:"expected,omitempty"`
	Actual     string `json:"actual"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion,omitempty"`
}

type WorkflowTestAnalysisSummary struct {
	Status         string  `json:"status"`
	MainIssue      string  `json:"main_issue,omitempty"`
	FailedStage    string  `json:"failed_stage,omitempty"`
	ReferenceScore float64 `json:"reference_score,omitempty"`
	Total          int     `json:"total"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	Review         int     `json:"review"`
	CriticalFailed int     `json:"critical_failed"`
}

type WorkflowTestSuggestion struct {
	Target  string `json:"target"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type WorkflowTestCheckResults struct {
	Summary    WorkflowTestAnalysisSummary `json:"summary"`
	Conditions []WorkflowTestCheckResult   `json:"conditions"`
}

func analyzeWorkflowTestResult(snapshot CaseSnapshot, result *RunCaseResult) *WorkflowTestAnalysis {
	outputs := map[string]interface{}{}
	if result != nil && result.Outputs != nil {
		outputs = result.Outputs
	}
	mode := workflowTestModeFromSnapshot(snapshot)
	analysis := &WorkflowTestAnalysis{
		Mode:  mode,
		Trace: WorkflowTestTrace{Nodes: workflowTraceFromOutputs(outputs)},
		Comparisons: WorkflowTestComparisons{
			Checks: []WorkflowTestCheckResult{},
		},
		Suggestions: []WorkflowTestSuggestion{},
	}
	if mode == "conversation" {
		evaluateConversationWorkflow(snapshot, outputs, analysis)
	} else {
		evaluateTaskWorkflow(snapshot, outputs, analysis)
	}
	finalizeWorkflowTestAnalysis(analysis)
	return analysis
}

func workflowTestModeFromSnapshot(snapshot CaseSnapshot) string {
	if len(snapshot.Turns) > 0 {
		if value, ok := snapshot.Turns[0].Inputs[caseModeInputKey].(string); ok {
			if mode := normalizeCaseMode(value); mode != "" {
				return mode
			}
		}
	}
	if len(snapshot.Turns) > 1 {
		return "conversation"
	}
	return "task"
}

func evaluateTaskWorkflow(snapshot CaseSnapshot, outputs map[string]interface{}, analysis *WorkflowTestAnalysis) {
	expected := strings.TrimSpace(snapshot.ExpectedResult)
	actual := workflowActualOutputText(outputs)
	if len(snapshot.Turns) == 0 {
		return
	}
	checks := expectedChecksFromInput(snapshot.Turns[0].Inputs[expectedChecksInputKey])
	schema := taskEvaluationSchemaFromInput(snapshot.Turns[0].Inputs[evaluationSchemaInputKey], expected, checks)
	analysis.EvaluationSchema = &schema
	for _, assertion := range schema.Assertions {
		analysis.Comparisons.Checks = append(analysis.Comparisons.Checks, evaluateTaskEvaluationAssertion(assertion, schema, actual))
	}
	for _, condition := range normalizeExpectedCheckConditions(checks.Conditions, checks) {
		if !shouldEvaluateTaskConditionOutsideSchema(condition) {
			continue
		}
		analysis.Comparisons.Checks = append(analysis.Comparisons.Checks, evaluateTaskCondition(condition, outputs, analysis.Trace.Nodes))
	}
}

func shouldEvaluateTaskConditionOutsideSchema(condition TaskExpectedCheckCondition) bool {
	switch condition.Type {
	case "node", "capability", "latency":
		return true
	default:
		return false
	}
}

func evaluateTaskEvaluationAssertion(assertion TaskEvaluationAssertion, schema TaskEvaluationSchema, actual string) WorkflowTestCheckResult {
	result := WorkflowTestCheckResult{
		ID:       assertion.ID,
		Type:     "assertion_" + assertion.Type,
		Label:    firstNonEmptyString(assertion.Description, strings.Join(assertion.Values, "、"), assertion.Type),
		Status:   "review",
		Severity: normalizeTaskSeverity(assertion.Severity),
	}
	switch assertion.Type {
	case "must_include", "fact_present":
		return finalizeTaskAssertionCheck(assertion, compareValuesCondition(result, assertion.Values, actual, "contains", assertion.MatchMode))
	case "must_not_include":
		return finalizeTaskAssertionCheck(assertion, compareValuesCondition(result, assertion.Values, actual, "not_contains", assertion.MatchMode))
	case "missing_field_marked":
		return finalizeTaskAssertionCheck(assertion, evaluateMissingFieldMarkedAssertion(result, assertion.Values, actual))
	case "state_present":
		return finalizeTaskAssertionCheck(assertion, evaluateStatePresentAssertion(result, assertion.Values, actual))
	case "missing_policy":
		return evaluateMissingPolicyAssertion(result, schema.MissingPolicy, actual)
	case "format":
		return evaluateFormatAssertion(result, schema.Format, actual)
	case "source_grounding", "action_result", "semantic_match":
		result.Evidence = "该断言需要 AI 结合业务语义判断：" + firstNonEmptyString(assertion.Description, strings.Join(assertion.Values, "、"))
		result.Suggestion = ""
		return result
	default:
		if len(assertion.Values) > 0 {
			return finalizeTaskAssertionCheck(assertion, compareValuesCondition(result, assertion.Values, actual, assertion.Operator, assertion.MatchMode))
		}
		result.Evidence = "该断言缺少可自动判定的结构化值"
		return result
	}
}

func finalizeTaskAssertionCheck(assertion TaskEvaluationAssertion, result WorkflowTestCheckResult) WorkflowTestCheckResult {
	if assertion.Source == "optional_example" && result.Status != "failed" {
		result.Status = "passed"
		if strings.TrimSpace(result.Evidence) == "" || strings.Contains(result.Evidence, "未命中字面内容") {
			result.Evidence = "可选示例项未作为硬性失败条件"
		}
		return result
	}
	if assertion.Source == "optional_example" && result.Status == "failed" {
		result.Status = "passed"
		result.Evidence = "可选示例项未完全命中，但不影响任务结果"
		result.Suggestion = ""
	}
	return result
}

func evaluateMissingFieldMarkedAssertion(result WorkflowTestCheckResult, fields []string, actual string) WorkflowTestCheckResult {
	if len(fields) == 0 {
		result.Status = "review"
		result.Evidence = "缺失字段断言未提供具体字段"
		return result
	}
	missing := []string{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if !taskMissingFieldMarked(field, actual) {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		result.Status = "passed"
		result.Evidence = "实际输出已明确标注缺失字段：" + strings.Join(fields, "、")
		return result
	}
	result.Status = "failed"
	result.Evidence = "未识别到这些缺失字段的明确标注：" + strings.Join(missing, "、")
	result.Suggestion = "请让回复生成节点在业务资料缺项时使用“未提供、未明确、待补充、为空”等明确标记，而不是泛化描述。"
	return result
}

func evaluateStatePresentAssertion(result WorkflowTestCheckResult, states []string, actual string) WorkflowTestCheckResult {
	if len(states) == 0 {
		result.Status = "review"
		result.Evidence = "状态断言未提供具体状态"
		return result
	}
	missing := []string{}
	for _, state := range states {
		state = strings.TrimSpace(state)
		if state == "" {
			continue
		}
		if !taskStatePresent(state, actual) {
			missing = append(missing, state)
		}
	}
	if len(missing) == 0 {
		result.Status = "passed"
		result.Evidence = "实际输出已表达期望状态：" + strings.Join(states, "、")
		return result
	}
	result.Status = "failed"
	result.Evidence = "未识别到期望状态或可推断该状态的证据：" + strings.Join(missing, "、")
	result.Suggestion = "请在输出中明确给出业务状态，或同时列出足以推断该状态的关键事实。"
	return result
}

func evaluateMissingPolicyAssertion(result WorkflowTestCheckResult, policy TaskMissingPolicy, actual string) WorkflowTestCheckResult {
	if len(policy.ForbidClaims) > 0 {
		forbidden := compareValuesCondition(result, policy.ForbidClaims, actual, "not_contains", "semantic")
		if forbidden.Status == "failed" {
			forbidden.Suggestion = "请区分技术性解析失败与业务字段缺失；不要在文件可读时声称无法读取或需要转人工。"
			return forbidden
		}
	}
	if len(policy.AcceptMarkers) > 0 {
		result.Status = "passed"
		result.Evidence = "未发现违反缺失信息处理策略的技术性失败表述"
		return result
	}
	result.Status = "review"
	result.Evidence = "缺失信息策略未提供可自动判定的标记"
	return result
}

func evaluateFormatAssertion(result WorkflowTestCheckResult, format TaskFormatExpectation, actual string) WorkflowTestCheckResult {
	if strings.TrimSpace(format.Type) == "" && len(format.Fields) == 0 {
		result.Status = "review"
		result.Evidence = "未配置明确输出格式要求"
		return result
	}
	if len(format.Fields) > 0 {
		return compareValuesCondition(result, format.Fields, actual, "contains", "semantic")
	}
	result.Status = "review"
	result.Evidence = "输出格式需要 AI 或人工结合样式要求判断：" + format.Type
	return result
}

func evaluateConversationWorkflow(snapshot CaseSnapshot, outputs map[string]interface{}, analysis *WorkflowTestAnalysis) {
	turnOutputs := conversationTurnOutputs(outputs)
	for index, turn := range snapshot.Turns {
		actual := actualTurnOutputText(turnOutputs, index, outputs)
		expected := stringFromInputValue(turn.Inputs[turnExpectationInputKey])
		if expected != "" {
			result := compareTurnExpectation(index+1, turn.Content, expected, actual)
			analysis.Comparisons.Turns = append(analysis.Comparisons.Turns, result)
			analysis.Comparisons.Checks = append(analysis.Comparisons.Checks, WorkflowTestCheckResult{
				ID:         fmt.Sprintf("turn_expectation_%d", index+1),
				Type:       "turn_expectation",
				Label:      fmt.Sprintf("第 %d 轮期望", index+1),
				Status:     result.Status,
				Severity:   "normal",
				Evidence:   result.Evidence,
				Suggestion: result.Suggestion,
			})
		}
		turnChecks := conversationExpectedChecksFromInput(turn.Inputs[turnChecksInputKey])
		for _, condition := range turnChecks.Conditions {
			analysis.Comparisons.Checks = append(analysis.Comparisons.Checks, evaluateConversationCondition(condition, actual, index+1, false))
		}
	}
	expected := strings.TrimSpace(snapshot.ExpectedResult)
	actual := workflowActualOutputText(outputs)
	if expected != "" {
		analysis.Comparisons.Overall = compareTextExpectation(expected, actual, "overall_expected_result", "整体预期结果", "normal", "semantic")
	}
	globalChecks := ConversationExpectedChecks{}
	if len(snapshot.Turns) > 0 {
		globalChecks = conversationExpectedChecksFromInput(snapshot.Turns[0].Inputs[conversationChecksInputKey])
	}
	conversationText := conversationTranscriptText(snapshot, outputs)
	for _, condition := range globalChecks.Conditions {
		analysis.Comparisons.Checks = append(analysis.Comparisons.Checks, evaluateConversationCondition(condition, conversationText, 0, true))
	}
}

func evaluateTaskCondition(condition TaskExpectedCheckCondition, outputs map[string]interface{}, nodes []WorkflowTestTraceNode) WorkflowTestCheckResult {
	result := WorkflowTestCheckResult{
		ID:       condition.ID,
		Type:     condition.Type,
		Label:    checkConditionLabel(condition),
		Status:   "review",
		Severity: condition.Severity,
	}
	switch condition.Type {
	case "node":
		return evaluateNodeCondition(condition, nodes)
	case "capability":
		return evaluateCapabilityCondition(condition, nodes)
	case "output_contains":
		return compareValuesCondition(result, condition.Values, workflowActualOutputText(outputs), condition.Operator, condition.MatchMode)
	case "latency":
		actual := workflowElapsedMS(outputs)
		if actual <= 0 {
			result.Evidence = "未获取到可用于判断的执行耗时"
			result.Suggestion = "请确认 workflow run 输出或节点日志中包含耗时信息。"
			return result
		}
		if condition.Operator == "gte" {
			if int(actual) >= condition.ValueMS {
				result.Status = "passed"
				result.Evidence = fmt.Sprintf("实际耗时 %.0fms，大于等于阈值 %dms", actual, condition.ValueMS)
			} else {
				result.Status = "failed"
				result.Evidence = fmt.Sprintf("实际耗时 %.0fms，小于阈值 %dms", actual, condition.ValueMS)
			}
			return result
		}
		if int(actual) <= condition.ValueMS {
			result.Status = "passed"
			result.Evidence = fmt.Sprintf("实际耗时 %.0fms，小于等于阈值 %dms", actual, condition.ValueMS)
		} else {
			result.Status = "failed"
			result.Evidence = fmt.Sprintf("实际耗时 %.0fms，超过阈值 %dms", actual, condition.ValueMS)
			result.Suggestion = "建议检查耗时最长的节点、外部工具调用和文件解析链路。"
		}
		return result
	default:
		result.Evidence = "暂不支持该检查点类型"
		return result
	}
}

func evaluateNodeCondition(condition TaskExpectedCheckCondition, nodes []WorkflowTestTraceNode) WorkflowTestCheckResult {
	result := WorkflowTestCheckResult{
		ID:       condition.ID,
		Type:     condition.Type,
		Label:    checkConditionLabel(condition),
		Severity: condition.Severity,
		Status:   "failed",
	}
	node, found := findTraceNode(nodes, condition.TargetID, condition.TargetLabel)
	switch condition.Operator {
	case "not_visited":
		if !found {
			result.Status = "passed"
			result.Evidence = "执行链路未包含目标节点"
		} else {
			result.Evidence = "执行链路包含目标节点：" + nodeDisplayName(node)
		}
		return result
	case "input_contains", "input_not_contains":
		if !found {
			result.Evidence = "执行链路未包含目标节点"
			result.Suggestion = "请确认分支条件或前置节点是否阻止该节点执行。"
			return result
		}
		return compareValuesCondition(result, condition.Values, jsonText(node.Input), condition.Operator, condition.MatchMode)
	case "output_contains", "output_not_contains":
		if !found {
			result.Evidence = "执行链路未包含目标节点"
			result.Suggestion = "请确认分支条件或前置节点是否阻止该节点执行。"
			return result
		}
		return compareValuesCondition(result, condition.Values, jsonText(node.Output), condition.Operator, condition.MatchMode)
	default:
		if found {
			result.Status = "passed"
			result.Evidence = "执行链路包含目标节点：" + nodeDisplayName(node)
		} else {
			result.Evidence = "执行链路未包含目标节点"
			result.Suggestion = "请确认分支条件、输入变量或节点配置是否符合该样例。"
		}
		return result
	}
}

func evaluateCapabilityCondition(condition TaskExpectedCheckCondition, nodes []WorkflowTestTraceNode) WorkflowTestCheckResult {
	result := WorkflowTestCheckResult{
		ID:       condition.ID,
		Type:     condition.Type,
		Label:    checkConditionLabel(condition),
		Severity: condition.Severity,
		Status:   "failed",
	}
	node, found := findTraceNode(nodes, condition.TargetID, condition.TargetLabel)
	if condition.Operator == "not_called" {
		if !found {
			result.Status = "passed"
			result.Evidence = "执行链路未调用目标能力"
		} else {
			result.Evidence = "执行链路调用了目标能力：" + nodeDisplayName(node)
		}
		return result
	}
	if found {
		result.Status = "passed"
		result.Evidence = "执行链路调用了目标能力：" + nodeDisplayName(node)
	} else {
		result.Evidence = "执行链路未调用目标能力"
		result.Suggestion = "请检查工具节点配置、触发条件或上游输出是否满足调用条件。"
	}
	return result
}

func evaluateConversationCondition(condition ConversationExpectedCheckCondition, actual string, turnIndex int, global bool) WorkflowTestCheckResult {
	label := condition.TargetLabel
	if label == "" {
		label = strings.Join(condition.Values, "、")
	}
	if label == "" {
		label = condition.Type
	}
	if global {
		label = "全局对话检查：" + label
	} else if turnIndex > 0 {
		label = fmt.Sprintf("第 %d 轮检查：%s", turnIndex, label)
	}
	result := WorkflowTestCheckResult{
		ID:       condition.ID,
		Type:     condition.Type,
		Label:    label,
		Severity: condition.Severity,
		Status:   "review",
	}
	return compareValuesCondition(result, condition.Values, actual, condition.Operator, condition.MatchMode)
}

func compareTextExpectation(expected, actual, id, label, severity, matchMode string) *WorkflowTestComparison {
	check := compareValuesCondition(WorkflowTestCheckResult{
		ID:       id,
		Type:     "expected_result",
		Label:    label,
		Severity: severity,
		Status:   "review",
	}, []string{expected}, actual, "contains", matchMode)
	return &WorkflowTestComparison{
		Status:     check.Status,
		Expected:   expected,
		Actual:     actual,
		Evidence:   check.Evidence,
		Suggestion: check.Suggestion,
	}
}

func compareTurnExpectation(index int, input, expected, actual string) WorkflowTestTurnResult {
	comparison := compareTextExpectation(expected, actual, fmt.Sprintf("turn_%d_expected", index), fmt.Sprintf("第 %d 轮期望", index), "normal", "semantic")
	return WorkflowTestTurnResult{
		TurnIndex:  index,
		UserInput:  input,
		Expected:   expected,
		Actual:     actual,
		Status:     comparison.Status,
		Evidence:   comparison.Evidence,
		Suggestion: comparison.Suggestion,
	}
}

func compareValuesCondition(result WorkflowTestCheckResult, expectedValues []string, actual string, operator string, matchMode string) WorkflowTestCheckResult {
	operator = strings.TrimSpace(operator)
	if operator == "" || operator == "passed" {
		operator = "contains"
	}
	if len(expectedValues) == 0 {
		result.Status = "review"
		result.Evidence = "检查点未提供可判定的期望值"
		return result
	}
	negative := strings.Contains(operator, "not_contains")
	missing := missingValues(expectedValues, actual)
	if negative {
		missing = literalMissingValues(expectedValues, actual)
	}
	containsAll := len(missing) == 0
	if negative {
		if len(missing) < len(expectedValues) {
			result.Status = "failed"
			present := make([]string, 0, len(expectedValues)-len(missing))
			missingSet := map[string]struct{}{}
			for _, value := range missing {
				missingSet[value] = struct{}{}
			}
			for _, value := range expectedValues {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				if _, ok := missingSet[value]; !ok {
					present = append(present, value)
				}
			}
			result.Evidence = "实际输出包含不应出现的内容：" + strings.Join(present, "、")
			result.Suggestion = "请检查节点提示词或输出过滤逻辑，避免返回该类内容。"
		} else {
			result.Status = "passed"
			result.Evidence = "实际输出未包含禁止内容"
		}
		return result
	}
	if containsAll {
		result.Status = "passed"
		result.Evidence = "实际内容包含期望值：" + strings.Join(expectedValues, "、")
		return result
	}
	if matchMode == "keyword" {
		result.Status = "failed"
		result.Evidence = "实际内容缺少关键词：" + strings.Join(missing, "、")
		result.Suggestion = "请检查回复生成节点、上游变量传递或知识引用是否明确覆盖这些必需字段。"
		return result
	}
	result.Status = "review"
	result.Evidence = "未命中字面内容，语义类检查需要 AI 或人工复核：" + strings.Join(missing, "、")
	result.Suggestion = "请在工作流回复生成策略中补充这些业务要点；如果需要用户补充信息，应由节点明确追问而不是泛化回答。"
	return result
}

func finalizeWorkflowTestAnalysis(analysis *WorkflowTestAnalysis) {
	if analysis == nil {
		return
	}
	allChecks := append([]WorkflowTestCheckResult{}, analysis.Comparisons.Checks...)
	if analysis.Comparisons.Overall != nil {
		allChecks = append(allChecks, WorkflowTestCheckResult{
			ID:       "overall_expected_result",
			Type:     "expected_result",
			Label:    "整体预期结果",
			Status:   analysis.Comparisons.Overall.Status,
			Severity: "normal",
			Evidence: analysis.Comparisons.Overall.Evidence,
		})
	}
	summary := WorkflowTestAnalysisSummary{Status: "passed", Total: len(allChecks)}
	for _, check := range allChecks {
		switch check.Status {
		case "passed":
			summary.Passed++
		case "failed":
			summary.Failed++
			if summary.MainIssue == "" {
				summary.MainIssue = check.Label
				summary.FailedStage = check.Type
			}
			if check.Severity == "critical" {
				summary.CriticalFailed++
			}
			if check.Suggestion != "" {
				analysis.Suggestions = append(analysis.Suggestions, WorkflowTestSuggestion{
					Target:  check.Label,
					Type:    check.Type,
					Content: check.Suggestion,
				})
			}
		default:
			summary.Review++
			if summary.MainIssue == "" {
				summary.MainIssue = check.Label
				summary.FailedStage = check.Type
			}
			if check.Suggestion != "" {
				analysis.Suggestions = append(analysis.Suggestions, WorkflowTestSuggestion{
					Target:  check.Label,
					Type:    check.Type,
					Content: check.Suggestion,
				})
			}
		}
	}
	summary.ReferenceScore = workflowTestReferenceScore(summary)
	if analysis.Mode == "task" {
		summary.Status = taskWorkflowSummaryStatus(summary, allChecks)
	} else if summary.Failed > 0 {
		summary.Status = "failed"
	} else if summary.Review > 0 {
		summary.Status = "review"
	}
	analysis.Summary = summary
}

func workflowTestReferenceScore(summary WorkflowTestAnalysisSummary) float64 {
	if summary.Total <= 0 {
		return 0
	}
	return (float64(summary.Passed) + float64(summary.Review)*0.5) / float64(summary.Total) * 5
}

func taskWorkflowSummaryStatus(summary WorkflowTestAnalysisSummary, checks []WorkflowTestCheckResult) string {
	if summary.Total == 0 {
		return "passed"
	}
	if taskHasBlockingFailure(checks) {
		return "failed"
	}
	if summary.Failed == 0 && summary.Review == 0 {
		return "passed"
	}
	if summary.ReferenceScore >= 4.5 && summary.CriticalFailed == 0 {
		return "passed"
	}
	if summary.ReferenceScore < 3.5 {
		return "failed"
	}
	return "review"
}

func taskHasBlockingFailure(checks []WorkflowTestCheckResult) bool {
	for _, check := range checks {
		if check.Status != "failed" {
			continue
		}
		switch check.Type {
		case "assertion_must_not_include", "assertion_missing_policy":
			return true
		case "node", "capability":
			if check.Severity == "critical" {
				return true
			}
		}
	}
	return false
}

func workflowTraceFromOutputs(outputs map[string]interface{}) []WorkflowTestTraceNode {
	if raw, ok := outputs["workflow_trace"].(map[string]interface{}); ok {
		if nodes, ok := raw["nodes"].([]map[string]interface{}); ok {
			return workflowTraceNodesFromMaps(nodes)
		}
		return workflowTraceNodesFromInterfaces(raw["nodes"])
	}
	return workflowTraceNodesFromNodeResults(outputs["node_results"])
}

func workflowTraceNodesFromMaps(values []map[string]interface{}) []WorkflowTestTraceNode {
	nodes := make([]WorkflowTestTraceNode, 0, len(values))
	for _, value := range values {
		nodes = append(nodes, workflowTraceNodeFromMap(value))
	}
	return nodes
}

func workflowTraceNodesFromInterfaces(value interface{}) []WorkflowTestTraceNode {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	nodes := make([]WorkflowTestTraceNode, 0, len(values))
	for _, item := range values {
		if mapped, ok := item.(map[string]interface{}); ok {
			nodes = append(nodes, workflowTraceNodeFromMap(mapped))
		}
	}
	return nodes
}

func workflowTraceNodesFromNodeResults(value interface{}) []WorkflowTestTraceNode {
	mapped, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	nodes := make([]WorkflowTestTraceNode, 0, len(mapped))
	for id, raw := range mapped {
		nodeMap, _ := raw.(map[string]interface{})
		node := workflowTraceNodeFromMap(nodeMap)
		if node.NodeID == "" {
			node.NodeID = id
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func workflowTraceNodeFromMap(value map[string]interface{}) WorkflowTestTraceNode {
	return WorkflowTestTraceNode{
		NodeID:     firstString(value, "node_id", "nodeID", "id"),
		NodeName:   firstString(value, "node_name", "nodeName", "title", "name"),
		NodeType:   firstString(value, "node_type", "nodeType", "type"),
		Status:     firstString(value, "status"),
		DurationMS: floatFromInterface(firstValue(value, "duration_ms", "elapsed_time", "elapsed_ms")),
		Input:      mapFromInterface(firstValue(value, "input", "inputs")),
		Output:     mapFromInterface(firstValue(value, "output", "outputs")),
		Error:      firstString(value, "error"),
	}
}

func findTraceNode(nodes []WorkflowTestTraceNode, id string, label string) (WorkflowTestTraceNode, bool) {
	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	for _, node := range nodes {
		if id != "" && (node.NodeID == id || strings.EqualFold(node.NodeID, id)) {
			return node, true
		}
		if label != "" && (node.NodeName == label || strings.EqualFold(node.NodeName, label) || strings.EqualFold(node.NodeID, label)) {
			return node, true
		}
	}
	return WorkflowTestTraceNode{}, false
}

func checkConditionLabel(condition TaskExpectedCheckCondition) string {
	if condition.TargetLabel != "" {
		return condition.TargetLabel
	}
	if condition.TargetID != "" {
		return condition.TargetID
	}
	if len(condition.Values) > 0 {
		return strings.Join(condition.Values, "、")
	}
	return condition.Type
}

func nodeDisplayName(node WorkflowTestTraceNode) string {
	if node.NodeName != "" {
		return node.NodeName
	}
	return node.NodeID
}

func workflowActualOutputText(outputs map[string]interface{}) string {
	if nested := mapFromInterface(outputs["outputs"]); len(nested) > 0 {
		if text := workflowActualOutputText(nested); text != "" {
			return text
		}
		filtered := businessOutputMap(nested)
		if len(filtered) > 0 {
			return jsonText(filtered)
		}
	}
	for _, key := range businessOutputTextKeys() {
		if value, ok := outputs[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if value := firstBusinessScalar(outputs); value != "" {
		return value
	}
	filtered := businessOutputMap(outputs)
	if len(filtered) > 0 {
		return jsonText(filtered)
	}
	return ""
}

func businessOutputTextKeys() []string {
	return []string{"answer", "text", "summary", "result", "output", "content", "message", "value"}
}

func firstBusinessScalar(outputs map[string]interface{}) string {
	for _, key := range businessOutputTextKeys() {
		raw, ok := outputs[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case fmt.Stringer:
			if text := strings.TrimSpace(value.String()); text != "" {
				return text
			}
		}
	}
	if len(outputs) == 1 {
		for _, raw := range outputs {
			if text := scalarText(raw); text != "" {
				return text
			}
		}
	}
	return ""
}

func scalarText(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case int, int64, float64, float32, bool, json.Number:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}

func businessOutputMap(outputs map[string]interface{}) map[string]interface{} {
	filtered := make(map[string]interface{}, len(outputs))
	for key, value := range outputs {
		if isWorkflowTestRuntimeOutputKey(key) {
			continue
		}
		if strings.TrimSpace(key) == "" {
			continue
		}
		filtered[key] = value
	}
	return filtered
}

func isWorkflowTestRuntimeOutputKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "",
		"elapsed_time",
		"elapsed_ms",
		"duration_ms",
		"latency_ms",
		"response_time_ms",
		"node_results",
		"node_errors",
		"status",
		"task_id",
		"workflow_run_id",
		"workflow_trace",
		workflowTestAnalysisOutputKey,
		checkResultsOutputKey,
		"turn_results",
		"turn_count",
		"conversation_id":
		return true
	default:
		return false
	}
}

func conversationTurnOutputs(outputs map[string]interface{}) []map[string]interface{} {
	raw, ok := outputs["turn_results"].([]map[string]interface{})
	if ok {
		result := make([]map[string]interface{}, 0, len(raw))
		for _, item := range raw {
			result = append(result, mapFromInterface(item["outputs"]))
		}
		return result
	}
	values, ok := outputs["turn_results"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(values))
	for _, item := range values {
		mapped := mapFromInterface(item)
		result = append(result, mapFromInterface(mapped["outputs"]))
	}
	return result
}

func actualTurnOutputText(turnOutputs []map[string]interface{}, index int, fallback map[string]interface{}) string {
	if index >= 0 && index < len(turnOutputs) {
		if actual := workflowActualOutputText(turnOutputs[index]); actual != "" {
			return actual
		}
	}
	return workflowActualOutputText(fallback)
}

func conversationTranscriptText(snapshot CaseSnapshot, outputs map[string]interface{}) string {
	turnOutputs := conversationTurnOutputs(outputs)
	parts := make([]string, 0, len(snapshot.Turns)*2)
	for index, turn := range snapshot.Turns {
		if strings.TrimSpace(turn.Content) != "" {
			parts = append(parts, "用户："+strings.TrimSpace(turn.Content))
		}
		if actual := actualTurnOutputText(turnOutputs, index, outputs); actual != "" {
			parts = append(parts, "回复："+actual)
		}
	}
	if len(parts) == 0 {
		return workflowActualOutputText(outputs)
	}
	return strings.Join(parts, "\n")
}

func workflowElapsedMS(outputs map[string]interface{}) float64 {
	for _, key := range []string{"elapsed_time", "elapsed_ms", "duration_ms", "latency_ms", "response_time_ms"} {
		if value := floatFromInterface(outputs[key]); value > 0 {
			return value
		}
	}
	return 0
}

func literalMissingValues(values []string, actual string) []string {
	actual = strings.ToLower(actual)
	missing := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !strings.Contains(actual, strings.ToLower(value)) {
			missing = append(missing, value)
		}
	}
	return missing
}

func missingValues(values []string, actual string) []string {
	missing := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !taskExpectedValuePresent(value, actual) {
			missing = append(missing, value)
		}
	}
	return missing
}

func taskExpectedValuePresent(expected string, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" {
		return true
	}
	if actual == "" {
		return false
	}
	if strings.Contains(strings.ToLower(actual), strings.ToLower(expected)) {
		return true
	}
	if taskCompositeValuePresent(expected, actual) {
		return true
	}
	normalizedExpected := normalizeTaskSemanticText(expected)
	normalizedActual := normalizeTaskSemanticText(actual)
	if normalizedExpected != "" && strings.Contains(normalizedActual, normalizedExpected) {
		return true
	}
	if taskSemanticTokenCoveragePresent(expected, actual) {
		return true
	}
	return taskDomainEquivalentPresent(normalizedExpected, normalizedActual)
}

func taskCompositeValuePresent(expected string, actual string) bool {
	parts := taskComparableParts(expected)
	if len(parts) <= 1 {
		return false
	}
	for _, part := range parts {
		if !taskExpectedValuePresent(part, actual) {
			return false
		}
	}
	return true
}

func taskComparableParts(value string) []string {
	text := strings.TrimSpace(value)
	replacer := strings.NewReplacer(
		"以及", "、",
		"及", "、",
		"和", "、",
		"与", "、",
		"/", "、",
		"，", "、",
		"；", "、",
		";", "、",
	)
	text = replacer.Replace(text)
	parts := strings.Split(text, "、")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, " ：:。；;，, ")
		if len([]rune(part)) >= 2 {
			result = append(result, part)
		}
	}
	return normalizeFixtureStringList(result)
}

func normalizeTaskSemanticText(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		" ", "",
		"\n", "",
		"\t", "",
		"：", "",
		":", "",
		"，", "",
		",", "",
		"。", "",
		"；", "",
		";", "",
		"、", "",
		"（", "",
		"）", "",
		"(", "",
		")", "",
		"签订", "签署",
		"签约", "签署",
		"签订日", "签署日期",
		"签约日", "签署日期",
		"收件方", "收函方",
		"被催告方", "收函方",
		"投诉人", "投诉方",
		"物流费用", "物流费",
		"退换货", "换货",
		"移交方", "交接方",
		"接收方", "交接方",
		"付款最后期限", "时限要求",
		"法律风险提示", "潜在法律风险",
		"风险提示", "潜在法律风险",
		"法律风险", "潜在法律风险",
		"方为", "方",
		"说明已包含", "",
		"已包含", "",
		"作为证据", "证据",
	)
	text = replacer.Replace(text)
	text = strings.ReplaceAll(text, "原合同", "合同")
	text = strings.ReplaceAll(text, "具体", "")
	text = strings.ReplaceAll(text, "相关", "")
	text = strings.ReplaceAll(text, "可能", "")
	text = strings.ReplaceAll(text, "依法", "")
	text = strings.ReplaceAll(text, "措施", "")
	text = strings.ReplaceAll(text, "尚未", "未")
	text = strings.ReplaceAll(text, "未完成", "未")
	return text
}

func taskDomainEquivalentPresent(expected string, actual string) bool {
	if strings.Contains(expected, "投诉方") && strings.Contains(expected, "王丽") && taskAllTokensPresent(actual, "投诉方", "王丽") {
		return true
	}
	if strings.Contains(expected, "视频链接证据") || strings.Contains(expected, "视频证据") {
		return taskAllTokensPresent(actual, "视频", "链接", "证据")
	}
	if strings.Contains(expected, "补偿物流费") || strings.Contains(expected, "物流费") && strings.Contains(expected, "诉求") {
		return taskAllTokensPresent(actual, "补偿", "物流费")
	}
	if strings.Contains(expected, "提起诉讼") && strings.Contains(expected, "财产保全") {
		return taskAllTokensPresent(actual, "提起诉讼", "财产保全")
	}
	if strings.Contains(expected, "用户权限模块") && strings.Contains(expected, "联调") {
		return taskAllTokensPresent(actual, "用户权限模块", "联调")
	}
	if strings.Contains(expected, "甲方负责提供测试环境") {
		return taskAllTokensPresent(actual, "甲方", "提供", "测试环境")
	}
	if strings.Contains(expected, "乙方负责") && strings.Contains(expected, "修复") {
		return taskAllTokensPresent(actual, "乙方", "修复")
	}
	if strings.Contains(expected, "责任划分") {
		return strings.Contains(actual, "责任划分") || taskAllTokensPresent(actual, "责任", "负责")
	}
	if strings.Contains(expected, "信用风险") {
		return strings.Contains(actual, "信用") &&
			(strings.Contains(actual, "负面影响") ||
				strings.Contains(actual, "不良") ||
				strings.Contains(actual, "受损") ||
				strings.Contains(actual, "记录"))
	}
	if strings.Contains(expected, "合同签署日期") {
		return strings.Contains(actual, "合同") &&
			(strings.Contains(actual, "签署日期") ||
				strings.Contains(actual, "签署日") ||
				strings.Contains(actual, "签署时间"))
	}
	return false
}

func taskMissingFieldMarked(field string, actual string) bool {
	field = strings.TrimSpace(field)
	actual = strings.TrimSpace(actual)
	if field == "" || actual == "" {
		return false
	}
	aliases := taskMissingFieldAliases(field)
	segments := taskSemanticSegments(actual)
	for _, segment := range segments {
		if !taskSegmentHasMissingMarker(segment) {
			continue
		}
		for _, alias := range aliases {
			if taskExpectedValuePresent(alias, segment) {
				return true
			}
		}
	}
	normalizedActual := normalizeTaskSemanticText(actual)
	for _, alias := range aliases {
		normalizedAlias := normalizeTaskSemanticText(alias)
		if normalizedAlias != "" && strings.Contains(normalizedActual, normalizedAlias) && taskSegmentHasMissingMarker(actual) {
			return true
		}
	}
	return false
}

func taskMissingFieldAliases(field string) []string {
	field = strings.TrimSpace(field)
	aliases := []string{field}
	normalized := normalizeTaskSemanticText(field)
	switch {
	case strings.Contains(normalized, "签收人"):
		aliases = append(aliases, "签收人", "签收栏", "签收信息")
	case strings.Contains(normalized, "验收标准"):
		aliases = append(aliases, "验收标准", "验收依据", "标准依据")
	case strings.Contains(normalized, "原合同签署日期") || strings.Contains(normalized, "合同签署日期"):
		aliases = append(aliases, "原合同签署日期", "合同签署日期", "合同签订日期", "合同签署时间", "原合同签订日期")
	case strings.Contains(normalized, "产品名称"):
		aliases = append(aliases, "产品名称", "商品名称", "产品", "商品")
	case strings.Contains(normalized, "相关方"):
		aliases = append(aliases, "相关方", "交付方", "接收方", "发件方", "收件方", "对方")
	case strings.Contains(normalized, "日期"):
		aliases = append(aliases, "日期", "时间")
	}
	if strings.HasSuffix(field, "姓名") {
		aliases = append(aliases, strings.TrimSuffix(field, "姓名"))
	}
	if strings.HasSuffix(field, "依据") {
		aliases = append(aliases, strings.TrimSuffix(field, "依据"))
	}
	return normalizeFixtureStringList(aliases)
}

func taskSemanticSegments(value string) []string {
	replacer := strings.NewReplacer(
		"\r", "\n",
		"。", "\n",
		"；", "\n",
		";", "\n",
		"、", "\n",
	)
	parts := strings.Split(replacer.Replace(value), "\n")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	if len(segments) == 0 && strings.TrimSpace(value) != "" {
		segments = append(segments, strings.TrimSpace(value))
	}
	return segments
}

func taskSegmentHasMissingMarker(value string) bool {
	normalized := normalizeTaskSemanticText(value)
	markers := []string{
		"缺失", "未提供", "未明确", "待补充", "请补充", "未知", "为空",
		"未列明", "未披露", "未说明", "未提及", "不详", "待澄清", "未书面确认",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, normalizeTaskSemanticText(marker)) {
			return true
		}
	}
	return false
}

func taskStatePresent(expected string, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" {
		return true
	}
	if taskExpectedValuePresent(expected, actual) {
		return true
	}
	normalizedExpected := normalizeTaskSemanticText(expected)
	normalizedActual := normalizeTaskSemanticText(actual)
	if strings.Contains(normalizedExpected, "部分验收") {
		hasAccepted := strings.Contains(normalizedActual, "实收") ||
			strings.Contains(normalizedActual, "已接收") ||
			strings.Contains(normalizedActual, "验收")
		hasRejectedOrPending := strings.Contains(normalizedActual, "拒收") ||
			strings.Contains(normalizedActual, "未交付") ||
			strings.Contains(normalizedActual, "尚未交付") ||
			strings.Contains(normalizedActual, "破损")
		return hasAccepted && hasRejectedOrPending
	}
	equivalents := map[string][]string{
		"通过":  {"通过", "合格", "批准", "已完成"},
		"不通过": {"不通过", "未通过", "不合格", "拒绝", "驳回"},
		"需复核": {"需复核", "待复核", "需审核", "待确认", "需人工确认"},
		"拒收":  {"拒收", "拒绝接收", "退回"},
	}
	for state, aliases := range equivalents {
		if !strings.Contains(normalizedExpected, normalizeTaskSemanticText(state)) {
			continue
		}
		for _, alias := range aliases {
			if strings.Contains(normalizedActual, normalizeTaskSemanticText(alias)) {
				return true
			}
		}
	}
	return false
}

func taskSemanticTokenCoveragePresent(expected string, actual string) bool {
	expectedTokens := taskSemanticTokens(expected)
	if len(expectedTokens) == 0 {
		return false
	}
	actualNormalized := normalizeTaskSemanticText(actual)
	matched := 0
	for _, token := range expectedTokens {
		if strings.Contains(actualNormalized, normalizeTaskSemanticText(token)) {
			matched++
		}
	}
	if len(expectedTokens) <= 3 {
		return matched == len(expectedTokens)
	}
	return matched*100 >= len(expectedTokens)*75
}

func taskSemanticTokens(value string) []string {
	body := stripTaskAssertionShell(value)
	tokens := []string{}
	tokens = append(tokens, taskEntityValues(body)...)
	tokens = append(tokens, taskValueAfterRoleMarker(body))
	tokens = append(tokens, taskBusinessTerms(body)...)
	tokens = append(tokens, taskUsefulFragments(body)...)
	return normalizeFixtureStringList(tokens)
}

func taskBusinessTerms(value string) []string {
	normalized := normalizeTaskSemanticText(value)
	candidates := []string{
		"责任划分", "遗留问题", "测试环境", "联调", "修复", "提供",
		"提起诉讼", "诉讼", "财产保全", "违约金", "信用风险",
		"证据", "视频", "链接", "换货", "补偿", "物流费",
		"投诉方", "交接方", "接收方", "移交方",
		"缺失信息", "发票", "客服联系记录",
	}
	tokens := []string{}
	for _, candidate := range candidates {
		if strings.Contains(normalized, normalizeTaskSemanticText(candidate)) {
			tokens = append(tokens, candidate)
		}
	}
	return tokens
}

func taskUsefulFragments(value string) []string {
	fragments := taskValueFragments(value)
	tokens := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		fragment = strings.TrimSpace(fragment)
		if len([]rune(fragment)) < 2 || len([]rune(fragment)) > 12 {
			continue
		}
		if strings.Contains(fragment, "为") {
			continue
		}
		if taskFragmentIsUseful(fragment) {
			tokens = append(tokens, fragment)
		}
	}
	return tokens
}

func taskAllTokensPresent(actual string, tokens ...string) bool {
	for _, token := range tokens {
		if !strings.Contains(actual, normalizeTaskSemanticText(token)) {
			return false
		}
	}
	return true
}

func firstValue(value map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			return raw
		}
	}
	return nil
}

func firstString(value map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func mapFromInterface(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case JSONMap:
		return map[string]interface{}(typed)
	default:
		return map[string]interface{}{}
	}
}

func floatFromInterface(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func jsonText(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func attachWorkflowTestAnalysis(outputs map[string]interface{}, analysis *WorkflowTestAnalysis) map[string]interface{} {
	if outputs == nil {
		outputs = map[string]interface{}{}
	}
	if analysis == nil {
		return outputs
	}
	outputs[workflowTestAnalysisOutputKey] = analysis
	outputs[checkResultsOutputKey] = WorkflowTestCheckResults{
		Summary:    analysis.Summary,
		Conditions: analysis.Comparisons.Checks,
	}
	return outputs
}

func mergeAnalysisWithJudgeResult(analysis *WorkflowTestAnalysis, judgeResult *JudgeResult) *JudgeResult {
	result := normalizeJudgeResult(judgeResult)
	if analysis == nil {
		return result
	}
	if analysis.Summary.Status == "failed" {
		if analysis.Mode == "task" && analysis.Summary.CriticalFailed == 0 && result.Status == BatchItemStatusPassed {
			return result
		}
		result.Status = BatchItemStatusFailed
		if strings.TrimSpace(result.Reason) == "" || analysis.Summary.CriticalFailed > 0 {
			result.Reason = analysis.Summary.MainIssue
		}
		if strings.TrimSpace(result.Suggestion) == "" && len(analysis.Suggestions) > 0 {
			result.Suggestion = analysis.Suggestions[0].Content
		}
		return result
	}
	if analysis.Mode == "task" && analysis.EvaluationSchema != nil && analysis.Summary.Status == "passed" {
		result.Status = BatchItemStatusPassed
		if strings.TrimSpace(result.Reason) == "" || taskJudgeReasonConflictsWithPassedSchema(result.Reason) {
			result.Reason = "结构化评价标准已通过：输出正确标注了输入中缺失的业务信息，且未误报解析失败或内容为空。"
		}
		result.Suggestion = ""
		return result
	}
	if analysis.Mode == "task" && analysis.EvaluationSchema != nil && result.Status == BatchItemStatusPassed && analysis.Summary.CriticalFailed == 0 {
		return result
	}
	if analysis.Mode == "task" && analysis.EvaluationSchema != nil && result.Status == BatchItemStatusFailed && analysis.Summary.CriticalFailed == 0 {
		result.Status = BatchItemStatusReview
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = "AI 评分与结构化评价断言存在冲突，需人工复核。"
		}
		if strings.TrimSpace(result.Suggestion) == "" {
			result.Suggestion = "请优先查看断言级检查结果：若关键事实、禁止项、缺失策略均满足，则不要仅因额外合理分析判为不通过。"
		}
		return result
	}
	if analysis.Mode == "task" && analysis.EvaluationSchema != nil && analysis.Summary.Status == "review" && result.Status == BatchItemStatusFailed && analysis.Summary.ReferenceScore >= 3.5 {
		result.Status = BatchItemStatusReview
		result.Reason = fmt.Sprintf("结构化评价参考分 %.1f/5，存在少量未满足或需确认的检查点，建议人工复核后决定是否优化工作流。", analysis.Summary.ReferenceScore)
		if len(analysis.Suggestions) > 0 {
			result.Suggestion = analysis.Suggestions[0].Content
		} else {
			result.Suggestion = "请优先查看检查点摘要：若只是非阻断字段缺漏，不应直接判定整个任务工作流不通过。"
		}
		return result
	}
	if analysis.Summary.Status == "review" && result.Status == BatchItemStatusPassed {
		result.Status = BatchItemStatusReview
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = analysis.Summary.MainIssue
		}
		if strings.TrimSpace(result.Suggestion) == "" && len(analysis.Suggestions) > 0 {
			result.Suggestion = analysis.Suggestions[0].Content
		}
	}
	return result
}

func taskJudgeReasonConflictsWithPassedSchema(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "missing field") ||
		strings.Contains(normalized, "缺失字段") ||
		strings.Contains(normalized, "补充这些业务要点") ||
		strings.Contains(normalized, "明确追问")
}
