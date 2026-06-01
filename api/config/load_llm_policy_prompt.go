package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadLLMPolicyPromptConfig(cfg *Config, source *envSource) error {
	enabled, err := source.bool(false, envLLMPolicyPromptEnabled)
	if err != nil {
		return err
	}

	filePath := strings.TrimSpace(source.string("", envLLMPolicyPromptFile))
	text := strings.TrimSpace(source.string("", envLLMPolicyPromptText))
	cfg.LLMPolicyPrompt = LLMPolicyPromptConfig{
		Enabled: enabled,
		File:    filePath,
	}
	if !enabled {
		return nil
	}

	if text != "" {
		cfg.LLMPolicyPrompt.Prompt = text
		return nil
	}
	if filePath == "" {
		return fmt.Errorf("%s is required when %s=true", envLLMPolicyPromptFile, envLLMPolicyPromptEnabled)
	}

	resolvedPath, err := resolvePolicyPromptFilePath(source, filePath)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", envLLMPolicyPromptFile, err)
	}
	prompt := strings.TrimSpace(string(content))
	if prompt == "" {
		return fmt.Errorf("%s must not be empty when %s=true", envLLMPolicyPromptFile, envLLMPolicyPromptEnabled)
	}

	cfg.LLMPolicyPrompt.File = resolvedPath
	cfg.LLMPolicyPrompt.Prompt = prompt
	return nil
}

func resolvePolicyPromptFilePath(source *envSource, rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return rawPath, nil
	}
	if source != nil && source.path != "" {
		return filepath.Join(filepath.Dir(source.path), rawPath), nil
	}
	return filepath.Abs(rawPath)
}
