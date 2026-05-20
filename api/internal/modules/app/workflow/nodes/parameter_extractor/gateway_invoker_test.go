package parameterextractor

import (
	"context"
	"errors"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type mockGatewayLLMClient struct {
	chatCalled          bool
	chatStreamCalled    bool
	appChatCalled       bool
	appChatStreamCalled bool
	lastAppCtx          *llmclient.AppContext
}

func (m *mockGatewayLLMClient) Chat(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	m.chatCalled = true
	return nil, errors.New("Chat should not be called")
}

func (m *mockGatewayLLMClient) ChatStream(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	m.chatStreamCalled = true
	return nil, errors.New("ChatStream should not be called")
}

func (m *mockGatewayLLMClient) CreateResponse(ctx context.Context, organizationID string, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) Embed(ctx context.Context, organizationID string, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) CreateImage(ctx context.Context, organizationID string, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) Rerank(ctx context.Context, organizationID string, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	m.appChatCalled = true
	m.lastAppCtx = appCtx
	return &llmadapter.ChatResponse{
		Choices: []llmadapter.Choice{
			{
				Message: llmadapter.Message{Role: "assistant", Content: "ok"},
			},
		},
	}, nil
}

func (m *mockGatewayLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	m.appChatStreamCalled = true
	m.lastAppCtx = appCtx
	ch := make(chan llmadapter.StreamResponse, 1)
	close(ch)
	return ch, nil
}

func (m *mockGatewayLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockGatewayLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func TestNewGatewayLLMInvoker_RequiresWorkspaceID(t *testing.T) {
	invoker, err := NewGatewayLLMInvoker(&mockGatewayLLMClient{}, "", "", "")
	if err == nil {
		t.Fatalf("expected constructor error when workspace_id is empty")
	}
	if invoker != nil {
		t.Fatalf("expected nil invoker when constructor fails")
	}
}

func TestGatewayLLMInvoker_Invoke_UsesAppChat(t *testing.T) {
	mockClient := &mockGatewayLLMClient{}
	invoker, err := NewGatewayLLMInvoker(mockClient, "org-1", "ws-1", llmclient.BillingSubjectTypeOrganization)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	resp, err := invoker.Invoke(context.Background(), "acc-1", "app-1", AppType, &InvokeRequest{
		ModelSlug: "test-model",
		Messages:  []PromptMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if resp == nil || resp.Text != "ok" {
		t.Fatalf("unexpected invoke response: %#v", resp)
	}

	if mockClient.chatCalled {
		t.Fatalf("expected Chat not to be called")
	}
	if !mockClient.appChatCalled {
		t.Fatalf("expected AppChat to be called")
	}
	if mockClient.lastAppCtx == nil {
		t.Fatalf("expected app context to be passed")
	}
	if mockClient.lastAppCtx.WorkspaceID != "ws-1" {
		t.Fatalf("unexpected workspace_id: %s", mockClient.lastAppCtx.WorkspaceID)
	}
	if mockClient.lastAppCtx.OrganizationID != "org-1" {
		t.Fatalf("unexpected organization_id: %s", mockClient.lastAppCtx.OrganizationID)
	}
	if mockClient.lastAppCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("unexpected billing_subject_type: %s", mockClient.lastAppCtx.BillingSubjectType)
	}
}

func TestGatewayLLMInvoker_InvokeStream_UsesAppChatStream(t *testing.T) {
	mockClient := &mockGatewayLLMClient{}
	invoker, err := NewGatewayLLMInvoker(mockClient, "org-1", "ws-1", "")
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	_, _, err = invoker.InvokeStream(context.Background(), "acc-1", "app-1", AppType, &InvokeRequest{
		ModelSlug: "test-model",
		Messages:  []PromptMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if mockClient.chatStreamCalled {
		t.Fatalf("expected ChatStream not to be called")
	}
	if !mockClient.appChatStreamCalled {
		t.Fatalf("expected AppChatStream to be called")
	}
}
