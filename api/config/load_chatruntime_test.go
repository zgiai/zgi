package config

import "testing"

func TestLoadChatRuntimeConfigUsesDefaultModelIdleTimeout(t *testing.T) {
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}
	cfg := &Config{}
	loadChatRuntimeConfig(cfg, source)
	if cfg.ChatRuntime.ModelIdleTimeoutSeconds != 300 {
		t.Fatalf("ModelIdleTimeoutSeconds = %d, want 300", cfg.ChatRuntime.ModelIdleTimeoutSeconds)
	}
}
