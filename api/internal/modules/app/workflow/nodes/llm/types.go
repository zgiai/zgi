package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// Mode represents the mode of LLM operation
type Mode string

const (
	ModeCompletion Mode = "completion"
	ModeChat       Mode = "chat"
)

// ImagePromptMessageContentDetail represents detail level for image prompt
type ImagePromptMessageContentDetail string

const (
	ImageDetailHigh ImagePromptMessageContentDetail = "high"
	ImageDetailLow  ImagePromptMessageContentDetail = "low"
	ImageDetailAuto ImagePromptMessageContentDetail = "auto"
)

const defaultVisionUserPromptText = "Analyze the uploaded image or file directly. Use all visible content, including questions, answers, annotations, scores, diagrams, and layout details, to complete the task."

// VariableSelector represents a variable selector
type VariableSelector struct {
	Variable      string   `json:"variable"`
	ValueSelector []string `json:"value_selector"`
}

// ModelConfig represents the LLM model configuration
type ModelConfig struct {
	Provider         string         `json:"provider"`
	Name             string         `json:"name"`
	Mode             Mode           `json:"mode"`
	CompletionParams map[string]any `json:"completion_params"`
}

// ContextConfig represents context configuration
type ContextConfig struct {
	Enabled           bool               `json:"enabled"`
	VariableSelectors []VariableSelector `json:"variable_selector,omitempty"`
	// Support simple string array format from frontend
	VariableSelectorRaw json.RawMessage `json:"-"`
}

// UnmarshalJSON custom unmarshaler to handle both formats
func (c *ContextConfig) UnmarshalJSON(data []byte) error {
	type Alias ContextConfig
	aux := &struct {
		VariableSelector json.RawMessage `json:"variable_selector,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.VariableSelector) > 0 {
		// Try to unmarshal as string array first (frontend format)
		var strArray []string
		if err := json.Unmarshal(aux.VariableSelector, &strArray); err == nil {
			// Convert string array to VariableSelector format
			if len(strArray) > 0 {
				c.VariableSelectors = []VariableSelector{
					{
						Variable:      "",
						ValueSelector: strArray,
					},
				}
			}
			return nil
		}

		// Try to unmarshal as VariableSelector array (backend format)
		var vsArray []VariableSelector
		if err := json.Unmarshal(aux.VariableSelector, &vsArray); err == nil {
			c.VariableSelectors = vsArray
			return nil
		}

		return fmt.Errorf("variable_selector must be either string array or VariableSelector array")
	}

	return nil
}

// VisionConfigOptions represents vision configuration options
type VisionConfigOptions struct {
	VariableSelector []string                        `json:"variable_selector"`
	Detail           ImagePromptMessageContentDetail `json:"detail"`
}

// VisionConfig represents vision configuration
type VisionConfig struct {
	Enabled bool                `json:"enabled"`
	Configs VisionConfigOptions `json:"configs"`
}

// NewVisionConfig creates a default vision config
func NewVisionConfig() VisionConfig {
	return VisionConfig{
		Enabled: false,
		Configs: VisionConfigOptions{
			VariableSelector: []string{"sys", "files"},
			Detail:           ImageDetailHigh,
		},
	}
}

// PromptConfig represents prompt configuration
type PromptConfig struct {
	TemplateVariables []VariableSelector `json:"template_variables"`
}

// NewPromptConfig creates a default prompt config
func NewPromptConfig() PromptConfig {
	return PromptConfig{
		TemplateVariables: make([]VariableSelector, 0),
	}
}

// PromptMessageRole represents the role in a prompt message
type PromptMessageRole string

const (
	PromptMessageRoleSystem    PromptMessageRole = "system"
	PromptMessageRoleUser      PromptMessageRole = "user"
	PromptMessageRoleAssistant PromptMessageRole = "assistant"
)

// RolePrefix represents role prefix configuration for memory
type RolePrefix struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// WindowConfig represents window configuration for memory
type WindowConfig struct {
	Enabled bool `json:"enabled"`
	Size    int  `json:"size,omitempty"`
}

// MemoryConfig represents memory configuration
type MemoryConfig struct {
	RolePrefix          RolePrefix   `json:"role_prefix"`
	Window              WindowConfig `json:"window"`
	QueryPromptTemplate string       `json:"query_prompt_template,omitempty"`
}

// NodeChatModelMessage represents a chat model message for LLM node
type NodeChatModelMessage struct {
	Role         PromptMessageRole `json:"role"`
	Text         string            `json:"text"`
	TemplateText *string           `json:"template_text,omitempty"`
	EditionType  string            `json:"edition_type,omitempty"`
}

// NodeCompletionModelPromptTemplate represents completion model prompt template
type NodeCompletionModelPromptTemplate struct {
	Text         string  `json:"text"`
	TemplateText *string `json:"template_text,omitempty"`
	EditionType  string  `json:"edition_type,omitempty"`
}

// --------------

// PromptMessageContentType represents the type of prompt message content
type PromptMessageContentType string

const (
	PromptMessageContentTypeText     PromptMessageContentType = "text"
	PromptMessageContentTypeImage    PromptMessageContentType = "image"
	PromptMessageContentTypeVideo    PromptMessageContentType = "video"
	PromptMessageContentTypeAudio    PromptMessageContentType = "audio"
	PromptMessageContentTypeDocument PromptMessageContentType = "document"
)

// PromptMessageContent represents content in a prompt message
type PromptMessageContent struct {
	Type     PromptMessageContentType        `json:"type"`
	Data     string                          `json:"data,omitempty"`
	URL      string                          `json:"url,omitempty"`
	MimeType string                          `json:"mime_type,omitempty"`
	Base64   string                          `json:"base64_data,omitempty"`
	Detail   ImagePromptMessageContentDetail `json:"detail,omitempty"`
}

// PromptMessage represents a message in the conversation
type PromptMessage struct {
	Role    PromptMessageRole `json:"role"`
	Content any               `json:"content"` // Can be string or []PromptMessageContent
}

// ResultChunk represents a chunk of LLM result for streaming
type ResultChunk struct {
	Model            string            `json:"model,omitempty"`
	PromptMessages   []PromptMessage   `json:"prompt_messages,omitempty"`
	Delta            *ResultChunkDelta `json:"delta,omitempty"`
	StructuredOutput any               `json:"structured_output,omitempty"`
}

// ResultChunkDelta represents the delta part of an LLM result chunk
type ResultChunkDelta struct {
	Message      *PromptMessage   `json:"message,omitempty"`
	Usage        *shared.LLMUsage `json:"usage,omitempty"`
	FinishReason string           `json:"finish_reason,omitempty"`
}

// Result represents the result from LLM invocation
type Result struct {
	Message      PromptMessage    `json:"message"`
	Usage        *shared.LLMUsage `json:"usage"`
	FinishReason *string          `json:"finish_reason"`
}

// StructuredOutput represents structured output from LLM
type StructuredOutput struct {
	StructuredOutput map[string]any `json:"structured_output"`
}

// NodeEventType represents different types of events that can be emitted by the LLM node
type NodeEventType string

const (
	NodeEventRunCompleted         NodeEventType = "run_completed"
	NodeEventRunFailed            NodeEventType = "run_failed"
	NodeEventStreamChunk          NodeEventType = "run_stream_chunk"
	NodeEventRetrieverResource    NodeEventType = "run_retriever_resource"
	NodeEventModelInvokeCompleted NodeEventType = "model_invoke_completed"
)

// NodeEvent represents an event emitted by the LLM node
type NodeEvent struct {
	Type      NodeEventType `json:"type"`
	NodeID    string        `json:"node_id"`
	Timestamp time.Time     `json:"timestamp"`
	Data      interface{}   `json:"data,omitempty"`
	Error     error         `json:"error,omitempty"`
}

// RunCompletedEventData represents data for run completed event
type RunCompletedEventData struct {
	RunResult *shared.NodeRunResult `json:"run_result"`
}

// RunStreamChunkEventData represents data for stream chunk event
type RunStreamChunkEventData struct {
	ChunkContent         string   `json:"chunk_content"`
	FromVariableSelector []string `json:"from_variable_selector,omitempty"`
}

// RunRetrieverResourceEventData represents data for retriever resource event
type RunRetrieverResourceEventData struct {
	RetrieverResources []*shared.RetrievalSourceMetadata `json:"retriever_resources"`
	Context            string                            `json:"context"`
}

// ModelInvokeCompletedEventData represents data for model invoke completed event
type ModelInvokeCompletedEventData struct {
	Text             string           `json:"text"`
	Usage            *shared.LLMUsage `json:"usage"`
	FinishReason     *string          `json:"finish_reason"`
	StructuredOutput any              `json:"structured_output,omitempty"`
}

// VariableTemplateParser handles parsing template variables
type VariableTemplateParser struct {
	Template string
}

// NewVariableTemplateParser creates a new VariableTemplateParser
func NewVariableTemplateParser(template string) *VariableTemplateParser {
	return &VariableTemplateParser{Template: template}
}

const REG = `\{\{(#[a-zA-Z0-9_]{1,50}(\.[a-zA-Z_][a-zA-Z0-9_]{0,29}){1,10}#)\}\}`

// Extra extracts all the template variable keys from the template string
func (vtp *VariableTemplateParser) Extra() []string {
	// Regular expression to match the template rules
	regex := regexp.MustCompile(REG)

	// Find all matches in the template
	matches := regex.FindAllStringSubmatch(vtp.Template, -1)

	// Extract first group matches
	firstGroupMatches := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			firstGroupMatches = append(firstGroupMatches, match[1])
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, key := range firstGroupMatches {
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}

	return result
}

// ExtractVariableSelectors extracts variable selectors from template
func (vtp *VariableTemplateParser) ExtractVariableSelectors() []VariableSelector {
	// extract and deduplicate variable keys first
	keys := vtp.Extra()

	selectors := make([]VariableSelector, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.Trim(key, "#")
		parts := strings.Split(trimmed, ".")
		if len(parts) < 2 {
			continue
		}
		selectors = append(selectors, VariableSelector{
			Variable:      key,   // keep original with '#'
			ValueSelector: parts, // split result without '#'
		})
	}
	return selectors
}

// Format formats the template string by replacing template variables with their corresponding values
func (vtp *VariableTemplateParser) Format(inputs map[string]interface{}) string {
	// Regular expression to match the template rules
	regex := regexp.MustCompile(REG)

	// Replace template variables with their values
	result := regex.ReplaceAllStringFunc(vtp.Template, func(match string) string {
		// Extract the key from the match
		submatches := regex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match // return original if no key found
		}

		key := submatches[1]
		value, exists := inputs[key]
		if !exists {
			return match // return original matched string if key not found
		}

		// Handle nil values
		if value == nil {
			return ""
		}

		// Convert value to string based on type
		var strValue string
		switch v := value.(type) {
		case string:
			strValue = v
		case bool:
			strValue = fmt.Sprintf("%t", v)
		case int, int8, int16, int32, int64:
			strValue = fmt.Sprintf("%d", v)
		case uint, uint8, uint16, uint32, uint64:
			strValue = fmt.Sprintf("%d", v)
		case float32, float64:
			strValue = fmt.Sprintf("%g", v)
		case []interface{}, map[string]interface{}:
			// Convert slice and map to JSON string
			if jsonBytes, err := json.Marshal(v); err == nil {
				strValue = string(jsonBytes)
			} else {
				strValue = fmt.Sprintf("%v", v)
			}
		default:
			strValue = fmt.Sprintf("%v", v)
		}

		// Remove template variables from the value recursively
		return RemoveTemplateVariables(strValue)
	})

	// Remove <|...|> markers
	markerRegex := regexp.MustCompile(`<\|.*?\|>`)
	return markerRegex.ReplaceAllString(result, "")
}

// RemoveTemplateVariables removes template variables from the given text
func RemoveTemplateVariables(text string) string {
	// Regular expression to match the template rules
	regex := regexp.MustCompile(REG)

	// Replace {{#key#}} with {#key#}
	return regex.ReplaceAllString(text, "{$1}")
}
