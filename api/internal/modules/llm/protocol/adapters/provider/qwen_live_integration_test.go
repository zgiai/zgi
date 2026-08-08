//go:build integration

package provider

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestQwenLiveCompatibleChat(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	if apiKey == "" {
		t.Fatal("DASHSCOPE_API_KEY is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("DASHSCOPE_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	qwenAdapter, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: apiKey, BaseURL: baseURL})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	t.Run("qwen3.8-max", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model:    "qwen3.8-max",
			Messages: []adapter.Message{{Role: "user", Content: "Reply with OK."}},
		})
		assertQwenLiveText(t, response)
	})

	t.Run("qwen3.8-max_vision", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model: "qwen3.8-max",
			Messages: []adapter.Message{{Role: "user", Content: []adapter.MessageContentPart{
				{Type: "text", Text: "Briefly describe the image."},
				{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "https://dashscope.oss-cn-beijing.aliyuncs.com/images/dog_and_girl.jpeg"}},
			}}},
		})
		assertQwenLiveText(t, response)
	})

	t.Run("qwen3.8-max_stream", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		stream, err := qwenAdapter.ChatCompletionStream(ctx, &adapter.ChatRequest{
			Model:    "qwen3.8-max",
			Messages: []adapter.Message{{Role: "user", Content: "Reply with OK."}},
		})
		if err != nil {
			t.Fatalf("ChatCompletionStream() error = %v", err)
		}
		var content strings.Builder
		for chunk := range stream {
			if chunk.Error != nil {
				t.Fatalf("stream error = %v", chunk.Error)
			}
			for _, choice := range chunk.Choices {
				if text, ok := choice.Delta.Content.(string); ok {
					content.WriteString(text)
				}
			}
		}
		if strings.TrimSpace(content.String()) == "" {
			t.Fatal("stream content is empty")
		}
	})

	t.Run("qwen3.8-max_tool_call", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model:    "qwen3.8-max",
			Messages: []adapter.Message{{Role: "user", Content: "Use lookup_weather for Hangzhou."}},
			Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{
				Name:        "lookup_weather",
				Description: "Look up weather for a city.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []string{"city"},
				},
			}}},
			ToolChoice: "auto",
		})
		if len(response.Choices) == 0 || len(response.Choices[0].Message.ToolCalls) == 0 {
			t.Fatal("tool call is missing")
		}
	})

	t.Run("stable", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model:    "qwen3.7-plus",
			Messages: []adapter.Message{{Role: "user", Content: "Reply with OK."}},
		})
		assertQwenLiveText(t, response)
	})

	t.Run("snapshot", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model:    "qwen3.7-plus-2026-05-26",
			Messages: []adapter.Message{{Role: "user", Content: "Reply with OK."}},
		})
		assertQwenLiveText(t, response)
	})

	t.Run("vision", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model: "qwen3.7-plus",
			Messages: []adapter.Message{{Role: "user", Content: []adapter.MessageContentPart{
				{Type: "text", Text: "Briefly describe the image."},
				{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "https://dashscope.oss-cn-beijing.aliyuncs.com/images/dog_and_girl.jpeg"}},
			}}},
		})
		assertQwenLiveText(t, response)
	})

	t.Run("stream", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		stream, err := qwenAdapter.ChatCompletionStream(ctx, &adapter.ChatRequest{
			Model:    "qwen3.7-plus",
			Messages: []adapter.Message{{Role: "user", Content: "Reply with OK."}},
		})
		if err != nil {
			t.Fatalf("ChatCompletionStream() error = %v", err)
		}
		var content strings.Builder
		for chunk := range stream {
			if chunk.Error != nil {
				t.Fatalf("stream error = %v", chunk.Error)
			}
			for _, choice := range chunk.Choices {
				if text, ok := choice.Delta.Content.(string); ok {
					content.WriteString(text)
				}
			}
		}
		if strings.TrimSpace(content.String()) == "" {
			t.Fatal("stream content is empty")
		}
	})

	t.Run("tool_call", func(t *testing.T) {
		response := qwenLiveChat(t, qwenAdapter, &adapter.ChatRequest{
			Model:    "qwen3.7-plus",
			Messages: []adapter.Message{{Role: "user", Content: "Use lookup_weather for Hangzhou."}},
			Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{
				Name:        "lookup_weather",
				Description: "Look up weather for a city.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []string{"city"},
				},
			}}},
			ToolChoice: "auto",
		})
		if len(response.Choices) == 0 || len(response.Choices[0].Message.ToolCalls) == 0 {
			t.Fatal("tool call is missing")
		}
	})
}

func TestQwenLiveHistoricalModelMatrix(t *testing.T) {
	if strings.TrimSpace(os.Getenv("QWEN_LIVE_HISTORY")) != "1" {
		t.Skip("set QWEN_LIVE_HISTORY=1 to probe the historical model matrix")
	}
	apiKey := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	if apiKey == "" {
		t.Fatal("DASHSCOPE_API_KEY is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("DASHSCOPE_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	qwenAdapter, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: apiKey, BaseURL: baseURL})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	models := []string{
		"qwen3.7-max",
		"qwen3.7-max-2026-05-20",
		"qwen3.7-max-2026-06-08",
		"qwen3.7-plus",
		"qwen3.7-plus-2026-05-26",
		"qwen3.6-max-preview",
		"qwen3.6-plus",
		"qwen3.6-plus-2026-04-02",
		"qwen3.6-flash",
		"qwen3.6-flash-2026-04-16",
		"qwen3.5-plus",
		"qwen3.5-plus-2026-02-15",
		"qwen3.5-flash",
		"qwen3.5-flash-2026-02-23",
		"qwen3-max",
		"qwen3-max-2025-09-23",
		"qwen-max",
		"qwen-plus",
		"qwen-plus-2025-12-01",
		"qwen-flash",
		"qwen-flash-2025-07-28",
		"qwen-turbo",
		"qwen-long",
		"qwen-vl-max",
		"qwen-vl-plus",
		"qwq-plus",
		"qvq-max",
		"qwen-max-latest",
		"qwen-max-2025-01-25",
		"qwen-plus-2024-11-27",
		"qwen-turbo-2024-09-19",
		"qwen-vl-max-2024-10-30",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			started := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			response, err := qwenAdapter.ChatCompletion(ctx, &adapter.ChatRequest{
				Model:    model,
				Messages: []adapter.Message{{Role: "user", Content: "Reply exactly OK."}},
			})
			if err != nil {
				t.Errorf("unavailable after %s: %v", time.Since(started).Round(time.Millisecond), err)
				return
			}
			if response == nil || len(response.Choices) == 0 {
				t.Errorf("empty response after %s", time.Since(started).Round(time.Millisecond))
				return
			}
			content, _ := response.Choices[0].Message.Content.(string)
			if strings.TrimSpace(content) == "" {
				t.Errorf(
					"empty content after %s (finish_reason=%q reasoning_chars=%d)",
					time.Since(started).Round(time.Millisecond),
					response.Choices[0].FinishReason,
					len(response.Choices[0].Message.ReasoningContent),
				)
				return
			}
			t.Logf("available in %s", time.Since(started).Round(time.Millisecond))
		})
	}
}

func TestQwenLiveHistoricalStreamingOnlyModels(t *testing.T) {
	if strings.TrimSpace(os.Getenv("QWEN_LIVE_HISTORY")) != "1" {
		t.Skip("set QWEN_LIVE_HISTORY=1 to probe historical streaming-only models")
	}
	apiKey := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	if apiKey == "" {
		t.Fatal("DASHSCOPE_API_KEY is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("DASHSCOPE_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	qwenAdapter, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: apiKey, BaseURL: baseURL})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	tests := []struct {
		model   string
		content any
	}{
		{model: "qwq-plus", content: "Reply exactly OK."},
		{model: "qvq-max", content: []adapter.MessageContentPart{
			{Type: "text", Text: "Briefly describe the image."},
			{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "https://dashscope.oss-cn-beijing.aliyuncs.com/images/dog_and_girl.jpeg"}},
		}},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			started := time.Now()
			stream, err := qwenAdapter.ChatCompletionStream(ctx, &adapter.ChatRequest{
				Model:    test.model,
				Messages: []adapter.Message{{Role: "user", Content: test.content}},
			})
			if err != nil {
				t.Fatalf("unavailable after %s: %v", time.Since(started).Round(time.Millisecond), err)
			}
			contentChars := 0
			reasoningChars := 0
			for chunk := range stream {
				if chunk.Error != nil {
					t.Fatalf("stream error after %s: %v", time.Since(started).Round(time.Millisecond), chunk.Error)
				}
				for _, choice := range chunk.Choices {
					if text, ok := choice.Delta.Content.(string); ok {
						contentChars += len(text)
					}
					reasoningChars += len(choice.Delta.ReasoningContent)
				}
			}
			if contentChars == 0 {
				t.Fatalf("empty final content (reasoning_chars=%d)", reasoningChars)
			}
			t.Logf("available in %s", time.Since(started).Round(time.Millisecond))
		})
	}
}

func qwenLiveChat(t *testing.T, qwenAdapter adapter.ChatCapable, request *adapter.ChatRequest) *adapter.ChatResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	response, err := qwenAdapter.ChatCompletion(ctx, request)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	return response
}

func assertQwenLiveText(t *testing.T, response *adapter.ChatResponse) {
	t.Helper()
	if response == nil || len(response.Choices) == 0 {
		t.Fatal("chat response has no choices")
	}
	content, _ := response.Choices[0].Message.Content.(string)
	if strings.TrimSpace(content) == "" {
		t.Fatal("chat response content is empty")
	}
}
