package systemvlm

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeVisionResolver struct {
	resolved *llmdefaultservice.ResolvedModel
}

func (f *fakeVisionResolver) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
	if useCase != llmmodelmodel.UseCaseVision {
		return nil, llmdefaultservice.ErrInvalidUseCase
	}
	return f.resolved, nil
}

type fakeVisionChatClient struct {
	request *llmadapter.ChatRequest
}

func (f *fakeVisionChatClient) Chat(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	f.request = req
	return &llmadapter.ChatResponse{
		Model: req.Model,
		Choices: []llmadapter.Choice{
			{
				Message: llmadapter.Message{
					Content: "# Invoice\n\nTotal: $42",
				},
				FinishReason: "stop",
			},
		},
		Usage: &llmadapter.Usage{PromptTokens: 12},
	}, nil
}

func TestSystemVLMAdapterUsesDefaultVisionModel(t *testing.T) {
	orgID := uuid.NewString()
	chatClient := &fakeVisionChatClient{}
	adapter := NewAdapter(chatClient, &fakeVisionResolver{
		resolved: &llmdefaultservice.ResolvedModel{
			Provider: "openai",
			Model:    "gpt-vision",
			Source:   llmdefaultservice.SourceExplicit,
		},
	})

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "scan.png",
		Data:       []byte("not-a-real-image-but-ok-for-data-url"),
		Intent:     contracts.ParseIntentPreview,
		Metadata: map[string]any{
			"organization_id": orgID,
		},
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact == nil || artifact.EngineUsed != contracts.ParseEngineVLM {
		t.Fatalf("artifact.EngineUsed = %v, want vlm", artifact)
	}
	if got := artifact.Metadata["system_vlm_model"]; got != "gpt-vision" {
		t.Fatalf("system_vlm_model = %v", got)
	}
	if artifact.Metadata["vision_image_document"] != true || artifact.Metadata["recommended_chunking"] != "full-doc" {
		t.Fatalf("image parsing metadata = %#v", artifact.Metadata)
	}
	if chatClient.request == nil {
		t.Fatalf("expected LLM chat request")
	}
	if chatClient.request.Provider != "openai" || chatClient.request.Model != "gpt-vision" {
		t.Fatalf("chat provider/model = %q/%q", chatClient.request.Provider, chatClient.request.Model)
	}
	if chatClient.request.ResponseFormat != nil {
		t.Fatalf("response format = %#v, want plain text", chatClient.request.ResponseFormat)
	}
	content, ok := chatClient.request.Messages[0].Content.([]llmadapter.MessageContentPart)
	if !ok {
		t.Fatalf("message content type = %T, want []MessageContentPart", chatClient.request.Messages[0].Content)
	}
	if len(content) < 2 || content[0].Type != "text" || content[0].Text == "" || content[1].Type != "image_url" {
		t.Fatalf("content parts = %#v, want image_url part", content)
	}
	if content[1].ImageURL == nil || !strings.HasPrefix(content[1].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("image content = %#v, want PNG data URI", content[1])
	}
	prompt := content[0].Text
	if !strings.Contains(prompt, "directly as Markdown") || !strings.Contains(prompt, "Flowchart or process diagram") {
		t.Fatalf("unexpected image understanding prompt: %q", prompt)
	}
}

func TestSystemVLMAdapterParsesUploadFileWithLoadedBytes(t *testing.T) {
	orgID := uuid.NewString()
	chatClient := &fakeVisionChatClient{}
	adapter := NewAdapter(chatClient, &fakeVisionResolver{
		resolved: &llmdefaultservice.ResolvedModel{
			Provider: "openai",
			Model:    "gpt-vision",
			Source:   llmdefaultservice.SourceExplicit,
		},
	})

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeUploadFile,
		SourceRef:  "file-1",
		FileName:   "scan.png",
		Data:       []byte("loaded-upload-bytes"),
		Intent:     contracts.ParseIntentDatasetIndex,
		Metadata: map[string]any{
			"organization_id": orgID,
		},
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.SourceType != contracts.ParseSourceTypeUploadFile {
		t.Fatalf("SourceType = %q, want upload_file", artifact.SourceType)
	}
	if artifact.EngineUsed != contracts.ParseEngineVLM {
		t.Fatalf("EngineUsed = %q, want vlm", artifact.EngineUsed)
	}
}

func TestSystemVLMAdapterRequiresOrganizationID(t *testing.T) {
	adapter := NewAdapter(&fakeVisionChatClient{}, &fakeVisionResolver{
		resolved: &llmdefaultservice.ResolvedModel{Provider: "openai", Model: "gpt-vision"},
	})

	_, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "scan.png",
		Data:       []byte("image"),
		Intent:     contracts.ParseIntentPreview,
	})
	if err == nil || !strings.Contains(err.Error(), "organization_id") {
		t.Fatalf("Parse() error = %v, want organization_id error", err)
	}
}
