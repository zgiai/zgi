package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
)

func (s *promptService) PlaygroundStream(
	ctx context.Context,
	organizationID,
	accountID,
	workspaceID string,
	req promptdto.PromptPlaygroundRequest,
	onEvent func(PromptOptimizeStreamEvent) error,
) error {
	rawPrompt := strings.TrimSpace(req.Prompt)
	hasMessages := len(req.Messages) > 0
	if rawPrompt == "" && !hasMessages {
		return fmt.Errorf("prompt cannot be empty")
	}
	if s == nil {
		return fmt.Errorf("prompt playground is unavailable")
	}
	if err := s.requirePromptWorkspaceAccess(
		ctx,
		organizationID,
		accountID,
		workspaceID,
		promptPlaygroundPermissionCodes()...,
	); err != nil {
		return err
	}
	if s.llmClient == nil || s.defaultModelSvc == nil {
		return fmt.Errorf("prompt playground is unavailable")
	}

	if onEvent != nil {
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "progress",
			Data: map[string]interface{}{
				"step":  "analyze",
				"index": 0,
			},
		}); err != nil {
			return err
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
		return fmt.Errorf("resolve playground model: %w", err)
	}
	if resolvedModel == nil || strings.TrimSpace(resolvedModel.Model) == "" {
		return fmt.Errorf("prompt playground model is not configured")
	}

	renderedPrompt, renderedMessages, detectedVariables := renderPromptPlaygroundPayload(rawPrompt, req.Messages, req.Input, req.Variables)

	if onEvent != nil {
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "meta",
			Data: map[string]interface{}{
				"provider":           resolvedModel.Provider,
				"model":              resolvedModel.Model,
				"rendered_prompt":    renderedPrompt,
				"rendered_messages":  renderedMessages,
				"detected_variables": detectedVariables,
			},
		}); err != nil {
			return err
		}
		if err := onEvent(PromptOptimizeStreamEvent{
			Event: "progress",
			Data: map[string]interface{}{
				"step":  "rewrite",
				"index": 2,
			},
		}); err != nil {
			return err
		}
	}

	appCtx := &llmclient.AppContext{
		OrganizationID:     organizationID,
		WorkspaceID:        workspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              optimizerAppID(organizationID, workspaceID),
		AppType:            "prompt_playground",
		AccountID:          accountID,
		SessionID:          "prompt-playground:" + accountID,
		ConversationID:     "prompt-playground",
	}

	request := buildPromptPlaygroundChatRequest(resolvedModel, renderedPrompt, strings.TrimSpace(req.Input))
	if hasMessages {
		request = buildPromptPlaygroundMessageRequest(resolvedModel, renderedMessages)
	}
	stream, err := s.llmClient.AppChatStream(ctx, appCtx, request)
	if err != nil {
		return fmt.Errorf("run prompt playground stream: %w", err)
	}

	var builder strings.Builder
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				finalOutput := strings.TrimSpace(builder.String())
				if onEvent != nil {
					if err := onEvent(PromptOptimizeStreamEvent{
						Event: "progress",
						Data: map[string]interface{}{
							"step":  "polish",
							"index": 3,
						},
					}); err != nil {
						return err
					}
					if err := onEvent(PromptOptimizeStreamEvent{
						Event: "done",
						Data: map[string]interface{}{
							"output":             finalOutput,
							"provider":           resolvedModel.Provider,
							"model":              resolvedModel.Model,
							"detected_variables": detectedVariables,
							"rendered_prompt":    renderedPrompt,
							"rendered_messages":  renderedMessages,
						},
					}); err != nil {
						return err
					}
				}
				return nil
			}
			if chunk.Error != nil {
				return chunk.Error
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
					return err
				}
			}
		}
	}
}

func buildPromptPlaygroundMessageRequest(
	resolvedModel *llmdefaultservice.ResolvedModel,
	renderedMessages []adapter.Message,
) *adapter.ChatRequest {
	temperature := 0.2
	maxTokens := 1600
	return &adapter.ChatRequest{
		Provider:    strings.TrimSpace(resolvedModel.Provider),
		Model:       strings.TrimSpace(resolvedModel.Model),
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Stream:      true,
		Messages:    renderedMessages,
	}
}

func buildPromptPlaygroundChatRequest(
	resolvedModel *llmdefaultservice.ResolvedModel,
	renderedPrompt string,
	input string,
) *adapter.ChatRequest {
	temperature := 0.2
	maxTokens := 1600
	messages := []adapter.Message{
		{
			Role:    "system",
			Content: renderedPrompt,
		},
	}
	if input != "" {
		messages = append(messages, adapter.Message{
			Role:    "user",
			Content: input,
		})
	}
	return &adapter.ChatRequest{
		Provider:    strings.TrimSpace(resolvedModel.Provider),
		Model:       strings.TrimSpace(resolvedModel.Model),
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Stream:      true,
		Messages:    messages,
	}
}

func renderPromptPlaygroundPrompt(
	rawPrompt string,
	input string,
	variables map[string]string,
) (string, []string) {
	detected := detectPromptOptimizerVariables(rawPrompt)
	replacements := map[string]string{}
	for key, value := range variables {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		replacements[trimmedKey] = value
	}
	if strings.TrimSpace(input) != "" {
		replacements["input"] = input
		replacements["query"] = input
		replacements["sys.query"] = input
	}

	rendered := rawPrompt
	for _, token := range detected {
		key := normalizePromptPlaygroundVariableKey(token)
		if value, ok := replacements[key]; ok {
			rendered = strings.ReplaceAll(rendered, token, value)
		}
	}
	return rendered, detected
}

func renderPromptPlaygroundPayload(
	rawPrompt string,
	messages []promptdto.PromptPlaygroundMessage,
	input string,
	variables map[string]string,
) (string, []adapter.Message, []string) {
	if len(messages) == 0 {
		renderedPrompt, detected := renderPromptPlaygroundPrompt(rawPrompt, input, variables)
		return renderedPrompt, nil, detected
	}

	renderedMessages := make([]adapter.Message, 0, len(messages))
	detectedSet := map[string]struct{}{}
	for _, message := range messages {
		renderedContent, detected := renderPromptPlaygroundPrompt(message.Content, input, variables)
		for _, token := range detected {
			detectedSet[token] = struct{}{}
		}
		renderedMessages = append(renderedMessages, adapter.Message{
			Role:    strings.TrimSpace(message.Role),
			Content: renderedContent,
		})
	}

	detected := make([]string, 0, len(detectedSet))
	for token := range detectedSet {
		detected = append(detected, token)
	}
	slices.Sort(detected)
	return "", renderedMessages, detected
}

func normalizePromptPlaygroundVariableKey(token string) string {
	trimmed := strings.TrimSpace(token)
	trimmed = strings.TrimPrefix(trimmed, "{{")
	trimmed = strings.TrimSuffix(trimmed, "}}")
	trimmed = strings.TrimPrefix(trimmed, "#")
	trimmed = strings.TrimSuffix(trimmed, "#")
	return strings.TrimSpace(trimmed)
}
