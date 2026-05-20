package config

import (
	"fmt"
	"strings"
)

func validateConfig(cfg *Config) error {
	if cfg.JWT.Secret == "" && !isDevelopmentMode(cfg) {
		return fmt.Errorf("%s is required", envSecretKey)
	}
	if !isDevelopmentMode(cfg) {
		if isKnownPlaceholderSecret(cfg.JWT.Secret) {
			return fmt.Errorf("%s must be changed from the example placeholder value", envSecretKey)
		}
		if strings.TrimSpace(cfg.Encryption.APIKeyEncryptionKey) == "" {
			return fmt.Errorf("%s is required", envAPIKeyEncryptionKey)
		}
		if isKnownPlaceholderSecret(cfg.Encryption.APIKeyEncryptionKey) {
			return fmt.Errorf("%s must be changed from the example placeholder value", envAPIKeyEncryptionKey)
		}
		if isKnownPlaceholderSecret(cfg.Encryption.LLMCredentialSecretKey) {
			return fmt.Errorf("%s must be changed from the example placeholder value", envLLMCredentialSecretKey)
		}
	}

	if cfg.Email.MailDefaultSendFrom == "" {
		return fmt.Errorf("%s is required", envEmailMailDefaultSendFrom)
	}

	if requiresResendAPIKey(cfg) && cfg.Email.ResendAPIKey == "" {
		return fmt.Errorf("%s is required", envEmailResendAPIKey)
	}

	if strings.EqualFold(cfg.Email.MailType, "smtp") && cfg.Email.SMTPServer == "" {
		return fmt.Errorf("%s is required", envEmailSMTPServer)
	}

	if strings.EqualFold(cfg.Email.MailType, "smtp") && cfg.Email.SMTPPort <= 0 {
		return fmt.Errorf("%s must be greater than 0", envEmailPort)
	}

	if cfg.Database.MaxIdleConns <= 0 {
		return fmt.Errorf("%s must be greater than 0", envDBMaxIdleConns)
	}

	if cfg.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("%s must be greater than 0", envDBMaxOpenConns)
	}

	if cfg.Database.ConnMaxLifetime <= 0 {
		return fmt.Errorf("%s must be greater than 0", envDBConnMaxLifetime)
	}

	if cfg.Redis.PoolSize <= 0 {
		return fmt.Errorf("%s must be greater than 0", envRedisPoolSize)
	}

	if cfg.Redis.MinIdleConns <= 0 {
		return fmt.Errorf("%s must be greater than 0", envRedisMinIdleConns)
	}

	if cfg.TaskQueue.Retention <= 0 {
		return fmt.Errorf("%s must be greater than 0", envTaskQueueRetention)
	}

	if cfg.Workflow.ExecutionTimeout <= 0 {
		return fmt.Errorf("%s must be greater than 0", envWorkflowExecutionTimeout)
	}

	if cfg.Workflow.LLMTimeout <= 0 {
		return fmt.Errorf("%s must be greater than 0", envWorkflowLLMTimeout)
	}

	if cfg.Workflow.HeartbeatInterval <= 0 {
		return fmt.Errorf("%s must be greater than 0", envWorkflowHeartbeatInterval)
	}

	if cfg.Workflow.CleanupTimeout <= 0 {
		return fmt.Errorf("%s must be greater than 0", envWorkflowCleanupTimeout)
	}

	if cfg.WorkflowFileExtraction.MaxContentSize <= 0 {
		return fmt.Errorf("%s must be greater than 0", envWorkflowFileExtractionMaxContentSize)
	}

	if cfg.WorkflowFileExtraction.ExtractionTimeout <= 0 {
		return fmt.Errorf("%s must be greater than 0", envWorkflowFileExtractionTimeout)
	}

	if cfg.AnswerNodeStreaming.ChunkSize < MinChunkSize || cfg.AnswerNodeStreaming.ChunkSize > MaxChunkSize {
		return fmt.Errorf("%s must be between %d and %d, got: %d", envAnswerNodeStreamingChunkSize, MinChunkSize, MaxChunkSize, cfg.AnswerNodeStreaming.ChunkSize)
	}

	if cfg.LLM.EncryptionKey != "" && len(cfg.LLM.EncryptionKey) != 32 {
		return fmt.Errorf("%s must be exactly 32 bytes long, got %d bytes", envLLMEncryptionKey, len(cfg.LLM.EncryptionKey))
	}

	return nil
}

// HasEmailDeliveryConfig reports whether the configured mail backend has enough
// settings to attempt delivery.
func HasEmailDeliveryConfig(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Email.MailType)) {
	case "resend":
		return strings.TrimSpace(cfg.Email.ResendAPIKey) != ""
	case "smtp":
		return strings.TrimSpace(cfg.Email.SMTPServer) != "" && cfg.Email.SMTPPort > 0
	default:
		return false
	}
}

func requiresResendAPIKey(cfg *Config) bool {
	if cfg == nil || !strings.EqualFold(strings.TrimSpace(cfg.Email.MailType), "resend") {
		return false
	}
	return isCloudEdition(cfg.Platform.Edition) ||
		cfg.Feature.EnableEmailCodeLogin ||
		cfg.Feature.AllowRegister
}

func isCloudEdition(edition string) bool {
	return strings.EqualFold(strings.TrimSpace(edition), "CLOUD")
}

func isDevelopmentMode(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.Server.Mode == "debug" || cfg.Server.Environment == "local" || cfg.Server.Environment == "dev"
}

func isKnownPlaceholderSecret(secret string) bool {
	switch strings.TrimSpace(secret) {
	case "change-me-in-production",
		"replace-with-strong-random-secret",
		"replace-with-32-byte-random-key",
		"32ByteLongSecretKeyForAESGCM!!!!",
		"32ByteLongSecretKeyForLLMCred!!!":
		return true
	default:
		return false
	}
}
