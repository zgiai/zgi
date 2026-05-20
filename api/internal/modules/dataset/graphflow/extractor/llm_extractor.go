package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/ginext/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/ginext/internal/modules/shared/model"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
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
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}

	resp, err := e.client.Chat(ctx, tenantID, req)
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
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}

	resp, err := e.client.Chat(ctx, tenantID, req)
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
