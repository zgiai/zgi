package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/app/workflow/suggestedquestions"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	sharedmodel "github.com/zgiai/ginext/internal/modules/shared/model"
)

// GenerateDraftWorkflowSuggestedQuestions generates editable suggested
// questions from the current workflow draft context. The request may provide
// unsaved graph/features from the editor; otherwise the saved draft is used.
func (s *WorkflowService) GenerateDraftWorkflowSuggestedQuestions(ctx context.Context, workspaceID, agentID string, req *dto.GenerateSuggestedQuestionsRequest, accountID string) (*dto.GenerateSuggestedQuestionsResponse, error) {
	if req == nil {
		req = &dto.GenerateSuggestedQuestionsRequest{}
	}

	workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		return nil, err
	}

	graph := workflow.GetGraphDict()
	if len(req.Graph) > 0 {
		graph = req.Graph
	}

	features := workflow.GetFeaturesDict()
	if len(req.Features) > 0 {
		features = req.Features
	}

	agentName := ""
	agentDescription := ""
	if s.agentsRepo != nil {
		if agent, err := s.agentsRepo.GetByID(ctx, agentID); err == nil && agent != nil {
			agentName = agent.Name
			agentDescription = agent.Description
		}
	}

	if s == nil || s.executor == nil {
		return nil, fmt.Errorf("workflow executor is not configured")
	}

	llm, ok := s.executor.GetLLMClient().(llmclient.LLMClient)
	if !ok || llm == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}

	organizationID := s.getOrganizationIDByWorkspace(ctx, workspaceID)
	provider, model, err := s.resolveSuggestedQuestionsModel(ctx, organizationID, req.Provider, req.Model)
	if err != nil {
		return nil, err
	}

	generator := suggestedquestions.NewGenerator(llm)
	generationContext := suggestedquestions.BuildContext(suggestedquestions.BuildContextInput{
		Locale:            req.Locale,
		AgentName:         agentName,
		AgentDescription:  agentDescription,
		WorkflowType:      string(workflow.Type),
		Graph:             graph,
		Features:          features,
		ExistingQuestions: req.ExistingQuestions,
	})

	result, err := generator.Generate(ctx, suggestedquestions.GenerateRequest{
		Context:        generationContext,
		Count:          req.Count,
		Provider:       provider,
		Model:          model,
		AgentID:        agentID,
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AccountID:      accountID,
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

func (s *WorkflowService) resolveSuggestedQuestionsModel(ctx context.Context, organizationID, explicitProvider, explicitModel string) (string, string, error) {
	provider := strings.TrimSpace(explicitProvider)
	model := strings.TrimSpace(explicitModel)
	if model != "" {
		return provider, model, nil
	}
	if strings.TrimSpace(organizationID) == "" {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}

	resolver := s.suggestedQuestionsDefaultModelResolver()
	if resolver == nil {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}

	resolved, err := resolver.ResolveModelType(ctx, organizationID, nil, nil, sharedmodel.ModelTypeLLM)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve default LLM model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}

	return strings.TrimSpace(resolved.Provider), strings.TrimSpace(resolved.Model), nil
}

func (s *WorkflowService) suggestedQuestionsDefaultModelResolver() llmdefaultservice.DefaultModelResolver {
	if s == nil || s.executor == nil || s.executor.graphFlowService == nil {
		return nil
	}
	return s.executor.graphFlowService.DefaultModelSvc
}

func isSuggestedQuestionsConfigurationError(err error) bool {
	return errors.Is(err, suggestedquestions.ErrModelNotConfigured)
}

func isSuggestedQuestionsModelOutputError(err error) bool {
	return errors.Is(err, suggestedquestions.ErrModelOutputInvalid)
}
