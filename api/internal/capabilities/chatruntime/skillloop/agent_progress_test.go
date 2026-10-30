package skillloop

import (
	"strings"
	"testing"
)

func TestAgenticSkillLoopSystemMessageProgressGuidance(t *testing.T) {
	content := messageContent(agenticSkillLoopSystemMessage(true).Content)
	for _, want := range []string{
		"may provide concise user-facing progress",
		"It may contain multiple sentences or a short list",
		"Do not acknowledge or restate the user's request or latest correction",
		"same language as the user's latest request",
		"If the user writes in Chinese, progress text must be Chinese",
		"Do not narrate every tool call",
		"current page evidence",
		"Finish each progress update before calling tools",
		"Do not start every task by listing resources or navigating",
		"Do not announce that you need to navigate",
		"describe the outcome as executed and verified",
		"reconcile the complete user request with the execution evidence",
		"A backend read or mutation does not prove that the page changed",
		"do not submit while you still intend to perform an open phase",
		"submit_intermediate_answer is for substantial user-facing deliverables only",
		"except when the requested destination is a generated or managed file",
		"Do not emit the same long body through submit_intermediate_answer and then repeat it in generate_file",
		"Emit the full body in chat only when the user explicitly requests both an inline copy and a file",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system message missing %q in:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{
		"at most one brief, high-level user-facing progress sentence",
		"Progress text must be one complete sentence",
		"briefly explain your next action",
		"After each skill/tool result, summarize what happened",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("system message still contains old guidance %q in:\n%s", unwanted, content)
		}
	}
}
