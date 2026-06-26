package gateway

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestTokenEstimator_NewAPIStyleTextCounting(t *testing.T) {
	estimator := NewTokenEstimator()

	tests := []struct {
		name  string
		model string
		text  string
		want  int
	}{
		{name: "openai tokenizer", model: "gpt-4o-mini", text: "antidisestablishmentarianism", want: 6},
		{name: "claude heuristic", model: "claude-3-sonnet", text: "你好世界", want: 5},
		{name: "gemini heuristic", model: "gemini-pro", text: "你好世界", want: 3},
		{name: "default heuristic", model: "custom-model", text: "你好世界", want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimator.EstimateTextTokensForModel(tt.model, tt.text)
			if got != tt.want {
				t.Fatalf("EstimateTextTokensForModel(%q, %q) = %d, want %d", tt.model, tt.text, got, tt.want)
			}
		})
	}
}

func TestTokenEstimator_HeuristicCountsLongAlphanumericRunsByLength(t *testing.T) {
	estimator := NewTokenEstimator()

	if got := estimator.EstimateTextTokensForModel("custom-model", strings.Repeat("a", 40)); got != 11 {
		t.Fatalf("long letter run tokens = %d, want 11", got)
	}
	if got := estimator.EstimateTextTokensForModel("custom-model", strings.Repeat("1", 30)); got != 16 {
		t.Fatalf("long number run tokens = %d, want 16", got)
	}
}

func TestTokenEstimator_EstimateChatPromptTokensAddsStructure(t *testing.T) {
	estimator := NewTokenEstimator()
	req := &adapter.ChatRequest{
		Model: "gpt-5",
		Messages: []adapter.Message{{
			Role:    "user",
			Name:    "tester",
			Content: "12345678",
		}},
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name: "lookup",
			},
		}},
	}

	got := estimator.EstimateChatPromptTokens(req)
	if got <= 20 {
		t.Fatalf("EstimateChatPromptTokens() = %d, want tool schema tokens included above old fixed overhead", got)
	}
}

func TestTokenEstimator_EmbeddingAndRerankUseModelSpecificCounter(t *testing.T) {
	estimator := NewTokenEstimator()

	openAIEmbedding := estimator.EstimateEmbeddingTokens("antidisestablishmentarianism", "text-embedding-3-large")
	if openAIEmbedding != 6 {
		t.Fatalf("openai embedding tokens = %d, want tokenizer count 6", openAIEmbedding)
	}

	claudeEmbedding := estimator.EstimateEmbeddingTokens("你好世界", "claude-3-sonnet")
	if claudeEmbedding != 5 {
		t.Fatalf("claude embedding tokens = %d, want 5", claudeEmbedding)
	}

	geminiEmbedding := estimator.EstimateEmbeddingTokens("你好世界", "gemini-pro")
	if geminiEmbedding != 3 {
		t.Fatalf("gemini embedding tokens = %d, want 3", geminiEmbedding)
	}

	rerankTokens := estimator.EstimateRerankTokens("", []interface{}{map[string]interface{}{"text": "你好世界"}}, "gemini-pro")
	if rerankTokens != 3 {
		t.Fatalf("gemini rerank tokens = %d, want 3", rerankTokens)
	}
}

func TestTokenEstimator_EmbeddingTokenIDInputCountsTokens(t *testing.T) {
	estimator := NewTokenEstimator()

	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{name: "token ids", input: []int{1, 2, 3}, want: 3},
		{name: "token id batches", input: [][]int{{1, 2}, {3, 4, 5}}, want: 5},
		{name: "json token ids", input: []interface{}{float64(1), float64(2), float64(3)}, want: 3},
		{name: "json token id batches", input: []interface{}{
			[]interface{}{float64(1), float64(2)},
			[]interface{}{float64(3), float64(4), float64(5)},
		}, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimator.EstimateEmbeddingTokens(tt.input, "text-embedding-3-small")
			if got != tt.want {
				t.Fatalf("EstimateEmbeddingTokens(%#v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnsureEmbeddingUsage_EstimatesMissingUsage(t *testing.T) {
	resp := &adapter.EmbeddingsResponse{}

	actualTokens, estimated := ensureEmbeddingUsage(resp, 7)
	if !estimated {
		t.Fatal("estimated = false, want true")
	}
	if actualTokens != 7 {
		t.Fatalf("actualTokens = %d, want 7", actualTokens)
	}
	if resp.Usage.PromptTokens != 7 || resp.Usage.CompletionTokens != 0 || resp.Usage.TotalTokens != 7 {
		t.Fatalf("embedding usage = %+v, want prompt=7 completion=0 total=7", resp.Usage)
	}
}

func TestEnsureEmbeddingUsageForSelection_DoesNotEstimatePlatformUsage(t *testing.T) {
	resp := &adapter.EmbeddingsResponse{}
	selection := &ProviderSelection{UseSystemProvider: true}

	actualTokens, estimated := ensureEmbeddingUsageForSelection(selection, resp, 7)

	if estimated {
		t.Fatal("estimated = true, want false for platform channel")
	}
	if actualTokens != 0 {
		t.Fatalf("actualTokens = %d, want 0 when platform response has no usage", actualTokens)
	}
	if resp.Usage.PromptTokens != 0 || resp.Usage.TotalTokens != 0 {
		t.Fatalf("embedding usage = %+v, want untouched empty usage", resp.Usage)
	}
}

func TestEnsureEmbeddingUsage_NormalizesUpstreamTotalOnly(t *testing.T) {
	resp := &adapter.EmbeddingsResponse{
		Usage: adapter.Usage{TotalTokens: 11},
	}

	actualTokens, estimated := ensureEmbeddingUsage(resp, 7)
	if estimated {
		t.Fatal("estimated = true, want false")
	}
	if actualTokens != 11 {
		t.Fatalf("actualTokens = %d, want 11", actualTokens)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.TotalTokens != 11 {
		t.Fatalf("embedding usage = %+v, want prompt=11 total=11", resp.Usage)
	}
}

func TestEnsureEmbeddingUsageForSelection_KeepsPlatformUpstreamUsage(t *testing.T) {
	resp := &adapter.EmbeddingsResponse{
		Usage: adapter.Usage{TotalTokens: 11},
	}
	selection := &ProviderSelection{UseSystemProvider: true}

	actualTokens, estimated := ensureEmbeddingUsageForSelection(selection, resp, 7)

	if estimated {
		t.Fatal("estimated = true, want false for upstream usage")
	}
	if actualTokens != 11 {
		t.Fatalf("actualTokens = %d, want upstream total 11", actualTokens)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.TotalTokens != 11 {
		t.Fatalf("embedding usage = %+v, want normalized upstream total", resp.Usage)
	}
}

func TestEnsureRerankUsage_EstimatesMissingUsage(t *testing.T) {
	resp := &adapter.RerankResponse{}

	actualTokens, estimated := ensureRerankUsage(resp, 9)
	if !estimated {
		t.Fatal("estimated = false, want true")
	}
	if actualTokens != 9 {
		t.Fatalf("actualTokens = %d, want 9", actualTokens)
	}
	if resp.Usage == nil {
		t.Fatal("rerank usage = nil, want estimated usage")
	}
	if resp.Usage.PromptTokens != 9 || resp.Usage.CompletionTokens != 0 || resp.Usage.TotalTokens != 9 {
		t.Fatalf("rerank usage = %+v, want prompt=9 completion=0 total=9", resp.Usage)
	}
}

func TestEnsureRerankUsageForSelection_DoesNotEstimatePlatformUsage(t *testing.T) {
	resp := &adapter.RerankResponse{}
	selection := &ProviderSelection{UseSystemProvider: true}

	actualTokens, estimated := ensureRerankUsageForSelection(selection, resp, 9)

	if estimated {
		t.Fatal("estimated = true, want false for platform channel")
	}
	if actualTokens != 0 {
		t.Fatalf("actualTokens = %d, want 0 when platform response has no usage", actualTokens)
	}
	if resp.Usage != nil {
		t.Fatalf("rerank usage = %+v, want nil untouched usage", resp.Usage)
	}
}
