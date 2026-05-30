package config

import "testing"

func TestFromEnvReadsShutdownTimeout(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS", "17")

	cfg := FromEnv()

	if cfg.ShutdownTimeoutSeconds != 17 {
		t.Fatalf("expected shutdown timeout 17, got %d", cfg.ShutdownTimeoutSeconds)
	}
}

func TestFromEnvDefaultsShutdownTimeout(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS", "")

	cfg := FromEnv()

	if cfg.ShutdownTimeoutSeconds != 10 {
		t.Fatalf("expected default shutdown timeout 10, got %d", cfg.ShutdownTimeoutSeconds)
	}
}

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
