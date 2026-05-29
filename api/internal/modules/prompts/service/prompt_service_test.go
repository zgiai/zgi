package service

import (
	"strings"
	"testing"

	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
)

func TestRuntimeReferenceLabelDefaultsToLatest(t *testing.T) {
	if got := runtimeReferenceLabel(RuntimePromptReference{}); got != latestLabel {
		t.Fatalf("expected default runtime label %q, got %q", latestLabel, got)
	}
}

func TestRuntimeReferenceLabelUsesExplicitTrimmedLabel(t *testing.T) {
	label := " production "
	if got := runtimeReferenceLabel(RuntimePromptReference{Label: &label}); got != "production" {
		t.Fatalf("expected explicit runtime label %q, got %q", "production", got)
	}
}

func TestReassignLabelsRequiresExistingTargetWhenRequested(t *testing.T) {
	err := reassignLabels(nil, []*promptmodel.PromptVersion{
		{Version: 1, Labels: []string{latestLabel}},
	}, 2, []string{"stable"}, true)
	if err == nil {
		t.Fatal("expected missing target version error")
	}
	if !strings.Contains(err.Error(), "prompt version not found") {
		t.Fatalf("expected prompt version not found error, got %v", err)
	}
}

func TestPromptOptimizerOutputLanguageNormalizesInterfaceLocale(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "simplified chinese", language: "zh-Hans", want: "Simplified Chinese"},
		{name: "english", language: "en-US", want: "English"},
		{name: "fallback custom", language: "fr-FR", want: "fr-FR"},
		{name: "empty", language: " ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promptOptimizerOutputLanguage(tt.language); got != tt.want {
				t.Fatalf("expected language %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildPromptOptimizerUserPromptUsesInterfaceLanguageForDeepOptimization(t *testing.T) {
	prompt := buildPromptOptimizerUserPrompt(
		promptOptimizerGoalDeep,
		"Write a sales email.",
		true,
		nil,
		"Simplified Chinese",
	)

	if !strings.Contains(prompt, "Use the system/interface language for the final optimized prompt: Simplified Chinese.") {
		t.Fatalf("expected deep optimizer prompt to require interface language, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Match the primary language of the user's original prompt") {
		t.Fatalf("deep optimizer prompt should not prefer original prompt language, got:\n%s", prompt)
	}
}

func TestNormalizePromptOptimizerInputRemovesZGISlotWrappers(t *testing.T) {
	raw := `你叫<zgi:slot name="agent_name" placeholder="智能体名称">小雪</zgi:slot>，请使用<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>。`
	got := normalizePromptOptimizerInput(raw)

	if strings.Contains(got, "<zgi:slot") {
		t.Fatalf("expected slot wrapper to be removed, got:\n%s", got)
	}
	if !strings.Contains(got, "你叫小雪") {
		t.Fatalf("expected slot value to be preserved as plain text, got:\n%s", got)
	}
	if !strings.Contains(got, `<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>`) {
		t.Fatalf("expected knowledge variable to be preserved, got:\n%s", got)
	}
}

func TestDetectPromptOptimizerVariablesIncludesZGICapabilityBlocks(t *testing.T) {
	raw := `当问题相关时使用<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>，计算时使用<zgi:skill id="calculator">计算器</zgi:skill>。`
	variables := detectPromptOptimizerVariables(raw)
	joined := strings.Join(variables, "\n")

	if !strings.Contains(joined, `<zgi:knowledge id="ds-1">产品知识库</zgi:knowledge>`) {
		t.Fatalf("expected knowledge variable to be detected, got %#v", variables)
	}
	if !strings.Contains(joined, `<zgi:skill id="calculator">计算器</zgi:skill>`) {
		t.Fatalf("expected skill variable to be detected, got %#v", variables)
	}
}
