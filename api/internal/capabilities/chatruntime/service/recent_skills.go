package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// recentSkillTurns is how many earlier turns decide the skills a turn starts
// with.
const recentSkillTurns = 3

// recentlyUsedSkillIDs lists the skills whose tools succeeded in the latest
// turns of a branch, newest first. A turn starts with them active, so their
// tools are in its tool list from the first round and their instructions are
// stored with its question once instead of resent after a tool call every
// turn (ADR 0043).
func recentlyUsedSkillIDs(branch []*runtimemodel.Message, turns int) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	counted := 0
	for index := len(branch) - 1; index >= 0 && counted < turns; index-- {
		message := branch[index]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		counted++
		for _, invocation := range skillInvocationMaps(message.Metadata) {
			if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
				!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
				continue
			}
			skillID := strings.ToLower(strings.TrimSpace(stringFromAny(invocation["skill_id"])))
			if skillID == "" {
				continue
			}
			if _, exists := seen[skillID]; exists {
				continue
			}
			seen[skillID] = struct{}{}
			out = append(out, skillID)
		}
	}
	return out
}
