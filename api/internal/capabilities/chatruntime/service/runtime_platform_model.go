package service

import (
	"context"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// The owning Runner calls this synchronously. Scripts choose only bounded text
// and an output limit; Agent routing, billing and task budgets remain owned by
// the prepared conversation. No history, tools or provider credentials escape.
func (s *service) newRuntimePlatformModel(prepared *PreparedChat, observer scriptModelUsageObserver) (func(context.Context, string, int) (string, error), func() *adapter.Usage) {
	var usage *adapter.Usage
	snapshot := func() *adapter.Usage { return addUsageCloned(nil, usage) }
	if s.llmClient == nil || prepared == nil || prepared.LLMRequest == nil {
		return nil, snapshot
	}
	appContext := newBillingAppContext(prepared)
	appContext.SuppressInvocationContent = true
	return func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		if strings.TrimSpace(prompt) == "" || len(prompt) > 32<<10 || maxTokens < 1 || maxTokens > 4096 {
			return "", apperror.New(catalog.CodeRequestInvalid, apperror.WithOperation("runtime_platform.model_arguments"))
		}
		if err := prepared.taskTokenBudget.exhausted(); err != nil {
			return "", err
		}
		if budget := prepared.taskTokenBudget; budget != nil {
			maxTokens = min(maxTokens, budget.MaxTokens-budget.UsedTokens)
		}
		response, err := s.llmClient.AppChat(ctx, appContext, &adapter.ChatRequest{
			Provider: prepared.LLMRequest.Provider, Model: prepared.LLMRequest.Model,
			Messages: []adapter.Message{{Role: "user", Content: prompt}}, MaxTokens: &maxTokens,
		})
		if response != nil {
			usage = addUsageCloned(usage, response.Usage)
			if observer != nil && response.Usage != nil {
				if budgetErr := observer(response.Usage); budgetErr != nil {
					return "", budgetErr
				}
			}
		}
		if err != nil {
			return "", apperror.Wrap(err, catalog.CodeInternal, apperror.WithOperation("runtime_platform.model_call"))
		}
		if response == nil || len(response.Choices) != 1 || response.Choices[0].FinishReason == "length" || len(response.Choices[0].Message.ToolCalls) != 0 {
			return "", apperror.New(catalog.CodeInternal, apperror.WithOperation("runtime_platform.model_incomplete"))
		}
		text, ok := response.Choices[0].Message.Content.(string)
		if !ok || strings.TrimSpace(text) == "" || len(text) > 24<<10 {
			return "", apperror.New(catalog.CodeInternal, apperror.WithOperation("runtime_platform.model_output"))
		}
		return text, nil
	}, snapshot
}
