package common

import "testing"

func TestResolveEngineDefaultsToLocal(t *testing.T) {
	if got := ResolveEngine("", ""); got != EngineLocal {
		t.Fatalf("ResolveEngine empty=%q, want %q", got, EngineLocal)
	}
	if got := ResolveEngine("", "unknown"); got != EngineLocal {
		t.Fatalf("ResolveEngine unknown=%q, want %q", got, EngineLocal)
	}
}

func TestResolveEngineRequestAndEnvPrecedence(t *testing.T) {
	if got := ResolveEngine("mineru", "local"); got != EngineMineru {
		t.Fatalf("request engine should win, got %q", got)
	}
	if got := ResolveEngine("", "gemini"); got != EngineVLM {
		t.Fatalf("backend env should be honored, got %q", got)
	}
	if got := ResolveEngine("reducto", "local"); got != EngineReducto {
		t.Fatalf("reducto request engine should win, got %q", got)
	}
}
