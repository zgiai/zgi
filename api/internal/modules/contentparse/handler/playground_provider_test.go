package handler

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestBuildFileRouteProviderStatusesIncludesAutoAndLegacyPPTFallback(t *testing.T) {
	catalog := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{Name: "reducto", Enabled: true, Adapter: "reducto"},
		{Name: "mineru", Enabled: true, Adapter: "mineru"},
		{Name: "local", Enabled: true, Adapter: "local"},
	}}
	health := &contracts.ParseHealth{Adapters: []contracts.AdapterHealth{
		{Name: "reducto", Available: true},
		{Name: "mineru", Available: true},
		{Name: "local", Available: true},
	}}

	providers, ext := buildFileRouteProviderStatuses("legacy.ppt", catalog, health)
	if ext != ".ppt" {
		t.Fatalf("extension=%q", ext)
	}
	if len(providers) != 4 || providers[0].Key != "auto" || providers[1].Key != "reducto" || providers[2].Key != "mineru" || providers[3].Key != "local" {
		t.Fatalf("providers=%+v", providers)
	}
	if !providers[0].Selectable {
		t.Fatal("auto route should be selectable when a provider is available")
	}
}
