package integrations

import (
	"context"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type HealthProbeDefinition struct {
	Supported       bool          `json:"supported"`
	MayIncurCost    bool          `json:"may_incur_cost"`
	Description     string        `json:"description,omitempty"`
	DescriptionI18n LocalizedText `json:"description_i18n,omitempty"`
}

type HealthProbeStatus string

const (
	HealthProbeStatusHealthy   HealthProbeStatus = "healthy"
	HealthProbeStatusDegraded  HealthProbeStatus = "degraded"
	HealthProbeStatusUnhealthy HealthProbeStatus = "unhealthy"
	HealthProbeStatusUnknown   HealthProbeStatus = "unknown"
)

type HealthProbeCheck struct {
	Code        string            `json:"code"`
	Status      HealthProbeStatus `json:"status"`
	Message     string            `json:"message,omitempty"`
	Remediation string            `json:"remediation,omitempty"`
}

type HealthProbeReport struct {
	Status  HealthProbeStatus  `json:"status"`
	Checks  []HealthProbeCheck `json:"checks,omitempty"`
	Profile *ConnectionProfile `json:"profile,omitempty"`
}

// HealthProbe is a provider-owned diagnostic. Implementations receive only a
// request-scoped resolved connection and must not retain its secret material.
type HealthProbe interface {
	ProbeConnection(context.Context, *ResolvedConnection) (*HealthProbeReport, error)
}

type DefaultActionPolicy struct {
	Enabled           bool                          `json:"enabled"`
	ApprovalPolicy    toolgovernance.ApprovalPolicy `json:"approval_policy"`
	DataEgressAllowed bool                          `json:"data_egress_allowed"`
}

type ProviderScopeCategory string

const (
	ProviderScopeCategoryIdentity  ProviderScopeCategory = "identity"
	ProviderScopeCategoryLifecycle ProviderScopeCategory = "lifecycle"
	ProviderScopeCategoryProvider  ProviderScopeCategory = "provider"
	ProviderScopeCategoryInternal  ProviderScopeCategory = "internal"
)

type ProviderScopeAccess string

const (
	ProviderScopeAccessUnknown  ProviderScopeAccess = "unknown"
	ProviderScopeAccessRead     ProviderScopeAccess = "read"
	ProviderScopeAccessWrite    ProviderScopeAccess = "write"
	ProviderScopeAccessManage   ProviderScopeAccess = "manage"
	ProviderScopeAccessIdentity ProviderScopeAccess = "identity"
	ProviderScopeAccessSession  ProviderScopeAccess = "session"
)

// ProviderScopeDefinition describes a provider-owned authorization scope for
// safe management-UI presentation. It never replaces the provider scope value
// used by runtime authorization.
type ProviderScopeDefinition struct {
	ID              string                `json:"id"`
	Label           string                `json:"label"`
	LabelI18n       LocalizedText         `json:"label_i18n,omitempty"`
	Description     string                `json:"description,omitempty"`
	DescriptionI18n LocalizedText         `json:"description_i18n,omitempty"`
	Category        ProviderScopeCategory `json:"category"`
	Access          ProviderScopeAccess   `json:"access"`
	Broad           bool                  `json:"broad"`
}

type ProviderDefinition struct {
	ID                   string                    `json:"id"`
	DriverID             string                    `json:"driver_id"`
	Name                 string                    `json:"name"`
	NameI18n             LocalizedText             `json:"name_i18n,omitempty"`
	Description          string                    `json:"description,omitempty"`
	DescriptionI18n      LocalizedText             `json:"description_i18n,omitempty"`
	Author               string                    `json:"author,omitempty"`
	Icon                 string                    `json:"icon,omitempty"`
	Tags                 []string                  `json:"tags,omitempty"`
	TagLabelsI18n        LocalizedLabelMap         `json:"tag_labels_i18n,omitempty"`
	Categories           []string                  `json:"categories,omitempty"`
	CategoryLabelsI18n   LocalizedLabelMap         `json:"category_labels_i18n,omitempty"`
	DocumentationURL     string                    `json:"documentation_url,omitempty"`
	DocumentationURLI18n LocalizedText             `json:"documentation_url_i18n,omitempty"`
	CatalogRevision      string                    `json:"catalog_revision"`
	AuthMethods          []AuthMethodDefinition    `json:"auth"`
	HealthProbe          HealthProbeDefinition     `json:"health_probe"`
	Scopes               []ProviderScopeDefinition `json:"scopes,omitempty"`
	Actions              []ActionDefinition        `json:"actions"`
}

// ProviderCatalogItem is the strongly typed, secret-free wire contract. The
// compatibility fields can be removed after the phase-one frontend migrates to
// auth and health_probe.
type ProviderCatalogItem struct {
	ID                         string                       `json:"id"`
	IntegrationID              string                       `json:"integration_id"`
	DriverID                   string                       `json:"driver_id"`
	Name                       string                       `json:"name"`
	NameI18n                   LocalizedText                `json:"name_i18n,omitempty"`
	Description                string                       `json:"description,omitempty"`
	DescriptionI18n            LocalizedText                `json:"description_i18n,omitempty"`
	Author                     string                       `json:"author,omitempty"`
	Icon                       string                       `json:"icon,omitempty"`
	Tags                       []string                     `json:"tags,omitempty"`
	TagLabelsI18n              LocalizedLabelMap            `json:"tag_labels_i18n,omitempty"`
	Categories                 []string                     `json:"categories,omitempty"`
	CategoryLabelsI18n         LocalizedLabelMap            `json:"category_labels_i18n,omitempty"`
	DocumentationURL           string                       `json:"documentation_url,omitempty"`
	DocumentationURLI18n       LocalizedText                `json:"documentation_url_i18n,omitempty"`
	Enabled                    bool                         `json:"enabled"`
	CatalogRevision            string                       `json:"catalog_revision"`
	Auth                       []AuthMethodDefinition       `json:"auth"`
	HealthProbe                HealthProbeDefinition        `json:"health_probe"`
	Scopes                     []ProviderScopeDefinition    `json:"scopes,omitempty"`
	CredentialSources          []ConnectionCredentialSource `json:"credential_sources"`
	AuthTypes                  []AuthMethodType             `json:"auth_types"`
	ConnectionTestMayIncurCost bool                         `json:"connection_test_may_incur_cost"`
	ConnectionSummary          *ProviderConnectionSummary   `json:"connection_summary,omitempty"`
	HealthState                ProviderHealthState          `json:"health_state,omitempty"`
	Actions                    []ActionSummary              `json:"actions"`
}

// ProviderHealthState is an aggregate of connection observations visible to
// the current caller. Configured deliberately means "credentials or a
// connection exist, but no successful health observation proves readiness".
type ProviderHealthState string

const (
	ProviderHealthStateReady         ProviderHealthState = "ready"
	ProviderHealthStateConfigured    ProviderHealthState = "configured"
	ProviderHealthStateSetupRequired ProviderHealthState = "setup_required"
	ProviderHealthStateDegraded      ProviderHealthState = "degraded"
	ProviderHealthStateUnavailable   ProviderHealthState = "unavailable"
	ProviderHealthStateUnknown       ProviderHealthState = "unknown"
)

// ProviderConnectionSummary contains aggregate metadata only. Its input must
// already have passed the caller-specific visibility check; connection names,
// owner identities, credentials, and per-connection error details never enter
// the provider catalog response.
type ProviderConnectionSummary struct {
	Total               int     `json:"total"`
	Active              int     `json:"active"`
	Invalid             int     `json:"invalid"`
	Disabled            int     `json:"disabled"`
	Healthy             int     `json:"healthy"`
	Degraded            int     `json:"degraded"`
	Unhealthy           int     `json:"unhealthy"`
	Unknown             int     `json:"unknown"`
	AuthRequired        int     `json:"auth_required"`
	ScopeDrifted        int     `json:"scope_drifted"`
	DefaultConnectionID *string `json:"default_connection_id,omitempty"`
}

type ActionSummary struct {
	IntegrationID          string                   `json:"integration_id"`
	DriverID               string                   `json:"driver_id"`
	ID                     string                   `json:"id"`
	ToolName               string                   `json:"tool_name"`
	Name                   string                   `json:"name"`
	NameI18n               LocalizedText            `json:"name_i18n,omitempty"`
	Description            string                   `json:"description,omitempty"`
	DescriptionI18n        LocalizedText            `json:"description_i18n,omitempty"`
	Effect                 toolgovernance.Effect    `json:"effect"`
	RiskLevel              toolgovernance.RiskLevel `json:"risk_level"`
	DataEgress             bool                     `json:"data_egress"`
	ExternalDestination    string                   `json:"external_destination,omitempty"`
	RequiredScopes         []string                 `json:"required_scopes,omitempty"`
	RequiredAnyScopes      []string                 `json:"required_any_scopes,omitempty"`
	PreferredScopes        []string                 `json:"preferred_scopes,omitempty"`
	SupportedAuthMethodIDs []string                 `json:"supported_auth_method_ids,omitempty"`
	ScopeLabelsI18n        LocalizedLabelMap        `json:"scope_labels_i18n,omitempty"`
	DefaultPolicy          DefaultActionPolicy      `json:"default_policy"`
	SchemaHash             string                   `json:"schema_hash"`
	SchemaRevision         string                   `json:"schema_revision"`
	CatalogRevision        string                   `json:"catalog_revision"`
	SupportedCallers       []tools.ToolInvokeFrom   `json:"supported_callers,omitempty"`
	SupportsBatch          bool                     `json:"supports_batch"`
}

type ActionSearchRequest struct {
	Query         string
	IntegrationID string
	Caller        tools.ToolInvokeFrom
	Limit         int
}

type ActionGovernanceRequest struct {
	OrganizationID string
	UserID         string
	IntegrationID  string
	ActionID       string
	InvokeFrom     tools.ToolInvokeFrom
	Input          map[string]interface{}
	Baseline       ActionDefinition
}

// DynamicActionGovernanceResolver is an optional provider hook for actions
// whose destination or risk depends on validated input. The shared integration
// runtime remains responsible for ensuring the result cannot weaken Baseline.
type DynamicActionGovernanceResolver interface {
	ResolveActionGovernance(context.Context, ActionGovernanceRequest) (ActionDefinition, error)
}

func defaultProviderDefinition(integrationID, driverID string, actions []ActionDefinition) ProviderDefinition {
	return ProviderDefinition{
		ID:          integrationID,
		DriverID:    driverID,
		Name:        humanizeIdentifier(integrationID),
		NameI18n:    LocalizedText{LocaleEnglishUS: humanizeIdentifier(integrationID)},
		Description: "External integration",
		DescriptionI18n: LocalizedText{
			LocaleEnglishUS:         "External integration",
			LocaleSimplifiedChinese: "外部集成",
		},
		Author: "ZGI",
		Icon:   "plug",
		Tags:   []string{"external"},
		TagLabelsI18n: LocalizedLabelMap{
			"external": {LocaleEnglishUS: "External", LocaleSimplifiedChinese: "外部应用"},
		},
		Categories: []string{"external"},
		CategoryLabelsI18n: LocalizedLabelMap{
			"external": {LocaleEnglishUS: "External applications", LocaleSimplifiedChinese: "外部应用"},
		},
		AuthMethods: []AuthMethodDefinition{
			{
				ID:               string(AuthMethodTypeAPIKey),
				Type:             AuthMethodTypeAPIKey,
				CredentialSource: ConnectionCredentialSourceOrganization,
				Label:            "API key",
				LabelI18n: LocalizedText{
					LocaleEnglishUS:         "API key",
					LocaleSimplifiedChinese: "API 密钥",
				},
				Available: true,
				Fields: []CredentialFieldDefinition{{
					Key: "api_key", Label: "API key",
					LabelI18n: LocalizedText{LocaleEnglishUS: "API key", LocaleSimplifiedChinese: "API 密钥"},
					Input:     CredentialFieldInputPassword, Required: true, Secret: true,
				}},
			},
		},
		HealthProbe: HealthProbeDefinition{Supported: true, MayIncurCost: true},
		Actions:     actions,
	}
}
