package agents

import (
	"context"
	"errors"
	"fmt"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/suggestedquestions"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"strings"
)

func (s *agentsService) GenerateAgentSuggestedQuestions(ctx context.Context, agentID, accountID string, req *dto.GenerateAgentSuggestedQuestionsRequest) (*dto.GenerateSuggestedQuestionsResponse, error) {
	if req == nil {
		req = &dto.GenerateAgentSuggestedQuestionsRequest{}
	}
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	if s.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}

	workspaceID := ag.TenantID.String()
	organizationID := workspaceID
	if s.enterpriseService != nil {
		if org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID); err == nil && org != nil && org.ID != "" {
			organizationID = org.ID
		}
	}

	cfgResp := agentConfigResponse(ag.ID.String(), cfg)
	provider, model, err := s.resolveAgentSuggestedQuestionsModel(ctx, organizationID, firstNonEmpty(req.Provider, cfgResp.ModelProvider), firstNonEmpty(req.Model, cfgResp.Model))
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.TrimSpace(req.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = cfgResp.SystemPrompt
	}
	homeTitle := normalizeAgentHomeTitle(firstNonEmpty(req.HomeTitle, cfgResp.HomeTitle))

	capabilities := make([]suggestedquestions.CapabilitySummary, 0, len(req.Skills)+len(req.KnowledgeRefs))
	for _, skill := range req.Skills {
		title := strings.TrimSpace(firstNonEmpty(skill.Name, skill.ID))
		if title == "" {
			continue
		}
		capabilities = append(capabilities, suggestedquestions.CapabilitySummary{
			Type:       "skill",
			Title:      cleanAgentContextText(title, 80),
			Dependency: cleanAgentContextText(skill.Description, 160),
		})
	}
	for _, ref := range req.KnowledgeRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		capabilities = append(capabilities, suggestedquestions.CapabilitySummary{
			Type:  "knowledge_ref",
			Title: cleanAgentContextText(ref, 120),
		})
	}
	if len(capabilities) > 12 {
		capabilities = capabilities[:12]
	}

	generator := suggestedquestions.NewGenerator(s.llmClient)
	result, err := generator.Generate(ctx, suggestedquestions.GenerateRequest{
		Context: suggestedquestions.WorkflowContext{
			Locale:            req.Locale,
			AgentName:         ag.Name,
			AgentDescription:  cleanAgentContextText(ag.Description, 300),
			WorkflowType:      "AGENT",
			OpeningStatement:  homeTitle,
			ExistingQuestions: normalizeSuggestedQuestions(req.ExistingQuestions),
			LLMPrompts: []suggestedquestions.PromptSummary{{
				NodeTitle: "System prompt",
				Role:      "system",
				Text:      cleanAgentContextText(systemPrompt, 1200),
				Model:     model,
			}},
			Capabilities: capabilities,
		},
		Count:          req.Count,
		Provider:       provider,
		Model:          model,
		AgentID:        agentID,
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		AppType:        "agent",
	})
	if err != nil {
		return nil, err
	}

	questions := make([]dto.SuggestedQuestionCandidate, 0, len(result.Questions))
	for _, question := range result.Questions {
		questions = append(questions, dto.SuggestedQuestionCandidate{
			Text:   question.Text,
			Reason: question.Reason,
		})
	}

	return &dto.GenerateSuggestedQuestionsResponse{
		Questions: questions,
		Warnings:  result.Warnings,
		Provider:  result.Provider,
		Model:     result.Model,
	}, nil
}

func (s *agentsService) resolveAgentSuggestedQuestionsModel(ctx context.Context, organizationID, explicitProvider, explicitModel string) (string, string, error) {
	provider := strings.TrimSpace(explicitProvider)
	model := strings.TrimSpace(explicitModel)
	if model != "" {
		if fallbackProvider, fallbackModel, err := s.resolveDefaultLLMModel(ctx, organizationID, "suggested questions"); err == nil && fallbackModel != "" && isReasoningSuggestedQuestionsModel(model) && !isReasoningSuggestedQuestionsModel(fallbackModel) {
			return fallbackProvider, fallbackModel, nil
		}
		return provider, model, nil
	}
	return s.resolveDefaultLLMModel(ctx, organizationID, "suggested questions")
}

func (s *agentsService) resolveDefaultLLMModel(ctx context.Context, organizationID, scope string) (string, string, error) {
	if s.defaultModelResolver == nil || strings.TrimSpace(organizationID) == "" {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}
	resolved, err := s.defaultModelResolver.ResolveModelType(ctx, organizationID, nil, nil, sharedmodel.ModelTypeLLM)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve default LLM model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}
	return strings.TrimSpace(resolved.Provider), strings.TrimSpace(resolved.Model), nil
}

func isReasoningSuggestedQuestionsModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	reasoningMarkers := []string{
		"reasoning",
		"reasoner",
		"thinking",
		"think",
		"deepseek-r1",
		"deepseek-reasoner",
		"qwq",
		"kimi-k2-thinking",
	}
	for _, marker := range reasoningMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isAgentSuggestedQuestionsConfigurationError(err error) bool {
	return errors.Is(err, suggestedquestions.ErrModelNotConfigured)
}

func isAgentSuggestedQuestionsModelOutputError(err error) bool {
	return errors.Is(err, suggestedquestions.ErrModelOutputInvalid)
}

func normalizeSuggestedQuestions(input []string) []string {
	out := make([]string, 0, len(input))
	for _, raw := range input {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if len([]rune(item)) > 200 {
			runes := []rune(item)
			item = string(runes[:200])
		}
		out = append(out, item)
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func cleanAgentContextText(input string, maxRunes int) string {
	text := strings.TrimSpace(input)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return text
}
