package graphflow

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultmodel "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/model"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	llmsharedtypes "github.com/zgiai/ginext/internal/modules/llm/shared/types"
	shared_model "github.com/zgiai/ginext/internal/modules/shared/model"
)

type graphPromptDefaultModelService struct{}

func (s *graphPromptDefaultModelService) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType shared_model.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	return &llmdefaultservice.ResolvedModel{UseCase: string(llmmodelmodel.UseCaseTextChat), Provider: "openai", Model: "test-model", Params: llmsharedtypes.JSONObject{}, Source: llmdefaultservice.SourceExplicit}, nil
}

func (s *graphPromptDefaultModelService) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
	return &llmdefaultservice.ResolvedModel{UseCase: string(useCase), Provider: "openai", Model: "test-model", Params: llmsharedtypes.JSONObject{}, Source: llmdefaultservice.SourceExplicit}, nil
}

func (s *graphPromptDefaultModelService) ListResolved(ctx context.Context, organizationID uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	return []*llmdefaultservice.ResolvedModel{}, nil
}

func (s *graphPromptDefaultModelService) Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string, params llmsharedtypes.JSONObject) (*llmdefaultmodel.DefaultModel, error) {
	return nil, nil
}

func (s *graphPromptDefaultModelService) Delete(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) error {
	return nil
}

type graphPromptLLMClient struct {
	lastPrompt string
}

func (m *graphPromptLLMClient) Chat(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	if len(req.Messages) > 0 {
		if content, ok := req.Messages[0].Content.(string); ok {
			m.lastPrompt = content
		}
	}
	return &llmadapter.ChatResponse{Choices: []llmadapter.Choice{{Message: llmadapter.Message{Content: `{"entities":["Seed"]}`}}}}, nil
}

func (m *graphPromptLLMClient) ChatStream(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) CreateResponse(ctx context.Context, organizationID string, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) Embed(ctx context.Context, organizationID string, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) CreateImage(ctx context.Context, organizationID string, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) Rerank(ctx context.Context, organizationID string, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) AppChat(ctx context.Context, appCtx *client.AppContext, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) AppChatStream(ctx context.Context, appCtx *client.AppContext, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) AppCreateResponse(ctx context.Context, appCtx *client.AppContext, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) AppEmbed(ctx context.Context, appCtx *client.AppContext, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) AppCreateImage(ctx context.Context, appCtx *client.AppContext, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, nil
}
func (m *graphPromptLLMClient) AppRerank(ctx context.Context, appCtx *client.AppContext, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, nil
}

func TestExtractQueryEntitiesUsesGoTemplate(t *testing.T) {
	llmClient := &graphPromptLLMClient{}
	svc := NewService(&config.Config{}, nil, nil, nil, llmClient, &graphPromptDefaultModelService{}, nil)

	entities, err := svc.ExtractQueryEntities(context.Background(), "org-1", "renewable energy", nil, nil)
	if err != nil {
		t.Fatalf("ExtractQueryEntities returned error: %v", err)
	}
	if len(entities) != 1 || entities[0] != "Seed" {
		t.Fatalf("unexpected entities: %#v", entities)
	}
	if !strings.Contains(llmClient.lastPrompt, "renewable energy") {
		t.Fatalf("prompt = %q, want rendered query", llmClient.lastPrompt)
	}
	if strings.Contains(llmClient.lastPrompt, "{{") {
		t.Fatalf("prompt still contains raw template markers: %q", llmClient.lastPrompt)
	}
}
