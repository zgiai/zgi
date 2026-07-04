package vectordb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
)

func TestWeaviateDeleteVector(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{
		WeaviateEndpoint: server.URL,
		WeaviateAPIKey:   "secret",
	})

	if err := client.DeleteVector(context.Background(), "node-1", "Dataset_1"); err != nil {
		t.Fatalf("DeleteVector returned error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if gotPath != "/v1/objects/Dataset_1/node-1" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
}

func TestWeaviateDeleteVectorTreatsNotFoundAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{WeaviateEndpoint: server.URL})

	if err := client.DeleteVector(context.Background(), "missing-node", "Dataset_1"); err != nil {
		t.Fatalf("DeleteVector returned error for not found: %v", err)
	}
}

func TestWeaviateDeleteObjectsByFieldUsesBatchDeleteEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var payload map[string]interface{}
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"results":{"matches":2,"successful":2,"failed":0}}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":{"matches":0,"successful":0,"failed":0}}`))
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{
		WeaviateEndpoint: server.URL,
		WeaviateAPIKey:   "secret",
	})

	if err := client.DeleteObjectsByField(context.Background(), "Dataset_1", "document_id", "doc-1"); err != nil {
		t.Fatalf("DeleteObjectsByField returned error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if gotPath != "/v1/batch/objects" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
	match, ok := payload["match"].(map[string]interface{})
	if !ok {
		t.Fatalf("match payload missing: %#v", payload)
	}
	if match["class"] != "Dataset_1" {
		t.Fatalf("class = %#v", match["class"])
	}
	where, ok := match["where"].(map[string]interface{})
	if !ok {
		t.Fatalf("where payload missing: %#v", match)
	}
	if where["operator"] != "Equal" {
		t.Fatalf("operator = %#v", where["operator"])
	}
	if where["valueText"] != "doc-1" {
		t.Fatalf("valueText = %#v", where["valueText"])
	}
	path, ok := where["path"].([]interface{})
	if !ok || len(path) != 1 || path[0] != "document_id" {
		t.Fatalf("path = %#v", where["path"])
	}
}

func TestWeaviateDeleteObjectsByFieldReportsFailedObjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"matches":2,"successful":1,"failed":1}}`))
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{WeaviateEndpoint: server.URL})

	if err := client.DeleteObjectsByField(context.Background(), "Dataset_1", "document_id", "doc-1"); err == nil {
		t.Fatalf("expected failed object count to return an error")
	}
}

func TestWeaviateDeleteObjectsByFieldRepeatsUntilNoMatches(t *testing.T) {
	responses := []string{
		`{"results":{"matches":10000,"successful":10000,"failed":0}}`,
		`{"results":{"matches":5,"successful":5,"failed":0}}`,
		`{"results":{"matches":0,"successful":0,"failed":0}}`,
	}
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %q, want %q", r.Method, http.MethodDelete)
		}
		if r.URL.Path != "/v1/batch/objects" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[requestCount]))
		requestCount++
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{WeaviateEndpoint: server.URL})

	if err := client.DeleteObjectsByField(context.Background(), "Dataset_1", "document_id", "doc-1"); err != nil {
		t.Fatalf("DeleteObjectsByField returned error: %v", err)
	}
	if requestCount != len(responses) {
		t.Fatalf("request count = %d, want %d", requestCount, len(responses))
	}
}

func TestWeaviateDeleteObjectsByFieldTreatsMissingClassAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":[{"message":"class Dataset_1 does not exist"}]}`))
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{WeaviateEndpoint: server.URL})

	if err := client.DeleteObjectsByField(context.Background(), "Dataset_1", "document_id", "doc-1"); err != nil {
		t.Fatalf("DeleteObjectsByField should treat missing class as clean: %v", err)
	}
}

func TestWeaviateDeleteObjectsByFieldDoesNotTreatEndpointNotFoundAsMissingClass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`404 page not found`))
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{WeaviateEndpoint: server.URL})

	if err := client.DeleteObjectsByField(context.Background(), "Dataset_1", "document_id", "doc-1"); err == nil {
		t.Fatalf("endpoint 404 should return an error")
	}
}

func TestWeaviateSearchByFullTextRequestsBM25Score(t *testing.T) {
	var graphQLBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/schema":
			_, _ = w.Write([]byte(`{"classes":[{"class":"Dataset_1"}]}`))
		case "/v1/graphql":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read graphql body: %v", err)
			}
			graphQLBody = string(body)
			_, _ = w.Write([]byte(`{
				"data": {
					"Get": {
						"Dataset_1": [
							{
								"_additional": {"id": "chunk-1", "score": 7.25},
								"doc_id": "chunk-1",
								"dataset_id": "dataset-1",
								"document_id": "document-1",
								"doc_hash": "hash-1",
								"text": "invoice 2026"
							}
						]
					}
				}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{WeaviateEndpoint: server.URL})
	results, err := client.SearchByFullText(context.Background(), "Dataset_1", "invoice", 3)
	if err != nil {
		t.Fatalf("SearchByFullText returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if !strings.Contains(graphQLBody, "score") {
		t.Fatalf("graphql query did not request _additional.score: %s", graphQLBody)
	}
	if strings.Contains(graphQLBody, "vector") {
		t.Fatalf("graphql query should not request vector payload for BM25: %s", graphQLBody)
	}
	additional, ok := results[0]["_additional"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing _additional in result: %#v", results[0])
	}
	if additional["score"] != 7.25 {
		t.Fatalf("score = %#v, want 7.25", additional["score"])
	}
}
