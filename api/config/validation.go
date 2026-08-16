package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
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

	if cfg.ExternalIntegrations.Enabled {
		if err := validateWebSearchConfig(cfg.WebSearch); err != nil {
			return err
		}
	}
	if err := validateExternalIntegrationsConfig(cfg.ExternalIntegrations); err != nil {
		return err
	}
	if err := validateExternalIntegrationOAuthDeployment(cfg); err != nil {
		return err
	}

	return nil
}

func validateExternalIntegrationsConfig(cfg ExternalIntegrationsConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if len(cfg.CredentialKeys) == 0 {
		return fmt.Errorf("%s must configure at least one credential key", envIntegrationCredentialKeysJSON)
	}
	if strings.TrimSpace(cfg.CredentialActiveKeyID) == "" {
		return fmt.Errorf("%s is required when %s is enabled", envIntegrationCredentialActiveKeyID, envExternalIntegrationsEnabled)
	}
	if _, ok := cfg.CredentialKeys[cfg.CredentialActiveKeyID]; !ok {
		return fmt.Errorf("%s must reference a key in %s", envIntegrationCredentialActiveKeyID, envIntegrationCredentialKeysJSON)
	}
	for keyID, key := range cfg.CredentialKeys {
		if strings.TrimSpace(keyID) == "" {
			return fmt.Errorf("%s contains an empty key id", envIntegrationCredentialKeysJSON)
		}
		if len(key) != 32 {
			return fmt.Errorf("credential key %q in %s must be exactly 32 bytes long", keyID, envIntegrationCredentialKeysJSON)
		}
	}
	positive := []struct {
		key   string
		value int
	}{
		{envIntegrationOrgDailyLimit, cfg.OrgDailyLimit},
		{envIntegrationTimeoutSeconds, cfg.TimeoutSeconds},
		{envIntegrationHealthFailureThreshold, cfg.Health.FailureThreshold},
		{envIntegrationOAuthRefreshWindowSeconds, cfg.OAuth.RefreshWindowSeconds},
		{envIntegrationOAuthFlowTTLSeconds, cfg.OAuth.FlowTTLSeconds},
	}
	for _, item := range positive {
		if item.value <= 0 {
			return fmt.Errorf("%s must be greater than 0", item.key)
		}
	}
	if cfg.OAuth.FlowTTLSeconds > 1800 {
		return fmt.Errorf("%s must not exceed 1800", envIntegrationOAuthFlowTTLSeconds)
	}
	if !validIntegrationOAuthURL(cfg.OAuth.CallbackURL) {
		return fmt.Errorf("%s must be an HTTPS URL or a loopback HTTP URL", envIntegrationOAuthCallbackURL)
	}
	if !validIntegrationOAuthURL(cfg.OAuth.ResultURL) {
		return fmt.Errorf("%s must be an HTTPS URL or a loopback HTTP URL", envIntegrationOAuthResultURL)
	}
	for key, client := range cfg.OAuth.Clients {
		if strings.TrimSpace(key) == "" || len(key) > 128 {
			return fmt.Errorf("%s contains an invalid provider key", envIntegrationOAuthClientsJSON)
		}
		if strings.TrimSpace(client.ClientID) == "" {
			return fmt.Errorf("%s client %q is missing client_id", envIntegrationOAuthClientsJSON, key)
		}
		if strings.TrimSpace(client.ClientSecret) == "" && !integrationOAuthPublicClientSupported(key) {
			return fmt.Errorf("%s client %q is missing client_secret", envIntegrationOAuthClientsJSON, key)
		}
	}
	return nil
}

// X OAuth 2.0 explicitly supports public PKCE clients. Other built-in OAuth
// providers currently use confidential web applications and must retain a
// client secret. Keep this allowlist narrow so a typo cannot silently weaken
// an unrelated provider's client authentication.
func integrationOAuthPublicClientSupported(providerKey string) bool {
	key := strings.ToLower(strings.TrimSpace(providerKey))
	key = strings.ReplaceAll(key, "/", ":")
	return key == "x" || key == "x:x"
}

func validIntegrationOAuthURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validateExternalIntegrationOAuthDeployment(cfg *Config) error {
	if cfg == nil || !cfg.ExternalIntegrations.Enabled || isDevelopmentMode(cfg) {
		return nil
	}
	urls := []struct {
		key   string
		value string
	}{
		{envIntegrationOAuthCallbackURL, cfg.ExternalIntegrations.OAuth.CallbackURL},
		{envIntegrationOAuthResultURL, cfg.ExternalIntegrations.OAuth.ResultURL},
	}
	for _, item := range urls {
		parsed, err := url.ParseRequestURI(strings.TrimSpace(item.value))
		if err != nil || parsed == nil ||
			!strings.EqualFold(parsed.Scheme, "https") ||
			isLoopbackIntegrationOAuthHost(parsed.Hostname()) {
			return fmt.Errorf("%s must be an externally reachable HTTPS URL outside development", item.key)
		}
	}
	return nil
}

func isLoopbackIntegrationOAuthHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validateWebSearchConfig(cfg WebSearchConfig) error {
	if !strings.EqualFold(strings.TrimSpace(cfg.Provider), "exa") {
		return fmt.Errorf("%s must be exa", envWebSearchProvider)
	}
	if cfg.Exa.TimeoutSeconds <= 0 {
		return fmt.Errorf("%s must be greater than 0", envExaTimeoutSeconds)
	}
	if cfg.Exa.MaxResults <= 0 {
		return fmt.Errorf("%s must be greater than 0", envExaMaxResults)
	}
	if cfg.Exa.MaxResults > 10 {
		return fmt.Errorf("%s must be less than or equal to 10", envExaMaxResults)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Exa.DefaultSearchType)) {
	case "auto", "fast", "instant":
	default:
		return fmt.Errorf("%s must be one of: auto, fast, instant", envExaDefaultSearchType)
	}
	if cfg.Exa.MaxFetchURLs <= 0 {
		return fmt.Errorf("%s must be greater than 0", envExaMaxFetchURLs)
	}
	if cfg.Exa.MaxFetchURLs > 5 {
		return fmt.Errorf("%s must be less than or equal to 5", envExaMaxFetchURLs)
	}
	if cfg.Exa.MaxContentCharacters <= 0 {
		return fmt.Errorf("%s must be greater than 0", envExaMaxContentCharacters)
	}
	if cfg.Exa.MaxContentCharacters > 20000 {
		return fmt.Errorf("%s must be less than or equal to 20000", envExaMaxContentCharacters)
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
