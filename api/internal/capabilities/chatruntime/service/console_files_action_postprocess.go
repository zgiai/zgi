package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	consoleFilesPostprocessMaxFileChars = 6000
	consoleFilesPostprocessMaxTokens    = 1500
)

type consoleFilesPostprocessLLM interface {
	AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error)
}

func (s *service) postprocessConsoleFilesActionAnswer(ctx context.Context, prepared *PreparedChat, run *actionservice.ActionRunView, decision AIChatActionDecision, fallback string) (string, *adapter.Usage, error) {
	if s == nil || s.llmClient == nil || prepared == nil || prepared.parts == nil || len(decision.Postprocess) == 0 {
		return fallback, nil, nil
	}
	started := time.Now()
	req, err := newConsoleFilesPostprocessLLMRequest(prepared, run, decision, fallback)
	if err != nil {
		return fallback, nil, err
	}
	resp, err := s.llmClient.AppChat(ctx, newBillingAppContext(prepared), req)
	if err != nil {
		s.persistModelInvocationBestEffort(context.WithoutCancel(ctx), prepared, skillloop.ModelInvocationTrace{
			Phase:      "action_postprocess",
			Round:      -1,
			Streaming:  false,
			StartedAt:  started,
			DurationMS: time.Since(started).Milliseconds(),
			Request:    req,
			Error:      err.Error(),
		})
		return fallback, nil, err
	}
	answer := strings.TrimSpace(aiChatActionPlannerResponseText(resp))
	if answer == "" {
		answer = fallback
	}
	s.persistModelInvocationBestEffort(context.WithoutCancel(ctx), prepared, skillloop.ModelInvocationTrace{
		Phase:      "action_postprocess",
		Round:      -1,
		Streaming:  false,
		StartedAt:  started,
		DurationMS: time.Since(started).Milliseconds(),
		Request:    req,
		Response:   &adapter.Message{Role: "assistant", Content: answer},
		Usage:      responseUsage(resp),
	})
	return answer, responseUsage(resp), nil
}

func newConsoleFilesPostprocessLLMRequest(prepared *PreparedChat, run *actionservice.ActionRunView, decision AIChatActionDecision, fallback string) (*adapter.ChatRequest, error) {
	if prepared == nil || prepared.parts == nil {
		return nil, fmt.Errorf("%w: prepared chat is required", ErrInvalidInput)
	}
	payload := map[string]interface{}{
		"user_query":      prepared.parts.Query,
		"postprocess":     decision.Postprocess,
		"fallback_answer": fallback,
		"files":           consoleFilesPostprocessFiles(run),
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal console files postprocess payload: %w", err)
	}
	temperature := 0.2
	maxTokens := consoleFilesPostprocessMaxTokens
	return &adapter.ChatRequest{
		Provider: prepared.parts.Provider,
		Model:    prepared.parts.ModelName,
		Messages: []adapter.Message{
			{Role: "system", Content: consoleFilesPostprocessSystemPrompt()},
			{Role: "user", Content: string(rawPayload)},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Stream:      false,
	}, nil
}

func consoleFilesPostprocessSystemPrompt() string {
	return strings.Join([]string{
		"You are AIChat composing the final answer after the system has already read files.",
		"Use only the JSON payload. Do not claim to access any file beyond the provided action output.",
		"Honor the requested postprocess operations such as translation or summarization.",
		"If file content is unavailable, say that clearly and use available metadata only.",
		"Answer directly in the user's language unless a target_language asks otherwise.",
	}, "\n")
}

func consoleFilesPostprocessFiles(run *actionservice.ActionRunView) []map[string]interface{} {
	files := actionRunOutputFiles(run)
	if len(files) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		item := map[string]interface{}{}
		copyStringPostprocessField(item, file, "id")
		copyStringPostprocessField(item, file, "name")
		copyStringPostprocessField(item, file, "extension")
		copyStringPostprocessField(item, file, "mime_type")
		copyStringPostprocessField(item, file, "content_status")
		copyStringPostprocessField(item, file, "filtered_reason")
		if preview := stringMetadataValue(file["content_preview"]); preview != "" {
			item["content_preview"] = truncateRunes(preview, consoleFilesPostprocessMaxFileChars)
		}
		if truncated, ok := file["content_truncated"].(bool); ok {
			item["content_truncated"] = truncated
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func copyStringPostprocessField(out map[string]interface{}, source map[string]interface{}, key string) {
	if out == nil || source == nil {
		return
	}
	if value := stringMetadataValue(source[key]); value != "" {
		out[key] = value
	}
}

func responseUsage(resp *adapter.ChatResponse) *adapter.Usage {
	if resp == nil || resp.Usage == nil {
		return nil
	}
	usage := *resp.Usage
	return &usage
}
