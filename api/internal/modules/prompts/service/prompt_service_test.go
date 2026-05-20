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
