package keys

import "testing"

func TestBuilderBuildsGlobalAndModulePrefixes(t *testing.T) {
	builder := DefaultBuilder()

	got := builder.Build("llm.models.available", "org-1", "chat")
	want := "zgi_cache:llm:models:available:org-1:chat"
	if got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuilderSupportsConfiguredGlobalPrefix(t *testing.T) {
	builder := NewBuilder("custom_cache")

	got := builder.Build("llm:models", "available")
	want := "custom_cache:llm:models:available"
	if got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuilderFallsBackToDefaultGlobalPrefix(t *testing.T) {
	builder := NewBuilder("")

	got := builder.Build("llm.models")
	want := "zgi_cache:llm:models"
	if got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestNormalizeModulePrefix(t *testing.T) {
	got := NormalizeModulePrefix(" .llm:models.available: ")
	want := "llm:models:available"
	if got != want {
		t.Fatalf("NormalizeModulePrefix() = %q, want %q", got, want)
	}
}

func TestBuilderKeepsEmptyPartsDistinct(t *testing.T) {
	builder := DefaultBuilder()

	got := builder.Build("llm.models.available", "", "chat")
	want := "zgi_cache:llm:models:available:_:chat"
	if got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuilderEscapesArbitraryPartsToAvoidDelimiterCollisions(t *testing.T) {
	builder := DefaultBuilder()

	left := builder.Build("a.b", "c")
	right := builder.Build("a", "b:c")
	if left == right {
		t.Fatalf("Build() collision: %q == %q", left, right)
	}

	got := builder.Build("a", "b:c")
	want := "zgi_cache:a:b%3Ac"
	if got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}
