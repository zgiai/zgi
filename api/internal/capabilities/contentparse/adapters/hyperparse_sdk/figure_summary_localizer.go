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
	maxTokens := 600
	req := &llmadapter.ChatRequest{
		Provider: strings.TrimSpace(resolved.Provider),
		Model:    strings.TrimSpace(resolved.Model),
		Messages: []llmadapter.Message{
			{
				Role: "user",
				Content: []llmadapter.MessageContentPart{
					{
						Type: "text",
						Text: `Analyze the image carefully and produce a faithful Chinese summary for document retrieval.

For a simple image, describe its main subject and key visible information concisely. For an information-dense image, preserve the important details and relationships instead of giving only a high-level description. In particular:
- For flowcharts, describe the start and end points, the sequence of steps, branch conditions, directions, dependencies, loops, and exception paths when visible.
- For diagrams, explain the main components and how they are connected or interact.
- For charts, state the axes or compared dimensions, major values or categories, trends, differences, and conclusions supported by the image.
- For tables, screenshots, and formulas, capture the key text, fields, structure, operations, and results needed to understand their meaning.

Use only information visible in the image. Do not invent, infer unsupported details, or omit important content merely to satisfy the length target. Aim for no more than 200 Chinese characters, but exceed this limit when necessary to describe complex content clearly and completely. Output only the Chinese summary, without a title, preface, explanation, or Markdown formatting.`,
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
