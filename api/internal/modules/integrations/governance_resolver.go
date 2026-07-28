package integrations

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// GovernanceManifestResolver applies organization policy before the shared
// approval engine freezes an invocation. It can only make provider governance
// stricter; effect, risk, and external destination remain provider-owned.
type GovernanceManifestResolver struct {
	registry *Registry
	policies ActionPolicyResolver
	safety   SafetyChecker
}

func NewGovernanceManifestResolver(registry *Registry, policies ActionPolicyResolver) *GovernanceManifestResolver {
	return &GovernanceManifestResolver{registry: registry, policies: policies, safety: DefaultSafetyChecker{}}
}

func (resolver *GovernanceManifestResolver) WithSafetyChecker(safety SafetyChecker) *GovernanceManifestResolver {
	if resolver != nil && safety != nil {
		resolver.safety = safety
	}
	return resolver
}

func (resolver *GovernanceManifestResolver) ResolveToolGovernanceManifest(ctx context.Context, req skills.ToolGovernanceRequest) (toolgovernance.Manifest, error) {
	manifest := req.Manifest
	if req.ProviderType != tools.ToolProviderTypeConnector {
		return manifest, nil
	}
	integrationID := strings.ToLower(strings.TrimSpace(req.ProviderID))
	toolName := strings.ToLower(strings.TrimSpace(req.ToolName))
	if integrationID == MetaProviderExternalIntegrations && toolName != "execute_action" {
		// The remaining facade tools read the in-process provider catalog and
		// selected-connection metadata; they are not provider actions.
		return manifest, nil
	}
	if resolver == nil || resolver.registry == nil {
		return manifest, NewError(ErrorCodeDisabled, "integration governance registry is unavailable", nil)
	}
	if resolver.policies == nil {
		return manifest, NewError(ErrorCodeAccessDenied, "integration action policy is unavailable", nil)
	}
	var action *ActionDefinition
	if integrationID == MetaProviderExternalIntegrations {
		if req.ExecutionContext.InvokeFrom != tools.ToolInvokeFromAIChat &&
			req.ExecutionContext.InvokeFrom != tools.ToolInvokeFromAgent {
			return req.Manifest, NewError(ErrorCodeAccessDenied, "external integration meta tools are not available to this caller", nil)
		}
		integrationID = strings.ToLower(governanceArgumentString(req.Arguments, "integration_id"))
		actionID := strings.ToLower(governanceArgumentString(req.Arguments, "action_id"))
		connectionID := governanceArgumentString(req.Arguments, "connection_id")
		connectionSelector := strings.ToLower(governanceArgumentString(req.Arguments, "connection_selector"))
		if connectionID != "" && connectionSelector != "" {
			return req.Manifest, invalidInput("execute_action accepts either connection_id or connection_selector, not both", nil)
		}
		if connectionSelector != "" && connectionSelector != "preferred" {
			return req.Manifest, invalidInput("execute_action connection_selector is invalid", nil)
		}
		if integrationID == "" || actionID == "" {
			return req.Manifest, invalidInput("execute_action requires integration_id and action_id", nil)
		}
		if connectionID == "" {
			// The meta-tool argument enricher must turn an omitted connection or
			// the preferred selector into a canonical UUID before governance. A
			// remaining selector means that no safe selected preference resolved.
			return req.Manifest, NewError(ErrorCodeAccessDenied, "execute_action preferred connection could not be resolved for this chat", nil)
		}
		parsedConnectionID, parseErr := uuid.Parse(connectionID)
		if parseErr != nil || parsedConnectionID == uuid.Nil {
			return req.Manifest, invalidInput("execute_action connection_id is invalid", parseErr)
		}
		connectionID = parsedConnectionID.String()
		// Keep the AssetRef identity canonical even if a non-model caller used
		// another UUID spelling. This map is a governance-owned request copy;
		// the actual invocation remains frozen separately.
		req.Arguments["connection_id"] = connectionID
		if !metaConnectionSelected(req.ExecutionContext.RuntimeParameters, integrationID, connectionID) {
			return req.Manifest, NewError(ErrorCodeAccessDenied, "integration connection is not selected for this chat", nil)
		}
		resolved, resolveErr := resolver.registry.ResolveDynamicActionGovernance(ctx, ActionGovernanceRequest{
			OrganizationID: req.ExecutionContext.OrganizationID,
			UserID:         req.ExecutionContext.UserID,
			IntegrationID:  integrationID,
			ActionID:       actionID,
			InvokeFrom:     req.ExecutionContext.InvokeFrom,
			Input:          nestedActionArguments(req.Arguments),
		})
		if resolveErr != nil {
			return req.Manifest, resolveErr
		}
		if err := validateMetaActionRevisionArguments(req.Arguments, resolved); err != nil {
			return req.Manifest, err
		}
		action = &resolved
		manifest = manifestForIntegrationAction(req.Manifest, integrationID, connectionID, resolved)
	} else {
		for _, candidate := range resolver.registry.Actions(integrationID) {
			if strings.EqualFold(candidate.ToolName, req.ToolName) {
				copyCandidate := candidate
				action = &copyCandidate
				break
			}
		}
	}
	if action == nil {
		return manifest, invalidInput("unknown integration action", nil)
	}
	if !actionSupportsCaller(*action, req.ExecutionContext.InvokeFrom) {
		return manifest, invalidInput("integration action is not available to this caller", nil)
	}
	if req.ExecutionContext.InvokeFrom == tools.ToolInvokeFromAgent && action.Effect != toolgovernance.EffectRead {
		return manifest, NewError(ErrorCodeAccessDenied, "write actions are unavailable in the non-interactive Agent runtime", nil)
	}
	if resolver.safety == nil {
		return manifest, NewError(ErrorCodeSensitiveInput, "integration governance safety check is unavailable", nil)
	}
	actionInput := req.Arguments
	if strings.EqualFold(strings.TrimSpace(req.ProviderID), MetaProviderExternalIntegrations) {
		actionInput = nestedActionArguments(req.Arguments)
	}
	if err := resolver.safety.Check(ctx, *action, actionInput); err != nil {
		return manifest, err
	}
	if action.DefaultPolicy != nil {
		if action.DataEgress && !action.DefaultPolicy.DataEgressAllowed {
			return manifest, NewError(ErrorCodeAccessDenied, "provider policy blocks data egress for this integration action", nil)
		}
	}
	decision, err := resolver.policies.Resolve(ctx, req.ExecutionContext.OrganizationID, integrationID, *action)
	if err != nil {
		return manifest, err
	}
	if !decision.Enabled {
		return manifest, NewError(ErrorCodeDisabled, "integration action is disabled by organization policy", nil)
	}
	if action.DataEgress && !decision.DataEgressAllowed {
		return manifest, NewError(ErrorCodeAccessDenied, "organization policy blocks data egress for this integration action", nil)
	}
	if decision.ApprovalPolicy == IntegrationApprovalPolicyAlwaysAsk {
		manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyAlwaysAsk
		manifest.ApprovalEveryInvocation = true
	}
	if req.ExecutionContext.InvokeFrom == tools.ToolInvokeFromAgent &&
		(manifest.ApprovalEveryInvocation ||
			toolgovernance.NormalizeApprovalPolicy(manifest.DefaultApprovalPolicy) == toolgovernance.ApprovalPolicyAlwaysAsk) {
		return manifest, NewError(ErrorCodeAccessDenied, "actions requiring interactive approval are unavailable in the Agent runtime", nil)
	}
	return manifest, nil
}

func governanceArgumentString(arguments map[string]interface{}, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}

func validateMetaActionRevisionArguments(arguments map[string]interface{}, action ActionDefinition) error {
	checks := []struct {
		key  string
		want string
	}{
		{key: "action_schema_hash", want: action.SchemaHash},
		{key: "action_schema_revision", want: action.SchemaRevision},
		{key: "catalog_revision", want: action.CatalogRevision},
	}
	for _, check := range checks {
		if got := governanceArgumentString(arguments, check.key); got != "" && got != check.want {
			return NewError(ErrorCodePolicyConflict, "integration action catalog changed; refresh the action guide and retry", nil)
		}
	}
	return nil
}

func metaConnectionSelected(parameters map[string]interface{}, integrationID, connectionID string) bool {
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	connectionID = strings.ToLower(strings.TrimSpace(connectionID))
	if integrationID == "" || connectionID == "" {
		return false
	}
	selectedRaw, fullSelectionProvided := parameters["integration_selected_connection_ids"]
	if matched, _ := governanceSelectionContains(selectedRaw, integrationID, connectionID); fullSelectionProvided {
		return matched
	}
	matched, _ := governanceSelectionContains(parameters["integration_connection_ids"], integrationID, connectionID)
	return matched
}

func governanceSelectionContains(raw interface{}, integrationID, connectionID string) (bool, bool) {
	matched := false
	configured := false
	switch values := raw.(type) {
	case map[string]string:
		for rawIntegrationID, candidate := range values {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			configured = true
			if strings.EqualFold(strings.TrimSpace(rawIntegrationID), integrationID) && strings.EqualFold(candidate, connectionID) {
				matched = true
			}
		}
	case map[string][]string:
		for rawIntegrationID, candidates := range values {
			for _, candidate := range candidates {
				candidate = strings.TrimSpace(candidate)
				if candidate == "" {
					continue
				}
				configured = true
				if strings.EqualFold(strings.TrimSpace(rawIntegrationID), integrationID) && strings.EqualFold(candidate, connectionID) {
					matched = true
				}
			}
		}
	case map[string]interface{}:
		for rawIntegrationID, candidates := range values {
			if governanceSelectedValueConfigured(candidates) {
				configured = true
			}
			if strings.EqualFold(strings.TrimSpace(rawIntegrationID), integrationID) && governanceSelectedValueContains(candidates, connectionID) {
				matched = true
			}
		}
	}
	return matched, configured
}

func governanceSelectedValueContains(value interface{}, connectionID string) bool {
	switch values := value.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(values), connectionID)
	case []string:
		for _, candidate := range values {
			if strings.EqualFold(strings.TrimSpace(candidate), connectionID) {
				return true
			}
		}
	case []interface{}:
		for _, candidate := range values {
			if raw, ok := candidate.(string); ok && strings.EqualFold(strings.TrimSpace(raw), connectionID) {
				return true
			}
		}
	}
	return false
}

func governanceSelectedValueConfigured(value interface{}) bool {
	switch values := value.(type) {
	case string:
		return strings.TrimSpace(values) != ""
	case []string:
		for _, candidate := range values {
			if strings.TrimSpace(candidate) != "" {
				return true
			}
		}
	case []interface{}:
		for _, candidate := range values {
			if raw, ok := candidate.(string); ok && strings.TrimSpace(raw) != "" {
				return true
			}
		}
	}
	return false
}

func nestedActionArguments(arguments map[string]interface{}) map[string]interface{} {
	if len(arguments) == 0 {
		return map[string]interface{}{}
	}
	if nested, ok := arguments["arguments"].(map[string]interface{}); ok && nested != nil {
		return nested
	}
	return map[string]interface{}{}
}

func manifestForIntegrationAction(base toolgovernance.Manifest, integrationID, connectionID string, action ActionDefinition) toolgovernance.Manifest {
	manifest := base
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	connectionID = strings.ToLower(strings.TrimSpace(connectionID))
	actionID := strings.ToLower(strings.TrimSpace(action.ID))

	// The facade provider id is intentionally shared by every Connected Apps
	// action, so it is not a sufficient SessionGrant boundary. Scope the
	// provider-owned action identity by integration and bind its approval to the
	// exact selected Connection through the standard AssetRef pipeline.
	// Integration/action identifiers cannot contain ':', making this identity
	// stable and unambiguous.
	boundIdentity := integrationID != "" && actionID != ""
	if parsed, err := uuid.Parse(connectionID); err != nil || parsed == uuid.Nil {
		boundIdentity = false
	}
	if boundIdentity {
		manifest.ToolID = integrationID + ":" + actionID
		manifest.RequiresAssetResolution = true
	} else {
		// A connector side effect must never receive a reusable approval when a
		// concrete provider/Connection identity cannot be represented safely.
		// Normal execute_action requests are rejected before reaching this path;
		// this remains a fail-closed guard for future callers and refactors.
		manifest.ToolID = actionID
		manifest.RequiresAssetResolution = false
		manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyAlwaysAsk
		manifest.ApprovalEveryInvocation = true
	}
	manifest.Domain = "external_integration"
	manifest.Effect = action.Effect
	manifest.RiskLevel = action.RiskLevel
	manifest.DataEgress = action.DataEgress
	manifest.ExternalDestination = action.ExternalDestination
	manifest.SensitiveDataAllowed = action.SensitiveDataAllowed
	manifest.ExternalSideEffect = action.Effect != toolgovernance.EffectRead && action.Effect != toolgovernance.EffectNone
	manifest.AssetType = "integration_connection"
	// Governance presents the complete scope contract. Runtime authorization
	// retains the any-of semantics through ActionScopeRequirement.
	manifest.PermissionScopes = ActionRequiredScopeIDs(action)
	if len(manifest.PermissionScopes) == 0 {
		manifest.PermissionScopes = []string{"integration:" + strings.TrimSpace(integrationID) + ":" + strings.TrimSpace(action.ID)}
	}
	manifest.AuditRequired = true
	manifest.IdempotencyRequired = manifest.ExternalSideEffect && !action.Idempotent
	if boundIdentity {
		manifest.ApprovalEveryInvocation = false
	}
	manifest.AllowedPermissionTiers = []toolgovernance.PermissionTier{
		toolgovernance.PermissionTierBasic,
		toolgovernance.PermissionTierAdvanced,
		toolgovernance.PermissionTierFull,
	}
	if action.DefaultPolicy != nil {
		if boundIdentity {
			manifest.DefaultApprovalPolicy = action.DefaultPolicy.ApprovalPolicy
			manifest.ApprovalEveryInvocation = action.DefaultPolicy.ApprovalPolicy == toolgovernance.ApprovalPolicyAlwaysAsk
		}
	}
	return toolgovernance.NormalizeManifest(manifest)
}
