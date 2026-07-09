package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	minGeneratedCaseCount = 1
	maxGeneratedCaseCount = 50
)

type GenerateCasesRequest struct {
	Count          int                   `json:"count"`
	ScenarioID     string                `json:"scenario_id,omitempty"`
	ScenarioIDs    []string              `json:"scenario_ids,omitempty"`
	Context        string                `json:"context,omitempty"`
	QuestionTypes  []string              `json:"question_types,omitempty"`
	TurnStrategy   string                `json:"turn_strategy,omitempty"`
	CaseMode       string                `json:"case_mode,omitempty"`
	FileGeneration *FileGenerationConfig `json:"file_generation,omitempty"`
	Prompt         string                `json:"prompt,omitempty"`
	Model          *Model                `json:"model,omitempty"`

	WorkflowContext string     `json:"-"`
	Scenarios       []Scenario `json:"-"`
	ExistingCases   []Case     `json:"-"`
	WorkspaceID     string     `json:"-"`
	AccountID       string     `json:"-"`
}

type CreateGenerationTaskRequest = GenerateCasesRequest

type GenerationTaskResponse struct {
	Task *GenerationTask `json:"task"`
}

type GeneratedCase struct {
	ScenarioID         string                     `json:"scenario_id,omitempty"`
	Content            string                     `json:"content"`
	ExpectedResult     string                     `json:"expected_result"`
	QuestionType       string                     `json:"question_type"`
	Turns              []CaseTurn                 `json:"turns,omitempty"`
	FileFixtures       []GeneratedFileFixture     `json:"file_fixtures,omitempty"`
	ConversationChecks ConversationExpectedChecks `json:"conversation_checks,omitempty"`
}

type FileGenerationConfig struct {
	Enabled      bool     `json:"enabled"`
	Formats      []string `json:"formats,omitempty"`
	FilesPerCase int      `json:"files_per_case,omitempty"`
	Complexities []string `json:"complexities,omitempty"`
	ContentTypes []string `json:"content_types,omitempty"`
}

type GeneratedFileFixture struct {
	Filename       string   `json:"filename,omitempty"`
	Format         string   `json:"format,omitempty"`
	Title          string   `json:"title,omitempty"`
	Content        string   `json:"content,omitempty"`
	Description    string   `json:"description,omitempty"`
	Facts          []string `json:"facts,omitempty"`
	ExpectedChecks []string `json:"expected_checks,omitempty"`
}

type ConversationExpectedChecks struct {
	Conditions []ConversationExpectedCheckCondition `json:"conditions,omitempty"`
}

type ConversationExpectedCheckCondition struct {
	ID          string   `json:"id,omitempty"`
	Type        string   `json:"type,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	TargetID    string   `json:"target_id,omitempty"`
	TargetLabel string   `json:"target_label,omitempty"`
	Values      []string `json:"values,omitempty"`
	MatchMode   string   `json:"match_mode,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	Source      string   `json:"source,omitempty"`
}

func (c *GeneratedCase) UnmarshalJSON(data []byte) error {
	type generatedCaseTurnWire struct {
		Role           string           `json:"role"`
		Content        string           `json:"content"`
		Attachments    []CaseAttachment `json:"attachments,omitempty"`
		Inputs         JSONMap          `json:"inputs,omitempty"`
		ExpectedResult string           `json:"expected_result,omitempty"`
		TurnChecks     json.RawMessage  `json:"turn_checks,omitempty"`
	}
	type generatedCaseWire struct {
		ScenarioID         string                  `json:"scenario_id,omitempty"`
		Content            string                  `json:"content"`
		ExpectedResult     string                  `json:"expected_result"`
		QuestionType       string                  `json:"question_type"`
		Turns              []generatedCaseTurnWire `json:"turns,omitempty"`
		FileFixtures       []GeneratedFileFixture  `json:"file_fixtures,omitempty"`
		ConversationChecks json.RawMessage         `json:"conversation_checks,omitempty"`
	}
	var raw generatedCaseWire
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.ScenarioID = raw.ScenarioID
	c.Content = raw.Content
	c.ExpectedResult = raw.ExpectedResult
	c.QuestionType = raw.QuestionType
	c.FileFixtures = raw.FileFixtures
	c.ConversationChecks = conversationExpectedChecksValue(raw.ConversationChecks)
	c.Turns = make([]CaseTurn, 0, len(raw.Turns))
	for _, turn := range raw.Turns {
		inputs := turn.Inputs
		expected := strings.TrimSpace(turn.ExpectedResult)
		checks := conversationExpectedChecksValue(turn.TurnChecks)
		if inputs == nil && (expected != "" || checks.Useful()) {
			inputs = JSONMap{}
		}
		if inputs != nil && expected != "" {
			inputs[turnExpectationInputKey] = expected
		}
		if inputs != nil && checks.Useful() {
			inputs[turnChecksInputKey] = checks.JSONMap()
		}
		c.Turns = append(c.Turns, CaseTurn{
			Role:        turn.Role,
			Content:     turn.Content,
			Attachments: turn.Attachments,
			Inputs:      inputs,
		})
	}
	return nil
}

func (c *ConversationExpectedCheckCondition) UnmarshalJSON(data []byte) error {
	type rawCondition struct {
		ID          json.RawMessage `json:"id"`
		Type        json.RawMessage `json:"type"`
		Operator    json.RawMessage `json:"operator"`
		TargetID    json.RawMessage `json:"target_id"`
		TargetLabel json.RawMessage `json:"target_label"`
		Values      json.RawMessage `json:"values"`
		MatchMode   json.RawMessage `json:"match_mode"`
		Severity    json.RawMessage `json:"severity"`
		Source      json.RawMessage `json:"source"`
	}
	var raw rawCondition
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.ID = fixtureStringValue(raw.ID)
	c.Type = fixtureStringValue(raw.Type)
	c.Operator = fixtureStringValue(raw.Operator)
	c.TargetID = fixtureStringValue(raw.TargetID)
	c.TargetLabel = fixtureStringValue(raw.TargetLabel)
	c.Values = fixtureStringListValue(raw.Values)
	c.MatchMode = fixtureStringValue(raw.MatchMode)
	c.Severity = fixtureStringValue(raw.Severity)
	c.Source = fixtureStringValue(raw.Source)
	return nil
}

func (f *GeneratedFileFixture) UnmarshalJSON(data []byte) error {
	type rawFixture struct {
		Filename       json.RawMessage `json:"filename"`
		Format         json.RawMessage `json:"format"`
		Title          json.RawMessage `json:"title"`
		Content        json.RawMessage `json:"content"`
		Description    json.RawMessage `json:"description"`
		Facts          json.RawMessage `json:"facts"`
		ExpectedChecks json.RawMessage `json:"expected_checks"`
	}
	var raw rawFixture
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Filename = fixtureStringValue(raw.Filename)
	f.Format = fixtureStringValue(raw.Format)
	f.Title = fixtureStringValue(raw.Title)
	f.Content = fixtureStringValue(raw.Content)
	f.Description = fixtureStringValue(raw.Description)
	f.Facts = fixtureStringListValue(raw.Facts)
	f.ExpectedChecks = fixtureStringListValue(raw.ExpectedChecks)
	return nil
}

type GenerateCasesResult struct {
	Cases []GeneratedCase `json:"cases"`
	Items []Case          `json:"items,omitempty"`
}

type CaseGenerator interface {
	GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error)
}

func normalizeGeneratedCases(result *GenerateCasesResult) ([]GeneratedCase, error) {
	if result == nil || len(result.Cases) == 0 {
		return nil, fmt.Errorf("generated cases are empty")
	}
	seen := map[string]struct{}{}
	items := make([]GeneratedCase, 0, len(result.Cases))
	for _, item := range result.Cases {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		if _, exists := seen[content]; exists {
			continue
		}
		seen[content] = struct{}{}
		questionType := strings.TrimSpace(item.QuestionType)
		if questionType == "" {
			questionType = CaseTypeCore
		}
		expectedResult := strings.TrimSpace(item.ExpectedResult)
		items = append(items, GeneratedCase{
			ScenarioID:         strings.TrimSpace(item.ScenarioID),
			Content:            content,
			ExpectedResult:     expectedResult,
			QuestionType:       normalizeGeneratedQuestionType(questionType),
			Turns:              normalizeGeneratedCaseTurns(item.Turns, content),
			FileFixtures:       normalizeGeneratedFileFixtures(item.FileFixtures),
			ConversationChecks: normalizeConversationExpectedChecks(item.ConversationChecks),
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("generated cases are empty")
	}
	return items, nil
}

func conversationExpectedChecksValue(raw json.RawMessage) ConversationExpectedChecks {
	if len(raw) == 0 || string(raw) == "null" {
		return ConversationExpectedChecks{}
	}
	var checks ConversationExpectedChecks
	if err := json.Unmarshal(raw, &checks); err == nil && checks.Useful() {
		return normalizeConversationExpectedChecks(checks)
	}
	var conditions []ConversationExpectedCheckCondition
	if err := json.Unmarshal(raw, &conditions); err == nil {
		return normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: conditions})
	}
	var condition ConversationExpectedCheckCondition
	if err := json.Unmarshal(raw, &condition); err == nil {
		return normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: []ConversationExpectedCheckCondition{condition}})
	}
	values := fixtureStringListValue(raw)
	if len(values) == 0 {
		return ConversationExpectedChecks{}
	}
	return normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: []ConversationExpectedCheckCondition{{
		Type:      "reply_contains",
		Operator:  "contains",
		Values:    values,
		MatchMode: "semantic",
		Severity:  "normal",
		Source:    "ai_generated",
	}}})
}

func conversationExpectedChecksFromInput(value interface{}) ConversationExpectedChecks {
	data, err := json.Marshal(value)
	if err != nil {
		return ConversationExpectedChecks{}
	}
	return conversationExpectedChecksValue(data)
}

func normalizeConversationExpectedChecks(checks ConversationExpectedChecks) ConversationExpectedChecks {
	conditions := make([]ConversationExpectedCheckCondition, 0, len(checks.Conditions))
	seenIDs := map[string]int{}
	for _, condition := range checks.Conditions {
		condition.ID = strings.TrimSpace(condition.ID)
		condition.Type = normalizeConversationCheckType(condition.Type)
		condition.Operator = normalizeConversationCheckOperator(condition.Type, condition.Operator)
		condition.TargetID = strings.TrimSpace(condition.TargetID)
		condition.TargetLabel = strings.TrimSpace(condition.TargetLabel)
		condition.Values = normalizeFixtureStringList(condition.Values)
		condition.MatchMode = normalizeMatchMode(condition.MatchMode)
		condition.Severity = normalizeSeverity(condition.Severity)
		condition.Source = normalizeCheckSource(condition.Source)
		if condition.Type == "" {
			continue
		}
		if requiresConversationCheckValues(condition.Type, condition.Operator) && len(condition.Values) == 0 {
			continue
		}
		condition.ID = uniqueWorkflowTestCheckID(seenIDs, condition.ID, fmt.Sprintf("check_%s_%d", condition.Type, len(conditions)+1))
		conditions = append(conditions, condition)
	}
	return ConversationExpectedChecks{Conditions: conditions}
}

func (c ConversationExpectedChecks) Useful() bool {
	return len(c.Conditions) > 0
}

func (c ConversationExpectedChecks) JSONMap() JSONMap {
	normalized := normalizeConversationExpectedChecks(c)
	if len(normalized.Conditions) == 0 {
		return JSONMap{}
	}
	return JSONMap{"conditions": normalized.Conditions}
}

func normalizeConversationCheckType(value string) string {
	switch strings.TrimSpace(value) {
	case "intent_understanding", "context_following", "memory", "clarification", "output_format", "fallback", "reply_contains", "safety", "task_completion", "consistency", "no_hallucination", "no_system_leak", "tone":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeConversationCheckOperator(checkType, value string) string {
	switch strings.TrimSpace(value) {
	case "passed", "contains", "not_contains":
		return strings.TrimSpace(value)
	}
	switch checkType {
	case "reply_contains", "output_format":
		return "contains"
	default:
		return "passed"
	}
}

func requiresConversationCheckValues(checkType, operator string) bool {
	switch checkType {
	case "reply_contains", "output_format", "intent_understanding", "context_following", "memory", "clarification", "fallback", "safety", "task_completion", "consistency", "no_hallucination", "no_system_leak", "tone":
		return true
	default:
		return operator == "contains" || operator == "not_contains"
	}
}

func defaultTurnConversationChecks(expectation string) ConversationExpectedChecks {
	expectation = strings.TrimSpace(expectation)
	if expectation == "" {
		return ConversationExpectedChecks{}
	}
	return normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: []ConversationExpectedCheckCondition{{
		Type:      "reply_contains",
		Operator:  "contains",
		Values:    []string{expectation},
		MatchMode: "semantic",
		Severity:  "normal",
		Source:    "system_default",
	}}})
}

func defaultGlobalConversationChecks() ConversationExpectedChecks {
	return normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: []ConversationExpectedCheckCondition{{
		Type:      "context_following",
		Operator:  "passed",
		Values:    []string{"后续回复应承接前文已确认的信息，避免重复询问已提供内容。"},
		MatchMode: "semantic",
		Severity:  "normal",
		Source:    "system_default",
	}}})
}

func stringFromInputValue(value interface{}) string {
	return strings.TrimSpace(fixtureValueString(value))
}

func normalizeGeneratedFileFixtures(fixtures []GeneratedFileFixture) []GeneratedFileFixture {
	normalized := make([]GeneratedFileFixture, 0, len(fixtures))
	for _, fixture := range fixtures {
		format := strings.TrimSpace(fixture.Format)
		content := strings.TrimSpace(fixture.Content)
		if format == "" && content == "" && strings.TrimSpace(fixture.Filename) == "" {
			continue
		}
		normalized = append(normalized, GeneratedFileFixture{
			Filename:       strings.TrimSpace(fixture.Filename),
			Format:         format,
			Title:          strings.TrimSpace(fixture.Title),
			Content:        content,
			Description:    strings.TrimSpace(fixture.Description),
			Facts:          normalizeFixtureStringList(fixture.Facts),
			ExpectedChecks: normalizeGeneratedFixtureExpectedChecks(fixture.ExpectedChecks),
		})
	}
	return normalized
}

func normalizeFixtureStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func fixtureStringValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return strings.TrimSpace(fixtureValueString(value))
}

func fixtureStringListValue(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return normalizeFixtureStringList(fixtureValueStrings(value))
}

func fixtureValueStrings(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return []string{typed}
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, fixtureValueStrings(item)...)
		}
		return result
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make([]string, 0, len(keys))
		for _, key := range keys {
			valueText := fixtureValueString(typed[key])
			if strings.TrimSpace(valueText) == "" {
				continue
			}
			result = append(result, strings.TrimSpace(key)+": "+valueText)
		}
		return result
	default:
		return []string{fixtureValueString(typed)}
	}
}

func fixtureValueString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64, bool:
		return fmt.Sprint(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func normalizeGeneratedCaseTurns(turns []CaseTurn, fallbackContent string) []CaseTurn {
	normalized := make([]CaseTurn, 0, len(turns))
	for _, turn := range turns {
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" && len(turn.Attachments) == 0 {
			continue
		}
		normalized = append(normalized, CaseTurn{
			Role:        role,
			Content:     content,
			Attachments: turn.Attachments,
			Inputs:      turn.Inputs,
		})
	}
	if len(normalized) == 0 && strings.TrimSpace(fallbackContent) != "" {
		normalized = append(normalized, CaseTurn{
			Role:    "user",
			Content: strings.TrimSpace(fallbackContent),
		})
	}
	return normalized
}

func normalizeGeneratedQuestionType(value string) string {
	switch strings.TrimSpace(value) {
	case CaseTypeCore, "核心问题":
		return CaseTypeCore
	case CaseTypeExtension, "扩展问法":
		return CaseTypeExtension
	case CaseTypeFuzzy, "模糊问题":
		return CaseTypeFuzzy
	case CaseTypeManual, "人工介入":
		return CaseTypeManual
	default:
		return CaseTypeCore
	}
}
