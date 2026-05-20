package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds every tunable for the plugin executor.
// Only the settings required for the local runtime are enabled for now.
type Config struct {
	// HTTP server
	HTTPPort int `envconfig:"HTTP_PORT" default:"14000"`

	// Path where plugins are stored/executed.
	PluginHome       string `envconfig:"PLUGIN_HOME" required:"true"`
	PackageCachePath string `envconfig:"PACKAGE_CACHE_PATH"`
	WorkspacePath    string `envconfig:"WORKSPACE_PATH"`

	// Local runtime settings
	PythonInterpreter              string        `envconfig:"PYTHON_INTERPRETER" default:"python3"`
	NodeInterpreter                string        `envconfig:"NODE_INTERPRETER" default:"node"`
	PipCommand                     string        `envconfig:"PIP_COMMAND" default:"pip"`
	NPMCommand                     string        `envconfig:"NPM_COMMAND" default:"npm"`
	UVPath                         string        `envconfig:"UV_PATH" default:""`
	PythonEnvInitTimeout           time.Duration `envconfig:"PYTHON_ENV_INIT_TIMEOUT" default:"10m"`
	PythonCompileExtra             string        `envconfig:"PYTHON_COMPILE_EXTRA_ARGS" default:""`
	PipMirrorURL                   string        `envconfig:"PIP_MIRROR_URL" default:""`
	PipPreferBinary                bool          `envconfig:"PIP_PREFER_BINARY" default:"false"`
	PipVerbose                     bool          `envconfig:"PIP_VERBOSE" default:"false"`
	PipExtraArgs                   string        `envconfig:"PIP_EXTRA_ARGS" default:""`
	StdoutBufferSize               int           `envconfig:"STDOUT_BUFFER_SIZE" default:"4096"`        // default 4 KiB
	StdoutMaxBufferSize            int           `envconfig:"STDOUT_MAX_BUFFER_SIZE" default:"5242880"` // default 5 MiB
	ShutdownTimeout                time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"15s"`
	InstallTimeout                 time.Duration `envconfig:"INSTALL_TIMEOUT" default:"2m"`
	MaxPackageSize                 int64         `envconfig:"MAX_PACKAGE_SIZE_BYTES" default:"0"` // 0 = unlimited
	MaxConcurrentRuns              int           `envconfig:"MAX_CONCURRENT_RUNS" default:"0"`    // 0 = unlimited
	SessionSweepIntervalSeconds    int           `envconfig:"SESSION_SWEEP_INTERVAL_SECONDS" default:"60"`
	ReuseSessionIdleTTLSeconds     int           `envconfig:"REUSE_SESSION_IDLE_TTL_SECONDS" default:"600"`
	ReuseSessionMaxLifetimeSeconds int           `envconfig:"REUSE_SESSION_MAX_LIFETIME_SECONDS" default:"3600"`

	// Access control
	APIKey                   string `envconfig:"API_KEY"`
	AdminApiEnabled          bool   `envconfig:"ADMIN_API_ENABLED" default:"false"`
	AdminAPIKeys             string `envconfig:"ADMIN_API_KEYS"`
	ReadonlyAPIKeys          string `envconfig:"READONLY_API_KEYS"`
	MultiTenantEnabled       bool   `envconfig:"MULTI_TENANT_ENABLED" default:"false"`
	RequireManifestSignature bool   `envconfig:"REQUIRE_MANIFEST_SIGNATURE" default:"false"`
	SignaturePublicKeyPath   string `envconfig:"SIGNATURE_PUBLIC_KEY_PATH"`
	SignatureAlgorithm       string `envconfig:"SIGNATURE_ALGORITHM" default:"rsa"`
	RateLimitPerMinute       int    `envconfig:"RATE_LIMIT_PER_MINUTE" default:"0"`
	TenantRateLimitPerMinute int    `envconfig:"TENANT_RATE_LIMIT_PER_MINUTE" default:"0"`

	// Data-plane database
	DBDriver          string        `envconfig:"DB_DRIVER" default:"postgres"`
	DBHost            string        `envconfig:"DB_HOST"`
	DBPort            int           `envconfig:"DB_PORT" default:"5432"`
	DBUser            string        `envconfig:"DB_USER"`
	DBPassword        string        `envconfig:"DB_PASSWORD"`
	DBName            string        `envconfig:"DB_NAME"`
	DBSSLMode         string        `envconfig:"DB_SSL_MODE" default:"disable"`
	DBOptions         string        `envconfig:"DB_OPTIONS"`
	DBMaxIdleConns    int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	DBMaxOpenConns    int           `envconfig:"DB_MAX_OPEN_CONNS" default:"20"`
	DBConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"1h"`
	DBConnMaxIdleTime time.Duration `envconfig:"DB_CONN_MAX_IDLE_TIME" default:"10m"`

	// Data-plane cache (Redis)
	RedisHost             string        `envconfig:"REDIS_HOST"`
	RedisPort             int           `envconfig:"REDIS_PORT" default:"6379"`
	RedisUsername         string        `envconfig:"REDIS_USERNAME"`
	RedisPassword         string        `envconfig:"REDIS_PASSWORD"`
	RedisDB               int           `envconfig:"REDIS_DB" default:"0"`
	RedisUseTLS           bool          `envconfig:"REDIS_USE_TLS" default:"false"`
	RedisSentinelEnabled  bool          `envconfig:"REDIS_SENTINEL_ENABLED" default:"false"`
	RedisSentinelMaster   string        `envconfig:"REDIS_SENTINEL_MASTER"`
	RedisSentinelAddrs    string        `envconfig:"REDIS_SENTINEL_ADDRS"`
	RedisSentinelUsername string        `envconfig:"REDIS_SENTINEL_USERNAME"`
	RedisSentinelPassword string        `envconfig:"REDIS_SENTINEL_PASSWORD"`
	RedisDialTimeout      time.Duration `envconfig:"REDIS_DIAL_TIMEOUT" default:"5s"`
	RedisReadTimeout      time.Duration `envconfig:"REDIS_READ_TIMEOUT" default:"3s"`
	RedisWriteTimeout     time.Duration `envconfig:"REDIS_WRITE_TIMEOUT" default:"3s"`
	RedisKeyPrefix        string        `envconfig:"REDIS_KEY_PREFIX" default:"plugin_runner"`

	// Proxy settings forwarded to the child process.
	HTTPProxy  string `envconfig:"HTTP_PROXY"`
	HTTPSProxy string `envconfig:"HTTPS_PROXY"`
	NoProxy    string `envconfig:"NO_PROXY"`
}

// Load reads environment variables (optionally from .env files) and builds the Config.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("EXECUTOR", &cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if cfg.WorkspacePath == "" {
		cfg.WorkspacePath = cfg.PluginHome
	}
	if cfg.PackageCachePath == "" {
		cfg.PackageCachePath = cfg.PluginHome
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate ensures essential values exist.
func (c *Config) Validate() error {
	if c.PluginHome == "" {
		return fmt.Errorf("plugin home is required")
	}
	if c.WorkspacePath == "" {
		return fmt.Errorf("workspace path is required")
	}
	if c.PackageCachePath == "" {
		return fmt.Errorf("package cache path is required")
	}
	if c.StdoutBufferSize <= 0 {
		return fmt.Errorf("stdout buffer size must be positive")
	}
	if c.StdoutMaxBufferSize <= 0 {
		return fmt.Errorf("stdout max buffer size must be positive")
	}
	if c.StdoutMaxBufferSize < c.StdoutBufferSize {
		return fmt.Errorf("stdout max buffer must be >= buffer size")
	}
	if c.HTTPPort <= 0 {
		return fmt.Errorf("http port must be positive")
	}
	if strings.TrimSpace(c.PythonInterpreter) == "" {
		return fmt.Errorf("python interpreter is required")
	}
	if strings.TrimSpace(c.NodeInterpreter) == "" {
		return fmt.Errorf("node interpreter is required")
	}
	if strings.TrimSpace(c.PipCommand) == "" {
		return fmt.Errorf("pip command is required")
	}
	if strings.TrimSpace(c.NPMCommand) == "" {
		return fmt.Errorf("npm command is required")
	}
	if c.PythonEnvInitTimeout <= 0 {
		return fmt.Errorf("python env init timeout must be positive")
	}
	if c.InstallTimeout <= 0 {
		return fmt.Errorf("install timeout must be positive")
	}
	if c.MaxPackageSize < 0 {
		return fmt.Errorf("max package size cannot be negative")
	}
	if c.MaxConcurrentRuns < 0 {
		return fmt.Errorf("max concurrent runs cannot be negative")
	}
	if c.SessionSweepIntervalSeconds <= 0 {
		return fmt.Errorf("session sweep interval seconds must be positive")
	}
	if c.ReuseSessionIdleTTLSeconds <= 0 {
		return fmt.Errorf("reuse session idle ttl seconds must be positive")
	}
	if c.ReuseSessionMaxLifetimeSeconds <= 0 {
		return fmt.Errorf("reuse session max lifetime seconds must be positive")
	}
	if c.RequireManifestSignature && strings.TrimSpace(c.SignaturePublicKeyPath) == "" {
		return fmt.Errorf("signature public key path is required when signature check is enabled")
	}
	if c.RateLimitPerMinute < 0 || c.TenantRateLimitPerMinute < 0 {
		return fmt.Errorf("rate limits cannot be negative")
	}

	if err := c.validateDatabase(); err != nil {
		return err
	}
	if err := c.validateRedis(); err != nil {
		return err
	}
	return nil
}

// DatabaseEnabled reports whether the DB settings are filled enough to open a connection.
func (c *Config) DatabaseEnabled() bool {
	return c.DBHost != ""
}

// RedisEnabled reports whether Redis (single node or sentinel) should be initialized.
func (c *Config) RedisEnabled() bool {
	if c.RedisSentinelEnabled {
		return c.RedisSentinelMaster != "" && c.RedisSentinelAddrs != ""
	}
	return c.RedisHost != ""
}

func (c *Config) validateDatabase() error {
	if !c.DatabaseEnabled() {
		return nil
	}

	driver := strings.ToLower(c.DBDriver)
	switch driver {
	case "postgres", "postgresql", "mysql":
	default:
		return fmt.Errorf("unsupported db driver %q", c.DBDriver)
	}

	if c.DBHost == "" {
		return fmt.Errorf("db host is required when database is enabled")
	}
	if c.DBPort <= 0 {
		return fmt.Errorf("db port must be positive")
	}
	if c.DBUser == "" {
		return fmt.Errorf("db user is required when database is enabled")
	}
	if c.DBName == "" {
		return fmt.Errorf("db name is required when database is enabled")
	}

	if c.DBMaxIdleConns < 0 {
		return fmt.Errorf("db max idle connections cannot be negative")
	}
	if c.DBMaxOpenConns < 0 {
		return fmt.Errorf("db max open connections cannot be negative")
	}
	if c.DBConnMaxLifetime < 0 {
		return fmt.Errorf("db connection max lifetime cannot be negative")
	}
	if c.DBConnMaxIdleTime < 0 {
		return fmt.Errorf("db connection max idle time cannot be negative")
	}

	sslMode := strings.ToLower(c.DBSSLMode)
	switch sslMode {
	case "disable", "require", "verify-ca", "verify-full", "":
	default:
		return fmt.Errorf("db ssl mode %q is not supported", c.DBSSLMode)
	}
	return nil
}

func (c *Config) validateRedis() error {
	if c.RedisDB < 0 {
		return fmt.Errorf("redis db cannot be negative")
	}
	if c.RedisPort < 0 {
		return fmt.Errorf("redis port cannot be negative")
	}

	if !c.RedisEnabled() {
		return nil
	}

	if c.RedisSentinelEnabled {
		if c.RedisSentinelMaster == "" {
			return fmt.Errorf("redis sentinel master is required when sentinel is enabled")
		}
		if c.RedisSentinelAddrs == "" {
			return fmt.Errorf("redis sentinel addresses are required when sentinel is enabled")
		}
		return nil
	}

	if c.RedisHost == "" {
		return fmt.Errorf("redis host is required when redis is enabled")
	}
	return nil
}
