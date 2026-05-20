package hyperparse

import "testing"

func TestSDKVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version empty")
	}
	if SDKVersion() != Version {
		t.Fatalf("SDKVersion()=%q want %q", SDKVersion(), Version)
	}
	if ModulePath == "" {
		t.Fatal("ModulePath empty")
	}
}
