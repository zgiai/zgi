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

func TestApplyModelCapabilitiesRejectsUnknownModelSpec(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{}, false, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "new-model"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err == nil {
		t.Fatal("applyModelCapabilities() error = nil, want unknown model error")
	}
}

func TestApplyModelCapabilitiesRejectsKnownUnsupportedModel(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{SupportsToolCall: false}, true, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "legacy-model", Surface: aiChatSurfaceContextualSidebar}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err == nil {
		t.Fatal("applyModelCapabilities() error = nil, want function-calling error")
	}
}

func TestApplyModelCapabilitiesSelectsWorkChatExecutionMode(t *testing.T) {
	tests := []struct {
		name string
		spec ModelSpec
		want string
	}{
		{name: "agent", spec: ModelSpec{UseCases: []string{"text-chat", "agent"}, SupportsToolCall: true}, want: executionModeAgentLoop},
		{name: "legacy tool chat", spec: ModelSpec{UseCases: []string{"text-chat"}, SupportsToolCall: true}, want: executionModeLegacyToolChat},
		{name: "direct chat", spec: ModelSpec{UseCases: []string{"text-chat"}}, want: executionModeDirectChat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
				return tt.spec, true, nil
			})}
			parts := &chatRequestParts{Provider: "private", ModelName: "model", Surface: aiChatSurfaceWorkChat}
			if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err != nil {
				t.Fatalf("applyModelCapabilities() error = %v", err)
			}
			if parts.ExecutionMode != tt.want {
				t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, tt.want)
			}
		})
	}
}

func TestApplyModelCapabilitiesKeepsAgentCallerOnAgentLoop(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{UseCases: []string{"text-chat"}, SupportsToolCall: true}, true, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "legacy-model", Surface: aiChatSurfaceExternalPageChat}
	caller := Caller{Type: runtimemodel.ConversationCallerAgent}
	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, caller, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if parts.ExecutionMode != executionModeAgentLoop {
		t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, executionModeAgentLoop)
	}
}

func TestApplyModelCapabilitiesRejectsResolverFailure(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{}, false, errModelSpecResolver
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "new-model"}

	err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts)
	if err == nil || !errors.Is(err, errModelSpecResolver) {
		t.Fatalf("applyModelCapabilities() error = %v, want resolver error", err)
	}
}

var errModelSpecResolver = errors.New("metadata store unavailable")

type modelSpecResolverFunc func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error)

func (fn modelSpecResolverFunc) Resolve(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (ModelSpec, bool, error) {
	return fn(ctx, organizationID, provider, modelName)
}
