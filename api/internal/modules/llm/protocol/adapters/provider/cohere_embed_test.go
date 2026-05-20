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

func TestCohereAdapterCreateEmbeddings_TextRequest_UsesEmbedParameters(t *testing.T) {
	t.Helper()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/embed" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v2/embed")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		fmt.Fprint(w, `{
			"id":"embed-1",
			"embeddings":{"float":[[0.1,0.2,0.3]]},
			"texts":["hello"],
			"meta":{"billed_units":{"input_tokens":11}}
		}`)
	}))
	defer server.Close()

	var req adapter.EmbeddingsRequest
	if err := json.Unmarshal([]byte(`{
		"model":"embed-v4.0",
		"input":"hello",
		"input_type":"search_query",
		"dimensions":1024,
		"truncate":"END",
		"max_tokens":256
	}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	a, err := NewCohereAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewCohereAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &req)
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if got["input_type"] != "search_query" {
		t.Fatalf("input_type = %#v, want %q", got["input_type"], "search_query")
	}
	if got["output_dimension"] != float64(1024) {
		t.Fatalf("output_dimension = %#v, want %v", got["output_dimension"], 1024)
	}
	if got["truncate"] != "END" {
		t.Fatalf("truncate = %#v, want %q", got["truncate"], "END")
	}
	if got["max_tokens"] != float64(256) {
		t.Fatalf("max_tokens = %#v, want %v", got["max_tokens"], 256)
	}
	if texts, ok := got["texts"].([]any); !ok || len(texts) != 1 || texts[0] != "hello" {
		t.Fatalf("texts = %#v, want [\"hello\"]", got["texts"])
	}
	if _, ok := got["images"]; ok {
		t.Fatalf("images = %#v, want omitted", got["images"])
	}
	if _, ok := got["inputs"]; ok {
		t.Fatalf("inputs = %#v, want omitted", got["inputs"])
	}
	if embeddingTypes, ok := got["embedding_types"].([]any); !ok || len(embeddingTypes) != 1 || embeddingTypes[0] != "float" {
		t.Fatalf("embedding_types = %#v, want [\"float\"]", got["embedding_types"])
	}

	if len(resp.Data) != 1 || len(resp.Data[0].Embedding) != 3 {
		t.Fatalf("response embeddings = %#v, want one 3-d vector", resp.Data)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.TotalTokens != 11 {
		t.Fatalf("usage = %+v, want prompt=11 total=11", resp.Usage)
	}
}

func TestCohereAdapterCreateEmbeddings_UsesImagesPayloadForImageInput(t *testing.T) {
	t.Helper()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		fmt.Fprint(w, `{"id":"embed-2","embeddings":{"float":[[0.1,0.2]]},"meta":{"billed_units":{"input_tokens":7}}}`)
	}))
	defer server.Close()

	var req adapter.EmbeddingsRequest
	if err := json.Unmarshal([]byte(`{
		"model":"embed-v4.0",
		"input":["data:image/png;base64,aGVsbG8="],
		"input_type":"image"
	}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	a, err := NewCohereAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewCohereAdapter() error = %v", err)
	}

	if _, err := a.CreateEmbeddings(context.Background(), &req); err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if got["input_type"] != "image" {
		t.Fatalf("input_type = %#v, want %q", got["input_type"], "image")
	}
	if images, ok := got["images"].([]any); !ok || len(images) != 1 || images[0] != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("images = %#v, want one data URI", got["images"])
	}
	if _, ok := got["texts"]; ok {
		t.Fatalf("texts = %#v, want omitted", got["texts"])
	}
}

func TestCohereAdapterCreateEmbeddings_UsesInputsPayloadForMixedContent(t *testing.T) {
	t.Helper()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		fmt.Fprint(w, `{"id":"embed-3","embeddings":{"float":[[0.1,0.2]]},"meta":{"billed_units":{"input_tokens":9}}}`)
	}))
	defer server.Close()

	var req adapter.EmbeddingsRequest
	if err := json.Unmarshal([]byte(`{
		"model":"embed-v4.0",
		"input":[
			{
				"content":[
					{"type":"text","text":"hello"},
					{"type":"image","image":"data:image/png;base64,aGVsbG8="}
				]
			}
		],
		"input_type":"classification"
	}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	a, err := NewCohereAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewCohereAdapter() error = %v", err)
	}

	if _, err := a.CreateEmbeddings(context.Background(), &req); err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if got["input_type"] != "classification" {
		t.Fatalf("input_type = %#v, want %q", got["input_type"], "classification")
	}
	if inputs, ok := got["inputs"].([]any); !ok || len(inputs) != 1 {
		t.Fatalf("inputs = %#v, want one mixed input object", got["inputs"])
	}
	if _, ok := got["texts"]; ok {
		t.Fatalf("texts = %#v, want omitted", got["texts"])
	}
	if _, ok := got["images"]; ok {
		t.Fatalf("images = %#v, want omitted", got["images"])
	}
}
