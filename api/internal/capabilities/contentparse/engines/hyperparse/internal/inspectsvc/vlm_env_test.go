package inspectsvc

import "testing"

func TestVLMAPIKeyPreferPrimary(t *testing.T) {
	t.Setenv(EnvVLMAPIKey, "primary-key")
	t.Setenv(EnvLegacyAPIKey, "legacy-key")
	if g := VLMAPIKey(); g != "primary-key" {
		t.Fatalf("want primary, got %q", g)
	}
}

func TestVLMAPIKeyFallbackLegacy(t *testing.T) {
	t.Setenv(EnvVLMAPIKey, "")
	t.Setenv(EnvLegacyAPIKey, "legacy-only")
	t.Setenv(EnvGeminiAPIKey, "gemini-key")
	if g := VLMAPIKey(); g != "legacy-only" {
		t.Fatalf("want legacy, got %q", g)
	}
}

func TestVLMAPIKeyFallbackGemini(t *testing.T) {
	t.Setenv(EnvVLMAPIKey, "")
	t.Setenv(EnvLegacyAPIKey, "")
	t.Setenv(EnvGeminiAPIKey, "gemini-only")
	if g := VLMAPIKey(); g != "gemini-only" {
		t.Fatalf("want gemini fallback, got %q", g)
	}
	if !VLMUsingGeminiCompatConfig() {
		t.Fatal("want gemini compat config when only GEMINI_API_KEY is set")
	}
}

func TestVLMConfiguredRequiresBaseURL(t *testing.T) {
	t.Setenv(EnvVLMAPIKey, "key")
	t.Setenv(EnvLegacyAPIKey, "")
	t.Setenv(EnvGeminiAPIKey, "")
	t.Setenv(EnvVLMModel, "model")
	t.Setenv(EnvLegacyModel, "")
	t.Setenv(EnvGeminiModel, "")
	t.Setenv(EnvVLMBaseURL, "")
	t.Setenv(EnvLegacyBaseURL, "")
	t.Setenv(EnvGeminiBaseURL, "")
	if VLMConfigured() {
		t.Fatal("want false when base URL is missing")
	}
	t.Setenv(EnvVLMBaseURL, "https://vlm.example/v1")
	if !VLMConfigured() {
		t.Fatal("want true when key, base URL, and model are set")
	}
}

func TestVLMAPIKeyEmpty(t *testing.T) {
	t.Setenv(EnvVLMAPIKey, "")
	t.Setenv(EnvLegacyAPIKey, "")
	t.Setenv(EnvGeminiAPIKey, "")
	if g := VLMAPIKey(); g != "" {
		t.Fatalf("want empty, got %q", g)
	}
}

func TestForceVLMEnv(t *testing.T) {
	t.Setenv(envForceVLM, "")
	if ForceVLM() {
		t.Fatal("want false when unset")
	}
	t.Setenv(envForceVLM, "1")
	if !ForceVLM() {
		t.Fatal("want true for 1")
	}
}

func TestFullPageVLMFallbackEnabledDefaultOff(t *testing.T) {
	t.Setenv(envForceVLM, "")
	t.Setenv(envFullPageVLMFallback, "")
	if FullPageVLMFallbackEnabled() {
		t.Fatal("want false by default")
	}
}

func TestFullPageVLMFallbackEnabledExplicit(t *testing.T) {
	t.Setenv(envForceVLM, "")
	t.Setenv(envFullPageVLMFallback, "1")
	if !FullPageVLMFallbackEnabled() {
		t.Fatal("want true when full-page fallback env is 1")
	}
}

func TestFullPageVLMFallbackEnabledForceOverrides(t *testing.T) {
	t.Setenv(envForceVLM, "1")
	t.Setenv(envFullPageVLMFallback, "0")
	if !FullPageVLMFallbackEnabled() {
		t.Fatal("want true when force vlm is 1")
	}
}
