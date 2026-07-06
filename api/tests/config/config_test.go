package config_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
)

const (
	defaultGRPCPort = 50051
)

func TestLoadUsesDefaultGRPCServerConfig(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if !cfg.Server.GRPCEnabled {
		t.Fatal("cfg.Server.GRPCEnabled = false, want true")
	}
	if got := cfg.Server.GRPCPort; got != defaultGRPCPort {
		t.Fatalf("cfg.Server.GRPCPort = %d, want %d", got, defaultGRPCPort)
	}
}

func TestLoadReadsNamedSSOFrontendCallbacks(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                              "release",
		"ENV":                                      "production",
		"SECRET_KEY":                               "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM":             "noreply@example.com",
		"EMAIL_RESEND_API_KEY":                     "test-api-key",
		"SSO_FRONTEND_CALLBACK_URL":                "https://app.example.com/sso/callback",
		"SSO_FRONTEND_CALLBACK_URL_FC_7Q2K9M":      "https://region-a.example.com/sso/callback",
		"SSO_FRONTEND_CALLBACK_URL_FC_M4X8T2P":     "https://region-b.example.com/sso/callback",
		"SSO_FRONTEND_CALLBACK_URL_EMPTY_IGNORED":  "",
		"SSO_FRONTEND_CALLBACK_URL_TRAILING_SPACE": " https://region-c.example.com/sso/callback ",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if got := cfg.Auth.SSO.FrontendCallbackURL; got != "https://app.example.com/sso/callback" {
		t.Fatalf("cfg.Auth.SSO.FrontendCallbackURL = %q", got)
	}
	if got := cfg.Auth.SSO.FrontendCallbackURLs["FC_7Q2K9M"]; got != "https://region-a.example.com/sso/callback" {
		t.Fatalf("cfg.Auth.SSO.FrontendCallbackURLs[FC_7Q2K9M] = %q", got)
	}
	if got := cfg.Auth.SSO.FrontendCallbackURLs["FC_M4X8T2P"]; got != "https://region-b.example.com/sso/callback" {
		t.Fatalf("cfg.Auth.SSO.FrontendCallbackURLs[FC_M4X8T2P] = %q", got)
	}
	if _, ok := cfg.Auth.SSO.FrontendCallbackURLs["EMPTY_IGNORED"]; ok {
		t.Fatal("cfg.Auth.SSO.FrontendCallbackURLs contains EMPTY_IGNORED, want omitted")
	}
	if got := cfg.Auth.SSO.FrontendCallbackURLs["TRAILING_SPACE"]; got != "https://region-c.example.com/sso/callback" {
		t.Fatalf("cfg.Auth.SSO.FrontendCallbackURLs[TRAILING_SPACE] = %q", got)
	}
}

func TestLoadIgnoresInvalidGRPCPortWhenDisabled(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"GRPC_ENABLED":                 "false",
		"GRPC_PORT":                    "invalid-port",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Server.GRPCEnabled {
		t.Fatal("cfg.Server.GRPCEnabled = true, want false")
	}
	if got := cfg.Server.GRPCPort; got != defaultGRPCPort {
		t.Fatalf("cfg.Server.GRPCPort = %d, want %d", got, defaultGRPCPort)
	}
}

func TestLoadFailsOnInvalidGRPCPortWhenEnabled(t *testing.T) {
	_, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"GRPC_ENABLED":                 "true",
		"GRPC_PORT":                    "invalid-port",
	}))
	if err == nil {
		t.Fatal("config.LoadFromFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "invalid GRPC_PORT") {
		t.Fatalf("config.LoadFromFile() error = %v, want error containing %q", err, "invalid GRPC_PORT")
	}
}

func TestLoadRestoresBusinessEnvKeys(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                     "release",
		"ENV":                             "production",
		"SECRET_KEY":                      "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM":    "noreply@example.com",
		"EMAIL_RESEND_API_KEY":            "test-api-key",
		"APP_NAME":                        "ZGI-GinKit",
		"DB_DRIVER":                       "postgres",
		"LOG_LEVEL":                       "info",
		"LOG_FILENAME":                    "logs/app.log",
		"LOG_MAX_SIZE":                    "100",
		"LOG_MAX_AGE":                     "30",
		"LOG_MAX_BACKUPS":                 "7",
		"LOG_COMPRESS":                    "true",
		"WEAVIATE_GRPC_ENABLED":           "false",
		"KEYWORD_DATA_SOURCE_TYPE":        "database",
		"BATCH_UPLOAD_LIMIT":              "10",
		"EMAIL_SMTP_SERVER":               "smtp.gmail.com",
		"EMAIL_PORT":                      "587",
		"EMAIL_SMTP_USERNAME":             "user@example.com",
		"EMAIL_SMTP_PASSWORD":             "secret",
		"EMAIL_SMTP_USE_TLS":              "false",
		"EMAIL_SMTP_OPPORTUNISTIC_TLS":    "false",
		"TASK_QUEUE_RETENTION":            "24h",
		"MODELMETA_API_URL":               "https://models.zgi.ai",
		"WORKFLOW_EXECUTION_TIMEOUT":      "300",
		"WORKFLOW_LLM_TIMEOUT":            "120",
		"WORKFLOW_HEARTBEAT_INTERVAL":     "5",
		"WORKFLOW_CLEANUP_TIMEOUT":        "30",
		"LLM_CREDENTIAL_SECRET_KEY":       "test-llm-credential-secret-32!!!",
		"OFFICIAL_MODEL_SYNC_STRICT_MODE": "false",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.App.Name != "ZGI-GinKit" {
		t.Fatalf("cfg.App.Name = %q, want %q", cfg.App.Name, "ZGI-GinKit")
	}
	if cfg.Database.Driver != "postgres" {
		t.Fatalf("cfg.Database.Driver = %q, want %q", cfg.Database.Driver, "postgres")
	}
	if cfg.Log.Level != "info" || cfg.Log.Filename != "logs/app.log" {
		t.Fatalf("cfg.Log = %#v, want level info and filename logs/app.log", cfg.Log)
	}
	if cfg.Email.SMTPServer != "smtp.gmail.com" || cfg.Email.SMTPPort != 587 {
		t.Fatalf("cfg.Email SMTP = %#v, want smtp.gmail.com:587", cfg.Email)
	}
	if cfg.TaskQueue.Retention != 24*time.Hour {
		t.Fatalf("cfg.TaskQueue.Retention = %s, want 24h", cfg.TaskQueue.Retention)
	}
	if cfg.VectorStore.WeaviateGRPC || cfg.VectorStore.KeywordDataSource != "database" {
		t.Fatalf("cfg.VectorStore = %#v, want grpc false and keyword database", cfg.VectorStore)
	}
	if cfg.Upload.BatchUploadLimit != 10 {
		t.Fatalf("cfg.Upload.BatchUploadLimit = %d, want 10", cfg.Upload.BatchUploadLimit)
	}
	if cfg.ModelMeta.APIURL != "https://models.zgi.ai" {
		t.Fatalf("cfg.ModelMeta.APIURL = %q, want %q", cfg.ModelMeta.APIURL, "https://models.zgi.ai")
	}
	if cfg.Workflow.ExecutionTimeout != 300 || cfg.Workflow.LLMTimeout != 120 ||
		cfg.Workflow.HeartbeatInterval != 5 || cfg.Workflow.CleanupTimeout != 30 {
		t.Fatalf("cfg.Workflow = %#v, want restored workflow defaults", cfg.Workflow)
	}
	if cfg.Encryption.LLMCredentialSecretKey != "test-llm-credential-secret-32!!!" {
		t.Fatalf("cfg.Encryption.LLMCredentialSecretKey = %q, want restored legacy credential secret", cfg.Encryption.LLMCredentialSecretKey)
	}
	if cfg.LLM.EncryptionKey != "" {
		t.Fatalf("cfg.LLM.EncryptionKey = %q, want empty when %s is not set", cfg.LLM.EncryptionKey, "LLM_ENCRYPTION_KEY")
	}
}

func TestLoadModelMetaDefaultsToZGICuratedAPIURL(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.ModelMeta.APIURL != "https://models.zgi.ai" {
		t.Fatalf("cfg.ModelMeta.APIURL = %q, want default curated model source", cfg.ModelMeta.APIURL)
	}
}

func TestLoadUsesDefaultAnswerNodeStreamingChunkSize(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.AnswerNodeStreaming.ChunkSize != 20 {
		t.Fatalf("cfg.AnswerNodeStreaming.ChunkSize = %d, want 20", cfg.AnswerNodeStreaming.ChunkSize)
	}
}

func TestLoadOverridesAnswerNodeStreamingChunkSize(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                      "release",
		"ENV":                              "production",
		"SECRET_KEY":                       "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM":     "noreply@example.com",
		"EMAIL_RESEND_API_KEY":             "test-api-key",
		"ANSWER_NODE_STREAMING_CHUNK_SIZE": "64",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.AnswerNodeStreaming.ChunkSize != 64 {
		t.Fatalf("cfg.AnswerNodeStreaming.ChunkSize = %d, want 64", cfg.AnswerNodeStreaming.ChunkSize)
	}
}

func TestLoadDefaultsLogLevelToInfoInProductionEnvironment(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE": "debug",
		"ENV":         "production",
		"SECRET_KEY":  "test-secret",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Log.Level != "info" {
		t.Fatalf("cfg.Log.Level = %q, want info", cfg.Log.Level)
	}
}

func TestLoadDefaultsLogLevelToInfoInReleaseMode(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE": "release",
		"ENV":         "local",
		"SECRET_KEY":  "test-secret",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Log.Level != "info" {
		t.Fatalf("cfg.Log.Level = %q, want info", cfg.Log.Level)
	}
}

func TestLoadDefaultsLogLevelToDebugLocally(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE": "debug",
		"ENV":         "local",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Log.Level != "debug" {
		t.Fatalf("cfg.Log.Level = %q, want debug", cfg.Log.Level)
	}
}

func TestLoadKeepsExplicitLogLevel(t *testing.T) {
	for _, level := range []string{"debug", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
				"SERVER_MODE": "release",
				"ENV":         "production",
				"SECRET_KEY":  "test-secret",
				"LOG_LEVEL":   level,
			}))
			if err != nil {
				t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
			}

			if cfg.Log.Level != level {
				t.Fatalf("cfg.Log.Level = %q, want %q", cfg.Log.Level, level)
			}
		})
	}
}

func TestLoadLLMEncryptionKeyUsesOnlyCanonicalKey(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"API_KEY_ENCRYPTION_KEY":       "test-api-key-encryption-key-32!!",
		"LLM_CREDENTIAL_SECRET_KEY":    "test-llm-credential-secret-32!!!",
		"LLM_ENCRYPTION_KEY":           "test-llm-main-encryption-key-32!",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.LLM.EncryptionKey != "test-llm-main-encryption-key-32!" {
		t.Fatalf("cfg.LLM.EncryptionKey = %q, want canonical LLM encryption key", cfg.LLM.EncryptionKey)
	}
}

func TestLoadLLMOutboundGuardDefaults(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if !cfg.LLM.GuardOutboundURL {
		t.Fatal("cfg.LLM.GuardOutboundURL = false, want true")
	}
	if !cfg.LLM.OutboundURLGuardEnabled() {
		t.Fatal("cfg.LLM.OutboundURLGuardEnabled() = false, want true")
	}
	if cfg.LLM.GuardOutboundDNS {
		t.Fatal("cfg.LLM.GuardOutboundDNS = true, want false")
	}
}

func TestLoadLLMOutboundGuardOverrides(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"LLM_GUARD_OUTBOUND_URL":       "false",
		"LLM_GUARD_OUTBOUND_DNS":       "true",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.LLM.GuardOutboundURL {
		t.Fatal("cfg.LLM.GuardOutboundURL = true, want false")
	}
	if cfg.LLM.OutboundURLGuardEnabled() {
		t.Fatal("cfg.LLM.OutboundURLGuardEnabled() = true, want false")
	}
	if !cfg.LLM.GuardOutboundDNS {
		t.Fatal("cfg.LLM.GuardOutboundDNS = false, want true")
	}
}

func TestLoadOpenTelemetryConfig(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                 "release",
		"ENV":                         "production",
		"SECRET_KEY":                  "test-secret",
		"OTEL_ENABLED":                "true",
		"OTEL_SERVICE_NAME":           "zgi-api-test",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "https://collector.example.com",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
		"OTEL_TRACES_SAMPLE_RATE":     "0.25",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if !cfg.OpenTelemetry.Enabled {
		t.Fatal("cfg.OpenTelemetry.Enabled = false, want true")
	}
	if cfg.OpenTelemetry.ServiceName != "zgi-api-test" {
		t.Fatalf("cfg.OpenTelemetry.ServiceName = %q, want zgi-api-test", cfg.OpenTelemetry.ServiceName)
	}
	if cfg.OpenTelemetry.Endpoint != "https://collector.example.com" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want https://collector.example.com", cfg.OpenTelemetry.Endpoint)
	}
	if cfg.OpenTelemetry.Protocol != "http/protobuf" {
		t.Fatalf("cfg.OpenTelemetry.Protocol = %q, want http/protobuf", cfg.OpenTelemetry.Protocol)
	}
	if cfg.OpenTelemetry.TraceSampleRate != 0.25 {
		t.Fatalf("cfg.OpenTelemetry.TraceSampleRate = %v, want 0.25", cfg.OpenTelemetry.TraceSampleRate)
	}
	if cfg.OpenTelemetry.TracesEndpoint != "" {
		t.Fatalf("cfg.OpenTelemetry.TracesEndpoint = %q, want empty", cfg.OpenTelemetry.TracesEndpoint)
	}
	if !cfg.OpenTelemetry.InstrumentHTTPClient {
		t.Fatal("cfg.OpenTelemetry.InstrumentHTTPClient = false, want true")
	}
	if !cfg.OpenTelemetry.InstrumentWorkflow {
		t.Fatal("cfg.OpenTelemetry.InstrumentWorkflow = false, want true")
	}
	if cfg.OpenTelemetry.InstrumentDB {
		t.Fatal("cfg.OpenTelemetry.InstrumentDB = true, want false")
	}
	if cfg.OpenTelemetry.InstrumentRedis {
		t.Fatal("cfg.OpenTelemetry.InstrumentRedis = true, want false")
	}
	if cfg.OpenTelemetry.InstrumentGRPC {
		t.Fatal("cfg.OpenTelemetry.InstrumentGRPC = true, want false")
	}
	if !cfg.OpenTelemetry.LLMLangfuseAttributes {
		t.Fatal("cfg.OpenTelemetry.LLMLangfuseAttributes = false, want true")
	}
	if cfg.OpenTelemetry.LLMCaptureContent != "summary" {
		t.Fatalf("cfg.OpenTelemetry.LLMCaptureContent = %q, want summary", cfg.OpenTelemetry.LLMCaptureContent)
	}
	if cfg.OpenTelemetry.LLMCaptureMaxChars != 65536 {
		t.Fatalf("cfg.OpenTelemetry.LLMCaptureMaxChars = %d, want 65536", cfg.OpenTelemetry.LLMCaptureMaxChars)
	}
}

func TestLoadOpenTelemetryConfigDoesNotDefaultEndpoint(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":  "release",
		"ENV":          "production",
		"SECRET_KEY":   "test-secret",
		"OTEL_ENABLED": "true",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want empty when OTEL_EXPORTER_OTLP_ENDPOINT is not configured", cfg.OpenTelemetry.Endpoint)
	}
}

func TestLoadOpenTelemetryHeadersAndTraceEndpoint(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                        "release",
		"ENV":                                "production",
		"SECRET_KEY":                         "test-secret",
		"OTEL_ENABLED":                       "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT":        "https://collector.example.com/base",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "https://collector.example.com/custom/v1/traces",
		"OTEL_EXPORTER_OTLP_HEADERS":         "Authorization=Basic base,x-common=base",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS":  "Authorization=Basic trace,x-trace=true",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "https://collector.example.com/base" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want base endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if cfg.OpenTelemetry.TracesEndpoint != "https://collector.example.com/custom/v1/traces" {
		t.Fatalf("cfg.OpenTelemetry.TracesEndpoint = %q, want trace endpoint", cfg.OpenTelemetry.TracesEndpoint)
	}
	if got := cfg.OpenTelemetry.Headers["Authorization"]; got != "Basic trace" {
		t.Fatalf("Authorization header = %q, want trace-specific header", got)
	}
	if got := cfg.OpenTelemetry.Headers["x-trace"]; got != "true" {
		t.Fatalf("x-trace header = %q, want true", got)
	}
	if _, ok := cfg.OpenTelemetry.Headers["x-common"]; ok {
		t.Fatal("base OTLP header leaked when trace-specific headers are set")
	}
}

func TestLoadLangfuseOpenTelemetryConfig(t *testing.T) {
	publicKey := "pk-lf-public"
	secretKey := "sk-lf-secret"
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                "release",
		"ENV":                        "production",
		"SECRET_KEY":                 "test-secret",
		"OTEL_ENABLED":               "true",
		"LANGFUSE_PUBLIC_KEY":        publicKey,
		"LANGFUSE_SECRET_KEY":        secretKey,
		"LANGFUSE_BASE_URL":          "https://cloud.langfuse.com/",
		"OTEL_EXPORTER_OTLP_HEADERS": "x-existing=true",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "https://cloud.langfuse.com/api/public/otel" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want Langfuse OTLP endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if cfg.OpenTelemetry.TracesEndpoint != "" {
		t.Fatalf("cfg.OpenTelemetry.TracesEndpoint = %q, want empty when Langfuse endpoint is used", cfg.OpenTelemetry.TracesEndpoint)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(publicKey+":"+secretKey))
	if got := cfg.OpenTelemetry.Headers["Authorization"]; got != wantAuth {
		t.Fatalf("Authorization header = %q, want %q", got, wantAuth)
	}
	if got := cfg.OpenTelemetry.Headers["x-langfuse-ingestion-version"]; got != "4" {
		t.Fatalf("x-langfuse-ingestion-version = %q, want 4", got)
	}
	if got := cfg.OpenTelemetry.Headers["x-existing"]; got != "true" {
		t.Fatalf("existing OTLP header = %q, want preserved", got)
	}
}

func TestLoadLangfuseOpenTelemetryConfigCanBeExplicitlyDisabled(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                 "release",
		"ENV":                         "production",
		"SECRET_KEY":                  "test-secret",
		"OTEL_ENABLED":                "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector:4318",
		"LANGFUSE_ENABLED":            "false",
		"LANGFUSE_PUBLIC_KEY":         "pk-lf-public",
		"LANGFUSE_SECRET_KEY":         "sk-lf-secret",
		"LANGFUSE_BASE_URL":           "https://cloud.langfuse.com",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "http://otel-collector:4318" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want original OTLP endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if len(cfg.OpenTelemetry.Headers) != 0 {
		t.Fatalf("cfg.OpenTelemetry.Headers = %#v, want empty", cfg.OpenTelemetry.Headers)
	}
}

func TestLoadLangfuseAuthStringForCollectorDoesNotOverrideOTLP(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                 "release",
		"ENV":                         "production",
		"SECRET_KEY":                  "test-secret",
		"OTEL_ENABLED":                "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector:4318",
		"LANGFUSE_AUTH_STRING":        "base64-public-secret",
		"LANGFUSE_OTEL_ENDPOINT":      "https://cloud.langfuse.com/api/public/otel",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "http://otel-collector:4318" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want collector endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if len(cfg.OpenTelemetry.Headers) != 0 {
		t.Fatalf("cfg.OpenTelemetry.Headers = %#v, want empty for collector mode", cfg.OpenTelemetry.Headers)
	}
}

func TestLoadLangfuseKeyPairWithCollectorEndpointRequiresDirectExport(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                 "release",
		"ENV":                         "production",
		"SECRET_KEY":                  "test-secret",
		"OTEL_ENABLED":                "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector:4318",
		"LANGFUSE_PUBLIC_KEY":         "pk-lf-public",
		"LANGFUSE_SECRET_KEY":         "sk-lf-secret",
		"LANGFUSE_BASE_URL":           "https://cloud.langfuse.com",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "http://otel-collector:4318" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want collector endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if len(cfg.OpenTelemetry.Headers) != 0 {
		t.Fatalf("cfg.OpenTelemetry.Headers = %#v, want empty without direct export intent", cfg.OpenTelemetry.Headers)
	}
}

func TestLoadLangfuseKeyPairWithTraceEndpointRequiresDirectExport(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                        "release",
		"ENV":                                "production",
		"SECRET_KEY":                         "test-secret",
		"OTEL_ENABLED":                       "true",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "https://collector.example.com/v1/traces",
		"LANGFUSE_PUBLIC_KEY":                "pk-lf-public",
		"LANGFUSE_SECRET_KEY":                "sk-lf-secret",
		"LANGFUSE_BASE_URL":                  "https://cloud.langfuse.com",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want empty base endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if cfg.OpenTelemetry.TracesEndpoint != "https://collector.example.com/v1/traces" {
		t.Fatalf("cfg.OpenTelemetry.TracesEndpoint = %q, want configured trace endpoint", cfg.OpenTelemetry.TracesEndpoint)
	}
	if len(cfg.OpenTelemetry.Headers) != 0 {
		t.Fatalf("cfg.OpenTelemetry.Headers = %#v, want empty without direct export intent", cfg.OpenTelemetry.Headers)
	}
}

func TestLoadLangfuseOpenTelemetryConfigExplicitlyEnabledWithAuthString(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                 "release",
		"ENV":                         "production",
		"SECRET_KEY":                  "test-secret",
		"OTEL_ENABLED":                "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector:4318",
		"LANGFUSE_ENABLED":            "true",
		"LANGFUSE_AUTH_STRING":        "base64-public-secret",
		"LANGFUSE_BASE_URL":           "https://cloud.langfuse.com",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "https://cloud.langfuse.com/api/public/otel" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want direct Langfuse endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if got := cfg.OpenTelemetry.Headers["Authorization"]; got != "Basic base64-public-secret" {
		t.Fatalf("Authorization header = %q, want Basic base64-public-secret", got)
	}
}

func TestLoadLangfuseOpenTelemetryConfigWithoutKeysDoesNotOverrideOTLP(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                 "release",
		"ENV":                         "production",
		"SECRET_KEY":                  "test-secret",
		"OTEL_ENABLED":                "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector:4318",
		"LANGFUSE_ENABLED":            "true",
		"LANGFUSE_BASE_URL":           "https://cloud.langfuse.com",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Endpoint != "http://otel-collector:4318" {
		t.Fatalf("cfg.OpenTelemetry.Endpoint = %q, want original OTLP endpoint", cfg.OpenTelemetry.Endpoint)
	}
	if len(cfg.OpenTelemetry.Headers) != 0 {
		t.Fatalf("cfg.OpenTelemetry.Headers = %#v, want empty", cfg.OpenTelemetry.Headers)
	}
}

func TestLoadOpenTelemetryInstrumentationConfig(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"OTEL_ENABLED":                 "true",
		"OTEL_INSTRUMENT_HTTP_CLIENT":  "false",
		"OTEL_INSTRUMENT_WORKFLOW":     "false",
		"OTEL_INSTRUMENT_DB":           "true",
		"OTEL_INSTRUMENT_REDIS":        "true",
		"OTEL_INSTRUMENT_GRPC":         "true",
		"OTEL_LLM_LANGFUSE_ATTRIBUTES": "false",
		"OTEL_LLM_CAPTURE_CONTENT":     "full",
		"OTEL_LLM_CAPTURE_MAX_CHARS":   "0",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.InstrumentHTTPClient {
		t.Fatal("cfg.OpenTelemetry.InstrumentHTTPClient = true, want false")
	}
	if cfg.OpenTelemetry.InstrumentWorkflow {
		t.Fatal("cfg.OpenTelemetry.InstrumentWorkflow = true, want false")
	}
	if !cfg.OpenTelemetry.InstrumentDB {
		t.Fatal("cfg.OpenTelemetry.InstrumentDB = false, want true")
	}
	if !cfg.OpenTelemetry.InstrumentRedis {
		t.Fatal("cfg.OpenTelemetry.InstrumentRedis = false, want true")
	}
	if !cfg.OpenTelemetry.InstrumentGRPC {
		t.Fatal("cfg.OpenTelemetry.InstrumentGRPC = false, want true")
	}
	if cfg.OpenTelemetry.LLMLangfuseAttributes {
		t.Fatal("cfg.OpenTelemetry.LLMLangfuseAttributes = true, want false")
	}
	if cfg.OpenTelemetry.LLMCaptureContent != "full" {
		t.Fatalf("cfg.OpenTelemetry.LLMCaptureContent = %q, want full", cfg.OpenTelemetry.LLMCaptureContent)
	}
	if cfg.OpenTelemetry.LLMCaptureMaxChars != 0 {
		t.Fatalf("cfg.OpenTelemetry.LLMCaptureMaxChars = %d, want 0", cfg.OpenTelemetry.LLMCaptureMaxChars)
	}
}

func TestLoadOpenTelemetryTracesExporterNoneDisablesTracing(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":          "release",
		"ENV":                  "production",
		"SECRET_KEY":           "test-secret",
		"OTEL_ENABLED":         "true",
		"OTEL_TRACES_EXPORTER": "none",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.OpenTelemetry.Enabled {
		t.Fatal("cfg.OpenTelemetry.Enabled = true, want false")
	}
}

func TestLoadCloudBootstrapConfig(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                        "release",
		"ENV":                                "production",
		"SECRET_KEY":                         "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM":       "noreply@example.com",
		"EMAIL_RESEND_API_KEY":               "test-api-key",
		"ZGI_RUN_MODE":                       "cloud",
		"ZGI_CLOUD_BOOTSTRAP_ADMIN_EMAIL":    "bootstrap@example.com",
		"ZGI_CLOUD_BOOTSTRAP_ADMIN_NAME":     "Bootstrap Admin",
		"ZGI_CLOUD_BOOTSTRAP_ADMIN_PASSWORD": "secret1234",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Platform.Edition != "CLOUD" {
		t.Fatalf("cfg.Platform.Edition = %q, want %q", cfg.Platform.Edition, "CLOUD")
	}
	if cfg.Platform.CloudBootstrap.AdminEmail != "bootstrap@example.com" {
		t.Fatalf("cfg.Platform.CloudBootstrap.AdminEmail = %q, want bootstrap@example.com", cfg.Platform.CloudBootstrap.AdminEmail)
	}
	if cfg.Platform.CloudBootstrap.AdminName != "Bootstrap Admin" {
		t.Fatalf("cfg.Platform.CloudBootstrap.AdminName = %q, want Bootstrap Admin", cfg.Platform.CloudBootstrap.AdminName)
	}
	if cfg.Platform.CloudBootstrap.AdminPassword != "secret1234" {
		t.Fatalf("cfg.Platform.CloudBootstrap.AdminPassword = %q, want secret1234", cfg.Platform.CloudBootstrap.AdminPassword)
	}
}

func TestLoadNormalizesZGIRunMode(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":  "release",
		"ENV":          "production",
		"SECRET_KEY":   "test-secret",
		"ZGI_RUN_MODE": "self-hosted",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Platform.Edition != "SELF_HOSTED" {
		t.Fatalf("cfg.Platform.Edition = %q, want %q", cfg.Platform.Edition, "SELF_HOSTED")
	}
	if cfg.JWT.Issuer != "SELF_HOSTED" {
		t.Fatalf("cfg.JWT.Issuer = %q, want %q", cfg.JWT.Issuer, "SELF_HOSTED")
	}
}

func TestLoadAllowsMissingResendAPIKeyOutsideCloudMode(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":  "release",
		"ENV":          "production",
		"SECRET_KEY":   "test-secret",
		"ZGI_RUN_MODE": "self-hosted",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}
	if config.HasEmailDeliveryConfig(cfg) {
		t.Fatal("config.HasEmailDeliveryConfig() = true, want false without Resend API key")
	}
}

func TestLoadRequiresResendAPIKeyInCloudMode(t *testing.T) {
	_, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":  "release",
		"ENV":          "production",
		"SECRET_KEY":   "test-secret",
		"ZGI_RUN_MODE": "cloud",
	}))
	if err == nil {
		t.Fatal("config.LoadFromFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "EMAIL_RESEND_API_KEY") {
		t.Fatalf("config.LoadFromFile() error = %v, want error containing EMAIL_RESEND_API_KEY", err)
	}
}

func TestLoadRequiresResendAPIKeyWhenEmailCodeLoginEnabled(t *testing.T) {
	_, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":             "release",
		"ENV":                     "production",
		"SECRET_KEY":              "test-secret",
		"ZGI_RUN_MODE":            "self-hosted",
		"ENABLE_EMAIL_CODE_LOGIN": "true",
	}))
	if err == nil {
		t.Fatal("config.LoadFromFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "EMAIL_RESEND_API_KEY") {
		t.Fatalf("config.LoadFromFile() error = %v, want error containing EMAIL_RESEND_API_KEY", err)
	}
}

func TestTaskQueueUsesProjectRedisConnection(t *testing.T) {
	taskQueueType := reflect.TypeOf(config.TaskQueueConfig{})
	if _, exists := taskQueueType.FieldByName("RedisAddr"); exists {
		t.Fatal("TaskQueueConfig must not keep a duplicate RedisAddr field; use Config.Redis host/port instead")
	}
	if _, exists := taskQueueType.FieldByName("RedisPassword"); exists {
		t.Fatal("TaskQueueConfig must not keep a duplicate RedisPassword field; use Config.Redis password instead")
	}

	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"REDIS_HOST":                   "main-redis",
		"REDIS_PORT":                   "6380",
		"REDIS_PASSWORD":               "main-secret",
		"TASK_QUEUE_REDIS_ADDR":        "legacy-queue-redis:9999",
		"TASK_QUEUE_REDIS_PASSWORD":    "legacy-queue-secret",
		"TASK_QUEUE_REDIS_DB":          "3",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Redis.Host != "main-redis" || cfg.Redis.Port != 6380 || cfg.Redis.Password != "main-secret" {
		t.Fatalf("cfg.Redis = %#v, want main redis connection", cfg.Redis)
	}
	if cfg.TaskQueue.RedisDB != 3 {
		t.Fatalf("cfg.TaskQueue.RedisDB = %d, want 3", cfg.TaskQueue.RedisDB)
	}
}

func TestSQLBaseInternalUsesProjectDatabaseConnection(t *testing.T) {
	sqlBaseType := reflect.TypeOf(config.SQLBaseConfig{})
	for _, fieldName := range []string{"InternalHost", "InternalPort", "InternalUser", "InternalPassword"} {
		if _, exists := sqlBaseType.FieldByName(fieldName); exists {
			t.Fatalf("SQLBaseConfig must not keep duplicate %s; internal SQL base should reuse Config.Database connection", fieldName)
		}
	}

	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"DB_HOST":                      "main-db",
		"DB_PORT":                      "15432",
		"DB_USERNAME":                  "main-user",
		"DB_PASSWORD":                  "main-secret",
		"SQL_BASE_INTERNAL_HOST":       "legacy-sql-base-db",
		"SQL_BASE_INTERNAL_PORT":       "25432",
		"SQL_BASE_INTERNAL_USER":       "legacy-user",
		"SQL_BASE_INTERNAL_PASSWORD":   "legacy-secret",
		"SQL_BASE_INTERNAL_DB":         "sql_base_db",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}

	if cfg.Database.Host != "main-db" || cfg.Database.Port != 15432 ||
		cfg.Database.Username != "main-user" || cfg.Database.Password != "main-secret" {
		t.Fatalf("cfg.Database = %#v, want main database connection", cfg.Database)
	}
	if cfg.SQLBase.InternalDB != "sql_base_db" {
		t.Fatalf("cfg.SQLBase.InternalDB = %q, want sql_base_db", cfg.SQLBase.InternalDB)
	}
}

func TestLoadFallsBackToEnvironmentWhenEnvFileMissing(t *testing.T) {
	restoreGlobalConfig(t)
	chdirToTempDir(t)

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("ENV", "production")
	t.Setenv("SECRET_KEY", "env-secret")
	t.Setenv("EMAIL_MAIL_DEFAULT_SEND_FROM", "noreply@example.com")
	t.Setenv("EMAIL_RESEND_API_KEY", "test-api-key")
	t.Setenv("API_KEY_ENCRYPTION_KEY", "test-api-key-encryption-key-32!!")
	t.Setenv("SERVER_PORT", "3100")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if got := cfg.Server.Port; got != 3100 {
		t.Fatalf("cfg.Server.Port = %d, want %d", got, 3100)
	}
	if got := cfg.JWT.Secret; got != "env-secret" {
		t.Fatalf("cfg.JWT.Secret = %q, want %q", got, "env-secret")
	}
}

func TestLoadWorkflowTestTaskBackend(t *testing.T) {
	restoreGlobalConfig(t)
	chdirToTempDir(t)

	t.Setenv("SERVER_MODE", "release")
	t.Setenv("ENV", "production")
	t.Setenv("SECRET_KEY", "env-secret")
	t.Setenv("EMAIL_MAIL_DEFAULT_SEND_FROM", "noreply@example.com")
	t.Setenv("EMAIL_RESEND_API_KEY", "test-api-key")
	t.Setenv("API_KEY_ENCRYPTION_KEY", "test-api-key-encryption-key-32!!")
	t.Setenv("WORKFLOW_TEST_TASK_BACKEND", "asynq")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if got := cfg.TaskQueue.WorkflowTestTaskBackend; got != "asynq" {
		t.Fatalf("cfg.TaskQueue.WorkflowTestTaskBackend = %q, want asynq", got)
	}
}

func TestLoadPrefersEnvFileOverEnvironment(t *testing.T) {
	restoreGlobalConfig(t)

	dir := t.TempDir()
	path := dir + "/.env"
	content := strings.Join([]string{
		"SERVER_MODE=release",
		"ENV=production",
		"SECRET_KEY=file-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM=noreply@example.com",
		"EMAIL_RESEND_API_KEY=test-api-key",
		"API_KEY_ENCRYPTION_KEY=test-api-key-encryption-key-32!!",
		"SERVER_PORT=3200",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}

	chdirToDir(t, dir)
	t.Setenv("SECRET_KEY", "env-secret")
	t.Setenv("SERVER_PORT", "3300")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if got := cfg.Server.Port; got != 3200 {
		t.Fatalf("cfg.Server.Port = %d, want %d", got, 3200)
	}
	if got := cfg.JWT.Secret; got != "file-secret" {
		t.Fatalf("cfg.JWT.Secret = %q, want %q", got, "file-secret")
	}
}

func TestLoadRejectsPlaceholderSecretInProduction(t *testing.T) {
	_, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "change-me-in-production",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
	}))
	if err == nil {
		t.Fatal("config.LoadFromFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "SECRET_KEY") {
		t.Fatalf("config.LoadFromFile() error = %v, want error containing SECRET_KEY", err)
	}
}

func TestLoadRejectsPlaceholderEncryptionKeyInProduction(t *testing.T) {
	_, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"API_KEY_ENCRYPTION_KEY":       "32ByteLongSecretKeyForAESGCM!!!!",
	}))
	if err == nil {
		t.Fatal("config.LoadFromFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "API_KEY_ENCRYPTION_KEY") {
		t.Fatalf("config.LoadFromFile() error = %v, want error containing API_KEY_ENCRYPTION_KEY", err)
	}
}

func TestLoadRejectsMissingAPIKeyEncryptionKeyInProduction(t *testing.T) {
	_, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "production",
		"SECRET_KEY":                   "test-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
		"EMAIL_RESEND_API_KEY":         "test-api-key",
		"API_KEY_ENCRYPTION_KEY":       "",
	}))
	if err == nil {
		t.Fatal("config.LoadFromFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "API_KEY_ENCRYPTION_KEY") {
		t.Fatalf("config.LoadFromFile() error = %v, want error containing API_KEY_ENCRYPTION_KEY", err)
	}
}

func TestLoadAllowsPlaceholderSecretForLocalDockerExample(t *testing.T) {
	cfg, err := config.LoadFromFile(writeEnvFile(t, map[string]string{
		"SERVER_MODE":                  "release",
		"ENV":                          "local",
		"SECRET_KEY":                   "replace-with-strong-random-secret",
		"EMAIL_MAIL_DEFAULT_SEND_FROM": "noreply@example.com",
	}))
	if err != nil {
		t.Fatalf("config.LoadFromFile() error = %v, want nil", err)
	}
	if cfg.Server.Environment != "local" {
		t.Fatalf("cfg.Server.Environment = %q, want local", cfg.Server.Environment)
	}
}

func writeEnvFile(t *testing.T, values map[string]string) string {
	t.Helper()

	if values["ENV"] == "production" {
		if _, ok := values["API_KEY_ENCRYPTION_KEY"]; !ok {
			values["API_KEY_ENCRYPTION_KEY"] = "test-api-key-encryption-key-32!!"
		}
	}

	var builder strings.Builder
	for key, value := range values {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	path := t.TempDir() + "/.env"
	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
	return path
}

func restoreGlobalConfig(t *testing.T) {
	t.Helper()

	previous := config.GlobalConfig
	config.GlobalConfig = nil
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})
}

func chdirToTempDir(t *testing.T) {
	t.Helper()
	chdirToDir(t, t.TempDir())
}

func chdirToDir(t *testing.T, dir string) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", dir, err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory to %q error = %v", previous, err)
		}
	})
}
