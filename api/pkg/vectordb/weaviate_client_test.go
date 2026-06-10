package vectordb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"matches":2,"successful":2,"failed":0}}`))
	}))
	defer server.Close()

	client := NewWeaviateClient(&config.VectorStoreConfig{
		WeaviateEndpoint: server.URL,
		WeaviateAPIKey:   "secret",
	})

	if err := client.DeleteObjectsByField(context.Background(), "Dataset_1", "document_id", "doc-1"); err != nil {
		t.Fatalf("DeleteObjectsByField returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotPath != "/v1/batch/objects/delete" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("authorization = %q", gotAuth)
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
