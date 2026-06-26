package gateway

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode"

	"github.com/tiktoken-go/tokenizer"
	"github.com/tiktoken-go/tokenizer/codec"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// TokenEstimator estimates token usage for requests
type TokenEstimator struct {
	cache sync.Map // Cache for tokenizers
}

// NewTokenEstimator creates a new token estimator
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{}
}

// EstimatePromptTokens estimates the number of tokens in the prompt
func (te *TokenEstimator) EstimatePromptTokens(messages []adapter.Message, model string) int {
	totalTokens := 0

	for _, msg := range messages {
		totalTokens += 4
		totalTokens += te.estimateTextTokensForModel(model, messageContentForTokenCount(msg.Content))
		totalTokens += 1 // role overhead
	}
	totalTokens += 2 // assistant priming

	return totalTokens
}

func (te *TokenEstimator) EstimateChatPromptTokens(req *adapter.ChatRequest) int {
	if req == nil {
		return 0
	}
	totalTokens := 0
	for _, msg := range req.Messages {
		totalTokens += 3
		totalTokens += te.estimateTextTokensForModel(req.Model, messageContentForTokenCount(msg.Content))
		if strings.TrimSpace(msg.Name) != "" {
			totalTokens += 3
		}
	}
	if len(req.Tools) > 0 {
		for _, tool := range req.Tools {
			totalTokens += 8
			totalTokens += te.estimateJSONTokens(req.Model, tool)
		}
	}
	if len(req.Functions) > 0 {
		for _, fn := range req.Functions {
			totalTokens += 8
			totalTokens += te.estimateJSONTokens(req.Model, fn)
		}
	}
	if req.ResponseFormat != nil {
		totalTokens += 4
		totalTokens += te.estimateJSONTokens(req.Model, req.ResponseFormat)
	}
	totalTokens += 3
	return totalTokens
}

// EstimateCompletionTokens estimates completion tokens based on max_tokens or default
func (te *TokenEstimator) EstimateCompletionTokens(maxTokens *int, model string) int {
	if maxTokens != nil && *maxTokens > 0 {
		return *maxTokens
	}

	// Default completion tokens based on model
	if strings.Contains(model, "gpt-4") {
		return 1000 // Default for GPT-4
	} else if strings.Contains(model, "gpt-3.5") {
		return 500 // Default for GPT-3.5
	}

	// Generic default
	return 500
}

func (te *TokenEstimator) estimateTextTokens(text string) int {
	return te.estimateTextTokensForModel("", text)
}

func (te *TokenEstimator) estimateTextTokensForModel(model, text string) int {
	if text == "" {
		return 0
	}
	if isOpenAITextModel(model) {
		encoded, err := te.tokenEncoder(model).Count(text)
		if err == nil {
			return encoded
		}
	}
	return estimateTokenByModel(model, text)
}

func (te *TokenEstimator) tokenEncoder(model string) tokenizer.Codec {
	key := strings.ToLower(strings.TrimSpace(model))
	if key == "" {
		key = "cl100k_base"
	}
	if cached, ok := te.cache.Load(key); ok {
		return cached.(tokenizer.Codec)
	}
	encoder, err := tokenizer.ForModel(tokenizer.Model(key))
	if err != nil {
		encoder = codec.NewCl100kBase()
	}
	actual, _ := te.cache.LoadOrStore(key, encoder)
	return actual.(tokenizer.Codec)
}

// EstimateTextTokens estimates token count for plain text.
func (te *TokenEstimator) EstimateTextTokens(text string) int {
	return te.estimateTextTokens(text)
}

func (te *TokenEstimator) EstimateTextTokensForModel(model, text string) int {
	return te.estimateTextTokensForModel(model, text)
}

func (te *TokenEstimator) estimateJSONTokens(model string, value interface{}) int {
	body, err := json.Marshal(value)
	if err != nil {
		return te.estimateTextTokensForModel(model, fmt.Sprint(value))
	}
	return te.estimateTextTokensForModel(model, string(body))
}

// EstimateTotalTokens estimates total tokens for a request
func (te *TokenEstimator) EstimateTotalTokens(
	messages []adapter.Message,
	maxTokens *int,
	model string,
) (promptTokens, completionTokens, totalTokens int) {

	// TODO: The token calculation method should be based on tiktoken standard
	promptTokens = te.EstimatePromptTokens(messages, model)
	completionTokens = te.EstimateCompletionTokens(maxTokens, model)
	totalTokens = promptTokens + completionTokens

	return
}

func (te *TokenEstimator) EstimateCreateResponsePromptTokens(req *adapter.CreateResponseRequest) int {
	if req == nil {
		return 0
	}
	totalTokens := 0
	if req.Input != nil {
		totalTokens += te.estimateJSONTokens(req.Model, req.Input)
	}
	if len(req.Messages) > 0 {
		totalTokens += te.EstimatePromptTokens(req.Messages, req.Model)
	}
	totalTokens += te.estimateTextTokensForModel(req.Model, req.Instructions)
	for _, tool := range req.Tools {
		totalTokens += 8
		totalTokens += te.estimateJSONTokens(req.Model, tool)
	}
	if req.ToolChoice != nil {
		totalTokens += 4
		totalTokens += te.estimateJSONTokens(req.Model, req.ToolChoice)
	}
	if req.ResponseFormat != nil {
		totalTokens += 4
		totalTokens += te.estimateJSONTokens(req.Model, req.ResponseFormat)
	}
	if totalTokens > 0 {
		totalTokens += 3
	}
	return totalTokens
}

func (te *TokenEstimator) EstimateCreateResponseCompletionTokens(req *adapter.CreateResponseRequest) int {
	if req == nil {
		return 0
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		return *req.MaxOutputTokens
	}
	return te.EstimateCompletionTokens(req.MaxTokens, req.Model)
}

// EstimateEmbeddingTokens estimates tokens for embedding request
func (te *TokenEstimator) EstimateEmbeddingTokens(input interface{}, model string) int {
	switch v := input.(type) {
	case string:
		return te.estimateTextTokensForModel(model, v)
	case []interface{}:
		return te.estimateEmbeddingInterfaceSliceTokens(v, model)
	case []string:
		totalTokens := 0
		for _, s := range v {
			totalTokens += te.estimateTextTokensForModel(model, s)
		}
		return totalTokens
	case []int:
		return len(v)
	case [][]int:
		totalTokens := 0
		for _, tokens := range v {
			totalTokens += len(tokens)
		}
		return totalTokens
	}

	return 0
}

func (te *TokenEstimator) estimateEmbeddingInterfaceSliceTokens(input []interface{}, model string) int {
	totalTokens := 0
	for _, item := range input {
		switch v := item.(type) {
		case string:
			totalTokens += te.estimateTextTokensForModel(model, v)
		case float64:
			totalTokens++
		case int:
			totalTokens++
		case []interface{}:
			totalTokens += countInterfaceTokenIDs(v)
		case []int:
			totalTokens += len(v)
		}
	}
	return totalTokens
}

func countInterfaceTokenIDs(input []interface{}) int {
	totalTokens := 0
	for _, item := range input {
		switch item.(type) {
		case float64, int:
			totalTokens++
		}
	}
	return totalTokens
}

// EstimateRerankTokens estimates tokens for rerank request
func (te *TokenEstimator) EstimateRerankTokens(query string, documents interface{}, model string) int {
	// Start with query tokens
	totalTokens := te.estimateTextTokensForModel(model, query)

	// Add document tokens
	switch v := documents.(type) {
	case []string:
		for _, doc := range v {
			totalTokens += te.estimateTextTokensForModel(model, doc)
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				totalTokens += te.estimateTextTokensForModel(model, s)
			} else if m, ok := item.(map[string]interface{}); ok {
				// If it's a structured document, extract text field
				if text, ok := m["text"].(string); ok {
					totalTokens += te.estimateTextTokensForModel(model, text)
				}
			}
		}
	}

	return totalTokens
}

type tokenFamily string

const (
	tokenFamilyOpenAI tokenFamily = "openai"
	tokenFamilyGemini tokenFamily = "gemini"
	tokenFamilyClaude tokenFamily = "claude"
)

type tokenMultipliers struct {
	word, number, cjk, symbol, mathSymbol, urlDelim, atSign, emoji, newline, space float64
}

var tokenMultiplierByFamily = map[tokenFamily]tokenMultipliers{
	tokenFamilyOpenAI: {word: 1.02, number: 1.55, cjk: 0.85, symbol: 0.4, mathSymbol: 2.68, urlDelim: 1.0, atSign: 2.0, emoji: 2.12, newline: 0.5, space: 0.42},
	tokenFamilyGemini: {word: 1.15, number: 2.8, cjk: 0.68, symbol: 0.38, mathSymbol: 1.05, urlDelim: 1.2, atSign: 2.5, emoji: 1.08, newline: 1.15, space: 0.2},
	tokenFamilyClaude: {word: 1.13, number: 1.63, cjk: 1.21, symbol: 0.4, mathSymbol: 4.52, urlDelim: 1.26, atSign: 2.82, emoji: 2.6, newline: 0.89, space: 0.39},
}

func estimateTokenByModel(model, text string) int {
	if text == "" {
		return 0
	}
	m := tokenMultiplierByFamily[tokenFamilyForModel(model)]
	var count float64
	var previous runeClass
	runLength := 0
	flushRun := func() {
		if runLength <= 0 {
			return
		}
		if previous == runeClassNumber {
			count += math.Ceil(float64(runLength)/3) * m.number
		} else {
			count += math.Ceil(float64(runLength)/4) * m.word
		}
		previous = runeClassNone
		runLength = 0
	}
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			flushRun()
			if r == '\n' || r == '\t' {
				count += m.newline
			} else {
				count += m.space
			}
		case isCJK(r):
			flushRun()
			count += m.cjk
		case isEmoji(r):
			flushRun()
			count += m.emoji
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			next := runeClassLatin
			if unicode.IsNumber(r) {
				next = runeClassNumber
			}
			if previous != next {
				flushRun()
				previous = next
			}
			runLength++
		case isMathSymbol(r):
			flushRun()
			count += m.mathSymbol
		case r == '@':
			flushRun()
			count += m.atSign
		case strings.ContainsRune("/:?&=;#%", r):
			flushRun()
			count += m.urlDelim
		default:
			flushRun()
			count += m.symbol
		}
	}
	flushRun()
	return int(math.Ceil(count))
}

type runeClass int

const (
	runeClassNone runeClass = iota
	runeClassLatin
	runeClassNumber
)

func tokenFamilyForModel(model string) tokenFamily {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "gemini"):
		return tokenFamilyGemini
	case strings.Contains(lower, "claude"), strings.Contains(lower, "anthropic"):
		return tokenFamilyClaude
	default:
		return tokenFamilyOpenAI
	}
}

func isOpenAITextModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(lower, "gpt-") ||
		strings.HasPrefix(lower, "text-embedding-") ||
		strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "chatgpt-") ||
		strings.Contains(lower, "openai")
}

func messageContentForTokenCount(content interface{}) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []interface{}:
		var text strings.Builder
		for _, part := range v {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			if partText, ok := partMap["text"].(string); ok {
				text.WriteString(partText)
			}
		}
		return text.String()
	case []adapter.MessageContentPart:
		var text strings.Builder
		for _, part := range v {
			text.WriteString(part.Text)
		}
		return text.String()
	default:
		return fmt.Sprint(v)
	}
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7A3)
}

func isEmoji(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1F9FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0x1FA00 && r <= 0x1FAFF)
}

func isMathSymbol(r rune) bool {
	return strings.ContainsRune("∑∫∂√∞≤≥≠≈±×÷∈∉∋∌⊂⊃⊆⊇∪∩∧∨¬∀∃∄∅∆∇∝", r) ||
		(r >= 0x2200 && r <= 0x22FF) ||
		(r >= 0x2A00 && r <= 0x2AFF) ||
		(r >= 0x1D400 && r <= 0x1D7FF)
}
