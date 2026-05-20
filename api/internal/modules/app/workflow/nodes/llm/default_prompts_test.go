package llm

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/prompt"
)

func TestGetDefaultConfig_UsesPromptTemplates(t *testing.T) {
	node := &Node{}
	cfg := node.getDefaultConfig(nil)

	configMap := cfg["config"].(map[string]any)
	promptTemplates := configMap["prompt_templates"].(map[string]any)

	chatPrompts := promptTemplates["chat_model"].(map[string]any)["prompts"].([]map[string]any)
	if got := chatPrompts[0]["text"]; got != defaultChatSystemPromptFallback {
		t.Fatalf("chat system prompt = %q, want %q", got, defaultChatSystemPromptFallback)
	}

	completionPrompt := promptTemplates["completion_model"].(map[string]any)["prompt"].(map[string]any)
	if got := completionPrompt["text"]; got != defaultCompletionPromptFallback {
		t.Fatalf("completion prompt = %q, want %q", got, defaultCompletionPromptFallback)
	}
}

func TestGetDefaultConfig_FallsBackWhenPromptLookupFails(t *testing.T) {
	originalGetPromptTemplate := getPromptTemplate
	getPromptTemplate = func(id prompt.TemplateID) (*prompt.Template, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() {
		getPromptTemplate = originalGetPromptTemplate
	})

	node := &Node{}
	cfg := node.getDefaultConfig(nil)

	configMap := cfg["config"].(map[string]any)
	promptTemplates := configMap["prompt_templates"].(map[string]any)

	chatPrompts := promptTemplates["chat_model"].(map[string]any)["prompts"].([]map[string]any)
	if got := chatPrompts[0]["text"]; got != defaultChatSystemPromptFallback {
		t.Fatalf("fallback chat system prompt = %q, want %q", got, defaultChatSystemPromptFallback)
	}

	completionPrompt := promptTemplates["completion_model"].(map[string]any)["prompt"].(map[string]any)
	if got := completionPrompt["text"]; got != defaultCompletionPromptFallback {
		t.Fatalf("fallback completion prompt = %q, want %q", got, defaultCompletionPromptFallback)
	}
}
