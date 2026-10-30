package service

import (
	"context"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestApplySkillConfigMarksMissingDatabaseBindings(t *testing.T) {
	svc := &service{}
	agent := Caller{Type: runtimemodel.ConversationCallerAgent}

	parts := &chatRequestParts{}
	if err := svc.applySkillConfig(context.Background(), Scope{}, agent, &RunConfig{}, parts); err != nil {
		t.Fatalf("applySkillConfig: %v", err)
	}
	if !parts.DatabaseBindingsAbsent {
		t.Fatal("agent run without database bindings must be marked")
	}

	bound := &RunConfig{DatabaseBindings: []AgentDatabaseBinding{{DataSourceID: "ds-1", TableIDs: []string{"t-1"}}}}
	if err := svc.applySkillConfig(context.Background(), Scope{}, agent, bound, parts); err != nil {
		t.Fatalf("applySkillConfig: %v", err)
	}
	if parts.DatabaseBindingsAbsent {
		t.Fatal("agent run with a bound table must not be marked")
	}

	if err := svc.applySkillConfig(context.Background(), Scope{}, Caller{Type: runtimemodel.ConversationCallerAIChat}, nil, parts); err != nil {
		t.Fatalf("applySkillConfig: %v", err)
	}
	if parts.DatabaseBindingsAbsent {
		t.Fatal("AI chat runs have no agent bindings and must not be marked")
	}
}

// After the last table is unbound the database skill disappears; without a
// note the model answered from earlier results or invented "0".
func TestDatabaseBindingsRemovedMessageOnlyAfterEarlierDatabaseUse(t *testing.T) {
	used := []*runtimemodel.Message{{
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{"skill_invocations": []interface{}{
			map[string]interface{}{"kind": "tool_call", "skill_id": skills.SkillAgentDatabase, "tool_name": "query_agent_database"},
		}},
	}}
	unused := []*runtimemodel.Message{{
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{"skill_invocations": []interface{}{
			map[string]interface{}{"kind": "tool_call", "skill_id": skills.SkillAgentKnowledge},
		}},
	}}

	message := databaseBindingsRemovedMessage(&chatRequestParts{DatabaseBindingsAbsent: true}, used)
	if message == nil || message.Role != "system" {
		t.Fatalf("message = %#v, want a system note", message)
	}
	if content, _ := message.Content.(string); !strings.Contains(content, "no database table is currently bound") {
		t.Fatalf("content = %q", content)
	}
	if got := databaseBindingsRemovedMessage(&chatRequestParts{DatabaseBindingsAbsent: true}, unused); got != nil {
		t.Fatalf("note added without earlier database use: %#v", got)
	}
	if got := databaseBindingsRemovedMessage(&chatRequestParts{}, used); got != nil {
		t.Fatalf("note added while tables are still bound: %#v", got)
	}
}
