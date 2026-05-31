package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                                   string
	APIKey                                 string
	MaxWorkers                             int
	TimeoutSeconds                         int
	OutputLimitKB                          int
	MaxActive                              int
	MaxConcurrentExecutions                int
	MaxConcurrentExecutionsPerProfile      int
	MaxActivePerOrganization               int
	MaxConcurrentExecutionsPerOrganization int
	MaxExecutionsPerMinutePerOrganization  int
	MaxQueuedExecutionsPerOrganization     int
	MaxWorkspaceFiles                      int
	MaxWorkspaceBytes                      int64
	MaxWorkspaceBytesPerOrganization       int64
	MaxArtifactManifestFiles               int
	MaxArtifactManifestBytes               int64
	MaxArtifactBytesPerOrganization        int64
	MaxDependencyProfilesPerOrganization   int
	MaxDependencyProfileSizeBytes          int64
	DependencyProfileBuildTimeoutSeconds   int
	QueueTimeoutMS                         int
	ShutdownTimeoutSeconds                 int
	SessionTTL                             int
	InteractiveTTL                         int
	CommandTimeout                         int
	MaxFileSizeKB                          int
	ObserverRetentionDays                  int
	ObserverMaxEvents                      int
	DatabaseURL                            string
	DataDir                                string
	CacheTTL                               int
	RedisAddr                              string
	RedisPassword                          string
	RedisDB                                int
	WorkerID                               string
	AdvertiseURL                           string
	PublicBaseURL                          string
	Environment                            string
	RuntimeBackend                         string
	SecureRootFS                           string
	BwrapBinary                            string
	ProxyTimeout                           int
}

func FromEnv() Config {
	port := getEnv("ZGI_SANDBOX_SERVER_PORT", "2660")
	workerID := getEnv("ZGI_SANDBOX_WORKER_ID", defaultWorkerID())
	advertiseURL := getEnv("ZGI_SANDBOX_ADVERTISE_URL", fmt.Sprintf("http://127.0.0.1:%s", port))

	return Config{
		Port:                                   port,
		APIKey:                                 getEnv("ZGI_SANDBOX_API_KEY", ""),
		MaxWorkers:                             getEnvInt("ZGI_SANDBOX_LITE_MAX_WORKERS", 4),
		TimeoutSeconds:                         getEnvInt("ZGI_SANDBOX_LITE_WORKER_TIMEOUT", 5),
		OutputLimitKB:                          getEnvInt("ZGI_SANDBOX_OUTPUT_LIMIT_KB", 1024),
		MaxActive:                              getEnvInt("ZGI_SANDBOX_MAX_ACTIVE", 6),
		MaxConcurrentExecutions:                getEnvIntAllowZero("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS", 0),
		MaxConcurrentExecutionsPerProfile:      getEnvIntAllowZero("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_PROFILE", 0),
		MaxActivePerOrganization:               getEnvIntAllowZero("ZGI_SANDBOX_MAX_ACTIVE_PER_ORGANIZATION", 0),
		MaxConcurrentExecutionsPerOrganization: getEnvIntAllowZero("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION", 0),
		MaxExecutionsPerMinutePerOrganization:  getEnvIntAllowZero("ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION", 0),
		MaxQueuedExecutionsPerOrganization:     getEnvIntAllowZero("ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION", 0),
		MaxWorkspaceFiles:                      getEnvIntAllowZero("ZGI_SANDBOX_MAX_WORKSPACE_FILES", 0),
		MaxWorkspaceBytes:                      getEnvInt64AllowZero("ZGI_SANDBOX_MAX_WORKSPACE_BYTES", 0),
		MaxWorkspaceBytesPerOrganization:       getEnvInt64AllowZero("ZGI_SANDBOX_MAX_WORKSPACE_BYTES_PER_ORGANIZATION", 0),
		MaxArtifactManifestFiles:               getEnvIntAllowZero("ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_FILES", 0),
		MaxArtifactManifestBytes:               getEnvInt64AllowZero("ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_BYTES", 0),
		MaxArtifactBytesPerOrganization:        getEnvInt64AllowZero("ZGI_SANDBOX_MAX_ARTIFACT_BYTES_PER_ORGANIZATION", 0),
		MaxDependencyProfilesPerOrganization:   getEnvIntAllowZero("ZGI_SANDBOX_MAX_DEPENDENCY_PROFILES_PER_ORGANIZATION", 0),
		MaxDependencyProfileSizeBytes:          getEnvInt64("ZGI_SANDBOX_MAX_DEPENDENCY_PROFILE_SIZE_BYTES", 512*1024*1024),
		DependencyProfileBuildTimeoutSeconds:   getEnvInt("ZGI_SANDBOX_DEPENDENCY_PROFILE_BUILD_TIMEOUT_SECONDS", 600),
		QueueTimeoutMS:                         getEnvInt("ZGI_SANDBOX_QUEUE_TIMEOUT_MS", 5000),
		ShutdownTimeoutSeconds:                 getEnvInt("ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS", 10),
		SessionTTL:                             getEnvInt("ZGI_SANDBOX_SESSION_TTL_SECONDS", 1800),
		InteractiveTTL:                         getEnvInt("ZGI_SANDBOX_INTERACTIVE_TTL_SECONDS", 3600),
		CommandTimeout:                         getEnvInt("ZGI_SANDBOX_COMMAND_TIMEOUT_SECONDS", 30),
		MaxFileSizeKB:                          getEnvInt("ZGI_SANDBOX_MAX_FILE_SIZE_KB", 256),
		ObserverRetentionDays:                  getEnvInt("ZGI_SANDBOX_OBSERVER_RETENTION_DAYS", 7),
		ObserverMaxEvents:                      getEnvInt("ZGI_SANDBOX_OBSERVER_MAX_EVENTS", 10000),
		DatabaseURL:                            getEnv("ZGI_SANDBOX_DATABASE_URL", "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable"),
		DataDir:                                getEnv("ZGI_SANDBOX_DATA_DIR", ".zgi-sandbox-data"),
		CacheTTL:                               getEnvInt("ZGI_SANDBOX_CACHE_TTL_SECONDS", 30),
		RedisAddr:                              getEnv("ZGI_SANDBOX_REDIS_ADDR", ""),
		RedisPassword:                          getEnv("ZGI_SANDBOX_REDIS_PASSWORD", ""),
		RedisDB:                                getEnvIntAllowZero("ZGI_SANDBOX_REDIS_DB", 0),
		WorkerID:                               workerID,
		AdvertiseURL:                           advertiseURL,
		PublicBaseURL:                          getEnv("ZGI_SANDBOX_PUBLIC_BASE_URL", advertiseURL),
		Environment:                            getEnv("ZGI_SANDBOX_ENV", "local"),
		RuntimeBackend:                         getEnv("ZGI_SANDBOX_RUNTIME_BACKEND", "preview"),
		SecureRootFS:                           getEnv("ZGI_SANDBOX_SECURE_ROOTFS", ""),
		BwrapBinary:                            getEnv("ZGI_SANDBOX_BWRAP_BINARY", "bwrap"),
		ProxyTimeout:                           getEnvInt("ZGI_SANDBOX_PROXY_TIMEOUT_SECONDS", 20),
	}
}

func (c Config) ValidateStartup() error {
	var validationErrors []error
	if err := validatePort(c.Port); err != nil {
		validationErrors = append(validationErrors, err)
	}
	validationErrors = append(validationErrors,
		requirePositiveInt("ZGI_SANDBOX_LITE_MAX_WORKERS", c.MaxWorkers),
		requirePositiveInt("ZGI_SANDBOX_LITE_WORKER_TIMEOUT", c.TimeoutSeconds),
		requirePositiveInt("ZGI_SANDBOX_OUTPUT_LIMIT_KB", c.OutputLimitKB),
		requirePositiveInt("ZGI_SANDBOX_MAX_ACTIVE", c.MaxActive),
		requirePositiveInt("ZGI_SANDBOX_QUEUE_TIMEOUT_MS", c.QueueTimeoutMS),
		requirePositiveInt("ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS", c.ShutdownTimeoutSeconds),
		requirePositiveInt("ZGI_SANDBOX_SESSION_TTL_SECONDS", c.SessionTTL),
		requirePositiveInt("ZGI_SANDBOX_INTERACTIVE_TTL_SECONDS", c.InteractiveTTL),
		requirePositiveInt("ZGI_SANDBOX_COMMAND_TIMEOUT_SECONDS", c.CommandTimeout),
		requirePositiveInt("ZGI_SANDBOX_MAX_FILE_SIZE_KB", c.MaxFileSizeKB),
		requirePositiveInt("ZGI_SANDBOX_CACHE_TTL_SECONDS", c.CacheTTL),
		requirePositiveInt("ZGI_SANDBOX_PROXY_TIMEOUT_SECONDS", c.ProxyTimeout),
		requirePositiveInt64("ZGI_SANDBOX_MAX_DEPENDENCY_PROFILE_SIZE_BYTES", c.MaxDependencyProfileSizeBytes),
		requirePositiveInt("ZGI_SANDBOX_DEPENDENCY_PROFILE_BUILD_TIMEOUT_SECONDS", c.DependencyProfileBuildTimeoutSeconds),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS", c.MaxConcurrentExecutions),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_PROFILE", c.MaxConcurrentExecutionsPerProfile),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_ACTIVE_PER_ORGANIZATION", c.MaxActivePerOrganization),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_CONCURRENT_EXECUTIONS_PER_ORGANIZATION", c.MaxConcurrentExecutionsPerOrganization),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION", c.MaxExecutionsPerMinutePerOrganization),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_QUEUED_EXECUTIONS_PER_ORGANIZATION", c.MaxQueuedExecutionsPerOrganization),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_WORKSPACE_FILES", c.MaxWorkspaceFiles),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_FILES", c.MaxArtifactManifestFiles),
		requireNonNegativeInt("ZGI_SANDBOX_MAX_DEPENDENCY_PROFILES_PER_ORGANIZATION", c.MaxDependencyProfilesPerOrganization),
		requireNonNegativeInt("ZGI_SANDBOX_OBSERVER_RETENTION_DAYS", c.ObserverRetentionDays),
		requireNonNegativeInt("ZGI_SANDBOX_OBSERVER_MAX_EVENTS", c.ObserverMaxEvents),
		requireNonNegativeInt("ZGI_SANDBOX_REDIS_DB", c.RedisDB),
		requireNonNegativeInt64("ZGI_SANDBOX_MAX_WORKSPACE_BYTES", c.MaxWorkspaceBytes),
		requireNonNegativeInt64("ZGI_SANDBOX_MAX_WORKSPACE_BYTES_PER_ORGANIZATION", c.MaxWorkspaceBytesPerOrganization),
		requireNonNegativeInt64("ZGI_SANDBOX_MAX_ARTIFACT_MANIFEST_BYTES", c.MaxArtifactManifestBytes),
		requireNonNegativeInt64("ZGI_SANDBOX_MAX_ARTIFACT_BYTES_PER_ORGANIZATION", c.MaxArtifactBytesPerOrganization),
	)
	if err := validateEnvironment(c.Environment); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if err := validateRuntimeBackend(c.RuntimeBackendName()); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if strings.TrimSpace(c.DataDir) == "" {
		validationErrors = append(validationErrors, errors.New("ZGI_SANDBOX_DATA_DIR must not be empty"))
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		validationErrors = append(validationErrors, errors.New("ZGI_SANDBOX_WORKER_ID must not be empty"))
	}
	if err := validateHTTPURL("ZGI_SANDBOX_ADVERTISE_URL", c.AdvertiseURL); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if err := validateHTTPURL("ZGI_SANDBOX_PUBLIC_BASE_URL", c.PublicBaseURL); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if c.RuntimeBackendName() == "linux-secure" {
		if err := validateSecureRootFS(c.SecureRootFS); err != nil {
			validationErrors = append(validationErrors, err)
		}
		if strings.TrimSpace(c.BwrapBinary) == "" {
			validationErrors = append(validationErrors, errors.New("ZGI_SANDBOX_BWRAP_BINARY is required when ZGI_SANDBOX_RUNTIME_BACKEND=linux-secure"))
		}
	}
	if c.IsProduction() && strings.TrimSpace(c.APIKey) == "" {
		validationErrors = append(validationErrors, errors.New("production sandbox deployments require ZGI_SANDBOX_API_KEY"))
	}
	if c.IsProduction() && !c.NetworkPolicyEnforced() {
		validationErrors = append(validationErrors, errors.New("production sandbox deployments require a runtime backend that enforces network policy"))
	}
	return errors.Join(compactErrors(validationErrors)...)
}

func (c Config) IsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(c.Environment)) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

func (c Config) RuntimeBackendName() string {
	switch strings.ToLower(strings.TrimSpace(c.RuntimeBackend)) {
	case "", "preview", "process", "preview-process":
		return "preview-process"
	case "linux-secure":
		return "linux-secure"
	default:
		return strings.ToLower(strings.TrimSpace(c.RuntimeBackend))
	}
}

func (c Config) NetworkPolicyEnforced() bool {
	return c.RuntimeBackendName() == "linux-secure"
}

func validatePort(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("ZGI_SANDBOX_SERVER_PORT must not be empty")
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("ZGI_SANDBOX_SERVER_PORT must be between 1 and 65535")
	}
	return nil
}

func validateEnvironment(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "dev", "development", "test", "staging", "production", "prod":
		return nil
	default:
		return fmt.Errorf("ZGI_SANDBOX_ENV must be local, dev, development, test, staging, production, or prod")
	}
}

func validateRuntimeBackend(value string) error {
	switch value {
	case "preview-process", "linux-secure":
		return nil
	default:
		return fmt.Errorf("ZGI_SANDBOX_RUNTIME_BACKEND is unsupported: %s", value)
	}
}

func validateSecureRootFS(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("ZGI_SANDBOX_SECURE_ROOTFS is required when ZGI_SANDBOX_RUNTIME_BACKEND=linux-secure")
	}
	if !filepath.IsAbs(value) {
		return errors.New("ZGI_SANDBOX_SECURE_ROOTFS must be an absolute path")
	}
	info, err := os.Lstat(value)
	if err != nil {
		return fmt.Errorf("ZGI_SANDBOX_SECURE_ROOTFS must reference an existing directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("ZGI_SANDBOX_SECURE_ROOTFS must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New("ZGI_SANDBOX_SECURE_ROOTFS must be a directory")
	}
	if info.Mode().Perm()&0o002 != 0 {
		return errors.New("ZGI_SANDBOX_SECURE_ROOTFS must not be world-writable")
	}
	return nil
}

func validateHTTPURL(name string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute http or https URL", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	return nil
}

func requirePositiveInt(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	return nil
}

func requirePositiveInt64(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	return nil
}

func requireNonNegativeInt(name string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s must be greater than or equal to 0", name)
	}
	return nil
}

func requireNonNegativeInt64(name string, value int64) error {
	if value < 0 {
		return fmt.Errorf("%s must be greater than or equal to 0", name)
	}
	return nil
}

func compactErrors(values []error) []error {
	errs := make([]error, 0, len(values))
	for _, err := range values {
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (c Config) PublicSnapshot() map[string]any {
	return map[string]any{
		"port":                                  c.Port,
		"max_workers":                           c.MaxWorkers,
		"timeout_seconds":                       c.TimeoutSeconds,
		"output_limit_kb":                       c.OutputLimitKB,
		"max_active":                            c.MaxActive,
		"max_concurrent_executions":             c.MaxConcurrentExecutions,
		"max_concurrent_executions_per_profile": c.MaxConcurrentExecutionsPerProfile,
		"max_active_per_organization":           c.MaxActivePerOrganization,
		"max_concurrent_executions_per_organization": c.MaxConcurrentExecutionsPerOrganization,
		"max_executions_per_minute_per_organization": c.MaxExecutionsPerMinutePerOrganization,
		"max_queued_executions_per_organization":     c.MaxQueuedExecutionsPerOrganization,
		"max_workspace_files":                        c.MaxWorkspaceFiles,
		"max_workspace_bytes":                        c.MaxWorkspaceBytes,
		"max_workspace_bytes_per_organization":       c.MaxWorkspaceBytesPerOrganization,
		"max_artifact_manifest_files":                c.MaxArtifactManifestFiles,
		"max_artifact_manifest_bytes":                c.MaxArtifactManifestBytes,
		"max_artifact_bytes_per_organization":        c.MaxArtifactBytesPerOrganization,
		"max_dependency_profiles_per_organization":   c.MaxDependencyProfilesPerOrganization,
		"max_dependency_profile_size_bytes":          c.MaxDependencyProfileSizeBytes,
		"dependency_profile_build_timeout_seconds":   c.DependencyProfileBuildTimeoutSeconds,
		"queue_timeout_ms":                           c.QueueTimeoutMS,
		"shutdown_timeout_seconds":                   c.ShutdownTimeoutSeconds,
		"session_ttl_seconds":                        c.SessionTTL,
		"interactive_ttl_seconds":                    c.InteractiveTTL,
		"command_timeout_seconds":                    c.CommandTimeout,
		"max_file_size_kb":                           c.MaxFileSizeKB,
		"observer_retention_days":                    c.ObserverRetentionDays,
		"observer_max_events":                        c.ObserverMaxEvents,
		"database_configured":                        c.DatabaseURL != "",
		"data_dir":                                   c.DataDir,
		"cache_ttl_seconds":                          c.CacheTTL,
		"redis_configured":                           c.RedisAddr != "",
		"redis_db":                                   c.RedisDB,
		"worker_id":                                  c.WorkerID,
		"advertise_url":                              c.AdvertiseURL,
		"public_base_url":                            c.PublicBaseURL,
		"environment":                                c.Environment,
		"runtime_backend":                            c.RuntimeBackendName(),
		"secure_rootfs_configured":                   c.SecureRootFS != "",
		"bwrap_binary":                               c.BwrapBinary,
		"proxy_timeout_seconds":                      c.ProxyTimeout,
		"network_policy_enforced":                    c.NetworkPolicyEnforced(),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return fallback
		}
		parsed = parsed*10 + int(char-'0')
	}

	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvIntAllowZero(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return fallback
		}
		parsed = parsed*10 + int(char-'0')
	}

	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	var parsed int64
	for _, char := range value {
		if char < '0' || char > '9' {
			return fallback
		}
		parsed = parsed*10 + int64(char-'0')
	}

	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvInt64AllowZero(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	var parsed int64
	for _, char := range value {
		if char < '0' || char > '9' {
			return fallback
		}
		parsed = parsed*10 + int64(char-'0')
	}

	return parsed
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "zgi-sandbox-local"
	}
	return hostname
}
