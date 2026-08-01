package metatools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderPublishesStrictBoundedSchemas(t *testing.T) {
	fixture := newMetaToolFixture(t)
	entity := fixture.provider.GetEntity()
	if entity.Identity.Name != ProviderID || entity.ProviderType != tools.ToolProviderTypeConnector || len(entity.Tools) != 4 {
		t.Fatalf("provider entity = %#v", entity)
	}
	for _, tool := range entity.Tools {
		if tool.InputSchema["additionalProperties"] != false {
			t.Errorf("tool %s input allows additional properties", tool.Identity.Name)
		}
		if tool.OutputSchema["additionalProperties"] != false {
			t.Errorf("tool %s output allows additional properties", tool.Identity.Name)
		}
		if err := tools.ValidateJSONSchema(tool.InputSchema); err != nil {
			t.Errorf("tool %s input schema: %v", tool.Identity.Name, err)
		}
		if err := tools.ValidateJSONSchema(tool.OutputSchema); err != nil {
			t.Errorf("tool %s output schema: %v", tool.Identity.Name, err)
		}
	}
	execute, err := fixture.provider.GetTool(ToolExecuteAction)
	if err != nil {
		t.Fatal(err)
	}
	executeSchema := execute.GetEntity().InputSchema
	properties, ok := executeSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("execute_action properties = %#v", executeSchema["properties"])
	}
	connectionSchema, ok := properties["connection_id"].(map[string]interface{})
	if !ok || connectionSchema["readOnly"] != true {
		t.Fatalf("connection_id schema = %#v, want server-owned readOnly field", properties["connection_id"])
	}
	for _, field := range []string{
		"integration_name", "integration_name_i18n", "action_name", "action_name_i18n",
		"argument_labels_i18n", "argument_value_labels_i18n",
	} {
		fieldSchema, fieldOK := properties[field].(map[string]interface{})
		if !fieldOK || fieldSchema["readOnly"] != true {
			t.Fatalf("%s schema = %#v, want server-owned readOnly field", field, properties[field])
		}
	}
	modelSchema := tools.ModelVisibleJSONSchema(executeSchema)
	modelSchemaJSON, marshalErr := json.Marshal(modelSchema)
	if marshalErr != nil {
		t.Fatalf("marshal model-visible execute schema: %v", marshalErr)
	}
	for _, forbidden := range []string{
		"integration_name", "integration_name_i18n", "action_name", "action_name_i18n",
		"argument_labels_i18n", "argument_value_labels_i18n",
		"connection_id", "connection_name", "connection_display_name", "connection_selection",
		"action_schema_hash", "action_schema_revision", "catalog_revision", "operation_batch", "batch_summary",
	} {
		if strings.Contains(string(modelSchemaJSON), forbidden) {
			t.Fatalf("model-visible execute schema contains %q: %s", forbidden, modelSchemaJSON)
		}
	}
	invalid := map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_id": fixture.connectionOne.ID.String(), "arguments": map[string]interface{}{}, "credential": "must-not-pass",
	}
	if err := tools.ValidateJSONSchemaValue(execute.GetEntity().InputSchema, invalid); err == nil {
		t.Fatal("execute_action accepted an undeclared top-level credential field")
	}
	base := map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID, "arguments": map[string]interface{}{},
	}
	if err := tools.ValidateJSONSchemaValue(execute.GetEntity().InputSchema, base); err != nil {
		t.Fatalf("execute_action rejected implicit preferred selection: %v", err)
	}
	withExplicitConnection := cloneMap(base)
	withExplicitConnection["connection_id"] = fixture.connectionOne.ID.String()
	if err := tools.ValidateJSONSchemaValue(execute.GetEntity().InputSchema, withExplicitConnection); err != nil {
		t.Fatalf("execute_action raw schema rejected canonical internal UUID: %v", err)
	}
	withSelector := cloneMap(base)
	withSelector["connection_selector"] = preferredSelector
	if err := tools.ValidateJSONSchemaValue(execute.GetEntity().InputSchema, withSelector); err != nil {
		t.Fatalf("execute_action rejected preferred selector: %v", err)
	}
	withAlias := cloneMap(base)
	withAlias["connection_selector"] = "default"
	if err := tools.ValidateJSONSchemaValue(execute.GetEntity().InputSchema, withAlias); err == nil {
		t.Fatal("execute_action accepted an untrusted selector alias")
	}
	withBoth := cloneMap(withSelector)
	withBoth["connection_id"] = fixture.connectionOne.ID.String()
	if err := tools.ValidateJSONSchemaValue(execute.GetEntity().InputSchema, withBoth); err == nil {
		t.Fatal("execute_action accepted both an explicit connection and a selector")
	}
}

func TestMetaToolsRejectNonAIChatCallers(t *testing.T) {
	fixture := newMetaToolFixture(t)
	tool, err := fixture.provider.GetTool(ToolListConnections)
	if err != nil {
		t.Fatal(err)
	}
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID: fixture.organizationID.String(), InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		},
	})
	_, err = tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{}, nil, nil, nil)
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("non-AIChat error = %v, code = %q", err, integrations.ErrorCode(err))
	}
}

func TestAgentMetaToolsUseOnlyExplicitSharedReadBinding(t *testing.T) {
	fixture := newAgentMetaToolFixture(t, toolgovernance.EffectRead, toolgovernance.ApprovalPolicyNeverAsk, true)
	fixture.access.agentPreferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.agentActionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	agentID := uuid.New()
	runtimeParameters := map[string]interface{}{
		"agent_id": agentID.String(),
		"integration_connection_ids": map[string]string{
			fixture.integrationID: fixture.connectionOne.ID.String(),
		},
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String()},
		},
		tools.AgentBindingAuthorizationsParameter: []tools.AgentBindingAuthorization{{
			BindingType:      "integration_connection",
			ResourceID:       fixture.connectionOne.ID.String(),
			ParentResourceID: fixture.integrationID,
			AccessMode:       "read",
			AllowedActionIDs: []string{fixture.actionID},
			BoundByAccountID: fixture.accountID.String(),
			BoundAtUnix:      123,
		}},
	}
	runtimeParameters = skills.WithAgentBindingVerifier(runtimeParameters, func(_ context.Context, check skills.AgentBindingCheck) (bool, error) {
		return check.BindingType == "integration_connection" &&
			check.ResourceID == fixture.connectionOne.ID.String() &&
			check.ParentResourceID == fixture.integrationID &&
			check.AccessMode == "read" &&
			check.ActionID == fixture.actionID, nil
	})

	listTool := fixture.agentRuntimeTool(t, ToolListConnections, runtimeParameters)
	messages, err := listTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Agent list_connections error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["count"] != 1 {
		t.Fatalf("Agent list_connections messages = %#v", messages)
	}
	assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID)

	executeTool := fixture.agentRuntimeTool(t, ToolExecuteAction, runtimeParameters)
	messages, err = executeTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID,
		"action_id":      fixture.actionID,
		"arguments":      map[string]interface{}{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Agent execute_action error = %v", err)
	}
	if len(messages) != 1 || len(fixture.executor.requests) != 1 {
		t.Fatalf("Agent execute_action result=%#v requests=%#v", messages, fixture.executor.requests)
	}
	request := fixture.executor.requests[0]
	if request.AgentID != agentID.String() || request.InvokeFrom != tools.ToolInvokeFromAgent || request.ConnectionID != fixture.connectionOne.ID.String() {
		t.Fatalf("Agent execution request = %#v", request)
	}
	if request.VerifyAgentConnection == nil {
		t.Fatal("Agent execution request has no request-scoped binding verifier")
	}
	matched, verifyErr := request.VerifyAgentConnection(context.Background(), integrations.AgentConnectionAuthorizationRequest{
		ConnectionID:  fixture.connectionOne.ID.String(),
		IntegrationID: fixture.integrationID,
		ActionID:      fixture.actionID,
	})
	if verifyErr != nil || !matched {
		t.Fatalf("request-scoped verifier matched=%v error=%v", matched, verifyErr)
	}
	assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID)
}

func TestAgentMetaToolsFailClosedForPersonalWriteAndAlwaysAskConnections(t *testing.T) {
	tests := []struct {
		name             string
		effect           toolgovernance.Effect
		approval         toolgovernance.ApprovalPolicy
		credentialSource integrations.ConnectionCredentialSource
		wantCode         string
	}{
		{name: "personal read", effect: toolgovernance.EffectRead, approval: toolgovernance.ApprovalPolicyNeverAsk, credentialSource: integrations.ConnectionCredentialSourceAccount, wantCode: integrations.ErrorCodeAccessDenied},
		{name: "shared write", effect: toolgovernance.EffectCreate, approval: toolgovernance.ApprovalPolicyNeverAsk, credentialSource: integrations.ConnectionCredentialSourceOrganization, wantCode: integrations.ErrorCodeInvalidInput},
		{name: "shared always ask", effect: toolgovernance.EffectRead, approval: toolgovernance.ApprovalPolicyAlwaysAsk, credentialSource: integrations.ConnectionCredentialSourceOrganization, wantCode: integrations.ErrorCodeAccessDenied},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newAgentMetaToolFixture(t, testCase.effect, testCase.approval, true)
			fixture.connectionOne.CredentialSource = testCase.credentialSource
			if testCase.credentialSource == integrations.ConnectionCredentialSourceAccount {
				owner := fixture.accountID
				fixture.connectionOne.OwnerAccountID = &owner
			}
			fixture.access.agentPreferenceAllowed[fixture.connectionOne.ID] = true
			fixture.access.agentActionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
			params := map[string]interface{}{
				"agent_id": uuid.NewString(),
				"integration_connection_ids": map[string]string{
					fixture.integrationID: fixture.connectionOne.ID.String(),
				},
				"integration_selected_connection_ids": map[string][]string{
					fixture.integrationID: {fixture.connectionOne.ID.String()},
				},
				tools.AgentBindingAuthorizationsParameter: []tools.AgentBindingAuthorization{{
					BindingType: "integration_connection", ResourceID: fixture.connectionOne.ID.String(),
					ParentResourceID: fixture.integrationID, AccessMode: "write",
					AllowedActionIDs: []string{fixture.actionID},
					BoundByAccountID: fixture.accountID.String(), BoundAtUnix: 123,
				}},
			}
			params = skills.WithAgentBindingVerifier(params, func(context.Context, skills.AgentBindingCheck) (bool, error) {
				return true, nil
			})
			tool := fixture.agentRuntimeTool(t, ToolExecuteAction, params)
			_, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
				"integration_id": fixture.integrationID,
				"action_id":      fixture.actionID,
				"arguments":      map[string]interface{}{},
			}, nil, nil, nil)
			if integrations.ErrorCode(err) != testCase.wantCode {
				t.Fatalf("error=%v code=%q, want %q", err, integrations.ErrorCode(err), testCase.wantCode)
			}
			if len(fixture.executor.requests) != 0 {
				t.Fatalf("provider executor ran for denied Agent action: %#v", fixture.executor.requests)
			}
		})
	}
}

func TestPreparationHintsExposeOnlyExecutableReadActions(t *testing.T) {
	fixture := newAgentMetaToolFixture(t, toolgovernance.EffectRead, toolgovernance.ApprovalPolicyNeverAsk, true)
	fixture.access.agentActionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	params := map[string]interface{}{
		"agent_id": uuid.NewString(),
		"integration_connection_ids": map[string]string{
			fixture.integrationID: fixture.connectionOne.ID.String(),
		},
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String()},
		},
		tools.AgentBindingAuthorizationsParameter: []tools.AgentBindingAuthorization{{
			BindingType: "integration_connection", ResourceID: fixture.connectionOne.ID.String(),
			ParentResourceID: fixture.integrationID, AccessMode: "read",
			AllowedActionIDs: []string{fixture.actionID}, BoundByAccountID: fixture.accountID.String(), BoundAtUnix: 123,
		}},
	}
	params = skills.WithAgentBindingVerifier(params, func(context.Context, skills.AgentBindingCheck) (bool, error) {
		return true, nil
	})
	runtimeTool := fixture.agentRuntimeTool(t, ToolGetActionGuide, params)
	tool, ok := runtimeTool.(*Tool)
	if !ok {
		t.Fatalf("runtime tool type = %T, want *Tool", runtimeTool)
	}
	target := integrations.ActionDefinition{PreparationHints: []integrations.ActionPreparationHint{{
		ActionID:        fixture.actionID,
		Relation:        integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{"recipient_id"},
		ResultPaths:     []string{"results[].id"},
		Description:     "Resolve the recipient before executing the target action.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Resolve the recipient before executing the target action.",
			integrations.LocaleSimplifiedChinese: "执行目标操作前先解析接收人。",
		},
	}}}
	hints := tool.preparationHintsOutput(context.Background(), fixture.accountID.String(), target, fixture.connectionOne)
	if len(hints) != 1 {
		t.Fatalf("preparation hints = %#v, want one executable read hint", hints)
	}
	hint, _ := hints[0].(map[string]interface{})
	if hint["action_id"] != fixture.actionID || hint["relation"] != string(integrations.ActionPreparationResolveTarget) {
		t.Fatalf("preparation hint = %#v", hint)
	}

	fixture.access.agentActionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = false
	if hints := tool.preparationHintsOutput(context.Background(), fixture.accountID.String(), target, fixture.connectionOne); len(hints) != 0 {
		t.Fatalf("unauthorized preparation hints = %#v, want none", hints)
	}
	fixture.access.agentActionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true

	policyKey := fixture.integrationID + "/" + fixture.actionID
	fixture.policies.decisions[policyKey] = integrations.ActionPolicyDecision{
		Enabled: false, ApprovalPolicy: integrations.IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}
	if hints := tool.preparationHintsOutput(context.Background(), fixture.accountID.String(), target, fixture.connectionOne); len(hints) != 0 {
		t.Fatalf("policy-disabled preparation hints = %#v, want none", hints)
	}
	delete(fixture.policies.decisions, policyKey)

	target.PreparationHints[0].ActionID = "gmail.missing"
	if hints := tool.preparationHintsOutput(context.Background(), fixture.accountID.String(), target, fixture.connectionOne); len(hints) != 0 {
		t.Fatalf("unknown preparation hints = %#v, want none", hints)
	}
}

func TestAgentReadActionUsesEffectiveOrganizationPolicy(t *testing.T) {
	fixture := newAgentMetaToolFixture(t, toolgovernance.EffectRead, toolgovernance.ApprovalPolicyNeverAsk, false)
	action, ok := fixture.registry.ActionDetail(fixture.integrationID, fixture.actionID)
	if !ok {
		t.Fatal("Agent action is not registered")
	}
	tool, ok := fixture.agentRuntimeTool(t, ToolSearchActions, map[string]interface{}{}).(*Tool)
	if !ok {
		t.Fatal("Agent meta tool has an unexpected type")
	}
	if tool.agentActionExecutableWithoutInteraction(context.Background(), fixture.organizationID, fixture.integrationID, action) {
		t.Fatal("default-disabled Agent action should not be executable without an organization override")
	}
	fixture.policies.decisions[fixture.integrationID+"/"+fixture.actionID] = integrations.ActionPolicyDecision{
		Enabled: true, ApprovalPolicy: integrations.IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}
	if !tool.agentActionExecutableWithoutInteraction(context.Background(), fixture.organizationID, fixture.integrationID, action) {
		t.Fatal("organization-enabled read action should be executable by an explicitly authorized Agent")
	}
	fixture.policies.decisions[fixture.integrationID+"/"+fixture.actionID] = integrations.ActionPolicyDecision{
		Enabled: true, ApprovalPolicy: integrations.IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
	if tool.agentActionExecutableWithoutInteraction(context.Background(), fixture.organizationID, fixture.integrationID, action) {
		t.Fatal("Agent action requiring interactive approval must remain unavailable")
	}
}

func TestListConnectionsRestrictsSelectionAndServerAccess(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = false
	tool := fixture.runtimeTool(t, ToolListConnections, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()},
		},
		"integration_connection_ids": map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	})
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["count"] != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	connections, ok := messages[0].Data["connections"].([]interface{})
	if !ok || len(connections) != 1 {
		t.Fatalf("connections = %#v", messages[0].Data["connections"])
	}
	connection := connections[0].(map[string]interface{})
	if connection["name"] != fixture.connectionOne.Name || connection["selection"] != preferredSelector {
		t.Fatalf("connection = %#v", connection)
	}
	for _, forbidden := range []string{"id", "connection_id", "is_default", "encrypted_credentials", "credentials", "api_key", "config"} {
		if _, exists := connection[forbidden]; exists {
			t.Fatalf("safe connection view exposes %s: %#v", forbidden, connection)
		}
	}
	assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID, fixture.connectionTwo.ID)
	if err := tools.ValidateJSONSchemaValue(tool.GetEntity().OutputSchema, messages[0].Data); err != nil {
		t.Fatalf("list_connections output schema error = %v", err)
	}
}

func TestSearchAndGuideExposeOnlyActionAuthorizedConnections(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	runtimeParameters := map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()},
		},
		"integration_connection_ids": map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	}
	searchTool := fixture.runtimeTool(t, ToolSearchActions, runtimeParameters)
	messages, err := searchTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{"query": "issue", "limit": 5}, nil, nil, nil)
	if err != nil {
		t.Fatalf("search Invoke() error = %v", err)
	}
	actions := messages[0].Data["actions"].([]interface{})
	if len(actions) != 1 {
		t.Fatalf("actions = %#v", actions)
	}
	action := actions[0].(map[string]interface{})
	if action["connection_name"] != fixture.connectionOne.Name || action["connection_selection"] != preferredSelector {
		t.Fatalf("preferred connection = %#v", action)
	}
	if !reflect.DeepEqual(action["required_any_scopes"], []interface{}{"pulls:write", "repo:write"}) ||
		!reflect.DeepEqual(action["preferred_scopes"], []interface{}{"repo:write"}) {
		t.Fatalf("search action alternative scope contract = %#v", action)
	}
	if !reflect.DeepEqual(action["required_arguments"], []interface{}{
		map[string]interface{}{"name": "title", "type": "string"},
	}) || !reflect.DeepEqual(action["optional_arguments"], []interface{}{
		map[string]interface{}{"name": "state", "type": "string"},
	}) {
		t.Fatalf("search action compact input contract = %#v", action)
	}
	if action["guide_recommended"] != true {
		t.Fatalf("search action guide_recommended = %#v, want true for an enum-constrained argument", action["guide_recommended"])
	}
	assertScopeLabelsOutput(t, action["scope_labels_i18n"])
	assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID, fixture.connectionTwo.ID)
	if err := tools.ValidateJSONSchemaValue(searchTool.GetEntity().OutputSchema, messages[0].Data); err != nil {
		t.Fatalf("search_actions output schema error = %v", err)
	}

	guideTool := fixture.runtimeTool(t, ToolGetActionGuide, runtimeParameters)
	messages, err = guideTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("guide Invoke() error = %v", err)
	}
	guide := messages[0].Data
	if guide["input_schema"] == nil || guide["output_schema"] == nil || guide["schema_revision"] == "" {
		t.Fatalf("guide = %#v", guide)
	}
	if guide["connection_name"] != fixture.connectionOne.Name || guide["connection_selection"] != preferredSelector {
		t.Fatalf("guide preferred connection = %#v", guide)
	}
	if !reflect.DeepEqual(guide["required_any_scopes"], []interface{}{"pulls:write", "repo:write"}) ||
		!reflect.DeepEqual(guide["preferred_scopes"], []interface{}{"repo:write"}) {
		t.Fatalf("guide alternative scope contract = %#v", guide)
	}
	assertScopeLabelsOutput(t, guide["scope_labels_i18n"])
	assertNoConnectionUUIDs(t, guide, fixture.connectionOne.ID, fixture.connectionTwo.ID)
	if err := tools.ValidateJSONSchemaValue(guideTool.GetEntity().OutputSchema, guide); err != nil {
		t.Fatalf("get_action_guide output schema error = %v", err)
	}
}

func TestCompactActionInputContractRecommendsGuideOnlyForNonTrivialSchemas(t *testing.T) {
	simple := compactActionInputContract(strictObject(map[string]interface{}{
		"query":     map[string]interface{}{"type": "string", "minLength": 1},
		"page_size": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50},
	}, "query"))
	if simple["guide_recommended"] != false {
		t.Fatalf("simple guide recommendation = %#v, want false", simple)
	}

	constrained := compactActionInputContract(strictObject(map[string]interface{}{
		"recipient_type": map[string]interface{}{"type": "string", "enum": []interface{}{"self", "open_id"}},
		"payload":        strictObject(map[string]interface{}{"text": map[string]interface{}{"type": "string"}}, "text"),
	}, "recipient_type", "payload"))
	if constrained["guide_recommended"] != true {
		t.Fatalf("constrained guide recommendation = %#v, want true", constrained)
	}
}

func TestSearchAndGuideReportScopeUpgradeBeforeExecution(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.connectionOne.AuthType = integrations.ConnectionAuthTypeOAuth2
	fixture.connectionOne.GrantedScopes = []string{"profile:read"}
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	runtimeParameters := map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String()},
		},
		"integration_connection_ids": map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	}
	searchTool := fixture.runtimeTool(t, ToolSearchActions, runtimeParameters)
	messages, err := searchTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "limit": 5,
	}, nil, nil, nil)
	if err != nil || len(messages) != 1 {
		t.Fatalf("search messages = %#v, err = %v", messages, err)
	}
	actions := messages[0].Data["actions"].([]interface{})
	if len(actions) != 1 {
		t.Fatalf("search actions = %#v", actions)
	}
	action := actions[0].(map[string]interface{})
	if action["availability"] != actionAvailabilityScopeGap || action["can_execute"] != false ||
		action["recovery_action"] != "upgrade_oauth_scope" {
		t.Fatalf("search availability = %#v", action)
	}
	guideTool := fixture.runtimeTool(t, ToolGetActionGuide, runtimeParameters)
	guideMessages, err := guideTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
	}, nil, nil, nil)
	if err != nil || len(guideMessages) != 1 {
		t.Fatalf("guide messages = %#v, err = %v", guideMessages, err)
	}
	if guideMessages[0].Data["availability"] != actionAvailabilityScopeGap ||
		guideMessages[0].Data["can_execute"] != false {
		t.Fatalf("guide availability = %#v", guideMessages[0].Data)
	}
}

func TestSearchAndGuideDoNotOfferActionsAvailableOnlyThroughNonPreferredConnection(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
	fixture.access.actionAllowed[fixture.connectionTwo.ID.String()+"/"+fixture.actionID] = true
	runtimeParameters := map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()},
		},
		"integration_connection_ids": map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	}
	searchTool := fixture.runtimeTool(t, ToolSearchActions, runtimeParameters)
	messages, err := searchTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "limit": 5,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("search Invoke() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["count"] != 0 {
		t.Fatalf("search exposed a non-preferred-only action: %#v", messages)
	}
	guideTool := fixture.runtimeTool(t, ToolGetActionGuide, runtimeParameters)
	_, err = guideTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
	}, nil, nil, nil)
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("guide error = %v, code = %q", err, integrations.ErrorCode(err))
	}
}

func TestSearchGuideAndExecuteRejectActionIncompatibleWithPreferredConnectionAuthentication(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.connectionOne.AuthMethodID = "tenant_app"
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	runtimeParameters := map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String()},
		},
		"integration_connection_ids": map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	}

	searchTool := fixture.runtimeTool(t, ToolSearchActions, runtimeParameters)
	messages, err := searchTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "limit": 5,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("search Invoke() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["count"] != 0 {
		t.Fatalf("search exposed an authentication-incompatible action: %#v", messages)
	}

	guideTool := fixture.runtimeTool(t, ToolGetActionGuide, runtimeParameters)
	_, err = guideTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
	}, nil, nil, nil)
	if integrations.ErrorCode(err) != integrations.ErrorCodeActionAuthMethod {
		t.Fatalf("guide error = %v, code = %q", err, integrations.ErrorCode(err))
	}

	executeTool := fixture.runtimeTool(t, ToolExecuteAction, runtimeParameters)
	_, err = executeTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"arguments": map[string]interface{}{"title": "hello"},
	}, nil, nil, nil)
	if integrations.ErrorCode(err) != integrations.ErrorCodeActionAuthMethod {
		t.Fatalf("execute error = %v, code = %q", err, integrations.ErrorCode(err))
	}
	if len(fixture.executor.requests) != 0 {
		t.Fatalf("executor received authentication-incompatible request: %#v", fixture.executor.requests)
	}
}

func TestExecuteActionRejectsConnectionOutsideSelectedSetBeforeDelegation(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
	fixture.access.actionAllowed[fixture.connectionTwo.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionTwo.ID.String()},
	})
	_, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_id": fixture.connectionTwo.ID.String(), "arguments": map[string]interface{}{"title": "hello"},
	}, nil, nil, nil)
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("Invoke() error = %v, code = %q", err, integrations.ErrorCode(err))
	}
	if len(fixture.executor.requests) != 0 {
		t.Fatalf("executor received unauthorized requests: %#v", fixture.executor.requests)
	}
}

func TestExplicitEmptyFullSelectionDoesNotRevivePreferredConnection(t *testing.T) {
	connectionID := uuid.New().String()
	got := selectedConnectionReferences(map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{},
		"integration_connection_ids":          map[string]string{"github": connectionID},
	})
	if len(got) != 0 {
		t.Fatalf("selected references = %#v, want empty authoritative full selection", got)
	}
}

func TestExecuteActionResolvesAndAuthorizesPreferredConnection(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		includeSelector bool
	}{
		{name: "implicit"},
		{name: "explicit preferred selector", includeSelector: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newMetaToolFixture(t)
			fixture.executor.result.Output["provider_debug"] = map[string]interface{}{
				"nested_connection":                             []interface{}{fixture.connectionTwo.ID.String()},
				"internal_" + fixture.connectionTwo.ID.String(): "hidden",
			}
			fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
			fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
			fixture.access.actionAllowed[fixture.connectionTwo.ID.String()+"/"+fixture.actionID] = true
			tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
				"integration_selected_connection_ids": map[string][]string{
					fixture.integrationID: {fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()},
				},
				"integration_connection_ids": map[string]string{fixture.integrationID: fixture.connectionTwo.ID.String()},
			})
			arguments := map[string]interface{}{
				"integration_id": fixture.integrationID, "action_id": fixture.actionID,
				"arguments": map[string]interface{}{"title": "hello"},
			}
			if testCase.includeSelector {
				arguments["connection_selector"] = preferredSelector
			}
			messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), arguments, nil, nil, nil)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if len(fixture.executor.requests) != 1 || fixture.executor.requests[0].ConnectionID != fixture.connectionTwo.ID.String() {
				t.Fatalf("executor requests = %#v", fixture.executor.requests)
			}
			if len(messages) != 1 || messages[0].Data["connection_name"] != fixture.connectionTwo.Name || messages[0].Data["connection_selection"] != preferredSelector {
				t.Fatalf("safe execution result = %#v", messages)
			}
			result := messages[0].Data["result"].(map[string]interface{})
			providerDebug := result["provider_debug"].(map[string]interface{})
			nestedConnection := providerDebug["nested_connection"].([]interface{})
			if len(nestedConnection) != 1 || nestedConnection[0] != hiddenReferenceSentinel {
				t.Fatalf("redacted provider result = %#v", result)
			}
			if providerDebug[hiddenReferenceSentinel] != "hidden" {
				t.Fatalf("redacted provider result key = %#v", providerDebug)
			}
			if _, exposed := messages[0].Data["connection_id"]; exposed {
				t.Fatalf("execution result exposed connection_id: %#v", messages[0].Data)
			}
			assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID, fixture.connectionTwo.ID)
		})
	}
}

func TestExecuteActionLabelsReplayedOperationForTheModel(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.executor.result.Replayed = true
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	})
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"arguments": map[string]interface{}{"title": "hello"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["operation_status"] != "already_completed" {
		t.Fatalf("replayed execution result = %#v", messages)
	}
}

func TestExecuteActionBatchUsesDistinctFrozenItemsAndReturnsAggregateStatus(t *testing.T) {
	fixture := newMetaToolFixture(t)
	enableFixtureSuccessGuard(t, fixture, "title")
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	})
	messageID := uuid.NewString()
	parameters := map[string]interface{}{
		"integration_id": fixture.integrationID,
		"action_id":      fixture.actionID,
		"connection_id":  fixture.connectionOne.ID.String(),
		"batch_items": []interface{}{
			map[string]interface{}{"title": "first"},
			map[string]interface{}{"title": "second"},
		},
	}
	if _, err := integrations.EnsureOperationBatchMetadata(
		parameters, messageID, fixture.connectionOne.ID.String(), fixture.integrationID, fixture.actionID,
	); err != nil {
		t.Fatalf("EnsureOperationBatchMetadata() error = %v", err)
	}
	if err := tools.ValidateJSONSchemaValue(tool.GetEntity().InputSchema, parameters); err != nil {
		t.Fatalf("batch input schema error = %v, parameters = %#v", err, parameters)
	}
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), parameters, nil, nil, &messageID)
	if err != nil {
		t.Fatalf("batch Invoke() error = %v", err)
	}
	if len(fixture.executor.requests) != 2 {
		t.Fatalf("batch requests = %#v", fixture.executor.requests)
	}
	if fixture.executor.requests[0].OperationItemID == fixture.executor.requests[1].OperationItemID ||
		fixture.executor.requests[0].BatchID == "" ||
		fixture.executor.requests[0].BatchID != fixture.executor.requests[1].BatchID {
		t.Fatalf("batch request identities = %#v", fixture.executor.requests)
	}
	if len(messages) != 1 || messages[0].Data["operation_status"] != "succeeded" {
		t.Fatalf("batch messages = %#v", messages)
	}
	batch, ok := messages[0].Data["batch"].(map[string]interface{})
	if !ok || batch["item_count"] != 2 || batch["succeeded_count"] != 2 || batch["status"] != "succeeded" {
		t.Fatalf("batch output = %#v", messages[0].Data["batch"])
	}
	encoded, marshalErr := json.Marshal(messages[0].Data)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), "batch-") || strings.Contains(string(encoded), "operation_item_id") {
		t.Fatalf("public batch output exposed internal identities: %s", encoded)
	}
	if err := tools.ValidateJSONSchemaValue(tool.GetEntity().OutputSchema, messages[0].Data); err != nil {
		t.Fatalf("batch output schema error = %v", err)
	}
}

func TestExecuteActionReturnsStructuredRetryStateForGuardedDefiniteFailure(t *testing.T) {
	fixture := newMetaToolFixture(t)
	enableFixtureSuccessGuard(t, fixture, "title")
	fixture.executor.err = integrations.NewError(integrations.ErrorCodeProviderRejected, "rejected", nil)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	})
	messageID := uuid.NewString()
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"arguments": map[string]interface{}{"title": "retry me"},
	}, nil, nil, &messageID)
	if err != nil {
		t.Fatalf("guarded failure must be returned as a structured retry state: %v", err)
	}
	if len(messages) != 1 || messages[0].Data["operation_status"] != "failed_safe" ||
		messages[0].Data["retry_safe"] != true || messages[0].Data["error_code"] != integrations.ErrorCodeProviderRejected {
		t.Fatalf("guarded failure output = %#v", messages)
	}
}

func TestExecuteActionPreservesPreferredClassificationAfterGovernanceEnrichment(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	})
	enricher := tool.(tools.ToolGovernanceArgumentEnricher)
	enriched := enricher.EnrichGovernanceArguments(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_selector": preferredSelector, "connection_name": "spoofed", "connection_selection": "explicit",
		"integration_name": "spoofed integration", "integration_name_i18n": map[string]interface{}{"en-US": "spoofed integration"},
		"action_name": "spoofed action", "action_name_i18n": map[string]interface{}{"en-US": "spoofed action"},
		"argument_labels_i18n": map[string]interface{}{"title": map[string]interface{}{"en-US": "spoofed title"}},
		"argument_value_labels_i18n": map[string]interface{}{
			"state": map[string]interface{}{"en-US": map[string]interface{}{"open": "spoofed open"}},
		},
		"arguments": map[string]interface{}{"title": "hello"},
	})
	if enriched["connection_id"] != fixture.connectionOne.ID.String() {
		t.Fatalf("enriched connection identity = %#v", enriched)
	}
	if enriched["connection_name"] != fixture.connectionOne.Name || enriched["connection_selection"] != preferredSelector {
		t.Fatalf("enriched safe connection identity = %#v", enriched)
	}
	definition, _ := fixture.registry.ProviderDefinition(fixture.integrationID)
	action, _ := fixture.registry.ActionDetail(fixture.integrationID, fixture.actionID)
	assertExecuteActionMetadata(t, enriched, definition, action)
	assertExecuteActionArgumentMetadata(t, enriched)
	if _, remains := enriched["connection_selector"]; remains {
		t.Fatalf("enriched selector was not canonicalized: %#v", enriched)
	}
	unresolved := enricher.EnrichGovernanceArguments(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": "unknown.action",
		"integration_name": "spoofed integration", "integration_name_i18n": map[string]interface{}{"en-US": "spoofed integration"},
		"action_name": "spoofed action", "action_name_i18n": map[string]interface{}{"en-US": "spoofed action"},
		"argument_labels_i18n": map[string]interface{}{"title": map[string]interface{}{"en-US": "spoofed title"}},
		"argument_value_labels_i18n": map[string]interface{}{
			"state": map[string]interface{}{"en-US": map[string]interface{}{"open": "spoofed open"}},
		},
		"connection_name": "spoofed connection", "connection_selection": "preferred",
		"arguments": map[string]interface{}{"title": "hello"},
	})
	for _, field := range []string{
		"integration_name", "integration_name_i18n", "action_name", "action_name_i18n",
		"argument_labels_i18n", "argument_value_labels_i18n",
		"connection_name", "connection_display_name", "connection_selection",
	} {
		if _, remains := unresolved[field]; remains {
			t.Fatalf("unresolved enrichment retained untrusted %s: %#v", field, unresolved)
		}
	}
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), enriched, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["connection_selection"] != preferredSelector {
		t.Fatalf("enriched preferred result = %#v", messages)
	}
	assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID)
}

func TestExecuteActionGovernanceEnrichmentReturnsMissingScopeReason(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.connectionOne.AuthType = integrations.ConnectionAuthTypeOAuth2
	fixture.connectionOne.GrantedScopes = []string{"profile:read"}
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricherWithError)
	if !ok {
		t.Fatal("execute_action does not implement fail-closed governance enrichment")
	}
	enriched, err := enricher.EnrichGovernanceArgumentsWithError(
		context.Background(),
		fixture.accountID.String(),
		map[string]interface{}{
			"integration_id": fixture.integrationID,
			"action_id":      fixture.actionID,
			"arguments":      map[string]interface{}{"title": "hello"},
		},
	)
	if integrations.ErrorCode(err) != integrations.ErrorCodeInsufficientScope {
		t.Fatalf("error = %v (%s)", err, integrations.ErrorCode(err))
	}
	if _, exposed := enriched["connection_id"]; exposed {
		t.Fatalf("failed enrichment exposed an internal connection id: %#v", enriched)
	}
}

func TestModelVisibleConnectionLabelsCannotEchoInternalUUID(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.connectionOne.Name = fixture.connectionOne.ID.String()
	displayName := "account " + fixture.connectionOne.ID.String()
	fixture.connectionOne.DisplayName = &displayName
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	runtimeParameters := map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
		"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
	}
	listTool := fixture.runtimeTool(t, ToolListConnections, runtimeParameters)
	listed, err := listTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("list Invoke() error = %v", err)
	}
	connections := listed[0].Data["connections"].([]interface{})
	if got := connections[0].(map[string]interface{})["name"]; got != fixture.integrationID {
		t.Fatalf("language-neutral connection fallback = %#v, want %q", got, fixture.integrationID)
	}
	assertNoConnectionUUIDs(t, listed[0].Data, fixture.connectionOne.ID)

	executeTool := fixture.runtimeTool(t, ToolExecuteAction, runtimeParameters)
	executed, err := executeTool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"arguments": map[string]interface{}{"title": "hello"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("execute Invoke() error = %v", err)
	}
	if got := executed[0].Data["connection_name"]; got != fixture.integrationID {
		t.Fatalf("language-neutral execution connection fallback = %#v, want %q", got, fixture.integrationID)
	}
	assertNoConnectionUUIDs(t, executed[0].Data, fixture.connectionOne.ID)
}

func TestSafeConnectionNameUsesLanguageNeutralFallbacks(t *testing.T) {
	if got := safeConnectionName(nil); got != hiddenReferenceSentinel {
		t.Fatalf("nil connection name = %q, want hidden-reference sentinel", got)
	}
	connection := &integrations.IntegrationConnection{ID: uuid.New(), IntegrationID: "github"}
	if got := safeConnectionName(connection); got != "github" {
		t.Fatalf("unnamed connection = %q, want stable integration identifier", got)
	}
	connection.Name = "reference " + connection.ID.String()
	if got := safeConnectionName(connection); got != "github" {
		t.Fatalf("unsafe connection name = %q, want stable integration identifier", got)
	}
	connection.IntegrationID = ""
	if got := safeConnectionName(connection); got != hiddenReferenceSentinel {
		t.Fatalf("connection without a safe label = %q, want hidden-reference sentinel", got)
	}
}

func TestLocalizedTextOutputDeterministicallyPreservesPrimaryLocales(t *testing.T) {
	values := integrations.LocalizedText{
		integrations.LocaleEnglishUS:         "English",
		integrations.LocaleSimplifiedChinese: "Chinese",
	}
	for _, locale := range []string{"aa", "ab", "ac", "ad", "ae", "af", "ag", "ah", "ai", "aj", "ak", "al", "am", "an", "ao"} {
		values[locale] = locale
	}
	first := localizedTextOutput(values, 128)
	second := localizedTextOutput(values, 128)
	if len(first) != 16 || first[integrations.LocaleEnglishUS] != "English" || first[integrations.LocaleSimplifiedChinese] != "Chinese" {
		t.Fatalf("bounded localized text = %#v", first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("localized text selection is not deterministic: %#v != %#v", first, second)
	}
}

func TestActionArgumentDisplayMetadataExtractsNestedObjectAndArrayPaths(t *testing.T) {
	schema := strictObject(map[string]interface{}{
		"filters": map[string]interface{}{
			"type": "object",
			"title_i18n": integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Filters", integrations.LocaleSimplifiedChinese: "筛选条件",
			},
			"properties": map[string]interface{}{
				"state": map[string]interface{}{
					"type": "string", "enum": []interface{}{"open", "closed"},
					"title_i18n": integrations.LocalizedText{
						integrations.LocaleEnglishUS: "State", integrations.LocaleSimplifiedChinese: "状态",
					},
					"enum_labels_i18n": map[string]map[string]string{
						integrations.LocaleEnglishUS:         {"open": "Open", "closed": "Closed"},
						integrations.LocaleSimplifiedChinese: {"open": "开启", "closed": "关闭"},
					},
				},
			},
		},
		"requests": map[string]interface{}{
			"type": "array",
			"title_i18n": integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Requests", integrations.LocaleSimplifiedChinese: "请求列表",
			},
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"mode": map[string]interface{}{
						"type": "string",
						"title_i18n": integrations.LocalizedText{
							integrations.LocaleEnglishUS: "Mode", integrations.LocaleSimplifiedChinese: "模式",
						},
					},
				},
			},
		},
	})

	labels, valueLabels := actionArgumentDisplayMetadata(schema)
	for path, want := range map[string]string{
		"filters": "筛选条件", "filters.state": "状态", "requests": "请求列表", "requests.mode": "模式",
	} {
		localized, ok := labels[path].(map[string]interface{})
		if !ok || localized[integrations.LocaleSimplifiedChinese] != want {
			t.Fatalf("argument label %q = %#v, want zh-Hans %q", path, labels[path], want)
		}
	}
	stateLabels, ok := valueLabels["filters.state"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested enum labels = %#v", valueLabels)
	}
	chinese, ok := stateLabels[integrations.LocaleSimplifiedChinese].(map[string]interface{})
	if !ok || chinese["open"] != "开启" || chinese["closed"] != "关闭" {
		t.Fatalf("nested localized enum labels = %#v", stateLabels)
	}
}

func TestActionArgumentDisplayMetadataHonorsDepthAndFieldLimits(t *testing.T) {
	properties := make(map[string]interface{}, maxArgumentDisplayFields+10)
	for index := 0; index < maxArgumentDisplayFields+10; index++ {
		properties[fmt.Sprintf("field_%03d", index)] = map[string]interface{}{
			"type": "string",
			"title_i18n": integrations.LocalizedText{
				integrations.LocaleEnglishUS: fmt.Sprintf("Field %d", index),
			},
		}
	}
	labels, _ := actionArgumentDisplayMetadata(strictObject(properties))
	if len(labels) != maxArgumentDisplayFields {
		t.Fatalf("bounded field metadata count = %d, want %d", len(labels), maxArgumentDisplayFields)
	}

	root := map[string]interface{}{"type": "object"}
	current := root
	pathParts := make([]string, 0, maxArgumentDisplayDepth+2)
	for depth := 0; depth < maxArgumentDisplayDepth+2; depth++ {
		name := fmt.Sprintf("level_%d", depth)
		pathParts = append(pathParts, name)
		child := map[string]interface{}{
			"type": "object",
			"title_i18n": integrations.LocalizedText{
				integrations.LocaleEnglishUS: fmt.Sprintf("Level %d", depth),
			},
		}
		current["properties"] = map[string]interface{}{name: child}
		current = child
	}
	deepLabels, _ := actionArgumentDisplayMetadata(root)
	lastAllowed := strings.Join(pathParts[:maxArgumentDisplayDepth], ".")
	firstBlocked := strings.Join(pathParts[:maxArgumentDisplayDepth+1], ".")
	if _, ok := deepLabels[lastAllowed]; !ok {
		t.Fatalf("last allowed nested path %q is missing: %#v", lastAllowed, deepLabels)
	}
	if _, leaked := deepLabels[firstBlocked]; leaked {
		t.Fatalf("nested path beyond depth limit %q was exposed: %#v", firstBlocked, deepLabels)
	}
}

func TestExecuteActionPreferredConnectionFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*metaToolFixture) map[string]interface{}
	}{
		{
			name: "no preferred connection",
			prepare: func(fixture *metaToolFixture) map[string]interface{} {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
				return map[string]interface{}{
					"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
				}
			},
		},
		{
			name: "preferred is outside full selection",
			prepare: func(fixture *metaToolFixture) map[string]interface{} {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
				fixture.access.actionAllowed[fixture.connectionTwo.ID.String()+"/"+fixture.actionID] = true
				return map[string]interface{}{
					"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
					"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionTwo.ID.String()},
				}
			},
		},
		{
			name: "preferred alias is not trusted",
			prepare: func(fixture *metaToolFixture) map[string]interface{} {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				return map[string]interface{}{
					"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
					"integration_connection_ids":          map[string]string{fixture.integrationID: "default"},
				}
			},
		},
		{
			name: "case-folded duplicate preference is ambiguous",
			prepare: func(fixture *metaToolFixture) map[string]interface{} {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
				return map[string]interface{}{
					"integration_selected_connection_ids": map[string][]string{
						fixture.integrationID: {fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()},
					},
					"integration_connection_ids": map[string]interface{}{
						fixture.integrationID: fixture.connectionOne.ID.String(), strings.ToUpper(fixture.integrationID): fixture.connectionTwo.ID.String(),
					},
				}
			},
		},
		{
			name: "preferred connection is not authorized for the action",
			prepare: func(fixture *metaToolFixture) map[string]interface{} {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				return map[string]interface{}{
					"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
					"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
				}
			},
		},
		{
			name: "preferred connection is not available",
			prepare: func(fixture *metaToolFixture) map[string]interface{} {
				fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
				return map[string]interface{}{
					"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
					"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
				}
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newMetaToolFixture(t)
			tool := fixture.runtimeTool(t, ToolExecuteAction, testCase.prepare(fixture))
			_, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
				"integration_id": fixture.integrationID, "action_id": fixture.actionID,
				"connection_selector": preferredSelector, "arguments": map[string]interface{}{"title": "hello"},
			}, nil, nil, nil)
			if err == nil {
				t.Fatal("Invoke() unexpectedly resolved an unsafe preferred connection")
			}
			if len(fixture.executor.requests) != 0 {
				t.Fatalf("executor received unsafe requests: %#v", fixture.executor.requests)
			}
		})
	}
}

func TestExecuteActionEnrichesFrozenIdentityAndDelegatesWithoutCredentials(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("execute_action tool does not enrich governance arguments")
	}
	action, _ := fixture.registry.ActionDetail(fixture.integrationID, fixture.actionID)
	definition, _ := fixture.registry.ProviderDefinition(fixture.integrationID)
	enriched := enricher.EnrichGovernanceArguments(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_id": fixture.connectionOne.ID.String(), "arguments": map[string]interface{}{"title": "hello"},
		"integration_name": "spoofed integration", "integration_name_i18n": map[string]interface{}{"fr-FR": "spoofed"},
		"action_name": "spoofed action", "action_name_i18n": map[string]interface{}{"fr-FR": "spoofed"},
		"argument_labels_i18n": map[string]interface{}{"title": map[string]interface{}{"fr-FR": "spoofed"}},
		"argument_value_labels_i18n": map[string]interface{}{
			"state": map[string]interface{}{"fr-FR": map[string]interface{}{"open": "spoofed"}},
		},
	})
	if enriched["action_schema_hash"] != action.SchemaHash || enriched["catalog_revision"] != action.CatalogRevision {
		t.Fatalf("enriched revisions = %#v", enriched)
	}
	assertExecuteActionMetadata(t, enriched, definition, action)
	assertExecuteActionArgumentMetadata(t, enriched)
	stale := enricher.EnrichGovernanceArguments(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_id": fixture.connectionOne.ID.String(), "arguments": map[string]interface{}{"title": "hello"},
		"action_schema_hash": "sha256:stale", "catalog_revision": "sha256:stale",
	})
	if stale["action_schema_hash"] != "sha256:stale" || stale["catalog_revision"] != "sha256:stale" {
		t.Fatalf("stale frozen revisions were silently upgraded: %#v", stale)
	}
	// Invocation must independently resolve display metadata rather than trust a
	// resumed or otherwise caller-supplied internal field.
	enriched["integration_name"] = "spoofed after enrichment"
	enriched["integration_name_i18n"] = map[string]interface{}{"fr-FR": "spoofed"}
	enriched["action_name"] = "spoofed after enrichment"
	enriched["action_name_i18n"] = map[string]interface{}{"fr-FR": "spoofed"}
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), enriched, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(fixture.executor.requests) != 1 {
		t.Fatalf("executor requests = %#v", fixture.executor.requests)
	}
	request := fixture.executor.requests[0]
	if request.ConnectionID != fixture.connectionOne.ID.String() || request.IntegrationID != fixture.integrationID || request.ActionID != fixture.actionID {
		t.Fatalf("delegated identity = %#v", request)
	}
	if !reflect.DeepEqual(request.Input, map[string]interface{}{"title": "hello"}) || request.Connection != nil {
		t.Fatalf("delegated input/connection = %#v / %#v", request.Input, request.Connection)
	}
	if len(messages) != 1 || messages[0].Data["action_schema_hash"] != action.SchemaHash || messages[0].Data["catalog_revision"] != action.CatalogRevision {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Data["connection_selection"] != "explicit" || messages[0].Data["connection_name"] != fixture.connectionOne.Name {
		t.Fatalf("explicit connection summary = %#v", messages[0].Data)
	}
	assertExecuteActionMetadata(t, messages[0].Data, definition, action)
	assertNoConnectionUUIDs(t, messages[0].Data, fixture.connectionOne.ID, fixture.connectionTwo.ID)
	if err := tools.ValidateJSONSchemaValue(tool.GetEntity().OutputSchema, messages[0].Data); err != nil {
		t.Fatalf("execute_action output schema error = %v", err)
	}
}

func TestExecuteActionRejectsStaleFrozenCatalogRevision(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
	})
	_, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_id": fixture.connectionOne.ID.String(), "arguments": map[string]interface{}{"title": "hello"},
		"action_schema_hash": "sha256:stale", "action_schema_revision": "stale", "catalog_revision": "sha256:stale",
	}, nil, nil, nil)
	if integrations.ErrorCode(err) != integrations.ErrorCodePolicyConflict {
		t.Fatalf("stale revision error = %v, code = %q", err, integrations.ErrorCode(err))
	}
	if len(fixture.executor.requests) != 0 {
		t.Fatalf("executor ran with stale catalog: %#v", fixture.executor.requests)
	}
}

func TestExecuteActionBoundsExecutorOutputWithoutReportingCompletedActionAsFailed(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	fixture.executor.result = &integrations.ActionResult{
		Output:      map[string]interface{}{"blob": strings.Repeat("x", maxExecuteActionOutputBytes)},
		ResultCount: 1, AttemptCount: 1,
	}
	tool := fixture.runtimeTool(t, ToolExecuteAction, map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
	})
	messages, err := tool.Invoke(context.Background(), fixture.accountID.String(), map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_id": fixture.connectionOne.ID.String(), "arguments": map[string]interface{}{"title": "hello"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("oversized completed action error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["result_truncated"] != true {
		t.Fatalf("bounded result = %#v", messages)
	}
	result, _ := messages[0].Data["result"].(map[string]interface{})
	if result["status"] != "completed" || result["content_truncated"] != true || result["result_code"] != resultCodeOutputTruncated {
		t.Fatalf("bounded completion marker = %#v", result)
	}
	if _, exists := result["message"]; exists {
		t.Fatalf("bounded completion marker contains locale-specific prose: %#v", result)
	}
	if encoded, marshalErr := json.Marshal(messages[0].Data); marshalErr != nil || len(encoded) > maxExecuteActionOutputBytes {
		t.Fatalf("bounded output size = %d, marshal error = %v", len(encoded), marshalErr)
	}
}

func TestSkillRuntimeFreezesRealActionIdentityAndCatalogRevisions(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(fixture.provider); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	manifestResolver := integrations.NewGovernanceManifestResolver(fixture.registry, allowActionPolicyResolver{})
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager).WithToolGovernanceGateway(
		skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).WithManifestResolver(manifestResolver),
	)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{skills.SkillExternalApps})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	invocation, err := runtime.CallSkillTool(context.Background(), resolved, skills.SkillExternalApps, ToolExecuteAction, map[string]interface{}{
		"integration_id": fixture.integrationID, "action_id": fixture.actionID,
		"connection_selector": preferredSelector, "arguments": map[string]interface{}{"title": "hello"},
	}, skills.ExecutionContext{
		OrganizationID: fixture.organizationID.String(), UserID: fixture.accountID.String(), ConversationID: uuid.NewString(),
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"integration_selected_connection_ids": map[string][]string{fixture.integrationID: {fixture.connectionOne.ID.String()}},
			"integration_connection_ids":          map[string]string{fixture.integrationID: fixture.connectionOne.ID.String()},
			"tool_governance_permission_tier":     "basic",
		},
	}, "call_external_action")
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil || invocation.Trace.Governance == nil || invocation.Trace.Governance.FrozenInvocation == nil {
		t.Fatalf("governance invocation = %#v", invocation)
	}
	frozen := invocation.Trace.Governance.FrozenInvocation
	action, _ := fixture.registry.ActionDetail(fixture.integrationID, fixture.actionID)
	if frozen.ToolID != fixture.integrationID+":"+fixture.actionID || frozen.Effect != action.Effect || frozen.RiskLevel != action.RiskLevel {
		t.Fatalf("frozen action governance = %#v", frozen)
	}
	if len(frozen.Assets) != 1 || frozen.Assets[0].Type != "integration_connection" || frozen.Assets[0].ID != fixture.connectionOne.ID.String() {
		t.Fatalf("frozen connection scope = %#v", frozen.Assets)
	}
	if frozen.Arguments["connection_id"] != fixture.connectionOne.ID.String() ||
		frozen.Arguments["action_schema_hash"] != action.SchemaHash ||
		frozen.Arguments["catalog_revision"] != action.CatalogRevision {
		t.Fatalf("frozen action identity/revisions = %#v", frozen.Arguments)
	}
	definition, _ := fixture.registry.ProviderDefinition(fixture.integrationID)
	assertExecuteActionMetadata(t, frozen.Arguments, definition, action)
	assertExecuteActionArgumentMetadata(t, frozen.Arguments)
	if len(fixture.executor.requests) != 0 {
		t.Fatalf("executor ran before approval: %#v", fixture.executor.requests)
	}
}

type metaToolFixture struct {
	organizationID uuid.UUID
	accountID      uuid.UUID
	integrationID  string
	actionID       string
	connectionOne  *integrations.IntegrationConnection
	connectionTwo  *integrations.IntegrationConnection
	registry       *integrations.Registry
	lookup         *fakeConnectionLookup
	access         *fakeConnectionAccess
	policies       *fakeActionPolicyResolver
	executor       *fakeActionExecutor
	provider       *Provider
}

func newMetaToolFixture(t *testing.T) *metaToolFixture {
	t.Helper()
	organizationID := uuid.New()
	accountID := uuid.New()
	integrationID := "github"
	actionID := "issue.create"
	action := integrations.ActionDefinition{
		ID: actionID, ToolName: "create_issue", Name: "Create issue", Description: "Create one repository issue.",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "Create issue", integrations.LocaleSimplifiedChinese: "创建议题",
		},
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "Create one repository issue.", integrations.LocaleSimplifiedChinese: "创建一个仓库议题。",
		},
		InputSchema: strictObject(map[string]interface{}{
			"title": map[string]interface{}{
				"type": "string", "minLength": 1, "maxLength": 256,
				"title_i18n": map[string]interface{}{
					integrations.LocaleEnglishUS: "Issue title", integrations.LocaleSimplifiedChinese: "议题标题",
					"x": "ignored invalid locale",
				},
			},
			"state": map[string]interface{}{
				"type": "string", "enum": []interface{}{"open", "closed"},
				"title_i18n": map[string]interface{}{
					integrations.LocaleEnglishUS: "Issue state", integrations.LocaleSimplifiedChinese: "议题状态",
				},
				"enum_labels_i18n": map[string]interface{}{
					integrations.LocaleEnglishUS:         map[string]interface{}{"open": "Open", "closed": "Closed", "spoofed": "Must be ignored"},
					integrations.LocaleSimplifiedChinese: map[string]interface{}{"open": "开启", "closed": "关闭"},
					"x":                                  map[string]interface{}{"open": "ignored invalid locale"},
				},
			},
		}, "title"),
		OutputSchema: strictObject(map[string]interface{}{
			"issue_id": map[string]interface{}{"type": "string"},
		}, "issue_id"),
		Effect: toolgovernance.EffectCreate, RiskLevel: toolgovernance.RiskLevelHigh,
		DataEgress: true, ExternalDestination: "api.github.com", Idempotent: false,
		RequiredScopes:    []string{"issues:write"},
		RequiredAnyScopes: []string{"repo:write", "pulls:write"},
		PreferredScopes:   []string{"repo:write"},
		ScopeLabelsI18n: integrations.LocalizedLabelMap{
			"issues:write": {
				integrations.LocaleEnglishUS: "Write issues", integrations.LocaleSimplifiedChinese: "写入议题",
			},
			"repo:write": {
				integrations.LocaleEnglishUS: "Write repositories", integrations.LocaleSimplifiedChinese: "写入仓库",
			},
			"pulls:write": {
				integrations.LocaleEnglishUS: "Write pull requests", integrations.LocaleSimplifiedChinese: "写入合并请求",
			},
		},
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
		},
		SupportedAuthMethodIDs: []string{"api_key"},
		SupportedCallers:       []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
	}
	registry := integrations.NewRegistry()
	definition := integrations.ProviderDefinition{
		ID: integrationID, DriverID: "github-rest", Name: "GitHub",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "GitHub", integrations.LocaleSimplifiedChinese: "GitHub",
		},
		Description: "GitHub test integration.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "GitHub test integration.", integrations.LocaleSimplifiedChinese: "GitHub 测试集成。",
		},
		AuthMethods: []integrations.AuthMethodDefinition{{
			ID: "api_key", Type: integrations.AuthMethodTypeAPIKey,
			CredentialSource: integrations.ConnectionCredentialSourceOrganization,
			Label:            "API key",
			LabelI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "API key", integrations.LocaleSimplifiedChinese: "API 密钥",
			},
			Available: true,
			Fields: []integrations.CredentialFieldDefinition{{
				Key: "token", Label: "Token",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Token", integrations.LocaleSimplifiedChinese: "令牌",
				},
				Input: integrations.CredentialFieldInputPassword, Required: true, Secret: true,
			}},
		}},
		Scopes: []integrations.ProviderScopeDefinition{
			{
				ID: "issues:write", Label: "Write issues",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Write issues", integrations.LocaleSimplifiedChinese: "写入议题",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite,
			},
			{
				ID: "repo:write", Label: "Write repositories",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Write repositories", integrations.LocaleSimplifiedChinese: "写入仓库",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite,
			},
			{
				ID: "pulls:write", Label: "Write pull requests",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Write pull requests", integrations.LocaleSimplifiedChinese: "写入合并请求",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite,
			},
		},
		Actions: []integrations.ActionDefinition{action},
	}
	if err := registry.Register(integrations.Registration{
		Definition: definition, Adapter: fakeRegistryAdapter{driverID: "github-rest"},
	}); err != nil {
		t.Fatalf("register integration: %v", err)
	}
	connectionOne := activeConnection(organizationID, integrationID, "github-rest", "Primary")
	connectionTwo := activeConnection(organizationID, integrationID, "github-rest", "Secondary")
	lookup := &fakeConnectionLookup{connections: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionOne.ID: connectionOne, connectionTwo.ID: connectionTwo,
	}}
	access := &fakeConnectionAccess{preferenceAllowed: map[uuid.UUID]bool{}, actionAllowed: map[string]bool{}}
	executor := &fakeActionExecutor{result: &integrations.ActionResult{
		Output: map[string]interface{}{"issue_id": "I_1"}, ProviderRequestID: "request-1", ResultCount: 1, AttemptCount: 1,
	}}
	policies := &fakeActionPolicyResolver{decisions: map[string]integrations.ActionPolicyDecision{}}
	provider, err := NewProvider(registry, executor, lookup, access, policies)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	return &metaToolFixture{
		organizationID: organizationID, accountID: accountID, integrationID: integrationID, actionID: actionID,
		connectionOne: connectionOne, connectionTwo: connectionTwo,
		registry: registry, lookup: lookup, access: access, policies: policies, executor: executor, provider: provider,
	}
}

func enableFixtureSuccessGuard(t *testing.T, fixture *metaToolFixture, targetPaths ...string) {
	t.Helper()
	definition, _ := fixture.registry.ProviderDefinition(fixture.integrationID)
	action, _ := fixture.registry.ActionDetail(fixture.integrationID, fixture.actionID)
	action.SuccessDeduplication = &integrations.SuccessDeduplicationDefinition{
		TargetArgumentPaths: append([]string(nil), targetPaths...),
	}
	definition.CatalogRevision = ""
	action.CatalogRevision = ""
	definition.Actions = []integrations.ActionDefinition{action}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{
		Definition: definition, Adapter: fakeRegistryAdapter{driverID: definition.DriverID},
	}); err != nil {
		t.Fatalf("register guarded action: %v", err)
	}
	provider, err := NewProvider(registry, fixture.executor, fixture.lookup, fixture.access, fixture.policies)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	fixture.registry = registry
	fixture.provider = provider
}

func newAgentMetaToolFixture(
	t *testing.T,
	effect toolgovernance.Effect,
	approval toolgovernance.ApprovalPolicy,
	defaultEnabled bool,
) *metaToolFixture {
	t.Helper()
	organizationID := uuid.New()
	accountID := uuid.New()
	integrationID := "gmail"
	actionID := "gmail.message.list"
	action := integrations.ActionDefinition{
		ID: actionID, ToolName: "list_messages", Name: "List messages",
		Description: "List mailbox messages.",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "List messages",
			integrations.LocaleSimplifiedChinese: "列出邮件",
		},
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "List mailbox messages.",
			integrations.LocaleSimplifiedChinese: "列出邮箱中的邮件。",
		},
		InputSchema: strictObject(map[string]interface{}{}),
		OutputSchema: strictObject(map[string]interface{}{
			"messages": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		}, "messages"),
		Effect: effect, RiskLevel: toolgovernance.RiskLevelLow,
		DataEgress: true, ExternalDestination: "gmail.googleapis.com", Idempotent: true,
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: defaultEnabled, ApprovalPolicy: approval, DataEgressAllowed: true,
		},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
	}
	if effect == toolgovernance.EffectRead {
		action.SupportedCallers = append(action.SupportedCallers, tools.ToolInvokeFromAgent)
	}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{
		Definition: integrations.ProviderDefinition{
			ID: integrationID, DriverID: "gmail", Name: "Gmail",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Gmail",
				integrations.LocaleSimplifiedChinese: "Gmail",
			},
			Description: "Gmail test integration.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Gmail test integration.",
				integrations.LocaleSimplifiedChinese: "Gmail 测试集成。",
			},
			AuthMethods: []integrations.AuthMethodDefinition{{
				ID: "none", Type: integrations.AuthMethodTypeNone,
				CredentialSource: integrations.ConnectionCredentialSourceOrganization,
				Label:            "No authentication", Available: true,
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "No authentication",
					integrations.LocaleSimplifiedChinese: "无需身份验证",
				},
			}},
			Actions: []integrations.ActionDefinition{action},
		},
		Adapter: fakeRegistryAdapter{driverID: "gmail"},
	}); err != nil {
		t.Fatalf("register Agent integration: %v", err)
	}
	connection := activeConnection(organizationID, integrationID, "gmail", "Team Gmail")
	connection.AuthMethodID = "api_key"
	lookup := &fakeConnectionLookup{connections: map[uuid.UUID]*integrations.IntegrationConnection{connection.ID: connection}}
	access := &fakeConnectionAccess{
		preferenceAllowed:      map[uuid.UUID]bool{},
		actionAllowed:          map[string]bool{},
		agentPreferenceAllowed: map[uuid.UUID]bool{},
		agentActionAllowed:     map[string]bool{},
	}
	executor := &fakeActionExecutor{result: &integrations.ActionResult{
		Output:      map[string]interface{}{"messages": []interface{}{"message-1"}},
		ResultCount: 1, AttemptCount: 1,
	}}
	policies := &fakeActionPolicyResolver{decisions: map[string]integrations.ActionPolicyDecision{}}
	provider, err := NewProvider(registry, executor, lookup, access, policies)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	return &metaToolFixture{
		organizationID: organizationID, accountID: accountID,
		integrationID: integrationID, actionID: actionID,
		connectionOne: connection,
		registry:      registry, lookup: lookup, access: access, policies: policies, executor: executor, provider: provider,
	}
}

func (fixture *metaToolFixture) runtimeTool(t *testing.T, name string, runtimeParameters map[string]interface{}) tools.Tool {
	t.Helper()
	tool, err := fixture.provider.GetTool(name)
	if err != nil {
		t.Fatal(err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID: fixture.organizationID.String(), InvokeFrom: tools.ToolInvokeFromAIChat, RuntimeParameters: runtimeParameters,
	})
}

func (fixture *metaToolFixture) agentRuntimeTool(t *testing.T, name string, runtimeParameters map[string]interface{}) tools.Tool {
	t.Helper()
	tool, err := fixture.provider.GetTool(name)
	if err != nil {
		t.Fatal(err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID: fixture.organizationID.String(), InvokeFrom: tools.ToolInvokeFromAgent, RuntimeParameters: runtimeParameters,
	})
}

func activeConnection(organizationID uuid.UUID, integrationID, driverID, name string) *integrations.IntegrationConnection {
	return &integrations.IntegrationConnection{
		ID: uuid.New(), OrganizationID: organizationID, IntegrationID: integrationID, DriverID: driverID, Name: name,
		CredentialSource: integrations.ConnectionCredentialSourceOrganization, AuthType: integrations.ConnectionAuthTypeAPIKey,
		AuthMethodID: "api_key",
		Status:       integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy,
		AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified,
	}
}

type fakeRegistryAdapter struct{ driverID string }

func (adapter fakeRegistryAdapter) DriverID() string { return adapter.driverID }

func (fakeRegistryAdapter) Execute(context.Context, integrations.ActionRequest) (*integrations.ActionResult, error) {
	return nil, errors.New("registry adapter should not be called directly")
}

type fakeConnectionLookup struct {
	connections map[uuid.UUID]*integrations.IntegrationConnection
}

func (lookup *fakeConnectionLookup) GetByID(_ context.Context, organizationID, connectionID uuid.UUID) (*integrations.IntegrationConnection, error) {
	connection := lookup.connections[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return nil, integrations.ErrConnectionNotFound
	}
	copyConnection := *connection
	return &copyConnection, nil
}

type fakeConnectionAccess struct {
	preferenceAllowed      map[uuid.UUID]bool
	actionAllowed          map[string]bool
	agentPreferenceAllowed map[uuid.UUID]bool
	agentActionAllowed     map[string]bool
}

type fakeActionPolicyResolver struct {
	decisions map[string]integrations.ActionPolicyDecision
	err       error
}

func (resolver *fakeActionPolicyResolver) Resolve(
	_ context.Context,
	_ string,
	integrationID string,
	action integrations.ActionDefinition,
) (integrations.ActionPolicyDecision, error) {
	if resolver.err != nil {
		return integrations.ActionPolicyDecision{}, resolver.err
	}
	if decision, exists := resolver.decisions[integrationID+"/"+action.ID]; exists {
		return decision, nil
	}
	decision := integrations.ActionPolicyDecision{
		Enabled: true, ApprovalPolicy: integrations.IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}
	if action.DefaultPolicy != nil {
		decision.Enabled = action.DefaultPolicy.Enabled
		decision.DataEgressAllowed = !action.DataEgress || action.DefaultPolicy.DataEgressAllowed
		if action.DefaultPolicy.ApprovalPolicy == toolgovernance.ApprovalPolicyAlwaysAsk {
			decision.ApprovalPolicy = integrations.IntegrationApprovalPolicyAlwaysAsk
		}
	}
	return decision, nil
}

func (access *fakeConnectionAccess) AuthorizeConnectionPreference(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, connectionID uuid.UUID) error {
	if access.preferenceAllowed[connectionID] {
		return nil
	}
	return integrations.NewError(integrations.ErrorCodeAccessDenied, "not visible", nil)
}

func (access *fakeConnectionAccess) AuthorizeConnectionUse(_ context.Context, request integrations.ConnectionAccessRequest) error {
	if access.actionAllowed[request.ConnectionID.String()+"/"+request.ActionID] {
		return nil
	}
	return integrations.NewError(integrations.ErrorCodeAccessDenied, "not authorized", nil)
}

func (access *fakeConnectionAccess) AuthorizeAgentConnectionPreference(_ context.Context, _ uuid.UUID, _ *uuid.UUID, connectionID uuid.UUID) error {
	if access.agentPreferenceAllowed[connectionID] {
		return nil
	}
	return integrations.NewError(integrations.ErrorCodeAccessDenied, "not available to Agent", nil)
}

func (access *fakeConnectionAccess) AuthorizeAgentConnectionActionPreference(
	_ context.Context,
	_ uuid.UUID,
	_ *uuid.UUID,
	connectionID uuid.UUID,
	_ string,
	actionID string,
	_ toolgovernance.Effect,
) error {
	if access.agentActionAllowed[connectionID.String()+"/"+actionID] {
		return nil
	}
	return integrations.NewError(integrations.ErrorCodeAccessDenied, "action not available to Agent", nil)
}

type fakeActionExecutor struct {
	requests []integrations.ActionRequest
	result   *integrations.ActionResult
	err      error
}

func (executor *fakeActionExecutor) Execute(_ context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	executor.requests = append(executor.requests, request)
	return executor.result, executor.err
}

type allowActionPolicyResolver struct{}

func (allowActionPolicyResolver) Resolve(context.Context, string, string, integrations.ActionDefinition) (integrations.ActionPolicyDecision, error) {
	return integrations.ActionPolicyDecision{
		Enabled: true, ApprovalPolicy: integrations.IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}, nil
}

func assertExecuteActionMetadata(
	t *testing.T,
	value map[string]interface{},
	definition integrations.ProviderDefinition,
	action integrations.ActionDefinition,
) {
	t.Helper()
	if value["integration_name"] != definition.Name || value["action_name"] != action.Name {
		t.Fatalf("execute action names = %#v, want integration %q and action %q", value, definition.Name, action.Name)
	}
	assertLocalizedText := func(field string, expected integrations.LocalizedText) {
		t.Helper()
		actual, ok := value[field].(map[string]interface{})
		if !ok || len(actual) != len(expected) {
			t.Fatalf("%s = %#v, want %#v", field, value[field], expected)
		}
		for locale, expectedText := range expected {
			if actual[locale] != expectedText {
				t.Fatalf("%s[%q] = %#v, want %q", field, locale, actual[locale], expectedText)
			}
		}
	}
	assertLocalizedText("integration_name_i18n", definition.NameI18n)
	assertLocalizedText("action_name_i18n", action.NameI18n)
}

func assertExecuteActionArgumentMetadata(t *testing.T, value map[string]interface{}) {
	t.Helper()
	argumentLabels, ok := value["argument_labels_i18n"].(map[string]interface{})
	if !ok || len(argumentLabels) != 2 {
		t.Fatalf("argument_labels_i18n = %#v", value["argument_labels_i18n"])
	}
	titleLabels, titleOK := argumentLabels["title"].(map[string]interface{})
	stateLabels, stateOK := argumentLabels["state"].(map[string]interface{})
	if !titleOK || !stateOK || len(titleLabels) != 2 || len(stateLabels) != 2 ||
		titleLabels[integrations.LocaleEnglishUS] != "Issue title" || titleLabels[integrations.LocaleSimplifiedChinese] != "议题标题" ||
		stateLabels[integrations.LocaleEnglishUS] != "Issue state" || stateLabels[integrations.LocaleSimplifiedChinese] != "议题状态" {
		t.Fatalf("argument labels = %#v", argumentLabels)
	}
	argumentValueLabels, ok := value["argument_value_labels_i18n"].(map[string]interface{})
	if !ok || len(argumentValueLabels) != 1 {
		t.Fatalf("argument_value_labels_i18n = %#v", value["argument_value_labels_i18n"])
	}
	stateValueLabels, ok := argumentValueLabels["state"].(map[string]interface{})
	if !ok || len(stateValueLabels) != 2 {
		t.Fatalf("state value labels = %#v", argumentValueLabels["state"])
	}
	englishLabels, englishOK := stateValueLabels[integrations.LocaleEnglishUS].(map[string]interface{})
	chineseLabels, chineseOK := stateValueLabels[integrations.LocaleSimplifiedChinese].(map[string]interface{})
	if !englishOK || !chineseOK || len(englishLabels) != 2 || len(chineseLabels) != 2 ||
		englishLabels["open"] != "Open" || englishLabels["closed"] != "Closed" ||
		chineseLabels["open"] != "开启" || chineseLabels["closed"] != "关闭" {
		t.Fatalf("localized state value labels = %#v", stateValueLabels)
	}
	if _, leaked := englishLabels["spoofed"]; leaked {
		t.Fatalf("enum label annotation exposed a value absent from the trusted enum: %#v", englishLabels)
	}
}

func assertScopeLabelsOutput(t *testing.T, value interface{}) {
	t.Helper()
	labels, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("scope_labels_i18n = %#v", value)
	}
	issueLabels, ok := labels["issues:write"].(map[string]interface{})
	if !ok || issueLabels[integrations.LocaleEnglishUS] != "Write issues" || issueLabels[integrations.LocaleSimplifiedChinese] != "写入议题" {
		t.Fatalf("localized scope labels = %#v", labels)
	}
}

func assertNoConnectionUUIDs(t *testing.T, value interface{}, connectionIDs ...uuid.UUID) {
	t.Helper()
	forbidden := make([]string, 0, len(connectionIDs))
	for _, connectionID := range connectionIDs {
		if connectionID != uuid.Nil {
			forbidden = append(forbidden, strings.ToLower(connectionID.String()))
		}
	}
	var inspect func(interface{}, string)
	inspect = func(current interface{}, path string) {
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, nested := range typed {
				inspect(nested, path+"."+key)
			}
		case []interface{}:
			for index, nested := range typed {
				inspect(nested, fmt.Sprintf("%s[%d]", path, index))
			}
		case []string:
			for index, nested := range typed {
				inspect(nested, fmt.Sprintf("%s[%d]", path, index))
			}
		case string:
			lowered := strings.ToLower(typed)
			for _, connectionID := range forbidden {
				if strings.Contains(lowered, connectionID) {
					t.Fatalf("model-visible output exposed a connection UUID at %s", path)
				}
			}
		}
	}
	inspect(value, "$")
}
