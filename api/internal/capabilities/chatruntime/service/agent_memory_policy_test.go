package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestBuildUpstreamMessagesAppendsAgentMemoryPolicy(t *testing.T) {
	parts := &chatRequestParts{
		Query:              "hello",
		SystemPrompt:       "You are a test agent.",
		AgentMemoryEnabled: true,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Description: "User profile facts", MaxChars: 800, Enabled: true},
			{Key: "scratch", Description: "Disabled scratch memory", MaxChars: 800, Enabled: false},
		},
	}

	result, err := (&service{}).buildUpstreamMessages(context.Background(), Scope{}, nil, parts)
	if err != nil {
		t.Fatalf("buildUpstreamMessages() error = %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("buildUpstreamMessages() returned no messages")
	}
	systemPrompt, ok := result.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("system message content type = %T, want string", result.Messages[0].Content)
	}
	assertContains(t, systemPrompt, "You are a test agent.")
	assertContains(t, systemPrompt, "Agent memory is enabled for this agent.")
	assertContains(t, systemPrompt, "internal memory planner")
	assertContains(t, systemPrompt, "internal Agent memory success note")
	assertContains(t, systemPrompt, "Choose keys by semantic fit")
	assertContains(t, systemPrompt, "Never say you remembered")
	assertContains(t, systemPrompt, "no internal Agent memory success note")
	assertContains(t, systemPrompt, "- profile: User profile facts")
	assertNotContains(t, systemPrompt, "read_agent_memory")
	assertNotContains(t, systemPrompt, "user-memory")
	assertNotContains(t, systemPrompt, "read_user_memory")
	assertNotContains(t, systemPrompt, "add_user_memory")
	assertNotContains(t, systemPrompt, "scratch")
}

func TestBuildUpstreamMessagesInjectsSavedAgentMemory(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	fakeMemory := &fakeAgentMemoryContextService{
		values: []agentmemory.SlotValueResponse{
			{SlotResponse: agentmemory.SlotResponse{Key: "profile"}, Content: "The user prefers to be called Lin."},
			{SlotResponse: agentmemory.SlotResponse{Key: "standing_instructions"}, Content: "Always answer as Xiao Xue."},
			{SlotResponse: agentmemory.SlotResponse{Key: "project_context"}, Content: ""},
		},
	}
	parts := &chatRequestParts{
		Query:                "hello",
		SystemPrompt:         "You are a test agent.",
		AgentMemoryEnabled:   true,
		AgentMemoryAgentID:   agentID.String(),
		AgentMemoryUserScope: agentmemory.UserScopeAccount,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Description: "User profile facts", MaxChars: 800, Enabled: true},
			{Key: "standing_instructions", Description: "Rules", MaxChars: 800, Enabled: true},
			{Key: "scratch", Description: "Disabled scratch memory", MaxChars: 800, Enabled: false},
		},
	}

	result, err := (&service{agentMemoryService: fakeMemory}).buildUpstreamMessages(context.Background(), Scope{
		WorkspaceID: &workspaceID,
		AccountID:   accountID,
	}, nil, parts)
	if err != nil {
		t.Fatalf("buildUpstreamMessages() error = %v", err)
	}
	systemPrompt := result.Messages[0].Content.(string)
	assertContains(t, systemPrompt, "Saved Agent memory for the current user:")
	assertContains(t, systemPrompt, "The user prefers to be called Lin.")
	assertContains(t, systemPrompt, "Saved standing_instructions are binding interaction rules")
	assertContains(t, systemPrompt, "Always answer as Xiao Xue.")
	assertContains(t, systemPrompt, "Agent memory is enabled for this agent.")
	if fakeMemory.calls != 1 {
		t.Fatalf("agent memory calls = %d, want 1", fakeMemory.calls)
	}
	if fakeMemory.lastReadUser != accountID {
		t.Fatalf("agent memory read user = %s, want scope account %s", fakeMemory.lastReadUser, accountID)
	}
	if len(fakeMemory.slots) != 2 || fakeMemory.slots[0].Key != "profile" || fakeMemory.slots[1].Key != "standing_instructions" {
		t.Fatalf("agent memory slots = %#v, want enabled profile and standing_instructions", fakeMemory.slots)
	}
	metadata, ok := result.Metadata["agent_memory"].(map[string]interface{})
	if !ok {
		t.Fatalf("agent_memory metadata type = %T", result.Metadata["agent_memory"])
	}
	if metadata["available"] != true || metadata["injected"] != true || metadata["value_count"] != 2 {
		t.Fatalf("agent_memory metadata = %#v, want available/injected with two values", metadata)
	}
}

func TestBuildUpstreamMessagesUsesDedicatedAgentMemoryUserID(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	memoryUserID := uuid.New()
	fakeMemory := &fakeAgentMemoryContextService{}
	parts := &chatRequestParts{
		Query:                "hello",
		SystemPrompt:         "You are a test agent.",
		AgentMemoryEnabled:   true,
		AgentMemoryAgentID:   agentID.String(),
		AgentMemoryUserScope: agentmemory.UserScopeEndUser,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Description: "User profile facts", MaxChars: 800, Enabled: true},
		},
	}

	_, err := (&service{agentMemoryService: fakeMemory}).buildUpstreamMessages(context.Background(), Scope{
		WorkspaceID:       &workspaceID,
		AccountID:         accountID,
		AgentMemoryUserID: &memoryUserID,
	}, nil, parts)
	if err != nil {
		t.Fatalf("buildUpstreamMessages() error = %v", err)
	}
	if fakeMemory.lastReadUser != memoryUserID {
		t.Fatalf("agent memory read user = %s, want dedicated memory user %s", fakeMemory.lastReadUser, memoryUserID)
	}
}

func TestApplyRunConfigToPartsDisablesUserMemoryForAgent(t *testing.T) {
	parts := &chatRequestParts{UseMemory: true}
	applyRunConfigToParts(RunConfig{
		UseMemory:      true,
		BillingAppType: runtimemodel.ConversationCallerAgent,
	}, parts)
	if parts.UseMemory {
		t.Fatal("applyRunConfigToParts() left UseMemory=true for agent caller, want false")
	}
}

func TestRenderAgentMemoryPolicyRequiresEnabledSlot(t *testing.T) {
	got := renderAgentMemoryPolicy(&chatRequestParts{
		AgentMemoryEnabled: true,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Description: "disabled", MaxChars: 800, Enabled: false},
		},
	})
	if got != "" {
		t.Fatalf("renderAgentMemoryPolicy() = %q, want empty", got)
	}
}

func TestParseNativeAgentMemoryDecision(t *testing.T) {
	confidence := 0.92
	got, err := parseNativeAgentMemoryDecision(`{"action":"update","key":"preferences","content":"Use Mermaid for chart requests.","confidence":0.92,"reason":"durable response format"}`)
	if err != nil {
		t.Fatalf("parseNativeAgentMemoryDecision() error = %v", err)
	}
	if got.Action != "update" || got.Key != "preferences" || got.Content != "Use Mermaid for chart requests." {
		t.Fatalf("decision = %#v, want update preferences", got)
	}
	if got.Confidence == nil || *got.Confidence != confidence {
		t.Fatalf("confidence = %#v, want %v", got.Confidence, confidence)
	}
}

func legacyNativeAgentMemoryDecisionMessageRoutesPersonaToStandingInstructions(t *testing.T) {
	msg := nativeAgentMemoryDecisionStateMessage(&AgentMemoryRuntimeState{
		EnabledSlots: []AgentMemorySlotConfig{
			{Key: "standing_instructions", Description: "durable rules", MaxChars: 800, Enabled: true},
		},
	}, "以后你就是我的猫娘助手")
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("decision message content type = %T, want string", msg.Content)
	}
	assertContains(t, content, "assistant persona")
	assertContains(t, content, "standing_instructions")
	assertContains(t, content, "以后你是")
	assertContains(t, content, "你要叫我")
}

func TestNativeAgentMemoryDecisionStateMessageRoutesPersonaToStandingInstructions(t *testing.T) {
	msg := nativeAgentMemoryDecisionStateMessage(&AgentMemoryRuntimeState{
		EnabledSlots: []AgentMemorySlotConfig{
			{Key: "standing_instructions", Description: "durable rules", MaxChars: 800, Enabled: true},
		},
	}, "以后你就是我的猫娘助手")
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("decision message content type = %T, want string", msg.Content)
	}
	assertContains(t, content, "assistant persona")
	assertContains(t, content, "standing_instructions")
	assertContains(t, content, "latest_user_message")
	assertNotContains(t, content, "Chinese examples")
}

func TestParseNativeAgentMemoryDecisionLowConfidenceBecomesNone(t *testing.T) {
	got, err := parseNativeAgentMemoryDecision(`{"action":"update","key":"preferences","content":"Use Mermaid.","confidence":0.3}`)
	if err != nil {
		t.Fatalf("parseNativeAgentMemoryDecision() error = %v", err)
	}
	if got.Action != "none" {
		t.Fatalf("decision action = %q, want none", got.Action)
	}
}

func TestShouldRunNativeAgentMemoryDecisionRunsForShortTurns(t *testing.T) {
	if !shouldRunNativeAgentMemoryDecision("hello") {
		t.Fatal("shouldRunNativeAgentMemoryDecision() = false for greeting, want true")
	}
	if !shouldRunNativeAgentMemoryDecision("Can you draw charts?") {
		t.Fatal("shouldRunNativeAgentMemoryDecision() = false for capability question, want true")
	}
	if shouldRunNativeAgentMemoryDecision("   ") {
		t.Fatal("shouldRunNativeAgentMemoryDecision() = true for blank query, want false")
	}
}

func TestValidateNativeAgentMemoryDecision(t *testing.T) {
	slots := []AgentMemorySlotConfig{{Key: "preferences", MaxChars: 40, Enabled: true}}
	toolName, args, err := validateNativeAgentMemoryDecision(nativeAgentMemoryDecision{
		Action:  "update",
		Key:     "preferences",
		Content: "Use Mermaid for charts.",
	}, slots)
	if err != nil {
		t.Fatalf("validateNativeAgentMemoryDecision() error = %v", err)
	}
	if toolName != agentMemoryToolUpdate || args["key"] != "preferences" || args["content"] != "Use Mermaid for charts." {
		t.Fatalf("toolName=%q args=%#v, want update preferences", toolName, args)
	}
}

func TestValidateNativeAgentMemoryDecisionRejectsInvalidKey(t *testing.T) {
	_, _, err := validateNativeAgentMemoryDecision(nativeAgentMemoryDecision{
		Action:  "update",
		Key:     "missing",
		Content: "Use Mermaid for charts.",
	}, []AgentMemorySlotConfig{{Key: "preferences", MaxChars: 80, Enabled: true}})
	if err == nil {
		t.Fatal("validateNativeAgentMemoryDecision() error = nil, want invalid key error")
	}
}

func TestValidateNativeAgentMemoryDecisionRejectsLongContent(t *testing.T) {
	_, _, err := validateNativeAgentMemoryDecision(nativeAgentMemoryDecision{
		Action:  "update",
		Key:     "preferences",
		Content: "This content is too long for the configured slot.",
	}, []AgentMemorySlotConfig{{Key: "preferences", MaxChars: 10, Enabled: true}})
	if err == nil {
		t.Fatal("validateNativeAgentMemoryDecision() error = nil, want length error")
	}
}

func TestValidateNativeAgentMemoryDecisionRejectsSensitiveContent(t *testing.T) {
	_, _, err := validateNativeAgentMemoryDecision(nativeAgentMemoryDecision{
		Action:  "update",
		Key:     "preferences",
		Content: "My bank card password is 123456.",
	}, []AgentMemorySlotConfig{{Key: "preferences", MaxChars: 120, Enabled: true}})
	if err == nil {
		t.Fatal("validateNativeAgentMemoryDecision() error = nil, want sensitive content error")
	}
}

func TestNativeAgentMemorySuccessNote(t *testing.T) {
	msg := nativeAgentMemorySuccessNote(nativeAgentMemoryDecision{Action: "update", Key: "preferences"}, map[string]interface{}{"key": "preferences"})
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("success note content type = %T, want string", msg.Content)
	}
	assertContains(t, content, "Agent memory update succeeded")
	assertContains(t, content, "preferences")
	assertContains(t, content, "Do not mention tools")
}

func TestRunNativeAgentMemoryPreflightUpdateAddsSuccessNote(t *testing.T) {
	prepared := preparedAgentMemoryPlannerChat("后续我让你画统计图的时候都用这个方案吧")
	fakeMemory := &fakeAgentMemoryContextService{}
	fakeLLM := &fakeAgentMemoryPlannerLLM{
		response: agentMemoryPlannerResponse(`{"action":"update","key":"preferences","content":"Use Mermaid syntax directly for future chart requests.","confidence":0.91,"reason":"durable output preference"}`),
	}
	svc := &service{agentMemoryService: fakeMemory, llmClient: fakeLLM}
	events := []StreamEvent{}

	_, err := svc.runNativeAgentMemoryPreflight(context.Background(), context.Background(), prepared, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("runNativeAgentMemoryPreflight() error = %v", err)
	}
	if fakeMemory.updateCalls != 1 {
		t.Fatalf("update calls = %d, want 1", fakeMemory.updateCalls)
	}
	if fakeMemory.lastUpdateUser != prepared.Scope.AccountID {
		t.Fatalf("agent memory update user = %s, want scope account %s", fakeMemory.lastUpdateUser, prepared.Scope.AccountID)
	}
	if fakeMemory.lastUpdateReq.Key != "preferences" || !strings.Contains(fakeMemory.lastUpdateReq.Content, "Mermaid") {
		t.Fatalf("last update request = %#v, want preferences Mermaid content", fakeMemory.lastUpdateReq)
	}
	last := prepared.LLMRequest.Messages[len(prepared.LLMRequest.Messages)-1].Content.(string)
	assertContains(t, last, "Agent memory update succeeded")
	assertNotContains(t, last, "no Agent memory mutation succeeded")
	agentMemory := preparedAgentMemoryMetadata(t, prepared)
	if agentMemory["planner_status"] != "success_update" || agentMemory["mutation_status"] != "success" || agentMemory["mutation_key"] != "preferences" {
		t.Fatalf("agent memory metadata = %#v, want success_update/success/preferences", agentMemory)
	}
	if invocations := prepared.Message.Metadata["skill_invocations"]; invocations != nil {
		t.Fatalf("skill invocations = %#v, want no legacy agent-memory invocation", invocations)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one memory mutation event", events)
	}
	if events[0].EventType != streamEventMemoryUpdate || events[0].Payload["memory_scope"] != "agent" || events[0].Payload["action"] != "update" {
		t.Fatalf("event = %#v, want agent memory_update event", events[0])
	}
	content, _ := events[0].Payload["content"].(string)
	if events[0].Payload["key"] != "preferences" || !strings.Contains(content, "Mermaid") {
		t.Fatalf("event payload = %#v, want preferences Mermaid content", events[0].Payload)
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("planner requests = %d, want 1", len(fakeLLM.requests))
	}
	if fakeLLM.requests[0].ResponseFormat != nil {
		t.Fatalf("planner response_format = %#v, want nil without explicit JSON-mode capability", fakeLLM.requests[0].ResponseFormat)
	}
}

func TestRunNativeAgentMemoryPreflightUsesDedicatedAgentMemoryUserID(t *testing.T) {
	prepared := preparedAgentMemoryPlannerChat("Please remember this chart preference.")
	memoryUserID := uuid.New()
	prepared.Scope.AgentMemoryUserID = &memoryUserID
	fakeMemory := &fakeAgentMemoryContextService{}
	fakeLLM := &fakeAgentMemoryPlannerLLM{
		response: agentMemoryPlannerResponse(`{"action":"update","key":"preferences","content":"Use Mermaid syntax directly for future chart requests.","confidence":0.91,"reason":"durable output preference"}`),
	}
	svc := &service{agentMemoryService: fakeMemory, llmClient: fakeLLM}

	_, err := svc.runNativeAgentMemoryPreflight(context.Background(), context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("runNativeAgentMemoryPreflight() error = %v", err)
	}
	if fakeMemory.lastUpdateUser != memoryUserID {
		t.Fatalf("agent memory update user = %s, want dedicated memory user %s", fakeMemory.lastUpdateUser, memoryUserID)
	}
}

func TestRunNativeAgentMemoryPreflightNoneAddsGuardNote(t *testing.T) {
	prepared := preparedAgentMemoryPlannerChat("你会画统计图吗")
	fakeMemory := &fakeAgentMemoryContextService{}
	fakeLLM := &fakeAgentMemoryPlannerLLM{
		response: agentMemoryPlannerResponse(`{"action":"none","key":"","content":"","confidence":0.9,"reason":"capability question"}`),
	}
	svc := &service{agentMemoryService: fakeMemory, llmClient: fakeLLM}

	_, err := svc.runNativeAgentMemoryPreflight(context.Background(), context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("runNativeAgentMemoryPreflight() error = %v", err)
	}
	if fakeMemory.updateCalls != 0 || fakeMemory.clearCalls != 0 {
		t.Fatalf("memory mutations update=%d clear=%d, want none", fakeMemory.updateCalls, fakeMemory.clearCalls)
	}
	last := prepared.LLMRequest.Messages[len(prepared.LLMRequest.Messages)-1].Content.(string)
	assertContains(t, last, "no Agent memory mutation succeeded")
	assertContains(t, last, "must not say memory was remembered")
	agentMemory := preparedAgentMemoryMetadata(t, prepared)
	if agentMemory["planner_status"] != "success_none" || agentMemory["planner_action"] != "none" {
		t.Fatalf("agent memory metadata = %#v, want success_none/none", agentMemory)
	}
	if invocations := prepared.Message.Metadata["skill_invocations"]; invocations != nil {
		t.Fatalf("skill invocations = %#v, want no planner invocation", invocations)
	}
}

func TestRunNativeAgentMemoryPreflightPlannerErrorIsTraceableAndNonBlocking(t *testing.T) {
	prepared := preparedAgentMemoryPlannerChat("以后回答都先给结论")
	fakeMemory := &fakeAgentMemoryContextService{}
	fakeLLM := &fakeAgentMemoryPlannerLLM{err: errors.New("json mode rejected")}
	svc := &service{agentMemoryService: fakeMemory, llmClient: fakeLLM}

	_, err := svc.runNativeAgentMemoryPreflight(context.Background(), context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("runNativeAgentMemoryPreflight() error = %v", err)
	}
	if fakeMemory.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", fakeMemory.updateCalls)
	}
	last := prepared.LLMRequest.Messages[len(prepared.LLMRequest.Messages)-1].Content.(string)
	assertContains(t, last, "no Agent memory mutation succeeded")
	agentMemory := preparedAgentMemoryMetadata(t, prepared)
	if agentMemory["planner_status"] != "error_llm" || agentMemory["planner_action"] != "none" {
		t.Fatalf("agent memory metadata = %#v, want error_llm/none", agentMemory)
	}
	if invocations := prepared.Message.Metadata["skill_invocations"]; invocations != nil {
		t.Fatalf("skill invocations = %#v, want planner hidden from user timeline", invocations)
	}
}

func TestRunNativeAgentMemoryPreflightSkipsWhenMemoryDisabled(t *testing.T) {
	prepared := preparedAgentMemoryPlannerChat("hello")
	prepared.parts.AgentMemoryEnabled = false
	prepared.parts.AgentMemoryRuntimeState = nil
	fakeMemory := &fakeAgentMemoryContextService{}
	fakeLLM := &fakeAgentMemoryPlannerLLM{
		response: agentMemoryPlannerResponse(`{"action":"none","confidence":0.9,"reason":"unused"}`),
	}
	svc := &service{agentMemoryService: fakeMemory, llmClient: fakeLLM}

	_, err := svc.runNativeAgentMemoryPreflight(context.Background(), context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("runNativeAgentMemoryPreflight() error = %v", err)
	}
	if len(fakeLLM.requests) != 0 {
		t.Fatalf("planner requests = %d, want 0", len(fakeLLM.requests))
	}
	if shouldRunNativeAgentMemoryPreflight(prepared, fakeMemory, true) {
		t.Fatal("shouldRunNativeAgentMemoryPreflight() = true for disabled memory, want false")
	}
}

func TestRunConfigHasAgentMemoryRequiresEnabledSlot(t *testing.T) {
	if runConfigHasAgentMemory(&RunConfig{
		AgentMemoryEnabled: true,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Enabled: false},
		},
	}) {
		t.Fatal("runConfigHasAgentMemory() = true for disabled-only slots, want false")
	}
	if !runConfigHasAgentMemory(&RunConfig{
		AgentMemoryEnabled: true,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Enabled: true},
		},
	}) {
		t.Fatal("runConfigHasAgentMemory() = false for enabled slot, want true")
	}
}

func TestSkillRuntimeParametersIncludeOnlyEnabledAgentMemorySlots(t *testing.T) {
	params := skillRuntimeParameters(Scope{}, RunConfig{
		AgentMemoryEnabled: true,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "scratch", Enabled: false},
			{Key: "profile", Enabled: true},
		},
	})
	raw, ok := params["agent_memory_slots"].([]AgentMemorySlotConfig)
	if !ok {
		t.Fatalf("agent_memory_slots type = %T, want []AgentMemorySlotConfig", params["agent_memory_slots"])
	}
	if len(raw) != 1 || raw[0].Key != "profile" {
		t.Fatalf("agent_memory_slots = %#v, want only profile", raw)
	}
}

func TestAgenticSkillLoopSystemMessageKeepsVisibleSkillFlow(t *testing.T) {
	msg := agenticSkillLoopSystemMessage()
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("agentic skill loop content type = %T, want string", msg.Content)
	}
	assertContains(t, content, "When using skills or tools")
	assertContains(t, content, "summarize what happened")
	assertNotContains(t, content, "agent-memory")
}

func TestEffectiveAgentSkillIDsDoesNotAutoAddAgentMemory(t *testing.T) {
	got := effectiveAgentSkillIDs(nil, []skills.SkillDiscoveryMetadata{
		{
			ID:               skills.SkillAgentMemory,
			Status:           skills.SkillStatusActive,
			SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
		},
	}, &RunConfig{
		AgentMemoryEnabled: true,
		AgentMemorySlots: []AgentMemorySlotConfig{
			{Key: "profile", Enabled: true},
		},
	})
	if len(got) != 0 {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want no auto-added agent-memory skill", got)
	}
}

func TestNativeAgentMemoryToolsDoNotExposeRead(t *testing.T) {
	tools := nativeAgentMemoryTools([]AgentMemorySlotConfig{{Key: "profile", Enabled: true}})
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Function.Name)
	}
	if strings.Contains(strings.Join(names, ","), "read_agent_memory") {
		t.Fatalf("nativeAgentMemoryTools() exposed read_agent_memory: %#v", names)
	}
	if strings.Join(names, ",") != "update_agent_memory,clear_agent_memory" {
		t.Fatalf("nativeAgentMemoryTools() = %#v, want update and clear", names)
	}
}

type fakeAgentMemoryContextService struct {
	calls          int
	slots          []agentmemory.RuntimeSlot
	values         []agentmemory.SlotValueResponse
	updateCalls    int
	clearCalls     int
	lastUpdateReq  agentmemory.UpdateValueRequest
	lastReadUser   uuid.UUID
	lastUpdateUser uuid.UUID
}

func (f *fakeAgentMemoryContextService) ReadUserMemory(_ context.Context, _, _ uuid.UUID, slots []agentmemory.RuntimeSlot, _ string, userID uuid.UUID) ([]agentmemory.SlotValueResponse, error) {
	f.calls++
	f.slots = append([]agentmemory.RuntimeSlot(nil), slots...)
	f.lastReadUser = userID
	return f.values, nil
}

func (f *fakeAgentMemoryContextService) UpdateValue(_ context.Context, _, _ uuid.UUID, _ []agentmemory.RuntimeSlot, _ string, userID uuid.UUID, req agentmemory.UpdateValueRequest, _ agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error) {
	f.updateCalls++
	f.lastUpdateReq = req
	f.lastUpdateUser = userID
	return &agentmemory.SlotValueResponse{SlotResponse: agentmemory.SlotResponse{Key: req.Key}, Content: req.Content}, nil
}

func (f *fakeAgentMemoryContextService) ClearValue(_ context.Context, _, _ uuid.UUID, _ []agentmemory.RuntimeSlot, _ string, _ uuid.UUID, key string, _ agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error) {
	f.clearCalls++
	return &agentmemory.SlotValueResponse{SlotResponse: agentmemory.SlotResponse{Key: key}}, nil
}

type fakeAgentMemoryPlannerLLM struct {
	response *adapter.ChatResponse
	err      error
	requests []*adapter.ChatRequest
}

func (f *fakeAgentMemoryPlannerLLM) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("unexpected Chat call")
}

func (f *fakeAgentMemoryPlannerLLM) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected ChatStream call")
}

func (f *fakeAgentMemoryPlannerLLM) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected CreateResponse call")
}

func (f *fakeAgentMemoryPlannerLLM) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected Embed call")
}

func (f *fakeAgentMemoryPlannerLLM) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected CreateImage call")
}

func (f *fakeAgentMemoryPlannerLLM) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected Rerank call")
}

func (f *fakeAgentMemoryPlannerLLM) AppChat(_ context.Context, _ *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.requests = append(f.requests, cloneChatRequest(req))
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func (f *fakeAgentMemoryPlannerLLM) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected AppChatStream call")
}

func (f *fakeAgentMemoryPlannerLLM) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected AppCreateResponse call")
}

func (f *fakeAgentMemoryPlannerLLM) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected AppEmbed call")
}

func (f *fakeAgentMemoryPlannerLLM) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected AppCreateImage call")
}

func (f *fakeAgentMemoryPlannerLLM) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected AppRerank call")
}

func preparedAgentMemoryPlannerChat(query string) *PreparedChat {
	organizationID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	contextControl := map[string]interface{}{
		"agent_memory": map[string]interface{}{
			"enabled":        true,
			"available":      true,
			"injected":       true,
			"value_count":    1,
			"context_status": "success",
		},
	}
	slots := []AgentMemorySlotConfig{
		{Key: "profile", Description: "User profile facts", MaxChars: 800, Enabled: true},
		{Key: "preferences", Description: "Durable answer preferences", MaxChars: 800, Enabled: true},
		{Key: "standing_instructions", Description: "Durable interaction rules", MaxChars: 800, Enabled: true},
		{Key: "project_context", Description: "Ongoing project context", MaxChars: 800, Enabled: true},
	}
	values := []agentmemory.SlotValueResponse{
		{SlotResponse: agentmemory.SlotResponse{Key: "preferences"}, Content: "Use concise technical examples."},
	}
	return &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			WorkspaceID:    &workspaceID,
			AccountID:      accountID,
		},
		Message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Query:          query,
			Metadata: map[string]interface{}{
				"context_control": contextControl,
			},
		},
		Scope: Scope{OrganizationID: organizationID, WorkspaceID: &workspaceID, AccountID: accountID},
		RunConfig: RunConfig{
			BillingAppID:   agentID.String(),
			BillingAppType: runtimemodel.ConversationCallerAgent,
		},
		parts: &chatRequestParts{
			Query:                   query,
			Provider:                "deepseek",
			ModelName:               "deepseek-chat",
			ContextControl:          contextControl,
			AgentMemoryEnabled:      true,
			AgentMemoryAgentID:      agentID.String(),
			AgentMemoryUserScope:    agentmemory.UserScopeAccount,
			AgentMemorySlots:        slots,
			AgentMemoryRuntimeState: &AgentMemoryRuntimeState{Enabled: true, AgentID: agentID, UserScope: agentmemory.UserScopeAccount, EnabledSlots: slots, SavedValues: values, ContextStatus: "success"},
		},
		LLMRequest: &adapter.ChatRequest{
			Provider: "deepseek",
			Model:    "deepseek-chat",
			Messages: []adapter.Message{
				{Role: "system", Content: "base system"},
				{Role: "assistant", Content: "I can draw charts using Mermaid."},
				{Role: "user", Content: query},
			},
		},
	}
}

func agentMemoryPlannerResponse(content string) *adapter.ChatResponse {
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: content},
		}},
		Usage: &adapter.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func preparedAgentMemoryMetadata(t *testing.T, prepared *PreparedChat) map[string]interface{} {
	t.Helper()
	contextControl, ok := prepared.Message.Metadata["context_control"].(map[string]interface{})
	if !ok {
		t.Fatalf("context_control metadata = %#v, want map", prepared.Message.Metadata["context_control"])
	}
	agentMemory, ok := contextControl["agent_memory"].(map[string]interface{})
	if !ok {
		t.Fatalf("agent_memory metadata = %#v, want map", contextControl["agent_memory"])
	}
	return agentMemory
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected %q not to contain %q", got, want)
	}
}
