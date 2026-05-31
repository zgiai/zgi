package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestFromEnvReadsDependencyProfileBuildLimits(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_MAX_DEPENDENCY_PROFILE_SIZE_BYTES", "1048576")
	t.Setenv("ZGI_SANDBOX_DEPENDENCY_PROFILE_BUILD_TIMEOUT_SECONDS", "120")

	cfg := FromEnv()

	if cfg.MaxDependencyProfileSizeBytes != 1048576 {
		t.Fatalf("expected dependency profile size limit 1048576, got %d", cfg.MaxDependencyProfileSizeBytes)
	}
	if cfg.DependencyProfileBuildTimeoutSeconds != 120 {
		t.Fatalf("expected dependency profile build timeout 120, got %d", cfg.DependencyProfileBuildTimeoutSeconds)
	}
}

func TestFromEnvReadsDependencyRootFSDir(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR", "/srv/zgi-sandbox/profiles")

	cfg := FromEnv()

	if cfg.DependencyRootFSDir != "/srv/zgi-sandbox/profiles" {
		t.Fatalf("expected dependency rootfs dir, got %s", cfg.DependencyRootFSDir)
	}
}

func TestFromEnvReadsSecureRuntimeLimits(t *testing.T) {
	t.Setenv("ZGI_SANDBOX_SECURE_RUNTIME_CPU_SECONDS", "3")
	t.Setenv("ZGI_SANDBOX_SECURE_RUNTIME_MEMORY_BYTES", "134217728")
	t.Setenv("ZGI_SANDBOX_SECURE_RUNTIME_PROCESS_LIMIT", "32")
	t.Setenv("ZGI_SANDBOX_SECURE_RUNTIME_OPEN_FILE_LIMIT", "64")

	cfg := FromEnv()

	if cfg.SecureRuntimeCPUSeconds != 3 {
		t.Fatalf("expected secure runtime cpu seconds 3, got %d", cfg.SecureRuntimeCPUSeconds)
	}
	if cfg.SecureRuntimeMemoryBytes != 134217728 {
		t.Fatalf("expected secure runtime memory bytes 134217728, got %d", cfg.SecureRuntimeMemoryBytes)
	}
	if cfg.SecureRuntimeProcessLimit != 32 {
		t.Fatalf("expected secure runtime process limit 32, got %d", cfg.SecureRuntimeProcessLimit)
	}
	if cfg.SecureRuntimeOpenFileLimit != 64 {
		t.Fatalf("expected secure runtime open file limit 64, got %d", cfg.SecureRuntimeOpenFileLimit)
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
		DependencyRootFSDir:                  "/srv/profiles",
		BwrapBinary:                          "bwrap",
		SecureRuntimeCPUSeconds:              3,
		SecureRuntimeMemoryBytes:             134217728,
		SecureRuntimeProcessLimit:            32,
		SecureRuntimeOpenFileLimit:           64,
		Environment:                          "local",
		AdvertiseURL:                         "http://127.0.0.1:2660",
		PublicBaseURL:                        "http://127.0.0.1:2660",
		ObserverMaxEvents:                    100,
		MaxConcurrentExecutions:              3,
		MaxArtifactManifestFiles:             12,
		MaxArtifactManifestBytes:             4096,
		MaxArtifactBytesPerOrganization:      8192,
		MaxDependencyProfilesPerOrganization: 2,
		MaxDependencyProfileSizeBytes:        1048576,
		DependencyProfileBuildTimeoutSeconds: 120,
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
	if snapshot["dependency_rootfs_configured"] != true {
		t.Fatalf("expected dependency rootfs configured flag, got %#v", snapshot["dependency_rootfs_configured"])
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
	if snapshot["max_dependency_profile_size_bytes"] != int64(1048576) {
		t.Fatalf("expected dependency profile size limit, got %#v", snapshot["max_dependency_profile_size_bytes"])
	}
	if snapshot["dependency_profile_build_timeout_seconds"] != 120 {
		t.Fatalf("expected dependency profile build timeout, got %#v", snapshot["dependency_profile_build_timeout_seconds"])
	}
	if snapshot["secure_runtime_cpu_seconds"] != 3 {
		t.Fatalf("expected secure runtime cpu seconds, got %#v", snapshot["secure_runtime_cpu_seconds"])
	}
	if snapshot["secure_runtime_memory_bytes"] != int64(134217728) {
		t.Fatalf("expected secure runtime memory bytes, got %#v", snapshot["secure_runtime_memory_bytes"])
	}
	if snapshot["secure_runtime_process_limit"] != 32 {
		t.Fatalf("expected secure runtime process limit, got %#v", snapshot["secure_runtime_process_limit"])
	}
	if snapshot["secure_runtime_open_file_limit"] != 64 {
		t.Fatalf("expected secure runtime open file limit, got %#v", snapshot["secure_runtime_open_file_limit"])
	}
}

func TestValidateStartupRejectsPreviewBackendInProduction(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Environment = "production"
	cfg.RuntimeBackend = "preview"
	cfg.APIKey = "secret"

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "runtime backend that enforces network policy") {
		t.Fatalf("expected production preview backend to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsMissingProductionAPIKey(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Environment = "production"
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = secureRootFSDir(t)
	cfg.APIKey = ""

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "ZGI_SANDBOX_API_KEY") {
		t.Fatalf("expected missing production API key to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsInvalidBounds(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Port = "70000"
	cfg.MaxWorkers = 0
	cfg.MaxConcurrentExecutions = -1
	cfg.MaxWorkspaceBytes = -1
	cfg.SecureRuntimeCPUSeconds = -1
	cfg.AdvertiseURL = "localhost:2660"

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected invalid startup config to be rejected")
	}
	for _, expected := range []string{
		"ZGI_SANDBOX_SERVER_PORT",
		"ZGI_SANDBOX_LITE_MAX_WORKERS",
		"ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS",
		"ZGI_SANDBOX_MAX_WORKSPACE_BYTES",
		"ZGI_SANDBOX_SECURE_RUNTIME_CPU_SECONDS",
		"ZGI_SANDBOX_ADVERTISE_URL",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected validation error for %s, got %v", expected, err)
		}
	}
}

func TestValidateStartupRejectsDisabledSecureRuntimeLimitsInProduction(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Environment = "production"
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = secureRootFSDir(t)
	cfg.APIKey = "secret"
	cfg.SecureRuntimeMemoryBytes = 0

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "ZGI_SANDBOX_SECURE_RUNTIME_MEMORY_BYTES") {
		t.Fatalf("expected disabled production secure runtime memory limit to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsUnsupportedRuntimeBackend(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "unknown"

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "ZGI_SANDBOX_RUNTIME_BACKEND") {
		t.Fatalf("expected unsupported runtime backend to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsSecureBackendWithoutRootFS(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = ""

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "ZGI_SANDBOX_SECURE_ROOTFS") {
		t.Fatalf("expected missing secure rootfs to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsRelativeSecureRootFS(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = "rootfs"

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected relative secure rootfs to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsMissingSecureRootFSPath(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = filepath.Join(t.TempDir(), "missing-rootfs")

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "existing directory") {
		t.Fatalf("expected missing secure rootfs path to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsSecureRootFSFile(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = filepath.Join(t.TempDir(), "rootfs-file")
	if err := os.WriteFile(cfg.SecureRootFS, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write rootfs file: %v", err)
	}

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("expected secure rootfs file to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsSecureRootFSSymlink(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	target := secureRootFSDir(t)
	cfg.SecureRootFS = filepath.Join(t.TempDir(), "rootfs-link")
	if err := os.Symlink(target, cfg.SecureRootFS); err != nil {
		t.Fatalf("create rootfs symlink: %v", err)
	}

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("expected secure rootfs symlink to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsWorldWritableSecureRootFS(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = secureRootFSDir(t)
	if err := os.Chmod(cfg.SecureRootFS, 0o777); err != nil {
		t.Fatalf("chmod rootfs: %v", err)
	}

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "must not be world-writable") {
		t.Fatalf("expected world-writable secure rootfs to be rejected, got %v", err)
	}
}

func TestValidateStartupRejectsInvalidDependencyRootFSDir(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = secureRootFSDir(t)
	cfg.DependencyRootFSDir = filepath.Join(t.TempDir(), "missing-profiles")

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "ZGI_SANDBOX_DEPENDENCY_ROOTFS_DIR") {
		t.Fatalf("expected missing dependency rootfs dir to be rejected, got %v", err)
	}
}

func TestValidateStartupAllowsDependencyRootFSDir(t *testing.T) {
	cfg := validStartupConfig()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = secureRootFSDir(t)
	cfg.DependencyRootFSDir = secureRootFSDir(t)

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected dependency rootfs dir to be allowed, got %v", err)
	}
}

func TestValidateStartupRejectsUnknownEnvironment(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Environment = "prod-like"

	if err := cfg.ValidateStartup(); err == nil || !strings.Contains(err.Error(), "ZGI_SANDBOX_ENV") {
		t.Fatalf("expected unknown environment to be rejected, got %v", err)
	}
}

func TestValidateStartupAllowsPreviewBackendOutsideProduction(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Environment = "local"
	cfg.RuntimeBackend = "preview"

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected local preview backend to be allowed, got %v", err)
	}
}

func TestValidateStartupAllowsSecureBackendInProduction(t *testing.T) {
	cfg := validStartupConfig()
	cfg.Environment = "prod"
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = secureRootFSDir(t)
	cfg.APIKey = "secret"

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected production secure backend to be allowed, got %v", err)
	}
}

func secureRootFSDir(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "rootfs")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create rootfs dir: %v", err)
	}
	return root
}

func validStartupConfig() Config {
	return Config{
		Port:                                   "2660",
		APIKey:                                 "",
		MaxWorkers:                             4,
		TimeoutSeconds:                         5,
		OutputLimitKB:                          1024,
		MaxActive:                              6,
		MaxConcurrentExecutions:                0,
		MaxConcurrentExecutionsPerProfile:      0,
		MaxActivePerOrganization:               0,
		MaxConcurrentExecutionsPerOrganization: 0,
		MaxExecutionsPerMinutePerOrganization:  0,
		MaxQueuedExecutionsPerOrganization:     0,
		MaxWorkspaceFiles:                      0,
		MaxWorkspaceBytes:                      0,
		MaxWorkspaceBytesPerOrganization:       0,
		MaxArtifactManifestFiles:               0,
		MaxArtifactManifestBytes:               0,
		MaxArtifactBytesPerOrganization:        0,
		MaxDependencyProfilesPerOrganization:   0,
		MaxDependencyProfileSizeBytes:          512 * 1024 * 1024,
		DependencyProfileBuildTimeoutSeconds:   600,
		QueueTimeoutMS:                         5000,
		ShutdownTimeoutSeconds:                 10,
		SessionTTL:                             1800,
		InteractiveTTL:                         3600,
		CommandTimeout:                         30,
		MaxFileSizeKB:                          256,
		ObserverRetentionDays:                  7,
		ObserverMaxEvents:                      10000,
		DatabaseURL:                            "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable",
		DataDir:                                ".zgi-sandbox-data",
		CacheTTL:                               30,
		RedisAddr:                              "",
		RedisPassword:                          "",
		RedisDB:                                0,
		WorkerID:                               "worker-a",
		AdvertiseURL:                           "http://127.0.0.1:2660",
		PublicBaseURL:                          "http://127.0.0.1:2660",
		Environment:                            "local",
		RuntimeBackend:                         "preview",
		SecureRootFS:                           "",
		DependencyRootFSDir:                    "",
		BwrapBinary:                            "bwrap",
		SecureRuntimeCPUSeconds:                2,
		SecureRuntimeMemoryBytes:               256 * 1024 * 1024,
		SecureRuntimeProcessLimit:              64,
		SecureRuntimeOpenFileLimit:             128,
		ProxyTimeout:                           20,
	}
}
