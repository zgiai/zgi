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

func TestReplaceProviderPreservesOtherProviders(t *testing.T) {
	Replace([]Entry{
		{Provider: "doubao", Model: "seed", Vendor: "豆包"},
		{Provider: "qwen", Model: "plus", Vendor: "千问"},
	})
	ReplaceProvider("doubao", []Entry{{Provider: "doubao", Model: "pro", Vendor: "豆包"}})

	if got := Lookup("doubao", "seed"); got != "" {
		t.Fatalf("stale provider model vendor = %q, want empty", got)
	}
	if got := Lookup("doubao", "pro"); got != "豆包" {
		t.Fatalf("new provider model vendor = %q, want 豆包", got)
	}
	if got := Lookup("qwen", "plus"); got != "千问" {
		t.Fatalf("other provider vendor = %q, want 千问", got)
	}
}
