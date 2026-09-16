package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestUnboundConversationHasNoConnectorCapability(t *testing.T) {
	t.Parallel()

	resolver := &chatRuntimeResolverStub{}
	svc := &service{connectorChatRuntime: resolver}
	parts := &chatRequestParts{ConversationID: uuid.NewString()}
	err := svc.applyConnectorSkillConfig(
		context.Background(),
		Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		parts,
	)
	if err != nil || parts.ConnectorToolsEnabled || len(parts.ConnectorSkillIDs) != 0 {
		t.Fatalf("legacy preference produced native capability: parts=%#v err=%v", parts, err)
	}
}

func TestAIChatConnectorSkillIsEnabledOnlyByConversationTargetBindingCapabilities(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.New()
	resolver := &chatRuntimeResolverStub{snapshot: connectorChatTestSnapshot(organizationID, ownerID, conversationID)}
	svc := &service{connectorChatRuntime: resolver}
	parts := &chatRequestParts{ConversationID: conversationID.String()}
	err := svc.applyConnectorSkillConfig(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: ownerID},
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		parts,
	)
	if err != nil || len(parts.ConnectorSkillIDs) != 1 || parts.ConnectorSkillIDs[0] != skills.SkillConnectorRuntime ||
		!parts.ConnectorToolsEnabled || len(resolver.capabilityRequests) != 1 {
		t.Fatalf("Connector skill parts = %#v, capability requests = %#v, err = %v", parts, resolver.capabilityRequests, err)
	}

	resolver.capabilityRequests = nil
	parts = &chatRequestParts{ConversationID: conversationID.String()}
	err = svc.applyConnectorSkillConfig(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: ownerID},
		Caller{Type: runtimemodel.ConversationCallerAgent},
		parts,
	)
	if err != nil || len(parts.ConnectorSkillIDs) != 0 || parts.ConnectorToolsEnabled || len(resolver.capabilityRequests) != 0 {
		t.Fatalf("Agent caller inherited Chat Connector capabilities: parts = %#v, requests = %#v, err = %v", parts, resolver.capabilityRequests, err)
	}
}

func TestAIChatConnectorOperationsAreProjectedFromFreshSnapshotWithoutLegacyProvider(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.New()
	snapshot := connectorChatTestSnapshot(organizationID, ownerID, conversationID)
	resolver := &chatRuntimeResolverStub{snapshot: snapshot}
	svc := &service{connectorChatRuntime: resolver}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillConnectorRuntime},
		Tools: []skills.SkillToolDefinition{{
			Name:         "legacy_tool_without_provider_schema",
			ProviderType: tools.ToolProviderTypeConnector,
			ProviderID:   "external-integrations",
		}},
	}}}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		Scope:        Scope{OrganizationID: organizationID, AccountID: ownerID},
		Caller:       Caller{Type: runtimemodel.ConversationCallerAIChat},
	}

	projected, err := projectConnectorOperations(resolved, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := projected.Get(skills.SkillConnectorRuntime)
	if !ok || len(doc.Tools) != 1 {
		t.Fatalf("projected external-apps document = %#v", doc)
	}
	operation := snapshot.Capabilities[0].Operations[0]
	definition := doc.Tools[0]
	if definition.Name != operation.ToolName || definition.ProviderID != snapshot.Capabilities[0].ConnectorID.String() ||
		definition.ProviderType != tools.ToolProviderTypeConnector || !definition.ConnectorNative ||
		definition.ConnectorOperationID != operation.ID || len(definition.InputSchema) == 0 {
		t.Fatalf("projected Connector tool definition = %#v", definition)
	}

	execution := svc.skillExecutionContext(prepared)
	if execution.ConnectorIdentityResolver != resolver || execution.InvokeFrom != tools.ToolInvokeFromAIChat {
		t.Fatalf("AI Chat execution context = %#v", execution)
	}
	prepared.Caller.Type = runtimemodel.ConversationCallerAgent
	prepared.RunConfig.BillingAppID = uuid.NewString()
	execution = svc.skillExecutionContext(prepared)
	if execution.ConnectorIdentityResolver != nil || execution.InvokeFrom != tools.ToolInvokeFromAgent {
		t.Fatalf("Agent execution context inherited Chat resolver = %#v", execution)
	}
}

func TestConnectorOnlyChatEntersNativeToolLoop(t *testing.T) {
	t.Parallel()

	parts := &chatRequestParts{
		ModelSupportsFunctionCalling: true,
		ConnectorToolsEnabled:        true,
		ConnectorSkillIDs:            []string{skills.SkillConnectorRuntime},
		SkillMode:                    skillModeAuto,
	}
	if got := executionModeForModel(Caller{Type: runtimemodel.ConversationCallerAIChat}, parts); got != executionModeNativeToolLoop {
		t.Fatalf("executionModeForModel() = %q", got)
	}
	if err := finalizeExecutionMode(Caller{Type: runtimemodel.ConversationCallerAIChat}, parts); err != nil {
		t.Fatal(err)
	}
	if parts.ExecutionMode != executionModeNativeToolLoop || !chatPartsToolLoopEnabled(parts) || !chatPartsBusinessSkillsEnabled(parts) {
		t.Fatalf("Connector-only execution route = %#v", parts)
	}
}

func TestPreparedConnectorProjectionRechecksDetachAtRunBoundary(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.New()
	initial := connectorChatTestSnapshot(organizationID, ownerID, conversationID)
	resolver := &chatRuntimeResolverStub{snapshots: []connectorruntimeauth.ChatCapabilitySnapshot{
		initial,
		{}, // the binding was detached after prepare and before the Tool loop
	}}
	svc := &service{connectorChatRuntime: resolver}
	parts := &chatRequestParts{ConversationID: conversationID.String()}
	if err := svc.applyConnectorSkillConfig(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: ownerID},
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		parts,
	); err != nil {
		t.Fatal(err)
	}
	if !parts.ConnectorToolsEnabled {
		t.Fatal("prepare boundary did not see the attached Connector")
	}
	resolved, err := svc.resolvePreparedBusinessSkills(context.Background(), &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Scope:        Scope{OrganizationID: organizationID, AccountID: ownerID},
		Caller:       Caller{Type: runtimemodel.ConversationCallerAIChat},
		parts:        parts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil || len(resolved.Skills) != 0 || parts.ConnectorToolsEnabled ||
		len(parts.ConnectorSkillIDs) != 0 || len(parts.SkillIDs) != 0 {
		t.Fatalf("detached Connector survived run boundary: resolved=%#v parts=%#v", resolved, parts)
	}
}

func TestConnectorProjectionFailsClosedOnToolIdentityConflict(t *testing.T) {
	t.Parallel()

	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	second := snapshot.Capabilities[0]
	second.BindingID = uuid.New()
	second.ConnectionID = uuid.New()
	second.ConnectorID = core.Identifier("exa")
	second.Definition.ID = "exa"
	second.Definition.ProviderID = "exa"
	second.Operations[0].ID = "exa.search"
	snapshot.Capabilities = append(snapshot.Capabilities, second)

	_, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceChat)
	if err == nil || !strings.Contains(err.Error(), "multiple provider or operation identities") {
		t.Fatalf("conflicting projection error = %v", err)
	}
}

func TestConnectorRuntimeOptionalDependencyIsInjected(t *testing.T) {
	t.Parallel()

	resolver := &chatRuntimeResolverStub{}
	svc := NewServiceWithSkillRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, resolver).(*service)
	if svc.connectorChatRuntime != resolver {
		t.Fatalf("Connector runtime dependency = %#v", svc.connectorChatRuntime)
	}
}

func TestDigitalHumanConnectorCapabilitiesResolveByHostScopeAndVersion(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ownerID := uuid.New()
	digitalHumanID := uuid.New()
	versionID := uuid.NewString()
	dhResolver := &digitalHumanRuntimeResolverStub{snapshot: digitalHumanConnectorTestSnapshot(uuid.NewString())}
	chatResolver := &chatRuntimeResolverStub{}
	agentResolver := &connectorNativeLoopAgentResolver{}
	svc := &service{
		connectorChatRuntime:         chatResolver,
		connectorAgentRuntime:        agentResolver,
		connectorDigitalHumanRuntime: dhResolver,
	}
	parts := &chatRequestParts{ConnectorTargetScope: "published", ConnectorTargetVersionID: versionID}
	err := svc.applyConnectorSkillConfig(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: ownerID},
		Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID},
		parts,
	)
	if err != nil {
		t.Fatalf("applyConnectorSkillConfig() error = %v", err)
	}
	if len(dhResolver.requests) != 1 {
		t.Fatalf("DigitalHuman resolver requests = %#v", dhResolver.requests)
	}
	request := dhResolver.requests[0]
	if request.OrganizationID != organizationID || request.OwnerAccountID != ownerID ||
		request.DigitalHumanID != digitalHumanID.String() ||
		request.Scope != core.ScopePublished || request.VersionID != versionID {
		t.Fatalf("DigitalHuman capability request = %#v", request)
	}
	if len(chatResolver.capabilityRequests) != 0 || len(agentResolver.requests) != 0 {
		t.Fatalf("DigitalHuman resolution reached another runtime: chat=%#v agent=%#v", chatResolver.capabilityRequests, agentResolver.requests)
	}
	if !parts.ConnectorToolsEnabled || len(parts.ConnectorSkillIDs) != 1 || parts.ConnectorSkillIDs[0] != skills.SkillConnectorRuntime {
		t.Fatalf("DigitalHuman projection = %#v", parts)
	}
}

func TestDigitalHumanConnectorCapabilitiesRequireHostIdentity(t *testing.T) {
	t.Parallel()

	dhResolver := &digitalHumanRuntimeResolverStub{snapshot: digitalHumanConnectorTestSnapshot(uuid.NewString())}
	svc := &service{connectorDigitalHumanRuntime: dhResolver}
	parts := &chatRequestParts{ConnectorTargetScope: "published", ConnectorTargetVersionID: uuid.NewString()}
	err := svc.applyConnectorSkillConfig(
		context.Background(),
		Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		Caller{Type: runtimemodel.ConversationCallerDigitalHuman},
		parts,
	)
	if err != nil {
		t.Fatalf("applyConnectorSkillConfig() error = %v", err)
	}
	if len(dhResolver.requests) != 0 || parts.ConnectorToolsEnabled || len(parts.ConnectorSkillIDs) != 0 {
		t.Fatalf("unidentified DigitalHuman resolved capabilities: requests=%#v parts=%#v", dhResolver.requests, parts)
	}
}

func TestDigitalHumanConnectorTargetNeverFallsBackToChat(t *testing.T) {
	t.Parallel()

	digitalHumanID := uuid.New()
	parts := &chatRequestParts{ConversationID: uuid.NewString()}
	targetID, surface, ok := connectorTargetForCaller(
		Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID}, parts)
	if !ok || targetID != digitalHumanID.String() || surface != catalog.SurfaceDigitalHuman {
		t.Fatalf("DigitalHuman target = %q/%q/%v", targetID, surface, ok)
	}
	if targetID, _, ok := connectorTargetForCaller(Caller{Type: runtimemodel.ConversationCallerDigitalHuman}, parts); ok || targetID != "" {
		t.Fatalf("unidentified DigitalHuman produced target %q/%v", targetID, ok)
	}
}

func TestDigitalHumanConnectorProjectionPinsEachBindingInstance(t *testing.T) {
	t.Parallel()

	firstInstance, secondInstance := uuid.NewString(), uuid.NewString()
	snapshot := digitalHumanConnectorTestSnapshot(firstInstance, secondInstance)
	projected, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceDigitalHuman)
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := projected.Get(skills.SkillConnectorRuntime)
	if !ok || len(doc.Tools) != 2 {
		t.Fatalf("projected DigitalHuman document = %#v", doc)
	}
	if !skills.SkillSupportsCaller(doc.Metadata.SupportedCallers, skills.SkillCallerDigitalHuman) {
		t.Fatalf("DigitalHuman Skill callers = %#v", doc.Metadata.SupportedCallers)
	}
	operationToolName := snapshot.Capabilities[0].Operations[0].ToolName
	byInstance := map[string]string{}
	for _, definition := range doc.Tools {
		if !definition.ConnectorNative || definition.ConnectorBindingInstanceID == "" {
			t.Fatalf("projected tool lost its binding instance = %#v", definition)
		}
		// The suffix is derived from the definition's own instance id, so a Tool
		// name can never drift away from the binding it is pinned to.
		digest := sha256.Sum256([]byte(definition.ConnectorBindingInstanceID))
		wantName := fmt.Sprintf("%s_%x", operationToolName, digest[:6])
		if definition.Name != wantName {
			t.Fatalf("Tool %q does not pin binding instance %q", definition.Name, definition.ConnectorBindingInstanceID)
		}
		if _, duplicate := byInstance[definition.ConnectorBindingInstanceID]; duplicate {
			t.Fatalf("duplicate binding instance %q", definition.ConnectorBindingInstanceID)
		}
		byInstance[definition.ConnectorBindingInstanceID] = definition.Name
	}
	if _, found := byInstance[firstInstance]; !found {
		t.Fatalf("first binding instance not projected: %#v", byInstance)
	}
	if _, found := byInstance[secondInstance]; !found {
		t.Fatalf("second binding instance not projected: %#v", byInstance)
	}
	if byInstance[firstInstance] == byInstance[secondInstance] {
		t.Fatalf("binding instances share Tool name %q", byInstance[firstInstance])
	}
}

// Chat and Agent keep exactly one active binding per Connector, so a capability
// that unexpectedly carries a persisted instance must never rename their public
// Tool or leak that instance into the definition identity.
func TestChatAndAgentConnectorProjectionKeepOriginalToolNameWithBindingInstance(t *testing.T) {
	t.Parallel()

	instanceID := uuid.NewString()
	for _, test := range []struct {
		name       string
		surface    catalog.Surface
		targetType core.TargetType
	}{
		{name: "chat", surface: catalog.SurfaceChat, targetType: core.TargetTypeChat},
		{name: "agent", surface: catalog.SurfaceAgent, targetType: core.TargetTypeAgent},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
			capability := &snapshot.Capabilities[0]
			capability.Definition.Surfaces = []catalog.Surface{test.surface}
			capability.Target = core.TargetRef{Type: test.targetType, ID: uuid.NewString(), BindingInstanceID: instanceID}

			projected, err := projectConnectorOperations(nil, snapshot, test.surface)
			if err != nil {
				t.Fatal(err)
			}
			doc, ok := projected.Get(skills.SkillConnectorRuntime)
			if !ok || len(doc.Tools) != 1 {
				t.Fatalf("projected %s document = %#v", test.surface, doc)
			}
			wantName := capability.Operations[0].ToolName
			definition := doc.Tools[0]
			if definition.Name != wantName {
				t.Fatalf("%s Tool name = %q, want the original %q", test.surface, definition.Name, wantName)
			}
			if definition.ConnectorBindingInstanceID != "" {
				t.Fatalf("%s Tool leaked a binding instance = %#v", test.surface, definition)
			}
		})
	}
}

func TestDigitalHumanRuntimeOptionalDependencyIsInjected(t *testing.T) {
	t.Parallel()

	resolver := &digitalHumanRuntimeResolverStub{}
	svc := NewServiceWithSkillRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, resolver).(*service)
	if svc.connectorDigitalHumanRuntime != resolver {
		t.Fatalf("DigitalHuman runtime dependency = %#v", svc.connectorDigitalHumanRuntime)
	}
}

// digitalHumanConnectorTestSnapshot builds one DigitalHuman capability per
// persisted binding instance of the same Connector and operation.
func digitalHumanConnectorTestSnapshot(instanceIDs ...string) connectorruntimeauth.ChatCapabilitySnapshot {
	objectSchema := map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
	}
	operation := catalog.Operation{
		ID: "github.user.get", ToolName: "get_github_user", SkillID: skills.SkillConnectorRuntime,
		Name: "Get GitHub user", Description: "Read the connected GitHub account.",
		InputSchema: objectSchema, OutputSchema: objectSchema,
		Effect: catalog.OperationEffectRead, RiskHint: catalog.OperationRiskHintLow, Idempotent: true,
	}
	definition := catalog.Definition{
		ID: "github", Surfaces: []catalog.Surface{catalog.SurfaceDigitalHuman}, Operations: []catalog.Operation{operation},
	}
	capabilities := make([]connectorruntimeauth.ChatCapability, 0, len(instanceIDs))
	for _, instanceID := range instanceIDs {
		capabilities = append(capabilities, connectorruntimeauth.ChatCapability{
			Target:       core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(), BindingInstanceID: instanceID},
			BindingID:    uuid.New(),
			ConnectionID: uuid.New(), ConnectorID: core.Identifier(definition.ID),
			Definition: definition, Operations: []catalog.Operation{operation},
		})
	}
	return connectorruntimeauth.ChatCapabilitySnapshot{Capabilities: capabilities}
}

type digitalHumanRuntimeResolverStub struct {
	snapshot         connectorruntimeauth.ChatCapabilitySnapshot
	requests         []connectorruntimeauth.DigitalHumanCapabilityRequest
	identityRequests []connectordispatch.IdentityResolutionRequest
}

func (stub *digitalHumanRuntimeResolverStub) ResolveDigitalHumanCapabilities(
	_ context.Context,
	request connectorruntimeauth.DigitalHumanCapabilityRequest,
) (connectorruntimeauth.ChatCapabilitySnapshot, error) {
	stub.requests = append(stub.requests, request)
	return stub.snapshot, nil
}

func (stub *digitalHumanRuntimeResolverStub) ResolveConnectorIdentity(
	_ context.Context,
	request connectordispatch.IdentityResolutionRequest,
) (connectordispatch.Identity, error) {
	stub.identityRequests = append(stub.identityRequests, request)
	return connectordispatch.Identity{}, nil
}

func connectorChatTestSnapshot(
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	conversationID uuid.UUID,
) connectorruntimeauth.ChatCapabilitySnapshot {
	objectSchema := map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
	}
	operation := catalog.Operation{
		ID: "github.user.get", ToolName: "get_github_user", SkillID: skills.SkillConnectorRuntime,
		Name: "Get GitHub user", Description: "Read the connected GitHub account.",
		InputSchema: objectSchema, OutputSchema: objectSchema,
		Effect: catalog.OperationEffectRead, RiskHint: catalog.OperationRiskHintLow, Idempotent: true,
	}
	definition := catalog.Definition{
		ID: "github", Surfaces: []catalog.Surface{catalog.SurfaceChat}, Operations: []catalog.Operation{operation},
	}
	return connectorruntimeauth.ChatCapabilitySnapshot{Capabilities: []connectorruntimeauth.ChatCapability{{
		BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier(definition.ID),
		Definition: definition, Operations: []catalog.Operation{operation},
	}}}
}

type chatRuntimeResolverStub struct {
	snapshot           connectorruntimeauth.ChatCapabilitySnapshot
	snapshots          []connectorruntimeauth.ChatCapabilitySnapshot
	capabilityRequests []connectorruntimeauth.ChatCapabilityRequest
	identityRequests   []connectordispatch.IdentityResolutionRequest
}

func (stub *chatRuntimeResolverStub) ResolveChatCapabilities(
	_ context.Context,
	request connectorruntimeauth.ChatCapabilityRequest,
) (connectorruntimeauth.ChatCapabilitySnapshot, error) {
	stub.capabilityRequests = append(stub.capabilityRequests, request)
	if len(stub.snapshots) > 0 {
		snapshot := stub.snapshots[0]
		stub.snapshots = stub.snapshots[1:]
		return snapshot, nil
	}
	return stub.snapshot, nil
}

func (stub *chatRuntimeResolverStub) ResolveConnectorIdentity(
	_ context.Context,
	request connectordispatch.IdentityResolutionRequest,
) (connectordispatch.Identity, error) {
	stub.identityRequests = append(stub.identityRequests, request)
	return connectordispatch.Identity{}, nil
}
