package integrations

import (
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

type ConnectionCapabilityPermission struct {
	ActionID        string                   `json:"action_id"`
	Name            string                   `json:"name"`
	NameI18n        LocalizedText            `json:"name_i18n,omitempty"`
	Description     string                   `json:"description,omitempty"`
	DescriptionI18n LocalizedText            `json:"description_i18n,omitempty"`
	Effect          toolgovernance.Effect    `json:"effect"`
	RiskLevel       toolgovernance.RiskLevel `json:"risk_level"`
	ScopeSatisfied  bool                     `json:"scope_satisfied"`
	RequiredScopes  []string                 `json:"required_scopes,omitempty"`
	MissingScopeIDs []string                 `json:"missing_scope_ids,omitempty"`
}

type ConnectionProviderPermission struct {
	ID              string                `json:"id"`
	Label           string                `json:"label"`
	LabelI18n       LocalizedText         `json:"label_i18n,omitempty"`
	Description     string                `json:"description,omitempty"`
	DescriptionI18n LocalizedText         `json:"description_i18n,omitempty"`
	Category        ProviderScopeCategory `json:"category"`
	Access          ProviderScopeAccess   `json:"access"`
	Broad           bool                  `json:"broad"`
	Known           bool                  `json:"known"`
}

// ConnectionPermissionSummary is a secret-free, presentation-only view of a
// connection's provider authorization. Runtime authorization continues to use
// IntegrationConnection.GrantedScopes and the action contract directly.
type ConnectionPermissionSummary struct {
	AdaptedCapabilities    []ConnectionCapabilityPermission `json:"adapted_capabilities"`
	IdentityPermissions    []ConnectionProviderPermission   `json:"identity_permissions"`
	LifecyclePermissions   []ConnectionProviderPermission   `json:"lifecycle_permissions"`
	ProviderPermissions    []ConnectionProviderPermission   `json:"provider_permissions"`
	UnknownPermissions     []ConnectionProviderPermission   `json:"unknown_permissions"`
	MissingPermissions     []ConnectionProviderPermission   `json:"missing_permissions"`
	ProviderScopesReported bool                             `json:"provider_scopes_reported"`
	HasBroadPermissions    bool                             `json:"has_broad_permissions"`
}

type ConnectionPermissionCatalog interface {
	ProviderDefinition(integrationID string) (ProviderDefinition, bool)
}

func BuildConnectionPermissionSummary(connection *IntegrationConnection, definition ProviderDefinition) *ConnectionPermissionSummary {
	if connection == nil {
		return nil
	}
	summary := &ConnectionPermissionSummary{
		AdaptedCapabilities:  []ConnectionCapabilityPermission{},
		IdentityPermissions:  []ConnectionProviderPermission{},
		LifecyclePermissions: []ConnectionProviderPermission{},
		ProviderPermissions:  []ConnectionProviderPermission{},
		UnknownPermissions:   []ConnectionProviderPermission{},
		MissingPermissions:   []ConnectionProviderPermission{},
	}

	granted := normalizedScopeSet(connection.GrantedScopes)
	summary.ProviderScopesReported = len(granted) > 0
	scopeDefinitions := make(map[string]ProviderScopeDefinition, len(definition.Scopes))
	for _, scope := range definition.Scopes {
		scopeDefinitions[scope.ID] = scope
	}
	actionScopeLabels := make(LocalizedLabelMap)
	for _, action := range definition.Actions {
		for scopeID, labels := range action.ScopeLabelsI18n {
			if _, exists := actionScopeLabels[scopeID]; !exists {
				actionScopeLabels[scopeID] = cloneLocalizedText(labels)
			}
		}
		if !ActionSupportsAuthMethod(action, connection.AuthMethodID) {
			continue
		}
		missing := missingConnectionScopes(connection, action.RequiredScopes, granted)
		summary.AdaptedCapabilities = append(summary.AdaptedCapabilities, ConnectionCapabilityPermission{
			ActionID:        action.ID,
			Name:            action.Name,
			NameI18n:        cloneLocalizedText(action.NameI18n),
			Description:     action.Description,
			DescriptionI18n: cloneLocalizedText(action.DescriptionI18n),
			Effect:          action.Effect,
			RiskLevel:       action.RiskLevel,
			ScopeSatisfied:  len(missing) == 0,
			RequiredScopes:  append([]string(nil), action.RequiredScopes...),
			MissingScopeIDs: missing,
		})
	}

	for _, scopeID := range sortedScopeSet(granted) {
		definition, known := scopeDefinitions[scopeID]
		permission := providerPermission(scopeID, definition, known, actionScopeLabels)
		if known && definition.Category == ProviderScopeCategoryInternal {
			continue
		}
		if permission.Broad {
			summary.HasBroadPermissions = true
		}
		switch permission.Category {
		case ProviderScopeCategoryIdentity:
			summary.IdentityPermissions = append(summary.IdentityPermissions, permission)
		case ProviderScopeCategoryLifecycle:
			summary.LifecyclePermissions = append(summary.LifecyclePermissions, permission)
		case ProviderScopeCategoryProvider:
			summary.ProviderPermissions = append(summary.ProviderPermissions, permission)
		default:
			summary.UnknownPermissions = append(summary.UnknownPermissions, permission)
		}
	}

	missingSeen := make(map[string]struct{}, len(connection.MissingRequiredScopes))
	for _, scopeID := range connection.MissingRequiredScopes {
		scopeID = strings.TrimSpace(scopeID)
		if scopeID == "" {
			continue
		}
		if _, exists := missingSeen[scopeID]; exists {
			continue
		}
		missingSeen[scopeID] = struct{}{}
		definition, known := scopeDefinitions[scopeID]
		summary.MissingPermissions = append(
			summary.MissingPermissions,
			providerPermission(scopeID, definition, known, actionScopeLabels),
		)
	}

	sort.Slice(summary.AdaptedCapabilities, func(i, j int) bool {
		return summary.AdaptedCapabilities[i].ActionID < summary.AdaptedCapabilities[j].ActionID
	})
	sortProviderPermissions(summary.IdentityPermissions)
	sortProviderPermissions(summary.LifecyclePermissions)
	sortProviderPermissions(summary.ProviderPermissions)
	sortProviderPermissions(summary.UnknownPermissions)
	sortProviderPermissions(summary.MissingPermissions)
	return summary
}

func missingConnectionScopes(connection *IntegrationConnection, required []string, granted map[string]struct{}) []string {
	if len(required) == 0 {
		return nil
	}
	// This matches Executor behavior: OAuth connections always enforce scopes,
	// while static credentials without a provider scope report are verified by
	// the actual action instead of being declared unavailable.
	if connection.AuthType != ConnectionAuthTypeOAuth2 && len(granted) == 0 {
		return nil
	}
	var missing []string
	for _, scopeID := range normalizeScopeRequirement(required) {
		if _, exists := granted[scopeID]; !exists {
			missing = append(missing, scopeID)
		}
	}
	return missing
}

func providerPermission(
	scopeID string,
	definition ProviderScopeDefinition,
	known bool,
	actionLabels LocalizedLabelMap,
) ConnectionProviderPermission {
	if known {
		return ConnectionProviderPermission{
			ID:              scopeID,
			Label:           definition.Label,
			LabelI18n:       cloneLocalizedText(definition.LabelI18n),
			Description:     definition.Description,
			DescriptionI18n: cloneLocalizedText(definition.DescriptionI18n),
			Category:        definition.Category,
			Access:          definition.Access,
			Broad:           definition.Broad,
			Known:           true,
		}
	}
	labels := cloneLocalizedText(actionLabels[scopeID])
	return ConnectionProviderPermission{
		ID:        scopeID,
		Label:     scopeID,
		LabelI18n: labels,
		Category:  "",
		Access:    ProviderScopeAccessUnknown,
		Known:     len(labels) > 0,
	}
}

func normalizedScopeSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func sortedScopeSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortProviderPermissions(values []ConnectionProviderPermission) {
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
}
