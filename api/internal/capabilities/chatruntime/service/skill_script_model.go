package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// Reuse the prepared Agent's authorized model and billing scope, but not its
// conversation messages, tools, or generation settings. The workflow owns its
// two prompts; the script receives neither credentials nor routing controls.
func (s *service) newSkillScriptModelChannel(prepared *PreparedChat) (*skills.ScriptModelChannel, func() *adapter.Usage) {
	var mu sync.Mutex
	var usage *adapter.Usage
	snapshot := func() *adapter.Usage {
		mu.Lock()
		defer mu.Unlock()
		return addUsageCloned(nil, usage)
	}
	if s.llmClient == nil || prepared == nil || prepared.LLMRequest == nil {
		return nil, snapshot
	}
	model, provider := prepared.LLMRequest.Model, prepared.LLMRequest.Provider
	appContext := newBillingAppContext(prepared)
	appContext.SuppressInvocationContent = true
	return &skills.ScriptModelChannel{
		Model: model,
		Complete: func(ctx context.Context, system, user string) (string, error) {
			temperature := 0.0
			maxTokens := 4096
			response, err := s.llmClient.AppChat(ctx, appContext, &adapter.ChatRequest{
				Provider: provider, Model: model,
				Messages:    []adapter.Message{{Role: "system", Content: system}, {Role: "user", Content: user}},
				Temperature: &temperature, MaxTokens: &maxTokens,
				ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
			})
			if response != nil {
				mu.Lock()
				usage = addUsageCloned(usage, response.Usage)
				mu.Unlock()
			}
			if err != nil {
				return "", fmt.Errorf("skill model request failed")
			}
			if response == nil || len(response.Choices) != 1 || response.Choices[0].FinishReason == "length" || len(response.Choices[0].Message.ToolCalls) != 0 {
				return "", fmt.Errorf("skill model response is incomplete")
			}
			content, ok := response.Choices[0].Message.Content.(string)
			if !ok || strings.TrimSpace(content) == "" {
				return "", fmt.Errorf("skill model response is empty")
			}
			return content, nil
		},
	}, snapshot
}
