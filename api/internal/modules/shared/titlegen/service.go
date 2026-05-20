package titlegen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/prompt"
)

const (
	SourceModel    = "model"
	SourceFallback = "fallback"

	defaultFallbackTitle = "New chat"
	maxTitleRunes        = 60
)

type Service interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
}

type GenerateRequest struct {
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	WorkspaceID    *uuid.UUID
	AppID          string
	AppType        string
	SessionID      string
	ConversationID string
	Messages       []Message
	FallbackTitle  string
}

type Message struct {
	Role    string
	Content string
}

type GenerateResult struct {
	Title  string
	Source string
}

type service struct {
	llmClient       llmclient.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
}

func NewService(llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) Service {
	return &service{llmClient: llmClient, defaultModelSvc: defaultModelSvc}
}

func (s *service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	fallback := fallbackTitle(req.FallbackTitle)
	if s == nil || s.llmClient == nil || s.defaultModelSvc == nil {
		return fallbackResult(fallback), nil
	}
	if req.OrganizationID == uuid.Nil || req.AccountID == uuid.Nil {
		return fallbackResult(fallback), nil
	}

	rendered, err := renderTitlePrompt(req.Messages)
	if err != nil {
		return fallbackResult(fallback), nil
	}
	resolved, err := s.defaultModelSvc.ResolveUseCase(ctx, req.OrganizationID.String(), llmmodelmodel.UseCaseTextChat, nil, nil)
	if err != nil || resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return fallbackResult(fallback), nil
	}

	resp, err := s.llmClient.AppChat(ctx, appContext(req), titleChatRequest(resolved, rendered))
	if err != nil {
		return fallbackResult(fallback), nil
	}
	title, err := parseTitleResponse(resp)
	if err != nil {
		return fallbackResult(fallback), nil
	}
	return &GenerateResult{Title: title, Source: SourceModel}, nil
}

func renderTitlePrompt(messages []Message) (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.CommonConversationTitle)
	if err != nil {
		return "", err
	}
	return tmpl.Render(struct {
		MessagesLast string
	}{
		MessagesLast: formatMessages(messages),
	})
}

func formatMessages(messages []Message) string {
	var builder strings.Builder
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(strings.ToUpper(role[:1]))
		if len(role) > 1 {
			builder.WriteString(strings.ToLower(role[1:]))
		}
		builder.WriteString(": ")
		builder.WriteString(content)
	}
	return builder.String()
}

func appContext(req GenerateRequest) *llmclient.AppContext {
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		appID = req.ConversationID
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = req.ConversationID
	}
	appCtx := &llmclient.AppContext{
		OrganizationID:     req.OrganizationID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              appID,
		AppType:            strings.TrimSpace(req.AppType),
		AccountID:          req.AccountID.String(),
		SessionID:          sessionID,
		ConversationID:     strings.TrimSpace(req.ConversationID),
	}
	if appCtx.AppType == "" {
		appCtx.AppType = "unknown"
	}
	if req.WorkspaceID != nil && *req.WorkspaceID != uuid.Nil {
		appCtx.WorkspaceID = req.WorkspaceID.String()
	}
	return appCtx
}

func titleChatRequest(resolved *llmdefaultservice.ResolvedModel, promptText string) *adapter.ChatRequest {
	temperature := 0.2
	maxTokens := 64
	return &adapter.ChatRequest{
		Provider:    strings.TrimSpace(resolved.Provider),
		Model:       strings.TrimSpace(resolved.Model),
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Messages: []adapter.Message{
			{Role: "user", Content: promptText},
		},
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}
}

func parseTitleResponse(resp *adapter.ChatResponse) (string, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty title response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return "", fmt.Errorf("title response content is not text")
	}
	var payload struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &payload); err != nil {
		return "", err
	}
	title := sanitizeTitle(payload.Title)
	if !validTitle(title) {
		return "", fmt.Errorf("invalid title")
	}
	return title, nil
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end >= start {
		return content[start : end+1]
	}
	return content
}

func sanitizeTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'`“”‘’")
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "。.!！?？,，;；:：")
	return strings.TrimSpace(title)
}

func validTitle(title string) bool {
	if title == "" || runeCount(title) > maxTitleRunes {
		return false
	}
	for _, r := range title {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}

func fallbackTitle(title string) string {
	title = sanitizeTitle(title)
	if title == "" {
		return defaultFallbackTitle
	}
	return title
}

func fallbackResult(title string) *GenerateResult {
	return &GenerateResult{Title: title, Source: SourceFallback}
}

func runeCount(value string) int {
	return len([]rune(value))
}
