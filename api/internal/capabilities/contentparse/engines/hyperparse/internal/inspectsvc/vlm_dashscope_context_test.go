package inspectsvc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDashscopeChatCompletionHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
	}))
	defer server.Close()

	t.Setenv(EnvVLMAPIKey, "test-key")
	t.Setenv(EnvVLMBaseURL, server.URL)
	t.Setenv(EnvVLMModel, "test-model")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, _, err := dashscopeChatCompletionContext(ctx, []map[string]any{{"type": "text", "text": "hello"}})
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("request ignored context cancellation, elapsed=%s", elapsed)
	}
}
