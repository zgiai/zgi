package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

// The person's own AI chat remembers them; chats embedded for others and
// Agents keep what they had (ADR 0021).
func TestAIChatMemoryPolicyCoversThePersonsOwnChat(t *testing.T) {
	for _, surface := range []string{aiChatSurfaceWorkChat, aiChatSurfaceContextualSidebar} {
		parts := &chatRequestParts{Surface: surface}
		applyCallerMemoryPolicy(Caller{Type: runtimemodel.ConversationCallerAIChat}, parts)
		if !parts.AgentMemoryEnabled || !isAIChatMemory(parts) || parts.AgentMemoryUserScope != agentmemory.UserScopeAccount ||
			parts.AgentMemoryConfigScope != agentmemory.ConfigScopePublished || parts.AgentMemoryConfigRevision != relationshipmemory.AIChatConfigRevision {
			t.Fatalf("%s parts = %#v", surface, parts)
		}
		if workspaceID, ok := agentMemoryWorkspaceID(Scope{}, parts); !ok || workspaceID != uuid.Nil {
			t.Fatalf("%s memory workspace = %s, %v", surface, workspaceID, ok)
		}
		identity := agentRelationshipMemoryIdentity(Scope{AccountID: uuid.New()}, parts)
		if decision := relationshipmemory.DecideAccess(identity); !decision.Read || identity.Surface != relationshipmemory.SurfaceAIChat {
			t.Fatalf("%s identity = %#v, decision = %#v", surface, identity, decision)
		}
	}

	external := &chatRequestParts{Surface: aiChatSurfaceExternalPageChat}
	applyCallerMemoryPolicy(Caller{Type: runtimemodel.ConversationCallerAIChat}, external)
	if external.AgentMemoryEnabled || isAIChatMemory(external) {
		t.Fatalf("external page chat parts = %#v", external)
	}

	agentID := uuid.New().String()
	agent := &chatRequestParts{Surface: aiChatSurfaceWorkChat, AgentMemoryEnabled: true, AgentMemoryAgentID: agentID}
	applyCallerMemoryPolicy(Caller{Type: runtimemodel.ConversationCallerAgent}, agent)
	if agent.AgentMemoryAgentID != agentID || isAIChatMemory(agent) {
		t.Fatalf("Agent parts = %#v", agent)
	}
}
