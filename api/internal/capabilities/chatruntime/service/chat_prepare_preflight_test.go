package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestContextualPreparePreflightsDoNotClassifyTurnIntent(t *testing.T) {
	fakeLLM := &fakeAgentMemoryPlannerLLM{}
	svc := &service{llmClient: fakeLLM}
	parts := &chatRequestParts{
		Query:     "open files and read the first document",
		Surface:   aiChatSurfaceContextualSidebar,
		ModelName: "test-model",
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator, skills.SkillFileReader},
	}

	preflight, err := svc.runContextualPreparePreflights(
		context.Background(),
		Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		&runtimemodel.Conversation{ID: uuid.New()},
		RunConfig{},
		parts,
		&adapter.ChatRequest{},
	)
	if err != nil {
		t.Fatalf("runContextualPreparePreflights() error = %v", err)
	}
	if preflight != nil {
		t.Fatalf("preflight = %#v, want nil when no memory preflight is needed", preflight)
	}
	if len(fakeLLM.requests) != 0 {
		t.Fatalf("LLM preflight requests = %d, want 0", len(fakeLLM.requests))
	}
	if parts.ModelTurnIntent != nil || parts.ModelTurnIntentError != "" {
		t.Fatalf("turn intent metadata was populated: intent=%#v error=%q", parts.ModelTurnIntent, parts.ModelTurnIntentError)
	}
}

func TestMarkUserMemoryPreflightTimeoutPreservesContext(t *testing.T) {
	parts := &chatRequestParts{ContextControl: map[string]interface{}{
		"existing": "value",
		"user_memory": map[string]interface{}{
			"previous": "kept",
		},
	}}

	markUserMemoryPreflightTimeout(parts)

	if parts.ContextControl["existing"] != "value" {
		t.Fatalf("existing context = %#v, want preserved", parts.ContextControl["existing"])
	}
	userMemory := mapFromOperationContext(parts.ContextControl["user_memory"])
	if userMemory["previous"] != "kept" {
		t.Fatalf("previous memory metadata = %#v, want preserved", userMemory["previous"])
	}
	if userMemory["planner_status"] != "timeout_non_blocking" || userMemory["planner_action"] != "none" {
		t.Fatalf("user_memory = %#v, want non-blocking timeout status", userMemory)
	}
}

func TestMarkUserMemoryPreflightErrorPreservesContext(t *testing.T) {
	parts := &chatRequestParts{ContextControl: map[string]interface{}{
		"existing": "value",
		"user_memory": map[string]interface{}{
			"previous": "kept",
		},
	}}

	markUserMemoryPreflightError(parts, errors.New("memory planner failed"))

	if parts.ContextControl["existing"] != "value" {
		t.Fatalf("existing context = %#v, want preserved", parts.ContextControl["existing"])
	}
	userMemory := mapFromOperationContext(parts.ContextControl["user_memory"])
	if userMemory["previous"] != "kept" {
		t.Fatalf("previous memory metadata = %#v, want preserved", userMemory["previous"])
	}
	if userMemory["planner_status"] != "error_non_blocking" || userMemory["planner_action"] != "none" {
		t.Fatalf("user_memory = %#v, want non-blocking error status", userMemory)
	}
	if userMemory["planner_error"] != "memory planner failed" {
		t.Fatalf("planner_error = %#v", userMemory["planner_error"])
	}
}

func TestApplyModelCapabilitiesAssumesFunctionCallingWhenSpecUnknown(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{}, false, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "new-model"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if !parts.FunctionCallingKnown || !parts.ModelSupportsFunctionCalling || !parts.FunctionCallingAssumed {
		t.Fatalf("function calling flags = known %v supported %v assumed %v", parts.FunctionCallingKnown, parts.ModelSupportsFunctionCalling, parts.FunctionCallingAssumed)
	}
	if parts.ModelCapabilityStatus != "model_spec_unknown" {
		t.Fatalf("ModelCapabilityStatus = %q", parts.ModelCapabilityStatus)
	}
	applyProtocolToolsPolicy(Caller{Type: runtimemodel.ConversationCallerAgent}, parts)
	if parts.ProtocolToolsEnabled {
		t.Fatal("ProtocolToolsEnabled = true for assumed function-calling capability")
	}
	metadata := streamingMessageMetadata(parts)
	capabilities := mapFromOperationContext(metadata["model_capabilities"])
	if capabilities["function_calling_assumed"] != true || capabilities["status"] != "model_spec_unknown" {
		t.Fatalf("model_capabilities = %#v", capabilities)
	}
}

func TestApplyModelCapabilitiesKeepsKnownUnsupportedModelDisabled(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{SupportsToolCall: false}, true, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "legacy-model"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if !parts.FunctionCallingKnown || parts.ModelSupportsFunctionCalling || parts.FunctionCallingAssumed {
		t.Fatalf("function calling flags = known %v supported %v assumed %v", parts.FunctionCallingKnown, parts.ModelSupportsFunctionCalling, parts.FunctionCallingAssumed)
	}
	if parts.ModelCapabilityStatus != "resolved" {
		t.Fatalf("ModelCapabilityStatus = %q", parts.ModelCapabilityStatus)
	}
}

func TestApplyModelCapabilitiesAssumesFunctionCallingWhenResolverFails(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{}, false, errors.New("metadata store unavailable")
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "new-model"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if !parts.FunctionCallingKnown || !parts.ModelSupportsFunctionCalling || !parts.FunctionCallingAssumed {
		t.Fatalf("function calling flags = known %v supported %v assumed %v", parts.FunctionCallingKnown, parts.ModelSupportsFunctionCalling, parts.FunctionCallingAssumed)
	}
	if parts.ModelCapabilityStatus != "resolver_error" || parts.ModelCapabilityError != "metadata store unavailable" {
		t.Fatalf("model capability status/error = %q/%q", parts.ModelCapabilityStatus, parts.ModelCapabilityError)
	}
}

type modelSpecResolverFunc func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error)

func (fn modelSpecResolverFunc) Resolve(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (ModelSpec, bool, error) {
	return fn(ctx, organizationID, provider, modelName)
}
