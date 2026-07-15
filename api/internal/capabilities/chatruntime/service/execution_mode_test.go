package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestStreamingMessageMetadataPersistsExecutionMode(t *testing.T) {
	parts := &chatRequestParts{
		Surface:       aiChatSurfaceWorkChat,
		ExecutionMode: executionModeLegacyToolChat,
	}
	metadata := streamingMessageMetadata(parts)
	if got := stringMetadataValue(metadata["execution_mode"]); got != executionModeLegacyToolChat {
		t.Fatalf("execution_mode = %q, want %q", got, executionModeLegacyToolChat)
	}
	if got := stringMetadataValue(metadata["model_use_case"]); got != "text-chat" {
		t.Fatalf("model_use_case = %q, want text-chat", got)
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
