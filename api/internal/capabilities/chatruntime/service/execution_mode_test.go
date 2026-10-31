package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestNativeExecutionModePersistsForContinuation(t *testing.T) {
	for _, mode := range []string{executionModeNativeAgentLoop, executionModeNativeToolLoop} {
		parts := &chatRequestParts{ExecutionMode: mode}
		metadata := streamingMessageMetadata(parts)
		if got := stringMetadataValue(metadata["execution_mode"]); got != mode {
			t.Fatalf("execution_mode = %q, want %q", got, mode)
		}
		restored := &chatRequestParts{}
		restoreExecutionModeFromMetadata(restored, metadata)
		if restored.ExecutionMode != mode {
			t.Fatalf("restored mode = %q, want %q", restored.ExecutionMode, mode)
		}
		if restored.ExecutionRouteReason != executionRoutePersistedMode {
			t.Fatalf("route reason = %q, want %q", restored.ExecutionRouteReason, executionRoutePersistedMode)
		}
	}
}

func TestExecutionModeForNewRequests(t *testing.T) {
	tests := []struct {
		name   string
		caller Caller
		parts  *chatRequestParts
		want   string
	}{
		{name: "agent", caller: Caller{Type: runtimemodel.ConversationCallerAgent}, parts: &chatRequestParts{ModelSupportsFunctionCalling: true}, want: executionModeNativeAgentLoop},
		{name: "work chat with skills", parts: &chatRequestParts{ModelSupportsFunctionCalling: true, SkillIDs: []string{"calculator"}}, want: executionModeNativeToolLoop},
		{name: "memory only", parts: &chatRequestParts{ModelSupportsFunctionCalling: true, AgentMemoryToolsEnabled: true}, want: executionModeNativeToolLoop},
		{name: "work chat without skills", parts: &chatRequestParts{ModelSupportsFunctionCalling: true}, want: executionModeDirectChat},
		{name: "work chat without function calling", parts: &chatRequestParts{}, want: executionModeDirectChat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := executionModeForModel(tt.caller, tt.parts); got != tt.want {
				t.Fatalf("executionModeForModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFinalizeExecutionModeDegradesAgentWithoutFunctionCalling(t *testing.T) {
	parts := &chatRequestParts{Provider: "private", ModelName: "text-only"}
	err := finalizeExecutionMode(Caller{Type: runtimemodel.ConversationCallerAgent}, parts)
	if err != nil || parts.ExecutionMode != executionModeDirectChat || parts.ExecutionRouteReason != executionRouteFunctionCallingUnavailable {
		t.Fatalf("finalizeExecutionMode() error=%v mode=%q reason=%q", err, parts.ExecutionMode, parts.ExecutionRouteReason)
	}
}

func TestFinalizeExecutionModeReevaluatesPersistedMemoryOnlyRoutes(t *testing.T) {
	parts := &chatRequestParts{ExecutionMode: executionModeDirectChat, ModelSupportsFunctionCalling: true, AgentMemoryToolsEnabled: true}
	if err := finalizeExecutionMode(Caller{}, parts); err != nil || parts.ExecutionMode != executionModeNativeToolLoop {
		t.Fatalf("enable memory: err=%v mode=%q", err, parts.ExecutionMode)
	}
	parts.AgentMemoryToolsEnabled = false
	parts.ExecutionMode = executionModeNativeToolLoop
	if err := finalizeExecutionMode(Caller{}, parts); err != nil || parts.ExecutionMode != executionModeDirectChat {
		t.Fatalf("disable memory: err=%v mode=%q", err, parts.ExecutionMode)
	}
}

func TestApplyRootRegenerationModelCapabilitiesKeepsPersistedModeForSameModel(t *testing.T) {
	for _, mode := range []string{executionModeNativeAgentLoop, executionModeNativeToolLoop, executionModeDirectChat} {
		t.Run(mode, func(t *testing.T) {
			provider := "private"
			message := &runtimemodel.Message{
				ModelProvider: &provider,
				ModelName:     "model",
				Metadata: map[string]interface{}{
					"execution_mode": mode,
				},
			}
			parts := &chatRequestParts{
				Provider:  provider,
				ModelName: message.ModelName,
				Surface:   aiChatSurfaceWorkChat,
			}
			svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
				return ModelSpec{UseCases: []string{"agent"}, SupportsToolCall: true}, true, nil
			})}

			if err := svc.applyRootRegenerationModelCapabilities(t.Context(), Scope{OrganizationID: uuid.New()}, Caller{}, message, parts); err != nil {
				t.Fatalf("applyRootRegenerationModelCapabilities() error = %v", err)
			}
			if parts.ExecutionMode != mode {
				t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, mode)
			}
		})
	}
}

func TestDirectChatNeverEnablesToolLoop(t *testing.T) {
	parts := &chatRequestParts{
		ExecutionMode:        executionModeDirectChat,
		ProtocolToolsEnabled: true,
		SkillMode:            skillModeAuto,
		SkillIDs:             []string{"file-manager"},
	}
	if chatPartsToolLoopEnabled(parts) {
		t.Fatal("chatPartsToolLoopEnabled() = true for direct chat")
	}
}

func TestNativeToolLoopUsesSkillLoopAndTextChatRouting(t *testing.T) {
	parts := &chatRequestParts{
		ExecutionMode: executionModeNativeToolLoop,
		SkillMode:     skillModeAuto,
		SkillIDs:      []string{"prompt-only"},
	}
	if !chatPartsToolLoopEnabled(parts) {
		t.Fatal("chatPartsToolLoopEnabled() = false, want prompt-only native tool loop")
	}
	prepared := &PreparedChat{parts: parts}
	if got := preparedModelUseCase(prepared); got != "text-chat" {
		t.Fatalf("preparedModelUseCase() = %q, want text-chat", got)
	}
}

// Messages persisted with a mode of the retired protocol loop, or with no
// mode, continue in the native loop (ADR 0046).
func TestRestoreExecutionModeContinuesRetiredModesNatively(t *testing.T) {
	for persisted, want := range map[string]string{
		"agent_loop":                 executionModeNativeAgentLoop,
		"legacy_tool_chat":           executionModeNativeToolLoop,
		"":                           executionModeNativeAgentLoop,
		executionModeNativeToolLoop:  executionModeNativeToolLoop,
		executionModeNativeAgentLoop: executionModeNativeAgentLoop,
	} {
		parts := &chatRequestParts{}
		restoreExecutionModeFromMetadata(parts, map[string]interface{}{"execution_mode": persisted})
		if parts.ExecutionMode != want {
			t.Fatalf("persisted %q restored as %q, want %q", persisted, parts.ExecutionMode, want)
		}
	}
}
