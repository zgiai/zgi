package knowledgeretrieval

import (
	"context"
	"errors"
	"testing"

	datasetmodel "github.com/zgiai/ginext/internal/modules/dataset/model"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

type mockLLMClientForEmbed struct {
	lastAppCtx *llmclient.AppContext
	lastReq    *llmadapter.EmbeddingsRequest
	resp       *llmadapter.EmbeddingsResponse
	err        error
}

func (m *mockLLMClientForEmbed) Chat(ctx context.Context, tenantID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) ChatStream(ctx context.Context, tenantID string, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) CreateResponse(ctx context.Context, tenantID string, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) Embed(ctx context.Context, tenantID string, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) CreateImage(ctx context.Context, tenantID string, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) Rerank(ctx context.Context, tenantID string, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	m.lastAppCtx = appCtx
	m.lastReq = req
	return m.resp, m.err
}

func (m *mockLLMClientForEmbed) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClientForEmbed) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

type mockEmbeddingInvoker struct {
	accountID string
	appID     string
	appType   string
	texts     []string
	model     string
	resp      [][]float64
	err       error
}

func (m *mockEmbeddingInvoker) Embed(ctx context.Context, accountID, appID, appType string, texts []string, model string) ([][]float64, error) {
	m.accountID = accountID
	m.appID = appID
	m.appType = appType
	m.texts = texts
	m.model = model
	return m.resp, m.err
}

func TestNewGatewayEmbeddingInvokerRequiresWorkspaceID(t *testing.T) {
	inv, err := NewGatewayEmbeddingInvoker(&mockLLMClientForEmbed{}, "", "", "")
	if err == nil {
		t.Fatalf("expected error when workspace_id is empty")
	}
	if inv != nil {
		t.Fatalf("expected nil invoker when workspace_id is empty")
	}
}

func TestGatewayEmbeddingInvokerEmbedSuccess(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockLLMClientForEmbed{
		resp: &llmadapter.EmbeddingsResponse{
			Data: []llmadapter.Embedding{
				{Embedding: []float32{1.0, 2.0}},
			},
			Model: "text-embedding-3-large",
		},
	}
	inv, err := NewGatewayEmbeddingInvoker(mockClient, "", "ws-1", llmclient.BillingSubjectTypeOrganization)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	vecs, err := inv.Embed(ctx, "acc-1", "app-1", "dataset", []string{"hello"}, "text-embedding-3-large")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vecs))
	}
	if mockClient.lastReq == nil || mockClient.lastReq.Model != "text-embedding-3-large" {
		t.Fatalf("expected request model to be set")
	}
	if mockClient.lastAppCtx == nil || mockClient.lastAppCtx.AccountID != "acc-1" || mockClient.lastAppCtx.AppID != "app-1" || mockClient.lastAppCtx.AppType != "dataset" {
		t.Fatalf("expected account/app context to be passed")
	}
	if mockClient.lastAppCtx.WorkspaceID != "ws-1" {
		t.Fatalf("expected workspace_id ws-1, got %s", mockClient.lastAppCtx.WorkspaceID)
	}
	if mockClient.lastAppCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("expected billing subject type %q, got %q", llmclient.BillingSubjectTypeOrganization, mockClient.lastAppCtx.BillingSubjectType)
	}
}

func TestGatewayEmbeddingInvokerEmbedError(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockLLMClientForEmbed{
		err: errors.New("embed failure"),
	}
	inv, err := NewGatewayEmbeddingInvoker(mockClient, "", "ws-1", "")
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	_, err = inv.Embed(ctx, "acc-1", "app-1", "dataset", []string{"hello"}, "model-x")
	if err == nil {
		t.Fatalf("expected error but got nil")
	}
}

func TestGatewayEmbeddingServiceUsesInvoker(t *testing.T) {
	ctx := context.Background()
	mockInv := &mockEmbeddingInvoker{
		resp: [][]float64{{0.1, 0.2}},
	}
	svc := newGatewayEmbeddingService(mockInv, "acc-2", "app-2", "dataset", "emb-model")

	vec, err := svc.EmbedText(ctx, "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 2 || vec[0] != 0.1 {
		t.Fatalf("unexpected vector: %#v", vec)
	}
	if mockInv.accountID != "acc-2" || mockInv.appID != "app-2" || mockInv.appType != "dataset" || mockInv.model != "emb-model" {
		t.Fatalf("invoker context not set as expected")
	}
	if len(mockInv.texts) != 1 || mockInv.texts[0] != "hi" {
		t.Fatalf("invoker texts not captured")
	}
}

func TestEmbeddingFactoryUsesDatasetModel(t *testing.T) {
	accountID := "acc-3"
	mockInv := &mockEmbeddingInvoker{
		resp: [][]float64{{0.3}},
	}
	svc := &defaultRetrievalService{embInvoker: mockInv}
	factory := svc.makeEmbeddingFactory(accountID)
	if factory == nil {
		t.Fatalf("expected embedding factory")
	}

	// Dataset without custom model uses default
	ds := &datasetmodel.Dataset{ID: "ds-1"}
	embSvc := factory(ds)
	if embSvc == nil {
		t.Fatalf("expected embedding service from factory")
	}
	if _, err := embSvc.EmbedText(context.Background(), "query"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockInv.model != "text-embedding-3-large" {
		t.Fatalf("expected default model, got %s", mockInv.model)
	}

	// Dataset with custom model
	custom := "custom-emb"
	ds2 := &datasetmodel.Dataset{ID: "ds-2", EmbeddingModel: &custom}
	embSvc2 := factory(ds2)
	if embSvc2 == nil {
		t.Fatalf("expected embedding service for custom model")
	}
	if _, err := embSvc2.EmbedText(context.Background(), "query"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockInv.model != custom {
		t.Fatalf("expected custom model, got %s", mockInv.model)
	}
	if mockInv.appID != "ds-2" {
		t.Fatalf("expected appID to match dataset ID, got %s", mockInv.appID)
	}
}
