package catalogvendor

import "testing"

func TestLookupMatchesNormalizedSupplierSpelling(t *testing.T) {
	Replace([]Entry{{Provider: "agicto-supplier", Model: "seed", Vendor: "Agicto Supplier"}})
	t.Cleanup(func() { Replace(nil) })

	for _, provider := range []string{"agicto", "Agicto Supplier", "agicto_supplier"} {
		if got := Lookup(provider, "seed"); got != "agicto supplier" {
			t.Fatalf("Lookup(%q, seed) = %q, want normalized vendor", provider, got)
		}
	}
}

func TestReplaceProviderClearsNormalizedKeys(t *testing.T) {
	Replace([]Entry{
		{Provider: "agicto-supplier", Model: "seed", Vendor: "v1"},
		{Provider: "qwen", Model: "plus", Vendor: "千问"},
	})
	t.Cleanup(func() { Replace(nil) })

	ReplaceProvider("agicto-supplier", []Entry{{Provider: "agicto-supplier", Model: "pro", Vendor: "v2"}})

	if got := Lookup("agicto", "seed"); got != "" {
		t.Fatalf("stale normalized vendor = %q, want empty", got)
	}
	if got := Lookup("agicto", "pro"); got != "v2" {
		t.Fatalf("new normalized vendor = %q, want v2", got)
	}
	if got := Lookup("qwen", "plus"); got != "千问" {
		t.Fatalf("other provider vendor = %q, want 千问", got)
	}
}
