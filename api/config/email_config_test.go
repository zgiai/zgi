package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadEmailConfigPrefersCanonicalKeys(t *testing.T) {
	values := map[string]string{
		envEmailProvider:      "smtp",
		envEmailSMTPPort:      "465",
		envEmailSMTPSecurity:  "implicit_tls",
		envEmailFromName:      "ZGI Platform",
		envEmailFromAddress:   "system@notify.example.com",
		envEmailResendBaseURL: "https://mail.example.com/v1/",
		envEmailFrom:          "Friendly Sender <friendly@example.com>",
		envResendAPIKey:       "standard-key",
		envResendBaseURL:      "https://proxy.example.com/v1",
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	if err := loadEmailConfig(cfg, source); err != nil {
		t.Fatalf("loadEmailConfig() error = %v", err)
	}
	if cfg.Email.MailType != "smtp" || cfg.Email.SMTPPort != 465 {
		t.Fatalf("unexpected provider config: %+v", cfg.Email)
	}
	if cfg.Email.SMTPSecurity != "implicit_tls" || !cfg.Email.SMTPUseTLS || cfg.Email.SMTPOpportunisticTLS {
		t.Fatalf("unexpected SMTP security config: %+v", cfg.Email)
	}
	if cfg.Email.MailDefaultSendFrom != "Friendly Sender <friendly@example.com>" {
		t.Fatalf("unexpected sender: %q", cfg.Email.MailDefaultSendFrom)
	}
	if cfg.Email.ResendAPIKey != "standard-key" || cfg.Email.ResendAPIURL != "https://proxy.example.com/v1" {
		t.Fatalf("unexpected resend base URL: %q", cfg.Email.ResendAPIURL)
	}
}

func TestLoadEmailConfigSupportsLegacyKeys(t *testing.T) {
	values := map[string]string{
		envEmailMailType:             "smtp",
		envEmailPort:                 "587",
		envEmailSMTPOpportunisticTLS: "true",
		envEmailMailDefaultSendFrom:  "Legacy <legacy@example.com>",
		envEmailResendAPIURL:         "https://legacy.example.com",
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	if err := loadEmailConfig(cfg, source); err != nil {
		t.Fatalf("loadEmailConfig() error = %v", err)
	}
	if cfg.Email.MailType != "smtp" || cfg.Email.SMTPPort != 587 || !cfg.Email.SMTPOpportunisticTLS {
		t.Fatalf("legacy SMTP keys were not preserved: %+v", cfg.Email)
	}
	if cfg.Email.MailDefaultSendFrom != "Legacy <legacy@example.com>" {
		t.Fatalf("unexpected legacy sender: %q", cfg.Email.MailDefaultSendFrom)
	}
}

func TestLoadEmailConfigSkipsEmptyCanonicalAliases(t *testing.T) {
	values := map[string]string{
		envEmailProvider:      "",
		envEmailMailType:      "smtp",
		envResendAPIKey:       "",
		envEmailResendAPIKey:  "legacy-key",
		envResendBaseURL:      "",
		envEmailResendBaseURL: "https://legacy.example.com/v1",
		envEmailSMTPServer:    "smtp.example.com",
		envEmailSMTPPort:      "587",
		envEmailSMTPSecurity:  "starttls",
		envEmailFrom:          "Sender <sender@example.com>",
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	requireNoError(t, loadEmailConfig(cfg, source))
	if cfg.Email.MailType != "smtp" || cfg.Email.ResendAPIKey != "legacy-key" || cfg.Email.ResendAPIURL != "https://legacy.example.com/v1" {
		t.Fatalf("empty canonical aliases hid legacy values: %+v", cfg.Email)
	}
}

func TestLoadEmailConfigUsesOfficialResendURLWhenBaseURLIsEmpty(t *testing.T) {
	values := map[string]string{
		envResendAPIKey:  "test-key",
		envResendBaseURL: "",
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	requireNoError(t, loadEmailConfig(cfg, source))
	if cfg.Email.ResendAPIURL != "https://api.resend.com" {
		t.Fatalf("ResendAPIURL = %q, want official default", cfg.Email.ResendAPIURL)
	}
}

func TestLoadEmailConfigCanonicalSMTPDefaultsToStartTLS(t *testing.T) {
	values := map[string]string{
		envEmailProvider:   "smtp",
		envEmailSMTPServer: "smtp.example.com",
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	requireNoError(t, loadEmailConfig(cfg, source))
	if cfg.Email.SMTPSecurity != "starttls" || cfg.Email.SMTPUseTLS || !cfg.Email.SMTPOpportunisticTLS {
		t.Fatalf("canonical SMTP did not default to STARTTLS: %+v", cfg.Email)
	}
}

func TestLoadEmailConfigPreservesExplicitSMTPNone(t *testing.T) {
	values := map[string]string{
		envEmailProvider:     "smtp",
		envEmailSMTPSecurity: "none",
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	requireNoError(t, loadEmailConfig(cfg, source))
	if cfg.Email.SMTPSecurity != "none" || cfg.Email.SMTPUseTLS || cfg.Email.SMTPOpportunisticTLS {
		t.Fatalf("explicit SMTP none was not preserved: %+v", cfg.Email)
	}
}

func TestLoadEmailConfigPreservesLegacySMTPDefaultsAndFlags(t *testing.T) {
	testCases := []struct {
		name              string
		values            map[string]string
		wantSecurity      string
		wantUseTLS        bool
		wantOpportunistic bool
	}{
		{
			name:         "legacy provider default",
			values:       map[string]string{envEmailMailType: "smtp"},
			wantSecurity: "",
		},
		{
			name:       "legacy implicit TLS flag",
			values:     map[string]string{envEmailProvider: "smtp", envEmailSMTPUseTLS: "true"},
			wantUseTLS: true,
		},
		{
			name:              "legacy opportunistic TLS flag",
			values:            map[string]string{envEmailProvider: "smtp", envEmailSMTPOpportunisticTLS: "true"},
			wantOpportunistic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				value, ok := tc.values[key]
				return value, ok
			}}
			cfg := &Config{}
			requireNoError(t, loadEmailConfig(cfg, source))
			if cfg.Email.SMTPSecurity != tc.wantSecurity || cfg.Email.SMTPUseTLS != tc.wantUseTLS || cfg.Email.SMTPOpportunisticTLS != tc.wantOpportunistic {
				t.Fatalf("legacy SMTP settings changed: %+v", cfg.Email)
			}
		})
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigRejectsUnknownSMTPSecurity(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Email.SMTPSecurity = "tls_if_available"

	if err := validateConfig(cfg); err == nil {
		t.Fatal("validateConfig() expected SMTP security error")
	}
}

func TestValidateConfigRejectsUnknownEmailProvider(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Email.MailType = "custom"

	if err := validateConfig(cfg); err == nil {
		t.Fatal("validateConfig() expected email provider error")
	}
}

func TestValidateConfigRejectsCleartextResendURLOutsideDevelopment(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Server = ServerConfig{Mode: "release", Environment: "production"}
	cfg.JWT.Secret = "production-test-secret"
	cfg.Encryption.APIKeyEncryptionKey = "production-test-encryption-key"
	cfg.Email.MailType = "resend"
	cfg.Email.ResendAPIURL = "http://mail.example.com/v1"

	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("validateConfig() error = %v, want HTTPS requirement", err)
	}
}

func TestValidateConfigAllowsCleartextResendURLInDevelopment(t *testing.T) {
	cfg := developmentTestConfig()
	cfg.Email.MailType = "resend"
	cfg.Email.ResendAPIURL = "http://127.0.0.1:8080/v1"

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() rejected local Resend proxy: %v", err)
	}
}

func TestEmailConfigJSONDoesNotExposeCredentials(t *testing.T) {
	payload, err := json.Marshal(EmailConfig{
		ResendAPIKey: "resend-secret",
		SMTPPassword: "smtp-secret",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(payload), "resend-secret") || strings.Contains(string(payload), "smtp-secret") {
		t.Fatalf("serialized email config exposed credentials: %s", payload)
	}
}

func developmentTestConfig() *Config {
	return &Config{
		Server:    ServerConfig{Mode: "debug", Environment: "local"},
		Email:     EmailConfig{MailType: "smtp", MailDefaultSendFrom: "noreply@example.com", SMTPServer: "localhost", SMTPPort: 587},
		Database:  DatabaseConfig{MaxIdleConns: 1, MaxOpenConns: 1, ConnMaxLifetime: 1},
		Redis:     RedisConfig{PoolSize: 1, MinIdleConns: 1},
		TaskQueue: TaskQueueConfig{Retention: 1},
		Workflow:  WorkflowConfig{ExecutionTimeout: 1, LLMTimeout: 1, HeartbeatInterval: 1, CleanupTimeout: 1},
		WorkflowFileExtraction: WorkflowFileExtractionConfig{
			MaxContentSize:    1,
			ExtractionTimeout: 1,
		},
		AnswerNodeStreaming: AnswerNodeStreamingConfig{ChunkSize: 1},
	}
}
