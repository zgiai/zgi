package workflowtest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const evaluationSchemaInputKey = "__evaluation_schema"

type TaskEvaluationSchema struct {
	GoalType          string                    `json:"goal_type,omitempty"`
	PrimaryObjective  string                    `json:"primary_objective,omitempty"`
	Assertions        []TaskEvaluationAssertion `json:"assertions,omitempty"`
	MissingPolicy     TaskMissingPolicy         `json:"missing_policy,omitempty"`
	AllowedExtraTypes []string                  `json:"allowed_extra_types,omitempty"`
	Format            TaskFormatExpectation     `json:"format,omitempty"`
	SourceGrounding   string                    `json:"source_grounding,omitempty"`
}

type TaskEvaluationAssertion struct {
	ID          string   `json:"id,omitempty"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Values      []string `json:"values,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	MatchMode   string   `json:"match_mode,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type TaskMissingPolicy struct {
	Mode           string   `json:"mode,omitempty"`
	AcceptMarkers  []string `json:"accept_markers,omitempty"`
	ForbidClaims   []string `json:"forbid_claims,omitempty"`
	ClarifyAllowed bool     `json:"clarify_allowed,omitempty"`
}

type TaskFormatExpectation struct {
	Type     string   `json:"type,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	Strict   bool     `json:"strict,omitempty"`
	Markdown bool     `json:"markdown,omitempty"`
}

func taskEvaluationSchemaFromInput(value interface{}, fallbackExpected string, checks TaskExpectedChecks) TaskEvaluationSchema {
	schema := normalizeTaskEvaluationSchema(taskEvaluationSchemaValue(value))
	if schema.GoalType == "" {
		schema.GoalType = inferTaskGoalType(fallbackExpected, checks)
	}
	if schema.PrimaryObjective == "" {
		schema.PrimaryObjective = strings.TrimSpace(fallbackExpected)
	}
	if len(schema.Assertions) == 0 {
		schema.Assertions = assertionsFromTaskChecks(checks)
	}
	if schema.MissingPolicy.Mode == "" {
		schema.MissingPolicy = defaultTaskMissingPolicy()
	}
	if len(schema.AllowedExtraTypes) == 0 {
		schema.AllowedExtraTypes = []string{"reasonable_analysis", "risk_notes", "missing_business_fields", "next_steps", "formatting"}
	}
	if schema.SourceGrounding == "" {
		schema.SourceGrounding = "input_supported"
	}
	schema.Assertions = ensureTaskSchemaPolicyAssertions(schema.Assertions, schema)
	return schema
}

func taskEvaluationSchemaValue(value interface{}) TaskEvaluationSchema {
	if value == nil {
		return TaskEvaluationSchema{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return TaskEvaluationSchema{}
	}
	var schema TaskEvaluationSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return TaskEvaluationSchema{}
	}
	return schema
}

func normalizeTaskEvaluationSchema(schema TaskEvaluationSchema) TaskEvaluationSchema {
	schema.GoalType = normalizeTaskGoalType(schema.GoalType)
	schema.PrimaryObjective = strings.TrimSpace(schema.PrimaryObjective)
	schema.SourceGrounding = strings.TrimSpace(schema.SourceGrounding)
	schema.AllowedExtraTypes = normalizeFixtureStringList(schema.AllowedExtraTypes)
	schema.MissingPolicy.Mode = strings.TrimSpace(schema.MissingPolicy.Mode)
	schema.MissingPolicy.AcceptMarkers = normalizeFixtureStringList(schema.MissingPolicy.AcceptMarkers)
	schema.MissingPolicy.ForbidClaims = normalizeFixtureStringList(schema.MissingPolicy.ForbidClaims)
	schema.Format.Type = strings.TrimSpace(schema.Format.Type)
	schema.Format.Fields = normalizeFixtureStringList(schema.Format.Fields)
	normalized := make([]TaskEvaluationAssertion, 0, len(schema.Assertions))
	seenAssertions := map[string]struct{}{}
	for _, assertion := range schema.Assertions {
		for _, expanded := range normalizeTaskEvaluationAssertion(assertion) {
			if expanded.Type == "" && expanded.Description == "" && len(expanded.Values) == 0 {
				continue
			}
			if expanded.ID == "" {
				expanded.ID = makeTaskEvaluationAssertionID(expanded, len(normalized)+1)
			}
			key := taskEvaluationAssertionKey(expanded)
			if _, exists := seenAssertions[key]; exists {
				continue
			}
			seenAssertions[key] = struct{}{}
			normalized = append(normalized, expanded)
		}
	}
	schema.Assertions = normalized
	return schema
}

func normalizeTaskEvaluationAssertion(assertion TaskEvaluationAssertion) []TaskEvaluationAssertion {
	assertion.ID = strings.TrimSpace(assertion.ID)
	assertion.Type = normalizeTaskAssertionType(assertion.Type)
	assertion.Description = strings.TrimSpace(assertion.Description)
	assertion.Values = normalizeFixtureStringList(assertion.Values)
	assertion.Operator = normalizeTaskAssertionOperator(assertion.Operator)
	assertion.Severity = normalizeTaskSeverity(assertion.Severity)
	assertion.MatchMode = normalizeTaskMatchMode(assertion.MatchMode)
	assertion.Source = strings.TrimSpace(assertion.Source)
	if assertion.Type == "" {
		assertion.Type = "semantic_match"
	}
	if assertion.Operator == "" {
		assertion.Operator = defaultOperatorForAssertion(assertion.Type)
	}
	if assertion.Severity == "" {
		assertion.Severity = "normal"
	}
	if assertion.MatchMode == "" {
		assertion.MatchMode = "semantic"
	}

	clauses := taskAssertionClauses(assertion)
	if len(clauses) == 0 {
		return []TaskEvaluationAssertion{assertion}
	}
	result := make([]TaskEvaluationAssertion, 0, len(clauses))
	for _, clause := range clauses {
		next := assertion
		next.ID = ""
		next.Description = clause
		if isTaskForbiddenAssertionText(clause) {
			next.Type = "must_not_include"
			next.Operator = "not_contains"
			next.Values = taskForbiddenValuesFromText(clause)
			next.MatchMode = "keyword"
		} else if fields := taskMissingFieldValuesFromText(clause); len(fields) > 0 {
			next.Type = "missing_field_marked"
			next.Operator = "marked"
			next.Values = fields
			next.MatchMode = "semantic"
		} else if values := taskStateValuesFromText(clause); len(values) > 0 {
			next.Type = "state_present"
			next.Operator = "contains"
			next.Values = values
			next.MatchMode = "semantic"
		} else if isTaskMetaAvailabilityAssertionText(clause) {
			next.Type = "semantic_match"
			next.Operator = "satisfies"
			next.Values = nil
			next.MatchMode = "semantic"
			if next.Severity == "critical" {
				next.Severity = "normal"
			}
		} else if next.Type == "must_include" {
			if values := taskRequiredValuesFromText(clause); len(values) > 0 {
				next.Values = values
				next.Operator = "contains"
				next.MatchMode = "keyword"
			}
		}
		if isOptionalExampleAssertionText(clause) {
			next.Severity = "hint"
			next.MatchMode = "semantic"
			next.Source = "optional_example"
		}
		if next.Type == "must_not_include" && len(next.Values) == 0 {
			next.Values = taskForbiddenValuesFromText(clause)
		}
		if next.Type == "must_include" && len(next.Values) == 0 {
			next.Values = []string{clause}
		}
		result = append(result, next)
	}
	return result
}

func taskEvaluationAssertionKey(assertion TaskEvaluationAssertion) string {
	return strings.Join([]string{
		assertion.Type,
		assertion.Operator,
		assertion.MatchMode,
		strings.Join(assertion.Values, "|"),
	}, "::")
}

func taskAssertionClauses(assertion TaskEvaluationAssertion) []string {
	values := assertion.Values
	if len(values) == 0 && assertion.Description != "" {
		values = []string{assertion.Description}
	}
	clauses := make([]string, 0, len(values))
	for _, value := range values {
		parts := splitTaskAssertionClauses(value)
		if len(parts) == 0 {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				clauses = append(clauses, trimmed)
			}
			continue
		}
		clauses = append(clauses, parts...)
	}
	return normalizeFixtureStringList(clauses)
}

func splitTaskAssertionClauses(value string) []string {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	replacements := []struct {
		old string
		new string
	}{
		{"、输出应", "\n输出应"},
		{"，输出应", "\n输出应"},
		{"；输出应", "\n输出应"},
		{";输出应", "\n输出应"},
		{"、不得", "\n不得"},
		{"，不得", "\n不得"},
		{"；不得", "\n不得"},
		{";不得", "\n不得"},
		{"、不得声称", "\n不得声称"},
	}
	for _, item := range replacements {
		text = strings.ReplaceAll(text, item.old, item.new)
	}
	parts := strings.Split(text, "\n")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isTaskForbiddenAssertionText(value string) bool {
	text := strings.TrimSpace(value)
	return strings.Contains(text, "不得") ||
		strings.Contains(text, "不应") ||
		strings.Contains(text, "不能") ||
		strings.Contains(text, "禁止")
}

func isOptionalExampleAssertionText(value string) bool {
	text := strings.TrimSpace(value)
	return strings.Contains(text, "如：") ||
		strings.Contains(text, "如:") ||
		strings.Contains(text, "例如") ||
		strings.Contains(text, "比如") ||
		strings.Contains(text, "（如") ||
		strings.Contains(text, "(如") ||
		strings.Contains(text, "包括但不限于") ||
		isTaskCommonMissingExample(text)
}

func isTaskCommonMissingExample(text string) bool {
	return strings.Contains(text, "缺失") &&
		strings.Contains(text, "发票") &&
		strings.Contains(text, "客服联系记录")
}

func isTaskMissingFieldAssertionText(value string) bool {
	text := strings.TrimSpace(value)
	if strings.Contains(text, "缺失信息") ||
		strings.Contains(text, "缺失条款") ||
		strings.Contains(text, "待补充信息") ||
		strings.Contains(text, "待澄清信息") {
		return true
	}
	return strings.Contains(text, "缺失") &&
		(strings.Contains(text, "标注") ||
			strings.Contains(text, "列出") ||
			strings.Contains(text, "明确") ||
			strings.Contains(text, "指出"))
}

func taskMissingFieldValuesFromText(value string) []string {
	if !isTaskMissingFieldAssertionText(value) {
		return nil
	}
	body := stripTaskAssertionShell(value)
	if index := strings.LastIndex(body, "："); index >= 0 && index+len("：") < len(body) {
		body = strings.TrimSpace(body[index+len("："):])
	} else if index := strings.LastIndex(body, ":"); index >= 0 && index+1 < len(body) {
		body = strings.TrimSpace(body[index+1:])
	}
	replacers := []string{
		"缺失信息包括", "缺失信息", "缺失条款包括", "缺失条款", "待补充信息", "待澄清信息",
		"应标注", "标注", "明确列出", "明确指出", "列出", "指出", "包括",
	}
	for _, prefix := range replacers {
		body = strings.TrimSpace(strings.TrimPrefix(body, prefix))
	}
	fields := taskValueFragments(body)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = cleanTaskMissingFieldName(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return normalizeFixtureStringList(result)
}

func cleanTaskMissingFieldName(value string) string {
	text := strings.TrimSpace(value)
	replacers := []struct {
		old string
		new string
	}{
		{"缺失", ""},
		{"未提供", ""},
		{"未明确", ""},
		{"待补充", ""},
		{"待澄清", ""},
		{"需补充", ""},
		{"需明确", ""},
		{"例如", ""},
		{"比如", ""},
		{"如", ""},
	}
	for _, item := range replacers {
		text = strings.ReplaceAll(text, item.old, item.new)
	}
	text = strings.Trim(text, " ：:。；;，,、")
	if len([]rune(text)) < 2 {
		return ""
	}
	return text
}

func taskStateValuesFromText(value string) []string {
	text := strings.TrimSpace(value)
	stateMarkers := []string{"验收状态", "处理状态", "审批结果", "分类结果", "风险等级", "状态"}
	hasStateMarker := false
	for _, marker := range stateMarkers {
		if strings.Contains(text, marker) {
			hasStateMarker = true
			break
		}
	}
	if !hasStateMarker {
		return nil
	}
	states := []string{"部分验收", "全部验收", "验收通过", "验收不通过", "通过", "不通过", "需复核", "待处理", "已处理", "拒收"}
	result := []string{}
	for _, state := range states {
		if strings.Contains(text, state) {
			result = append(result, state)
		}
	}
	return normalizeFixtureStringList(result)
}

func isTaskMetaAvailabilityAssertionText(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" || !strings.Contains(text, "已提供") {
		return false
	}
	return !taskHasConcreteEntityValue(text)
}

func taskHasConcreteEntityValue(value string) bool {
	if len(taskEntityValues(value)) > 0 {
		return true
	}
	for _, fragment := range taskValueFragments(value) {
		if taskFragmentLooksConcrete(fragment) {
			return true
		}
	}
	return false
}

func taskFragmentLooksConcrete(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" || strings.Contains(text, "已提供") {
		return false
	}
	if len([]rune(text)) >= 4 && !strings.Contains(text, "名称") && !strings.Contains(text, "编号") && !strings.Contains(text, "字段") {
		return true
	}
	return false
}

func taskForbiddenValuesFromText(value string) []string {
	body := stripTaskAssertionShell(value)
	body = strings.TrimPrefix(body, "声称")
	body = strings.TrimPrefix(body, "出现")
	body = strings.TrimPrefix(body, "返回")
	body = strings.TrimPrefix(body, "提示")
	return taskValueFragments(body)
}

func taskRequiredValuesFromText(value string) []string {
	body := stripTaskAssertionShell(value)
	values := taskEntityValues(body)
	values = append(values, taskDerivedAtomicValues(body)...)
	fragments := taskValueFragments(body)
	for _, fragment := range fragments {
		if taskFragmentContainsEntity(fragment, values) {
			continue
		}
		if taskFragmentIsUseful(fragment) {
			values = append(values, fragment)
		}
	}
	return normalizeFixtureStringList(values)
}

func taskDerivedAtomicValues(value string) []string {
	text := strings.TrimSpace(value)
	values := []string{}
	if name := taskValueAfterRoleMarker(text); name != "" {
		values = append(values, name)
	}
	if strings.Contains(text, "责任划分") {
		values = append(values, "责任划分")
	}
	if strings.Contains(text, "视频") && strings.Contains(text, "证据") {
		values = append(values, "视频证据")
	}
	if strings.Contains(text, "换货") {
		values = append(values, "换货")
	}
	if strings.Contains(text, "物流费") {
		values = append(values, "物流费")
	}
	if strings.Contains(text, "财产保全") {
		values = append(values, "财产保全")
	}
	if strings.Contains(text, "提起诉讼") {
		values = append(values, "提起诉讼")
	}
	if strings.Contains(text, "用户权限模块") {
		values = append(values, "用户权限模块")
	}
	if strings.Contains(text, "联调") {
		values = append(values, "联调")
	}
	if strings.Contains(text, "测试环境") {
		values = append(values, "测试环境")
	}
	return normalizeFixtureStringList(values)
}

func taskValueAfterRoleMarker(value string) string {
	markers := []string{"投诉方为", "投诉人为", "客户为", "甲方为", "乙方为", "交接方为", "接收方为"}
	for _, marker := range markers {
		index := strings.Index(value, marker)
		if index < 0 {
			continue
		}
		after := strings.TrimSpace(value[index+len(marker):])
		after = strings.Trim(after, " ：:。；;，,、")
		if cut := strings.IndexAny(after, "，,；;、。"); cut >= 0 {
			after = strings.TrimSpace(after[:cut])
		}
		if after != "" && len([]rune(after)) <= 24 {
			return after
		}
	}
	return ""
}

func taskFragmentContainsEntity(fragment string, entities []string) bool {
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity != "" && strings.Contains(fragment, entity) {
			return true
		}
	}
	return false
}

func stripTaskAssertionShell(value string) string {
	text := strings.TrimSpace(value)
	replacers := []string{
		"输出应包含", "输出应标注", "输出应说明", "输出应列出", "输出应指出", "输出应提及", "输出应引用", "输出应明确列出", "输出应明确指出",
		"应包含", "应标注", "应说明", "应列出", "应指出", "应提及", "应引用", "应明确列出", "应明确指出",
		"不得声称", "不得提示", "不得出现", "不得", "不能声称", "不能提示", "不能", "禁止",
		"关键事实陈述涉及", "核心义务包括", "包括", "涉及", "为",
	}
	for _, prefix := range replacers {
		text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	if index := strings.LastIndex(text, "："); index >= 0 && index+len("：") < len(text) {
		text = strings.TrimSpace(text[index+len("："):])
	} else if index := strings.LastIndex(text, ":"); index >= 0 && index+1 < len(text) {
		text = strings.TrimSpace(text[index+1:])
	}
	text = strings.Trim(text, " 。；;，,")
	return text
}

var (
	taskDatePattern     = regexp.MustCompile(`\d{4}年\d{1,2}月\d{1,2}日|\d{1,2}月\d{1,2}日|\d+个工作日`)
	taskMoneyPattern    = regexp.MustCompile(`\d[\d,]*(?:\.\d+)?\s*元`)
	taskCodePattern     = regexp.MustCompile(`[A-Za-z]{2,}[A-Za-z0-9-]*\d[A-Za-z0-9-]*`)
	taskQuantityPattern = regexp.MustCompile(`\d+\s*件`)
	taskOrgPattern      = regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·-]{2,}?(?:有限公司|物流有限公司|服务公司|制造厂|事务所|公司|工厂)`)
)

func taskEntityValues(value string) []string {
	values := []string{}
	patterns := []*regexp.Regexp{taskDatePattern, taskMoneyPattern, taskCodePattern, taskQuantityPattern, taskOrgPattern}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllString(value, -1) {
			if cleaned := cleanTaskEntityValue(match); cleaned != "" {
				values = append(values, cleaned)
			}
		}
	}
	return normalizeFixtureStringList(values)
}

func cleanTaskEntityValue(value string) string {
	text := strings.Trim(strings.TrimSpace(value), "与和及、，；; ")
	prefixes := []string{"甲方为", "乙方为", "甲方", "乙方", "交接方为", "接收方为", "移交方", "接收方", "发函方", "收函方", "投诉方", "投诉人", "为"}
	for _, prefix := range prefixes {
		text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
		text = strings.Trim(text, " ：:。；;，,、")
	}
	return text
}

func taskValueFragments(value string) []string {
	text := strings.TrimSpace(value)
	replacer := strings.NewReplacer(
		"以及", "、",
		"及", "、",
		"和", "、",
		"与", "、",
		"并", "、",
		"其中", "、",
		"或", "、",
		"，", "、",
		"；", "、",
		";", "、",
		"（", "、",
		"）", "、",
		"(", "、",
		")", "、",
	)
	text = replacer.Replace(text)
	parts := strings.Split(text, "、")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = cleanTaskValueFragment(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return normalizeFixtureStringList(values)
}

func cleanTaskValueFragment(value string) string {
	text := strings.TrimSpace(value)
	replacers := []struct {
		old string
		new string
	}{
		{"型号 ", ""},
		{"型号", ""},
		{"共计", ""},
		{"共", ""},
		{"将于", ""},
		{"内付款的时限要求", ""},
		{"付款的时限要求", ""},
		{"的时限要求", ""},
		{"未付款", ""},
		{"未支付", ""},
		{"付款", ""},
		{"支付", ""},
		{"缺乏", ""},
		{"约定", ""},
		{"关键风险如", ""},
		{"缺失条款", ""},
		{"服务标准", "服务验收标准"},
		{"两项诉求", ""},
		{"作为证据", "证据"},
		{"说明已包含", ""},
		{"已包含", ""},
	}
	for _, item := range replacers {
		text = strings.ReplaceAll(text, item.old, item.new)
	}
	text = strings.Trim(text, " ：:。；;，,")
	if strings.Contains(text, "按月") {
		return "按月"
	}
	return strings.TrimSpace(text)
}

func taskFragmentIsUseful(value string) bool {
	text := strings.TrimSpace(value)
	if len([]rune(text)) < 2 {
		return false
	}
	if strings.Contains(text, "已提供") && !taskHasConcreteEntityValue(text) {
		return false
	}
	stop := map[string]struct{}{
		"输出": {}, "应": {}, "包含": {}, "说明": {}, "列出": {}, "指出": {}, "提及": {}, "引用": {},
		"关键事实陈述": {}, "核心义务": {}, "关键风险": {}, "缺失条款": {}, "内容": {}, "要求": {},
	}
	if _, ok := stop[text]; ok {
		return false
	}
	return !strings.Contains(text, "输出应")
}

func assertionsFromTaskChecks(checks TaskExpectedChecks) []TaskEvaluationAssertion {
	conditions := normalizeExpectedCheckConditions(checks.Conditions, checks)
	result := make([]TaskEvaluationAssertion, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Type == "latency" || condition.Type == "node" || condition.Type == "capability" {
			continue
		}
		if condition.Type == "output_contains" && valuesAreMissingFieldExpectations(condition.Values) {
			continue
		}
		result = append(result, TaskEvaluationAssertion{
			ID:          condition.ID,
			Type:        "must_include",
			Description: checkConditionLabel(condition),
			Values:      condition.Values,
			Operator:    "contains",
			Severity:    normalizeTaskSeverity(condition.Severity),
			MatchMode:   normalizeTaskMatchMode(condition.MatchMode),
			Source:      firstNonEmptyString(condition.Source, "expected_checks"),
		})
	}
	return result
}

func valuesAreMissingFieldExpectations(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !isMissingFieldExpectationValue(value) {
			return false
		}
	}
	return true
}

func isMissingFieldExpectationValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "missing field:") ||
		strings.HasPrefix(normalized, "missing fields:") ||
		strings.HasPrefix(normalized, "缺失字段：") ||
		strings.HasPrefix(normalized, "缺失字段:")
}

func ensureTaskSchemaPolicyAssertions(assertions []TaskEvaluationAssertion, schema TaskEvaluationSchema) []TaskEvaluationAssertion {
	hasMissingPolicy := false
	for _, assertion := range assertions {
		if assertion.Type == "missing_policy" {
			hasMissingPolicy = true
			break
		}
	}
	if !hasMissingPolicy && (len(schema.MissingPolicy.AcceptMarkers) > 0 || len(schema.MissingPolicy.ForbidClaims) > 0) {
		assertions = append(assertions, TaskEvaluationAssertion{
			ID:          "missing_policy",
			Type:        "missing_policy",
			Description: "缺失信息处理策略",
			Operator:    "satisfies",
			Severity:    "normal",
			MatchMode:   "semantic",
			Source:      "system_policy",
		})
	}
	return assertions
}

func defaultTaskMissingPolicy() TaskMissingPolicy {
	return TaskMissingPolicy{
		Mode: "explicit_markers_allowed",
		AcceptMarkers: []string{
			"缺失", "未提供", "未明确", "待补充", "请补充", "未知", "N/A", "无",
			"missing", "not provided", "unknown", "to be supplied",
		},
		ForbidClaims: []string{
			"文件无法读取", "内容为空", "解析失败", "格式不支持", "需要转人工", "请重新上传",
			"unreadable", "empty file", "parse failed", "manual handling required",
		},
	}
}

func inferTaskGoalType(expected string, checks TaskExpectedChecks) string {
	text := strings.ToLower(strings.TrimSpace(expected))
	switch {
	case strings.Contains(text, "分类") || strings.Contains(text, "分级") || strings.Contains(text, "等级") || strings.Contains(text, "class"):
		return "classify"
	case strings.Contains(text, "判断") || strings.Contains(text, "是否") || strings.Contains(text, "审批") || strings.Contains(text, "decision"):
		return "decision"
	case strings.Contains(text, "分析") || strings.Contains(text, "风险") || strings.Contains(text, "建议") || strings.Contains(text, "洞察"):
		return "analyze"
	case strings.Contains(text, "调用") || strings.Contains(text, "接口") || strings.Contains(text, "http") || strings.Contains(text, "工具"):
		return "action"
	case strings.Contains(text, "转换") || strings.Contains(text, "摘要") || strings.Contains(text, "总结") || strings.Contains(text, "格式"):
		return "transform"
	case len(checks.OutputContains) > 0:
		return "extract"
	default:
		return "general"
	}
}

func normalizeTaskGoalType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "extract", "classify", "transform", "analyze", "decision", "action", "general":
		return strings.ToLower(strings.TrimSpace(value))
	case "extraction":
		return "extract"
	case "classification":
		return "classify"
	case "analysis":
		return "analyze"
	default:
		return ""
	}
}

func normalizeTaskAssertionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "must_include", "must_not_include", "format", "missing_policy", "source_grounding", "action_result", "semantic_match", "fact_present", "missing_field_marked", "state_present":
		return strings.ToLower(strings.TrimSpace(value))
	case "required_fact", "required_facts", "include":
		return "fact_present"
	case "missing_field", "missing_fields", "business_missing_field":
		return "missing_field_marked"
	case "state", "status", "state_match":
		return "state_present"
	case "forbidden", "forbidden_claim", "forbidden_claims", "not_include":
		return "must_not_include"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeTaskAssertionOperator(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "contains", "not_contains", "satisfies", "equals", "matches", "valid", "marked":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeTaskSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "normal", "hint":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "normal"
	}
}

func normalizeTaskMatchMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "keyword", "semantic":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "semantic"
	}
}

func defaultOperatorForAssertion(assertionType string) string {
	switch assertionType {
	case "must_not_include":
		return "not_contains"
	case "format", "missing_policy", "source_grounding", "action_result", "semantic_match", "missing_field_marked":
		return "satisfies"
	default:
		return "contains"
	}
}

func makeTaskEvaluationAssertionID(assertion TaskEvaluationAssertion, index int) string {
	base := assertion.Type
	if base == "" {
		base = "assertion"
	}
	return fmt.Sprintf("%s_%d", base, index)
}
