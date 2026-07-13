package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestAgentProtocolToolLoopDoesNotRequireBusinessSkills(t *testing.T) {
	parts := &chatRequestParts{
		FunctionCallingKnown:         true,
		ModelSupportsFunctionCalling: true,
	}
	caller := Caller{Type: runtimemodel.ConversationCallerAgent}
	applyProtocolToolsPolicy(caller, parts)

	svc := &service{skillRuntime: skills.NewRuntimeWithCatalog(nil, nil, t.TempDir())}
	err := svc.applySkillConfig(context.Background(), Scope{OrganizationID: uuid.New()}, caller, &RunConfig{}, parts)
	if err != nil {
		t.Fatalf("applySkillConfig() error = %v", err)
	}
	if !parts.ProtocolToolsEnabled {
		t.Fatal("ProtocolToolsEnabled = false, want true for function-calling Agent")
	}
	if chatPartsBusinessSkillsEnabled(parts) {
		t.Fatalf("business skills enabled with SkillIDs = %#v and mode = %q", parts.SkillIDs, parts.SkillMode)
	}
	if !chatPartsToolLoopEnabled(parts) {
		t.Fatal("chatPartsToolLoopEnabled() = false, want protocol-only loop")
	}
}

func TestProtocolToolPolicyDoesNotEnableAIChatOrUnsupportedAgent(t *testing.T) {
	tests := []struct {
		name   string
		caller Caller
		parts  chatRequestParts
	}{
		{
			name:   "aichat",
			caller: Caller{Type: runtimemodel.ConversationCallerAIChat},
			parts: chatRequestParts{
				FunctionCallingKnown:         true,
				ModelSupportsFunctionCalling: true,
			},
		},
		{
			name:   "unsupported agent model",
			caller: Caller{Type: runtimemodel.ConversationCallerAgent},
			parts: chatRequestParts{
				FunctionCallingKnown:         true,
				ModelSupportsFunctionCalling: false,
			},
		},
		{
			name:   "agent model with assumed function calling",
			caller: Caller{Type: runtimemodel.ConversationCallerAgent},
			parts: chatRequestParts{
				FunctionCallingKnown:         true,
				ModelSupportsFunctionCalling: true,
				FunctionCallingAssumed:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyProtocolToolsPolicy(tt.caller, &tt.parts)
			if tt.parts.ProtocolToolsEnabled {
				t.Fatal("ProtocolToolsEnabled = true, want false")
			}
		})
	}
}
