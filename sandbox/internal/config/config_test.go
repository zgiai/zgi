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

func TestFromEnvReadsOrganizationExecutionRateLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION", "7")

	cfg := FromEnv()

	if cfg.MaxExecutionsPerMinutePerOrganization != 7 {
		t.Fatalf("expected organization execution rate limit 7, got %d", cfg.MaxExecutionsPerMinutePerOrganization)
	}
}

func TestFromEnvReadsOrganizationConcurrentExecutionLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION", "3")

	cfg := FromEnv()

	if cfg.MaxConcurrentExecutionsPerOrganization != 3 {
		t.Fatalf("expected organization concurrent execution limit 3, got %d", cfg.MaxConcurrentExecutionsPerOrganization)
	}
}

func TestFromEnvReadsServiceConcurrentExecutionLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS", "2")

	cfg := FromEnv()

	if cfg.MaxConcurrentExecutions != 2 {
		t.Fatalf("expected service concurrent execution limit 2, got %d", cfg.MaxConcurrentExecutions)
	}
}

func TestFromEnvReadsProfileConcurrentExecutionLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_PROFILE", "4")

	cfg := FromEnv()

	if cfg.MaxConcurrentExecutionsPerProfile != 4 {
		t.Fatalf("expected profile concurrent execution limit 4, got %d", cfg.MaxConcurrentExecutionsPerProfile)
	}
}

func TestFromEnvReadsOrganizationQueuedExecutionLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION", "5")

	cfg := FromEnv()

	if cfg.MaxQueuedExecutionsPerOrganization != 5 {
		t.Fatalf("expected organization queued execution limit 5, got %d", cfg.MaxQueuedExecutionsPerOrganization)
	}
}

func TestFromEnvReadsWorkspaceByteLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_WORKSPACE_BYTES", "65536")

	cfg := FromEnv()

	if cfg.MaxWorkspaceBytes != 65536 {
		t.Fatalf("expected workspace byte limit 65536, got %d", cfg.MaxWorkspaceBytes)
	}
}

func TestFromEnvReadsOrganizationWorkspaceByteLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_WORKSPACE_BYTES_PER_ORGANIZATION", "131072")

	cfg := FromEnv()

	if cfg.MaxWorkspaceBytesPerOrganization != 131072 {
		t.Fatalf("expected organization workspace byte limit 131072, got %d", cfg.MaxWorkspaceBytesPerOrganization)
	}
}

func TestFromEnvReadsWorkspaceFileLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_WORKSPACE_FILES", "9")

	cfg := FromEnv()

	if cfg.MaxWorkspaceFiles != 9 {
		t.Fatalf("expected workspace file limit 9, got %d", cfg.MaxWorkspaceFiles)
	}
}

func TestFromEnvReadsArtifactManifestLimits(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_FILES", "12")
	t.Setenv("ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_BYTES", "4096")
	t.Setenv("ZGI_SANDBOX_MAX_ARTIFACT_BYTES_PER_ORGANIZATION", "8192")

	cfg := FromEnv()

	if cfg.MaxArtifactManifestFiles != 12 {
		t.Fatalf("expected artifact manifest file limit 12, got %d", cfg.MaxArtifactManifestFiles)
	}
	if cfg.MaxArtifactManifestBytes != 4096 {
		t.Fatalf("expected artifact manifest byte limit 4096, got %d", cfg.MaxArtifactManifestBytes)
	}
	if cfg.MaxArtifactBytesPerOrganization != 8192 {
		t.Fatalf("expected organization artifact byte limit 8192, got %d", cfg.MaxArtifactBytesPerOrganization)
	}
}

func TestFromEnvReadsOrganizationDependencyProfileLimit(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_DEPENDENCY_PROFILES_PER_ORGANIZATION", "2")

	cfg := FromEnv()

	if cfg.MaxDependencyProfilesPerOrganization != 2 {
		t.Fatalf("expected organization dependency profile limit 2, got %d", cfg.MaxDependencyProfilesPerOrganization)
	}
}

func TestPublicSnapshotOmitsSecrets(t *testing.T) {
	cfg := Config{
		Port:                                 "2660",
		APIKey:                               "secret-api-key",
		RedisPassword:                        "secret-redis-password",
		DatabaseURL:                          "postgres://user:secret-db-password@127.0.0.1:5432/postgres",
		RedisAddr:                            "127.0.0.1:6379",
		RedisDB:                              2,
		WorkerID:                             "worker-a",
		RuntimeBackend:                       "preview",
		SecureRootFS:                         "/srv/rootfs",
		BwrapBinary:                          "bwrap",
		Environment:                          "local",
		AdvertiseURL:                         "http://127.0.0.1:2660",
		PublicBaseURL:                        "http://127.0.0.1:2660",
		ObserverMaxEvents:                    100,
		MaxConcurrentExecutions:              3,
		MaxArtifactManifestFiles:             12,
		MaxArtifactManifestBytes:             4096,
		MaxArtifactBytesPerOrganization:      8192,
		MaxDependencyProfilesPerOrganization: 2,
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
	if snapshot["max_concurrent_executions"] != 3 {
		t.Fatalf("expected service concurrent execution limit, got %#v", snapshot["max_concurrent_executions"])
	}
	if snapshot["max_artifact_manifest_files"] != 12 {
		t.Fatalf("expected artifact manifest file limit, got %#v", snapshot["max_artifact_manifest_files"])
	}
	if snapshot["max_artifact_manifest_bytes"] != int64(4096) {
		t.Fatalf("expected artifact manifest byte limit, got %#v", snapshot["max_artifact_manifest_bytes"])
	}
	if snapshot["max_artifact_bytes_per_organization"] != int64(8192) {
		t.Fatalf("expected organization artifact byte limit, got %#v", snapshot["max_artifact_bytes_per_organization"])
	}
	if snapshot["max_dependency_profiles_per_organization"] != 2 {
		t.Fatalf("expected organization dependency profile limit, got %#v", snapshot["max_dependency_profiles_per_organization"])
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
