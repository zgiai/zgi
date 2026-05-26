package vectordb

import (
	"context"
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
