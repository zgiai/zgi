package config

import "testing"

func TestValidateStartupRejectsPreviewBackendInProduction(t *testing.T) {
	cfg := Config{
		Environment:    "production",
		RuntimeBackend: "preview",
	}

	if err := cfg.ValidateStartup(); err == nil {
		t.Fatal("expected production preview backend to be rejected")
	}
}

func TestValidateStartupAllowsPreviewBackendOutsideProduction(t *testing.T) {
	cfg := Config{
		Environment:    "local",
		RuntimeBackend: "preview",
	}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected local preview backend to be allowed, got %v", err)
	}
}

func TestValidateStartupAllowsSecureBackendInProduction(t *testing.T) {
	cfg := Config{
		Environment:    "prod",
		RuntimeBackend: "linux-secure",
	}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected production secure backend to be allowed, got %v", err)
	}
}
