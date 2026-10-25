package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
)

func validateConfig(cfg *Config) error {
	if err := validateConsoleAPIURL(cfg); err != nil {
		return err
	}
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
		return fmt.Errorf("%s is required", envEmailFrom)
	}
	if _, err := mail.ParseAddress(cfg.Email.MailDefaultSendFrom); err != nil {
		return fmt.Errorf("email sender address is invalid: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Email.MailType)) {
	case "resend":
		baseURL, err := url.Parse(strings.TrimSpace(cfg.Email.ResendAPIURL))
		if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
			return fmt.Errorf("%s must be an absolute HTTP(S) URL", envEmailResendBaseURL)
		}
		if baseURL.Scheme != "https" && !isDevelopmentMode(cfg) {
			return fmt.Errorf("%s must use HTTPS outside development", envEmailResendBaseURL)
		}
	case "smtp":
	default:
		return fmt.Errorf("%s must be one of: resend, smtp", envEmailProvider)
	}

	switch cfg.Email.SMTPSecurity {
	case "", "none", "starttls", "implicit_tls":
	default:
		return fmt.Errorf("%s must be one of: none, starttls, implicit_tls", envEmailSMTPSecurity)
	}

	if requiresResendAPIKey(cfg) && cfg.Email.ResendAPIKey == "" {
		return fmt.Errorf("%s (or %s) is required", envResendAPIKey, envEmailResendAPIKey)
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

	switch cfg.Workflow.ImageInputURLMode {
	case "", WorkflowImageInputURLModeZGIProxy, WorkflowImageInputURLModePublicStorageURL:
	default:
		return fmt.Errorf("%s must be one of: %s, %s", envWorkflowImageInputURLMode, WorkflowImageInputURLModeZGIProxy, WorkflowImageInputURLModePublicStorageURL)
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

	if err := validateConnectorsConfig(cfg.Connectors); err != nil {
		return err
	}

	return nil
}

func validateConsoleAPIURL(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%s is required", envConsoleAPIURL)
	}
	rawURL := strings.TrimSpace(cfg.Console.APIURL)
	if rawURL == "" {
		// Hand-built configs used by isolated package tests may omit optional
		// Console integration settings. The normal loader always supplies the
		// local default before validation.
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL", envConsoleAPIURL)
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not include a path, query, or fragment", envConsoleAPIURL)
	}
	if parsed.Scheme != "https" && !isDevelopmentMode(cfg) {
		return fmt.Errorf("%s must use HTTPS outside development", envConsoleAPIURL)
	}
	return nil
}

func validateConnectorsConfig(cfg ConnectorsConfig) error {
	if err := validateSocialWebhookConfig(cfg); err != nil {
		return err
	}
	if err := validateSocialOAuthConfig(cfg); err != nil {
		return err
	}
	if cfg.SocialMCPScript != "" && (!filepath.IsAbs(cfg.SocialMCPScript) || strings.ContainsAny(cfg.SocialMCPScript, "\x00\r\n")) {
		return fmt.Errorf("CONNECTOR_SOCIAL_MCP_SCRIPT must be an absolute script path")
	}
	if strings.ContainsAny(cfg.SocialMCPNode, "\x00\r\n\t ") {
		return fmt.Errorf("CONNECTOR_SOCIAL_MCP_NODE must be an executable name or path without shell arguments")
	}
	if len(cfg.CredentialKeys) == 0 {
		return fmt.Errorf("%s must configure at least one Connector credential key", envConnectorCredentialKeysJSON)
	}
	if strings.TrimSpace(cfg.CredentialActiveKeyID) == "" {
		return fmt.Errorf("%s is required", envConnectorCredentialActiveKeyID)
	}
	if _, ok := cfg.CredentialKeys[cfg.CredentialActiveKeyID]; !ok {
		return fmt.Errorf("%s must reference a key in %s", envConnectorCredentialActiveKeyID, envConnectorCredentialKeysJSON)
	}
	for keyID, key := range cfg.CredentialKeys {
		if strings.TrimSpace(keyID) == "" {
			return fmt.Errorf("%s contains an empty key id", envConnectorCredentialKeysJSON)
		}
		if len(key) != 32 {
			return fmt.Errorf("credential key %q in %s must be exactly 32 bytes long", keyID, envConnectorCredentialKeysJSON)
		}
	}
	seenDirectories := make(map[string]struct{}, len(cfg.PluginDirectories))
	for _, directory := range cfg.PluginDirectories {
		if strings.TrimSpace(directory) == "" || directory != strings.TrimSpace(directory) {
			return fmt.Errorf("%s contains an invalid directory", envConnectorPluginDirectoriesJSON)
		}
		if _, duplicate := seenDirectories[directory]; duplicate {
			return fmt.Errorf("%s contains a duplicate directory", envConnectorPluginDirectoriesJSON)
		}
		seenDirectories[directory] = struct{}{}
	}
	return nil
}

// X OAuth 2.0 explicitly supports public PKCE clients. Other built-in OAuth
// providers currently use confidential web applications and must retain a
// client secret. Keep this allowlist narrow so a typo cannot silently weaken
// an unrelated provider's client authentication.

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
