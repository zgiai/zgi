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
