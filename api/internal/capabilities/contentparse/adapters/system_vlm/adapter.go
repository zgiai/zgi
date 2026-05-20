package systemvlm

import (
	"context"
	"fmt"
	"strings"

	hyperparsesdk "github.com/zgiai/ginext/internal/capabilities/contentparse/adapters/hyperparse_sdk"
	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	extractvlm "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/vlm"
	"github.com/zgiai/ginext/internal/contracts"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const adapterName = "system_vlm"

type VisionChatClient interface {
	Chat(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error)
}

type DefaultVisionModelResolver interface {
	ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error)
}

type Adapter struct {
	llmClient          VisionChatClient
	defaultModelSvc    DefaultVisionModelResolver
	fallbackModelName  string
	defaultTemperature float64
}

func NewAdapter(llmClient VisionChatClient, defaultModelSvc DefaultVisionModelResolver) *Adapter {
	return &Adapter{
		llmClient:          llmClient,
		defaultModelSvc:    defaultModelSvc,
		defaultTemperature: 0,
	}
}

func (a *Adapter) Name() string {
	return adapterName
}

func (a *Adapter) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	if a == nil || a.llmClient == nil || a.defaultModelSvc == nil {
		return nil, fmt.Errorf("system VLM adapter is not initialized")
	}
	if req.SourceType != contracts.ParseSourceTypeBytes {
		return nil, fmt.Errorf("system VLM adapter requires byte input for source type %q", req.SourceType)
	}
	if strings.TrimSpace(req.FileName) == "" {
		return nil, fmt.Errorf("system VLM adapter requires file_name")
	}
	if len(req.Data) == 0 {
		return nil, fmt.Errorf("system VLM adapter requires non-empty data")
	}

	organizationID := metadataString(req.Metadata, "organization_id")
	if organizationID == "" {
		return nil, fmt.Errorf("organization_id is required for system VLM parsing")
	}

	resolved, err := a.defaultModelSvc.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseVision, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve default vision model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return nil, fmt.Errorf("default vision model is not configured")
	}

	vlmClient := extractvlm.NewWithChatCompletion(resolved.Model, a.fallbackModelName, a.chatCompletionFunc(organizationID, resolved.Provider))
	result, err := vlmClient.ParseBytes(ctx, req.FileName, req.Data, hyperparsesdk.ParseOptionsForRequest(req))
	if err != nil {
		return nil, err
	}

	artifact := hyperparsesdk.MapDocumentResult(req, extractcommon.EngineVLM, result)
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]any{}
	}
	artifact.Metadata["system_vlm_provider"] = strings.TrimSpace(resolved.Provider)
	artifact.Metadata["system_vlm_model"] = strings.TrimSpace(resolved.Model)
	artifact.Metadata["system_vlm_source"] = strings.TrimSpace(resolved.Source)
	return artifact, nil
}

func (a *Adapter) Health(_ context.Context) (contracts.AdapterHealth, error) {
	available := a != nil && a.llmClient != nil && a.defaultModelSvc != nil
	return contracts.AdapterHealth{
		Name:      adapterName,
		Available: available,
		Details: map[string]any{
			"system_default_vision_model": true,
			"requires_organization_id":    true,
			"uses_env_base_url":           false,
		},
	}, nil
}

func (a *Adapter) chatCompletionFunc(organizationID, provider string) extractvlm.ChatCompletionFunc {
	return func(ctx context.Context, req extractvlm.ChatCompletionRequest) (*extractvlm.ChatCompletionResponse, error) {
		maxTokens := req.MaxTokens
		temperature := a.defaultTemperature
		chatReq := &llmadapter.ChatRequest{
			Provider:    strings.TrimSpace(provider),
			Model:       strings.TrimSpace(req.Model),
			Messages:    []llmadapter.Message{{Role: "user", Content: cloneUserContent(req.UserContent)}},
			MaxTokens:   &maxTokens,
			Temperature: &temperature,
		}
		if strings.EqualFold(req.ResponseFormat, "json_object") {
			chatReq.ResponseFormat = &llmadapter.ResponseFormat{Type: "json_object"}
		}

		resp, err := a.llmClient.Chat(ctx, organizationID, chatReq)
		if err != nil {
			return nil, fmt.Errorf("system vision model request failed: %w", err)
		}
		if resp == nil || len(resp.Choices) == 0 {
			return nil, fmt.Errorf("system vision model returned no choices")
		}
		choice := resp.Choices[0]
		return &extractvlm.ChatCompletionResponse{
			Content:      chatContentText(choice.Message.Content),
			Model:        firstNonEmptyString(resp.Model, req.Model),
			FinishReason: choice.FinishReason,
			PromptTokens: promptTokens(resp),
		}, nil
	}
}

func cloneUserContent(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(input))
	for _, item := range input {
		if len(item) == 0 {
			continue
		}
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = value
		}
		out = append(out, cloned)
	}
	return out
}

func chatContentText(content any) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []llmadapter.MessageContentPart:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case []map[string]any:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			partType := strings.TrimSpace(fmt.Sprint(part["type"]))
			if partType != "text" && partType != "output_text" && partType != "input_text" {
				continue
			}
			if text := strings.TrimSpace(fmt.Sprint(part["text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func promptTokens(resp *llmadapter.ChatResponse) int {
	if resp == nil || resp.Usage == nil {
		return 0
	}
	return resp.Usage.PromptTokens
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var _ VisionChatClient = llmclient.LLMClient(nil)
