package suggestedquestions

import "strings"

const (
	maxStartVariables = 12
	maxPromptSnippets = 8
	maxNotes          = 6
	maxCapabilities   = 12
)

// BuildContext extracts a compact, non-secret workflow summary suitable for
// generating user-facing suggested questions.
func BuildContext(input BuildContextInput) WorkflowContext {
	features := input.Features
	if features == nil {
		features = map[string]interface{}{}
	}

	workflowContext := WorkflowContext{
		Locale:            normalizeLocale(input.Locale),
		AgentName:         cleanShortText(input.AgentName),
		AgentDescription:  cleanText(input.AgentDescription, 300),
		WorkflowType:      cleanShortText(input.WorkflowType),
		OpeningStatement:  extractOpeningStatement(features),
		ExistingQuestions: existingQuestions(features, input.ExistingQuestions),
	}

	nodes := sliceValue(input.Graph["nodes"])
	capabilitySeen := make(map[string]struct{})
	for _, item := range nodes {
		node := mapValue(item)
		if node == nil {
			continue
		}
		data := mapValue(node["data"])
		if data == nil {
			data = map[string]interface{}{}
		}
		nodeType := firstString(data["type"], node["type"])
		if nodeType == "" {
			continue
		}
		title := firstString(data["title"], data["label"], node["id"], nodeType)

		addCapability(&workflowContext, capabilitySeen, nodeType, title)

		switch nodeType {
		case "start":
			workflowContext.StartVariables = append(
				workflowContext.StartVariables,
				extractVariables(data)...,
			)
			if len(workflowContext.StartVariables) > maxStartVariables {
				workflowContext.StartVariables = workflowContext.StartVariables[:maxStartVariables]
			}
		case "llm":
			workflowContext.LLMPrompts = append(
				workflowContext.LLMPrompts,
				extractPromptTemplates(title, data)...,
			)
			if len(workflowContext.LLMPrompts) > maxPromptSnippets {
				workflowContext.LLMPrompts = workflowContext.LLMPrompts[:maxPromptSnippets]
			}
		case "note":
			if note := extractNote(title, data); note.Text != "" || note.Title != "" {
				workflowContext.Notes = append(workflowContext.Notes, note)
				if len(workflowContext.Notes) > maxNotes {
					workflowContext.Notes = workflowContext.Notes[:maxNotes]
				}
			}
		}
	}

	return workflowContext
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return "zh-Hans"
	}
	if strings.HasPrefix(strings.ToLower(locale), "zh") {
		return "zh-Hans"
	}
	return "en-US"
}

func existingQuestions(features map[string]interface{}, explicit []string) []string {
	if len(explicit) > 0 {
		return uniqueTrimmed(explicit, 12)
	}

	items := sliceValue(features["suggested_questions"])
	values := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			values = append(values, text)
		}
	}
	return uniqueTrimmed(values, 12)
}

func extractOpeningStatement(features map[string]interface{}) string {
	statementType := stringValue(features["opening_statement_type"])
	if statementType == "message" {
		return cleanText(stringValue(features["opening_statement"]), 500)
	}
	if statementType == "slogan" {
		return cleanText(stringValue(features["opening_slogan"]), 220)
	}

	if slogan := cleanText(stringValue(features["opening_slogan"]), 220); slogan != "" {
		return slogan
	}
	return cleanText(stringValue(features["opening_statement"]), 500)
}

func extractVariables(data map[string]interface{}) []VariableSummary {
	items := sliceValue(data["variables"])
	result := make([]VariableSummary, 0, len(items))
	for _, item := range items {
		variable := mapValue(item)
		if variable == nil {
			continue
		}
		name := firstString(variable["variable"], variable["name"], variable["label"])
		if name == "" || isSensitiveKey(name) {
			continue
		}
		result = append(result, VariableSummary{
			Name:        name,
			Type:        firstString(variable["type"], variable["value_type"], variable["valueType"]),
			Description: cleanText(firstString(variable["description"], variable["desc"]), 220),
			Required:    boolValue(variable["required"]),
		})
		if len(result) >= maxStartVariables {
			break
		}
	}
	return result
}

func extractPromptTemplates(nodeTitle string, data map[string]interface{}) []PromptSummary {
	rawPromptTemplate := data["prompt_template"]
	items := sliceValue(rawPromptTemplate)
	if len(items) == 0 {
		if prompt := mapValue(rawPromptTemplate); prompt != nil {
			items = []interface{}{prompt}
		}
	}

	result := make([]PromptSummary, 0, len(items))
	model := extractModelName(data)
	for _, item := range items {
		prompt := mapValue(item)
		if prompt == nil {
			continue
		}
		text := cleanText(stringValue(prompt["text"]), defaultTextLimit)
		if text == "" {
			text = cleanText(stringValue(prompt["template_text"]), defaultTextLimit)
		}
		if text == "" {
			continue
		}
		result = append(result, PromptSummary{
			NodeTitle: nodeTitle,
			Role:      firstString(prompt["role"], "user"),
			Text:      text,
			Model:     model,
		})
		if len(result) >= maxPromptSnippets {
			break
		}
	}
	return result
}

func extractModelName(data map[string]interface{}) string {
	model := mapValue(data["model"])
	if model == nil {
		return ""
	}
	provider := firstString(model["provider"])
	name := firstString(model["name"], model["model"])
	if provider != "" && name != "" {
		return provider + "/" + name
	}
	return name
}

func extractNote(title string, data map[string]interface{}) NoteSummary {
	text := firstString(data["text"], data["content"], data["desc"], data["description"])
	return NoteSummary{
		Title: title,
		Text:  cleanText(text, 500),
	}
}

func addCapability(ctx *WorkflowContext, seen map[string]struct{}, nodeType, title string) {
	if len(ctx.Capabilities) >= maxCapabilities {
		return
	}
	key := strings.ToLower(nodeType + ":" + title)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	ctx.Capabilities = append(ctx.Capabilities, CapabilitySummary{
		Type:       nodeType,
		Title:      title,
		Dependency: dependencyLabel(nodeType),
	})
}

func dependencyLabel(nodeType string) string {
	nodeType = strings.ToLower(strings.TrimSpace(nodeType))
	normalizedNodeType := strings.ReplaceAll(nodeType, "_", "-")
	switch nodeType {
	case "knowledge_retrieval":
		return "knowledge_base"
	case "calldatabase", "sqlgenerator":
		return "database"
	case "notification_sms":
		return "sms_channel"
	case "approval":
		return "human_approval"
	case "code":
		return "code_sandbox"
	case "imagegen":
		return "image_model"
	case "document_extractor":
		return "file_input"
	case "httprequest":
		return "http_api"
	}

	switch normalizedNodeType {
	case "knowledge-retrieval":
		return "knowledge_base"
	case "call-database", "sql-generator", "calldatabase", "sqlgenerator":
		return "database"
	case "notification-sms":
		return "sms_channel"
	case "approval":
		return "human_approval"
	case "code":
		return "code_sandbox"
	case "image-gen", "imagegen":
		return "image_model"
	case "document-extractor":
		return "file_input"
	case "http-request", "httprequest":
		return "http_api"
	default:
		return ""
	}
}
