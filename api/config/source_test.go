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

func TestEnvSourceProcessEnvironmentOverridesDotenvAcrossAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("RESEND_API_KEY=file-canonical\nEMAIL_SMTP_PORT=587\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMAIL_RESEND_API_KEY", "runtime-legacy")
	t.Setenv("EMAIL_PORT", "465")

	source, err := newEnvSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := source.nonEmptyString("", envResendAPIKey, envEmailResendAPIKey); got != "runtime-legacy" {
		t.Fatalf("Resend API key = %q, want runtime legacy alias", got)
	}
	port, err := source.nonEmptyInt(587, envEmailSMTPPort, envEmailPort)
	if err != nil {
		t.Fatal(err)
	}
	if port != 465 {
		t.Fatalf("SMTP port = %d, want runtime legacy alias value 465", port)
	}
}
