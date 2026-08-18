package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestStreamingMessageMetadataPersistsExecutionMode(t *testing.T) {
	parts := &chatRequestParts{
		Surface:              aiChatSurfaceWorkChat,
		ExecutionMode:        executionModeLegacyToolChat,
		ExecutionRouteReason: executionRoutePersistedMode,
	}
	metadata := streamingMessageMetadata(parts)
	if got := stringMetadataValue(metadata["execution_mode"]); got != executionModeLegacyToolChat {
		t.Fatalf("execution_mode = %q, want %q", got, executionModeLegacyToolChat)
	}
	if got := stringMetadataValue(metadata["model_use_case"]); got != "text-chat" {
		t.Fatalf("model_use_case = %q, want text-chat", got)
	}
	if got := stringMetadataValue(metadata["execution_route_reason"]); got != executionRoutePersistedMode {
		t.Fatalf("execution_route_reason = %q, want %q", got, executionRoutePersistedMode)
	}

	restored := &chatRequestParts{}
	restoreExecutionModeFromMetadata(restored, metadata)
	if restored.ExecutionMode != executionModeLegacyToolChat {
		t.Fatalf("restored ExecutionMode = %q, want %q", restored.ExecutionMode, executionModeLegacyToolChat)
	}
}

func TestRestoreExecutionModeKeepsLegacyContinuationOnAgentLoop(t *testing.T) {
	parts := &chatRequestParts{}
	restoreExecutionModeFromMetadata(parts, nil)
	if parts.ExecutionMode != executionModeAgentLoop {
		t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, executionModeAgentLoop)
	}
}

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

func TestApplyRootRegenerationModelCapabilitiesRetiresLegacyExecutionModes(t *testing.T) {
	for _, tt := range []struct {
		name          string
		persistedMode string
		caller        Caller
		skillIDs      []string
		wantMode      string
	}{
		{
			name:          "agent loop",
			persistedMode: executionModeAgentLoop,
			caller:        Caller{Type: runtimemodel.ConversationCallerAgent},
			wantMode:      executionModeNativeAgentLoop,
		},
		{
			name:          "legacy tool chat",
			persistedMode: executionModeLegacyToolChat,
			caller:        Caller{Type: runtimemodel.ConversationCallerAIChat},
			skillIDs:      []string{"calculator"},
			wantMode:      executionModeNativeToolLoop,
		},
		{
			name:     "message without persisted mode",
			caller:   Caller{Type: runtimemodel.ConversationCallerAgent},
			wantMode: executionModeNativeAgentLoop,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := "private"
			message := &runtimemodel.Message{
				ModelProvider: &provider,
				ModelName:     "model",
				Metadata:      map[string]interface{}{},
			}
			if tt.persistedMode != "" {
				message.Metadata["execution_mode"] = tt.persistedMode
			}
			parts := &chatRequestParts{
				Provider:  provider,
				ModelName: message.ModelName,
				Surface:   aiChatSurfaceWorkChat,
				SkillIDs:  tt.skillIDs,
			}
			svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
				return ModelSpec{UseCases: []string{"text-chat", "agent"}, SupportsToolCall: true}, true, nil
			})}

			if err := svc.applyRootRegenerationModelCapabilities(t.Context(), Scope{OrganizationID: uuid.New()}, tt.caller, message, parts); err != nil {
				t.Fatalf("applyRootRegenerationModelCapabilities() error = %v", err)
			}
			if parts.ExecutionMode != "" {
				t.Fatalf("ExecutionMode before finalization = %q, want empty", parts.ExecutionMode)
			}
			if err := finalizeExecutionMode(tt.caller, parts); err != nil {
				t.Fatalf("finalizeExecutionMode() error = %v", err)
			}
			if parts.ExecutionMode != tt.wantMode {
				t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, tt.wantMode)
			}
		})
	}
}

func TestApplyRootRegenerationModelCapabilitiesRecomputesModeForChangedModelIdentity(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider string
		model    string
	}{
		{name: "provider changed", provider: "other", model: "old-model"},
		{name: "model changed", provider: "private", model: "new-model"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			persistedProvider := "private"
			message := &runtimemodel.Message{
				ModelProvider: &persistedProvider,
				ModelName:     "old-model",
				Metadata: map[string]interface{}{
					"execution_mode": executionModeLegacyToolChat,
				},
			}
			parts := &chatRequestParts{
				Provider:  tt.provider,
				ModelName: tt.model,
				Surface:   aiChatSurfaceWorkChat,
			}
			svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
				return ModelSpec{UseCases: []string{"text-chat"}}, true, nil
			})}

			if err := svc.applyRootRegenerationModelCapabilities(t.Context(), Scope{OrganizationID: uuid.New()}, Caller{}, message, parts); err != nil {
				t.Fatalf("applyRootRegenerationModelCapabilities() error = %v", err)
			}
			if err := finalizeExecutionMode(Caller{}, parts); err != nil {
				t.Fatalf("finalizeExecutionMode() error = %v", err)
			}
			if parts.ExecutionMode != executionModeDirectChat {
				t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, executionModeDirectChat)
			}
		})
	}
}

func TestRetiredAgentLoopRegenerationRejectsLostRequiredCapability(t *testing.T) {
	provider := "private"
	message := &runtimemodel.Message{
		ModelProvider: &provider,
		ModelName:     "model",
		Metadata: map[string]interface{}{
			"execution_mode": executionModeAgentLoop,
		},
	}
	parts := &chatRequestParts{
		Provider:  provider,
		ModelName: message.ModelName,
		Surface:   aiChatSurfaceWorkChat,
	}
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{UseCases: []string{"text-chat"}}, true, nil
	})}

	caller := Caller{Type: runtimemodel.ConversationCallerAgent}
	if err := svc.applyRootRegenerationModelCapabilities(t.Context(), Scope{OrganizationID: uuid.New()}, caller, message, parts); err != nil {
		t.Fatalf("applyRootRegenerationModelCapabilities() error = %v", err)
	}
	if err := finalizeExecutionMode(caller, parts); err != nil {
		t.Fatalf("finalizeExecutionMode() error = %v", err)
	}
	if parts.ExecutionMode != executionModeDirectChat {
		t.Fatalf("execution mode = %q, want direct-chat fallback", parts.ExecutionMode)
	}
}

func TestLegacyToolChatContinuationOverridesAgentProtocolInstructions(t *testing.T) {
	message := continuationMessageForExecutionMode(adapter.Message{
		Role:    "system",
		Content: "Call update_plan before continuing.",
	}, executionModeLegacyToolChat)
	content := stringFromAny(message.Content)
	if !strings.Contains(content, "Agent planning and terminal-answer protocol tools are not available") {
		t.Fatalf("continuation content = %q, want legacy override", content)
	}
}

func TestPreparedModelUseCaseUsesPersistedExecutionMode(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"execution_mode": executionModeLegacyToolChat,
		}},
	}
	if got := preparedModelUseCase(prepared); got != "text-chat" {
		t.Fatalf("preparedModelUseCase() = %q, want text-chat", got)
	}

	prepared.parts = &chatRequestParts{ExecutionMode: executionModeAgentLoop}
	if got := preparedModelUseCase(prepared); got != "agent" {
		t.Fatalf("preparedModelUseCase() = %q, want agent", got)
	}

	prepared.parts = &chatRequestParts{ExecutionMode: executionModeNativeToolLoop}
	if got := preparedModelUseCase(prepared); got != "text-chat" {
		t.Fatalf("preparedModelUseCase() = %q, want text-chat", got)
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

func TestLegacyToolChatUsesNaturalFinalAnswer(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{"status": "in_progress"},
		}},
		parts: &chatRequestParts{ExecutionMode: executionModeLegacyToolChat},
	}
	if skillLoopPrefersExplicitFinalAnswer(prepared) {
		t.Fatal("skillLoopPrefersExplicitFinalAnswer() = true for legacy tool chat")
	}
}
