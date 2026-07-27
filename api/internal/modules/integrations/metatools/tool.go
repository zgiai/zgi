package metatools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	maxActionGuideBytes         = 64 * 1024
	maxExecuteActionOutputBytes = 256 * 1024
	maxArgumentDisplayBytes     = 64 * 1024
	maxArgumentDisplayDepth     = 6
	maxArgumentDisplayFields    = 64
	hiddenReferenceSentinel     = "__zgi_hidden_reference__"
	resultCodeOutputTruncated   = "integration_result_truncated"
)

type Tool struct {
	name        string
	entity      tools.ToolEntity
	registry    *integrations.Registry
	executor    ActionExecutor
	connections ConnectionLookup
	access      ConnectionAuthorizer
	runtime     *tools.ToolRuntime
}

type selectedConnection struct {
	record *integrations.IntegrationConnection
	view   map[string]interface{}
}

type agentConnectionPreferenceAuthorizer interface {
	AuthorizeAgentConnectionPreference(context.Context, uuid.UUID, *uuid.UUID, uuid.UUID) error
	AuthorizeAgentConnectionActionPreference(
		context.Context,
		uuid.UUID,
		*uuid.UUID,
		uuid.UUID,
		string,
		string,
		toolgovernance.Effect,
	) error
}

func (t *Tool) GetEntity() tools.ToolEntity { return t.entity }

func (t *Tool) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeConnector }

func (t *Tool) GetTenantID() string {
	if t.runtime == nil {
		return ""
	}
	return strings.TrimSpace(t.runtime.TenantID)
}

func (t *Tool) Invoke(ctx context.Context, userID string, parameters map[string]interface{}, conversationID, appID, messageID *string) ([]tools.ToolInvokeMessage, error) {
	if t == nil || t.runtime == nil {
		return nil, fmt.Errorf("external integration meta tool runtime is not configured")
	}
	if t.runtime.InvokeFrom != tools.ToolInvokeFromAIChat && t.runtime.InvokeFrom != tools.ToolInvokeFromAgent {
		return nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "external integration meta tools are not available to this caller", nil)
	}
	if t.runtime.InvokeFrom == tools.ToolInvokeFromAgent {
		if runtimeString(t.runtime.RuntimeParameters, "agent_id") == "" ||
			skills.AgentBindingVerifierFromRuntimeParameters(t.runtime.RuntimeParameters) == nil {
			return nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "Agent integration binding authorization is unavailable", nil)
		}
	}
	if err := tools.ValidateJSONSchemaValue(t.entity.InputSchema, parameters); err != nil {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "meta tool arguments do not match the declared schema", err)
	}
	if err := validateTopLevelParameters(t.name, parameters); err != nil {
		return nil, err
	}
	var output map[string]interface{}
	var err error
	switch t.name {
	case ToolListConnections:
		output, err = t.listConnections(ctx, userID, parameters)
	case ToolSearchActions:
		output, err = t.searchActions(ctx, userID, parameters)
	case ToolGetActionGuide:
		output, err = t.getActionGuide(ctx, userID, parameters)
	case ToolExecuteAction:
		output, err = t.executeAction(ctx, userID, parameters, conversationID, appID, messageID)
	default:
		err = integrations.NewError(integrations.ErrorCodeInvalidInput, "unknown external integration meta tool", nil)
	}
	if err != nil {
		return nil, err
	}
	if err := tools.ValidateJSONSchemaValue(t.entity.OutputSchema, output); err != nil {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "meta tool result does not match the declared schema", err)
	}
	return []tools.ToolInvokeMessage{{Type: tools.ToolInvokeMessageTypeJSON, Data: output}}, nil
}

func (t *Tool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *Tool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	clone := *t
	clone.runtime = runtime
	return &clone
}

func (t *Tool) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }

func (t *Tool) EnrichGovernanceArguments(ctx context.Context, userID string, parameters map[string]interface{}) map[string]interface{} {
	out := cloneMap(parameters)
	if t == nil || t.name != ToolExecuteAction || t.registry == nil {
		return out
	}
	// Display metadata and connection labels are server-owned. Clear all
	// caller-supplied values before attempting resolution so an invalid action
	// cannot leave spoofed metadata in an approval payload.
	clearExecuteActionDisplayMetadata(out)
	integrationID := normalizedString(out["integration_id"])
	actionID := normalizedString(out["action_id"])
	definition, definitionOK := t.registry.ProviderDefinition(integrationID)
	action, ok := t.registry.ActionDetail(integrationID, actionID)
	if !definitionOK || !ok {
		return out
	}
	setExecuteActionDisplayMetadata(out, definition, action)
	setExecuteActionArgumentDisplayMetadata(out, action)
	if selected, selection, resolveErr := t.resolveExecutionConnection(ctx, userID, integrationID, action.ID, action.Effect, out); resolveErr == nil {
		// Freeze the canonical UUID for governance, approval and execution. The
		// public projection strips it and retains only the safe server labels.
		out["connection_id"] = selected.record.ID.String()
		delete(out, "connection_selector")
		out["connection_name"] = safeConnectionName(selected.record)
		out["connection_selection"] = selection
		if displayName := safeConnectionDisplayName(selected.record); displayName != "" {
			out["connection_display_name"] = displayName
		}
	}
	// Populate missing provider-owned revisions. Existing values are preserved
	// so a resumed frozen invocation fails closed if the catalog changed after
	// approval instead of silently upgrading to a different contract.
	setRevisionIfMissing(out, "action_schema_hash", action.SchemaHash)
	setRevisionIfMissing(out, "action_schema_revision", action.SchemaRevision)
	setRevisionIfMissing(out, "catalog_revision", action.CatalogRevision)
	return out
}

func (t *Tool) listConnections(ctx context.Context, userID string, parameters map[string]interface{}) (map[string]interface{}, error) {
	connections, err := t.availableConnections(ctx, userID)
	if err != nil {
		return nil, err
	}
	filter := normalizedString(parameters["integration_id"])
	items := make([]interface{}, 0, len(connections))
	for _, connection := range connections {
		if filter != "" && !strings.EqualFold(connection.record.IntegrationID, filter) {
			continue
		}
		items = append(items, connection.view)
		if len(items) >= maxSelectedConnections {
			break
		}
	}
	return map[string]interface{}{"connections": items, "count": len(items)}, nil
}

func (t *Tool) searchActions(ctx context.Context, userID string, parameters map[string]interface{}) (map[string]interface{}, error) {
	connections, err := t.availableConnections(ctx, userID)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(stringValue(parameters["query"])))
	filter := normalizedString(parameters["integration_id"])
	limit := integerValue(parameters["limit"], 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	byIntegration := groupConnectionsByIntegration(connections)
	integrationIDs := sortedKeys(byIntegration)
	items := make([]interface{}, 0, limit)
	for _, integrationID := range integrationIDs {
		if filter != "" && integrationID != filter {
			continue
		}
		actions := t.registry.SearchActionSummaries(integrations.ActionSearchRequest{
			Query: query, IntegrationID: integrationID, Caller: t.runtime.InvokeFrom, Limit: limit,
		})
		for _, action := range actions {
			preferred, resolveErr := t.resolvePreferredFromAvailable(ctx, userID, integrationID, action.ID, action.Effect, byIntegration[integrationID])
			if resolveErr != nil {
				continue
			}
			items = append(items, actionSummaryOutput(action, preferred.record))
			if len(items) >= limit {
				return map[string]interface{}{"actions": items, "count": len(items)}, nil
			}
		}
	}
	return map[string]interface{}{"actions": items, "count": len(items)}, nil
}

func (t *Tool) getActionGuide(ctx context.Context, userID string, parameters map[string]interface{}) (map[string]interface{}, error) {
	integrationID := normalizedString(parameters["integration_id"])
	actionID := normalizedString(parameters["action_id"])
	action, ok := t.registry.ActionDetail(integrationID, actionID)
	if !ok {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "unknown integration action", nil)
	}
	if !supportsCaller(action, t.runtime.InvokeFrom) {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration action is not available to this caller", nil)
	}
	if err := validateActionRevisions(parameters, action); err != nil {
		return nil, err
	}
	connections, err := t.availableConnections(ctx, userID)
	if err != nil {
		return nil, err
	}
	preferred, err := t.resolvePreferredFromAvailable(ctx, userID, integrationID, action.ID, action.Effect, groupConnectionsByIntegration(connections)[integrationID])
	if err != nil {
		return nil, err
	}
	definition, _ := t.registry.ProviderDefinition(integrationID)
	output := actionSummaryOutput(actionSummary(definition, action), preferred.record)
	output["input_schema"] = cloneMap(action.InputSchema)
	output["output_schema"] = cloneMap(action.OutputSchema)
	output["schema_revision"] = action.SchemaRevision
	encoded, marshalErr := json.Marshal(output)
	if marshalErr != nil || len(encoded) > maxActionGuideBytes {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "integration action guide exceeds the safe response limit", marshalErr)
	}
	return output, nil
}

func (t *Tool) executeAction(ctx context.Context, userID string, parameters map[string]interface{}, conversationID, appID, messageID *string) (map[string]interface{}, error) {
	integrationID := normalizedString(parameters["integration_id"])
	actionID := normalizedString(parameters["action_id"])
	definition, definitionOK := t.registry.ProviderDefinition(integrationID)
	action, ok := t.registry.ActionDetail(integrationID, actionID)
	if !definitionOK || !ok {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "unknown integration action", nil)
	}
	if !supportsCaller(action, t.runtime.InvokeFrom) {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration action is not available to this caller", nil)
	}
	if err := validateActionRevisions(parameters, action); err != nil {
		return nil, err
	}
	selected, selection, err := t.resolveExecutionConnection(ctx, userID, integrationID, action.ID, action.Effect, parameters)
	if err != nil {
		return nil, err
	}
	organizationID, accountID, workspaceID, err := t.authorizationContext(userID)
	if err != nil {
		return nil, err
	}
	actionArguments, ok := parameters["arguments"].(map[string]interface{})
	if !ok || actionArguments == nil {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action arguments must be an object", nil)
	}
	request := integrations.ActionRequest{
		OrganizationID: organizationID.String(), WorkspaceID: optionalUUIDString(workspaceID), UserID: accountID.String(),
		AgentID:        runtimeString(t.runtime.RuntimeParameters, "agent_id"),
		ConversationID: optionalString(conversationID), AppID: optionalString(appID), MessageID: optionalString(messageID),
		ConnectionID: selected.record.ID.String(), InvokeFrom: t.runtime.InvokeFrom,
		IntegrationID: integrationID, ActionID: action.ID, Input: cloneMap(actionArguments),
	}
	if t.runtime.InvokeFrom == tools.ToolInvokeFromAgent {
		authorization, authorized := t.agentBindingAuthorization(selected.record, action)
		verifier := skills.AgentBindingVerifierFromRuntimeParameters(t.runtime.RuntimeParameters)
		if !authorized || verifier == nil {
			return nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "Agent integration binding authorization is unavailable", nil)
		}
		request.VerifyAgentConnection = func(ctx context.Context, check integrations.AgentConnectionAuthorizationRequest) (bool, error) {
			return verifier(ctx, skills.AgentBindingCheck{
				BindingType:      "integration_connection",
				ResourceID:       check.ConnectionID,
				ParentResourceID: check.IntegrationID,
				AccessMode:       authorization.AccessMode,
				ActionID:         check.ActionID,
			})
		}
	}
	result, err := t.executor.Execute(ctx, request)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Output == nil {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "integration returned an invalid response", nil)
	}
	safeResult, ok := redactInternalConnectionID(result.Output, selected.record.ID.String()).(map[string]interface{})
	if !ok || safeResult == nil {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "integration returned an invalid response", nil)
	}
	output := map[string]interface{}{
		"integration_id": integrationID, "action_id": action.ID,
		"connection_name": safeConnectionName(selected.record), "connection_selection": selection,
		"action_schema_hash": action.SchemaHash, "schema_revision": action.SchemaRevision, "catalog_revision": action.CatalogRevision,
		"result_count": max(result.ResultCount, 0), "attempt_count": max(result.AttemptCount, 0), "result": safeResult,
	}
	setExecuteActionDisplayMetadata(output, definition, action)
	if displayName := safeConnectionDisplayName(selected.record); displayName != "" {
		output["connection_display_name"] = displayName
	}
	if requestID := strings.TrimSpace(result.ProviderRequestID); requestID != "" {
		output["provider_request_id"] = boundedString(requestID, 512)
	}
	if result.CostUSD != nil && !math.IsNaN(*result.CostUSD) && !math.IsInf(*result.CostUSD, 0) && *result.CostUSD >= 0 {
		output["cost_usd"] = *result.CostUSD
	}
	output, ok = redactInternalConnectionID(output, selected.record.ID.String()).(map[string]interface{})
	if !ok || output == nil {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "integration returned an invalid response", nil)
	}
	encoded, marshalErr := json.Marshal(output)
	if marshalErr != nil || len(encoded) > maxExecuteActionOutputBytes {
		// The external operation has already completed successfully. Return a
		// bounded success marker instead of an error that could encourage an
		// unsafe duplicate retry of a side-effecting action.
		output["result"] = map[string]interface{}{
			"status": "completed", "content_truncated": true, "result_code": resultCodeOutputTruncated,
		}
		output["result_truncated"] = true
	}
	return output, nil
}

func (t *Tool) availableConnections(ctx context.Context, userID string) ([]selectedConnection, error) {
	organizationID, accountID, workspaceID, err := t.authorizationContext(userID)
	if err != nil {
		return nil, err
	}
	references := selectedConnectionReferences(t.runtime.RuntimeParameters)
	if len(references) == 0 {
		return nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "no integration connections are selected for this chat", nil)
	}
	out := make([]selectedConnection, 0, len(references))
	seen := make(map[uuid.UUID]struct{}, len(references))
	for _, reference := range references {
		connectionID, parseErr := uuid.Parse(reference.connectionID)
		if parseErr != nil || connectionID == uuid.Nil {
			continue
		}
		if _, exists := seen[connectionID]; exists {
			continue
		}
		connection, lookupErr := t.connections.GetByID(ctx, organizationID, connectionID)
		if lookupErr != nil || connection == nil || connection.Status != integrations.ConnectionStatusActive {
			continue
		}
		if reference.integrationID != "" && !strings.EqualFold(connection.IntegrationID, reference.integrationID) {
			continue
		}
		if !t.registry.Configured(connection.IntegrationID) {
			continue
		}
		if t.runtime.InvokeFrom == tools.ToolInvokeFromAgent {
			if connection.CredentialSource != integrations.ConnectionCredentialSourceOrganization {
				continue
			}
			agentAccess, ok := t.access.(agentConnectionPreferenceAuthorizer)
			if !ok || agentAccess.AuthorizeAgentConnectionPreference(ctx, organizationID, workspaceID, connectionID) != nil {
				continue
			}
			if !t.agentConnectionHasReadableAction(ctx, connection, organizationID, workspaceID, agentAccess) {
				continue
			}
		} else {
			if accessErr := t.access.AuthorizeConnectionPreference(ctx, organizationID, accountID, workspaceID, connectionID); accessErr != nil {
				continue
			}
		}
		selection := "selected"
		if preferredID, preferredErr := preferredRuntimeConnectionID(t.runtime.RuntimeParameters, connection.IntegrationID); preferredErr == nil &&
			preferredID == connection.ID && runtimeSelectionContains(t.runtime.RuntimeParameters, connection.IntegrationID, connection.ID) {
			selection = preferredSelector
		}
		seen[connectionID] = struct{}{}
		out = append(out, selectedConnection{record: connection, view: safeConnectionView(connection, selection)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].record.IntegrationID == out[j].record.IntegrationID {
			return out[i].record.ID.String() < out[j].record.ID.String()
		}
		return out[i].record.IntegrationID < out[j].record.IntegrationID
	})
	if len(out) == 0 {
		return nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "selected integration connections are not available", nil)
	}
	return out, nil
}

func (t *Tool) resolveExecutionConnection(
	ctx context.Context,
	userID string,
	integrationID string,
	actionID string,
	effect toolgovernance.Effect,
	parameters map[string]interface{},
) (selectedConnection, string, error) {
	connectionIDRaw, connectionIDProvided := parameters["connection_id"]
	connectionID := strings.TrimSpace(stringValue(connectionIDRaw))
	selectorRaw, selectorProvided := parameters["connection_selector"]
	selector := normalizedString(selectorRaw)
	if connectionIDProvided && selectorProvided {
		return selectedConnection{}, "", integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action accepts either connection_id or connection_selector, not both", nil)
	}
	if !connectionIDProvided {
		if selectorProvided && selector != preferredSelector {
			return selectedConnection{}, "", integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action connection_selector is invalid", nil)
		}
		preferred, err := t.resolvePreferredConnection(ctx, userID, integrationID, actionID, effect)
		return preferred, preferredSelector, err
	}
	if connectionID == "" {
		return selectedConnection{}, "", integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action connection_id is invalid", nil)
	}
	parsedConnectionID, err := uuid.Parse(connectionID)
	if err != nil || parsedConnectionID == uuid.Nil {
		return selectedConnection{}, "", integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action connection_id is invalid", err)
	}
	connections, err := t.availableConnections(ctx, userID)
	if err != nil {
		return selectedConnection{}, "", err
	}
	selected, err := t.resolveExplicitFromAvailable(ctx, userID, integrationID, actionID, effect, parsedConnectionID, connections)
	if err != nil {
		return selectedConnection{}, "", err
	}
	selection := "explicit"
	if preferredID, preferredErr := preferredRuntimeConnectionID(t.runtime.RuntimeParameters, integrationID); preferredErr == nil &&
		preferredID == parsedConnectionID && runtimeSelectionContains(t.runtime.RuntimeParameters, integrationID, parsedConnectionID) {
		selection = preferredSelector
	}
	return selected, selection, nil
}

func (t *Tool) resolvePreferredConnection(
	ctx context.Context,
	userID string,
	integrationID string,
	actionID string,
	effect toolgovernance.Effect,
) (selectedConnection, error) {
	preferredID, err := preferredRuntimeConnectionID(t.runtime.RuntimeParameters, integrationID)
	if err != nil {
		return selectedConnection{}, err
	}
	if !runtimeSelectionContains(t.runtime.RuntimeParameters, integrationID, preferredID) {
		return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "preferred connection is not selected for this chat", nil)
	}
	connections, err := t.availableConnections(ctx, userID)
	if err != nil {
		return selectedConnection{}, err
	}
	return t.resolvePreferredFromAvailable(ctx, userID, integrationID, actionID, effect, connections)
}

func (t *Tool) resolvePreferredFromAvailable(
	ctx context.Context,
	userID string,
	integrationID string,
	actionID string,
	effect toolgovernance.Effect,
	connections []selectedConnection,
) (selectedConnection, error) {
	preferredID, err := preferredRuntimeConnectionID(t.runtime.RuntimeParameters, integrationID)
	if err != nil {
		return selectedConnection{}, err
	}
	if !runtimeSelectionContains(t.runtime.RuntimeParameters, integrationID, preferredID) {
		return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "preferred connection is not selected for this chat", nil)
	}
	return t.resolveExplicitFromAvailable(ctx, userID, integrationID, actionID, effect, preferredID, connections)
}

func (t *Tool) resolveExplicitFromAvailable(
	ctx context.Context,
	userID string,
	integrationID string,
	actionID string,
	effect toolgovernance.Effect,
	connectionID uuid.UUID,
	connections []selectedConnection,
) (selectedConnection, error) {
	var selected selectedConnection
	for _, connection := range connections {
		if connection.record.ID == connectionID && strings.EqualFold(connection.record.IntegrationID, integrationID) {
			selected = connection
			break
		}
	}
	if selected.record == nil {
		return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "connection is not selected or available for this chat", nil)
	}
	action, exists := t.registry.ActionDetail(integrationID, actionID)
	if !exists || !integrations.ActionSupportsAuthMethod(action, selected.record.AuthMethodID) {
		return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "integration action is not available for this connection authentication method", nil)
	}
	if t.runtime.InvokeFrom == tools.ToolInvokeFromAgent && !agentActionExecutableWithoutInteraction(action) {
		return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "integration action requires interactive approval or write access and is unavailable to Agents", nil)
	}
	organizationID, accountID, workspaceID, err := t.authorizationContext(userID)
	if err != nil {
		return selectedConnection{}, err
	}
	if t.runtime.InvokeFrom == tools.ToolInvokeFromAgent {
		authorization, authorized := t.agentBindingAuthorization(selected.record, action)
		verifier := skills.AgentBindingVerifierFromRuntimeParameters(t.runtime.RuntimeParameters)
		agentAccess, accessOK := t.access.(agentConnectionPreferenceAuthorizer)
		if !authorized || verifier == nil || !accessOK {
			return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "Agent integration binding does not authorize this action", nil)
		}
		matched, verifyErr := verifier(ctx, skills.AgentBindingCheck{
			BindingType:      "integration_connection",
			ResourceID:       selected.record.ID.String(),
			ParentResourceID: integrationID,
			AccessMode:       authorization.AccessMode,
			ActionID:         action.ID,
		})
		if verifyErr != nil || !matched {
			return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "Agent integration binding does not authorize this action", verifyErr)
		}
		if accessErr := agentAccess.AuthorizeAgentConnectionActionPreference(
			ctx,
			organizationID,
			workspaceID,
			selected.record.ID,
			integrationID,
			action.ID,
			action.Effect,
		); accessErr != nil {
			return selectedConnection{}, integrations.NewError(integrations.ErrorCodeAccessDenied, "Agent shared connection grant does not authorize this action", accessErr)
		}
	} else if err := t.access.AuthorizeConnectionUse(ctx, integrations.ConnectionAccessRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, AccountID: accountID,
		ConnectionID: selected.record.ID, IntegrationID: integrationID,
		ActionID: actionID, Effect: effect,
	}); err != nil {
		return selectedConnection{}, err
	}
	return selected, nil
}

func (t *Tool) agentConnectionHasReadableAction(
	ctx context.Context,
	connection *integrations.IntegrationConnection,
	organizationID uuid.UUID,
	workspaceID *uuid.UUID,
	access agentConnectionPreferenceAuthorizer,
) bool {
	if t == nil || t.runtime == nil || connection == nil || access == nil {
		return false
	}
	verifier := skills.AgentBindingVerifierFromRuntimeParameters(t.runtime.RuntimeParameters)
	if verifier == nil {
		return false
	}
	for _, action := range t.registry.Actions(connection.IntegrationID) {
		if !agentActionExecutableWithoutInteraction(action) ||
			!integrations.ActionSupportsAuthMethod(action, connection.AuthMethodID) {
			continue
		}
		authorization, ok := t.agentBindingAuthorization(connection, action)
		if !ok {
			continue
		}
		matched, err := verifier(ctx, skills.AgentBindingCheck{
			BindingType:      "integration_connection",
			ResourceID:       connection.ID.String(),
			ParentResourceID: connection.IntegrationID,
			AccessMode:       authorization.AccessMode,
			ActionID:         action.ID,
		})
		if err != nil || !matched {
			continue
		}
		if err := access.AuthorizeAgentConnectionActionPreference(
			ctx,
			organizationID,
			workspaceID,
			connection.ID,
			connection.IntegrationID,
			action.ID,
			action.Effect,
		); err == nil {
			return true
		}
	}
	return false
}

func (t *Tool) agentBindingAuthorization(
	connection *integrations.IntegrationConnection,
	action integrations.ActionDefinition,
) (tools.AgentBindingAuthorization, bool) {
	if t == nil || t.runtime == nil || connection == nil || !agentActionExecutableWithoutInteraction(action) {
		return tools.AgentBindingAuthorization{}, false
	}
	authorization, ok := tools.AgentBindingAuthorizationFor(
		t.runtime.RuntimeParameters,
		"integration_connection",
		strings.ToLower(strings.TrimSpace(connection.IntegrationID)),
		strings.ToLower(strings.TrimSpace(connection.ID.String())),
		"read",
	)
	if !ok || !authorization.AllowsAction(action.ID) {
		return tools.AgentBindingAuthorization{}, false
	}
	return authorization, true
}

func agentActionExecutableWithoutInteraction(action integrations.ActionDefinition) bool {
	if action.Effect != toolgovernance.EffectRead || !supportsCaller(action, tools.ToolInvokeFromAgent) {
		return false
	}
	if action.DefaultPolicy == nil {
		return false
	}
	return action.DefaultPolicy.Enabled &&
		toolgovernance.NormalizeApprovalPolicy(action.DefaultPolicy.ApprovalPolicy) != toolgovernance.ApprovalPolicyAlwaysAsk
}

func (t *Tool) authorizationContext(userID string) (uuid.UUID, uuid.UUID, *uuid.UUID, error) {
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(t.runtime.TenantID))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(userID))
	if organizationErr != nil || accountErr != nil || organizationID == uuid.Nil || accountID == uuid.Nil {
		return uuid.Nil, uuid.Nil, nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration invocation identity is invalid", nil)
	}
	var workspaceID *uuid.UUID
	if raw := runtimeString(t.runtime.RuntimeParameters, "workspace_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil || parsed == uuid.Nil {
			return uuid.Nil, uuid.Nil, nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration workspace identity is invalid", err)
		}
		workspaceID = &parsed
	}
	return organizationID, accountID, workspaceID, nil
}

type selectedReference struct {
	integrationID string
	connectionID  string
}

func preferredRuntimeConnectionID(parameters map[string]interface{}, integrationID string) (uuid.UUID, error) {
	integrationID = normalizedString(integrationID)
	if integrationID == "" || len(parameters) == 0 {
		return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "no preferred connection is configured for this integration", nil)
	}
	raw, exists := parameters["integration_connection_ids"]
	if !exists {
		return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "no preferred connection is configured for this integration", nil)
	}
	candidates := make([]string, 0, 1)
	switch values := raw.(type) {
	case map[string]string:
		for candidateIntegrationID, value := range values {
			if strings.EqualFold(strings.TrimSpace(candidateIntegrationID), integrationID) && strings.TrimSpace(value) != "" {
				candidates = append(candidates, strings.TrimSpace(value))
			}
		}
	case map[string]interface{}:
		for candidateIntegrationID, rawValue := range values {
			if !strings.EqualFold(strings.TrimSpace(candidateIntegrationID), integrationID) {
				continue
			}
			value, ok := rawValue.(string)
			if !ok {
				return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "preferred connection metadata is invalid", nil)
			}
			if strings.TrimSpace(value) != "" {
				candidates = append(candidates, strings.TrimSpace(value))
			}
		}
	default:
		return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "preferred connection metadata is invalid", nil)
	}
	if len(candidates) == 0 {
		return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "no preferred connection is configured for this integration", nil)
	}
	if len(candidates) != 1 {
		return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "preferred connection is ambiguous for this integration", nil)
	}
	connectionID, err := uuid.Parse(candidates[0])
	if err != nil || connectionID == uuid.Nil {
		return uuid.Nil, integrations.NewError(integrations.ErrorCodeAccessDenied, "preferred connection identity is invalid", err)
	}
	return connectionID, nil
}

func runtimeSelectionContains(parameters map[string]interface{}, integrationID string, connectionID uuid.UUID) bool {
	if connectionID == uuid.Nil || len(parameters) == 0 {
		return false
	}
	raw, exists := parameters["integration_selected_connection_ids"]
	if !exists {
		return false
	}
	integrationID = normalizedString(integrationID)
	connectionIDString := connectionID.String()
	switch values := raw.(type) {
	case map[string]string:
		for candidateIntegrationID, candidate := range values {
			if strings.EqualFold(strings.TrimSpace(candidateIntegrationID), integrationID) && strings.EqualFold(strings.TrimSpace(candidate), connectionIDString) {
				return true
			}
		}
	case map[string][]string:
		for candidateIntegrationID, candidates := range values {
			if !strings.EqualFold(strings.TrimSpace(candidateIntegrationID), integrationID) {
				continue
			}
			for _, candidate := range candidates {
				if strings.EqualFold(strings.TrimSpace(candidate), connectionIDString) {
					return true
				}
			}
		}
	case map[string]interface{}:
		for candidateIntegrationID, candidates := range values {
			if !strings.EqualFold(strings.TrimSpace(candidateIntegrationID), integrationID) {
				continue
			}
			for _, candidate := range stringSlice(candidates) {
				if strings.EqualFold(strings.TrimSpace(candidate), connectionIDString) {
					return true
				}
			}
		}
	}
	return false
}

func selectedConnectionReferences(parameters map[string]interface{}) []selectedReference {
	result := make([]selectedReference, 0, 8)
	appendMap := func(raw interface{}) {
		switch values := raw.(type) {
		case map[string]string:
			for integrationID, connectionID := range values {
				result = append(result, selectedReference{integrationID: normalizedString(integrationID), connectionID: strings.TrimSpace(connectionID)})
			}
		case map[string][]string:
			for integrationID, connectionIDs := range values {
				for _, connectionID := range connectionIDs {
					result = append(result, selectedReference{integrationID: normalizedString(integrationID), connectionID: strings.TrimSpace(connectionID)})
				}
			}
		case map[string]interface{}:
			for integrationID, rawConnections := range values {
				for _, connectionID := range stringSlice(rawConnections) {
					result = append(result, selectedReference{integrationID: normalizedString(integrationID), connectionID: connectionID})
				}
			}
		default:
			for _, connectionID := range stringSlice(raw) {
				result = append(result, selectedReference{connectionID: connectionID})
			}
		}
	}
	selectedRaw, fullSelectionProvided := parameters["integration_selected_connection_ids"]
	appendMap(selectedRaw)
	// The complete selection set is authoritative. The preferred-connection
	// map is accepted only as a compatibility fallback when the server did not
	// inject the full-selection key at all. An explicitly empty full set means
	// the user currently selected nothing and must not revive a stale preferred
	// connection.
	if !fullSelectionProvided {
		appendMap(parameters["integration_connection_ids"])
	}
	seen := make(map[string]struct{}, len(result))
	normalized := make([]selectedReference, 0, len(result))
	for _, reference := range result {
		reference.integrationID = normalizedString(reference.integrationID)
		reference.connectionID = strings.ToLower(strings.TrimSpace(reference.connectionID))
		if reference.connectionID == "" {
			continue
		}
		key := reference.integrationID + "/" + reference.connectionID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, reference)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].integrationID == normalized[j].integrationID {
			return normalized[i].connectionID < normalized[j].connectionID
		}
		return normalized[i].integrationID < normalized[j].integrationID
	})
	if len(normalized) > maxRuntimeSelections {
		normalized = normalized[:maxRuntimeSelections]
	}
	return normalized
}

func stringSlice(value interface{}) []string {
	switch values := value.(type) {
	case string:
		if value := strings.TrimSpace(values); value != "" {
			return []string{value}
		}
	case []string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, raw := range values {
			if value := strings.TrimSpace(stringValue(raw)); value != "" {
				out = append(out, value)
			}
		}
		return out
	}
	return nil
}

func safeConnectionView(connection *integrations.IntegrationConnection, selection string) map[string]interface{} {
	view := map[string]interface{}{
		"integration_id": strings.ToLower(strings.TrimSpace(connection.IntegrationID)),
		"driver_id":      strings.ToLower(strings.TrimSpace(connection.DriverID)), "name": safeConnectionName(connection),
		"selection": selection,
		"status":    string(connection.Status), "health_status": string(connection.HealthStatus),
		"auth_status": string(connection.AuthStatus), "scope_status": string(connection.ScopeStatus),
	}
	if displayName := safeConnectionDisplayName(connection); displayName != "" {
		view["display_name"] = displayName
	}
	if connection.AttentionCode != nil && strings.TrimSpace(*connection.AttentionCode) != "" {
		view["attention_code"] = boundedString(*connection.AttentionCode, 64)
	}
	return view
}

func safeConnectionName(connection *integrations.IntegrationConnection) string {
	if connection == nil {
		return hiddenReferenceSentinel
	}
	name := boundedString(connection.Name, 128)
	if name == "" || strings.Contains(strings.ToLower(name), strings.ToLower(connection.ID.String())) {
		integrationID := boundedString(strings.ToLower(strings.TrimSpace(connection.IntegrationID)), 96)
		if integrationID != "" {
			return integrationID
		}
		return hiddenReferenceSentinel
	}
	return name
}

func safeConnectionDisplayName(connection *integrations.IntegrationConnection) string {
	if connection == nil || connection.DisplayName == nil {
		return ""
	}
	displayName := boundedString(*connection.DisplayName, 255)
	if displayName == "" || strings.Contains(strings.ToLower(displayName), strings.ToLower(connection.ID.String())) {
		return ""
	}
	return displayName
}

func groupConnectionsByIntegration(connections []selectedConnection) map[string][]selectedConnection {
	result := make(map[string][]selectedConnection)
	for _, connection := range connections {
		integrationID := normalizedString(connection.record.IntegrationID)
		result[integrationID] = append(result[integrationID], connection)
	}
	return result
}

func actionSummary(definition integrations.ProviderDefinition, action integrations.ActionDefinition) integrations.ActionSummary {
	policy := integrations.DefaultActionPolicy{}
	if action.DefaultPolicy != nil {
		policy = *action.DefaultPolicy
	}
	return integrations.ActionSummary{
		IntegrationID: definition.ID, DriverID: definition.DriverID, ID: action.ID, ToolName: action.ToolName,
		Name: action.Name, NameI18n: cloneLocalizedText(action.NameI18n),
		Description: action.Description, DescriptionI18n: cloneLocalizedText(action.DescriptionI18n), Effect: action.Effect, RiskLevel: action.RiskLevel,
		DataEgress: action.DataEgress, ExternalDestination: action.ExternalDestination,
		RequiredScopes:         append([]string(nil), action.RequiredScopes...),
		SupportedAuthMethodIDs: append([]string(nil), action.SupportedAuthMethodIDs...),
		ScopeLabelsI18n:        cloneLocalizedLabelMap(action.ScopeLabelsI18n), DefaultPolicy: policy,
		SchemaHash: action.SchemaHash, SchemaRevision: action.SchemaRevision, CatalogRevision: action.CatalogRevision,
		SupportedCallers: append([]tools.ToolInvokeFrom(nil), action.SupportedCallers...),
	}
}

func supportsCaller(action integrations.ActionDefinition, caller tools.ToolInvokeFrom) bool {
	if len(action.SupportedCallers) == 0 {
		return true
	}
	for _, allowed := range action.SupportedCallers {
		if allowed == caller {
			return true
		}
	}
	return false
}

func setRevisionIfMissing(parameters map[string]interface{}, key, value string) {
	if strings.TrimSpace(stringValue(parameters[key])) == "" {
		parameters[key] = value
	}
}

func clearExecuteActionDisplayMetadata(parameters map[string]interface{}) {
	for _, key := range []string{
		"integration_name",
		"integration_name_i18n",
		"action_name",
		"action_name_i18n",
		"argument_labels_i18n",
		"argument_value_labels_i18n",
		"connection_name",
		"connection_display_name",
		"connection_selection",
	} {
		delete(parameters, key)
	}
}

func setExecuteActionDisplayMetadata(output map[string]interface{}, definition integrations.ProviderDefinition, action integrations.ActionDefinition) {
	output["integration_name"] = boundedString(definition.Name, 128)
	output["action_name"] = boundedString(action.Name, 128)
	if localized := localizedTextOutput(definition.NameI18n, 128); len(localized) > 0 {
		output["integration_name_i18n"] = localized
	} else {
		delete(output, "integration_name_i18n")
	}
	if localized := localizedTextOutput(action.NameI18n, 128); len(localized) > 0 {
		output["action_name_i18n"] = localized
	} else {
		delete(output, "action_name_i18n")
	}
}

func setExecuteActionArgumentDisplayMetadata(output map[string]interface{}, action integrations.ActionDefinition) {
	argumentLabels, argumentValueLabels := actionArgumentDisplayMetadata(action.InputSchema)
	if len(argumentLabels) > 0 {
		output["argument_labels_i18n"] = argumentLabels
	} else {
		delete(output, "argument_labels_i18n")
	}
	if len(argumentValueLabels) > 0 {
		output["argument_value_labels_i18n"] = argumentValueLabels
	} else {
		delete(output, "argument_value_labels_i18n")
	}
}

func validateActionRevisions(parameters map[string]interface{}, action integrations.ActionDefinition) error {
	checks := []struct {
		key  string
		want string
	}{
		{key: "action_schema_hash", want: action.SchemaHash},
		{key: "action_schema_revision", want: action.SchemaRevision},
		{key: "catalog_revision", want: action.CatalogRevision},
	}
	for _, check := range checks {
		if got := strings.TrimSpace(stringValue(parameters[check.key])); got != "" && got != check.want {
			return integrations.NewError(integrations.ErrorCodePolicyConflict, "integration action catalog changed; refresh the action guide and retry", nil)
		}
	}
	return nil
}

func actionSummaryOutput(action integrations.ActionSummary, connection *integrations.IntegrationConnection) map[string]interface{} {
	output := map[string]interface{}{
		"integration_id": action.IntegrationID, "action_id": action.ID, "name": boundedString(action.Name, 128),
		"description": boundedString(action.Description, 1200), "effect": string(action.Effect), "risk_level": string(action.RiskLevel),
		"data_egress": action.DataEgress, "required_scopes": stringInterfaces(action.RequiredScopes),
		"schema_hash": action.SchemaHash, "catalog_revision": action.CatalogRevision,
		"connection_name": safeConnectionName(connection), "connection_selection": preferredSelector,
	}
	if displayName := safeConnectionDisplayName(connection); displayName != "" {
		output["connection_display_name"] = displayName
	}
	if localized := localizedTextOutput(action.NameI18n, 128); len(localized) > 0 {
		output["name_i18n"] = localized
	}
	if localized := localizedTextOutput(action.DescriptionI18n, 1200); len(localized) > 0 {
		output["description_i18n"] = localized
	}
	if localized := localizedLabelMapOutput(action.ScopeLabelsI18n, 128, 128); len(localized) > 0 {
		output["scope_labels_i18n"] = localized
	}
	if destination := strings.TrimSpace(action.ExternalDestination); destination != "" {
		output["external_destination"] = boundedString(destination, 255)
	}
	return output
}

func cloneLocalizedText(values integrations.LocalizedText) integrations.LocalizedText {
	if len(values) == 0 {
		return nil
	}
	out := make(integrations.LocalizedText, len(values))
	for locale, value := range values {
		out[locale] = value
	}
	return out
}

func cloneLocalizedLabelMap(values integrations.LocalizedLabelMap) integrations.LocalizedLabelMap {
	if len(values) == 0 {
		return nil
	}
	out := make(integrations.LocalizedLabelMap, len(values))
	for key, labels := range values {
		out[key] = cloneLocalizedText(labels)
	}
	return out
}

func localizedLabelMapOutput(values integrations.LocalizedLabelMap, maxLabels int, maxRunes int) map[string]interface{} {
	if len(values) == 0 || maxLabels <= 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]interface{}, min(len(keys), maxLabels))
	for _, key := range keys {
		if len(out) >= maxLabels {
			break
		}
		normalizedKey := boundedString(strings.ToLower(strings.TrimSpace(key)), 128)
		if normalizedKey == "" {
			continue
		}
		if localized := localizedTextOutput(values[key], maxRunes); len(localized) > 0 {
			out[normalizedKey] = localized
		}
	}
	return out
}

func localizedTextOutput(values integrations.LocalizedText, maxRunes int) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	locales := sortedKeys(values)
	orderedLocales := make([]string, 0, len(locales))
	for _, preferredLocale := range []string{integrations.LocaleEnglishUS, integrations.LocaleSimplifiedChinese} {
		if _, exists := values[preferredLocale]; exists {
			orderedLocales = append(orderedLocales, preferredLocale)
		}
	}
	for _, locale := range locales {
		if locale != integrations.LocaleEnglishUS && locale != integrations.LocaleSimplifiedChinese {
			orderedLocales = append(orderedLocales, locale)
		}
	}
	out := make(map[string]interface{}, min(len(values), 16))
	for _, locale := range orderedLocales {
		if len(out) >= 16 {
			break
		}
		value := values[locale]
		locale = boundedString(locale, 35)
		value = boundedString(value, maxRunes)
		if locale != "" && value != "" {
			out[locale] = value
		}
	}
	return out
}

func actionArgumentDisplayMetadata(inputSchema map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	if len(inputSchema) == 0 {
		return nil, nil
	}
	collector := argumentDisplayMetadataCollector{
		argumentLabels:      make(map[string]interface{}),
		argumentValueLabels: make(map[string]interface{}),
	}
	collector.collect(inputSchema, nil, 0)
	argumentLabels := collector.argumentLabels
	argumentValueLabels := collector.argumentValueLabels
	if len(argumentLabels) == 0 {
		argumentLabels = nil
	}
	if len(argumentValueLabels) == 0 {
		argumentValueLabels = nil
	}
	encoded, err := json.Marshal(map[string]interface{}{
		"argument_labels_i18n":       argumentLabels,
		"argument_value_labels_i18n": argumentValueLabels,
	})
	if err != nil || len(encoded) > maxArgumentDisplayBytes {
		return nil, nil
	}
	return argumentLabels, argumentValueLabels
}

type argumentDisplayMetadataCollector struct {
	argumentLabels      map[string]interface{}
	argumentValueLabels map[string]interface{}
	fieldCount          int
}

func (collector *argumentDisplayMetadataCollector) collect(schema map[string]interface{}, parentPath []string, depth int) {
	if collector == nil || depth >= maxArgumentDisplayDepth || collector.fieldCount >= maxArgumentDisplayFields {
		return
	}
	properties := stringKeyedObject(schema["properties"])
	for _, argumentName := range sortedKeys(properties) {
		if collector.fieldCount >= maxArgumentDisplayFields {
			return
		}
		if strings.TrimSpace(argumentName) == "" || len([]rune(argumentName)) > 128 {
			continue
		}
		propertySchema := stringKeyedObject(properties[argumentName])
		if len(propertySchema) == 0 {
			continue
		}
		fieldPath := append(append([]string(nil), parentPath...), argumentName)
		pathKey := strings.Join(fieldPath, ".")
		if len([]rune(pathKey)) > 512 {
			continue
		}
		collector.fieldCount++
		if labels := localizedTextAnnotation(propertySchema["title_i18n"], 128); len(labels) > 0 {
			collector.argumentLabels[pathKey] = labels
		}
		allowedValues := declaredStringEnumValues(propertySchema["enum"])
		if labels := enumValueLabelsAnnotation(propertySchema["enum_labels_i18n"], allowedValues); len(labels) > 0 {
			collector.argumentValueLabels[pathKey] = labels
		}
		collector.collect(propertySchema, fieldPath, depth+1)
	}
	if collector.fieldCount >= maxArgumentDisplayFields {
		return
	}
	if itemSchema := stringKeyedObject(schema["items"]); len(itemSchema) > 0 {
		collector.collect(itemSchema, parentPath, depth+1)
	}
}

func localizedTextAnnotation(value interface{}, maxRunes int) map[string]interface{} {
	entries := stringKeyedObject(value)
	if len(entries) == 0 {
		return nil
	}
	localized := make(integrations.LocalizedText, len(entries))
	for _, rawLocale := range sortedKeys(entries) {
		locale := strings.TrimSpace(rawLocale)
		if locale != rawLocale || len([]rune(locale)) < 2 || len([]rune(locale)) > 35 {
			continue
		}
		if label := boundedString(stringValue(entries[rawLocale]), maxRunes); label != "" {
			localized[locale] = label
		}
	}
	return localizedTextOutput(localized, maxRunes)
}

func declaredStringEnumValues(value interface{}) map[string]struct{} {
	allowed := make(map[string]struct{})
	add := func(raw string) {
		if raw != "" && len([]rune(raw)) <= 256 {
			allowed[raw] = struct{}{}
		}
	}
	switch values := value.(type) {
	case []string:
		for _, raw := range values {
			add(raw)
		}
	case []interface{}:
		for _, raw := range values {
			if text, ok := enumDisplayValueKey(raw); ok {
				add(text)
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func enumDisplayValueKey(value interface{}) (string, bool) {
	if text, ok := value.(string); ok {
		return text, true
	}
	switch value.(type) {
	case nil, bool,
		float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		json.Number:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	default:
		return "", false
	}
}

func enumValueLabelsAnnotation(value interface{}, allowedValues map[string]struct{}) map[string]interface{} {
	if len(allowedValues) == 0 {
		return nil
	}
	localizedEntries := stringKeyedObject(value)
	if len(localizedEntries) == 0 {
		return nil
	}
	locales := sortedKeys(localizedEntries)
	orderedLocales := make([]string, 0, len(locales))
	for _, preferredLocale := range []string{integrations.LocaleEnglishUS, integrations.LocaleSimplifiedChinese} {
		if _, exists := localizedEntries[preferredLocale]; exists {
			orderedLocales = append(orderedLocales, preferredLocale)
		}
	}
	for _, locale := range locales {
		if locale != integrations.LocaleEnglishUS && locale != integrations.LocaleSimplifiedChinese {
			orderedLocales = append(orderedLocales, locale)
		}
	}
	out := make(map[string]interface{}, min(len(localizedEntries), 16))
	for _, rawLocale := range orderedLocales {
		if len(out) >= 16 {
			break
		}
		locale := strings.TrimSpace(rawLocale)
		if locale != rawLocale || len([]rune(locale)) < 2 || len([]rune(locale)) > 35 {
			continue
		}
		valueEntries := stringKeyedObject(localizedEntries[rawLocale])
		if len(valueEntries) == 0 {
			continue
		}
		labels := make(map[string]interface{}, min(len(valueEntries), 64))
		for _, enumValue := range sortedKeys(valueEntries) {
			if len(labels) >= 64 {
				break
			}
			if _, allowed := allowedValues[enumValue]; !allowed {
				continue
			}
			if label := boundedString(stringValue(valueEntries[enumValue]), 128); label != "" {
				labels[enumValue] = label
			}
		}
		if len(labels) > 0 {
			out[locale] = labels
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringKeyedObject(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[key] = nested
		}
		return out
	case integrations.LocalizedText:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[key] = nested
		}
		return out
	case map[string]map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[key] = nested
		}
		return out
	case map[string]map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[key] = nested
		}
		return out
	case map[string]integrations.LocalizedText:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[key] = nested
		}
		return out
	default:
		return nil
	}
}

func stringInterfaces(values []string) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func validateTopLevelParameters(toolName string, parameters map[string]interface{}) error {
	allowed := map[string]map[string]struct{}{
		ToolListConnections: {"integration_id": {}},
		ToolSearchActions:   {"query": {}, "integration_id": {}, "limit": {}},
		ToolGetActionGuide:  {"integration_id": {}, "action_id": {}},
		ToolExecuteAction: {
			"integration_id": {}, "integration_name": {}, "integration_name_i18n": {},
			"action_id": {}, "action_name": {}, "action_name_i18n": {},
			"argument_labels_i18n": {}, "argument_value_labels_i18n": {},
			"connection_id": {}, "connection_selector": {},
			"connection_name": {}, "connection_display_name": {}, "connection_selection": {}, "arguments": {},
			"action_schema_hash": {}, "action_schema_revision": {}, "catalog_revision": {},
		},
	}[toolName]
	for key := range parameters {
		if _, ok := allowed[key]; !ok {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "meta tool arguments contain an unsupported field", nil)
		}
	}
	if toolName == ToolExecuteAction {
		_, connectionIDProvided := parameters["connection_id"]
		selectorRaw, selectorProvided := parameters["connection_selector"]
		if connectionIDProvided && selectorProvided {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action accepts either connection_id or connection_selector, not both", nil)
		}
		if selectorProvided && normalizedString(selectorRaw) != preferredSelector {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "execute_action connection_selector is invalid", nil)
		}
	}
	return nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func runtimeString(parameters map[string]interface{}, key string) string {
	return strings.TrimSpace(stringValue(parameters[key]))
}

func normalizedString(value interface{}) string {
	return strings.ToLower(strings.TrimSpace(stringValue(value)))
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	valueString, _ := value.(string)
	return valueString
}

func integerValue(value interface{}, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func boundedString(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func cloneMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]interface{}); ok {
			out[key] = cloneMap(nested)
		} else {
			out[key] = value
		}
	}
	return out
}

func redactInternalConnectionID(value interface{}, connectionID string) interface{} {
	connectionID = strings.ToLower(strings.TrimSpace(connectionID))
	if connectionID == "" {
		return value
	}
	redactString := func(input string) string {
		if strings.Contains(strings.ToLower(input), connectionID) {
			return hiddenReferenceSentinel
		}
		return input
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[redactString(key)] = redactInternalConnectionID(nested, connectionID)
		}
		return out
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[redactString(key)] = redactString(nested)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, nested := range typed {
			out[index] = redactInternalConnectionID(nested, connectionID)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		for index, nested := range typed {
			out[index] = redactString(nested)
		}
		return out
	case string:
		return redactString(typed)
	default:
		return value
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func optionalUUIDString(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}
