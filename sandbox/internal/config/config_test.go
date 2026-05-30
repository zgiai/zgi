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

func TestFromEnvReadsObserverRetention(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_OBSERVER_RETENTION_DAYS", "3")
	t.Setenv("ZGI_SANDBOX_OBSERVER_MAX_EVENTS", "250")

	cfg := FromEnv()

	if cfg.ObserverRetentionDays != 3 {
		t.Fatalf("expected observer retention 3 days, got %d", cfg.ObserverRetentionDays)
	}
	if cfg.ObserverMaxEvents != 250 {
		t.Fatalf("expected observer max events 250, got %d", cfg.ObserverMaxEvents)
	}
}

func TestPublicSnapshotOmitsSecrets(t *testing.T) {
	cfg := Config{
		Port:              "2660",
		APIKey:            "secret-api-key",
		RedisPassword:     "secret-redis-password",
		DatabaseURL:       "postgres://user:secret-db-password@127.0.0.1:5432/postgres",
		RedisAddr:         "127.0.0.1:6379",
		RedisDB:           2,
		WorkerID:          "worker-a",
		RuntimeBackend:    "preview",
		SecureRootFS:      "/srv/rootfs",
		BwrapBinary:       "bwrap",
		Environment:       "local",
		AdvertiseURL:      "http://127.0.0.1:2660",
		PublicBaseURL:     "http://127.0.0.1:2660",
		ObserverMaxEvents: 100,
	}

	snapshot := cfg.PublicSnapshot()

	if _, ok := snapshot["api_key"]; ok {
		t.Fatal("expected api_key to be omitted")
	}
	if _, ok := snapshot["redis_password"]; ok {
		t.Fatal("expected redis_password to be omitted")
	}
	if _, ok := snapshot["database_url"]; ok {
		t.Fatal("expected database_url to be omitted")
	}
	if snapshot["database_configured"] != true {
		t.Fatalf("expected database configured flag, got %#v", snapshot["database_configured"])
	}
	if snapshot["redis_configured"] != true {
		t.Fatalf("expected redis configured flag, got %#v", snapshot["redis_configured"])
	}
	if snapshot["secure_rootfs_configured"] != true {
		t.Fatalf("expected rootfs configured flag, got %#v", snapshot["secure_rootfs_configured"])
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
