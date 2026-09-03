package exa

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type transport func(*http.Request) (*http.Response, error)

func (run transport) RoundTrip(r *http.Request) (*http.Response, error) { return run(r) }
func TestNativeExaSearchContentsAndVerification(t *testing.T) {
	calls := 0
	client, err := New(&http.Client{Transport: transport(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("x-api-key") != "fixture-key" {
			t.Error("missing key")
		}
		var body map[string]any
		if json.NewDecoder(request.Body).Decode(&body) != nil {
			t.Fatal("request encoding")
		}
		if request.URL.Path == "/contents" {
			if body["maxAgeHours"] != float64(0) {
				t.Error("freshness ignored")
			}
			if _, ok := body["urls"]; !ok {
				t.Error("missing urls")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"requestId":"r-1","resolvedSearchType":"auto","costDollars":{"total":0.01},"results":[{"url":"https://example.com/page","title":"Result","text":"Content","highlights":["Summary"]}],"statuses":[{"id":"https://example.com/missing","status":"error","error":{"tag":"CRAWL_NOT_FOUND"}}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := client.Verify(context.Background(), "fixture-key")
	if err != nil || verification.AccountID != "" || len(verification.Operations) != 2 {
		t.Fatalf("verification %v", err)
	}
	for _, test := range []struct {
		op    string
		input map[string]any
	}{
		{"web.search", map[string]any{"query": "public documentation"}},
		{"web.fetch", map[string]any{"urls": []string{"https://example.com/page", "https://example.com/missing"}, "freshness": "force_live"}},
	} {
		result, err := client.Invoke(context.Background(), test.op, "fixture-key", test.input)
		if err != nil {
			t.Fatal(err)
		}
		schema, _ := providers.NewSchemas("web-search")
		if schema.Output(test.op, result.Value) != nil {
			t.Fatalf("invalid output: %#v", result.Value)
		}
	}
	if calls != 3 {
		t.Fatalf("unexpected requests %d", calls)
	}
}
func TestNativeExaRejectsUnsafeURLsAndDoesNotRetry(t *testing.T) {
	calls := 0
	client, _ := New(&http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"fixture-key"}`))}, nil
	})})
	for _, address := range []string{"http://127.0.0.1/private", "http://169.254.169.254/latest", "file:///etc/passwd", "https://user:pass@example.com/"} {
		_, err := client.Invoke(context.Background(), "web.fetch", "fixture-key", map[string]any{"urls": []string{address}})
		if err == nil || calls != 0 {
			t.Fatal("unsafe URL submitted")
		}
	}
	_, err := client.Invoke(context.Background(), "web.search", "fixture-key", map[string]any{"query": "safe"})
	if err == nil || !providers.FailureOf(err).Retryable || calls != 1 || strings.Contains(err.Error(), "fixture-key") {
		t.Fatal("unsafe throttling behavior")
	}
}
