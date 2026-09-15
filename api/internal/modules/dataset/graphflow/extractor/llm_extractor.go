package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const (
	globalEntityExtractionMaxTokens = 1024
	// Graph extraction only needs a compact JSON document, but reasoning models
	// spend part of the completion budget on hidden reasoning tokens. A small cap
	// truncates the JSON and surfaces as "unexpected end of JSON input", so the
	// budget must leave room for both reasoning and the entity/relationship list.
	segmentExtractionMaxTokens    = 16384
	globalEntityExtractionTimeout = 30 * time.Second
	// Reasoning models answer a single segment in tens of seconds; this budget has
	// to cover the slowest accepted model rather than the median one.
	segmentExtractionTimeout = 120 * time.Second
)

type LLMExtractor struct {
	client          client.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
	model           *string
	provider        *string
}

// LLMExtractor implements Extractor interface.
func NewLLMExtractor(llmClient client.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService, model *string, provider *string) *LLMExtractor {
	return &LLMExtractor{
		client:          llmClient,
		defaultModelSvc: defaultModelSvc,
		model:           model,
		provider:        provider,
	}
}

// GenerateGlobalEntities generates the list of global core entities.
func (e *LLMExtractor) GenerateGlobalEntities(ctx context.Context, tenantID string, text string) ([]string, error) {
	if e.client == nil {
		return nil, fmt.Errorf("llm client is not initialized")
	}

	promptText, err := renderGlobalEntitySummaryPrompt(text)
	if err != nil {
		return nil, fmt.Errorf("failed to render global entity summary prompt: %w", err)
	}

	temp := 0.1
	maxTokens := globalEntityExtractionMaxTokens
	resolvedModel, err := resolveTextChatModel(ctx, tenantID, e.defaultModelSvc, e.provider, e.model)
	if err != nil {
		return nil, err
	}

	req := &adapter.ChatRequest{
		Provider: resolvedModel.Provider,
		Model:    resolvedModel.Model,
		Messages: []adapter.Message{
			{Role: "user", Content: promptText},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}
	applyThinkingPolicy(req)

	requestCtx, cancel := context.WithTimeout(ctx, globalEntityExtractionTimeout)
	defer cancel()

	resp, err := e.client.Chat(requestCtx, tenantID, req)
	if err != nil {
		return nil, fmt.Errorf("llm global entity extraction failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	content := resp.Choices[0].Message.Content
	contentStr, ok := content.(string)
	if !ok {
		return nil, fmt.Errorf("llm response content is not a string")
	}

	contentStr = strings.TrimPrefix(contentStr, "```json")
	contentStr = strings.TrimSuffix(contentStr, "```")
	contentStr = strings.TrimSpace(contentStr)

	var result struct {
		CoreEntities []string `json:"core_entities"`
	}
	if err := json.Unmarshal([]byte(contentStr), &result); err != nil {
		return []string{}, nil
	}

	return result.CoreEntities, nil
}

func (e *LLMExtractor) Extract(ctx context.Context, tenantID string, text string, documentTitle string, globalEntities []string) (*ExtractionResult, error) {
	if e.client == nil {
		return nil, fmt.Errorf("llm client is not initialized")
	}

	globalContext := defaultGlobalContextValue
	if len(globalEntities) > 0 {
		globalContext = "- " + strings.Join(globalEntities, "\n- ")
	}

	if documentTitle == "" {
		documentTitle = defaultDocumentTitleValue
	}

	promptText, err := renderGraphExtractionPrompt(globalContext, documentTitle, text)
	if err != nil {
		return nil, fmt.Errorf("failed to render graph extraction prompt: %w", err)
	}

	temp := 0.1
	maxTokens := segmentExtractionMaxTokens
	resolvedModel, err := resolveTextChatModel(ctx, tenantID, e.defaultModelSvc, e.provider, e.model)
	if err != nil {
		return nil, err
	}

	req := &adapter.ChatRequest{
		Provider: resolvedModel.Provider,
		Model:    resolvedModel.Model,
		Messages: []adapter.Message{
			{
				Role:    "user",
				Content: promptText,
			},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}
	applyThinkingPolicy(req)

	requestCtx, cancel := context.WithTimeout(ctx, segmentExtractionTimeout)
	defer cancel()

	resp, err := e.client.Chat(requestCtx, tenantID, req)
	if err != nil {
		return nil, fmt.Errorf("llm extraction failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	content := resp.Choices[0].Message.Content
	contentStr, ok := content.(string)
	if !ok {
		return nil, fmt.Errorf("llm response content is not a string")
	}

	contentStr = strings.TrimPrefix(contentStr, "```json")
	contentStr = strings.TrimSuffix(contentStr, "```")
	contentStr = strings.TrimSpace(contentStr)

	var result ExtractionResult
	if err := json.Unmarshal([]byte(contentStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse llm output: %w", err)
	}

	logger.DebugContext(ctx, "graphflow llm extraction completed",
		zap.String("tenant_id", tenantID),
		zap.String("model", resolvedModel.Model),
		zap.Int("text_length", len(text)),
		zap.Int("document_title_length", len(documentTitle)),
		zap.Int("global_entities_count", len(globalEntities)),
		zap.Int("entities_count", len(result.Entities)),
		zap.Int("relationships_count", len(result.Relationships)),
	)

	return &result, nil
}

// thinkingDisablingParameters returns the provider-specific request fields that
// turn hidden reasoning off for a model that would otherwise emit it.
//
// Graph extraction asks for one compact JSON document per segment, so reasoning
// output only burns the completion budget and can truncate that JSON. Providers
// spell this differently: Alibaba-compatible endpoints use enable_thinking,
// while DeepSeek-compatible endpoints use thinking. Providers without a known
// switch get no extra field, so an unknown parameter can never break the call.
func thinkingDisablingParameters(provider, model string) map[string]interface{} {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))

	switch normalizedProvider {
	case "qwen", "dashscope", "deepseek":
		// Known switch; apply it even though the model name may not advertise it.
	case "openai":
		// Only the o-series always reasons; plain GPT models reject the field.
		if !isReasoningModelName(normalizedModel) {
			return nil
		}
	default:
		if !isReasoningModelName(normalizedModel) {
			return nil
		}
	}

	parameters := map[string]interface{}{"enable_thinking": false}
	if normalizedProvider != "qwen" && normalizedProvider != "dashscope" {
		parameters["thinking"] = map[string]interface{}{"type": "disabled"}
	}
	return parameters
}

// isReasoningModelName reports whether a model identifier names a family that
// always reasons, such as OpenAI o-series or Kimi thinking variants.
func isReasoningModelName(model string) bool {
	for _, marker := range []string{"thinking", "reasoner", "reasoning"} {
		if strings.Contains(model, marker) {
			return true
		}
	}
	for _, prefix := range []string{"o1", "o3", "o4"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// applyThinkingPolicy rewrites req so the model answers without hidden reasoning.
func applyThinkingPolicy(req *adapter.ChatRequest) {
	if req == nil {
		return
	}
	parameters := thinkingDisablingParameters(req.Provider, req.Model)
	if len(parameters) == 0 {
		return
	}
	if req.AdditionalParameters == nil {
		req.AdditionalParameters = make(map[string]interface{}, len(parameters))
	}
	for key, value := range parameters {
		req.AdditionalParameters[key] = value
	}
}

func resolveTextChatModel(ctx context.Context, organizationID string, defaultModelSvc llmdefaultservice.DefaultModelService, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
	resolved, err := llmruntime.NewModelResolver(defaultModelSvc).ResolveFromPointers(ctx, organizationID, explicitProvider, explicitModel, shared_model.ModelTypeLLM)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve text model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return nil, fmt.Errorf("default text model is not configured")
	}
	return resolved, nil
}
