package extractor

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultmodel "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/model"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	llmsharedtypes "github.com/zgiai/ginext/internal/modules/llm/shared/types"
	shared_model "github.com/zgiai/ginext/internal/modules/shared/model"
)

type extractorPromptDefaultModelService struct{}

func (s *extractorPromptDefaultModelService) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType shared_model.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	return &llmdefaultservice.ResolvedModel{UseCase: string(llmmodelmodel.UseCaseTextChat), Provider: "openai", Model: "test-model", Params: llmsharedtypes.JSONObject{}, Source: llmdefaultservice.SourceExplicit}, nil
}

func (s *extractorPromptDefaultModelService) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
	return &llmdefaultservice.ResolvedModel{UseCase: string(useCase), Provider: "openai", Model: "test-model", Params: llmsharedtypes.JSONObject{}, Source: llmdefaultservice.SourceExplicit}, nil
}

func (s *extractorPromptDefaultModelService) ListResolved(ctx context.Context, organizationID uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	return []*llmdefaultservice.ResolvedModel{}, nil
}

func (s *extractorPromptDefaultModelService) Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string, params llmsharedtypes.JSONObject) (*llmdefaultmodel.DefaultModel, error) {
	return nil, nil
}

func (s *extractorPromptDefaultModelService) Delete(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) error {
	return nil
}

type extractorPromptLLMClient struct {
	lastPrompt string
	response   string
}

func (m *extractorPromptLLMClient) Chat(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	if len(req.Messages) > 0 {
		if content, ok := req.Messages[0].Content.(string); ok {
			m.lastPrompt = content
		}
	}
	return &llmadapter.ChatResponse{Choices: []llmadapter.Choice{{Message: llmadapter.Message{Content: m.response}}}}, nil
}

func (m *extractorPromptLLMClient) ChatStream(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) CreateResponse(ctx context.Context, organizationID string, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) Embed(ctx context.Context, organizationID string, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) CreateImage(ctx context.Context, organizationID string, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) Rerank(ctx context.Context, organizationID string, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) AppChat(ctx context.Context, appCtx *client.AppContext, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) AppChatStream(ctx context.Context, appCtx *client.AppContext, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) AppCreateResponse(ctx context.Context, appCtx *client.AppContext, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) AppEmbed(ctx context.Context, appCtx *client.AppContext, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) AppCreateImage(ctx context.Context, appCtx *client.AppContext, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, nil
}
func (m *extractorPromptLLMClient) AppRerank(ctx context.Context, appCtx *client.AppContext, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, nil
}

func TestGenerateGlobalEntitiesUsesGoTemplate(t *testing.T) {
	llmClient := &extractorPromptLLMClient{response: `{"core_entities":["Alpha"]}`}
	extractor := NewLLMExtractor(llmClient, &extractorPromptDefaultModelService{}, nil, nil)

	entities, err := extractor.GenerateGlobalEntities(context.Background(), "org-1", "Document body")
	if err != nil {
		t.Fatalf("GenerateGlobalEntities returned error: %v", err)
	}
	if len(entities) != 1 || entities[0] != "Alpha" {
		t.Fatalf("unexpected entities: %#v", entities)
	}
	if !strings.Contains(llmClient.lastPrompt, "Document body") {
		t.Fatalf("prompt = %q, want rendered document text", llmClient.lastPrompt)
	}
	if strings.Contains(llmClient.lastPrompt, "{{") {
		t.Fatalf("prompt still contains raw template markers: %q", llmClient.lastPrompt)
	}
}

func TestExtractUsesGoTemplate(t *testing.T) {
	llmClient := &extractorPromptLLMClient{response: `{"entities":[{"name":"Alice","type":"Person"}],"relations":[]}`}
	extractor := NewLLMExtractor(llmClient, &extractorPromptDefaultModelService{}, nil, nil)

	result, err := extractor.Extract(context.Background(), "org-1", "Segment text", "Doc Title", []string{"Alpha", "Beta"})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(result.Entities) != 1 || result.Entities[0].Name != "Alice" {
		t.Fatalf("unexpected extraction result: %#v", result)
	}
	for _, expected := range []string{"Doc Title", "Segment text", "Alpha", "Beta"} {
		if !strings.Contains(llmClient.lastPrompt, expected) {
			t.Fatalf("prompt = %q, want %q", llmClient.lastPrompt, expected)
		}
	}
	if strings.Contains(llmClient.lastPrompt, "{{") {
		t.Fatalf("prompt still contains raw template markers: %q", llmClient.lastPrompt)
	}
}
