package question_answer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func normalizeNodeData(nodeData *NodeData) {
	if nodeData.AnswerType == "" {
		nodeData.AnswerType = AnswerTypeText
	}
	nodeData.AnswerType = strings.TrimSpace(nodeData.AnswerType)
	if nodeData.AnswerType != AnswerTypeChoice {
		nodeData.AnswerType = AnswerTypeText
	}
	nodeData.ChoiceMode = strings.TrimSpace(nodeData.ChoiceMode)
	if nodeData.ChoiceMode == "" {
		nodeData.ChoiceMode = ChoiceModeStatic
	}
	if len(nodeData.DynamicChoices.Selector) >= 2 {
		nodeData.ChoiceMode = ChoiceModeDynamic
	}
	if nodeData.MaxAnswerCount <= 0 {
		nodeData.MaxAnswerCount = defaultMaxAnswerCount
	}
	if nodeData.Model.Provider == "" && nodeData.ModelConfig.Provider != "" {
		nodeData.Model = nodeData.ModelConfig
	}
	if nodeData.ModelConfig.Provider == "" && nodeData.Model.Provider != "" {
		nodeData.ModelConfig = nodeData.Model
	}
	if strings.TrimSpace(nodeData.ExtractionInstruction) == "" && strings.TrimSpace(nodeData.CompletionInstruction) != "" {
		nodeData.ExtractionInstruction = nodeData.CompletionInstruction
	}
	for i := range nodeData.Choices {
		nodeData.Choices[i] = normalizeChoice(nodeData.Choices[i], i)
	}
	for i := range nodeData.ExtractionFields {
		nodeData.ExtractionFields[i] = normalizeExtractionField(nodeData.ExtractionFields[i])
	}
}

func normalizeChoice(choice Choice, index int) Choice {
	choice.ID = strings.TrimSpace(choice.ID)
	choice.Label = strings.TrimSpace(choice.Label)
	choice.Value = strings.TrimSpace(choice.Value)
	if choice.ID == "" {
		choice.ID = fmt.Sprintf("option_%d", index+1)
	}
	if choice.Value == "" {
		choice.Value = choice.ID
	}
	return choice
}

func normalizeExtractionField(field ExtractionField) ExtractionField {
	field.Name = strings.TrimSpace(field.Name)
	field.Type = strings.ToLower(strings.TrimSpace(field.Type))
	field.Description = strings.TrimSpace(field.Description)
	if field.Type == "" {
		field.Type = ExtractionFieldTypeString
	}
	return field
}

func (n *Node) validateConfig() error {
	if strings.TrimSpace(n.NodeData.Question) == "" {
		return fmt.Errorf("question is required")
	}
	if n.NodeData.AnswerType == AnswerTypeChoice {
		if n.isDynamicChoiceMode() {
			if len(n.NodeData.DynamicChoices.Selector) < 2 {
				return fmt.Errorf("dynamic choices selector is required")
			}
			return nil
		}
		if len(n.NodeData.Choices) == 0 {
			return fmt.Errorf("question answer choices are required")
		}
		seen := make(map[string]struct{}, len(n.NodeData.Choices))
		for _, choice := range n.NodeData.Choices {
			if choice.ID == "" {
				return fmt.Errorf("question answer choice id is required")
			}
			if _, exists := seen[choice.ID]; exists {
				return fmt.Errorf("duplicated question answer choice id: %s", choice.ID)
			}
			seen[choice.ID] = struct{}{}
		}
		return nil
	}

	if !n.NodeData.ExtractFromAnswer {
		return nil
	}
	if len(n.NodeData.ExtractionFields) == 0 {
		return fmt.Errorf("question answer text extraction fields are required")
	}
	seenFields := make(map[string]struct{}, len(n.NodeData.ExtractionFields))
	for _, field := range n.NodeData.ExtractionFields {
		if field.Name == "" {
			return fmt.Errorf("question answer extraction field name is required")
		}
		if _, exists := seenFields[field.Name]; exists {
			return fmt.Errorf("duplicated question answer extraction field name: %s", field.Name)
		}
		seenFields[field.Name] = struct{}{}
		if !isSupportedExtractionFieldType(field.Type) {
			return fmt.Errorf("unsupported question answer extraction field type: %s", field.Type)
		}
	}

	model := n.effectiveModel()
	modelName := strings.TrimSpace(model.Name)
	if modelName == "" {
		modelName = strings.TrimSpace(model.Model)
	}
	if strings.TrimSpace(model.Provider) == "" || modelName == "" {
		return fmt.Errorf("question answer text extraction requires model config")
	}
	return nil
}

func isSupportedExtractionFieldType(fieldType string) bool {
	switch fieldType {
	case ExtractionFieldTypeString, ExtractionFieldTypeNumber, ExtractionFieldTypeBoolean:
		return true
	default:
		return false
	}
}

func (n *Node) effectiveModel() ModelConfig {
	model := n.NodeData.Model
	if strings.TrimSpace(model.Provider) == "" && strings.TrimSpace(n.NodeData.ModelConfig.Provider) != "" {
		model = n.NodeData.ModelConfig
	}
	return model
}

func (n *Node) maxAnswerCount() int {
	if n.NodeData.MaxAnswerCount <= 0 {
		return defaultMaxAnswerCount
	}
	return n.NodeData.MaxAnswerCount
}

func (n *Node) isDynamicChoiceMode() bool {
	return n.NodeData.ChoiceMode == ChoiceModeDynamic || len(n.NodeData.DynamicChoices.Selector) >= 2
}

func (n *Node) renderQuestion(question string) (string, error) {
	question = strings.TrimSpace(question)
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return question, nil
	}
	if err := validateQuestionTemplateVariables(n.GraphRuntimeState.VariablePool, question); err != nil {
		return "", err
	}
	return strings.TrimSpace(n.GraphRuntimeState.VariablePool.ConvertTemplate(question).Text()), nil
}

func validateQuestionTemplateVariables(variablePool *entities.VariablePool, question string) error {
	if variablePool == nil || question == "" {
		return nil
	}
	matches := entities.VariablePattern.FindAllStringSubmatch(question, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		selectorText := strings.TrimSpace(match[1])
		selector := strings.Split(selectorText, ".")
		if variablePool.GetWithPath(selector) == nil {
			return fmt.Errorf("question template variable not found: %s", selectorText)
		}
	}
	return nil
}

func (n *Node) previousAnswers() []AnswerRound {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil
	}
	variable := n.GraphRuntimeState.VariablePool.Get([]string{n.NodeID, "answers"})
	if variable == nil {
		return nil
	}
	return coerceAnswerRounds(variable.ToObject())
}

func (n *Node) previousQuestion() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return ""
	}
	variable := n.GraphRuntimeState.VariablePool.Get([]string{n.NodeID, "question"})
	if variable == nil {
		return ""
	}
	return strings.TrimSpace(variable.Text())
}

func coerceAnswerRounds(value any) []AnswerRound {
	if value == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var rounds []AnswerRound
	if err := json.Unmarshal(payload, &rounds); err != nil {
		return nil
	}
	return rounds
}

func currentAnswer(vp *entities.VariablePool, nodeID string) string {
	if vp == nil {
		return ""
	}
	if value, ok := vp.UserInputs["sys.query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := vp.UserInputs["query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if variable := vp.Get([]string{nodeID, "sys.query"}); variable != nil {
		return strings.TrimSpace(variable.Text())
	}
	if variable := vp.Get([]string{nodeID, "query"}); variable != nil {
		return strings.TrimSpace(variable.Text())
	}
	return ""
}

func optionID(vp *entities.VariablePool) string {
	if vp == nil {
		return ""
	}
	if value, ok := vp.UserInputs["question_answer_option_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := vp.UserInputs["inputs.question_answer_option_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func (n *Node) resolveChoices() ([]Choice, error) {
	if n.isDynamicChoiceMode() {
		variable := n.GraphRuntimeState.VariablePool.GetWithPath(n.NodeData.DynamicChoices.Selector)
		if variable == nil {
			return nil, fmt.Errorf("dynamic choices variable not found at selector: %v", n.NodeData.DynamicChoices.Selector)
		}
		choices, err := parseDynamicChoices(variable.ToObject())
		if err != nil {
			return nil, err
		}
		if len(choices) == 0 {
			return nil, fmt.Errorf("dynamic choices are empty")
		}
		return choices, nil
	}
	return append([]Choice(nil), n.NodeData.Choices...), nil
}

func parseDynamicChoices(value any) ([]Choice, error) {
	if typedChoices, ok := value.([]Choice); ok {
		choices := make([]Choice, 0, len(typedChoices))
		for index, choice := range typedChoices {
			choices = append(choices, normalizeChoice(choice, index))
		}
		return choices, nil
	}

	items, ok := value.([]any)
	if !ok {
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal dynamic choices: %w", err)
		}
		if err := json.Unmarshal(payload, &items); err != nil {
			return nil, fmt.Errorf("dynamic choices must be an array")
		}
	}

	choices := make([]Choice, 0, len(items))
	for index, item := range items {
		switch typed := item.(type) {
		case string:
			choices = append(choices, normalizeChoice(Choice{ID: typed, Label: typed, Value: typed}, index))
		case map[string]any:
			choices = append(choices, normalizeChoice(choiceFromMap(typed), index))
		default:
			payload, err := json.Marshal(typed)
			if err != nil {
				return nil, fmt.Errorf("marshal dynamic choice %d: %w", index+1, err)
			}
			var record map[string]any
			if err := json.Unmarshal(payload, &record); err != nil {
				return nil, fmt.Errorf("dynamic choice %d must be string or object", index+1)
			}
			choices = append(choices, normalizeChoice(choiceFromMap(record), index))
		}
	}
	return choices, nil
}

func choiceFromMap(record map[string]any) Choice {
	return Choice{
		ID:    stringValue(record["id"]),
		Label: stringValue(record["label"]),
		Value: stringValue(record["value"]),
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if typed == nil {
			return ""
		}
		return fmt.Sprintf("%v", typed)
	}
}

func matchChoice(choices []Choice, optionID string, answer string) (Choice, bool) {
	optionID = strings.TrimSpace(optionID)
	answer = strings.TrimSpace(answer)
	for _, choice := range choices {
		if optionID != "" && optionID == choice.ID {
			return choice, true
		}
	}
	if answer == "" {
		return Choice{}, false
	}
	for _, choice := range choices {
		if answer == choice.ID || answer == choice.Value || answer == choice.Label {
			return choice, true
		}
	}
	return Choice{}, false
}

func textExtractionSystemPrompt() string {
	return strings.Join([]string{
		"You extract fields from user answers for a workflow question-answer node.",
		"Use all collected answers, not only the latest answer.",
		"Return fields with only values you can extract from the user answers.",
		"Return missing_fields for configured fields that cannot be extracted.",
		"When required fields are missing, provide one concise follow_up_question.",
		"Return only a JSON object.",
	}, " ")
}

func textExtractionUserPrompt(instruction, question, answer string, rounds []AnswerRound, fields []ExtractionField) string {
	payload := map[string]any{
		"task":                   "Extract configured fields from the user's answers.",
		"extraction_instruction": instruction,
		"question":               question,
		"latest_answer":          answer,
		"answers":                rounds,
		"fields":                 fields,
		"schema": map[string]any{
			"fields":             "object with extracted field values",
			"missing_fields":     "array of missing field names",
			"follow_up_question": "follow-up question when required fields are missing",
			"reason":             "short reason",
		},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func (n *Node) validateExtractedFields(fields map[string]any) (map[string]any, []string, error) {
	normalized := make(map[string]any, len(n.NodeData.ExtractionFields))
	missing := make([]string, 0)
	for _, field := range n.NodeData.ExtractionFields {
		value, exists := fields[field.Name]
		if !exists || isEmptyExtractedValue(value) {
			if field.Required {
				missing = append(missing, field.Name)
			}
			continue
		}

		normalizedValue, err := normalizeExtractedValue(field, value)
		if err != nil {
			return nil, nil, err
		}
		normalized[field.Name] = normalizedValue
	}
	sort.Strings(missing)
	return normalized, missing, nil
}

func isEmptyExtractedValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func normalizeExtractedValue(field ExtractionField, value any) (any, error) {
	switch field.Type {
	case ExtractionFieldTypeString:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("extracted field %s must be string", field.Name)
		}
		return strings.TrimSpace(text), nil
	case ExtractionFieldTypeNumber:
		switch typed := value.(type) {
		case float64, float32, int, int64, int32, json.Number:
			return typed, nil
		default:
			return nil, fmt.Errorf("extracted field %s must be number", field.Name)
		}
	case ExtractionFieldTypeBoolean:
		typed, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("extracted field %s must be boolean", field.Name)
		}
		return typed, nil
	default:
		return nil, fmt.Errorf("unsupported question answer extraction field type: %s", field.Type)
	}
}

func applyCompletionParams(req *llmadapter.ChatRequest, params map[string]any) {
	if req == nil || params == nil {
		return
	}
	if value, ok := floatParam(params["temperature"]); ok {
		req.Temperature = &value
	}
	if value, ok := floatParam(params["top_p"]); ok {
		req.TopP = &value
	}
	if value, ok := intParam(params["max_tokens"]); ok {
		req.MaxTokens = &value
	}
	req.AdditionalParameters = params
}

func floatParam(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func intParam(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func adapterUsage(usage *llmadapter.Usage) *shared.LLMUsage {
	if usage == nil {
		return nil
	}
	return &shared.LLMUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func (n *Node) workflowRunID() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || n.GraphRuntimeState.VariablePool.SystemVariables == nil {
		return ""
	}
	return n.GraphRuntimeState.VariablePool.SystemVariables.WorkflowRunID
}

func (n *Node) conversationID() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || n.GraphRuntimeState.VariablePool.SystemVariables == nil {
		return ""
	}
	return n.GraphRuntimeState.VariablePool.SystemVariables.ConversationID
}

func (n *Node) billingSubjectType() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || n.GraphRuntimeState.VariablePool.SystemVariables == nil {
		return ""
	}
	return n.GraphRuntimeState.VariablePool.SystemVariables.BillingSubjectType
}
