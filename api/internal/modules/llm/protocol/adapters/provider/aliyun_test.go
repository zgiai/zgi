package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAliyunAdapterChatCompletion_UsesNativeTextGeneration(t *testing.T) {
	t.Helper()

	var (
		gotPath      string
		gotAuth      string
		gotAccept    string
		gotWorkspace string
		gotPayload   map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotWorkspace = r.Header.Get("X-DashScope-Workspace")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"request_id":"chat-1",
			"output":{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hello"}}]},
			"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v1",
		Headers: map[string]string{
			"Accept":                "application/problem+json",
			"X-DashScope-Workspace": "workspace-1",
		},
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "say hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/api/v1/services/aigc/text-generation/generation" {
		t.Fatalf("path = %q, want native text generation", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", gotAccept)
	}
	if gotWorkspace != "workspace-1" {
		t.Fatalf("X-DashScope-Workspace = %q, want workspace-1", gotWorkspace)
	}
	input, ok := gotPayload["input"].(map[string]any)
	if !ok {
		t.Fatalf("payload.input = %#v, want object", gotPayload["input"])
	}
	messages, ok := input["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("payload.input.messages = %#v, want one message", input["messages"])
	}
	message := messages[0].(map[string]any)
	if got := message["content"]; got != "say hello" {
		t.Fatalf("message.content = %#v, want text string", got)
	}
	if resp.Choices[0].Message.Content != "hello" {
		t.Fatalf("response content = %#v, want hello", resp.Choices[0].Message.Content)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v, want total_tokens=5", resp.Usage)
	}
}

func TestAliyunAdapterChatCompletion_UsesNativeMultimodalGeneration(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"output":{"choices":[{"message":{"content":"seen"}}]}}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "qwen-vl-max",
		Messages: []adapter.Message{{
			Role: "user",
			Content: []adapter.MessageContentPart{
				{Type: "text", Text: "describe"},
				{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "https://example.com/a.png"}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/api/v1/services/aigc/multimodal-generation/generation" {
		t.Fatalf("path = %q, want native multimodal generation", gotPath)
	}
	input := gotPayload["input"].(map[string]any)
	messages := input["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if got := content[0].(map[string]any)["text"]; got != "describe" {
		t.Fatalf("first content = %#v, want text", content[0])
	}
	if got := content[1].(map[string]any)["image"]; got != "https://example.com/a.png" {
		t.Fatalf("second content image = %#v, want image url", got)
	}
}

func TestAliyunAdapterChatCompletion_Qwen36UsesNativeMultimodalGeneration(t *testing.T) {
	for _, model := range []string{"qwen3.6-plus", "qwen3.6-plus-2026-04-02", "qwen3.6-flash", "qwen3.6-flash-2026-04-16"} {
		t.Run(model, func(t *testing.T) {
			var (
				gotPath    string
				gotPayload map[string]any
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()

				if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				gotPath = r.URL.Path

				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"output":{"choices":[{"message":{"role":"assistant","content":"ok"}}]}}`)
			}))
			defer server.Close()

			a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
			if err != nil {
				t.Fatalf("NewAliyunAdapter() error = %v", err)
			}

			_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
				Model:    model,
				Messages: []adapter.Message{{Role: "user", Content: "hello"}},
			})
			if err != nil {
				t.Fatalf("ChatCompletion() error = %v", err)
			}

			if gotPath != "/api/v1/services/aigc/multimodal-generation/generation" {
				t.Fatalf("path = %q, want native multimodal generation", gotPath)
			}
			input := gotPayload["input"].(map[string]any)
			messages := input["messages"].([]any)
			content := messages[0].(map[string]any)["content"].([]any)
			if got := content[0].(map[string]any)["text"]; got != "hello" {
				t.Fatalf("content text = %#v, want hello", got)
			}
		})
	}
}

func TestAliyunAdapterChatCompletion_AllowsDataURLImage(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"output":{"choices":[{"message":{"role":"assistant","content":"ok"}}]}}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "qwen-vl-max",
		Messages: []adapter.Message{{
			Role: "user",
			Content: []adapter.MessageContentPart{
				{Type: "text", Text: "describe"},
				{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "data:image/png;base64,abc"}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	input := gotPayload["input"].(map[string]any)
	messages := input["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if got := content[1].(map[string]any)["image"]; got != "data:image/png;base64,abc" {
		t.Fatalf("image = %#v, want data URL", got)
	}
}

func TestAliyunAdapterChatCompletion_RejectsLocalImageURL(t *testing.T) {
	t.Helper()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: "https://dashscope.aliyuncs.com/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "qwen-vl-max",
		Messages: []adapter.Message{{
			Role: "user",
			Content: []adapter.MessageContentPart{
				{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "http://localhost:3000/files/a.png"}},
			},
		}},
	})
	if err == nil {
		t.Fatal("ChatCompletion() error = nil, want local image URL rejection")
	}
}

func TestAliyunAdapterChatCompletionStream_UsesNativeSSE(t *testing.T) {
	t.Helper()

	var (
		gotAccept    string
		gotSSE       string
		gotPath      string
		gotWorkspace string
		gotPayload   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotAccept = r.Header.Get("Accept")
		gotSSE = r.Header.Get("X-DashScope-SSE")
		gotPath = r.URL.Path
		gotWorkspace = r.Header.Get("X-DashScope-Workspace")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"request_id\":\"s1\",\"output\":{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"he\"}}]}}\n\n")
		fmt.Fprint(w, "data: {\"request_id\":\"s1\",\"output\":{\"choices\":[{\"finish_reason\":\"stop\",\"message\":{\"role\":\"assistant\",\"content\":\"llo\"}}]},\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}\n\n")
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v1",
		Headers: map[string]string{
			"Accept":                "application/problem+json",
			"X-DashScope-SSE":       "disable",
			"X-DashScope-Workspace": "workspace-1",
		},
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "say hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var chunks []adapter.StreamResponse
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	if gotPath != "/api/v1/services/aigc/text-generation/generation" {
		t.Fatalf("path = %q, want native text generation", gotPath)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", gotAccept)
	}
	if gotSSE != "enable" {
		t.Fatalf("X-DashScope-SSE = %q, want enable", gotSSE)
	}
	if gotWorkspace != "workspace-1" {
		t.Fatalf("X-DashScope-Workspace = %q, want workspace-1", gotWorkspace)
	}
	if _, ok := gotPayload["stream"]; ok {
		t.Fatalf("payload.stream = %#v, want omitted", gotPayload["stream"])
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Content != "he" || chunks[1].Choices[0].Delta.Content != "llo" {
		t.Fatalf("chunks = %#v, want text deltas", chunks)
	}
	if chunks[2].Usage == nil || chunks[2].Usage.TotalTokens != 3 {
		t.Fatalf("final usage = %+v, want total_tokens=3", chunks[2].Usage)
	}
}
func TestAliyunAdapterChatCompletionStream_ParsesNativeOutputText(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"request_id\":\"s1\",\"output\":{\"text\":\"foo\"}}\n\n")
		fmt.Fprint(w, "data: {\"request_id\":\"s1\",\"output\":{\"text\":\"bar\"},\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}\n\n")
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "say hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var got string
	var usage *adapter.Usage
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("stream error = %v", chunk.Error)
		}
		if len(chunk.Choices) > 0 {
			if text, ok := chunk.Choices[0].Delta.Content.(string); ok {
				got += text
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	if got != "foobar" {
		t.Fatalf("stream text = %q, want foobar", got)
	}
	if usage == nil || usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v, want total_tokens=3", usage)
	}
}

func TestAliyunAdapterChatCompletionStream_ParsesNativeDeltaContent(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"request_id\":\"s1\",\"output\":{\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"foo\"}}]}}\n\n")
		fmt.Fprint(w, "data: {\"request_id\":\"s1\",\"output\":{\"choices\":[{\"delta\":{\"content\":\"bar\"}}]},\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}\n\n")
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "say hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var got string
	var usage *adapter.Usage
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("stream error = %v", chunk.Error)
		}
		if len(chunk.Choices) > 0 {
			if text, ok := chunk.Choices[0].Delta.Content.(string); ok {
				got += text
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	if got != "foobar" {
		t.Fatalf("stream text = %q, want foobar", got)
	}
	if usage == nil || usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v, want total_tokens=3", usage)
	}
}

func TestAliyunAdapterChatCompletion_RejectsUnsupportedContentPart(t *testing.T) {
	t.Helper()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: "https://dashscope.aliyuncs.com/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "qwen-vl-max",
		Messages: []adapter.Message{{
			Role:    "user",
			Content: []adapter.MessageContentPart{{Type: "audio_url"}},
		}},
	})
	if err == nil {
		t.Fatal("ChatCompletion() error = nil, want unsupported content type error")
	}
}
func TestAliyunAdapterCreateEmbeddings_UsesNativeTextEmbeddingEndpoint(t *testing.T) {
	t.Helper()

	var (
		gotAuth    string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"output":{"embeddings":[{"embedding":[0.1,0.2],"text_index":0}]},
			"usage":{"input_tokens":3,"total_tokens":3}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model: "text-embedding-v4",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotPath != "/api/v1/services/embeddings/text-embedding/text-embedding" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v1/services/embeddings/text-embedding/text-embedding")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if got := gotPayload["model"]; got != "text-embedding-v4" {
		t.Fatalf("payload.model = %#v, want %q", got, "text-embedding-v4")
	}
	input, ok := gotPayload["input"].(map[string]any)
	if !ok {
		t.Fatalf("payload.input = %#v, want object", gotPayload["input"])
	}
	texts, ok := input["texts"].([]any)
	if !ok || len(texts) != 1 || texts[0] != "hello" {
		t.Fatalf("payload.input.texts = %#v, want [hello]", input["texts"])
	}
	if resp.Model != "text-embedding-v4" {
		t.Fatalf("response model = %q, want %q", resp.Model, "text-embedding-v4")
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(response.data) = %d, want 1", len(resp.Data))
	}
}

func TestAliyunAdapterCreateEmbeddings_QwenVLEmbeddingUsesNativeMultimodalEndpoint(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"output":{"embeddings":[{"embedding":[0.1,0.2],"index":0},{"embedding":[0.3,0.4],"index":1}]},
			"usage":{"input_tokens":6,"total_tokens":6}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model: "qwen3-vl-embedding",
		Input: []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotPath != "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding" {
		t.Fatalf("path = %q, want multimodal embedding endpoint", gotPath)
	}
	input, ok := gotPayload["input"].(map[string]any)
	if !ok {
		t.Fatalf("payload.input = %#v, want object", gotPayload["input"])
	}
	contents, ok := input["contents"].([]any)
	if !ok || len(contents) != 2 {
		t.Fatalf("payload.input.contents = %#v, want two text contents", input["contents"])
	}
	first, ok := contents[0].(map[string]any)
	if !ok || first["text"] != "hello" {
		t.Fatalf("first content = %#v, want text hello", contents[0])
	}
	if len(resp.Data) != 2 || resp.Data[1].Index != 1 {
		t.Fatalf("response.data = %#v, want two indexed embeddings", resp.Data)
	}
}

func TestAliyunAdapterRerank_Qwen3Rerank_UsesCompatibleAPI(t *testing.T) {
	t.Helper()

	topN := 2
	var (
		gotPath    string
		gotAuth    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"rerank-1",
			"results":[
				{"index":1,"relevance_score":0.91,"document":"doc-b"},
				{"index":0,"relevance_score":0.42,"document":"doc-a"}
			],
			"usage":{"total_tokens":8}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:     "qwen3-rerank",
		Query:     "query",
		Documents: []string{"doc-a", "doc-b"},
		TopN:      &topN,
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if gotPath != "/compatible-api/v1/reranks" {
		t.Fatalf("path = %q, want %q", gotPath, "/compatible-api/v1/reranks")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if got := gotPayload["model"]; got != "qwen3-rerank" {
		t.Fatalf("payload.model = %#v, want %q", got, "qwen3-rerank")
	}
	if got := gotPayload["query"]; got != "query" {
		t.Fatalf("payload.query = %#v, want %q", got, "query")
	}
	if got := gotPayload["top_n"]; got != float64(2) {
		t.Fatalf("payload.top_n = %#v, want %v", got, 2)
	}
	documents, ok := gotPayload["documents"].([]any)
	if !ok || len(documents) != 2 {
		t.Fatalf("payload.documents = %#v, want two items", gotPayload["documents"])
	}
	if resp.Model != "qwen3-rerank" {
		t.Fatalf("response model = %q, want %q", resp.Model, "qwen3-rerank")
	}
	if len(resp.Results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(resp.Results))
	}
	if resp.Results[0].Index != 1 || resp.Results[0].Text != "doc-b" {
		t.Fatalf("results[0] = %#v, want index=1 text=%q", resp.Results[0], "doc-b")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 8 {
		t.Fatalf("usage = %+v, want total_tokens=8", resp.Usage)
	}
}
func TestAliyunAdapterRerank_GTERerank_UsesNativeAPI(t *testing.T) {
	t.Helper()

	topN := 1
	returnDocuments := true
	var (
		gotPath    string
		gotAuth    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"output":{
				"results":[
					{"index":0,"relevance_score":0.97,"document":"doc-a"}
				]
			},
			"usage":{"total_tokens":5}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:           "gte-rerank-v2",
		Query:           "query",
		Documents:       []string{"doc-a", "doc-b"},
		TopN:            &topN,
		ReturnDocuments: &returnDocuments,
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if gotPath != "/api/v1/services/rerank/text-rerank/text-rerank" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v1/services/rerank/text-rerank/text-rerank")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if got := gotPayload["model"]; got != "gte-rerank-v2" {
		t.Fatalf("payload.model = %#v, want %q", got, "gte-rerank-v2")
	}
	input, ok := gotPayload["input"].(map[string]any)
	if !ok {
		t.Fatalf("payload.input = %#v, want object", gotPayload["input"])
	}
	if got := input["query"]; got != "query" {
		t.Fatalf("payload.input.query = %#v, want %q", got, "query")
	}
	documents, ok := input["documents"].([]any)
	if !ok || len(documents) != 2 {
		t.Fatalf("payload.input.documents = %#v, want two items", input["documents"])
	}
	parameters, ok := gotPayload["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("payload.parameters = %#v, want object", gotPayload["parameters"])
	}
	if got := parameters["top_n"]; got != float64(1) {
		t.Fatalf("payload.parameters.top_n = %#v, want %v", got, 1)
	}
	if got := parameters["return_documents"]; got != true {
		t.Fatalf("payload.parameters.return_documents = %#v, want true", got)
	}
	if resp.Model != "gte-rerank-v2" {
		t.Fatalf("response model = %q, want %q", resp.Model, "gte-rerank-v2")
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(resp.Results))
	}
	if resp.Results[0].Text != "doc-a" {
		t.Fatalf("results[0] = %#v, want text=%q", resp.Results[0], "doc-a")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v, want total_tokens=5", resp.Usage)
	}
}

func TestAliyunAdapterListModels_UsesUpstreamModelsEndpointAndDocumentedCapabilityMapping(t *testing.T) {
	t.Helper()

	var (
		gotPath string
		gotAuth string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"object":"list",
			"data":[
				{"id":"qwen-plus","object":"model","owned_by":"dashscope"},
				{"id":"qwen-image-max","object":"model","owned_by":"dashscope"},
				{"id":"text-embedding-v4","object":"model","owned_by":"dashscope"},
				{"id":"qwen3-rerank","object":"model","owned_by":"dashscope"},
				{"id":"unknown-model","object":"model","owned_by":"dashscope"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if gotPath != "/compatible-mode/v1/models" {
		t.Fatalf("path = %q, want %q", gotPath, "/compatible-mode/v1/models")
	}
	if gotAuth != "Bearer runtime-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer runtime-key")
	}

	qwenPlus := findAliyunModel(t, models, "qwen-plus")
	if qwenPlus.Type != "chat" {
		t.Fatalf("qwen-plus type = %q, want %q", qwenPlus.Type, "chat")
	}
	if !containsAliyunValue(qwenPlus.Capabilities, "chat") || !containsAliyunValue(qwenPlus.Capabilities, "stream") {
		t.Fatalf("qwen-plus capabilities = %#v, want chat+stream", qwenPlus.Capabilities)
	}

	qwenImage := findAliyunModel(t, models, "qwen-image-max")
	if qwenImage.Type != "image" {
		t.Fatalf("qwen-image-max type = %q, want %q", qwenImage.Type, "image")
	}
	if !containsAliyunValue(qwenImage.Capabilities, "image") {
		t.Fatalf("qwen-image-max capabilities = %#v, want image", qwenImage.Capabilities)
	}

	embedding := findAliyunModel(t, models, "text-embedding-v4")
	if embedding.Type != "embedding" {
		t.Fatalf("text-embedding-v4 type = %q, want %q", embedding.Type, "embedding")
	}
	if !containsAliyunValue(embedding.Capabilities, "embedding") {
		t.Fatalf("text-embedding-v4 capabilities = %#v, want embedding", embedding.Capabilities)
	}

	rerank := findAliyunModel(t, models, "qwen3-rerank")
	if rerank.Type != "rerank" {
		t.Fatalf("qwen3-rerank type = %q, want %q", rerank.Type, "rerank")
	}
	if !containsAliyunValue(rerank.Capabilities, "rerank") {
		t.Fatalf("qwen3-rerank capabilities = %#v, want rerank", rerank.Capabilities)
	}

	unknown := findAliyunModel(t, models, "unknown-model")
	if len(unknown.Capabilities) != 0 {
		t.Fatalf("unknown-model capabilities = %#v, want empty for undocumented model", unknown.Capabilities)
	}
}

func TestAliyunAdapterListModels_FallbackDoesNotReturnEmptyOnUnsupportedModelsEndpoint(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"not found","code":"NOT_FOUND"}}`, http.StatusNotFound)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) == 0 {
		t.Fatal("ListModels() returned empty catalog, want documented fallback models")
	}

	if !containsAliyunValue(findAliyunModel(t, models, "qwen-plus").Capabilities, "chat") {
		t.Fatalf("fallback qwen-plus capabilities = %#v, want chat", findAliyunModel(t, models, "qwen-plus").Capabilities)
	}
	if !containsAliyunValue(findAliyunModel(t, models, "text-embedding-v4").Capabilities, "embedding") {
		t.Fatalf("fallback text-embedding-v4 capabilities = %#v, want embedding", findAliyunModel(t, models, "text-embedding-v4").Capabilities)
	}
	if !containsAliyunValue(findAliyunModel(t, models, "qwen3-rerank").Capabilities, "rerank") {
		t.Fatalf("fallback qwen3-rerank capabilities = %#v, want rerank", findAliyunModel(t, models, "qwen3-rerank").Capabilities)
	}
}

func TestAliyunAdapterGetProviderInfo_DeclaresDocumentedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://dashscope.aliyuncs.com/api/v1",
	})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}

	if !containsAliyunValue(info.Capabilities, "chat") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "chat")
	}
	if !containsAliyunValue(info.Capabilities, "stream") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "stream")
	}
	if !containsAliyunValue(info.Capabilities, "image") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "image")
	}
	if !containsAliyunValue(info.Capabilities, "embedding") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "embedding")
	}
	if !containsAliyunValue(info.Capabilities, "rerank") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "rerank")
	}
	if !containsAliyunValue(info.Capabilities, "model_listing") {
		t.Fatalf("Capabilities = %#v, want to contain %q", info.Capabilities, "model_listing")
	}
}

func findAliyunModel(t *testing.T, models []adapter.Model, id string) adapter.Model {
	t.Helper()

	for _, model := range models {
		if model.ID == id {
			return model
		}
	}

	t.Fatalf("model %q not found in %#v", id, models)
	return adapter.Model{}
}

func containsAliyunValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAliyunAdapterChatCompletion_PreservesToolCallMessages(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"output":{"choices":[{"message":{"role":"assistant","content":"ok"}}]}}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	idx := 0
	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "qwen-plus",
		Messages: []adapter.Message{
			{Role: "assistant", Content: "", ToolCalls: []adapter.ToolCall{{Index: &idx, ID: "call_1", Type: "function", Function: adapter.FunctionCall{Name: "lookup", Arguments: `{"city":"hz"}`}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "sunny"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	input := gotPayload["input"].(map[string]any)
	messages := input["messages"].([]any)
	assistantMsg := messages[0].(map[string]any)
	toolCalls, ok := assistantMsg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("assistant tool_calls = %#v, want one tool call", assistantMsg["tool_calls"])
	}
	function := toolCalls[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "lookup" || function["arguments"] != `{"city":"hz"}` {
		t.Fatalf("tool function = %#v, want lookup args", function)
	}
	toolMsg := messages[1].(map[string]any)
	if toolMsg["tool_call_id"] != "call_1" {
		t.Fatalf("tool_call_id = %#v, want call_1", toolMsg["tool_call_id"])
	}
}

func TestAliyunAdapterChatCompletion_ParsesToolCalls(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"request_id":"chat-tool-1",
			"output":{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"hz\"}"}}]}}]},
			"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "lookup"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("tool calls = %#v, want one", calls)
	}
	if calls[0].ID != "call_1" || calls[0].Type != "function" || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments != `{"city":"hz"}` {
		t.Fatalf("tool call = %#v, want full function call", calls[0])
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
}

func TestAliyunAdapterChatCompletionStream_ParsesToolCallDelta(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data:"+`{"request_id":"s-tool-1","output":{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\""}}]}}]}}`+"\n\n")
		fmt.Fprint(w, "data:"+`{"request_id":"s-tool-1","output":{"choices":[{"finish_reason":"tool_calls","delta":{"tool_calls":[{"index":0,"function":{"arguments":"hz\"}"}}]}}]},"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`+"\n\n")
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "lookup"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var calls []adapter.ToolCall
	var arguments string
	var finishReason string
	var usage *adapter.Usage
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("stream error = %v", chunk.Error)
		}
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			finishReason = choice.FinishReason
			if len(choice.Delta.ToolCalls) > 0 {
				calls = append(calls, choice.Delta.ToolCalls...)
				arguments += choice.Delta.ToolCalls[0].Function.Arguments
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	if len(calls) != 2 {
		t.Fatalf("tool call delta count = %d, want 2", len(calls))
	}
	if calls[0].ID != "call_1" || calls[0].Type != "function" || calls[0].Function.Name != "lookup" {
		t.Fatalf("first tool call delta = %#v, want id/type/name", calls[0])
	}
	if arguments != `{"city":"hz"}` {
		t.Fatalf("tool arguments = %q, want JSON args", arguments)
	}
	if finishReason != "tool_calls" {
		t.Fatalf("finish reason = %q, want tool_calls", finishReason)
	}
	if usage == nil || usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v, want total_tokens=3", usage)
	}
}

func TestAliyunAdapterChatCompletion_MapsNativeParameters(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"output":{"choices":[{"message":{"role":"assistant","content":"ok"}}]}}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	presencePenalty := 0.3
	frequencyPenalty := 0.4
	seed := 123
	_, err = a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:            "qwen-plus",
		Messages:         []adapter.Message{{Role: "user", Content: "say hello"}},
		PresencePenalty:  &presencePenalty,
		FrequencyPenalty: &frequencyPenalty,
		Seed:             &seed,
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	parameters := gotPayload["parameters"].(map[string]any)
	if got := parameters["presence_penalty"]; got != presencePenalty {
		t.Fatalf("presence_penalty = %#v, want %v", got, presencePenalty)
	}
	if got := parameters["frequency_penalty"]; got != frequencyPenalty {
		t.Fatalf("frequency_penalty = %#v, want %v", got, frequencyPenalty)
	}
	if got := parameters["seed"]; got != float64(seed) {
		t.Fatalf("seed = %#v, want %d", got, seed)
	}
}

func TestAliyunAdapterChatCompletion_ParsesReasoningContent(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"request_id":"chat-reasoning-1",
			"output":{"choices":[{"message":{"role":"assistant","reasoning_content":"think","content":"answer"}}]}
		}`)
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if got := resp.Choices[0].Message.ReasoningContent; got != "think" {
		t.Fatalf("reasoning_content = %q, want think", got)
	}
	if got := resp.Choices[0].Message.Content; got != "answer" {
		t.Fatalf("content = %#v, want answer", got)
	}
}

func TestAliyunAdapterChatCompletionStream_ParsesReasoningContentDelta(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"request_id\":\"s-reasoning-1\",\"output\":{\"choices\":[{\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"think\"}}]}}\n\n")
		fmt.Fprint(w, "data: {\"request_id\":\"s-reasoning-1\",\"output\":{\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}}\n\n")
	}))
	defer server.Close()

	a, err := NewAliyunAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL + "/api/v1"})
	if err != nil {
		t.Fatalf("NewAliyunAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "question"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var reasoning string
	var content string
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatalf("stream error = %v", chunk.Error)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		reasoning += choice.Delta.ReasoningContent
		if text, ok := choice.Delta.Content.(string); ok {
			content += text
		}
	}

	if reasoning != "think" {
		t.Fatalf("reasoning_content = %q, want think", reasoning)
	}
	if content != "answer" {
		t.Fatalf("content = %q, want answer", content)
	}
}
