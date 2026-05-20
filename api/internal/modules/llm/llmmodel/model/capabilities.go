package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// Model Capabilities - Aligned with ModelHub API structure
// ============================================================================

// ModelEndpoints represents capability endpoints the model supports
type ModelEndpoints struct {
	ChatCompletions  bool `json:"chat_completions" gorm:"default:false"`
	Responses        bool `json:"responses" gorm:"default:false"`
	Realtime         bool `json:"realtime" gorm:"default:false"`
	Assistants       bool `json:"assistants" gorm:"default:false"`
	Batch            bool `json:"batch" gorm:"default:false"`
	Embeddings       bool `json:"embeddings" gorm:"default:false"`
	FineTuning       bool `json:"fine_tuning" gorm:"default:false"`
	ImageGeneration  bool `json:"image_generation" gorm:"default:false"`
	Vision           bool `json:"vision" gorm:"default:false"`
	SpeechGeneration bool `json:"speech_generation" gorm:"default:false"`
	Transcription    bool `json:"transcription" gorm:"default:false"`
	Translation      bool `json:"translation" gorm:"default:false"`
	Moderation       bool `json:"moderation" gorm:"default:false"`
	Videos           bool `json:"videos" gorm:"default:false"`
	ImageEdit        bool `json:"image_edit" gorm:"default:false"`
}

// ModelFeatures represents feature capabilities of the model
type ModelFeatures struct {
	Streaming        bool `json:"streaming" gorm:"default:true"`
	FunctionCalling  bool `json:"function_calling" gorm:"default:false"`
	StructuredOutput bool `json:"structured_output" gorm:"default:false"`
	JsonMode         bool `json:"json_mode" gorm:"default:false"`
	Distillation     bool `json:"distillation" gorm:"default:false"`
	Reasoning        bool `json:"reasoning" gorm:"default:false"`
	SystemPrompt     bool `json:"system_prompt" gorm:"default:true"`
	Logprobs         bool `json:"logprobs" gorm:"default:false"`
	WebSearch        bool `json:"web_search" gorm:"default:false"`
	FileSearch       bool `json:"file_search" gorm:"default:false"`
	CodeInterpreter  bool `json:"code_interpreter" gorm:"default:false"`
	ComputerUse      bool `json:"computer_use" gorm:"default:false"`
	Mcp              bool `json:"mcp" gorm:"default:false"`
	ReasoningEffort  bool `json:"reasoning_effort" gorm:"default:false"`
	Attachment       bool `json:"attachment" gorm:"default:false"`
}

// ModelTools represents tool capabilities the model supports
type ModelTools struct {
	WebSearch         bool `json:"web_search" gorm:"default:false"`
	FileSearch        bool `json:"file_search" gorm:"default:false"`
	ImageGeneration   bool `json:"image_generation" gorm:"default:false"`
	CodeInterpreter   bool `json:"code_interpreter" gorm:"default:false"`
	ComputerUse       bool `json:"computer_use" gorm:"default:false"`
	Mcp               bool `json:"mcp" gorm:"default:false"`
	ParallelToolCalls bool `json:"parallel_tool_calls" gorm:"default:true"`
}

// ModelParameters represents fixed standard parameter presence
type ModelParameters struct {
	SupportsTemperature      bool `json:"temperature" gorm:"default:true"`
	SupportsTopP             bool `json:"top_p" gorm:"default:true"`
	SupportsPresencePenalty  bool `json:"presence_penalty" gorm:"default:false"`
	SupportsFrequencyPenalty bool `json:"frequency_penalty" gorm:"default:false"`
	SupportsLogitBias        bool `json:"logit_bias" gorm:"default:false"`
	SupportsSeed             bool `json:"seed" gorm:"default:false"`
	SupportsStop             bool `json:"stop" gorm:"default:true"`
	MaxStopSequences         int  `json:"max_stop_sequences" gorm:"default:4"`
}

// ParameterDefinition represents a dynamic parameter metadata for UI rendering
type ParameterDefinition struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // slider, select, switch, number, text
	Label       string      `json:"label"`
	Default     interface{} `json:"default"`
	Min         *float64    `json:"min,omitempty"`
	Max         *float64    `json:"max,omitempty"`
	Step        *float64    `json:"step,omitempty"`
	Options     []string    `json:"options,omitempty"`
	Description string      `json:"description,omitempty"`
}

// ParameterDefinitions is a slice of ParameterDefinition with JSONB support
type ParameterDefinitions []ParameterDefinition

func (p *ParameterDefinitions) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}

	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("failed to scan ParameterDefinitions: expected []byte or string, got %T", value)
	}

	normalized, err := NormalizeParameterDefinitionsJSON(raw)
	if err != nil {
		return err
	}
	*p = normalized
	return nil
}

func (p ParameterDefinitions) Value() (driver.Value, error) {
	if p == nil {
		return "[]", nil
	}
	return json.Marshal(p)
}

// NormalizeParameterDefinitionsJSON accepts either the array shape used by API
// storage or the object shape produced by Console catalog publications.
func NormalizeParameterDefinitionsJSON(raw []byte) (ParameterDefinitions, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ParameterDefinitions{}, nil
	}

	switch trimmed[0] {
	case '[':
		var params ParameterDefinitions
		if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
			return nil, err
		}
		return params, nil
	case '{':
		var object map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &object); err != nil {
			return nil, err
		}
		return NormalizeParameterDefinitionsMap(object), nil
	default:
		return nil, fmt.Errorf("failed to scan ParameterDefinitions: unsupported JSON shape")
	}
}

// NormalizeParameterDefinitionsMap converts Console's parameter-governance object
// into the array shape expected by API storage and response assembly.
func NormalizeParameterDefinitionsMap(raw map[string]interface{}) ParameterDefinitions {
	if len(raw) == 0 {
		return ParameterDefinitions{}
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make(ParameterDefinitions, 0, len(keys))
	for _, key := range keys {
		definition, ok := raw[key].(map[string]interface{})
		if !ok {
			continue
		}
		if supported, exists := definition["supported"]; exists {
			supportedBool, ok := supported.(bool)
			if ok && !supportedBool {
				continue
			}
		}

		param := ParameterDefinition{
			Name:        key,
			Label:       stringValue(definition["label"], humanizeParameterName(key)),
			Default:     definition["default"],
			Min:         float64Ptr(definition["min"]),
			Max:         float64Ptr(definition["max"]),
			Step:        float64Ptr(definition["step"]),
			Options:     stringSliceValue(definition["options"]),
			Description: stringValue(definition["description"], ""),
		}
		param.Type = inferParameterType(definition, param)
		params = append(params, param)
	}

	return params
}

func inferParameterType(source map[string]interface{}, param ParameterDefinition) string {
	if explicit, ok := source["type"].(string); ok && strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if len(param.Options) > 0 {
		return "select"
	}
	switch param.Default.(type) {
	case bool:
		return "switch"
	}
	if param.Min != nil || param.Max != nil || param.Step != nil {
		return "number"
	}
	switch param.Default.(type) {
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return "text"
	}
}

func stringValue(value interface{}, fallback string) string {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return text
}

func stringSliceValue(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func float64Ptr(value interface{}) *float64 {
	switch typed := value.(type) {
	case float64:
		return &typed
	case float32:
		converted := float64(typed)
		return &converted
	case int:
		converted := float64(typed)
		return &converted
	case int8:
		converted := float64(typed)
		return &converted
	case int16:
		converted := float64(typed)
		return &converted
	case int32:
		converted := float64(typed)
		return &converted
	case int64:
		converted := float64(typed)
		return &converted
	case uint:
		converted := float64(typed)
		return &converted
	case uint8:
		converted := float64(typed)
		return &converted
	case uint16:
		converted := float64(typed)
		return &converted
	case uint32:
		converted := float64(typed)
		return &converted
	case uint64:
		converted := float64(typed)
		return &converted
	default:
		return nil
	}
}

func humanizeParameterName(name string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ")
	parts := strings.Fields(replacer.Replace(name))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func defaultUseCases() []string {
	return []string{string(UseCaseTextChat)}
}

// NormalizeUseCases canonicalizes, deduplicates, and sorts use case values while
// preserving unknown values for forward compatibility.
func NormalizeUseCases(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		canonical := canonicalUseCase(value)
		if canonical == "" {
			continue
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}

	sort.Strings(normalized)
	return normalized
}

func canonicalUseCase(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "":
		return ""
	case "chat":
		return string(UseCaseTextChat)
	case "image", "image-generation":
		return string(UseCaseImageGen)
	case "video", "video-generation", "videos":
		return string(UseCaseVideoGen)
	case "tts", "speech":
		return string(UseCaseTextToSpeech)
	case "stt", "transcription":
		return string(UseCaseSpeechToText)
	default:
		return normalized
	}
}

func InferUseCasesFromLegacyType(modelType string) []string {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "", "llm", "chat":
		return defaultUseCases()
	case "text-embedding", "embedding", "embeddings":
		return []string{string(UseCaseEmbedding)}
	case "rerank":
		return []string{string(UseCaseRerank)}
	case "image", "image-gen", "image-generation":
		return []string{string(UseCaseImageGen)}
	case "tts", "speech":
		return []string{string(UseCaseTextToSpeech)}
	case "stt", "transcription":
		return []string{string(UseCaseSpeechToText)}
	case "moderation":
		return []string{string(UseCaseModeration)}
	default:
		return nil
	}
}

func InferUseCasesFromEndpoints(endpoints ModelEndpoints) []string {
	useCases := make([]string, 0, 8)
	if endpoints.ChatCompletions || endpoints.Responses || endpoints.Realtime || endpoints.Assistants {
		useCases = append(useCases, string(UseCaseTextChat))
	}
	if endpoints.Embeddings {
		useCases = append(useCases, string(UseCaseEmbedding))
	}
	if endpoints.ImageGeneration {
		useCases = append(useCases, string(UseCaseImageGen))
	}
	if endpoints.ImageEdit {
		useCases = append(useCases, string(UseCaseImageGen))
	}
	if endpoints.Vision {
		useCases = append(useCases, string(UseCaseVision))
	}
	if endpoints.SpeechGeneration {
		useCases = append(useCases, string(UseCaseTextToSpeech))
	}
	if endpoints.Transcription {
		useCases = append(useCases, string(UseCaseSpeechToText))
	}
	if endpoints.Moderation {
		useCases = append(useCases, string(UseCaseModeration))
	}
	if endpoints.Translation {
		useCases = append(useCases, "translation")
	}
	if endpoints.Videos {
		useCases = append(useCases, string(UseCaseVideoGen))
	}
	return NormalizeUseCases(useCases)
}

func EnsureUseCases(values []string, endpoints *ModelEndpoints) []string {
	normalized := NormalizeUseCases(values)
	if len(normalized) > 0 {
		return normalized
	}
	if endpoints != nil {
		derived := InferUseCasesFromEndpoints(*endpoints)
		if len(derived) > 0 {
			return derived
		}
	}
	return defaultUseCases()
}

// DefaultEndpointsForType returns default endpoints based on model type.
// Deprecated: Use DefaultEndpointsForUseCases for new code.
func DefaultEndpointsForType(modelType string) ModelEndpoints {
	return DefaultEndpointsForUseCases(InferUseCasesFromLegacyType(modelType))
}

// DefaultEndpointsForUseCases returns default endpoints derived from a use_cases array.
func DefaultEndpointsForUseCases(useCases []string) ModelEndpoints {
	endpoints := ModelEndpoints{}
	for _, uc := range NormalizeUseCases(useCases) {
		switch uc {
		case string(UseCaseTextChat):
			endpoints.ChatCompletions = true
		case string(UseCaseEmbedding):
			endpoints.Embeddings = true
		case string(UseCaseImageGen):
			endpoints.ImageGeneration = true
		case string(UseCaseVideoGen):
			endpoints.Videos = true
		case string(UseCaseTextToSpeech):
			endpoints.SpeechGeneration = true
		case string(UseCaseSpeechToText):
			endpoints.Transcription = true
		case string(UseCaseModeration):
			endpoints.Moderation = true
		case string(UseCaseVision):
			endpoints.Vision = true
		case "translation":
			endpoints.Translation = true
		}
	}
	return endpoints
}

// DefaultFeaturesForLLM returns default features for LLM type models
func DefaultFeaturesForLLM() ModelFeatures {
	return ModelFeatures{
		Streaming:    true,
		SystemPrompt: true,
	}
}

// DefaultTools returns empty tools
func DefaultTools() ModelTools {
	return ModelTools{}
}

// DefaultParameters returns default parameters
func DefaultParameters() ModelParameters {
	return ModelParameters{
		SupportsStop:     true,
		MaxStopSequences: 4,
	}
}
