package draftgen

import (
	"context"
	"fmt"
	"strings"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const taskDraftAppType = "automation_task_draft"

type Generator struct {
	llmClient llmclient.LLMClient
}

func NewGenerator(llmClient llmclient.LLMClient) *Generator {
	return &Generator{llmClient: llmClient}
}

func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if g == nil || g.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}

	provider := cleanShortText(req.Provider)
	model := cleanShortText(req.Model)
	if model == "" {
		return nil, ErrModelNotConfigured
	}

	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return nil, err
	}

	userPrompt, err := buildUserPrompt(req)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	temperature := 0.2
	maxTokens := 1600
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Messages: []adapter.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}

	resp, err := g.llmClient.AppChat(timeoutCtx, buildAppContext(req), chatReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%w: empty response", ErrModelOutputInvalid)
	}

	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("%w: response content is not text", ErrModelOutputInvalid)
	}

	result, err := ParseDraft(content, req.Timezone, req.Locale)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModelOutputInvalid, err)
	}
	result.Provider = provider
	result.Model = model
	return result, nil
}

func buildAppContext(req GenerateRequest) *llmclient.AppContext {
	appID := cleanShortText(req.WorkspaceID)
	billingSubjectType := llmclient.BillingSubjectTypeWorkspace
	if appID == "" {
		appID = cleanShortText(req.OrganizationID)
		billingSubjectType = llmclient.BillingSubjectTypeOrganization
	}

	return &llmclient.AppContext{
		OrganizationID:     cleanShortText(req.OrganizationID),
		WorkspaceID:        cleanShortText(req.WorkspaceID),
		BillingSubjectType: billingSubjectType,
		AppID:              appID,
		AppType:            taskDraftAppType,
		AccountID:          cleanShortText(req.AccountID),
	}
}
