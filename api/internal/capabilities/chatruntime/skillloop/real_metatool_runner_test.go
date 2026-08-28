package skillloop

import (
	"context"
	"testing"

	"github.com/google/uuid"
	integrations "github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/wecom"
	integrationmetatools "github.com/zgiai/zgi/api/internal/modules/integrations/metatools"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestRunnerProjectedAliasReceivesCompletedStatusFromRealMetaToolOrdinaryRead(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	connection := &integrations.IntegrationConnection{
		ID: organizationID, OrganizationID: organizationID,
		IntegrationID: wecom.IntegrationID, DriverID: wecom.DriverID, Name: "WeCom test",
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
		AuthType:         integrations.ConnectionAuthTypeAPIKey,
		AuthMethodID:     "api_key",
		Status:           integrations.ConnectionStatusActive,
		HealthStatus:     integrations.ConnectionHealthHealthy,
		AuthStatus:       integrations.ConnectionAuthValid,
		ScopeStatus:      integrations.ConnectionScopeVerified,
		GrantedScopes:    []string{wecom.ScopeContacts, wecom.ScopeSend},
	}
	registry := integrations.NewRegistry()
	definition := wecom.ProviderDefinition()
	definition.AuthMethods = []integrations.AuthMethodDefinition{{
		ID: "api_key", Type: integrations.AuthMethodTypeAPIKey,
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
		Label:            "API key", LabelI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "API key", integrations.LocaleSimplifiedChinese: "API 密钥",
		},
		Available: true,
		Fields: []integrations.CredentialFieldDefinition{{
			Key: "token", Label: "Token", LabelI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Token", integrations.LocaleSimplifiedChinese: "令牌",
			},
			Input: integrations.CredentialFieldInputPassword, Required: true, Secret: true,
		}},
	}}
	for index := range definition.Actions {
		definition.Actions[index].SupportedAuthMethodIDs = []string{"api_key"}
	}
	if err := registry.Register(integrations.Registration{
		Definition: definition, Adapter: realMetaToolRunnerAdapter{},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	executor := &realMetaToolRunnerExecutor{}
	access := &realMetaToolRunnerAccess{}
	policy := &realMetaToolRunnerPolicy{}
	provider, err := integrationmetatools.NewProvider(
		registry,
		executor,
		realMetaToolRunnerConnectionLookup{connection: connection},
		access,
		policy,
	)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	executeTool, err := provider.GetTool(integrationmetatools.ToolExecuteAction)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillExternalApps, Name: "External Apps", RuntimeType: skills.SkillRuntimeTypePrompt, MaxCallsPerTurn: 4},
		Tools: []skills.SkillToolDefinition{{
			Name: integrationmetatools.ToolExecuteAction, ProviderType: provider.GetProviderType(), ProviderID: integrationmetatools.ProviderID,
			InputSchema: executeTool.GetEntity().InputSchema,
		}},
	}}}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	executionContext := skills.ExecutionContext{
		OrganizationID: organizationID.String(), UserID: accountID.String(), InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"integration_selected_connection_ids": map[string][]string{wecom.IntegrationID: {connection.ID.String()}},
			"integration_connection_ids":          map[string]string{wecom.IntegrationID: connection.ID.String()},
		},
	}
	projectionService, err := integrationmetatools.NewActionProjectionService(
		registry,
		realMetaToolRunnerConnectionLookup{connection: connection},
		access,
		policy,
	)
	if err != nil {
		t.Fatalf("NewActionProjectionService() error = %v", err)
	}
	projections, err := projectionService.ProjectActions(context.Background(), integrationmetatools.ActionProjectionRequest{
		ExecutionContext: executionContext,
		Query:            "Search WeCom contacts for Alice",
	})
	if err != nil {
		t.Fatalf("ProjectActions() error = %v", err)
	}
	var searchProjection integrationmetatools.ActionProjection
	for _, projection := range projections {
		if projection.IntegrationID == wecom.IntegrationID && projection.ActionID == wecom.ActionContactSearch {
			searchProjection = projection
			break
		}
	}
	if searchProjection.ActionID == "" {
		t.Fatalf("ProjectActions() = %#v, want %s", projections, wecom.ActionContactSearch)
	}
	preferenceCallsAfterProjection := access.preferenceCalls
	useCallsAfterProjection := access.useCalls
	searchUseCallsAfterProjection := access.usedActions[wecom.ActionContactSearch]
	toolSet := skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{{
		Name:        searchProjection.ToolName,
		NameScope:   searchProjection.IntegrationID + "/" + searchProjection.ActionID,
		Description: searchProjection.Description,
		InputSchema: searchProjection.InputSchema,
		Binding: skills.NativeToolBinding{
			SkillID:            skills.SkillExternalApps,
			ToolName:           integrationmetatools.ToolExecuteAction,
			Effect:             searchProjection.Effect,
			IntentMatched:      searchProjection.IntentMatched,
			IntentGroup:        searchProjection.IntentGroup,
			IntentTokens:       searchProjection.IntentTokens,
			BindingFingerprint: searchProjection.BindingFingerprint,
			ConnectionBinding:  skills.NativeExternalActionConnectionBindingHash(searchProjection.ConnectionID),
			ArgumentEnvelope:   "arguments",
			FixedArguments: map[string]interface{}{
				"integration_id":         searchProjection.IntegrationID,
				"action_id":              searchProjection.ActionID,
				"connection_id":          searchProjection.ConnectionID,
				"action_schema_hash":     searchProjection.SchemaHash,
				"action_schema_revision": searchProjection.SchemaRevision,
				"catalog_revision":       searchProjection.CatalogRevision,
			},
		},
	}}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added = %d, toolSet=%#v", added, toolSet)
	}
	alias := projectedExternalActionAliasForAction(t, toolSet, wecom.IntegrationID, wecom.ActionContactSearch)
	modelCall := projectedExternalActionRunnerToolCall(t, "real-meta-read", alias, map[string]interface{}{
		"query": "Alice",
	})
	call := nativeExecutionCall(modelCall, &toolSet)
	if call.Function.Name != skills.MetaToolCallSkillTool {
		t.Fatalf("nativeExecutionCall() function = %q, want %q", call.Function.Name, skills.MetaToolCallSkillTool)
	}
	wrapped, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		t.Fatalf("nativeExecutionCall() arguments error = %v", err)
	}
	executeArguments := evidenceMapFromAny(wrapped["arguments"])
	businessArguments := evidenceMapFromAny(executeArguments["arguments"])
	if numericValue(businessArguments["max_results"]) != 10 {
		t.Fatalf("projected default max_results = %#v, call=%#v", businessArguments["max_results"], call)
	}
	step := (&Runner{SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}).handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("real-meta-conversation", "real-meta-message", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		executionContext,
		0,
		map[string]int{},
		map[string]struct{}{skills.SkillExternalApps: {}},
		map[string]interface{}{},
		1,
		nil,
	)
	if step.recoverable || step.trace.Status != "success" {
		t.Fatalf("handleProgressiveSkillCall() = %#v", step)
	}
	if got := evidenceStringFromAny(step.trace.Result["operation_status"]); got != "completed" {
		t.Fatalf("real metatool operation_status = %q, trace=%#v", got, step.trace)
	}
	if numericValue(step.trace.Result["result_count"]) != 1 {
		t.Fatalf("real metatool result_count = %#v, trace=%#v", step.trace.Result["result_count"], step.trace)
	}
	providerResult := evidenceMapFromAny(step.trace.Result["provider_result"])
	if providerResult["provider"] != wecom.IntegrationID || len(executor.requests) != 1 {
		t.Fatalf("real metatool result=%#v trace=%#v requests=%#v", providerResult, step.trace, executor.requests)
	}
	if numericValue(executor.requests[0].Input["max_results"]) != 10 {
		t.Fatalf("executor max_results = %#v, request=%#v", executor.requests[0].Input["max_results"], executor.requests[0])
	}
	if preferenceCallsAfterProjection == 0 || access.useCalls <= useCallsAfterProjection ||
		access.usedActions[wecom.ActionContactSearch] <= searchUseCallsAfterProjection || access.lastUse.ActionID != wecom.ActionContactSearch {
		t.Fatalf("connection authorization calls preference=%d (after projection=%d) use=%d (after projection=%d) last=%#v",
			access.preferenceCalls, preferenceCallsAfterProjection, access.useCalls, useCallsAfterProjection, access.lastUse)
	}
	if policy.resolveCalls == 0 || !policy.resolvedActions[wecom.ActionContactSearch] {
		t.Fatalf("policy Resolve() calls=%d actions=%#v", policy.resolveCalls, policy.resolvedActions)
	}
}

type realMetaToolRunnerAdapter struct{}

func (realMetaToolRunnerAdapter) DriverID() string { return wecom.DriverID }
func (realMetaToolRunnerAdapter) Execute(context.Context, integrations.ActionRequest) (*integrations.ActionResult, error) {
	return nil, nil
}

type realMetaToolRunnerExecutor struct {
	requests []integrations.ActionRequest
}

func (executor *realMetaToolRunnerExecutor) Execute(_ context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	executor.requests = append(executor.requests, request)
	return &integrations.ActionResult{
		Output: map[string]interface{}{
			"provider": wecom.IntegrationID,
			"members": []interface{}{map[string]interface{}{
				"recipient_ref": "wm-alice", "name": "Alice", "department_ids": []interface{}{1},
			}},
		},
		ResultCount: 1, AttemptCount: 1,
	}, nil
}

type realMetaToolRunnerConnectionLookup struct {
	connection *integrations.IntegrationConnection
}

func (lookup realMetaToolRunnerConnectionLookup) GetByID(_ context.Context, organizationID, connectionID uuid.UUID) (*integrations.IntegrationConnection, error) {
	if lookup.connection == nil || lookup.connection.OrganizationID != organizationID || lookup.connection.ID != connectionID {
		return nil, integrations.ErrConnectionNotFound
	}
	copyConnection := *lookup.connection
	return &copyConnection, nil
}

type realMetaToolRunnerAccess struct {
	preferenceCalls int
	useCalls        int
	lastUse         integrations.ConnectionAccessRequest
	usedActions     map[string]int
}

func (access *realMetaToolRunnerAccess) AuthorizeConnectionPreference(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, uuid.UUID) error {
	access.preferenceCalls++
	return nil
}
func (access *realMetaToolRunnerAccess) AuthorizeConnectionUse(_ context.Context, request integrations.ConnectionAccessRequest) error {
	access.useCalls++
	access.lastUse = request
	if access.usedActions == nil {
		access.usedActions = map[string]int{}
	}
	access.usedActions[request.ActionID]++
	return nil
}

type realMetaToolRunnerPolicy struct {
	resolveCalls    int
	resolvedActions map[string]bool
}

func (policy *realMetaToolRunnerPolicy) Resolve(_ context.Context, _, _ string, action integrations.ActionDefinition) (integrations.ActionPolicyDecision, error) {
	policy.resolveCalls++
	if policy.resolvedActions == nil {
		policy.resolvedActions = map[string]bool{}
	}
	policy.resolvedActions[action.ID] = true
	return integrations.ActionPolicyDecision{Enabled: true, DataEgressAllowed: true}, nil
}
