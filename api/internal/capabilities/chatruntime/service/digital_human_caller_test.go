package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type digitalHumanBindingReaderStub struct {
	wantRef   resourcebindings.SnapshotRef
	wantMatch resourcebindings.Match
	allowed   bool
}

func (s digitalHumanBindingReaderStub) ListSnapshot(context.Context, resourcebindings.SnapshotRef) ([]resourcebindings.Binding, error) {
	return nil, nil
}

func (s digitalHumanBindingReaderStub) Verify(_ context.Context, ref resourcebindings.SnapshotRef, match resourcebindings.Match) (resourcebindings.Decision, error) {
	if ref != s.wantRef || match != s.wantMatch {
		return resourcebindings.Decision{Reason: "mismatch"}, nil
	}
	if !s.allowed {
		return resourcebindings.Decision{Reason: "resource_not_bound"}, nil
	}
	return resourcebindings.Decision{Allowed: true}, nil
}

func TestDigitalHumanCallerIsNotNormalizedToAIChat(t *testing.T) {
	if got := normalizeCallerType(runtimemodel.ConversationCallerDigitalHuman); got != runtimemodel.ConversationCallerDigitalHuman {
		t.Fatalf("normalizeCallerType() = %q", got)
	}
	if !isAutonomousSubjectCaller(runtimemodel.ConversationCallerDigitalHuman) || !isDigitalHumanCaller(runtimemodel.ConversationCallerDigitalHuman) {
		t.Fatal("DigitalHuman caller is not classified as an autonomous subject")
	}
}

func TestDigitalHumanCallerForcesUserAndAgentMemoryOff(t *testing.T) {
	parts := &chatRequestParts{
		UseMemory: true, AgentMemoryEnabled: true, AgentMemoryAutoExtractionEnabled: true,
		AgentMemoryToolsEnabled: true, AgentMemorySlots: []AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
		AgentMemoryUserScope: "user", AgentMemoryConfigScope: "agent-user", AgentMemoryConfigRevision: "r1",
		AgentMemoryAgentID: uuid.NewString(), AgentMemoryRuntimeState: &AgentMemoryRuntimeState{}, AgentMemoryContext: "secret context",
	}
	caller := Caller{Type: runtimemodel.ConversationCallerDigitalHuman}
	applyCallerMemoryPolicy(caller, parts)
	applyAgentMemoryToolsPolicy(parts)

	if parts.UseMemory || parts.AgentMemoryEnabled || parts.AgentMemoryAutoExtractionEnabled || parts.AgentMemoryToolsEnabled ||
		len(parts.AgentMemorySlots) != 0 || parts.AgentMemoryUserScope != "" || parts.AgentMemoryConfigScope != "" ||
		parts.AgentMemoryConfigRevision != "" || parts.AgentMemoryAgentID != "" || parts.AgentMemoryRuntimeState != nil || parts.AgentMemoryContext != "" {
		t.Fatalf("memory state was not cleared: %#v", parts)
	}
	if runConfigAllowsUserMemory(RunConfig{UseMemory: true, BillingAppType: runtimemodel.ConversationCallerDigitalHuman}) {
		t.Fatal("DigitalHuman RunConfig enabled account memory")
	}
}

func TestDigitalHumanUsesSharedAgentContextManager(t *testing.T) {
	messageID := uuid.New()
	prepared := &PreparedChat{
		Caller: Caller{Type: runtimemodel.ConversationCallerDigitalHuman},
		Message: &runtimemodel.Message{
			ID:       messageID,
			Metadata: map[string]interface{}{},
		},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		LLMRequest: &adapter.ChatRequest{
			Provider: "test-provider",
			Model:    "test-model",
			Messages: []adapter.Message{
				{Role: "system", Content: "You are a DigitalHuman."},
				{Role: "user", Content: "Continue the current task."},
			},
		},
		contextBudget: &contextBudgetResult{
			Spec: ModelSpec{ContextWindow: 128_000, MaxOutputTokens: 8_000},
			Budget: &budgetComputation{
				ConfiguredAgentWindowK: 256,
				ReservedOutputTokens:   8_000,
			},
		},
	}

	runtime := &service{}
	manager, err := runtime.newAgentContextManager(t.Context(), prepared)
	if err != nil {
		t.Fatalf("newAgentContextManager() error = %v", err)
	}
	if manager == nil {
		t.Fatal("DigitalHuman context manager = nil")
	}
	request, decision, err := manager.PrepareBeforeModelCall(t.Context(), prepared.LLMRequest)
	if err != nil {
		t.Fatalf("PrepareBeforeModelCall() error = %v", err)
	}
	if request == nil || decision.AgentRunID != messageID.String() {
		t.Fatalf("context decision = %#v", decision)
	}
	if decision.Budget.AgentContextWindow != 128_000 || !decision.Budget.AgentContextWindowClamped {
		t.Fatalf("DigitalHuman context budget = %#v", decision.Budget)
	}
	if prepared.Message.Metadata["agent_run_id"] != messageID.String() {
		t.Fatalf("agent_run_id = %#v", prepared.Message.Metadata["agent_run_id"])
	}
}

func TestDigitalHumanSkillProfileDeniesAgentAndUserMemoryManagement(t *testing.T) {
	got := filterDigitalHumanSkillIDs([]string{
		"calculator", skills.SkillAgentManagement, skills.SkillAgentMemory, skills.SkillUserMemory, skills.SkillAgentWorkflow,
	})
	want := map[string]bool{"calculator": false, skills.SkillAgentWorkflow: false}
	for _, id := range got {
		if _, ok := want[id]; ok {
			want[id] = true
		}
		if id == skills.SkillAgentManagement || id == skills.SkillAgentMemory || id == skills.SkillUserMemory {
			t.Fatalf("forbidden skill survived: %q", id)
		}
	}
	for id, present := range want {
		if !present {
			t.Fatalf("allowed skill missing: %q; got %#v", id, got)
		}
	}
}

func TestDigitalHumanUsesPinnedBindingVerifierAndAutonomousToolProfile(t *testing.T) {
	digitalHumanID, versionID := uuid.New(), uuid.New()
	ref := resourcebindings.SnapshotRef{Subject: runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}, Phase: resourcebindings.PhasePublished, VersionID: &versionID}
	match := resourcebindings.Match{
		Type: resourcebindings.BindingTypeIntegrationConnection, ResourceID: "connection-1", ParentResourceID: "feishu",
		AccessMode: resourcebindings.AccessModeExecute, ActionID: "message.send",
	}
	prepared := &PreparedChat{
		Scope:        Scope{OrganizationID: ref.Subject.OrganizationID, AccountID: uuid.New(), WorkspaceID: &ref.Subject.WorkspaceID},
		Caller:       Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID},
		RunConfig:    RunConfig{BindingReader: digitalHumanBindingReaderStub{wantRef: ref, wantMatch: match, allowed: true}, BindingSnapshot: ref},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()}, Message: &runtimemodel.Message{ID: uuid.New()},
	}
	var runtime *service
	verifier := runtime.currentAgentBindingVerifier(prepared)
	if verifier == nil {
		t.Fatal("DigitalHuman binding verifier = nil")
	}
	allowed, err := verifier(t.Context(), skills.AgentBindingCheck{
		BindingType: string(match.Type), ResourceID: match.ResourceID, ParentResourceID: match.ParentResourceID,
		AccessMode: string(match.AccessMode), ActionID: match.ActionID,
	})
	if err != nil || !allowed {
		t.Fatalf("binding verifier = %v, %v", allowed, err)
	}
	execution := runtime.skillExecutionContext(prepared)
	if execution.InvokeFrom != tools.ToolInvokeFromAgent {
		t.Fatalf("InvokeFrom = %q, want compatibility autonomous profile", execution.InvokeFrom)
	}
}

func TestDigitalHumanBindingVerifierRejectsUnboundResourceBeforeProvider(t *testing.T) {
	digitalHumanID, versionID := uuid.New(), uuid.New()
	ref := resourcebindings.SnapshotRef{Subject: runtimeidentity.SubjectRef{
		Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
	}, Phase: resourcebindings.PhasePublished, VersionID: &versionID}
	match := resourcebindings.Match{
		Type: resourcebindings.BindingTypeSkill, ResourceID: "unbound-skill", AccessMode: resourcebindings.AccessModeExecute,
	}
	prepared := &PreparedChat{
		Caller:    Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID},
		RunConfig: RunConfig{BindingReader: digitalHumanBindingReaderStub{wantRef: ref, wantMatch: match}, BindingSnapshot: ref},
	}
	var runtime *service
	verifier := runtime.currentAgentBindingVerifier(prepared)
	if verifier == nil {
		t.Fatal("DigitalHuman binding verifier = nil")
	}
	allowed, err := verifier(t.Context(), skills.AgentBindingCheck{
		BindingType: string(match.Type), ResourceID: match.ResourceID, AccessMode: string(match.AccessMode),
	})
	if err != nil || allowed {
		t.Fatalf("unbound resource verifier = %v, %v; provider must not run", allowed, err)
	}
}

func TestDigitalHumanSkillExecutionContextKeepsAgentProfileWithDigitalHumanTarget(t *testing.T) {
	digitalHumanID := uuid.New()
	runID := uuid.NewString()
	versionID := uuid.NewString()
	config := RunConfig{
		ConnectorTargetScope:     "published",
		ConnectorTargetVersionID: versionID,
		ConnectorRunID:           runID,
		ConnectorInteractive:     true,
	}
	parts := &chatRequestParts{}
	applyRunConfigToParts(config, parts)
	prepared := &PreparedChat{
		Scope:        Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		Caller:       Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID},
		RunConfig:    config,
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		parts:        parts,
	}

	runtime := &service{}
	execution := runtime.skillExecutionContext(prepared)
	if execution.InvokeFrom != tools.ToolInvokeFromAgent {
		t.Fatalf("InvokeFrom = %q, want the existing autonomous profile", execution.InvokeFrom)
	}
	if execution.ConnectorTargetType != connectorcore.TargetTypeDigitalHuman {
		t.Fatalf("ConnectorTargetType = %q, want explicit DigitalHuman", execution.ConnectorTargetType)
	}
	if execution.AppID != digitalHumanID.String() {
		t.Fatalf("AppID = %q, want host %q", execution.AppID, digitalHumanID)
	}
	if execution.ConnectorTargetScope != "published" || execution.ConnectorTargetVersionID != versionID {
		t.Fatalf("published binding coordinates lost: scope=%q version=%q", execution.ConnectorTargetScope, execution.ConnectorTargetVersionID)
	}
	if execution.ConnectorRunID != runID || !execution.ConnectorInteractive {
		t.Fatalf("run coordinates lost: runID=%q interactive=%v", execution.ConnectorRunID, execution.ConnectorInteractive)
	}
}

func TestDigitalHumanSkillExecutionContextUsesDigitalHumanIdentityResolver(t *testing.T) {
	digitalHumanID := uuid.New()
	prepared := &PreparedChat{
		Scope:        Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		Caller:       Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		parts:        &chatRequestParts{},
	}

	agentResolver := &connectorNativeLoopAgentResolver{}
	dhResolver := &digitalHumanRuntimeResolverStub{}
	runtime := &service{connectorAgentRuntime: agentResolver, connectorDigitalHumanRuntime: dhResolver}
	execution := runtime.skillExecutionContext(prepared)
	if execution.InvokeFrom != tools.ToolInvokeFromAgent {
		t.Fatalf("InvokeFrom = %q, want the compatibility autonomous profile", execution.InvokeFrom)
	}
	if execution.ConnectorIdentityResolver != dhResolver {
		t.Fatalf("ConnectorIdentityResolver = %#v, want the DigitalHuman resolver", execution.ConnectorIdentityResolver)
	}
	if execution.ConnectorIdentityResolver == agentResolver {
		t.Fatal("DigitalHuman execution borrowed the Agent identity resolver")
	}

	// A missing DigitalHuman resolver must fail closed instead of falling back to
	// the Agent resolver implied by the compatibility InvokeFrom profile.
	runtime = &service{connectorAgentRuntime: agentResolver}
	execution = runtime.skillExecutionContext(prepared)
	if execution.ConnectorIdentityResolver != nil {
		t.Fatalf("missing DigitalHuman resolver fell back to %#v", execution.ConnectorIdentityResolver)
	}
}

func TestChatAndAgentSkillExecutionContextKeepsConnectorTargetDerived(t *testing.T) {
	for _, callerType := range []string{runtimemodel.ConversationCallerAIChat, runtimemodel.ConversationCallerAgent} {
		callerID := uuid.New()
		prepared := &PreparedChat{
			Scope:        Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
			Caller:       Caller{Type: callerType, ID: &callerID},
			Conversation: &runtimemodel.Conversation{ID: uuid.New()},
			Message:      &runtimemodel.Message{ID: uuid.New()},
			parts:        &chatRequestParts{},
		}

		runtime := &service{}
		execution := runtime.skillExecutionContext(prepared)
		if execution.ConnectorTargetType != "" || execution.ConnectorRunID != "" || execution.ConnectorInteractive {
			t.Fatalf("%s execution context inherited DigitalHuman coordinates: %#v", callerType, execution)
		}
	}
}
