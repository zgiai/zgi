package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// databaseBindingsRemovedMessage tells the model that the database tables it
// used earlier in this conversation are no longer bound. Without it the
// database skill silently disappears after the last table is unbound, and the
// model answers from its earlier tool results or invents a value (for
// example "0") instead of saying the table is gone.
//
// The note is only added when the conversation history shows earlier
// agent-database tool use, so agents that never had a database are unaffected.
func databaseBindingsRemovedMessage(parts *chatRequestParts, branch []*runtimemodel.Message) *adapter.Message {
	if parts == nil || !parts.DatabaseBindingsAbsent || !branchUsedAgentDatabase(branch) {
		return nil
	}
	return &adapter.Message{Role: "system", Content: strings.Join([]string{
		"Database tables are no longer bound to this agent, so database tools are unavailable in this turn.",
		"Earlier database results in this conversation may be outdated and must not be reused as current data.",
		"If the user asks for database or table data, say that no database table is currently bound and that a table must be bound in the agent configuration. Never guess or invent values such as 0.",
	}, "\n")}
}

func branchUsedAgentDatabase(branch []*runtimemodel.Message) bool {
	for _, message := range branch {
		if message == nil {
			continue
		}
		for _, invocation := range skillInvocationMaps(message.Metadata) {
			if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentDatabase) {
				return true
			}
		}
	}
	return false
}
