package integrations

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestGovernanceResolverAlwaysAskRequiresEveryInvocationApproval(t *testing.T) {
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), &testAdapter{driverID: DriverExa})
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{
				Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
			}, nil
		},
	))
	manifest, err := resolver.ResolveToolGovernanceManifest(context.Background(), skills.ToolGovernanceRequest{
		Manifest:     toolgovernance.Manifest{ToolID: "web.search"},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   IntegrationWebSearch,
		ToolName:     "search_web",
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID,
		},
	})
	if err != nil {
		t.Fatalf("ResolveToolGovernanceManifest() error = %v", err)
	}
	if manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || !manifest.ApprovalEveryInvocation {
		t.Fatalf("resolved manifest = %#v", manifest)
	}
}

func TestGovernanceResolverAllowsOrganizationToEnableDefaultDisabledAction(t *testing.T) {
	action := testAction("mail.send", "send_mail")
	action.Effect = toolgovernance.EffectCreate
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
	registry := registerTestAction(t, action, &testAdapter{driverID: DriverExa})
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{
				Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
			}, nil
		},
	))

	manifest, err := resolver.ResolveToolGovernanceManifest(context.Background(), skills.ToolGovernanceRequest{
		Manifest:     toolgovernance.Manifest{ToolID: "mail.send"},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   IntegrationWebSearch,
		ToolName:     action.ToolName,
		Arguments:    map[string]interface{}{"message": "hello"},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID,
		},
	})
	if err != nil {
		t.Fatalf("ResolveToolGovernanceManifest() error = %v", err)
	}
	if manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("resolved manifest approval = %q", manifest.DefaultApprovalPolicy)
	}
}

func TestGovernanceResolverPreservesProviderAlwaysAskWhenOrganizationEnablesAction(t *testing.T) {
	action := testAction("mail.send", "send_mail")
	action.Effect = toolgovernance.EffectExternalSend
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
	registry := registerTestAction(t, action, &testAdapter{driverID: DriverExa})
	repository := newMemoryActionPolicyRepository()
	policies := NewActionPolicyService(repository, staticConnectionCatalog{driver: DriverExa, actions: []ActionDefinition{action}})
	organizationID := uuid.MustParse(testOrganizationID)
	if _, err := policies.Replace(context.Background(), organizationID, IntegrationWebSearch, []ActionPolicyInput{{
		ActionID: action.ID, Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}}, nil); err != nil {
		t.Fatal(err)
	}
	resolver := NewGovernanceManifestResolver(registry, policies)

	manifest, err := resolver.ResolveToolGovernanceManifest(context.Background(), skills.ToolGovernanceRequest{
		Manifest:     toolgovernance.Manifest{ToolID: action.ID},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   IntegrationWebSearch,
		ToolName:     action.ToolName,
		Arguments:    map[string]interface{}{"query": "hello"},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID,
		},
	})
	if err != nil {
		t.Fatalf("ResolveToolGovernanceManifest() error = %v", err)
	}
	if manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || !manifest.ApprovalEveryInvocation {
		t.Fatalf("resolved manifest = %#v", manifest)
	}
}

func TestGovernanceResolverMetaExecuteUsesRealHighRiskWriteAction(t *testing.T) {
	action := testAction("issue.create", "create_issue")
	action.Effect = toolgovernance.EffectCreate
	action.RiskLevel = toolgovernance.RiskLevelHigh
	action.ExternalDestination = "api.github.com"
	action.RequiredScopes = []string{"issues:write"}
	action.RequiredAnyScopes = []string{"repo:write", "pulls:write"}
	action.PreferredScopes = []string{"repo:write"}
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
	registry := NewRegistry()
	if err := registry.Register(localizedTestRegistration("github", &testAdapter{driverID: "github-rest"}, []ActionDefinition{action})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true}, nil
		},
	))
	manifest, err := resolver.ResolveToolGovernanceManifest(context.Background(), skills.ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID: "integration.execute_dynamic", SkillID: skills.SkillExternalApps, Domain: "external_integration",
			Effect: toolgovernance.EffectRead, AssetType: "integration_connection", RiskLevel: toolgovernance.RiskLevelLow,
			PermissionScopes: []string{"integration:dynamic:execute"}, DefaultApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk,
			AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
		},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   MetaProviderExternalIntegrations,
		ToolName:     "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "github", "action_id": "issue.create", "connection_id": testConnectionID,
			"arguments": map[string]interface{}{"query": "title"},
		},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID, UserID: testUserID, InvokeFrom: tools.ToolInvokeFromAIChat,
			RuntimeParameters: map[string]interface{}{
				"integration_selected_connection_ids": map[string][]string{"github": {testConnectionID}},
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveToolGovernanceManifest() error = %v", err)
	}
	resolvedAction, _ := registry.ActionDetail("github", "issue.create")
	if manifest.ToolID != "github:issue.create" || manifest.Effect != toolgovernance.EffectCreate || manifest.RiskLevel != toolgovernance.RiskLevelHigh {
		t.Fatalf("resolved manifest identity/governance = %#v", manifest)
	}
	if !manifest.RequiresAssetResolution || manifest.AssetType != "integration_connection" {
		t.Fatalf("resolved manifest connection scope = %#v", manifest)
	}
	if !manifest.ExternalSideEffect || !manifest.DataEgress || manifest.ExternalDestination != "api.github.com" {
		t.Fatalf("resolved manifest side effect = %#v", manifest)
	}
	if manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || !manifest.AuditRequired {
		t.Fatalf("resolved manifest policy = %#v", manifest)
	}
	if !slices.Equal(manifest.PermissionScopes, ActionRequiredScopeIDs(resolvedAction)) {
		t.Fatalf("resolved manifest scopes = %#v, action = %#v", manifest.PermissionScopes, resolvedAction)
	}
}

func TestGovernanceResolverAgentMetaExecuteAllowsOnlyNonInteractiveReadActions(t *testing.T) {
	readAction := testAction("mail.message.list", "list_messages")
	readAction.SupportedCallers = []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent}
	readAction.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
	registry := NewRegistry()
	if err := registry.Register(localizedTestRegistration("gmail", &testAdapter{driverID: "gmail"}, []ActionDefinition{readAction})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	policy := IntegrationApprovalPolicyInherit
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: true, ApprovalPolicy: policy, DataEgressAllowed: true}, nil
		},
	))
	request := skills.ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID: "integration.execute_dynamic", SkillID: skills.SkillExternalApps,
			DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
			AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
		},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   MetaProviderExternalIntegrations,
		ToolName:     "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "gmail", "action_id": readAction.ID, "connection_id": testConnectionID,
			"arguments": map[string]interface{}{"query": "unread"},
		},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID, UserID: testUserID, InvokeFrom: tools.ToolInvokeFromAgent,
			RuntimeParameters: map[string]interface{}{
				"integration_selected_connection_ids": map[string][]string{"gmail": {testConnectionID}},
			},
		},
	}
	manifest, err := resolver.ResolveToolGovernanceManifest(context.Background(), request)
	if err != nil {
		t.Fatalf("Agent read governance error = %v", err)
	}
	if manifest.Effect != toolgovernance.EffectRead || manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
		t.Fatalf("Agent read manifest = %#v", manifest)
	}

	policy = IntegrationApprovalPolicyAlwaysAsk
	if _, err := resolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("Agent always-ask error=%v code=%q", err, ErrorCode(err))
	}

	writeAction := readAction
	writeAction.ID = "mail.message.send"
	writeAction.ToolName = "send_message"
	writeAction.Effect = toolgovernance.EffectExternalSend
	writeAction.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
	writeRegistry := NewRegistry()
	if err := writeRegistry.Register(localizedTestRegistration("gmail", &testAdapter{driverID: "gmail"}, []ActionDefinition{writeAction})); err != nil {
		t.Fatalf("register write action: %v", err)
	}
	writeResolver := NewGovernanceManifestResolver(writeRegistry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true}, nil
		},
	))
	request.Arguments["action_id"] = writeAction.ID
	if _, err := writeResolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("Agent write error=%v code=%q", err, ErrorCode(err))
	}
}

func TestGovernanceResolverMetaExecuteFailsClosedUntilPreferredConnectionIsCanonicalized(t *testing.T) {
	action := testAction("issue.create", "create_issue")
	registry := NewRegistry()
	if err := registry.Register(localizedTestRegistration("github", &testAdapter{driverID: "github-rest"}, []ActionDefinition{action})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true}, nil
		},
	))
	request := skills.ToolGovernanceRequest{
		Manifest:     toolgovernance.Manifest{ToolID: "integration.execute_dynamic"},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   MetaProviderExternalIntegrations,
		ToolName:     "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": "github", "action_id": action.ID, "connection_selector": "preferred",
			"arguments": map[string]interface{}{},
		},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID, UserID: testUserID, InvokeFrom: tools.ToolInvokeFromAIChat,
			RuntimeParameters: map[string]interface{}{
				"integration_selected_connection_ids": map[string][]string{"github": {testConnectionID}},
				"integration_connection_ids":          map[string]string{"github": testConnectionID},
			},
		},
	}
	if _, err := resolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("unresolved preferred selector error = %v, code = %q", err, ErrorCode(err))
	}
	request.Arguments["connection_id"] = testConnectionID
	if _, err := resolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("ambiguous connection selector error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestGovernanceResolverSessionGrantIsScopedToIntegrationAndConnection(t *testing.T) {
	action := testAction("record.create", "create_record")
	action.Effect = toolgovernance.EffectCreate
	action.RiskLevel = toolgovernance.RiskLevelMedium
	action.ExternalDestination = "shared-api.example.com"
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
	registry := NewRegistry()
	for _, registration := range []Registration{
		localizedTestRegistration("provider-a", &testAdapter{driverID: "provider-a-rest"}, []ActionDefinition{action}),
		localizedTestRegistration("provider-b", &testAdapter{driverID: "provider-b-rest"}, []ActionDefinition{action}),
	} {
		if err := registry.Register(registration); err != nil {
			t.Fatalf("Register(%s) error = %v", registration.IntegrationID, err)
		}
	}
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true}, nil
		},
	))
	gateway := skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).WithManifestResolver(resolver)
	connectionA := testConnectionID
	connectionB := "55555555-5555-4555-8555-555555555555"
	conversationID := "33333333-3333-4333-8333-333333333333"
	selected := map[string][]string{
		"provider-a": {connectionA, connectionB},
		// Reusing the ID here deliberately isolates the provider-identity
		// boundary. Production preferences additionally ensure a Connection
		// belongs to exactly one integration.
		"provider-b": {connectionA},
	}
	request := func(integrationID, connectionID string, grants []toolgovernance.SessionGrant) skills.ToolGovernanceRequest {
		runtimeParameters := map[string]interface{}{
			"integration_selected_connection_ids": selected,
			"tool_governance_permission_tier":     "basic",
		}
		if len(grants) > 0 {
			runtimeParameters["tool_governance_session_grants"] = grants
		}
		return skills.ToolGovernanceRequest{
			Manifest: toolgovernance.Manifest{
				ToolID: "integration.execute_dynamic", SkillID: skills.SkillExternalApps, Domain: "external_integration",
				Effect: toolgovernance.EffectRead, AssetType: "integration_connection", RiskLevel: toolgovernance.RiskLevelLow,
				PermissionScopes: []string{"integration:dynamic:execute"}, DefaultApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk,
				AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
			},
			ProviderType: tools.ToolProviderTypeConnector,
			ProviderID:   MetaProviderExternalIntegrations,
			SkillID:      skills.SkillExternalApps,
			ToolName:     "execute_action",
			Arguments: map[string]interface{}{
				"integration_id": integrationID, "action_id": action.ID, "connection_id": connectionID,
				"arguments": map[string]interface{}{"query": "safe value"},
			},
			ExecutionContext: skills.ExecutionContext{
				OrganizationID: testOrganizationID, UserID: testUserID, ConversationID: conversationID,
				InvokeFrom: tools.ToolInvokeFromAIChat, RuntimeParameters: runtimeParameters,
			},
		}
	}

	approved, err := gateway.DecideSkillTool(context.Background(), request("provider-a", connectionA, nil))
	if err != nil {
		t.Fatalf("DecideSkillTool(provider-a/connection-a) error = %v", err)
	}
	if approved.Status != toolgovernance.DecisionStatusNeedsApproval || approved.ApprovalEvent == nil {
		t.Fatalf("initial decision = %#v", approved)
	}
	if approved.Manifest.ToolID != "provider-a:record.create" || !approved.Manifest.RequiresAssetResolution {
		t.Fatalf("provider-scoped manifest = %#v", approved.Manifest)
	}
	if len(approved.Assets) != 1 || approved.Assets[0].Type != "integration_connection" || approved.Assets[0].ID != connectionA || approved.Assets[0].Metadata["integration_id"] != "provider-a" {
		t.Fatalf("connection-scoped assets = %#v", approved.Assets)
	}
	grant := approved.ApprovalEvent.Grant

	sameConnection, err := gateway.DecideSkillTool(context.Background(), request("provider-a", connectionA, []toolgovernance.SessionGrant{grant}))
	if err != nil {
		t.Fatalf("DecideSkillTool(same connection) error = %v", err)
	}
	if sameConnection.Status != toolgovernance.DecisionStatusAllowed {
		t.Fatalf("same-connection session grant decision = %#v", sameConnection)
	}

	differentConnection, err := gateway.DecideSkillTool(context.Background(), request("provider-a", connectionB, []toolgovernance.SessionGrant{grant}))
	if err != nil {
		t.Fatalf("DecideSkillTool(different connection) error = %v", err)
	}
	if differentConnection.Status != toolgovernance.DecisionStatusNeedsApproval {
		t.Fatalf("connection-a grant authorized connection-b: %#v", differentConnection)
	}

	differentProvider, err := gateway.DecideSkillTool(context.Background(), request("provider-b", connectionA, []toolgovernance.SessionGrant{grant}))
	if err != nil {
		t.Fatalf("DecideSkillTool(different provider) error = %v", err)
	}
	if differentProvider.Status != toolgovernance.DecisionStatusNeedsApproval {
		t.Fatalf("provider-a grant authorized provider-b: %#v", differentProvider)
	}
}

func TestManifestForIntegrationActionFallsBackToPerInvocationApprovalWithoutConnectionIdentity(t *testing.T) {
	action := testAction("record.create", "create_record")
	action.Effect = toolgovernance.EffectCreate
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
	manifest := manifestForIntegrationAction(toolgovernance.Manifest{}, "provider-a", "", action)
	if !manifest.ApprovalEveryInvocation || manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("unbound action did not fail closed: %#v", manifest)
	}
	if manifest.RequiresAssetResolution {
		t.Fatalf("unbound action claimed a resolvable connection asset: %#v", manifest)
	}
}

func TestGovernanceResolverMetaExecuteFailsClosedForUnknownOrUnselectedAction(t *testing.T) {
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), &testAdapter{driverID: DriverExa})
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			return ActionPolicyDecision{Enabled: true, DataEgressAllowed: true}, nil
		},
	))
	request := skills.ToolGovernanceRequest{
		Manifest:     toolgovernance.Manifest{ToolID: "integration.execute_dynamic"},
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   MetaProviderExternalIntegrations,
		ToolName:     "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": IntegrationWebSearch, "action_id": "missing.action", "connection_id": testConnectionID,
			"arguments": map[string]interface{}{},
		},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID, UserID: testUserID, InvokeFrom: tools.ToolInvokeFromAIChat,
			RuntimeParameters: map[string]interface{}{
				"integration_connection_ids": map[string]string{IntegrationWebSearch: testConnectionID},
			},
		},
	}
	if _, err := resolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("unknown action error = %v, code = %q", err, ErrorCode(err))
	}
	request.Arguments["action_id"] = ActionWebSearch
	request.Arguments["connection_id"] = "55555555-5555-4555-8555-555555555555"
	request.ExecutionContext.RuntimeParameters["integration_selected_connection_ids"] = map[string][]string{
		IntegrationWebSearch: {testConnectionID},
	}
	request.ExecutionContext.RuntimeParameters["integration_connection_ids"] = map[string]string{
		IntegrationWebSearch: "55555555-5555-4555-8555-555555555555",
	}
	if _, err := resolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("unselected connection error = %v, code = %q", err, ErrorCode(err))
	}
	request.Arguments["connection_id"] = testConnectionID
	request.Arguments["catalog_revision"] = "sha256:stale"
	if _, err := resolver.ResolveToolGovernanceManifest(context.Background(), request); ErrorCode(err) != ErrorCodePolicyConflict {
		t.Fatalf("stale catalog error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestMetaConnectionSelectedTreatsExplicitEmptyFullSetAsAuthoritative(t *testing.T) {
	connectionID := "4a5a8a62-4a9e-4cc7-8fbd-1009877fc728"
	parameters := map[string]interface{}{
		"integration_selected_connection_ids": map[string][]string{},
		"integration_connection_ids":          map[string]string{"github": connectionID},
	}
	if metaConnectionSelected(parameters, "github", connectionID) {
		t.Fatal("explicit empty full selection revived a stale preferred connection")
	}
}

func TestGovernanceResolverMetaExecuteBlocksSensitiveInputBeforeApproval(t *testing.T) {
	registry := registerTestAction(t, testAction(ActionWebSearch, "search_web"), &testAdapter{driverID: DriverExa})
	policyCalls := 0
	resolver := NewGovernanceManifestResolver(registry, executorPolicyResolverFunc(
		func(context.Context, string, string, ActionDefinition) (ActionPolicyDecision, error) {
			policyCalls++
			return ActionPolicyDecision{Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: true}, nil
		},
	))
	_, err := resolver.ResolveToolGovernanceManifest(context.Background(), skills.ToolGovernanceRequest{
		Manifest:     toolgovernance.Manifest{ToolID: "integration.execute_dynamic"},
		ProviderType: tools.ToolProviderTypeConnector, ProviderID: MetaProviderExternalIntegrations, ToolName: "execute_action",
		Arguments: map[string]interface{}{
			"integration_id": IntegrationWebSearch, "action_id": ActionWebSearch, "connection_id": testConnectionID,
			"arguments": map[string]interface{}{"query": "authorization: Bearer abcdefghijklmnopqrstuvwxyz"},
		},
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: testOrganizationID, UserID: testUserID, InvokeFrom: tools.ToolInvokeFromAIChat,
			RuntimeParameters: map[string]interface{}{
				"integration_selected_connection_ids": map[string][]string{IntegrationWebSearch: {testConnectionID}},
			},
		},
	})
	if ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("sensitive preflight error = %v, code = %q", err, ErrorCode(err))
	}
	if policyCalls != 0 {
		t.Fatalf("organization policy called %d times after sensitive preflight", policyCalls)
	}
}
