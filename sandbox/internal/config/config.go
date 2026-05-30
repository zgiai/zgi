package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                   string
	APIKey                 string
	MaxWorkers             int
	TimeoutSeconds         int
	OutputLimitKB          int
	MaxActive              int
	QueueTimeoutMS         int
	ShutdownTimeoutSeconds int
	SessionTTL             int
	InteractiveTTL         int
	CommandTimeout         int
	MaxFileSizeKB          int
	ObserverRetentionDays  int
	ObserverMaxEvents      int
	DatabaseURL            string
	DataDir                string
	CacheTTL               int
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	WorkerID               string
	AdvertiseURL           string
	PublicBaseURL          string
	Environment            string
	RuntimeBackend         string
	SecureRootFS           string
	BwrapBinary            string
	ProxyTimeout           int
}

func FromEnv() Config {
	port := getEnv("ZGI_SANDBOX_SERVER_PORT", "2660")
	workerID := getEnv("ZGI_SANDBOX_WORKER_ID", defaultWorkerID())
	advertiseURL := getEnv("ZGI_SANDBOX_ADVERTISE_URL", fmt.Sprintf("http://127.0.0.1:%s", port))

	return Config{
		Port:                   port,
		APIKey:                 getEnv("ZGI_SANDBOX_API_KEY", ""),
		MaxWorkers:             getEnvInt("ZGI_SANDBOX_LITE_MAX_WORKERS", 4),
		TimeoutSeconds:         getEnvInt("ZGI_SANDBOX_LITE_WORKER_TIMEOUT", 5),
		OutputLimitKB:          getEnvInt("ZGI_SANDBOX_OUTPUT_LIMIT_KB", 1024),
		MaxActive:              getEnvInt("ZGI_SANDBOX_MAX_ACTIVE", 6),
		QueueTimeoutMS:         getEnvInt("ZGI_SANDBOX_QUEUE_TIMEOUT_MS", 5000),
		ShutdownTimeoutSeconds: getEnvInt("ZGI_SANDBOX_SHUTDOWN_TIMEOUT_SECONDS", 10),
		SessionTTL:             getEnvInt("ZGI_SANDBOX_SESSION_TTL_SECONDS", 1800),
		InteractiveTTL:         getEnvInt("ZGI_SANDBOX_INTERACTIVE_TTL_SECONDS", 3600),
		CommandTimeout:         getEnvInt("ZGI_SANDBOX_COMMAND_TIMEOUT_SECONDS", 30),
		MaxFileSizeKB:          getEnvInt("ZGI_SANDBOX_MAX_FILE_SIZE_KB", 256),
		ObserverRetentionDays:  getEnvInt("ZGI_SANDBOX_OBSERVER_RETENTION_DAYS", 7),
		ObserverMaxEvents:      getEnvInt("ZGI_SANDBOX_OBSERVER_MAX_EVENTS", 10000),
		DatabaseURL:            getEnv("ZGI_SANDBOX_DATABASE_URL", "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable"),
		DataDir:                getEnv("ZGI_SANDBOX_DATA_DIR", ".zgi-sandbox-data"),
		CacheTTL:               getEnvInt("ZGI_SANDBOX_CACHE_TTL_SECONDS", 30),
		RedisAddr:              getEnv("ZGI_SANDBOX_REDIS_ADDR", ""),
		RedisPassword:          getEnv("ZGI_SANDBOX_REDIS_PASSWORD", ""),
		RedisDB:                getEnvIntAllowZero("ZGI_SANDBOX_REDIS_DB", 0),
		WorkerID:               workerID,
		AdvertiseURL:           advertiseURL,
		PublicBaseURL:          getEnv("ZGI_SANDBOX_PUBLIC_BASE_URL", advertiseURL),
		Environment:            getEnv("ZGI_SANDBOX_ENV", "local"),
		RuntimeBackend:         getEnv("ZGI_SANDBOX_RUNTIME_BACKEND", "preview"),
		SecureRootFS:           getEnv("ZGI_SANDBOX_SECURE_ROOTFS", ""),
		BwrapBinary:            getEnv("ZGI_SANDBOX_BWRAP_BINARY", "bwrap"),
		ProxyTimeout:           getEnvInt("ZGI_SANDBOX_PROXY_TIMEOUT_SECONDS", 20),
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

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "zgi-sandbox-local"
	}
	return hostname
}
