package service

import (
	"os"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// unifiedSkillDispatchEnabled reports whether skill tools go through the
// unified tool, the default since ADR 0044. ZGI_SKILL_TOOL_DISPATCH=native
// restores native per-tool functions.
func unifiedSkillDispatchEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("ZGI_SKILL_TOOL_DISPATCH")), "native")
}

// unifiedSkillDispatchSystemMessage tells the model how to reach skill tools
// in unified mode. It depends only on the mode, so it stays in the cached
// system prompt.
func unifiedSkillDispatchSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: "Skill tools are called through zgi: set command to \"<skill_id> <tool_name>\" and input to the tool's arguments, as the active skill's instructions list them. Other functions in this request are called directly."}
}
