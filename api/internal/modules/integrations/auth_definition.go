package integrations

import (
	"context"
	"strings"
)

// AuthMethodType describes how a connection obtains provider credentials. It
// is deliberately independent from the persisted ConnectionAuthType so the
// catalog can advertise auth methods before their storage workflow is enabled.
type AuthMethodType string

const (
	// AuthMethodTypePlatform is retained only to identify and reject legacy
	// provider definitions. External integrations no longer advertise or
	// execute deployment-owned credentials.
	AuthMethodTypePlatform         AuthMethodType = "platform"
	AuthMethodTypeAPIKey           AuthMethodType = "api_key"
	AuthMethodTypeCustomCredential AuthMethodType = "custom_credential"
	AuthMethodTypeOAuth2           AuthMethodType = "oauth2"
	AuthMethodTypeServiceAccount   AuthMethodType = "service_account"
	AuthMethodTypeNone             AuthMethodType = "no_auth"
)

// AuthIdentityKind describes the external identity represented by a
// connection. It is intentionally separate from CredentialSource, which
// describes who owns and may manage the connection inside ZGI.
type AuthIdentityKind string

const (
	AuthIdentityKindUser        AuthIdentityKind = "user"
	AuthIdentityKindApplication AuthIdentityKind = "application"
	AuthIdentityKindChannel     AuthIdentityKind = "channel"
	AuthIdentityKindService     AuthIdentityKind = "service"
)

// AuthAcquisitionStrategy describes how durable credential material enters a
// connection. These values are catalog metadata; they never contain runtime
// endpoints or secret values.
type AuthAcquisitionStrategy string

const (
	AuthAcquisitionStrategyBrowserRedirect AuthAcquisitionStrategy = "browser_redirect"
	AuthAcquisitionStrategyManualForm      AuthAcquisitionStrategy = "manual_form"
	AuthAcquisitionStrategyNone            AuthAcquisitionStrategy = "none"
)

// AuthLifecycleStrategy describes how a connection maintains usable runtime
// credentials without introducing provider-specific auth method types.
type AuthLifecycleStrategy string

const (
	AuthLifecycleStrategyStatic           AuthLifecycleStrategy = "static"
	AuthLifecycleStrategyOAuthRefresh     AuthLifecycleStrategy = "oauth_refresh"
	AuthLifecycleStrategyExchangeOnDemand AuthLifecycleStrategy = "exchange_on_demand"
	AuthLifecycleStrategySignedRequest    AuthLifecycleStrategy = "signed_request"
)

// RequestAuthStrategy describes the generic mechanism used at the outbound
// request boundary. ProviderCustom is the explicit escape hatch for protocols
// that cannot be represented safely by a shared request authenticator.
type RequestAuthStrategy string

const (
	RequestAuthStrategyBearerHeader    RequestAuthStrategy = "bearer_header"
	RequestAuthStrategyAPIKeyHeader    RequestAuthStrategy = "api_key_header"
	RequestAuthStrategyAPIKeyQuery     RequestAuthStrategy = "api_key_query"
	RequestAuthStrategyBasicHeader     RequestAuthStrategy = "basic_header"
	RequestAuthStrategyOAuth1Signature RequestAuthStrategy = "oauth1_signature"
	RequestAuthStrategyWebhookURL      RequestAuthStrategy = "webhook_url"
	RequestAuthStrategyProviderCustom  RequestAuthStrategy = "provider_custom"
	RequestAuthStrategyNone            RequestAuthStrategy = "none"
)

// AuthScopeEvidence describes where a connection's stored scope snapshot
// comes from. Provider-reported snapshots may be used to present exact grant
// coverage. Connector-declared scopes describe the adapter contract only;
// the provider still verifies each operation at runtime.
type AuthScopeEvidence string

const (
	AuthScopeEvidenceProviderReported  AuthScopeEvidence = "provider_reported"
	AuthScopeEvidenceConnectorDeclared AuthScopeEvidence = "connector_declared"
)

type CredentialFieldInput string

const (
	CredentialFieldInputText     CredentialFieldInput = "text"
	CredentialFieldInputPassword CredentialFieldInput = "password"
	CredentialFieldInputTextarea CredentialFieldInput = "textarea"
	CredentialFieldInputSelect   CredentialFieldInput = "select"
	CredentialFieldInputURL      CredentialFieldInput = "url"
)

type CredentialFieldOption struct {
	Value     string        `json:"value"`
	Label     string        `json:"label"`
	LabelI18n LocalizedText `json:"label_i18n,omitempty"`
}

// CredentialFieldDefinition is safe catalog metadata. Secret values are never
// represented here or returned by a catalog endpoint.
type CredentialFieldDefinition struct {
	Key             string                  `json:"key"`
	Label           string                  `json:"label"`
	LabelI18n       LocalizedText           `json:"label_i18n,omitempty"`
	Description     string                  `json:"description,omitempty"`
	DescriptionI18n LocalizedText           `json:"description_i18n,omitempty"`
	Input           CredentialFieldInput    `json:"input"`
	Required        bool                    `json:"required"`
	Secret          bool                    `json:"secret"`
	Placeholder     string                  `json:"placeholder,omitempty"`
	PlaceholderI18n LocalizedText           `json:"placeholder_i18n,omitempty"`
	Options         []CredentialFieldOption `json:"options,omitempty"`
}

type AuthSetupStepAction string

const (
	AuthSetupStepActionNone              AuthSetupStepAction = ""
	AuthSetupStepActionOpenConsole       AuthSetupStepAction = "open_console"
	AuthSetupStepActionOpenDocumentation AuthSetupStepAction = "open_documentation"
	AuthSetupStepActionCopyCallbackURL   AuthSetupStepAction = "copy_callback_url"
)

type AuthSetupNoticeLevel string

const (
	AuthSetupNoticeLevelInfo    AuthSetupNoticeLevel = "info"
	AuthSetupNoticeLevelWarning AuthSetupNoticeLevel = "warning"
)

// AuthSetupStepDefinition is browser-safe, provider-owned onboarding content.
// It describes where administrators must configure an external application;
// it never contains credential values or runtime OAuth endpoints.
type AuthSetupStepDefinition struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	TitleI18n       LocalizedText       `json:"title_i18n,omitempty"`
	Description     string              `json:"description,omitempty"`
	DescriptionI18n LocalizedText       `json:"description_i18n,omitempty"`
	Action          AuthSetupStepAction `json:"action,omitempty"`
}

type AuthSetupNoticeDefinition struct {
	ID       string               `json:"id"`
	Level    AuthSetupNoticeLevel `json:"level"`
	Text     string               `json:"text"`
	TextI18n LocalizedText        `json:"text_i18n,omitempty"`
}

// AuthSetupGuideDefinition is a secret-free setup guide for one authentication
// method. Keeping it on AuthMethodDefinition lets OAuth, service-account, and
// token-based methods share the same UI without provider-specific branching.
type AuthSetupGuideDefinition struct {
	ConsoleURL        string                      `json:"console_url,omitempty"`
	DocumentationURL  string                      `json:"documentation_url,omitempty"`
	ExpandedByDefault bool                        `json:"expanded_by_default,omitempty"`
	Steps             []AuthSetupStepDefinition   `json:"steps,omitempty"`
	Notices           []AuthSetupNoticeDefinition `json:"notices,omitempty"`
}

// OAuthMethodMetadata is safe to expose through the provider catalog.
// Provider endpoints and OAuth client credentials intentionally remain in
// server-side contracts.
type OAuthMethodMetadata struct {
	ConnectEnabled      bool   `json:"connect_enabled"`
	ReconnectEnabled    bool   `json:"reconnect_enabled"`
	ScopeUpgradeEnabled bool   `json:"scope_upgrade_enabled"`
	ClientConfigured    bool   `json:"client_configured"`
	ClientConfigID      string `json:"client_config_id"`
	// ProviderSetupURL points administrators to the provider-owned OAuth
	// application console. It is catalog metadata, never an OAuth runtime
	// endpoint and never derived from browser input.
	ProviderSetupURL string `json:"provider_setup_url,omitempty"`
	// IdentityScopes are the provider-owned minimum scopes required to
	// establish and maintain the connected identity after OAuth completes.
	// They must be requestable provider scopes; scopes that merely appear in
	// a token response must not be declared here. They are always added
	// server-side, even when the user selects only a write action.
	IdentityScopes   []string                    `json:"identity_scopes,omitempty"`
	DefaultActionIDs []string                    `json:"default_action_ids,omitempty"`
	ClientFields     []CredentialFieldDefinition `json:"client_fields,omitempty"`
}

type AuthMethodDefinition struct {
	ID                  string                      `json:"id"`
	Type                AuthMethodType              `json:"type"`
	CredentialSource    ConnectionCredentialSource  `json:"credential_source"`
	IdentityKind        AuthIdentityKind            `json:"identity_kind"`
	AcquisitionStrategy AuthAcquisitionStrategy     `json:"acquisition_strategy"`
	LifecycleStrategy   AuthLifecycleStrategy       `json:"lifecycle_strategy"`
	RequestAuthStrategy RequestAuthStrategy         `json:"request_auth_strategy"`
	ScopeEvidence       AuthScopeEvidence           `json:"scope_evidence"`
	Label               string                      `json:"label"`
	LabelI18n           LocalizedText               `json:"label_i18n,omitempty"`
	Description         string                      `json:"description,omitempty"`
	DescriptionI18n     LocalizedText               `json:"description_i18n,omitempty"`
	Available           bool                        `json:"available"`
	Fields              []CredentialFieldDefinition `json:"fields,omitempty"`
	OAuth               *OAuthMethodMetadata        `json:"oauth,omitempty"`
	SetupGuide          *AuthSetupGuideDefinition   `json:"setup_guide,omitempty"`
}

// CredentialValidationRequest contains request-scoped candidate values. An
// implementation must not retain, serialize, or log Credentials.
type CredentialValidationRequest struct {
	IntegrationID string
	DriverID      string
	AuthMethodID  string
	Credentials   map[string]string
	Config        map[string]interface{}
}

// CredentialValidator optionally adds provider-specific validation after the
// catalog's structural field validation.
type CredentialValidator interface {
	ValidateCredentials(context.Context, CredentialValidationRequest) error
}

type CredentialValidatorFunc func(context.Context, CredentialValidationRequest) error

func (validator CredentialValidatorFunc) ValidateCredentials(ctx context.Context, request CredentialValidationRequest) error {
	return validator(ctx, request)
}

func validAuthMethodType(value AuthMethodType) bool {
	switch AuthMethodType(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AuthMethodTypeAPIKey, AuthMethodTypeCustomCredential, AuthMethodTypeOAuth2, AuthMethodTypeServiceAccount, AuthMethodTypeNone:
		return true
	default:
		return false
	}
}

func validCredentialFieldInput(value CredentialFieldInput) bool {
	switch CredentialFieldInput(strings.ToLower(strings.TrimSpace(string(value)))) {
	case CredentialFieldInputText, CredentialFieldInputPassword, CredentialFieldInputTextarea, CredentialFieldInputSelect, CredentialFieldInputURL:
		return true
	default:
		return false
	}
}

func validAuthIdentityKind(value AuthIdentityKind) bool {
	switch AuthIdentityKind(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AuthIdentityKindUser, AuthIdentityKindApplication, AuthIdentityKindChannel, AuthIdentityKindService:
		return true
	default:
		return false
	}
}

func validAuthScopeEvidence(value AuthScopeEvidence) bool {
	switch AuthScopeEvidence(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AuthScopeEvidenceProviderReported, AuthScopeEvidenceConnectorDeclared:
		return true
	default:
		return false
	}
}

func validAuthAcquisitionStrategy(value AuthAcquisitionStrategy) bool {
	switch AuthAcquisitionStrategy(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AuthAcquisitionStrategyBrowserRedirect, AuthAcquisitionStrategyManualForm, AuthAcquisitionStrategyNone:
		return true
	default:
		return false
	}
}

func validAuthLifecycleStrategy(value AuthLifecycleStrategy) bool {
	switch AuthLifecycleStrategy(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AuthLifecycleStrategyStatic, AuthLifecycleStrategyOAuthRefresh, AuthLifecycleStrategyExchangeOnDemand, AuthLifecycleStrategySignedRequest:
		return true
	default:
		return false
	}
}

func validRequestAuthStrategy(value RequestAuthStrategy) bool {
	switch RequestAuthStrategy(strings.ToLower(strings.TrimSpace(string(value)))) {
	case RequestAuthStrategyBearerHeader, RequestAuthStrategyAPIKeyHeader, RequestAuthStrategyAPIKeyQuery,
		RequestAuthStrategyBasicHeader, RequestAuthStrategyOAuth1Signature, RequestAuthStrategyWebhookURL,
		RequestAuthStrategyProviderCustom, RequestAuthStrategyNone:
		return true
	default:
		return false
	}
}
