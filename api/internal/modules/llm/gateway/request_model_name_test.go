package gateway

import (
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestValidateRequest_TrimsModelButPreservesFullIdentifier(t *testing.T) {
	svc := &llmGatewayServiceImpl{}

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
			name:  "cursor prefix maps to canonical model",
			input: " cursor-gpt-5 ",
			want:  "gpt-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &adapter.ChatRequest{
				Model: tt.input,
				Messages: []adapter.Message{{
					Role:    "user",
					Content: "hi",
				}},
			}

			if err := svc.validateRequest(req); err != nil {
				t.Fatalf("validateRequest() error = %v", err)
			}
			if req.Model != tt.want {
				t.Fatalf("req.Model = %q, want %q", req.Model, tt.want)
			}
		})
	}
}

func TestValidateImageRequest_TrimsModelButPreservesFullIdentifier(t *testing.T) {
	svc := &llmGatewayServiceImpl{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single slash image model keeps full name",
			input: " Qwen/Qwen-Image ",
			want:  "Qwen/Qwen-Image",
		},
		{
			name:  "multi slash image model keeps full name",
			input: " Pro/qwen/team/Qwen-Image ",
			want:  "Pro/qwen/team/Qwen-Image",
		},
		{
			name:  "native short image model still works",
			input: " gpt-image-1 ",
			want:  "gpt-image-1",
		},
		{
			name:  "cursor prefix maps image model",
			input: " cursor-gpt-image-1 ",
			want:  "gpt-image-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &adapter.ImageRequest{
				Model:  tt.input,
				Prompt: "draw a cat",
			}

			if err := svc.validateImageRequest(req); err != nil {
				t.Fatalf("validateImageRequest() error = %v", err)
			}
			if req.Model != tt.want {
				t.Fatalf("req.Model = %q, want %q", req.Model, tt.want)
			}
		})
	}
}
