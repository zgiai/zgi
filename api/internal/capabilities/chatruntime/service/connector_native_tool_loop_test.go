package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	toolprovider "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// Exercise the service-to-model boundary: a prepared flag or a populated Skill
// catalog alone does not make an attached Connector callable by the model.
func TestConnectorNativeToolLoopFirstRequestUsesCurrentTargetBindings(t *testing.T) {
	for _, callerType := range []string{runtimemodel.ConversationCallerAIChat, runtimemodel.ConversationCallerAgent} {
		for _, stream := range []bool{false, true} {
			for _, bindingState := range []string{"attached", "unbound", "detached_before_run"} {
				name := fmt.Sprintf("%s/stream=%t/%s", callerType, stream, bindingState)
				t.Run(name, func(t *testing.T) {
					ctx := context.Background()
					definition := connectorNativeLoopWebSearchDefinition(t)
					attached := connectorruntimeauth.ChatCapabilitySnapshot{Capabilities: []connectorruntimeauth.ChatCapability{{
						BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier(definition.ID),
						Definition: definition, Operations: definition.Operations,
					}}}
					initial, current := attached, attached
					if bindingState == "unbound" {
						initial = connectorruntimeauth.ChatCapabilitySnapshot{}
					}
					if bindingState != "attached" {
						current = connectorruntimeauth.ChatCapabilitySnapshot{}
					}
					chatResolver := &chatRuntimeResolverStub{snapshots: []connectorruntimeauth.ChatCapabilitySnapshot{initial, current}}
					agentResolver := &connectorNativeLoopAgentResolver{snapshots: []connectorruntimeauth.ChatCapabilitySnapshot{initial, current}}
					llm := &toolGovernanceStreamLLM{
						appChatResponses: []*adapter.ChatResponse{postVerifierServiceTestChatResponse("Test response.")},
						streamChunks:     []string{"Test response."},
					}
					catalogDir := t.TempDir()
					writeConnectorNativeLoopSkill(t, catalogDir, skills.SkillConnectorRuntime, "Connected operations.")
					const ordinarySkillID = "ordinary-candidate"
					const ordinaryInstructions = "ORDINARY_SKILL_BODY_MUST_NOT_BE_PRELOADED"
					writeConnectorNativeLoopSkill(t, catalogDir, ordinarySkillID, ordinaryInstructions)
					runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir)
					svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil, chatResolver, agentResolver).(*service)

					conversationID, messageID, agentID := uuid.New(), uuid.New(), uuid.New()
					scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}
					caller := Caller{Type: callerType}
					mode := executionModeNativeToolLoop
					if callerType == runtimemodel.ConversationCallerAgent {
						caller.ID = &agentID
						mode = executionModeNativeAgentLoop
					}
					provider := "test-provider"
					if stream {
						provider = "qwen"
					}
					parts := &chatRequestParts{
						ConversationID: conversationID.String(),
						Query:          "帮我用websearch查询一下西安9.2日天气",
						Surface:        aiChatSurfaceWorkChat,
						Provider:       provider,
						SkillMode:      skillModeAuto,
						SkillIDs:       []string{ordinarySkillID},
						ExecutionMode:  mode,
					}
					if err := svc.applyConnectorSkillConfig(ctx, scope, caller, parts); err != nil {
						t.Fatalf("prepare Connector configuration: %v", err)
					}
					if parts.ConnectorToolsEnabled != (bindingState != "unbound") {
						t.Fatalf("prepare did not observe the initial binding state: enabled=%t", parts.ConnectorToolsEnabled)
					}
					prepared := &PreparedChat{
						Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: scope.OrganizationID, AccountID: scope.AccountID},
						Message: &runtimemodel.Message{
							ID: messageID, ConversationID: conversationID,
							Metadata: streamingMessageMetadataWithTaskID(parts, messageID.String()),
						},
						LLMRequest: &adapter.ChatRequest{
							Provider: provider, Model: "test-model",
							Messages: []adapter.Message{{Role: "user", Content: parts.Query}},
						},
						Scope: scope, Caller: caller, parts: parts,
					}
					var answer string
					var err error
					if stream {
						answer, _, err = svc.runPreparedToolStream(ctx, ctx, prepared, func(string) error { return nil }, nil)
					} else {
						answer, _, err = svc.runPreparedToolLoop(ctx, ctx, prepared, nil, nil)
					}
					if err != nil {
						t.Fatalf("run prepared native tool loop: %v", err)
					}
					if answer != "Test response." {
						t.Fatalf("answer = %q, want the fake model response", answer)
					}
					requests, otherRequests := llm.appChatRequests, llm.streamRequests
					if stream {
						requests, otherRequests = llm.streamRequests, llm.appChatRequests
					}
					if len(requests) != 1 || len(otherRequests) != 0 {
						t.Fatalf("outbound calls: selected transport=%d, other=%d; want exactly the first model request without a discovery round", len(requests), len(otherRequests))
					}
					first := requests[0]
					for _, operation := range definition.Operations {
						connectorNativeLoopAssertToolSchema(t, first, operation, bindingState == "attached")
					}
					for _, message := range first.Messages {
						if strings.Contains(stringFromAny(message.Content), ordinaryInstructions) {
							t.Fatal("direct Connector projection also activated an unrelated ordinary Skill")
						}
					}
					if callerType == runtimemodel.ConversationCallerAIChat {
						want := connectorruntimeauth.ChatCapabilityRequest{OrganizationID: scope.OrganizationID, OwnerAccountID: scope.AccountID, ConversationID: conversationID.String()}
						if !reflect.DeepEqual(chatResolver.capabilityRequests, []connectorruntimeauth.ChatCapabilityRequest{want, want}) || len(agentResolver.requests) != 0 {
							t.Fatalf("Chat did not re-read its owner-scoped target: chat=%#v agent=%#v", chatResolver.capabilityRequests, agentResolver.requests)
						}
					} else {
						want := connectorruntimeauth.AgentCapabilityRequest{OrganizationID: scope.OrganizationID, OwnerAccountID: scope.AccountID, AgentID: agentID.String()}
						if !reflect.DeepEqual(agentResolver.requests, []connectorruntimeauth.AgentCapabilityRequest{want, want}) || len(chatResolver.capabilityRequests) != 0 {
							t.Fatalf("Agent did not re-read its owner-scoped target: agent=%#v chat=%#v", agentResolver.requests, chatResolver.capabilityRequests)
						}
					}
					if len(chatResolver.identityRequests) != 0 || len(agentResolver.identityRequests) != 0 {
						t.Fatal("schema-only regression unexpectedly dispatched a Connector operation")
					}
				})
			}
		}
	}
}

func TestConnectorNativeToolLoopDispatchesDirectModelCallWithResolvedIdentity(t *testing.T) {
	for _, scenario := range []struct {
		status    core.OutcomeStatus
		withError bool
	}{{core.OutcomeStatusSuccessful, false}, {core.OutcomeStatusFailed, false}, {core.OutcomeStatusUnknown, false}, {core.OutcomeStatusFailed, true}, {core.OutcomeStatusUnknown, true}} {
		outcomeStatus := scenario.status
		for _, callerType := range []string{runtimemodel.ConversationCallerAIChat, runtimemodel.ConversationCallerAgent} {
			for _, budgetPruned := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/error=%t/budget_pruned=%t", callerType, outcomeStatus, scenario.withError, budgetPruned), func(t *testing.T) {
					ctx := context.Background()
					definition := connectorNativeLoopWebSearchDefinition(t)
					if budgetPruned {
						// Keep the owning Skill and search operation active while making
						// only fetch too large for the schema budget. A guessed fetch call
						// must not fall back to the original, unpruned resolved document.
						for index := range definition.Operations {
							if definition.Operations[index].ToolName == "fetch_webpage" {
								definition.Operations[index].InputSchema["description"] = strings.Repeat("Detailed webpage fetch documentation. ", 4000)
							}
						}
					}
					bindingID, connectionID := uuid.New(), uuid.New()
					snapshot := connectorruntimeauth.ChatCapabilitySnapshot{Capabilities: []connectorruntimeauth.ChatCapability{{
						BindingID: bindingID, ConnectionID: connectionID, ConnectorID: core.Identifier(definition.ID),
						Definition: definition, Operations: definition.Operations,
					}}}
					chatResolver := &connectorNativeLoopChatIdentityResolver{
						chatRuntimeResolverStub: &chatRuntimeResolverStub{snapshot: snapshot},
						bindingID:               bindingID,
						connectionID:            connectionID,
					}
					agentResolver := &connectorNativeLoopAgentResolver{
						snapshots: []connectorruntimeauth.ChatCapabilitySnapshot{snapshot, snapshot},
						bindingID: bindingID, connectionID: connectionID,
					}
					dispatcher := &connectorNativeLoopDispatcher{outcomeStatus: outcomeStatus}
					if scenario.withError {
						dispatcher.err = core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
					}
					provider, err := toolprovider.NewProvider(definition)
					if err != nil {
						t.Fatal(err)
					}
					manager := tools.NewToolManager(nil)
					if err := manager.RegisterProvider(provider); err != nil {
						t.Fatal(err)
					}
					catalogDir := t.TempDir()
					writeConnectorNativeLoopSkill(t, catalogDir, skills.SkillConnectorRuntime, "Connected operations.")
					runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir).WithConnectorDispatcher(dispatcher)
					const callID = "call-connector-direct"
					toolName, arguments := "search_web", `{"query":"Xi'an weather September 2","num_results":1}`
					if budgetPruned {
						toolName, arguments = "fetch_webpage", `{"urls":["https://example.com/weather"]}`
					}
					llm := &toolGovernanceStreamLLM{appChatResponses: []*adapter.ChatResponse{
						{Choices: []adapter.Choice{{
							Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{
								ID: callID, Type: "function", Function: adapter.FunctionCall{Name: toolName, Arguments: arguments},
							}}},
							FinishReason: "tool_calls",
						}}},
						postVerifierServiceTestChatResponse("The mock search result is available."),
					}}
					ledgers := map[uuid.UUID]*connectordispatch.ActionLedger{}
					actionFactory := repository.ConnectorActionResolverFactory(func(messageID, _, _, _ uuid.UUID) connectordispatch.ActionInstanceResolver {
						if ledgers[messageID] == nil {
							ledgers[messageID] = &connectordispatch.ActionLedger{}
						}
						return func(_ context.Context, identity connectordispatch.Identity, input any) (connectordispatch.Identity, error) {
							result, _, err := connectordispatch.ResolveActionInstance(ledgers[messageID], "turn", identity.CorrelationID, identity, input)
							return result, err
						}
					})
					svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil, chatResolver, agentResolver, actionFactory).(*service)
					conversationID, messageID, agentID := uuid.New(), uuid.New(), uuid.New()
					scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}
					caller := Caller{Type: callerType, Source: runtimemodel.ConversationSourceConsole}
					target := core.TargetRef{Type: core.TargetTypeChat, ID: conversationID.String()}
					mode := executionModeNativeToolLoop
					runConfig := RunConfig{}
					if callerType == runtimemodel.ConversationCallerAgent {
						caller.ID = &agentID
						target = core.TargetRef{Type: core.TargetTypeAgent, ID: agentID.String()}
						mode = executionModeNativeAgentLoop
						runConfig.BillingAppID = agentID.String()
					}
					parts := &chatRequestParts{
						ConversationID: conversationID.String(), Query: "Use web search to find Xi'an weather.",
						Surface: aiChatSurfaceContextualSidebar, Provider: "test-provider", SkillMode: skillModeAuto, ExecutionMode: mode,
					}
					if err := svc.applyConnectorSkillConfig(ctx, scope, caller, parts); err != nil {
						t.Fatal(err)
					}
					prepared := &PreparedChat{
						Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: scope.OrganizationID, AccountID: scope.AccountID},
						Message: &runtimemodel.Message{
							ID: messageID, ConversationID: conversationID,
							Metadata: streamingMessageMetadataWithTaskID(parts, messageID.String()),
						},
						LLMRequest: &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: parts.Query}}},
						Scope:      scope, Caller: caller, RunConfig: runConfig, parts: parts,
					}
					answer, _, err := svc.runPreparedToolLoop(ctx, ctx, prepared, nil, nil)
					if budgetPruned {
						if len(llm.appChatRequests) == 0 {
							t.Fatalf("budget regression never reached the model's guessed tool call: %v", err)
						}
						for _, operation := range definition.Operations {
							connectorNativeLoopAssertToolSchema(t, llm.appChatRequests[0], operation, operation.ToolName == "search_web")
						}
						diagnostics := mapFromOperationContext(prepared.Message.Metadata["native_skill_diagnostics"])
						if !containsNativeSkillID(stringSliceFromAny(diagnostics["active_skill_ids"]), skills.SkillConnectorRuntime) {
							t.Fatal("budget regression did not keep the owning Skill active")
						}
						if len(dispatcher.invocations) != 0 || len(chatResolver.identityRequests) != 0 || len(agentResolver.identityRequests) != 0 {
							t.Fatal("model guessed a budget-pruned operation and reached native identity resolution or dispatch")
						}
						return
					}
					if err != nil {
						t.Fatalf("direct Connector model call: %v", err)
					}
					validAnswer := answer == "The mock search result is available."
					if outcomeStatus == core.OutcomeStatusFailed {
						validAnswer = strings.Contains(answer, "1 failed")
					}
					if outcomeStatus == core.OutcomeStatusUnknown {
						validAnswer = strings.Contains(answer, "1 have an unknown result")
					}
					wantRounds := 1
					if outcomeStatus == core.OutcomeStatusSuccessful {
						wantRounds = 2
					}
					if !validAnswer || len(llm.appChatRequests) != wantRounds || len(llm.streamRequests) != 0 {
						t.Fatalf("direct call did not respect native terminal boundary: answer=%q chat=%d want=%d stream=%d", answer, len(llm.appChatRequests), wantRounds, len(llm.streamRequests))
					}
					for _, operation := range definition.Operations {
						connectorNativeLoopAssertToolSchema(t, llm.appChatRequests[0], operation, true)
					}
					if len(dispatcher.invocations) != 1 {
						t.Fatalf("native dispatch count = %d, want one direct search_web invocation", len(dispatcher.invocations))
					}
					invocation := dispatcher.invocations[0]
					var wantArguments map[string]interface{}
					if err := json.Unmarshal([]byte(arguments), &wantArguments); err != nil {
						t.Fatal(err)
					}
					// ToolEngine supplies the operation's declared public default;
					// it must not inject Connector identity into business arguments.
					wantArguments["search_type"] = "auto"
					if !reflect.DeepEqual(invocation.Input, wantArguments) {
						t.Fatalf("native dispatcher did not receive business-only arguments: %#v", invocation.Input)
					}
					identity := invocation.Identity
					if identity.OrganizationID != scope.OrganizationID || identity.OwnerAccountID != scope.AccountID ||
						identity.ActingPrincipalID != scope.AccountID || identity.Target != target ||
						identity.BindingID != bindingID || identity.ConnectionID != connectionID ||
						identity.ConnectorID.String() != definition.ID || identity.OperationID.String() != "web.search" ||
						!identity.Interactive || identity.LogicalOperationID == "" || identity.CorrelationID == "" {
						t.Fatalf("native dispatch lost the server-resolved owner/target identity: %#v", identity)
					}
					identityRequests := chatResolver.identityRequests
					if callerType == runtimemodel.ConversationCallerAgent {
						identityRequests = agentResolver.identityRequests
					}
					if len(identityRequests) != 1 || identityRequests[0].Target != target || identityRequests[0].OwnerAccountID != scope.AccountID {
						t.Fatalf("runtime did not resolve current invocation identity: %#v", identityRequests)
					}
					if wantRounds == 1 {
						return
					}
					foundResult := false
					for _, message := range llm.appChatRequests[1].Messages {
						if message.Role == "tool" && message.ToolCallID == callID && strings.Contains(stringFromAny(message.Content), "mock-web-search-result") {
							foundResult = true
						}
					}
					if !foundResult {
						t.Fatal("second model request did not receive the native Connector result")
					}
				})
			}
		}
	}

}

func connectorNativeLoopWebSearchDefinition(t *testing.T) catalog.Definition {
	t.Helper()
	defs, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.ID == "web-search" {
			return d
		}
	}
	t.Fatal("Web Search contract missing")
	return catalog.Definition{}
}

func connectorNativeLoopAssertToolSchema(t *testing.T, request *adapter.ChatRequest, operation catalog.Operation, wantPresent bool) {
	t.Helper()
	var matches []adapter.Tool
	for _, tool := range request.Tools {
		if tool.Function.Name == operation.ToolName {
			matches = append(matches, tool)
		}
	}
	if !wantPresent {
		if len(matches) != 0 {
			t.Fatalf("unbound or detached operation %q survived into the first model request", operation.ToolName)
		}
		return
	}
	if len(matches) != 1 || matches[0].Type != "function" {
		t.Fatalf("first request contains %d copies of %q, want one native function; tools=%#v", len(matches), operation.ToolName, request.Tools)
	}
	wantSchema := tools.ModelVisibleJSONSchema(operation.InputSchema)
	if len(wantSchema) == 0 {
		t.Fatalf("real operation %q has no valid model-visible schema", operation.ToolName)
	}
	encoded, err := json.Marshal(matches[0].Function.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	var gotSchema map[string]interface{}
	if err := json.Unmarshal(encoded, &gotSchema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotSchema, wantSchema) {
		t.Fatalf("%s lost its admitted business schema: got=%s want=%#v", operation.ToolName, encoded, wantSchema)
	}
	if err := tools.ValidateJSONSchema(gotSchema); err != nil {
		t.Fatalf("%s outbound JSON schema is invalid: %v", operation.ToolName, err)
	}
	properties := mapFromOperationContext(gotSchema["properties"])
	for _, forbidden := range []string{"api_key", "token", "connection_id", "binding_id", "authorization"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("%s exposes runtime-owned argument %q", operation.ToolName, forbidden)
		}
	}
	if operation.ToolName == "search_web" {
		if got := mapFromOperationContext(properties["include_domains"])["type"]; got != "array" {
			t.Fatalf("search_web include_domains type = %v, want array", got)
		}
		if got := mapFromOperationContext(properties["start_published_date"])["format"]; got != "date-time" {
			t.Fatalf("search_web start_published_date format = %v, want date-time", got)
		}
	} else if operation.ToolName == "fetch_webpage" {
		if got := mapFromOperationContext(properties["urls"])["type"]; got != "array" {
			t.Fatalf("fetch_webpage urls type = %v, want array", got)
		}
	}
}

func writeConnectorNativeLoopSkill(t *testing.T, catalogDir, skillID, instructions string) {
	t.Helper()
	root := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: Test-only candidate Skill.\nwhen_to_use: Use only when explicitly requested.\nruntime_type: prompt\nsupported_callers: [aichat, agent]\n---\n\n%s\n", skillID, instructions)
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type connectorNativeLoopAgentResolver struct {
	snapshots        []connectorruntimeauth.ChatCapabilitySnapshot
	requests         []connectorruntimeauth.AgentCapabilityRequest
	identityRequests []connectordispatch.IdentityResolutionRequest
	bindingID        uuid.UUID
	connectionID     uuid.UUID
}

func (stub *connectorNativeLoopAgentResolver) ResolveAgentCapabilities(_ context.Context, request connectorruntimeauth.AgentCapabilityRequest) (connectorruntimeauth.ChatCapabilitySnapshot, error) {
	stub.requests = append(stub.requests, request)
	if len(stub.snapshots) == 0 {
		return connectorruntimeauth.ChatCapabilitySnapshot{}, nil
	}
	snapshot := stub.snapshots[0]
	stub.snapshots = stub.snapshots[1:]
	return snapshot, nil
}

func (stub *connectorNativeLoopAgentResolver) ResolveConnectorIdentity(_ context.Context, request connectordispatch.IdentityResolutionRequest) (connectordispatch.Identity, error) {
	stub.identityRequests = append(stub.identityRequests, request)
	return connectorNativeLoopResolvedIdentity(request, stub.bindingID, stub.connectionID), nil
}

type connectorNativeLoopChatIdentityResolver struct {
	*chatRuntimeResolverStub
	bindingID    uuid.UUID
	connectionID uuid.UUID
}

func (stub *connectorNativeLoopChatIdentityResolver) ResolveConnectorIdentity(_ context.Context, request connectordispatch.IdentityResolutionRequest) (connectordispatch.Identity, error) {
	stub.identityRequests = append(stub.identityRequests, request)
	return connectorNativeLoopResolvedIdentity(request, stub.bindingID, stub.connectionID), nil
}

func connectorNativeLoopResolvedIdentity(request connectordispatch.IdentityResolutionRequest, bindingID, connectionID uuid.UUID) connectordispatch.Identity {
	return connectordispatch.Identity{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID, ActingPrincipalID: request.ActingPrincipalID,
		BindingID: bindingID, ConnectionID: connectionID, ConnectorID: request.ConnectorID, Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID, Interactive: request.Interactive, Confirmation: request.Confirmation,
	}
}

type connectorNativeLoopDispatcher struct {
	invocations   []connectordispatch.Invocation
	outcomeStatus core.OutcomeStatus
	err           error
}

func (stub *connectorNativeLoopDispatcher) Dispatch(_ context.Context, invocation connectordispatch.Invocation) (connectorexecution.ExecutionResult, error) {
	stub.invocations = append(stub.invocations, invocation)
	return connectorexecution.ExecutionResult{
		State:   connectorexecution.ExecutionStateCompleted,
		Outcome: core.OperationOutcome{ID: uuid.New(), Status: stub.outcomeStatus},
		Value:   map[string]interface{}{"results": []interface{}{}, "test_marker": "mock-web-search-result"},
	}, stub.err
}
