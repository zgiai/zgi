package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvSourceProcessEnvironmentOverridesDotenvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("RESEND_BASE_URL=https://file.example/v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RESEND_BASE_URL", "https://runtime.example/v1")

	source, err := newEnvSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := source.string("", envResendBaseURL); got != "https://runtime.example/v1" {
		t.Fatalf("RESEND_BASE_URL = %q, want process environment override", got)
	}
}
