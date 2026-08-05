package config

import "testing"

func TestLoadChatRuntimeConfigEnablesNativeAgentLoopByDefault(t *testing.T) {
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}
	cfg := &Config{}
	loadChatRuntimeConfig(cfg, source)
	if !cfg.ChatRuntime.NativeAgentLoopEnabled {
		t.Fatal("NativeAgentLoopEnabled = false, want default true")
	}
	if !cfg.ChatRuntime.NativeSkillProgressiveDisclosureEnabled {
		t.Fatal("NativeSkillProgressiveDisclosureEnabled = false, want default true")
	}
	if !cfg.ChatRuntime.NativeModelProgressEnabled {
		t.Fatal("NativeModelProgressEnabled = false, want default true")
	}
	if cfg.ChatRuntime.ModelIdleTimeoutSeconds != 300 {
		t.Fatalf("ModelIdleTimeoutSeconds = %d, want 300", cfg.ChatRuntime.ModelIdleTimeoutSeconds)
	}
}

func TestLoadChatRuntimeConfigCanDisableNativeSkillProgressiveDisclosure(t *testing.T) {
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChatRuntimeNativeSkillProgressiveDisclosureEnabled {
			return "false", true
		}
		return "", false
	}}
	cfg := &Config{}
	loadChatRuntimeConfig(cfg, source)
	if cfg.ChatRuntime.NativeSkillProgressiveDisclosureEnabled {
		t.Fatal("NativeSkillProgressiveDisclosureEnabled = true, want false")
	}
}

func TestLoadChatRuntimeConfigCanDisableNativeAgentLoop(t *testing.T) {
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChatRuntimeNativeAgentLoopEnabled {
			return "false", true
		}
		return "", false
	}}
	cfg := &Config{}
	loadChatRuntimeConfig(cfg, source)
	if cfg.ChatRuntime.NativeAgentLoopEnabled {
		t.Fatal("NativeAgentLoopEnabled = true, want false")
	}
}

func TestLoadChatRuntimeConfigCanDisableNativeModelProgress(t *testing.T) {
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChatRuntimeNativeModelProgressEnabled {
			return "false", true
		}
		return "", false
	}}
	cfg := &Config{}
	loadChatRuntimeConfig(cfg, source)
	if cfg.ChatRuntime.NativeModelProgressEnabled {
		t.Fatal("NativeModelProgressEnabled = true, want false")
	}
}
