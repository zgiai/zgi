package hyperparsesdk

import (
	"context"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type DefaultChatFigureSummaryLocalizer struct {
	llmClient       llmclient.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
}

func NewDefaultChatFigureSummaryLocalizer(llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) *DefaultChatFigureSummaryLocalizer {
	return &DefaultChatFigureSummaryLocalizer{
		llmClient:       llmClient,
		defaultModelSvc: defaultModelSvc,
	}
}

func (l *DefaultChatFigureSummaryLocalizer) LocalizeReductoFigureSummary(ctx context.Context, organizationID, text string) (string, error) {
	if l == nil || l.llmClient == nil || l.defaultModelSvc == nil {
		return "", fmt.Errorf("figure summary localizer is not configured")
	}
	organizationID = strings.TrimSpace(organizationID)
	source := strings.TrimSpace(text)
	if organizationID == "" || source == "" {
		return "", fmt.Errorf("organization id and summary text are required")
	}

	resolved, err := l.defaultModelSvc.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseTextChat, nil, nil)
	if err != nil {
		return "", fmt.Errorf("resolve default chat model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", fmt.Errorf("default chat model is not configured")
	}

	temperature := 0.1
	maxTokens := 220
	req := &llmadapter.ChatRequest{
		Provider: strings.TrimSpace(resolved.Provider),
		Model:    strings.TrimSpace(resolved.Model),
		Messages: []llmadapter.Message{
			{
				Role:    "system",
				Content: "你是文档入库前的图片摘要本地化助手。把英文图片/图表摘要改写成简洁、准确的中文。不要补充原文没有的信息，不要解释任务，只输出中文摘要。",
			},
			{
				Role:    "user",
				Content: source,
			},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	}

	resp, err := l.llmClient.Chat(ctx, organizationID, req)
	if err != nil {
		return "", fmt.Errorf("localize figure summary: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("localize figure summary returned no choices")
	}
	output := strings.TrimSpace(chatContentText(resp.Choices[0].Message.Content))
	if output == "" {
		return "", fmt.Errorf("localize figure summary returned empty content")
	}
	return output, nil
}

func (l *DefaultChatFigureSummaryLocalizer) SummarizeMineruFigureImage(ctx context.Context, organizationID, imageURL string) (string, error) {
	if l == nil || l.llmClient == nil || l.defaultModelSvc == nil {
		return "", fmt.Errorf("figure summary localizer is not configured")
	}
	organizationID = strings.TrimSpace(organizationID)
	imageURL = strings.TrimSpace(imageURL)
	if organizationID == "" || imageURL == "" {
		return "", fmt.Errorf("organization id and image url are required")
	}

	resolved, err := l.defaultModelSvc.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseVision, nil, nil)
	if err != nil {
		return "", fmt.Errorf("resolve default vision model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", fmt.Errorf("default vision model is not configured")
	}

	temperature := 0.1
	maxTokens := 180
	req := &llmadapter.ChatRequest{
		Provider: strings.TrimSpace(resolved.Provider),
		Model:    strings.TrimSpace(resolved.Model),
		Messages: []llmadapter.Message{
			{
				Role: "user",
				Content: []llmadapter.MessageContentPart{
					{
						Type: "text",
						Text: "请用中文简要概括这张图片或图表表达的核心信息。只描述图中可见内容，不要补充没有依据的信息，不要输出标题或解释，控制在80字以内。",
					},
					{
						Type:     "image_url",
						ImageURL: &llmadapter.ImageURL{URL: imageURL, Detail: "auto"},
					},
				},
			},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	}

	resp, err := l.llmClient.Chat(ctx, organizationID, req)
	if err != nil {
		return "", fmt.Errorf("summarize mineru figure image: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("summarize mineru figure image returned no choices")
	}
	output := strings.TrimSpace(chatContentText(resp.Choices[0].Message.Content))
	if output == "" {
		return "", fmt.Errorf("summarize mineru figure image returned empty content")
	}
	return output, nil
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
