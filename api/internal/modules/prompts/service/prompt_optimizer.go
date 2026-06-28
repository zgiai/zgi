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
)

const (
	promptOptimizerGoalGeneral      = "general"
	promptOptimizerGoalReliable     = "reliable"
	promptOptimizerGoalStructured   = "structured"
	promptOptimizerGoalProfessional = "professional"
	promptOptimizerGoalJSON         = "json"
	promptOptimizerGoalDeep         = "deep"
)

const (
	promptOptimizerDefaultMaxTokens     = 8192
	promptOptimizerDeepDefaultMaxTokens = 16000
	promptOptimizerPlatformMaxTokens    = 32000
	promptOptimizerTargetMaxCharsLimit  = 50000
)

var promptOptimizerVariablePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)<zgi:(knowledge|skill)\b[^>]*>.*?</zgi:(knowledge|skill)>`),
	regexp.MustCompile(`\{\{#[^{}]+#\}\}`),
	regexp.MustCompile(`\{\{[^{}]+\}\}`),
	regexp.MustCompile(`\$\{[^{}]+\}`),
}

var promptOptimizerZGITemplateBlockPattern = regexp.MustCompile(`(?s)<zgi:(slot|knowledge|skill)\b([^>]*)>(.*?)</zgi:(slot|knowledge|skill)>`)

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
	optimizerPrompt := normalizePromptOptimizerInput(rawPrompt)
	if optimizerPrompt == "" {
		return nil, fmt.Errorf("raw prompt cannot be empty")
	}
	if s == nil {
		return nil, fmt.Errorf("prompt optimizer is unavailable")
	}
	if err := s.requirePromptWorkspaceAccess(ctx, organizationID, accountID, workspaceID, promptOptimizePermissionCodes()...); err != nil {
		return nil, err
	}

	goal := normalizePromptOptimizerGoal(req.Goal)
	preserveVariables := req.PreserveVariables == nil || *req.PreserveVariables
	detectedVariables := detectPromptOptimizerVariables(optimizerPrompt)
	outputLanguage := promptOptimizerOutputLanguage(req.Language)
	editInstruction := strings.TrimSpace(req.EditInstruction)
	targetMaxChars := normalizePromptOptimizerTargetMaxChars(req.TargetMaxChars)

	promptID, err := s.resolveOptimizerPromptID(ctx, organizationID, accountID, workspaceID, req.PromptID)
	if err != nil {
		return nil, err
	}
	if s.llmClient == nil || s.defaultModelSvc == nil {
		return nil, fmt.Errorf("prompt optimizer is unavailable")
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
			optimizerPrompt,
			preserveVariables,
			detectedVariables,
			outputLanguage,
			editInstruction,
			targetMaxChars,
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
	finishReason := promptOptimizerResponseFinishReason(response)
	truncated := promptOptimizerFinishReasonIsTruncated(finishReason)
	if truncated {
		return &promptdto.PromptOptimizeResponse{
			Goal:              goal,
			PreserveVariables: preserveVariables,
			DetectedVariables: detectedVariables,
			Output:            output,
			Variants: promptdto.PromptOptimizeVariantsResponse{
				Safe:     output,
				Balanced: output,
				Advanced: output,
			},
			Truncated:      true,
			FinishReason:   finishReason,
			TargetMaxChars: targetMaxChars,
		}, nil
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
		FinishReason:   finishReason,
		TargetMaxChars: targetMaxChars,
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
	optimizerPrompt := normalizePromptOptimizerInput(rawPrompt)
	if optimizerPrompt == "" {
		return nil, fmt.Errorf("raw prompt cannot be empty")
	}
	if s == nil {
		return nil, fmt.Errorf("prompt optimizer is unavailable")
	}
	if err := s.requirePromptWorkspaceAccess(ctx, organizationID, accountID, workspaceID, promptOptimizePermissionCodes()...); err != nil {
		return nil, err
	}

	goal := normalizePromptOptimizerGoal(req.Goal)
	preserveVariables := req.PreserveVariables == nil || *req.PreserveVariables
	detectedVariables := detectPromptOptimizerVariables(optimizerPrompt)
	outputLanguage := promptOptimizerOutputLanguage(req.Language)
	editInstruction := strings.TrimSpace(req.EditInstruction)
	targetMaxChars := normalizePromptOptimizerTargetMaxChars(req.TargetMaxChars)

	promptID, err := s.resolveOptimizerPromptID(ctx, organizationID, accountID, workspaceID, req.PromptID)
	if err != nil {
		return nil, err
	}
	if s.llmClient == nil || s.defaultModelSvc == nil {
		return nil, fmt.Errorf("prompt optimizer is unavailable")
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
				"target_max_chars":   targetMaxChars,
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
			optimizerPrompt,
			preserveVariables,
			detectedVariables,
			outputLanguage,
			editInstruction,
			targetMaxChars,
			true,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("optimize prompt stream with llm: %w", err)
	}

	var builder strings.Builder
	finishReason := ""
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
				truncated := promptOptimizerFinishReasonIsTruncated(finishReason)
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
				runID := ""
				if !truncated {
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
					runID = run.ID
				}
				result := &promptdto.PromptOptimizeResponse{
					Goal:              goal,
					PreserveVariables: preserveVariables,
					DetectedVariables: detectedVariables,
					RunID:             runID,
					Output:            finalOutput,
					Variants: promptdto.PromptOptimizeVariantsResponse{
						Safe:     finalOutput,
						Balanced: finalOutput,
						Advanced: finalOutput,
					},
					Truncated:      truncated,
					FinishReason:   finishReason,
					TargetMaxChars: targetMaxChars,
				}
				if onEvent != nil {
					if err := onEvent(PromptOptimizeStreamEvent{
						Event: "done",
						Data: map[string]interface{}{
							"goal":               goal,
							"preserve_variables": preserveVariables,
							"detected_variables": detectedVariables,
							"run_id":             runID,
							"output":             finalOutput,
							"provider":           resolvedModel.Provider,
							"model":              resolvedModel.Model,
							"truncated":          truncated,
							"finish_reason":      finishReason,
							"target_max_chars":   targetMaxChars,
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
			if reason := promptOptimizerStreamFinishReason(chunk); reason != "" {
				finishReason = reason
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

func normalizePromptOptimizerTargetMaxChars(value int) int {
	if value <= 0 {
		return 0
	}
	if value > promptOptimizerTargetMaxCharsLimit {
		return promptOptimizerTargetMaxCharsLimit
	}
	return value
}

func promptOptimizerMaxTokens(resolvedModel *llmdefaultservice.ResolvedModel, goal string) int {
	fallback := promptOptimizerDefaultMaxTokens
	if goal == promptOptimizerGoalDeep {
		fallback = promptOptimizerDeepDefaultMaxTokens
	}
	if resolvedModel == nil || resolvedModel.MaxOutputTokens <= 0 {
		return fallback
	}
	if resolvedModel.MaxOutputTokens > promptOptimizerPlatformMaxTokens {
		return promptOptimizerPlatformMaxTokens
	}
	return resolvedModel.MaxOutputTokens
}

func promptOptimizerResponseFinishReason(resp *adapter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Choices[0].FinishReason)
}

func promptOptimizerStreamFinishReason(resp adapter.StreamResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Choices[0].FinishReason)
}

func promptOptimizerFinishReasonIsTruncated(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	if normalized == "" {
		return false
	}
	return normalized == "length" ||
		strings.Contains(normalized, "max_token") ||
		strings.Contains(normalized, "token_limit") ||
		strings.Contains(normalized, "output_limit")
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

func normalizePromptOptimizerInput(rawPrompt string) string {
	source := strings.TrimSpace(rawPrompt)
	if source == "" {
		return ""
	}
	normalized := promptOptimizerZGITemplateBlockPattern.ReplaceAllStringFunc(source, func(token string) string {
		matches := promptOptimizerZGITemplateBlockPattern.FindStringSubmatch(token)
		if len(matches) < 5 || matches[1] != matches[4] {
			return token
		}
		if matches[1] != "slot" {
			return token
		}
		return decodePromptOptimizerTemplateText(matches[3])
	})
	return strings.TrimSpace(normalized)
}

func decodePromptOptimizerTemplateText(value string) string {
	return strings.NewReplacer(
		"&quot;", `"`,
		"&apos;", "'",
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
	).Replace(value)
}

func promptOptimizerOutputLanguage(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch {
	case normalized == "":
		return ""
	case strings.HasPrefix(normalized, "zh"):
		return "Simplified Chinese"
	case strings.HasPrefix(normalized, "en"):
		return "English"
	default:
		return strings.TrimSpace(language)
	}
}

func promptOptimizerLengthInstruction(targetMaxChars int) string {
	if targetMaxChars <= 0 {
		return "No explicit product character limit is provided. Keep the prompt concise and directly usable."
	}
	return fmt.Sprintf(
		"Final prompt must be no more than %d characters. If the source contains too much material, compress it while preserving identity, task flow, tool rules, safety/compliance, escalation rules, and prohibitions; shorten examples and repetitive explanation first. Do not add filler just to use the budget.",
		targetMaxChars,
	)
}

func promptOptimizerEditInstructionBlock(editInstruction string) string {
	trimmed := strings.TrimSpace(editInstruction)
	if trimmed == "" {
		return "No additional user-directed edit request."
	}
	return trimmed + "\n\nFollow this edit request unless it conflicts with variable preservation, ZGI tag preservation, output language, or the target length."
}

func (s *promptService) resolveOptimizerPromptID(
	ctx context.Context,
	organizationID,
	accountID,
	workspaceID,
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
		promptOptimizePermissionCodes()...,
	)
	if err != nil {
		return nil, err
	}
	if prompt.Source != promptmodel.PromptSourceOfficial &&
		(prompt.WorkspaceID == nil || strings.TrimSpace(*prompt.WorkspaceID) != strings.TrimSpace(workspaceID)) {
		return nil, fmt.Errorf("prompt not found")
	}
	return &prompt.ID, nil
}

func buildPromptOptimizerChatRequest(
	resolvedModel *llmdefaultservice.ResolvedModel,
	goal string,
	rawPrompt string,
	preserveVariables bool,
	detectedVariables []string,
	outputLanguage string,
	editInstruction string,
	targetMaxChars int,
	stream bool,
) *adapter.ChatRequest {
	temperature := 0.2
	maxTokens := promptOptimizerMaxTokens(resolvedModel, goal)
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
					outputLanguage,
					editInstruction,
					targetMaxChars,
				),
			},
		},
	}
}

func promptOptimizerSystemPrompt() string {
	return `You are an expert prompt engineer embedded in a product. Convert ordinary user prompts into strong, production-ready prompts that can be used immediately.

Use this internal CRISPE-style method before writing the final prompt, but never reveal the method or your reasoning:
1. Role and capability: infer the most useful expert role or role combination for solving the user's request.
2. Context: infer the likely background, motivation, constraints, audience, and success criteria behind the request.
3. Task: turn the request into a clear task list with concrete responsibilities, boundaries, and decision rules.
4. Output format: choose the most useful output structure and writing style, such as Markdown, table, JSON, checklist, dialogue, or concise prose.
5. Examples: add example, counterexample, demonstration, or few-shot requirements only when they make the final result more reliable.
6. Final prompt: write as if you are the user asking the target model. Make the result complete, specific, and directly executable.

Output requirements:
1. Output only the final optimized prompt itself. Do not output JSON wrappers, Markdown fences, explanations, analysis steps, framework labels, headings about optimization, or extra commentary.
2. Preserve the user's original intent and domain. Strengthen clarity, reliability, structure, actionability, constraints, and evaluation criteria.
3. Use the requested system/interface language for the final optimized prompt when it is provided. If no requested system/interface language is provided, match the primary language of the user's original prompt.
4. If the original prompt is for a system prompt, agent instruction, workflow node, RAG answer, classifier, extraction task, JSON generation, code task, email, support reply, or other specific scenario, preserve that scenario and optimize for it.
5. If variables or placeholders are detected and variable preservation is enabled, preserve every detected variable token exactly as an opaque literal. Do not translate, remove, rename, reorder, escape, split, wrap, or change its syntax. This includes ZGI capability references such as <zgi:knowledge ...>...</zgi:knowledge> and <zgi:skill ...>...</zgi:skill>.
6. The final prompt should usually contain a strong expert role, useful background/context, a concrete task list, output requirements, constraints/guardrails, and example requirements. For very short original prompts, enrich them with reasonable missing context instead of merely rephrasing them.
7. Do not output a meta-prompt asking another model to optimize the prompt. Output the optimized prompt that should be used directly.`
}

func buildPromptOptimizerUserPrompt(
	goal string,
	rawPrompt string,
	preserveVariables bool,
	detectedVariables []string,
	outputLanguage string,
	editInstruction string,
	targetMaxChars int,
) string {
	goalDescription := map[string]string{
		promptOptimizerGoalGeneral:      "Create a balanced, high-quality prompt with clear role, context, task, constraints, and output format.",
		promptOptimizerGoalReliable:     "Reduce ambiguity, prevent common failure modes, define boundaries, and make outputs consistent.",
		promptOptimizerGoalStructured:   "Make the output contract explicit with sections, ordered steps, tables, schemas, or checklists when appropriate.",
		promptOptimizerGoalProfessional: "Improve business writing, product tone, executive clarity, and polished wording without weakening instructions.",
		promptOptimizerGoalJSON:         "Make structured-output requirements strict, valid, parseable, and resilient for programmatic consumption.",
		promptOptimizerGoalDeep:         "Apply the full internal CRISPE-style prompt engineering method and produce a more complete production-ready prompt.",
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

	languageInstruction := "Use the primary language of the original user prompt."
	if strings.TrimSpace(outputLanguage) != "" {
		languageInstruction = fmt.Sprintf("Use the system/interface language for the final optimized prompt: %s.", outputLanguage)
	}
	lengthInstruction := promptOptimizerLengthInstruction(targetMaxChars)
	editInstructionBlock := promptOptimizerEditInstructionBlock(editInstruction)

	if goal == promptOptimizerGoalDeep {
		return fmt.Sprintf(`Optimize the following user prompt into one high-quality, directly usable result.

Use the full internal CRISPE-style method:
1. Infer the best expert role and capabilities.
2. Infer useful context, background, objective, audience, constraints, and success criteria.
3. Convert the request into a concrete task list with boundaries and failure handling.
4. Select the best output format and style for the result.
5. Add example requirements only when they improve reliability.
6. Write the final prompt as the user, ready to paste into the target model.

Do not show the method, analysis, or step labels. Output only the optimized prompt.

Quality bar for deep optimization:
- The result must read like a complete prompt the user can paste into ChatGPT or another LLM.
- The result should be substantially better than the original, not just a polished rewrite.
- Include an expert role, inferred context, explicit task list, output format, constraints, and example requirements when the original prompt does not already provide them.
- %s
- %s
- Do not use the words "CRISPE", "role and capability", "context explanation", "task statement", "output format", "case requirement", or "optimized prompt" as labels in the final answer.

Optimization goal:
%s

User-directed edit request:
%s

Preserve variables:
%s

Detected variables:
%s

Variable preservation rules:
- If Preserve variables is true, copy every detected variable exactly as listed.
- Treat detected variables as immutable opaque tokens, not natural language.
- Do not alter XML-like ZGI tags, attributes, labels, brackets, braces, punctuation, or casing.
- The original prompt has already had editable slot wrappers removed; do not invent or reintroduce slot wrapper syntax.

Original user prompt:
%s`, languageInstruction, lengthInstruction, goalDescription, editInstructionBlock, variableMode, variableList, rawPrompt)
	}

	return fmt.Sprintf(`Optimize the following user prompt into one high-quality, directly usable result.

Internally infer the role, context, task list, output format, and useful examples before writing the final prompt. Do not show that analysis.

Output language:
%s

Length target:
%s

Optimization goal:
%s

User-directed edit request:
%s

Preserve variables:
%s

Detected variables:
%s

Variable preservation rules:
- If Preserve variables is true, copy every detected variable exactly as listed.
- Treat detected variables as immutable opaque tokens, not natural language.
- Do not alter XML-like ZGI tags, attributes, labels, brackets, braces, punctuation, or casing.
- The original prompt has already had editable slot wrappers removed; do not invent or reintroduce slot wrapper syntax.

Original user prompt:
%s`, languageInstruction, lengthInstruction, goalDescription, editInstructionBlock, variableMode, variableList, rawPrompt)
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
