package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestAliyunAdapterCreateEmbeddings_UsesOpenAICompatibleEndpoint(t *testing.T) {
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
			"object":"list",
			"data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],
			"model":"text-embedding-v4",
			"usage":{"prompt_tokens":3,"total_tokens":3}
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

	if gotPath != "/compatible-mode/v1/embeddings" {
		t.Fatalf("path = %q, want %q", gotPath, "/compatible-mode/v1/embeddings")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if got := gotPayload["model"]; got != "text-embedding-v4" {
		t.Fatalf("payload.model = %#v, want %q", got, "text-embedding-v4")
	}
	if got := gotPayload["input"]; got != "hello" {
		t.Fatalf("payload.input = %#v, want %q", got, "hello")
	}
	if resp.Model != "text-embedding-v4" {
		t.Fatalf("response model = %q, want %q", resp.Model, "text-embedding-v4")
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(response.data) = %d, want 1", len(resp.Data))
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
