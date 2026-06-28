package skillloop

import (
	"strings"
	"testing"
)

func TestVisibleAgentProgressText(t *testing.T) {
	usefulChinese := "\u6211\u4f1a\u5148\u786e\u8ba4\u5f53\u524d\u914d\u7f6e\uff0c\u518d\u6267\u884c\u4fee\u6539\u3002"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "keeps high level user progress",
			input: usefulChinese,
			want:  usefulChinese,
		},
		{
			name:  "drops tool protocol narration",
			input: "\u5df2\u5b8c\u6210 list_agents \u64cd\u4f5c\uff0c\u73b0\u5728\u6839\u636e\u7528\u6237\u660e\u786e\u7684\u4fee\u6539\u9700\u6c42\u4fee\u8ba2\u64cd\u4f5c\u8ba1\u5212\u3002",
			want:  "",
		},
		{
			name:  "keeps page context reasoning without protocol markers",
			input: "\u5f53\u524d\u9875\u9762\u4e0a\u5df2\u6709\u4e30\u5bcc\u7684\u4e0a\u4e0b\u6587\u3002\u6839\u636e\u9875\u9762\u4e0a\u4e0b\u6587\uff0c\u6211\u5df2\u77e5\u6b64\u667a\u80fd\u4f53\u7684\u5168\u90e8\u914d\u7f6e\u3002",
			want:  "\u5f53\u524d\u9875\u9762\u4e0a\u5df2\u6709\u4e30\u5bcc\u7684\u4e0a\u4e0b\u6587\u3002",
		},
		{
			name:  "keeps natural navigation reasoning without protocol markers",
			input: "\u8ba9\u6211\u770b\u770b\u662f\u5426\u53ef\u4ee5\u5bfc\u822a\u5230\u914d\u7f6e\u9875\u9762\u6765\u8fdb\u884c\u4fee\u6539\u3002",
			want:  "\u8ba9\u6211\u770b\u770b\u662f\u5426\u53ef\u4ee5\u5bfc\u822a\u5230\u914d\u7f6e\u9875\u9762\u6765\u8fdb\u884c\u4fee\u6539\u3002",
		},
		{
			name:  "keeps only the first visible sentence",
			input: "I will check the editable configuration first. Then I will apply the update.",
			want:  "I will check the editable configuration first.",
		},
		{
			name:  "drops internal ids",
			input: "I found the target ID: 123e4567-e89b-12d3-a456-426614174000.",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visibleAgentProgressText(tt.input); got != tt.want {
				t.Fatalf("visibleAgentProgressText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgenticSkillLoopSystemMessageProgressGuidance(t *testing.T) {
	content := messageContent(AgenticSkillLoopSystemMessage().Content)
	for _, want := range []string{
		"at most one brief, high-level user-facing progress sentence",
		"Do not narrate every tool call",
		"submit_intermediate_answer is for substantial user-facing deliverables only",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system message missing %q in:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{
		"briefly explain your next action",
		"After each skill/tool result, summarize what happened",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("system message still contains old guidance %q in:\n%s", unwanted, content)
		}
	}
}
