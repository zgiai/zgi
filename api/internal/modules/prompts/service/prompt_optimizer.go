package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	promptOptimizerGoalGeneral      = "general"
	promptOptimizerGoalReliable     = "reliable"
	promptOptimizerGoalStructured   = "structured"
	promptOptimizerGoalProfessional = "professional"
	promptOptimizerGoalJSON         = "json"
	promptOptimizerGoalDeep         = "deep"
)

var promptOptimizerVariablePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\{\{#[^{}]+#\}\}`),
	regexp.MustCompile(`\{\{[^{}]+\}\}`),
	regexp.MustCompile(`\$\{[^{}]+\}`),
}

func (s *promptService) Optimize(
	ctx context.Context,
	organizationID,
	accountID,
	workspaceID string,
	req promptdto.PromptOptimizeRequest,
) (*promptdto.PromptOptimizeResponse, error) {
	rawPrompt := strings.TrimSpace(req.RawPrompt)
	if rawPrompt == "" {
		return nil, fmt.Errorf("raw prompt cannot be empty")
	}
	if s == nil || s.llmClient == nil || s.defaultModelSvc == nil {
		return nil, fmt.Errorf("prompt optimizer is unavailable")
	}

	goal := normalizePromptOptimizerGoal(req.Goal)
	preserveVariables := req.PreserveVariables == nil || *req.PreserveVariables
	detectedVariables := detectPromptOptimizerVariables(rawPrompt)

	promptID, err := s.resolveOptimizerPromptID(ctx, organizationID, accountID, req.PromptID)
	if err != nil {
		return nil, err
	}

	resolvedModel, err := s.defaultModelSvc.ResolveUseCase(
		ctx,
		organizationID,
		llmmodelmodel.UseCaseTextChat,
		trimmedStringPtr(req.Provider),
		trimmedStringPtr(req.Model),
	)
	if err != nil {
		return nil, fmt.Errorf("resolve optimizer model: %w", err)
	}
	if resolvedModel == nil || strings.TrimSpace(resolvedModel.Model) == "" {
		return nil, fmt.Errorf("prompt optimizer model is not configured")
	}

	appCtx := &llmclient.AppContext{
		OrganizationID:     organizationID,
		WorkspaceID:        workspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              optimizerAppID(organizationID, workspaceID),
		AppType:            "prompt_optimizer",
		AccountID:          accountID,
		SessionID:          "prompt-optimizer:" + accountID,
		ConversationID:     "prompt-optimizer",
	}

	response, err := s.llmClient.AppChat(
		ctx,
		appCtx,
		buildPromptOptimizerChatRequest(
			resolvedModel,
			goal,
			rawPrompt,
			preserveVariables,
			detectedVariables,
			false,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("optimize prompt with llm: %w", err)
	}

	output, err := parsePromptOptimizerTextResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse optimizer response: %w", err)
	}

	run := &promptmodel.PromptOptimizationRun{
		OrganizationID:    organizationID,
		WorkspaceID:       stringPtr(workspaceID),
		PromptID:          promptID,
		AccountID:         accountID,
		Goal:              goal,
		Provider:          stringPtr(resolvedModel.Provider),
		Model:             stringPtr(resolvedModel.Model),
		PreserveVariables: preserveVariables,
		DetectedVariables: append([]string{}, detectedVariables...),
		RawPrompt:         rawPrompt,
		SafeOutput:        output,
		BalancedOutput:    output,
		AdvancedOutput:    output,
	}
	if err := s.repo.CreateOptimizationRun(ctx, run); err != nil {
		return nil, fmt.Errorf("save prompt optimization run: %w", err)
	}

	return &promptdto.PromptOptimizeResponse{
		Goal:              goal,
		PreserveVariables: preserveVariables,
		DetectedVariables: detectedVariables,
		RunID:             run.ID,
		Output:            output,
		Variants: promptdto.PromptOptimizeVariantsResponse{
			Safe:     output,
			Balanced: output,
			Advanced: output,
		},
	}, nil
}

func (s *promptService) OptimizeStream(
	ctx context.Context,
	organizationID,
	accountID,
	workspaceID string,
	req promptdto.PromptOptimizeRequest,
	onEvent func(PromptOptimizeStreamEvent) error,
) (*promptdto.PromptOptimizeResponse, error) {
	rawPrompt := strings.TrimSpace(req.RawPrompt)
	if rawPrompt == "" {
		return nil, fmt.Errorf("raw prompt cannot be empty")
	}
	if s == nil || s.llmClient == nil || s.defaultModelSvc == nil {
		return nil, fmt.Errorf("prompt optimizer is unavailable")
	}

	goal := normalizePromptOptimizerGoal(req.Goal)
	preserveVariables := req.PreserveVariables == nil || *req.PreserveVariables
	detectedVariables := detectPromptOptimizerVariables(rawPrompt)

	promptID, err := s.resolveOptimizerPromptID(ctx, organizationID, accountID, req.PromptID)
	if err != nil {
		return nil, err
	}

	if onEvent != nil {
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "progress",
			Data: map[string]interface{}{
				"step":  "analyze",
				"index": 0,
			},
		}); err != nil {
			return nil, err
		}
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "progress",
			Data: map[string]interface{}{
				"step":  "variables",
				"index": 1,
			},
		}); err != nil {
			return nil, err
		}
	}

	resolvedModel, err := s.defaultModelSvc.ResolveUseCase(
		ctx,
		organizationID,
		llmmodelmodel.UseCaseTextChat,
		trimmedStringPtr(req.Provider),
		trimmedStringPtr(req.Model),
	)
	if err != nil {
		return nil, fmt.Errorf("resolve optimizer model: %w", err)
	}
	if resolvedModel == nil || strings.TrimSpace(resolvedModel.Model) == "" {
		return nil, fmt.Errorf("prompt optimizer model is not configured")
	}

	if onEvent != nil {
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "meta",
			Data: map[string]interface{}{
				"goal":               goal,
				"preserve_variables": preserveVariables,
				"detected_variables": detectedVariables,
				"provider":           resolvedModel.Provider,
				"model":              resolvedModel.Model,
			},
		}); err != nil {
			return nil, err
		}
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "progress",
			Data: map[string]interface{}{
				"step":  "rewrite",
				"index": 2,
			},
		}); err != nil {
			return nil, err
		}
	}

	appCtx := &llmclient.AppContext{
		OrganizationID:     organizationID,
		WorkspaceID:        workspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              optimizerAppID(organizationID, workspaceID),
		AppType:            "prompt_optimizer",
		AccountID:          accountID,
		SessionID:          "prompt-optimizer:" + accountID,
		ConversationID:     "prompt-optimizer",
	}

	stream, err := s.llmClient.AppChatStream(
		ctx,
		appCtx,
		buildPromptOptimizerChatRequest(
			resolvedModel,
			goal,
			rawPrompt,
			preserveVariables,
			detectedVariables,
			true,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("optimize prompt stream with llm: %w", err)
	}

	var builder strings.Builder
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				finalOutput := strings.TrimSpace(builder.String())
				if finalOutput == "" {
					return nil, fmt.Errorf("optimizer response output is empty")
				}
				if onEvent != nil {
					if err := onEvent(PromptOptimizeStreamEvent{
						Event: "progress",
						Data: map[string]interface{}{
							"step":  "polish",
							"index": 3,
						},
					}); err != nil {
						return nil, err
					}
				}
				run := &promptmodel.PromptOptimizationRun{
					OrganizationID:    organizationID,
					WorkspaceID:       stringPtr(workspaceID),
					PromptID:          promptID,
					AccountID:         accountID,
					Goal:              goal,
					Provider:          stringPtr(resolvedModel.Provider),
					Model:             stringPtr(resolvedModel.Model),
					PreserveVariables: preserveVariables,
					DetectedVariables: append([]string{}, detectedVariables...),
					RawPrompt:         rawPrompt,
					SafeOutput:        finalOutput,
					BalancedOutput:    finalOutput,
					AdvancedOutput:    finalOutput,
				}
				if err := s.repo.CreateOptimizationRun(ctx, run); err != nil {
					return nil, fmt.Errorf("save prompt optimization run: %w", err)
				}
				result := &promptdto.PromptOptimizeResponse{
					Goal:              goal,
					PreserveVariables: preserveVariables,
					DetectedVariables: detectedVariables,
					RunID:             run.ID,
					Output:            finalOutput,
					Variants: promptdto.PromptOptimizeVariantsResponse{
						Safe:     finalOutput,
						Balanced: finalOutput,
						Advanced: finalOutput,
					},
				}
				if onEvent != nil {
					if err := onEvent(PromptOptimizeStreamEvent{
						Event: "done",
						Data: map[string]interface{}{
							"goal":               goal,
							"preserve_variables": preserveVariables,
							"detected_variables": detectedVariables,
							"run_id":             run.ID,
							"output":             finalOutput,
							"provider":           resolvedModel.Provider,
							"model":              resolvedModel.Model,
						},
					}); err != nil {
						return nil, err
					}
				}
				return result, nil
			}
			if chunk.Error != nil {
				return nil, chunk.Error
			}
			if chunk.Done {
				continue
			}
			text := promptOptimizerStreamChunkText(chunk)
			if text == "" {
				continue
			}
			builder.WriteString(text)
			if onEvent != nil {
				if err := onEvent(PromptOptimizeStreamEvent{
					Event: "chunk",
					Data: map[string]interface{}{
						"text": text,
					},
				}); err != nil {
					return nil, err
				}
			}
		}
	}
}

func optimizerAppID(organizationID, workspaceID string) string {
	if strings.TrimSpace(workspaceID) != "" {
		return workspaceID
	}
	return organizationID
}

func normalizePromptOptimizerGoal(goal string) string {
	switch strings.TrimSpace(goal) {
	case promptOptimizerGoalReliable,
		promptOptimizerGoalStructured,
		promptOptimizerGoalProfessional,
		promptOptimizerGoalJSON,
		promptOptimizerGoalDeep:
		return strings.TrimSpace(goal)
	default:
		return promptOptimizerGoalGeneral
	}
}

func trimmedStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func detectPromptOptimizerVariables(rawPrompt string) []string {
	variables := make([]string, 0)
	seen := make(map[string]struct{})
	for _, pattern := range promptOptimizerVariablePatterns {
		for _, match := range pattern.FindAllString(rawPrompt, -1) {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			variables = append(variables, match)
		}
	}
	return variables
}

func (s *promptService) resolveOptimizerPromptID(
	ctx context.Context,
	organizationID,
	accountID,
	rawPromptID string,
) (*string, error) {
	if strings.TrimSpace(rawPromptID) == "" {
		return nil, nil
	}
	prompt, err := s.getAccessiblePrompt(
		ctx,
		organizationID,
		accountID,
		rawPromptID,
		workspace_model.WorkspacePermissionAgentView,
		workspace_model.WorkspacePermissionAgentManage,
	)
	if err != nil {
		return nil, err
	}
	return &prompt.ID, nil
}

func buildPromptOptimizerChatRequest(
	resolvedModel *llmdefaultservice.ResolvedModel,
	goal string,
	rawPrompt string,
	preserveVariables bool,
	detectedVariables []string,
	stream bool,
) *adapter.ChatRequest {
	temperature := 0.2
	maxTokens := 1200
	if goal == promptOptimizerGoalDeep {
		maxTokens = 1800
	}
	return &adapter.ChatRequest{
		Provider:    strings.TrimSpace(resolvedModel.Provider),
		Model:       strings.TrimSpace(resolvedModel.Model),
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Stream:      stream,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: promptOptimizerSystemPrompt(),
			},
			{
				Role: "user",
				Content: buildPromptOptimizerUserPrompt(
					goal,
					rawPrompt,
					preserveVariables,
					detectedVariables,
				),
			},
		},
	}
}

func promptOptimizerSystemPrompt() string {
	return `You are an in-product tool dedicated to optimizing user prompts. Your job is not to explain how to optimize; your job is to directly produce an improved prompt that the user can use immediately.

Output requirements:
1. Output only the final optimized prompt itself. Do not output JSON wrappers, Markdown fences, explanations, analysis steps, headings, or extra commentary.
2. Preserve the user's original intent while clearly improving clarity, reliability, structure, and actionability.
3. Match the primary language of the user's original prompt.
4. If the original prompt requires a specific format or scenario, such as JSON, tables, lists, Markdown, email, customer support, classifiers, or extraction tasks, preserve and strengthen that requirement instead of changing the use case.
5. If variables or placeholders are detected and variable preservation is enabled, preserve them exactly. Do not translate, remove, rename, or change their bracket syntax.
6. By default, produce one high-quality, directly usable result that is not unnecessarily verbose.
7. Do not generate a meta-prompt such as "please optimize the following prompt"; directly output the optimized prompt itself.`
}

func buildPromptOptimizerUserPrompt(
	goal string,
	rawPrompt string,
	preserveVariables bool,
	detectedVariables []string,
) string {
	goalDescription := map[string]string{
		promptOptimizerGoalGeneral:      "Prioritize clarity, completeness, and actionability.",
		promptOptimizerGoalReliable:     "Prioritize reducing ambiguity and improving reliability, consistency, and boundary constraints.",
		promptOptimizerGoalStructured:   "Prioritize a clearer output structure suitable for sections, steps, lists, or structured presentation.",
		promptOptimizerGoalProfessional: "Prioritize a more professional tone, stronger business writing, and more polished wording.",
		promptOptimizerGoalJSON:         "Prioritize stronger structured-output constraints for JSON, field extraction, or programmatic consumption.",
		promptOptimizerGoalDeep:         "Apply a deeper, more comprehensive prompt-optimization method.",
	}[goal]

	variableMode := "false"
	if preserveVariables {
		variableMode = "true"
	}

	variableList := "[]"
	if len(detectedVariables) > 0 {
		bytes, _ := json.Marshal(detectedVariables)
		variableList = string(bytes)
	}

	if goal == promptOptimizerGoalDeep {
		return fmt.Sprintf(`Optimize the following user prompt into one high-quality, directly usable result.

Use the following deeper optimization method internally, but do not show these steps to the user:
1. Determine the expert role or role combination best suited to solve the user's request.
2. Fill in the most likely useful background, context, and real objective.
3. Clarify the task list so the model knows exactly what actions to complete.
4. Choose the output format and writing style best suited for consuming the result.
5. Add example or demonstration requirements when useful for making the result more stable.
6. Output only one optimized prompt. Do not output the analysis steps above.

Optimization goal:
%s

Preserve variables:
%s

Detected variables:
%s

Original user prompt:
%s`, goalDescription, variableMode, variableList, rawPrompt)
	}

	return fmt.Sprintf(`Optimize the following user prompt into one high-quality, directly usable result.

Optimization goal:
%s

Preserve variables:
%s

Detected variables:
%s

Original user prompt:
%s`, goalDescription, variableMode, variableList, rawPrompt)
}

func parsePromptOptimizerTextResponse(resp *adapter.ChatResponse) (string, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty optimizer response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return "", fmt.Errorf("optimizer response is not text")
	}
	output := strings.TrimSpace(content)
	if output == "" {
		return "", fmt.Errorf("optimizer response output is empty")
	}
	return output, nil
}

func promptOptimizerStreamChunkText(resp adapter.StreamResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	content := resp.Choices[0].Delta.Content
	switch typed := content.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
