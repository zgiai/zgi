package inspectsvc

import (
	"context"
	"testing"
)

func TestDashscopeModelForRequestUsesPreferredFastModel(t *testing.T) {
	t.Setenv(EnvVLMModel, "qwen-vl-max-latest")
	t.Setenv(EnvVLMModelFast, "qwen-vl-plus")

	if got := dashscopeModelForRequest(VLMModelFast()); got != "qwen-vl-plus" {
		t.Fatalf("model=%q", got)
	}
}

func TestDashscopeModelForRequestFallsBackToMainModel(t *testing.T) {
	t.Setenv(EnvVLMModel, "qwen-vl-max-latest")
	t.Setenv(EnvVLMModelFast, "")

	if got := dashscopeModelForRequest(VLMModelFast()); got != "qwen-vl-max-latest" {
		t.Fatalf("model=%q", got)
	}
}

func TestDashscopeModelForRequestRequiresExplicitModel(t *testing.T) {
	t.Setenv(EnvVLMModel, "")
	t.Setenv(EnvVLMModelFast, "")
	t.Setenv(EnvLegacyModel, "")
	t.Setenv(EnvGeminiModel, "")

	if got := dashscopeModelForRequest(VLMModelFast()); got != "" {
		t.Fatalf("model=%q", got)
	}
}

func TestDashscopeChatCompletionRequiresExplicitBaseURL(t *testing.T) {
	t.Setenv(EnvVLMAPIKey, "key")
	t.Setenv(EnvLegacyAPIKey, "")
	t.Setenv(EnvGeminiAPIKey, "")
	t.Setenv(EnvVLMBaseURL, "")
	t.Setenv(EnvLegacyBaseURL, "")
	t.Setenv(EnvGeminiBaseURL, "")
	t.Setenv(EnvVLMModel, "model")
	t.Setenv(EnvLegacyModel, "")
	t.Setenv(EnvGeminiModel, "")

	_, _, err := dashscopeChatCompletionWithModelContext(context.Background(), []map[string]any{}, "")
	if err == nil {
		t.Fatal("expected missing base URL error")
	}
}
