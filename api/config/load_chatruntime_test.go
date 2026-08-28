package config

import "testing"

func TestLoadChatRuntimeConfigUsesDefaultModelIdleTimeout(t *testing.T) {
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}
	cfg := &Config{}
	if err := loadChatRuntimeConfig(cfg, source); err != nil {
		t.Fatal(err)
	}
	if cfg.ChatRuntime.ModelIdleTimeoutSeconds != 300 {
		t.Fatalf("ModelIdleTimeoutSeconds = %d, want 300", cfg.ChatRuntime.ModelIdleTimeoutSeconds)
	}
	if cfg.ChatRuntime.AgentContextWindowK != 256 {
		t.Fatalf("AgentContextWindowK = %d, want 256", cfg.ChatRuntime.AgentContextWindowK)
	}
	if cfg.ChatRuntime.ContextPromptDumpEnabled {
		t.Fatal("ContextPromptDumpEnabled = true, want false")
	}
}

func TestLoadChatRuntimeConfigReadsContextPromptDumpEnabled(t *testing.T) {
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChatRuntimeContextPromptDumpEnabled {
			return "true", true
		}
		return "", false
	}}
	cfg := &Config{}
	if err := loadChatRuntimeConfig(cfg, source); err != nil {
		t.Fatal(err)
	}
	if !cfg.ChatRuntime.ContextPromptDumpEnabled {
		t.Fatal("ContextPromptDumpEnabled = false, want true")
	}
}

func TestLoadChatRuntimeConfigRejectsInvalidContextPromptDumpEnabled(t *testing.T) {
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChatRuntimeContextPromptDumpEnabled {
			return "yes", true
		}
		return "", false
	}}
	if err := loadChatRuntimeConfig(&Config{}, source); err == nil {
		t.Fatal("loadChatRuntimeConfig() error = nil")
	}
}

func TestLoadChatRuntimeConfigRejectsInvalidAgentContextWindow(t *testing.T) {
	for _, value := range []string{"0", "-1", "invalid", "18446744073709551615"} {
		t.Run(value, func(t *testing.T) {
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				if key == envChatRuntimeAgentContextWindowK {
					return value, true
				}
				return "", false
			}}
			if err := loadChatRuntimeConfig(&Config{}, source); err == nil {
				t.Fatalf("loadChatRuntimeConfig(%q) error = nil", value)
			}
		})
	}
}
