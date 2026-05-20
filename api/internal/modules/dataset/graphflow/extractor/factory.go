package extractor

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/openie"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Strategy constants for extraction strategies.
const (
	StrategyLLM    = "llm"
	StrategyOpenIE = "openie"
)

// NewExtractorByStrategy creates an extractor for the requested strategy.
func NewExtractorByStrategy(strategy string, llmClient client.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService, model *string, provider *string) Extractor {
	switch strategy {
	case StrategyOpenIE:
		adapter := &gatewayClientAdapter{
			client:          llmClient,
			defaultModelSvc: defaultModelSvc,
			model:           model,
			provider:        provider,
		}
		return &OpenIEAdapter{extractor: openie.NewExtractor(adapter)}
	default:
		return NewLLMExtractor(llmClient, defaultModelSvc, model, provider)
	}
}

// OpenIEAdapter adapts openie.Extractor to the Extractor interface.
type OpenIEAdapter struct {
	extractor *openie.Extractor
}

// GenerateGlobalEntities is not supported yet and returns an empty list.
func (a *OpenIEAdapter) GenerateGlobalEntities(ctx context.Context, tenantID string, text string) ([]string, error) {
	return []string{}, nil
}

// Extract implements the Extractor interface.
func (a *OpenIEAdapter) Extract(ctx context.Context, tenantID string, text string, documentTitle string, globalEntities []string) (*ExtractionResult, error) {
	ctx = context.WithValue(ctx, "tenant_id", tenantID)
	result, err := a.extractor.Extract(ctx, text)
	if err != nil {
		return nil, err
	}

	entities := make([]ExtractedEntity, 0)
	for _, e := range result.Entities {
		switch v := e.(type) {
		case string:
			entities = append(entities, ExtractedEntity{Name: v})
		case map[string]interface{}:
			entity := ExtractedEntity{}
			if name, ok := v["name"].(string); ok {
				entity.Name = name
			}
			if typ, ok := v["type"].(string); ok {
				entity.Type = typ
			}
			entities = append(entities, entity)
		}
	}

	relationships := make([]ExtractedRelationship, len(result.Triples))
	for i, t := range result.Triples {
		relationships[i] = ExtractedRelationship{
			Source: t.Subject,
			Target: t.Object,
			Type:   t.Predicate,
		}
	}

	return &ExtractionResult{
		Entities:      entities,
		Relationships: relationships,
	}, nil
}

// gatewayClientAdapter adapts LLMClient to the openie.LLMClient interface.
type gatewayClientAdapter struct {
	client          client.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
	model           *string
	provider        *string
}

func (a *gatewayClientAdapter) Complete(ctx context.Context, prompt string) (string, error) {
	tenantID, ok := ctx.Value("tenant_id").(string)
	if !ok {
		return "", fmt.Errorf("tenant_id missing in context")
	}

	temp := 0.1
	resolvedModel, err := resolveTextChatModel(ctx, tenantID, a.defaultModelSvc, a.provider, a.model)
	if err != nil {
		return "", err
	}

	req := &adapter.ChatRequest{
		Provider: resolvedModel.Provider,
		Model:    resolvedModel.Model,
		Messages: []adapter.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: &temp,
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}

	resp, err := a.client.Chat(ctx, tenantID, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}

	content := resp.Choices[0].Message.Content
	contentStr, ok := content.(string)
	if !ok {
		return "", fmt.Errorf("response content is not string")
	}

	return contentStr, nil
}
