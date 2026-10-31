package tokenestimate

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestEstimateChatRequestIncludesPromptBearingComponents(t *testing.T) {
	temperature := 0.2
	request := &adapter.ChatRequest{
		Model:       "deepseek-chat",
		Temperature: &temperature,
		Messages: []adapter.Message{{
			Role:    "user",
			Content: "summarize the record",
		}},
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name:        "lookup_record",
				Description: "Look up one record",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{"type": "string"},
					},
				},
			},
		}},
		ToolChoice:     map[string]interface{}{"type": "auto"},
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
		AdditionalParameters: map[string]interface{}{
			"reasoning_effort": "low",
		},
	}

	result := NewEstimator().EstimateChatRequest(request)
	for _, name := range []string{"messages", "tools", "tool_choice", "response_format", "additional_parameters"} {
		component, ok := result.Components[name]
		if !ok {
			t.Fatalf("component %q missing: %#v", name, result.Components)
		}
		if component.Tokens <= 0 || component.Characters <= 0 {
			t.Fatalf("component %q = %+v, want positive counts", name, component)
		}
	}

	total := 0
	for _, component := range result.Components {
		total += component.Tokens
	}
	if result.Tokens != total {
		t.Fatalf("tokens = %d, want component total %d", result.Tokens, total)
	}
	if result.Characters <= result.Components["messages"].Characters {
		t.Fatalf("request characters = %d, want larger than messages-only %d", result.Characters, result.Components["messages"].Characters)
	}
	if result.Tokenizer == "" {
		t.Fatal("tokenizer is empty")
	}
}

func TestEstimateChatRequestDoesNotTreatGenerationControlsAsPromptComponents(t *testing.T) {
	temperature := 0.7
	maxTokens := 4096
	request := &adapter.ChatRequest{
		Model:       "deepseek-chat",
		Messages:    []adapter.Message{{Role: "user", Content: "hello"}},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Stop:        []string{"DONE"},
	}

	result := NewEstimator().EstimateChatRequest(request)
	if len(result.Components) != 1 {
		t.Fatalf("components = %#v, want messages only", result.Components)
	}
	if _, ok := result.Components["messages"]; !ok {
		t.Fatalf("messages component missing: %#v", result.Components)
	}
	if result.Characters <= result.Components["messages"].Characters {
		t.Fatalf("request characters = %d, want controls reflected beyond messages %d", result.Characters, result.Components["messages"].Characters)
	}
}

func TestEstimateTextSupportsMultilingualActionProgress(t *testing.T) {
	estimator := NewEstimator()
	cases := []string{
		"我正在整理数据，完成后会给你一份可下载的分析报告。",
		"أقوم الآن بتحليل البيانات وسأقدم لك تقريرًا قابلاً للتنزيل عند الاكتمال.",
		"データを整理しています。完了後、ダウンロードできるレポートをお渡しします。",
		"กำลังวิเคราะห์ข้อมูลและจะส่งรายงานที่ดาวน์โหลดได้เมื่อเสร็จสิ้น 📊",
		"I’m analyzing the data now and will provide a downloadable report when it is ready. 🚀",
	}
	for _, text := range cases {
		result := estimator.EstimateText(text, "deepseek-chat")
		if result.Tokens <= 0 {
			t.Fatalf("EstimateText(%q) = %#v, want positive token count", text, result)
		}
		if result.Tokenizer != fallbackCJKCompact {
			t.Fatalf("EstimateText(%q) tokenizer = %q, want the CJK-compact fallback for DeepSeek", text, result.Tokenizer)
		}
	}
}

func TestFallbackEstimatesEachCharacterClassAtItsOwnRate(t *testing.T) {
	estimator := NewEstimator()
	chinese := strings.Repeat("数据整理完成生成报告", 100) // 1,000 Han characters
	if got := estimator.EstimateText(chinese, "deepseek-v4-flash").Tokens; got != 800 {
		t.Fatalf("DeepSeek estimate for 1,000 Han characters = %d, want 800", got)
	}
	if got := estimator.EstimateText(chinese, "claude-sonnet-5-5").Tokens; got != 1500 {
		t.Fatalf("Claude estimate for 1,000 Han characters = %d, want 1500", got)
	}
	// One emoji no longer makes a whole English text count as two bytes per token.
	english := strings.Repeat("abc ", 300) + "🚀"
	if got := estimator.EstimateText(english, "deepseek-v4-flash").Tokens; got != 400+2 {
		t.Fatalf("English estimate = %d, want 402", got)
	}
	if got := estimator.EstimateText("报告", "qwen/qwen3-max").Tokenizer; got != fallbackCJKCompact {
		t.Fatalf("a provider-prefixed Qwen model uses %q", got)
	}
}

// Reasoning counts for models that are sent it, and only for them.
func TestEstimateMessagesCountsReasoningForModelsThatReplayIt(t *testing.T) {
	estimator := NewEstimator()
	plain := []adapter.Message{{Role: "assistant", Content: "done"}}
	withReasoning := []adapter.Message{{Role: "assistant", Content: "done", ReasoningContent: strings.Repeat("thinking ", 200)}}
	if estimator.EstimateMessages(withReasoning, "deepseek-v4-flash").Tokens <= estimator.EstimateMessages(plain, "deepseek-v4-flash").Tokens {
		t.Fatal("want reasoning counted for DeepSeek")
	}
	if estimator.EstimateMessages(withReasoning, "gpt-5").Tokens != estimator.EstimateMessages(plain, "gpt-5").Tokens {
		t.Fatal("want reasoning not counted for a model that is not sent it")
	}
}
