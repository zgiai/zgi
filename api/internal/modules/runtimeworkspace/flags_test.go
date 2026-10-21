package runtimeworkspace

import "testing"

func TestRuntimeEnabledIsFailClosed(t *testing.T) {
	t.Setenv("ZGI_RUNTIME_WORKSPACE_ENABLED", "")
	if RuntimeEnabled() {
		t.Fatal("runtime workspace must be disabled when unset")
	}
	t.Setenv("ZGI_RUNTIME_WORKSPACE_ENABLED", "true")
	if !RuntimeEnabled() {
		t.Fatal("runtime workspace must be enabled when explicitly configured")
	}
}
