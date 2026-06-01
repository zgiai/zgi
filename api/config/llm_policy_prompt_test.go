package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLLMPolicyPromptConfig_DefaultDisabled(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}

	if err := loadLLMPolicyPromptConfig(cfg, source); err != nil {
		t.Fatalf("loadLLMPolicyPromptConfig() error = %v", err)
	}
	if cfg.LLMPolicyPrompt.Enabled {
		t.Fatal("LLMPolicyPrompt.Enabled = true, want false")
	}
	if cfg.LLMPolicyPrompt.Prompt != "" {
		t.Fatalf("LLMPolicyPrompt.Prompt = %q, want empty", cfg.LLMPolicyPrompt.Prompt)
	}
}

func TestLoadLLMPolicyPromptConfig_LoadsTextOverride(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		switch key {
		case envLLMPolicyPromptEnabled:
			return "true", true
		case envLLMPolicyPromptText:
			return "  policy from env  ", true
		case envLLMPolicyPromptFile:
			return "/tmp/ignored-policy.txt", true
		default:
			return "", false
		}
	}}

	if err := loadLLMPolicyPromptConfig(cfg, source); err != nil {
		t.Fatalf("loadLLMPolicyPromptConfig() error = %v", err)
	}
	if !cfg.LLMPolicyPrompt.Enabled {
		t.Fatal("LLMPolicyPrompt.Enabled = false, want true")
	}
	if cfg.LLMPolicyPrompt.Prompt != "policy from env" {
		t.Fatalf("LLMPolicyPrompt.Prompt = %q, want text override", cfg.LLMPolicyPrompt.Prompt)
	}
}

func TestLoadLLMPolicyPromptConfig_LoadsPromptFile(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "policy.txt")
	if err := os.WriteFile(promptPath, []byte("\npolicy from file\n"), 0600); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		switch key {
		case envLLMPolicyPromptEnabled:
			return "true", true
		case envLLMPolicyPromptFile:
			return promptPath, true
		default:
			return "", false
		}
	}}

	if err := loadLLMPolicyPromptConfig(cfg, source); err != nil {
		t.Fatalf("loadLLMPolicyPromptConfig() error = %v", err)
	}
	if cfg.LLMPolicyPrompt.Prompt != "policy from file" {
		t.Fatalf("LLMPolicyPrompt.Prompt = %q, want file content", cfg.LLMPolicyPrompt.Prompt)
	}
}

func TestLoadLLMPolicyPromptConfig_EnabledWithoutPromptFails(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envLLMPolicyPromptEnabled {
			return "true", true
		}
		return "", false
	}}

	err := loadLLMPolicyPromptConfig(cfg, source)
	if err == nil {
		t.Fatal("loadLLMPolicyPromptConfig() error = nil, want missing prompt error")
	}
	if !strings.Contains(err.Error(), envLLMPolicyPromptFile) {
		t.Fatalf("error = %v, want mention %s", err, envLLMPolicyPromptFile)
	}
}
