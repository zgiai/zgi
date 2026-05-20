package knowledgeretrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/llm"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// mergeModelParameters merges user-provided model parameters with fixed defaults
// Default values will be used unless explicitly overridden by user parameters
func mergeModelParameters(userParams map[string]any, defaults map[string]any) map[string]any {
	merged := make(map[string]any)

	// First, apply all defaults
	for key, value := range defaults {
		merged[key] = value
	}

	// Then, override with user-provided parameters
	if userParams != nil {
		for key, value := range userParams {
			merged[key] = value
		}
	}

	return merged
}

const (
	Prefix = "Respond to the human as helpfully and accurately as possible. You have access to the following tools:"
	Suffix = "Begin! Reminder to ALWAYS respond with a valid json blob of a single action. Use tools if necessary. Respond directly if appropriate. Format is Action:```$JSON_BLOB```then Observation:.\nThought:"

	FormatInstructions = `Use a json blob to specify a tool by providing an action key (tool name) and an action_input key (tool input).
The nouns in the format of "Thought", "Action", "Action Input", "Final Answer" must be expressed in English.
Valid "action" values: "Final Answer" or %s

Provide only ONE action per $JSON_BLOB, as shown:

` + "```" + `
{
  "action": $TOOL_NAME,
  "action_input": $INPUT
}
` + "```" + `

Follow this format:

Question: input question to answer
Thought: consider previous and subsequent steps
Action:
` + "```" + `
$JSON_BLOB
` + "```" + `
Observation: action result
... (repeat Thought/Action/Observation N times)
Thought: I know what to respond
Action:
` + "```" + `
{
  "action": "Final Answer",
  "action_input": "Final response to human"
}
` + "```"
)

// ReactOutput represents the parsed result from REACT reasoning
type ReactOutput struct {
	Action        string
	ActionInput   map[string]any
	IsFinalAnswer bool
	RawText       string
}

// ReactMultiDatasetRouter selects a dataset via REACT-style prompting.
type ReactMultiDatasetRouter struct {
	llmInvoker llmInvoker
}

// NewReactMultiDatasetRouter builds a router that uses the gateway invoker by default.
func NewReactMultiDatasetRouter() *ReactMultiDatasetRouter {
	invoker, _ := NewGatewayLLMInvoker(nil, "", "", "")
	return &ReactMultiDatasetRouter{
		llmInvoker: invoker,
	}
}

// Invoke prompts the LLM with REACT instructions to pick one dataset; returns empty string if none.
func (r *ReactMultiDatasetRouter) Invoke(
	ctx context.Context,
	query string,
	datasetTools []DatasetTool,
	modelConfig llm.ModelConfig,
	userID string,
	appID string,
) (string, error) {
	// Handle edge cases first
	if len(datasetTools) == 0 {
		return "", nil
	}
	if len(datasetTools) == 1 {
		return datasetTools[0].Name, nil
	}

	// Try REACT invoke
	result, err := r.reactInvoke(
		ctx,
		query,
		modelConfig,
		datasetTools,
		userID,
		appID,
		"",
		"",
		"",
	)
	if err != nil {
		return "", nil
	}
	return result, nil
}

// invokeLLM encapsulates the LLM invocation with standardized error handling and future extensibility
func (r *ReactMultiDatasetRouter) invokeLLM(
	ctx context.Context,
	modelSlug string,
	modelParameters map[string]any,
	promptMessages []PromptMessage,
	stop []string,
	userID string,
	appID string,
) (string, error) {
	if r.llmInvoker == nil {
		return "", ErrInvokerNotConfigured
	}

	invokeReq := &InvokeRequest{
		ModelSlug:  modelSlug,
		Messages:   toInvokerMessages(promptMessages),
		Parameters: modelParameters,
		Stop:       stop,
		UserID:     userID,
	}

	resp, err := r.llmInvoker.Invoke(ctx, userID, appID, AppType, invokeReq)
	if err != nil {
		return "", fmt.Errorf("failed to invoke LLM: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("failed to invoke LLM: empty response")
	}

	return resp.Text, nil
}

func toInvokerMessages(msgs []PromptMessage) []PromptMessage {
	out := make([]PromptMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, PromptMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return out
}

// reactInvoke performs the REACT reasoning for dataset selection
func (r *ReactMultiDatasetRouter) reactInvoke(
	ctx context.Context,
	query string,
	modelConfig llm.ModelConfig,
	tools []DatasetTool,
	userID string,
	appID string,
	prefix string,
	suffix string,
	formatInstructions string,
) (string, error) {

	if prefix == "" {
		prefix = Prefix
	}
	if suffix == "" {
		suffix = Suffix
	}
	if formatInstructions == "" {
		formatInstructions = FormatInstructions
	}

	var promptMessages any
	if modelConfig.Mode == llm.ModeChat {
		// return: []llm.NodeChatModelMessage
		promptMessages = r.createChatPrompt(query, tools, prefix, suffix, formatInstructions)
	} else {
		// return: []llm.NodeCompletionModelPromptTemplate
		promptMessages = r.createCompletionPrompt(tools, prefix, formatInstructions)
	}

	stop := []string{"Observation:"}

	// Handle invoke result
	// Current getPrompt only works for current node
	promptMsgs := getPrompt(promptMessages, modelConfig)

	// Invoke LLM using encapsulated method
	resultText, err := r.invokeLLM(
		ctx,
		modelConfig.Name,
		modelConfig.CompletionParams,
		promptMsgs,
		stop,
		userID,
		appID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to invoke LLM: %w", err)
	}

	// Parse result

	reactOutput, err := r.parseReactOutput(resultText)
	if err != nil {
		return "", fmt.Errorf("failed to parse REACT output: %w", err)
	}

	if reactOutput.IsFinalAnswer {
		// If it's final answer, return the output
		if output, ok := reactOutput.ActionInput["output"].(string); ok {
			return output, nil
		}
		return resultText, nil
	}

	// If it's a tool action, return the tool name
	// This matches  which returns the tool name for dataset selection
	return reactOutput.Action, nil
}

// createChatPrompt creates the prompt messages for REACT reasoning
func (r *ReactMultiDatasetRouter) createChatPrompt(
	query string,
	tools []DatasetTool,
	prefix string,
	suffix string,
	formatInstructions string,
) []llm.NodeChatModelMessage {

	// Build tool strings
	toolStrings := make([]string, 0, len(tools))
	toolNames := make([]string, 0, len(tools))

	for _, tool := range tools {
		toolStrings = append(toolStrings,
			fmt.Sprintf(
				"%s: %s, args: {'query': {'title': 'Query', 'description': 'Query for the dataset to be used to retrieve the dataset.', 'type': 'string'}}",
				tool.Name,
				tool.Description,
			),
		)
		toolNames = append(toolNames, tool.Name)
	}

	formattedTools := strings.Join(toolStrings, "\n")
	toolNamesStr := strings.Join(toolNames, ", ")
	finalFormatInstructions := fmt.Sprintf(formatInstructions, toolNamesStr)

	template := strings.Join([]string{prefix, formattedTools, finalFormatInstructions, suffix}, "\n\n")

	promptMessages := make([]llm.NodeChatModelMessage, 0, 2)

	// Create system and user messages
	systemMessage := llm.NodeChatModelMessage{
		Role: llm.PromptMessageRoleSystem,
		Text: template,
	}
	promptMessages = append(promptMessages, systemMessage)

	userMessage := llm.NodeChatModelMessage{
		Role: llm.PromptMessageRoleUser,
		Text: query,
	}
	promptMessages = append(promptMessages, userMessage)

	return promptMessages
}

// createCompletionPrompt creates the prompt messages for REACT reasoning
func (r *ReactMultiDatasetRouter) createCompletionPrompt(
	tools []DatasetTool,
	prefix string,
	formatInstructions string,
) []llm.NodeCompletionModelPromptTemplate {
	suffix := ""
	toolStrings := make([]string, 0, len(tools))
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolStrings = append(toolStrings, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
		toolNames = append(toolNames, tool.Name)
	}
	toolStr := strings.Join(toolStrings, "\n")
	toolNamesStr := strings.Join(toolNames, ", ")
	finalFormatInstructions := fmt.Sprintf(formatInstructions, toolNamesStr)

	template := strings.Join([]string{prefix, toolStr, finalFormatInstructions, suffix}, "\n\n")

	return []llm.NodeCompletionModelPromptTemplate{
		{
			Text: template,
		},
	}
}

// parseReactOutput parses the REACT output to extract the selected tool using regex matching
func (r *ReactMultiDatasetRouter) parseReactOutput(output string) (*ReactOutput, error) {
	// Use regex to match JSON blob
	pattern := regexp.MustCompile("```(\\w*)\\n?({[\\s\\S]*?})```")
	matches := pattern.FindStringSubmatch(output)

	if matches == nil {
		// No JSON blob found, treat as final answer with the entire output
		return &ReactOutput{
			Action:        "Final Answer",
			ActionInput:   map[string]any{"output": output},
			IsFinalAnswer: true,
			RawText:       output,
		}, nil
	}

	jsonStr := matches[2]

	var response any
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		return nil, fmt.Errorf("could not parse LLM output: %s", output)
	}

	// Handle array responses
	var actionData map[string]any

	switch v := response.(type) {
	case map[string]any:
		actionData = v
	case []any:
		if len(v) > 0 {
			if firstItem, ok := v[0].(map[string]any); ok {
				actionData = firstItem
			}
		}
	default:
		return nil, fmt.Errorf("unexpected response format: %s", output)
	}

	action, actionExists := actionData["action"].(string)
	if !actionExists {
		return nil, fmt.Errorf("missing action in response: %s", output)
	}

	if action == "Final Answer" {
		actionInput := make(map[string]any)

		if input, exists := actionData["action_input"]; exists {
			actionInput["output"] = input
		}
		return &ReactOutput{
			Action:        action,
			ActionInput:   actionInput,
			IsFinalAnswer: true,
			RawText:       output,
		}, nil
	}
	// Handle tool action
	actionInput := make(map[string]any)

	if input, exists := actionData["action_input"]; exists {
		if inputMap, ok := input.(map[string]any); ok {
			actionInput = inputMap
		} else {
			actionInput["input"] = input
		}
	}

	return &ReactOutput{
		Action:        action,
		ActionInput:   actionInput,
		IsFinalAnswer: false,
		RawText:       output,
	}, nil

}

// executeTool executes the selected dataset tool
func (r *ReactMultiDatasetRouter) executeTool(toolName string, actionInput map[string]any, userID string, tenantID string) (string, error) {
	// For dataset tools, the tool execution typically involves retrieving data from the dataset
	// In this simplified implementation, we'll return a mock result or the query parameter
	query, ok := actionInput["query"].(string)
	if !ok {
		// If no specific query is provided, use a default message
		return fmt.Sprintf("Retrieved data from dataset '%s'", toolName), nil
	}

	return fmt.Sprintf("Retrieved data from dataset '%s' for query: %s", toolName, query), nil
}

// FunctionCallMultiDatasetRouter implements function call routing strategy for dataset selection
type FunctionCallMultiDatasetRouter struct {
	llmInvoker llmInvoker
}

// NewFunctionCallMultiDatasetRouter builds a gateway-based function-call router.
func NewFunctionCallMultiDatasetRouter() *FunctionCallMultiDatasetRouter {
	invoker, _ := NewGatewayLLMInvoker(nil, "", "", "")
	return &FunctionCallMultiDatasetRouter{
		llmInvoker: invoker,
	}
}

// Invoke lets the LLM pick a dataset via tool/function call; caller should handle fallback if unsupported.
func (f *FunctionCallMultiDatasetRouter) Invoke(
	ctx context.Context,
	query string,
	datasetTools []DatasetTool,
	modelConfig llm.ModelConfig,
	userID string,
	appID string,
) (string, error) {
	// Handle edge cases first
	if len(datasetTools) == 0 {
		return "", nil
	}
	if len(datasetTools) == 1 {
		return datasetTools[0].Name, nil
	}
	// Create prompt messages
	promptMessages := []PromptMessage{
		{
			Role:    "system",
			Content: "You are a helpful AI assistant.",
		},
		{
			Role:    "user",
			Content: query,
		},
	}

	// Convert DatasetTool to PromptMessageTool
	var tools []PromptMessageTool
	for _, tool := range datasetTools {
		promptTool := PromptMessageTool{
			Type: "function",
			Function: map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		}
		tools = append(tools, promptTool)
	}

	// Merge model parameters with fixed defaults
	mergedParameters := mergeModelParameters(modelConfig.CompletionParams, map[string]any{
		"temperature": 0.2,
		"top_p":       0.3,
		"max_tokens":  1500,
	})

	if f.llmInvoker == nil {
		return "", ErrInvokerNotConfigured
	}

	invokeTools := toAdapterTools(tools)

	resp, err := f.llmInvoker.Invoke(ctx, userID, appID, AppType, &InvokeRequest{
		ModelSlug:  modelConfig.Name,
		Messages:   toInvokerMessages(promptMessages),
		Parameters: mergedParameters,
		Tools:      invokeTools,
		ToolChoice: "auto",
		UserID:     userID,
	})
	if err != nil {
		return "", err
	}

	// Parse result for tool calls using simplified helper function
	if toolName, ok := f.parseToolCallFromResult(resp); ok {
		return toolName, nil
	}

	return "", nil
}

// parseToolCallFromResult extracts the tool name from LLM result using simplified logic
func (f *FunctionCallMultiDatasetRouter) parseToolCallFromResult(result *InvokeResult) (string, bool) {
	if result == nil {
		return "", false
	}

	choices, ok := result.RawChoices.([]llmadapter.Choice)
	if !ok || len(choices) == 0 {
		return "", false
	}

	msg := choices[0].Message
	if len(msg.ToolCalls) == 0 {
		return "", false
	}

	call := msg.ToolCalls[0]
	if call.Function.Name == "" {
		return "", false
	}

	return call.Function.Name, true
}

// toAdapterTools normalizes PromptMessageTool into gateway adapter tools.
func toAdapterTools(tools []PromptMessageTool) []llmadapter.Tool {
	out := make([]llmadapter.Tool, 0, len(tools))
	for _, t := range tools {
		name, desc, params := extractFunctionFields(t.Function)
		out = append(out, llmadapter.Tool{
			Type: t.Type,
			Function: llmadapter.Function{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
	return out
}

// extractFunctionFields normalizes different function representations into name/description/parameters.
func extractFunctionFields(fn any) (string, string, any) {
	switch v := fn.(type) {
	case llmadapter.Function:
		return v.Name, v.Description, v.Parameters
	case map[string]any:
		return fmt.Sprint(v["name"]), fmt.Sprint(v["description"]), v["parameters"]
	default:
		// best-effort string formatting
		return fmt.Sprint(fn), "", nil
	}
}

// getPrompt converts prompt messages to standard PromptMessage format
func getPrompt(promptMessages any, modelConfig llm.ModelConfig) []PromptMessage {
	var result []PromptMessage

	// The current implementation only targets the knowledge retrieval node.
	switch modelConfig.Mode {
	case llm.ModeChat:
		if chatMessages, ok := promptMessages.([]llm.NodeChatModelMessage); ok {
			for _, msg := range chatMessages {
				var processedText string
				if msg.EditionType == "basic" || strings.TrimSpace(msg.EditionType) == "" {
					processedText = msg.Text
				} else if msg.EditionType == "template" {
					processedText = msg.Text
				}

				promptMsg := PromptMessage{
					Role:    string(msg.Role),
					Content: processedText,
				}
				result = append(result, promptMsg)
			}
		}
	case llm.ModeCompletion:
		if completionTemplates, ok := promptMessages.([]llm.NodeCompletionModelPromptTemplate); ok {
			for _, template := range completionTemplates {
				processedText := template.Text

				promptMsg := PromptMessage{
					Role:    "user", // completion mode typically uses user role
					Content: processedText,
				}
				result = append(result, promptMsg)
			}
		}
	}

	return result
}
