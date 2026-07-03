package suggestedquestions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Generator produces suggested questions using the platform LLM client.
type Generator struct {
	llmClient client.LLMClient
}

const (
	defaultQuestionCount = 3
	maxQuestionCount     = 6
)

var (
	ErrModelNotConfigured = errors.New("suggested question generation requires a configured default LLM model")
	ErrModelOutputInvalid = errors.New("suggested question model output is not usable")
)

// NewGenerator creates a generator backed by an LLM client.
func NewGenerator(llmClient client.LLMClient) *Generator {
	return &Generator{llmClient: llmClient}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// Generate returns editable suggested questions for the supplied workflow context.
func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if g == nil || g.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}

	count := req.Count
	if count <= 0 {
		count = defaultQuestionCount
	}
	if count > maxQuestionCount {
		count = maxQuestionCount
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

	userPrompt, err := buildUserPrompt(req.Context, count)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	temperature := 0.25
	maxTokens := 800
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

	appCtx := &client.AppContext{
		OrganizationID:     req.OrganizationID,
		WorkspaceID:        req.WorkspaceID,
		BillingSubjectType: client.BillingSubjectTypeWorkspace,
		AppID:              req.AgentID,
		AppType:            firstNonEmpty(req.AppType, "workflow"),
		AccountID:          req.AccountID,
	}

	resp, err := g.llmClient.AppChat(timeoutCtx, appCtx, chatReq)
	if err != nil && isResponseFormatUnsupportedError(err) {
		chatReq.ResponseFormat = nil
		resp, err = g.llmClient.AppChat(timeoutCtx, appCtx, chatReq)
	}
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

	questions, warnings, err := ParseQuestions(content, count, req.Context.ExistingQuestions)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModelOutputInvalid, err)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("%w: response did not contain suggested questions", ErrModelOutputInvalid)
	}

	return &GenerateResult{
		Questions: questions,
		Warnings:  warnings,
		Provider:  provider,
		Model:     model,
	}, nil
}

func isResponseFormatUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "response_format") &&
		(strings.Contains(message, "unsupported") ||
			strings.Contains(message, "not support") ||
			strings.Contains(message, "not_supported") ||
			strings.Contains(message, "invalid parameter") ||
			strings.Contains(message, "invalid_param") ||
			strings.Contains(message, "不支持"))
}
