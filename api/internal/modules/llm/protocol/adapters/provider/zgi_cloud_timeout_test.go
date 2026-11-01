package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/requesttimeout"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Without a configured timeout the Console hop follows the request default
// (1200s), not the old fixed 120s.
func TestZGICloudAdapterDefaultTimeoutFollowsRequestDefault(t *testing.T) {
	cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		ProviderName: zgiCloudAdapterName,
		BaseURL:      "http://console.internal/v1/internal",
		AuthHook:     func(*http.Request) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := cloud.httpClient.StandardClient().Timeout; got != requesttimeout.Default() {
		t.Fatalf("zgi-cloud timeout = %s, want %s", got, requesttimeout.Default())
	}
}

// A direct provider adapter never sends the caller's timeout header, even when
// the request context carries one: only the Console auth hook adds it.
func TestZGICloudTimeoutHeaderIsNotSentByDirectAdapters(t *testing.T) {
	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seen <- r.Header.Get(requesttimeout.HeaderName):
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	direct, err := NewOpenAIAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: server.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := requesttimeout.WithTimeout(context.Background(), 2400*time.Second)
	if _, err := direct.ChatCompletion(ctx, &adapter.ChatRequest{
		Model:    "m",
		Messages: []adapter.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if got := <-seen; got != "" {
		t.Fatalf("direct provider saw %s = %q", requesttimeout.HeaderName, got)
	}
}
