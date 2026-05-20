package gateway

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestNormalizeRequestedModelName_TrimAndCursorPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single slash model keeps full name",
			input: " ByteDance-Seed/Seed-OSS-36B-Instruct ",
			want:  "ByteDance-Seed/Seed-OSS-36B-Instruct",
		},
		{
			name:  "multi slash model keeps full name",
			input: " Pro/deepseek-ai/DeepSeek-V3 ",
			want:  "Pro/deepseek-ai/DeepSeek-V3",
		},
		{
			name:  "native short model still works",
			input: " deepseek-chat ",
			want:  "deepseek-chat",
		},
		{
			name:  "cursor prefix strips once",
			input: "cursor-gpt-5",
			want:  "gpt-5",
		},
		{
			name:  "cursor prefix keeps inner cursor token",
			input: "cursor-cursor-gpt-5",
			want:  "cursor-gpt-5",
		},
		{
			name:  "cursor prefix with whitespace",
			input: " cursor-gpt-5 ",
			want:  "gpt-5",
		},
		{
			name:  "contains cursor but not prefix",
			input: "my-cursor-gpt-5",
			want:  "my-cursor-gpt-5",
		},
		{
			name:  "cursor prefix only becomes empty model",
			input: "cursor-",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRequestedModelName(tt.input); got != tt.want {
				t.Fatalf("normalizeRequestedModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCloneChatRequestWithNormalizedModel_MapsCursorPrefix(t *testing.T) {
	req := &adapter.ChatRequest{
		Model: " cursor-gpt-5 ",
	}

	normalized := cloneChatRequestWithNormalizedModel(req)

	if normalized.Model != "gpt-5" {
		t.Fatalf("normalized.Model = %q, want %q", normalized.Model, "gpt-5")
	}
	if req.Model != " cursor-gpt-5 " {
		t.Fatalf("original request model was mutated: %q", req.Model)
	}
}
