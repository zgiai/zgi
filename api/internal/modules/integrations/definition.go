package integrations

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationWebSearch = "web-search"
	DriverExa            = "exa"
	// MetaProviderExternalIntegrations is the hidden AIChat connector provider
	// that discovers and invokes actions from the user's selected connections.
	// It is a runtime facade, not a catalog integration or credential owner.
	MetaProviderExternalIntegrations = "external-integrations"

	ActionWebSearch = "web.search"
	ActionWebFetch  = "web.fetch"
)

type ActionDefinition struct {
	ID                   string                   `json:"id"`
	ToolName             string                   `json:"tool_name"`
	Name                 string                   `json:"name"`
	NameI18n             LocalizedText            `json:"name_i18n,omitempty"`
	Description          string                   `json:"description,omitempty"`
	DescriptionI18n      LocalizedText            `json:"description_i18n,omitempty"`
	InputSchema          map[string]interface{}   `json:"input_schema"`
	OutputSchema         map[string]interface{}   `json:"output_schema"`
	Effect               toolgovernance.Effect    `json:"effect"`
	RiskLevel            toolgovernance.RiskLevel `json:"risk_level"`
	DataEgress           bool                     `json:"data_egress"`
	ExternalDestination  string                   `json:"external_destination,omitempty"`
	SensitiveDataAllowed bool                     `json:"sensitive_data_allowed"`
	Idempotent           bool                     `json:"idempotent"`
	RequiredScopes       []string                 `json:"required_scopes,omitempty"`
	// SupportedAuthMethodIDs restricts this action to provider auth methods
	// that can actually execute it. An empty list keeps the legacy behavior of
	// supporting every auth method declared by the provider.
	SupportedAuthMethodIDs []string               `json:"supported_auth_method_ids,omitempty"`
	ScopeLabelsI18n        LocalizedLabelMap      `json:"scope_labels_i18n,omitempty"`
	DefaultPolicy          *DefaultActionPolicy   `json:"default_policy"`
	SchemaHash             string                 `json:"schema_hash"`
	SchemaRevision         string                 `json:"schema_revision"`
	CatalogRevision        string                 `json:"catalog_revision"`
	SupportedCallers       []tools.ToolInvokeFrom `json:"supported_callers,omitempty"`
}

type ActionRequest struct {
	OrganizationID string
	WorkspaceID    string
	UserID         string
	AgentID        string
	ConversationID string
	AppID          string
	MessageID      string
	ConnectionID   string
	InvokeFrom     tools.ToolInvokeFrom
	IntegrationID  string
	ActionID       string
	Input          map[string]interface{}
	// Connection is populated by Executor after organization-scoped resolution.
	// Callers must never provide credentials directly.
	Connection *ResolvedConnection
	// VerifyAgentConnection is a request-scoped verifier that retains the exact
	// draft/published Agent binding scope selected by chatruntime.
	VerifyAgentConnection func(context.Context, AgentConnectionAuthorizationRequest) (bool, error)
}

type ActionResult struct {
	Output            map[string]interface{}
	ProviderRequestID string
	CostUSD           *float64
	ResultCount       int
	AttemptCount      int
}

type Adapter interface {
	DriverID() string
	Execute(ctx context.Context, req ActionRequest) (*ActionResult, error)
}

type AgentConnectionAuthorizationRequest struct {
	OrganizationID string
	AgentID        string
	ConnectionID   string
	IntegrationID  string
	ActionID       string
}

// AgentConnectionAuthorizer rechecks a persisted Agent connection binding at
// invocation time. This prevents a stale published/draft runtime snapshot from
// authorizing a connection or action that has since been removed.
type AgentConnectionAuthorizer interface {
	AuthorizeAgentConnection(context.Context, AgentConnectionAuthorizationRequest) error
}

type Registration struct {
	// Definition is the canonical provider catalog contract. IntegrationID and
	// Actions remain as a transition path for the phase-one Exa registration.
	Definition          ProviderDefinition
	IntegrationID       string
	Adapter             Adapter
	Actions             []ActionDefinition
	ConnectionTester    ConnectionTester
	CredentialValidator CredentialValidator
	HealthProbe         HealthProbe
	GovernanceResolver  DynamicActionGovernanceResolver
	OAuth2Provider      OAuth2Provider
}

type ResolvedAction struct {
	IntegrationID string
	Adapter       Adapter
	Definition    ActionDefinition
}

// ActionSupportsAuthMethod reports whether a connection using authMethodID can
// execute action. Provider definitions are normalized to lowercase; callers
// may safely pass persisted legacy values with arbitrary casing.
func ActionSupportsAuthMethod(action ActionDefinition, authMethodID string) bool {
	if len(action.SupportedAuthMethodIDs) == 0 {
		return true
	}
	authMethodID = strings.ToLower(strings.TrimSpace(authMethodID))
	if authMethodID == "" {
		return false
	}
	for _, supported := range action.SupportedAuthMethodIDs {
		if strings.EqualFold(strings.TrimSpace(supported), authMethodID) {
			return true
		}
	}
	return false
}
