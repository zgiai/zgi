package contentparse

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestNewModuleBuildsService(t *testing.T) {
	module := NewModule()
	if module == nil {
		t.Fatal("expected module")
	}
	if module.Service == nil {
		t.Fatal("expected service")
	}
	if module.SDKAdapter == nil {
		t.Fatal("expected sdk adapter")
	}
}

func TestDefaultCatalogUsesSystemVLMProviderShape(t *testing.T) {
	catalog := DefaultProviderCatalog()
	var vlm *contracts.ParseProviderConfig
	for i := range catalog.Providers {
		if catalog.Providers[i].Name == "vlm" {
			vlm = &catalog.Providers[i]
			break
		}
	}
	if vlm == nil {
		t.Fatal("expected vlm provider")
	}
	if vlm.Adapter != "system_vlm" {
		t.Fatalf("vlm adapter = %q, want system_vlm", vlm.Adapter)
	}
	if vlm.Enabled {
		t.Fatal("default VLM provider should be disabled until the system adapter is injected")
	}
	if vlm.BaseURL != "" || vlm.APIKeyEnv != "" {
		t.Fatalf("vlm BaseURL/APIKeyEnv = %q/%q, want empty", vlm.BaseURL, vlm.APIKeyEnv)
	}
}

func TestModuleCanEnableSystemVLMProviderOverride(t *testing.T) {
	module := NewModule(WithProviderOverrides(SystemVLMProviderConfig(true)))
	var enabled bool
	for _, provider := range module.Catalog.Providers {
		if provider.Name == "vlm" {
			enabled = provider.Enabled && provider.Adapter == "system_vlm"
			break
		}
	}
	if !enabled {
		t.Fatal("expected system VLM provider override to be enabled")
	}
}
