package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                                  string
	APIKey                                string
	MaxWorkers                            int
	TimeoutSeconds                        int
	OutputLimitKB                         int
	MaxActive                             int
	MaxActivePerOrganization              int
	MaxExecutionsPerMinutePerOrganization int
	MaxWorkspaceBytes                     int64
	QueueTimeoutMS                        int
	ShutdownTimeoutSeconds                int
	SessionTTL                            int
	InteractiveTTL                        int
	CommandTimeout                        int
	MaxFileSizeKB                         int
	ObserverRetentionDays                 int
	ObserverMaxEvents                     int
	DatabaseURL                           string
	DataDir                               string
	CacheTTL                              int
	RedisAddr                             string
	RedisPassword                         string
	RedisDB                               int
	WorkerID                              string
	AdvertiseURL                          string
	PublicBaseURL                         string
	Environment                           string
	RuntimeBackend                        string
	SecureRootFS                          string
	BwrapBinary                           string
	ProxyTimeout                          int
}

func FromEnv() Config {
	port := getEnv("ZGI_SANDBOX_SERVER_PORT", "2660")
	workerID := getEnv("ZGI_SANDBOX_WORKER_ID", defaultWorkerID())
	advertiseURL := getEnv("ZGI_SANDBOX_ADVERTISE_URL", fmt.Sprintf("http://127.0.0.1:%s", port))

	return Config{
		Port:                                  port,
		APIKey:                                getEnv("ZGI_SANDBOX_API_KEY", ""),
		MaxWorkers:                            getEnvInt("ZGI_SANDBOX_LITE_MAX_WORKERS", 4),
		TimeoutSeconds:                        getEnvInt("ZGI_SANDBOX_LITE_WORKER_TIMEOUT", 5),
		OutputLimitKB:                         getEnvInt("ZGI_SANDBOX_OUTPUT_LIMIT_KB", 1024),
		MaxActive:                             getEnvInt("ZGI_SANDBOX_MAX_ACTIVE", 6),
		MaxActivePerOrganization:              getEnvIntAllowZero("ZGI_SANDBOX_MAX_ACTIVE_PER_ORGANIZATION", 0),
		MaxExecutionsPerMinutePerOrganization: getEnvIntAllowZero("ZGI_SANDBOX_MAX_EXECUTIONS_PER_MINUTE_PER_ORGANIZATION", 0),
		MaxWorkspaceBytes:                     getEnvInt64AllowZero("ZGI_SANDBOX_MAX_WORKSPACE_BYTES", 0),
		QueueTimeoutMS:                        getEnvInt("ZGI_SANDBOX_QUEUE_TIMEOUT_MS", 5000),
		ShutdownTimeoutSeconds:                getEnvInt("ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS", 10),
		SessionTTL:                            getEnvInt("ZGI_SANDBOX_SESSION_TTL_SECONDS", 1800),
		InteractiveTTL:                        getEnvInt("ZGI_SANDBOX_INTERACTIVE_TTL_SECONDS", 3600),
		CommandTimeout:                        getEnvInt("ZGI_SANDBOX_COMMAND_TIMEOUT_SECONDS", 30),
		MaxFileSizeKB:                         getEnvInt("ZGI_SANDBOX_MAX_FILE_SIZE_KB", 256),
		ObserverRetentionDays:                 getEnvInt("ZGI_SANDBOX_OBSERVER_RETENTION_DAYS", 7),
		ObserverMaxEvents:                     getEnvInt("ZGI_SANDBOX_OBSERVER_MAX_EVENTS", 10000),
		DatabaseURL:                           getEnv("ZGI_SANDBOX_DATABASE_URL", "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable"),
		DataDir:                               getEnv("ZGI_SANDBOX_DATA_DIR", ".zgi-sandbox-data"),
		CacheTTL:                              getEnvInt("ZGI_SANDBOX_CACHE_TTL_SECONDS", 30),
		RedisAddr:                             getEnv("ZGI_SANDBOX_REDIS_ADDR", ""),
		RedisPassword:                         getEnv("ZGI_SANDBOX_REDIS_PASSWORD", ""),
		RedisDB:                               getEnvIntAllowZero("ZGI_SANDBOX_REDIS_DB", 0),
		WorkerID:                              workerID,
		AdvertiseURL:                          advertiseURL,
		PublicBaseURL:                         getEnv("ZGI_SANDBOX_PUBLIC_BASE_URL", advertiseURL),
		Environment:                           getEnv("ZGI_SANDBOX_ENV", "local"),
		RuntimeBackend:                        getEnv("ZGI_SANDBOX_RUNTIME_BACKEND", "preview"),
		SecureRootFS:                          getEnv("ZGI_SANDBOX_SECURE_ROOTFS", ""),
		BwrapBinary:                           getEnv("ZGI_SANDBOX_BWRAP_BINARY", "bwrap"),
		ProxyTimeout:                          getEnvInt("ZGI_SANDBOX_PROXY_TIMEOUT_SECONDS", 20),
	}
}

func (c Config) ValidateStartup() error {
	if c.IsProduction() && !c.NetworkPolicyEnforced() {
		return errors.New("production sandbox deployments require a runtime backend that enforces network policy")
	}
	return nil
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

func (c Config) PublicSnapshot() map[string]any {
	return map[string]any{
		"port":                        c.Port,
		"max_workers":                 c.MaxWorkers,
		"timeout_seconds":             c.TimeoutSeconds,
		"output_limit_kb":             c.OutputLimitKB,
		"max_active":                  c.MaxActive,
		"max_active_per_organization": c.MaxActivePerOrganization,
		"max_executions_per_minute_per_organization": c.MaxExecutionsPerMinutePerOrganization,
		"max_workspace_bytes":                        c.MaxWorkspaceBytes,
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
