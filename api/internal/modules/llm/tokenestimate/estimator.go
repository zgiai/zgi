package tokenestimate

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tiktoken-go/tokenizer"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	messageOverheadTokens = 4
	replyPrimerTokens     = 2
	imagePartTokens       = 1024
)

type Estimator struct {
	cache sync.Map
}

type Result struct {
	Tokens    int
	Tokenizer string
}

// ComponentResult describes one prompt-bearing part of a chat request without
// retaining the serialized value itself.
type ComponentResult struct {
	Tokens     int
	Characters int
}

// ChatRequestResult estimates the complete prompt-bearing surface of a
// ChatRequest. Characters is the size of the full serialized request, while
// Components contains only counts for values that providers may incorporate
// into the model prompt.
type ChatRequestResult struct {
	Tokens     int
	Characters int
	Tokenizer  string
	Components map[string]ComponentResult
}

func NewEstimator() *Estimator {
	return &Estimator{}
}

func (e *Estimator) EstimateMessages(messages []adapter.Message, model string) Result {
	tokenizerName := e.tokenizerName(model)
	total := replyPrimerTokens
	for _, message := range messages {
		total += messageOverheadTokens
		total += e.estimateText(message.Role, model, tokenizerName)
		if message.Name != "" {
			total += e.estimateText(message.Name, model, tokenizerName)
		}
		total += e.estimateContent(message.Content, model, tokenizerName)
		if message.FunctionCall != nil {
			total += e.estimateText(fmt.Sprintf("%v", message.FunctionCall), model, tokenizerName)
		}
		if len(message.ToolCalls) > 0 {
			total += e.estimateText(fmt.Sprintf("%v", message.ToolCalls), model, tokenizerName)
		}
		if message.ToolCallID != "" {
			total += e.estimateText(message.ToolCallID, model, tokenizerName)
		}
	}
	return Result{Tokens: total, Tokenizer: tokenizerName}
}

// EstimateChatRequest estimates the final provider request, including tool and
// response schemas that are not represented by EstimateMessages.
func (e *Estimator) EstimateChatRequest(request *adapter.ChatRequest) ChatRequestResult {
	if request == nil {
		return ChatRequestResult{}
	}

	model := strings.TrimSpace(request.Model)
	tokenizerName := e.tokenizerName(model)
	result := ChatRequestResult{
		Characters: serializedChatRequestCharacters(request),
		Tokenizer:  tokenizerName,
		Components: map[string]ComponentResult{},
	}

	messages := e.EstimateMessages(request.Messages, model)
	result.addComponent("messages", messages.Tokens, serializedCharacters(request.Messages))
	if len(request.Functions) > 0 {
		result.addJSONComponent(e, "functions", request.Functions, model, tokenizerName)
	}
	if request.FunctionCall != nil {
		result.addJSONComponent(e, "function_call", request.FunctionCall, model, tokenizerName)
	}
	if len(request.Tools) > 0 {
		result.addJSONComponent(e, "tools", request.Tools, model, tokenizerName)
	}
	if request.ToolChoice != nil {
		result.addJSONComponent(e, "tool_choice", request.ToolChoice, model, tokenizerName)
	}
	if request.ResponseFormat != nil {
		result.addJSONComponent(e, "response_format", request.ResponseFormat, model, tokenizerName)
	}
	if len(request.AdditionalParameters) > 0 {
		result.addJSONComponent(e, "additional_parameters", request.AdditionalParameters, model, tokenizerName)
	}
	return result
}

func (r *ChatRequestResult) addComponent(name string, tokens int, characters int) {
	if r == nil || strings.TrimSpace(name) == "" || (tokens <= 0 && characters <= 0) {
		return
	}
	component := ComponentResult{Tokens: tokens, Characters: characters}
	r.Components[name] = component
	r.Tokens += component.Tokens
}

func (r *ChatRequestResult) addJSONComponent(e *Estimator, name string, value interface{}, model string, tokenizerName string) {
	serialized := serializeForEstimate(value)
	r.addComponent(name, e.estimateText(serialized, model, tokenizerName), utf8.RuneCountInString(serialized))
}

func serializedChatRequestCharacters(request *adapter.ChatRequest) int {
	if request == nil {
		return 0
	}
	characters := serializedCharacters(request)
	if len(request.AdditionalParameters) > 0 {
		characters += serializedCharacters(request.AdditionalParameters)
	}
	return characters
}

func serializedCharacters(value interface{}) int {
	return utf8.RuneCountInString(serializeForEstimate(value))
}

func serializeForEstimate(value interface{}) string {
	encoded, err := json.Marshal(value)
	if err == nil {
		return string(encoded)
	}
	return fmt.Sprintf("%v", value)
}

func (e *Estimator) estimateContent(content interface{}, model string, tokenizerName string) int {
	switch typed := content.(type) {
	case nil:
		return 0
	case string:
		return e.estimateText(typed, model, tokenizerName)
	case []adapter.MessageContentPart:
		total := 0
		for _, part := range typed {
			total += e.estimateMessagePart(part, model, tokenizerName)
		}
		return total
	case []interface{}:
		total := 0
		for _, item := range typed {
			total += e.estimateGenericPart(item, model, tokenizerName)
		}
		return total
	case []map[string]interface{}:
		total := 0
		for _, item := range typed {
			total += e.estimateMapPart(item, model, tokenizerName)
		}
		return total
	default:
		return e.estimateText(fmt.Sprintf("%v", content), model, tokenizerName)
	}
}

func (e *Estimator) estimateMessagePart(part adapter.MessageContentPart, model string, tokenizerName string) int {
	switch part.Type {
	case "text", "input_text":
		return e.estimateText(part.Text, model, tokenizerName)
	case "image_url", "input_image":
		return imagePartTokens
	default:
		if part.Text != "" {
			return e.estimateText(part.Text, model, tokenizerName)
		}
		if part.ImageURL != nil {
			return imagePartTokens
		}
		return e.estimateText(fmt.Sprintf("%v", part), model, tokenizerName)
	}
}

func (e *Estimator) estimateGenericPart(item interface{}, model string, tokenizerName string) int {
	switch typed := item.(type) {
	case adapter.MessageContentPart:
		return e.estimateMessagePart(typed, model, tokenizerName)
	case map[string]interface{}:
		return e.estimateMapPart(typed, model, tokenizerName)
	default:
		return e.estimateText(fmt.Sprintf("%v", item), model, tokenizerName)
	}
}

func (e *Estimator) estimateMapPart(part map[string]interface{}, model string, tokenizerName string) int {
	partType, _ := part["type"].(string)
	switch partType {
	case "text", "input_text":
		text, _ := part["text"].(string)
		return e.estimateText(text, model, tokenizerName)
	case "image_url", "input_image":
		return imagePartTokens
	default:
		if text, ok := part["text"].(string); ok {
			return e.estimateText(text, model, tokenizerName)
		}
		if _, ok := part["image_url"]; ok {
			return imagePartTokens
		}
		return e.estimateText(fmt.Sprintf("%v", part), model, tokenizerName)
	}
}

func (e *Estimator) estimateText(text string, model string, tokenizerName string) int {
	if text == "" {
		return 0
	}
	if codec, ok := e.codecFor(model, tokenizerName); ok {
		count, err := codec.Count(text)
		if err == nil {
			if count == 0 {
				return 1
			}
			return count
		}
	}
	return estimateTextFallback(text)
}

func (e *Estimator) codecFor(model string, tokenizerName string) (tokenizer.Codec, bool) {
	key := strings.TrimSpace(model) + ":" + tokenizerName
	if cached, ok := e.cache.Load(key); ok {
		codec, ok := cached.(tokenizer.Codec)
		return codec, ok
	}

	var codec tokenizer.Codec
	var err error
	if strings.HasPrefix(tokenizerName, "tiktoken:") {
		codec, err = tokenizer.ForModel(tokenizer.Model(strings.TrimSpace(model)))
		if err != nil {
			encoding := strings.TrimPrefix(tokenizerName, "tiktoken:")
			codec, err = tokenizer.Get(tokenizer.Encoding(encoding))
		}
	}
	if err != nil || codec == nil {
		return nil, false
	}
	e.cache.Store(key, codec)
	return codec, true
}

func (e *Estimator) tokenizerName(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return "fallback:conservative"
	}
	if isO200KModel(name) {
		return "tiktoken:o200k_base"
	}
	if isCl100KModel(name) {
		return "tiktoken:cl100k_base"
	}
	return "fallback:conservative"
}

func isO200KModel(name string) bool {
	return strings.HasPrefix(name, "gpt-5") ||
		strings.HasPrefix(name, "gpt-4.1") ||
		strings.HasPrefix(name, "gpt-4o") ||
		strings.HasPrefix(name, "chatgpt-4o") ||
		strings.HasPrefix(name, "o1") ||
		strings.HasPrefix(name, "o3") ||
		strings.HasPrefix(name, "o4")
}

func isCl100KModel(name string) bool {
	return strings.HasPrefix(name, "gpt-4") ||
		strings.HasPrefix(name, "gpt-3.5") ||
		strings.HasPrefix(name, "gpt-35") ||
		strings.HasPrefix(name, "text-embedding-ada-002")
}

func estimateTextFallback(text string) int {
	runes := utf8.RuneCountInString(text)
	if runes == 0 {
		return 0
	}
	bytes := len(text)
	if bytes > runes {
		tokens := (bytes + 1) / 2
		if tokens < 1 {
			return 1
		}
		return tokens
	}
	tokens := (bytes + 2) / 3
	if tokens < 1 {
		return 1
	}
	return tokens
}
