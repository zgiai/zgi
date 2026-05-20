package titlegen_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	defaultmodelmodel "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/model"
	defaultmodelservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	llmsharedtypes "github.com/zgiai/ginext/internal/modules/llm/shared/types"
	sharedmodel "github.com/zgiai/ginext/internal/modules/shared/model"
	"github.com/zgiai/ginext/internal/modules/shared/titlegen"
)

func TestGenerateUsesModelTitle(t *testing.T) {
	orgID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	conversationID := uuid.New().String()
	llm := &fakeLLMClient{chatContent: `{"title":"周末旅行计划"}`}
	defaults := &fakeDefaultModelService{resolved: &defaultmodelservice.ResolvedModel{
		Provider: "openai",
		Model:    "gpt-test",
	}}

	svc := titlegen.NewService(llm, defaults)
	result, err := svc.Generate(context.Background(), titlegen.GenerateRequest{
		OrganizationID: orgID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
		AppID:          conversationID,
		AppType:        "aichat",
		SessionID:      conversationID,
		ConversationID: conversationID,
		Messages:       []titlegen.Message{{Role: "user", Content: "帮我规划周末去杭州的旅行"}},
		FallbackTitle:  "帮我规划周末去杭州",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Title != "周末旅行计划" || result.Source != titlegen.SourceModel {
		t.Fatalf("result = %#v, want model title", result)
	}
	if len(llm.appContexts) != 1 {
		t.Fatalf("app context count = %d, want 1", len(llm.appContexts))
	}
	appCtx := llm.appContexts[0]
	if appCtx.AppType != "aichat" || appCtx.AppID != conversationID || appCtx.ConversationID != conversationID {
		t.Fatalf("unexpected app context = %#v", appCtx)
	}
	if appCtx.WorkspaceID != workspaceID.String() {
		t.Fatalf("workspace_id = %q, want %s", appCtx.WorkspaceID, workspaceID)
	}
	if len(llm.requests) != 1 || llm.requests[0].Model != "gpt-test" || llm.requests[0].Provider != "openai" {
		t.Fatalf("request = %#v, want default model", llm.requests)
	}
}

func TestGenerateFallsBackOnInvalidJSON(t *testing.T) {
	svc := titlegen.NewService(
		&fakeLLMClient{chatContent: `not json`},
		&fakeDefaultModelService{resolved: &defaultmodelservice.ResolvedModel{Provider: "openai", Model: "gpt-test"}},
	)
	result, err := svc.Generate(context.Background(), validRequest("Fallback Title"))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Title != "Fallback Title" || result.Source != titlegen.SourceFallback {
		t.Fatalf("result = %#v, want fallback", result)
	}
}

func TestGenerateFallsBackWhenDefaultModelUnavailable(t *testing.T) {
	svc := titlegen.NewService(
		&fakeLLMClient{chatContent: `{"title":"周末旅行计划"}`},
		&fakeDefaultModelService{err: errors.New("missing default model")},
	)
	result, err := svc.Generate(context.Background(), validRequest("Fallback Title"))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Title != "Fallback Title" || result.Source != titlegen.SourceFallback {
		t.Fatalf("result = %#v, want fallback", result)
	}
}

func validRequest(fallback string) titlegen.GenerateRequest {
	conversationID := uuid.New().String()
	return titlegen.GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		AppID:          conversationID,
		AppType:        "aichat",
		SessionID:      conversationID,
		ConversationID: conversationID,
		Messages:       []titlegen.Message{{Role: "user", Content: "Help me plan a trip"}},
		FallbackTitle:  fallback,
	}
}

type fakeDefaultModelService struct {
	resolved *defaultmodelservice.ResolvedModel
	err      error
}

func (f *fakeDefaultModelService) ResolveModelType(context.Context, string, *string, *string, sharedmodel.ModelType) (*defaultmodelservice.ResolvedModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDefaultModelService) ResolveUseCase(context.Context, string, llmmodelmodel.UseCase, *string, *string) (*defaultmodelservice.ResolvedModel, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resolved, nil
}

func (f *fakeDefaultModelService) ListResolved(context.Context, uuid.UUID) ([]*defaultmodelservice.ResolvedModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDefaultModelService) Upsert(context.Context, uuid.UUID, *uuid.UUID, llmmodelmodel.UseCase, string, string, llmsharedtypes.JSONObject) (*defaultmodelmodel.DefaultModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDefaultModelService) Delete(context.Context, uuid.UUID, llmmodelmodel.UseCase) error {
	return errors.New("not implemented")
}

type fakeLLMClient struct {
	chatContent string
	requests    []*adapter.ChatRequest
	appContexts []*llmclient.AppContext
}

func (f *fakeLLMClient) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppChat(_ context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.appContexts = append(f.appContexts, appCtx)
	f.requests = append(f.requests, req)
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{Message: adapter.Message{Content: f.chatContent}}},
	}, nil
}

func (f *fakeLLMClient) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}
