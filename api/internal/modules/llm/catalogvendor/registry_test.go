package catalogvendor

import "testing"

func TestReplaceAndLookup(t *testing.T) {
	Replace([]Entry{{Provider: "doubao", Model: "seed", Vendor: " 豆包 "}})
	if got := Lookup("doubao", "seed"); got != "豆包" {
		t.Fatalf("Lookup() = %q, want 豆包", got)
	}

	Replace(nil)
	if got := Lookup("doubao", "seed"); got != "" {
		t.Fatalf("Lookup() after replacement = %q, want empty", got)
	}
}
