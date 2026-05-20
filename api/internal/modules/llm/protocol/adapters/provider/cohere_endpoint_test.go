package provider

import (
	"testing"
	"time"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestCohereAdapterEndpoint_CompatWithVersionedBaseURL(t *testing.T) {
	a, err := NewCohereAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.cohere.com/v2",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewCohereAdapter() error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "chat endpoint does not duplicate v2",
			path: "/v2/chat",
			want: "https://api.cohere.com/v2/chat",
		},
		{
			name: "embed endpoint does not duplicate v2",
			path: "/v2/embed",
			want: "https://api.cohere.com/v2/embed",
		},
		{
			name: "models endpoint can still use v1",
			path: "/v1/models",
			want: "https://api.cohere.com/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.endpoint(tt.path)
			if got != tt.want {
				t.Fatalf("endpoint(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
