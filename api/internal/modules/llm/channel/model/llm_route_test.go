package model

import (
	"testing"
)

func TestLLMRouteGetEffectiveModels_NormalizesLegacyProviderWildcard(t *testing.T) {
	route := &LLMRoute{
		Models: []string{" gpt-4o ", "*:anthropic", "", "gpt-4o", "*"},
	}

	got := route.GetEffectiveModels()
	want := []string{"gpt-4o", "*"}

	if len(got) != len(want) {
		t.Fatalf("len(GetEffectiveModels()) = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("GetEffectiveModels()[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}

func TestLLMRouteSupportsModel_LegacyProviderWildcardBehavesAsGlobalWildcard(t *testing.T) {
	route := &LLMRoute{
		Models: []string{"*:anthropic"},
	}

	if !route.SupportsModel("claude-3-opus") {
		t.Fatal("SupportsModel(claude-3-opus) = false, want true")
	}
	if !route.SupportsModel("gpt-4o") {
		t.Fatal("SupportsModel(gpt-4o) = false, want true")
	}
}

func TestLLMRouteSupportsModelForProvider_OfficialRouteRequiresExactPair(t *testing.T) {
	route := &LLMRoute{
		IsOfficial: true,
		Models:     []string{"same-name", "Pro/gpt-4.1"},
		OfficialProviderModels: []ProviderModel{
			{Provider: "openai", Model: "same-name"},
			{Provider: "openai", Model: "Pro/gpt-4.1"},
		},
	}

	if !route.SupportsModelForProvider("openai", "same-name") {
		t.Fatal("official exact provider-model pair should be supported")
	}
	if route.SupportsModelForProvider("anthropic", "same-name") {
		t.Fatal("official same-name model under the wrong provider should be rejected")
	}
	if route.SupportsModelForProvider("", "same-name") {
		t.Fatal("official model with missing provider should be rejected")
	}
	if route.SupportsModelForProvider("openai", "gpt-4.1") {
		t.Fatal("official model must not strip the Pro/ prefix")
	}
}

func TestLLMRouteSupportsModelForProvider_OfficialLegacySnapshotFallsBackToModelNames(t *testing.T) {
	route := &LLMRoute{
		IsOfficial: true,
		Models:     []string{"legacy-model"},
	}

	if !route.SupportsModelForProvider("openai", "legacy-model") {
		t.Fatal("official route without provider-model pairs should preserve legacy model-name support")
	}
	if route.SupportsModelForProvider("openai", "missing-model") {
		t.Fatal("legacy fallback must not allow a model absent from the snapshot")
	}
	if route.SupportsModelForProvider("", "legacy-model") {
		t.Fatal("legacy fallback must still reject a missing provider")
	}
}

func TestLLMRouteSupportsModelForProvider_PrivateRouteBehaviorIsUnchanged(t *testing.T) {
	route := &LLMRoute{Models: []string{"same-name"}}

	if !route.SupportsModelForProvider("different-provider", "same-name") {
		t.Fatal("private route support should remain based on its existing model list")
	}
}
