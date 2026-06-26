package titlegen

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var titleCodeFencePattern = regexp.MustCompile("(?s)^```(?:json)?\\s*(.*?)\\s*```$")

const (
	SourceModel    = "model"
	SourceFallback = "fallback"

	defaultFallbackTitle = "New chat"
	maxTitleRunes        = 60
	titleAttemptTimeout  = 4 * time.Second
)

type Service interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
}

type GenerateRequest struct {
	OrganizationID    uuid.UUID
	AccountID         uuid.UUID
	WorkspaceID       *uuid.UUID
	AppID             string
	AppType           string
	SessionID         string
	ConversationID    string
	Messages          []Message
	FallbackTitle     string
	PreferredProvider string
	PreferredModel    string
	PreferredUseCase  string
}

type Message struct {
	Role    string
	Content string
}

type GenerateResult struct {
	Title  string
	Source string
}

type RouteModelProvider interface {
	ListTitleModels(ctx context.Context, organizationID uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error)
}

type service struct {
	llmClient          llmclient.LLMClient
	defaultModelSvc    llmdefaultservice.DefaultModelService
	routeModelProvider RouteModelProvider
}

var successfulTitleModelCache sync.Map

func NewService(llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) Service {
	return NewServiceWithRouteModelProvider(llmClient, defaultModelSvc, nil)
}

func NewServiceWithRouteModelProvider(llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService, routeModelProvider RouteModelProvider) Service {
	return &service{llmClient: llmClient, defaultModelSvc: defaultModelSvc, routeModelProvider: routeModelProvider}
}

type tenantRouteModelProvider struct {
	routeRepo channelrepo.TenantRouteRepository
}

func NewTenantRouteModelProvider(routeRepo channelrepo.TenantRouteRepository) RouteModelProvider {
	if routeRepo == nil {
		return nil
	}
	return &tenantRouteModelProvider{routeRepo: routeRepo}
}

func (p *tenantRouteModelProvider) ListTitleModels(ctx context.Context, organizationID uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	if p == nil || p.routeRepo == nil || organizationID == uuid.Nil {
		return nil, nil
	}
	routes, err := p.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	type routeCandidate struct {
		routeIndex int
		modelIndex int
		model      *llmdefaultservice.ResolvedModel
	}
	candidates := make([]routeCandidate, 0)
	seen := map[string]bool{}
	for routeIndex, route := range routes {
		if route == nil {
			continue
		}
		provider := strings.TrimSpace(route.ChannelProvider)
		for modelIndex, modelName := range route.GetEffectiveModels() {
			modelName = strings.TrimSpace(modelName)
			if !titleRouteModelAllowed(modelName) {
				continue
			}
			key := candidateKey(provider, modelName)
			if key == "::" || seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, routeCandidate{
				routeIndex: routeIndex,
				modelIndex: modelIndex,
				model: &llmdefaultservice.ResolvedModel{
					UseCase:  string(llmmodelmodel.UseCaseTextChat),
					Provider: provider,
					Model:    modelName,
					Source:   llmdefaultservice.SourceAuto,
				},
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftScore := titleRouteModelScore(candidates[i].model.Model)
		rightScore := titleRouteModelScore(candidates[j].model.Model)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if candidates[i].routeIndex != candidates[j].routeIndex {
			return candidates[i].routeIndex < candidates[j].routeIndex
		}
		return candidates[i].modelIndex < candidates[j].modelIndex
	})
	models := make([]*llmdefaultservice.ResolvedModel, 0, len(candidates))
	for _, candidate := range candidates {
		models = append(models, candidate.model)
	}
	return models, nil
}

func (s *service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	fallback := fallbackTitle(req.FallbackTitle)
	if s == nil || s.llmClient == nil {
		logger.DebugContext(ctx, "conversation title generation skipped because dependencies are unavailable")
		return fallbackResult(fallback), nil
	}
	if req.OrganizationID == uuid.Nil || req.AccountID == uuid.Nil {
		logger.DebugContext(ctx, "conversation title generation skipped because request scope is incomplete")
		return fallbackResult(fallback), nil
	}

	rendered, err := renderTitlePrompt(req.Messages)
	if err != nil {
		logger.DebugContext(ctx, "conversation title prompt render failed", err)
		return fallbackResult(fallback), nil
	}

	candidates := s.titleModelCandidates(ctx, req)
	if len(candidates) == 0 {
		logger.DebugContext(ctx, "conversation title generation skipped because no candidate model resolved", "organization_id", req.OrganizationID.String())
		return fallbackResult(fallback), nil
	}

	var rawTitleFallback string
	for _, candidate := range candidates {
		if candidate == nil || strings.TrimSpace(candidate.Model) == "" || isReasoningTitleModel(candidate) {
			if candidate != nil {
				logger.DebugContext(ctx, "conversation title candidate skipped", "provider", candidate.Provider, "model", candidate.Model, "use_case", candidate.UseCase)
			}
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, titleAttemptTimeout)
		resp, err := s.llmClient.AppChat(attemptCtx, appContext(req), titleChatRequest(candidate, rendered))
		cancel()
		if err != nil {
			logger.DebugContext(ctx, "conversation title model attempt failed", "provider", candidate.Provider, "model", candidate.Model, err)
			continue
		}
		title, err := parseTitleResponse(resp)
		if err != nil {
			if rawTitleFallback == "" {
				rawTitleFallback = parseRawTitleResponse(resp)
			}
			logger.DebugContext(ctx, "conversation title model response rejected", "provider", candidate.Provider, "model", candidate.Model, err)
			continue
		}
		s.rememberSuccessfulTitleModel(req, candidate)
		return &GenerateResult{Title: title, Source: SourceModel}, nil
	}

	if rawTitleFallback != "" {
		logger.DebugContext(ctx, "conversation title generation accepted non-json model response", "organization_id", req.OrganizationID.String(), "candidate_count", len(candidates))
		return &GenerateResult{Title: rawTitleFallback, Source: SourceModel}, nil
	}

	logger.DebugContext(ctx, "conversation title generation fell back after all candidates failed", "organization_id", req.OrganizationID.String(), "candidate_count", len(candidates))
	return fallbackResult(fallback), nil
}

func (s *service) titleModelCandidates(ctx context.Context, req GenerateRequest) []*llmdefaultservice.ResolvedModel {
	candidates := make([]*llmdefaultservice.ResolvedModel, 0, 4)
	seen := map[string]bool{}
	reasoningKeys := map[string]bool{}
	add := func(model *llmdefaultservice.ResolvedModel) {
		if model == nil {
			return
		}
		if !titleCandidateUseCase(model) {
			return
		}
		if !titleRouteModelAllowed(model.Model) {
			return
		}
		key := candidateKey(model.Provider, model.Model)
		if key == "::" || seen[key] || reasoningKeys[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, model)
	}

	var resolvedDefaults []*llmdefaultservice.ResolvedModel
	if s.defaultModelSvc != nil {
		var err error
		resolvedDefaults, err = s.defaultModelSvc.ListResolved(ctx, req.OrganizationID)
		if err == nil {
			for _, resolved := range resolvedDefaults {
				key := candidateKey(resolved.Provider, resolved.Model)
				if key == "::" {
					continue
				}
				if isReasoningTitleModel(resolved) {
					reasoningKeys[key] = true
				}
			}
		}
		if resolved, err := s.defaultModelSvc.ResolveUseCase(ctx, req.OrganizationID.String(), llmmodelmodel.UseCaseTextChat, nil, nil); err == nil {
			add(resolved)
		}
	}
	if cached := s.cachedSuccessfulTitleModel(req); cached != nil {
		add(cached)
	}
	for _, resolved := range resolvedDefaults {
		add(resolved)
	}
	if preferred := preferredTitleModel(req); preferred != nil {
		add(preferred)
	}
	if s.routeModelProvider != nil {
		routeModels, err := s.routeModelProvider.ListTitleModels(ctx, req.OrganizationID)
		if err != nil {
			logger.DebugContext(ctx, "conversation title route model candidates failed", "organization_id", req.OrganizationID.String(), err)
		}
		for _, routeModel := range routeModels {
			add(routeModel)
		}
	}
	return candidates
}

func preferredTitleModel(req GenerateRequest) *llmdefaultservice.ResolvedModel {
	provider := strings.TrimSpace(req.PreferredProvider)
	model := strings.TrimSpace(req.PreferredModel)
	if model == "" {
		return nil
	}
	useCase := strings.TrimSpace(req.PreferredUseCase)
	if useCase == "" {
		useCase = string(llmmodelmodel.UseCaseTextChat)
	}
	candidate := &llmdefaultservice.ResolvedModel{UseCase: useCase}
	if !titleCandidateUseCase(candidate) {
		return nil
	}
	return &llmdefaultservice.ResolvedModel{
		UseCase:  useCase,
		Provider: provider,
		Model:    model,
		Source:   llmdefaultservice.SourceAuto,
	}
}

func (s *service) rememberSuccessfulTitleModel(req GenerateRequest, model *llmdefaultservice.ResolvedModel) {
	if s == nil || model == nil || strings.TrimSpace(model.Model) == "" {
		return
	}
	successfulTitleModelCache.Store(titleModelCacheKey(req), &llmdefaultservice.ResolvedModel{
		UseCase:  string(llmmodelmodel.UseCaseTextChat),
		Provider: strings.TrimSpace(model.Provider),
		Model:    strings.TrimSpace(model.Model),
		Source:   model.Source,
	})
}

func (s *service) cachedSuccessfulTitleModel(req GenerateRequest) *llmdefaultservice.ResolvedModel {
	if s == nil {
		return nil
	}
	value, ok := successfulTitleModelCache.Load(titleModelCacheKey(req))
	if !ok {
		return nil
	}
	model, ok := value.(*llmdefaultservice.ResolvedModel)
	if !ok {
		return nil
	}
	return model
}

func titleModelCacheKey(req GenerateRequest) string {
	return req.OrganizationID.String() + ":" + req.AccountID.String()
}

func titleCandidateUseCase(model *llmdefaultservice.ResolvedModel) bool {
	if model == nil {
		return false
	}
	switch strings.TrimSpace(model.UseCase) {
	case string(llmmodelmodel.UseCaseTextChat), string(llmmodelmodel.UseCaseVision), string(llmmodelmodel.UseCaseFuncCalling):
		return true
	default:
		return false
	}
}

func isReasoningTitleModel(model *llmdefaultservice.ResolvedModel) bool {
	if model == nil {
		return false
	}
	if strings.TrimSpace(model.UseCase) == string(llmmodelmodel.UseCaseReasoning) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(model.Model))
	reasoningHints := []string{"reasoning", "thinking", "deepseek-r1", "-r1", "r1-", "o1", "o3", "o4"}
	for _, hint := range reasoningHints {
		if strings.Contains(name, hint) {
			return true
		}
	}
	return false
}

func titleRouteModelAllowed(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "*" {
		return false
	}
	lower := strings.ToLower(modelName)
	blockedHints := []string{
		"image", "img", "flux", "stable-diffusion", "sdxl", "dall-e", "midjourney",
		"embedding", "embed", "rerank", "tts", "speech", "audio", "whisper",
		"moderation", "vision", "vlm", "ocr",
		"reasoning", "thinking", "deepseek-r1", "-r1", "r1-", "o1", "o3", "o4",
	}
	for _, hint := range blockedHints {
		if strings.Contains(lower, hint) {
			return false
		}
	}
	return true
}

func titleRouteModelScore(modelName string) int {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	score := 0
	for _, hint := range []string{"chat", "qwen", "gpt", "claude", "gemini", "deepseek", "kimi", "doubao", "ernie", "glm"} {
		if strings.Contains(lower, hint) {
			score += 10
		}
	}
	for _, hint := range []string{"mini", "flash", "turbo", "plus", "max", "lite", "speed"} {
		if strings.Contains(lower, hint) {
			score += 2
		}
	}
	return score
}

func candidateKey(provider string, model string) string {
	return strings.TrimSpace(provider) + "::" + strings.TrimSpace(model)
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

func parseRawTitleResponse(resp *adapter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return ""
	}
	title := sanitizeRawTitle(content)
	if !validTitle(title) {
		return ""
	}
	return title
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

func sanitizeRawTitle(content string) string {
	content = strings.TrimSpace(content)
	if matches := titleCodeFencePattern.FindStringSubmatch(content); len(matches) == 2 {
		content = strings.TrimSpace(matches[1])
	}
	if strings.Contains(content, "{") || strings.Contains(content, "}") {
		return ""
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"title:", "Title:", "标题：", "标题:"} {
			line = strings.TrimPrefix(line, prefix)
		}
		return sanitizeTitle(line)
	}
	return sanitizeTitle(content)
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
		if unicode.IsControl(r) || r == '\n' || r == '\r' {
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
