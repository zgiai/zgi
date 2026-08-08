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

func TestQwenLiveEmbeddingAndRerankCapabilities(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	if apiKey == "" {
		t.Fatal("DASHSCOPE_API_KEY is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("DASHSCOPE_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: apiKey, BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}

	for _, model := range []string{"text-embedding-v4", "qwen3.7-text-embedding", "qwen3-vl-embedding"} {
		t.Run(model, func(t *testing.T) {
			started := time.Now()
			resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
				Model: model, Input: "semantic retrieval test", Dimensions: 256, InputType: "query",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Data) != 1 || len(resp.Data[0].Embedding) == 0 {
				t.Fatalf("empty embedding response: %#v", resp)
			}
			t.Logf("duration=%s dimension=%d tokens=%d", time.Since(started).Round(time.Millisecond), len(resp.Data[0].Embedding), resp.Usage.TotalTokens)
		})
	}

	for _, model := range []string{"qwen3-rerank", "qwen3-vl-rerank"} {
		t.Run(model, func(t *testing.T) {
			started := time.Now()
			topN := 2
			returnDocuments := true
			resp, err := a.Rerank(context.Background(), &adapter.RerankRequest{
				Model: model,
				Query: "What is a reranking model?",
				Documents: []string{
					"Reranking scores candidates by relevance.",
					"Quantum computing uses qubits.",
					"A reranker improves retrieval precision.",
				},
				TopN: &topN, ReturnDocuments: &returnDocuments,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Results) != 2 {
				t.Fatalf("results=%d, want 2", len(resp.Results))
			}
			t.Logf("duration=%s top_index=%d top_score=%.4f", time.Since(started).Round(time.Millisecond), resp.Results[0].Index, resp.Results[0].RelevanceScore)
		})
	}
}
