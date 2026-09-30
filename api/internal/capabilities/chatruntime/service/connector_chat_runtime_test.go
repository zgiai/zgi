package service

import (
	"context"
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
