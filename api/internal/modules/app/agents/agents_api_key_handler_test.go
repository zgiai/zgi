package agents

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestPublishedAgentConfigRequiresExternalUserOnlyWhenMemoryEnabledWithSlots(t *testing.T) {
	tests := []struct {
		name string
		cfg  dto.AgentConfigResponse
		want bool
	}{
		{
			name: "disabled memory",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: false,
				AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
			},
			want: false,
		},
		{
			name: "enabled memory without enabled slots",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: true,
				AgentMemorySlots: []dto.AgentMemorySlotConfig{
					{Key: "profile", Enabled: false},
					{Key: "", Enabled: true},
				},
			},
			want: false,
		},
		{
			name: "enabled memory with enabled slot",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: true,
				AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publishedAgentConfigRequiresExternalUser(tt.cfg)
			if got != tt.want {
				t.Fatalf("publishedAgentConfigRequiresExternalUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternalAgentMemoryUserIDStableAcrossAPIKeys(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()

	first := externalAgentMemoryUserID(workspaceID, agentID, " external-user-1 ")
	second := externalAgentMemoryUserID(workspaceID, agentID, "external-user-1")
	if first != second {
		t.Fatalf("externalAgentMemoryUserID() changed for same workspace/agent/user: %s != %s", first, second)
	}

	otherUser := externalAgentMemoryUserID(workspaceID, agentID, "External-User-1")
	if otherUser == first {
		t.Fatalf("externalAgentMemoryUserID() ignored user case, got same id %s", first)
	}

	otherAgent := externalAgentMemoryUserID(workspaceID, uuid.New(), "external-user-1")
	if otherAgent == first {
		t.Fatalf("externalAgentMemoryUserID() ignored agent scope, got same id %s", first)
	}
}
