package llm

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"

	llmClient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmAdapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// mockLLMClient implements LLMClient for testing.
type mockLLMClient struct {
	appChatStreamFn func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error)
}

func (m *mockLLMClient) Chat(ctx context.Context, tenantID string, req *llmAdapter.ChatRequest) (*llmAdapter.ChatResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) ChatStream(ctx context.Context, tenantID string, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) CreateResponse(ctx context.Context, tenantID string, req *llmAdapter.CreateResponseRequest) (*llmAdapter.CreateResponseResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) Embed(ctx context.Context, tenantID string, req *llmAdapter.EmbeddingsRequest) (*llmAdapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) CreateImage(ctx context.Context, tenantID string, req *llmAdapter.ImageRequest) (*llmAdapter.ImageResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) Rerank(ctx context.Context, tenantID string, req *llmAdapter.RerankRequest) (*llmAdapter.RerankResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) AppChat(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (*llmAdapter.ChatResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) AppChatStream(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
	if m.appChatStreamFn != nil {
		return m.appChatStreamFn(ctx, appCtx, req)
	}
	return nil, nil
}

func (m *mockLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.CreateResponseRequest) (*llmAdapter.CreateResponseResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) AppEmbed(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.EmbeddingsRequest) (*llmAdapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) AppCreateImage(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ImageRequest) (*llmAdapter.ImageResponse, error) {
	return nil, nil
}

func (m *mockLLMClient) AppRerank(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.RerankRequest) (*llmAdapter.RerankResponse, error) {
	return nil, nil
}

func TestNewGatewayLLMInvokerRequiresWorkspaceID(t *testing.T) {
	convey.Convey("NewGatewayLLMInvoker requires workspace_id", t, func() {
		invoker, err := NewGatewayLLMInvoker(&mockLLMClient{}, "", "", "")
		convey.So(invoker, convey.ShouldBeNil)
		convey.So(err, convey.ShouldNotBeNil)
	})
}

func TestNewGatewayLLMInvokerWithWorkspaceID(t *testing.T) {
	convey.Convey("NewGatewayLLMInvoker accepts explicit organization/workspace IDs", t, func() {
		invoker, err := NewGatewayLLMInvoker(&mockLLMClient{}, "org-1", "ws-1", llmClient.BillingSubjectTypeOrganization)
		convey.So(err, convey.ShouldBeNil)
		convey.So(invoker, convey.ShouldNotBeNil)

		gwInvoker, ok := invoker.(*gatewayLLMInvoker)
		convey.So(ok, convey.ShouldBeTrue)
		convey.So(gwInvoker.organizationID, convey.ShouldEqual, "org-1")
		convey.So(gwInvoker.workspaceID, convey.ShouldEqual, "ws-1")
		convey.So(gwInvoker.billingSubjectType, convey.ShouldEqual, llmClient.BillingSubjectTypeOrganization)
	})
}

func TestGatewayLLMInvokerInvokeStream_TextChunks(t *testing.T) {
	convey.Convey("InvokeStream forwards streaming text chunks", t, func() {
		receivedWorkspaceID := ""
		mockClient := &mockLLMClient{
			appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
				receivedWorkspaceID = appCtx.WorkspaceID
				convey.So(req.Provider, convey.ShouldEqual, "ollama")
				ch := make(chan llmAdapter.StreamResponse, 2)
				go func() {
					defer close(ch)
					ch <- llmAdapter.StreamResponse{
						Choices: []llmAdapter.StreamChoice{
							{Delta: llmAdapter.Message{Role: "assistant", Content: "hello"}},
						},
					}
					ch <- llmAdapter.StreamResponse{
						Choices: []llmAdapter.StreamChoice{
							{Delta: llmAdapter.Message{Role: "assistant", Content: "world"}, FinishReason: "stop"},
						},
						Done: true,
					}
				}()
				return ch, nil
			},
		}

		invoker, err := NewGatewayLLMInvoker(mockClient, "", "ws-1", llmClient.BillingSubjectTypeOrganization)
		convey.So(err, convey.ShouldBeNil)
		req := &LLMInvokeRequest{
			ProviderSlug: "ollama",
			ModelSlug:    "gpt-4",
			Messages: []PromptMessage{
				{Role: PromptMessageRoleUser, Content: "hi"},
			},
		}

		resultChan, errChan, err := invoker.InvokeStream(context.Background(), "account-1", "app-1", "agent", req)
		convey.So(err, convey.ShouldBeNil)

		var chunks []string
		for chunk := range resultChan {
			convey.So(chunk, convey.ShouldNotBeNil)
			convey.So(chunk.Delta, convey.ShouldNotBeNil)
			if chunk.Delta.Message == nil {
				continue
			}
			convey.So(chunk.Delta.Message.Role, convey.ShouldEqual, PromptMessageRoleAssistant)
			content, _ := chunk.Delta.Message.Content.(string)
			chunks = append(chunks, content)
		}
		convey.So(chunks, convey.ShouldResemble, []string{"hello", "world"})

		select {
		case e, ok := <-errChan:
			convey.So(ok, convey.ShouldBeFalse)
			convey.So(e, convey.ShouldBeNil)
		case <-time.After(time.Second):
			convey.So(false, convey.ShouldBeTrue) // timeout
		}
		convey.So(receivedWorkspaceID, convey.ShouldEqual, "ws-1")
	})
}

func TestGatewayLLMInvokerInvokeStream_Structured(t *testing.T) {
	convey.Convey("InvokeStream aggregates structured output", t, func() {
		mockClient := &mockLLMClient{
			appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
				ch := make(chan llmAdapter.StreamResponse, 3)
				go func() {
					defer close(ch)
					ch <- llmAdapter.StreamResponse{
						Choices: []llmAdapter.StreamChoice{
							{Delta: llmAdapter.Message{Role: "assistant", Content: `{"foo":`}},
						},
					}
					ch <- llmAdapter.StreamResponse{
						Choices: []llmAdapter.StreamChoice{
							{Delta: llmAdapter.Message{Role: "assistant", Content: `"bar"`}},
						},
					}
					ch <- llmAdapter.StreamResponse{
						Choices: []llmAdapter.StreamChoice{
							{Delta: llmAdapter.Message{Role: "assistant", Content: `}`}, FinishReason: "stop"},
						},
						Done: true,
					}
				}()
				return ch, nil
			},
		}

		invoker := &gatewayLLMInvoker{client: mockClient}
		req := &LLMInvokeRequest{
			ModelSlug:        "gpt-4",
			Messages:         []PromptMessage{{Role: PromptMessageRoleUser, Content: "hi"}},
			StructuredOutput: map[string]any{"type": "object"},
		}

		resultChan, errChan, err := invoker.InvokeStream(context.Background(), "account-1", "app-1", "agent", req)
		convey.So(err, convey.ShouldBeNil)

		var chunks []string
		var structured map[string]any
		for chunk := range resultChan {
			if chunk.StructuredOutput != nil {
				if m, ok := chunk.StructuredOutput.(map[string]any); ok {
					structured = m
				}
				continue
			}
			content, _ := chunk.Delta.Message.Content.(string)
			chunks = append(chunks, content)
		}

		convey.So(chunks, convey.ShouldResemble, []string{`{"foo":`, `"bar"`, `}`})
		convey.So(structured, convey.ShouldResemble, map[string]any{"foo": "bar"})

		select {
		case e, ok := <-errChan:
			convey.So(ok, convey.ShouldBeFalse)
			convey.So(e, convey.ShouldBeNil)
		case <-time.After(time.Second):
			convey.So(false, convey.ShouldBeTrue)
		}
	})
}

func TestGatewayLLMInvokerInvokeStream_PreservesVisionContentParts(t *testing.T) {
	convey.Convey("InvokeStream preserves multimodal image_url parts for gateway requests", t, func() {
		mockClient := &mockLLMClient{
			appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
				convey.So(req, convey.ShouldNotBeNil)
				convey.So(len(req.Messages), convey.ShouldEqual, 1)
				convey.So(req.Messages[0].Role, convey.ShouldEqual, string(PromptMessageRoleUser))

				contentParts, ok := req.Messages[0].Content.([]llmAdapter.MessageContentPart)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(len(contentParts), convey.ShouldEqual, 2)

				convey.So(contentParts[0].Type, convey.ShouldEqual, "image_url")
				convey.So(contentParts[0].ImageURL, convey.ShouldNotBeNil)
				convey.So(contentParts[0].ImageURL.URL, convey.ShouldEqual, "https://example.com/paper.jpg")
				convey.So(contentParts[0].ImageURL.Detail, convey.ShouldEqual, "high")

				convey.So(contentParts[1].Type, convey.ShouldEqual, "text")
				convey.So(contentParts[1].Text, convey.ShouldEqual, "Analyze the uploaded image or file directly. Use all visible content, including questions, answers, annotations, scores, diagrams, and layout details, to complete the task.")

				ch := make(chan llmAdapter.StreamResponse, 1)
				go func() {
					defer close(ch)
					ch <- llmAdapter.StreamResponse{
						Choices: []llmAdapter.StreamChoice{
							{Delta: llmAdapter.Message{Role: "assistant", Content: "vision ok"}, FinishReason: "stop"},
						},
						Done: true,
					}
				}()
				return ch, nil
			},
		}

		invoker, err := NewGatewayLLMInvoker(mockClient, "org-1", "ws-1", "")
		convey.So(err, convey.ShouldBeNil)

		req := &LLMInvokeRequest{
			ModelSlug: "gpt-4o",
			Messages: []PromptMessage{
				{
					Role: PromptMessageRoleUser,
					Content: []PromptMessageContent{
						{
							Type:   PromptMessageContentTypeImage,
							URL:    "https://example.com/paper.jpg",
							Detail: ImageDetailHigh,
						},
						{
							Type: PromptMessageContentTypeText,
							Data: "Analyze the uploaded image or file directly. Use all visible content, including questions, answers, annotations, scores, diagrams, and layout details, to complete the task.",
						},
					},
				},
			},
		}

		resultChan, errChan, err := invoker.InvokeStream(context.Background(), "account-1", "app-1", "agent", req)
		convey.So(err, convey.ShouldBeNil)

		var chunks []string
		for chunk := range resultChan {
			if chunk.Delta == nil || chunk.Delta.Message == nil {
				continue
			}
			content, _ := chunk.Delta.Message.Content.(string)
			if content != "" {
				chunks = append(chunks, content)
			}
		}

		convey.So(chunks, convey.ShouldResemble, []string{"vision ok"})

		select {
		case e, ok := <-errChan:
			convey.So(ok, convey.ShouldBeFalse)
			convey.So(e, convey.ShouldBeNil)
		case <-time.After(time.Second):
			convey.So(false, convey.ShouldBeTrue)
		}
	})
}
