package model

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/catalogvendor"
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

func TestLLMRouteSupportsModelForProvider_OfficialRouteUsesLogicalModel(t *testing.T) {
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
	if !route.SupportsModelForProvider("anthropic", "same-name") {
		t.Fatal("official model should remain available independent of supplier provider")
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

// Discovery must not depend on the flat Models list alone: a snapshot whose
// list has gone stale would otherwise hide a model the route still advertises
// through its provider/model pairs.
func TestLLMRouteSupportsModelForProvider_UsesProviderPairsWhenFlatListIsStale(t *testing.T) {
	route := &LLMRoute{
		IsOfficial:             true,
		Models:                 []string{"stale-flat-name"},
		OfficialProviderModels: []ProviderModel{{Provider: "openai", Model: "same-name"}},
	}

	if !route.SupportsModelForProvider("openai", "same-name") {
		t.Fatal("a stale flat model list must not hide an advertised provider/model pair")
	}
	if route.SupportsModelForProvider("openai", "never-advertised") {
		t.Fatal("a model in neither the flat list nor the pairs must stay unsupported")
	}
}

// SupportsPlatformModel is the boundary invocation enforces, so it must accept
// exactly what the router accepts: the exact pair, or a sibling supplier that
// maps to the same logical vendor.
func TestLLMRouteSupportsPlatformModel_MatchesRouterBoundary(t *testing.T) {
	catalogvendor.Replace([]catalogvendor.Entry{
		{Provider: "agicto", Model: "deepseek-v4-pro", Vendor: "deepseek"},
		{Provider: "siliconflow", Model: "deepseek-v4-pro", Vendor: "deepseek"},
	})
	t.Cleanup(func() { catalogvendor.Replace(nil) })

	route := &LLMRoute{
		IsOfficial: true,
		Models:     []string{"deepseek-v4-pro", "same-name"},
		OfficialProviderModels: []ProviderModel{
			{Provider: "siliconflow", Model: "deepseek-v4-pro"},
			{Provider: "openai", Model: "same-name"},
		},
	}

	if !route.SupportsPlatformModel("siliconflow", "deepseek-v4-pro") {
		t.Fatal("the exact advertised pair must be supported")
	}
	if !route.SupportsPlatformModel("agicto", "deepseek-v4-pro") {
		t.Fatal("a sibling supplier of the same vendor must be supported, as the router resolves to it")
	}
	if route.SupportsPlatformModel("anthropic", "same-name") {
		t.Fatal("a different-vendor supplier must be rejected, as the router rejects it")
	}
}

func TestLLMRouteSupportsModelForProvider_PrivateRouteBehaviorIsUnchanged(t *testing.T) {
	route := &LLMRoute{Models: []string{"same-name"}}

	if !route.SupportsModelForProvider("different-provider", "same-name") {
		t.Fatal("private route support should remain based on its existing model list")
	}
}
