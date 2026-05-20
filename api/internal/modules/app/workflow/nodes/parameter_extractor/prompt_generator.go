package parameterextractor

import (
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/template"
	"github.com/zgiai/zgi/api/internal/prompt"
)

// PromptGenerator generates prompts for parameter extraction
type PromptGenerator struct {
	nodeData     NodeData
	variablePool *entities.VariablePool
}

// NewPromptGenerator creates a new prompt generator
func NewPromptGenerator(
	nodeData NodeData,
	variablePool *entities.VariablePool,
) *PromptGenerator {
	return &PromptGenerator{
		nodeData:     nodeData,
		variablePool: variablePool,
	}
}

// GeneratePromptEngineeringChatPrompt generates prompts for prompt engineering mode (chat)
// Simplified to only support prompt engineering mode with Gateway format
func (pg *PromptGenerator) GeneratePromptEngineeringChatPrompt(
	query string,
	files []*file.File,
	instruction string,
) ([]PromptMessage, error) {
	messages := make([]PromptMessage, 0)

	// Add system message
	systemPrompt, err := renderWorkflowPromptTemplate(prompt.WorkflowParameterExtractorSystem, struct{}{})
	if err != nil {
		return nil, err
	}
	messages = append(messages, PromptMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// Inject few-shot examples
	messages, err = pg.injectPromptEngineeringExamples(messages)
	if err != nil {
		return nil, err
	}

	// Build user message content
	textContent := query

	// Format instruction (use provided instruction or node instruction)
	finalInstruction := instruction
	if finalInstruction == "" && pg.nodeData.Instruction != nil {
		finalInstruction = pg.formatInstruction()
	}

	// Build parameter structure
	structure := pg.generateParameterSchema()

	userPrompt, err := renderWorkflowPromptTemplate(
		prompt.WorkflowParameterExtractorChatUser,
		promptEngineeringTemplateData{
			Structure:   structure,
			Instruction: finalInstruction,
			Text:        textContent,
		},
	)
	if err != nil {
		return nil, err
	}

	// Add user message with vision support
	userMessage := pg.buildUserMessageWithVision(userPrompt, files)
	messages = append(messages, userMessage)

	return messages, nil
}

// generateParameterSchema creates JSON schema for parameters
func (pg *PromptGenerator) generateParameterSchema() string {
	schema := make(map[string]any)
	properties := make(map[string]any)
	required := make([]string, 0)

	for _, param := range pg.nodeData.Parameters {
		paramSchema := make(map[string]any)
		paramSchema["description"] = param.Description

		// Map parameter type to JSON schema type
		switch param.Type {
		case ParameterTypeString:
			paramSchema["type"] = "string"
		case ParameterTypeNumber:
			paramSchema["type"] = "number"
		case ParameterTypeBool:
			paramSchema["type"] = "boolean"
		case ParameterTypeSelect:
			paramSchema["type"] = "string"
			if len(param.Options) > 0 {
				paramSchema["enum"] = param.Options
			}
		case ParameterTypeArrayString:
			paramSchema["type"] = "array"
			paramSchema["items"] = map[string]any{"type": "string"}
		case ParameterTypeArrayNumber:
			paramSchema["type"] = "array"
			paramSchema["items"] = map[string]any{"type": "number"}
		case ParameterTypeArrayObject:
			paramSchema["type"] = "array"
			paramSchema["items"] = map[string]any{"type": "object"}
		}

		properties[param.Name] = paramSchema

		if param.Required {
			required = append(required, param.Name)
		}
	}

	schema["type"] = "object"
	schema["properties"] = properties
	if len(required) > 0 {
		schema["required"] = required
	}

	// Convert to JSON string
	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "{}"
	}

	return string(jsonBytes)
}

// injectPromptEngineeringExamples injects few-shot examples for prompt engineering mode
func (pg *PromptGenerator) injectPromptEngineeringExamples(messages []PromptMessage) ([]PromptMessage, error) {
	for _, example := range promptEngineeringExamples {
		content, err := renderWorkflowPromptTemplate(example.templateID, struct{}{})
		if err != nil {
			return nil, err
		}

		messages = append(messages, PromptMessage{
			Role:    example.role,
			Content: content,
		})
	}

	return messages, nil
}

// formatInstruction formats custom instructions with variable templates
func (pg *PromptGenerator) formatInstruction() string {
	if pg.nodeData.Instruction == nil || *pg.nodeData.Instruction == "" {
		return ""
	}

	instruction := *pg.nodeData.Instruction

	// Render template with variable pool
	if pg.variablePool != nil {
		// Convert variable pool to map for template rendering
		variableMap := pg.variablePoolToMap()
		renderer := template.NewPongo2RendererWithVariablePool(variableMap)

		rendered, err := renderer.Render(instruction, variableMap)
		if err != nil {
			// If rendering fails, return original instruction
			return instruction
		}
		instruction = rendered
	}

	return instruction
}

// variablePoolToMap converts variable pool to a map for template rendering
func (pg *PromptGenerator) variablePoolToMap() map[string]interface{} {
	result := make(map[string]interface{})

	if pg.variablePool == nil {
		return result
	}

	// Get all variables from the pool
	// This is a simplified implementation - in production, you'd iterate through all variables
	// For now, we'll support common variable patterns like {{#sys.query#}}

	// Get system variables
	if sysQuery := pg.variablePool.Get([]string{"sys", "query"}); sysQuery != nil {
		result["sys.query"] = sysQuery.ToObject()
	}
	if sysConvID := pg.variablePool.Get([]string{"sys", "conversation_id"}); sysConvID != nil {
		result["sys.conversation_id"] = sysConvID.ToObject()
	}
	if sysUserID := pg.variablePool.Get([]string{"sys", "user_id"}); sysUserID != nil {
		result["sys.user_id"] = sysUserID.ToObject()
	}

	return result
}

// buildUserMessageWithVision builds a user message with vision support
// If vision is enabled and files are provided, it creates a multi-part content message
// Otherwise, it returns a simple text message
func (pg *PromptGenerator) buildUserMessageWithVision(textContent string, files []*file.File) PromptMessage {
	// If vision is not enabled or no files, return simple text message
	if !pg.nodeData.Vision.Enabled || len(files) == 0 {
		return PromptMessage{
			Role:    "user",
			Content: textContent,
		}
	}

	// Build multi-part content with images
	contentParts := make([]map[string]any, 0)

	// Add image parts first
	for _, f := range files {
		if f == nil {
			continue
		}

		// Generate URL for the file
		url, err := f.GenerateURL()
		if err != nil || url == nil {
			continue
		}

		// Determine detail level
		detail := "auto"
		if pg.nodeData.Vision.Configs.Detail != "" {
			detail = pg.nodeData.Vision.Configs.Detail
		}

		// Add image part
		imagePart := map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url":    *url,
				"detail": detail,
			},
		}
		contentParts = append(contentParts, imagePart)
	}

	// Add text part
	textPart := map[string]any{
		"type": "text",
		"text": textContent,
	}
	contentParts = append(contentParts, textPart)

	return PromptMessage{
		Role:    "user",
		Content: contentParts,
	}
}
