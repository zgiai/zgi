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
			name:  "drops assertive navigation progress without tool evidence",
			input: "\u6211\u9700\u8981\u5148\u5bfc\u822a\u5230\u667a\u80fd\u4f53\u9875\u9762\uff0c\u7136\u540e\u4fee\u6539\u63cf\u8ff0\u3002",
			want:  "",
		},
		{
			name:  "keeps only the first visible sentence",
			input: "I will check the editable configuration first. Then I will apply the update.",
			want:  "I will check the editable configuration first.",
		},
		{
			name:  "repairs incomplete numbered Chinese progress",
			input: "\u597d\u7684\uff0c\u5df2\u5b8c\u6210\u4ee5\u4e0b\u6b65\u9aa4\uff1a 1.",
			want:  "\u6211\u6b63\u5728\u6839\u636e\u5df2\u5b8c\u6210\u7684\u7ed3\u679c\u7ee7\u7eed\u5904\u7406\u3002",
		},
		{
			name:  "repairs incomplete previous progress",
			input: "\u597d\u7684\uff0c\u4e4b\u524d\u5df2\u5b8c\u6210\uff1a 1.",
			want:  "\u6211\u6b63\u5728\u6839\u636e\u5df2\u5b8c\u6210\u7684\u7ed3\u679c\u7ee7\u7eed\u5904\u7406\u3002",
		},
		{
			name:  "keeps complete numbered Chinese progress item",
			input: "\u597d\u7684\uff0c\u4e4b\u524d\u5df2\u5b8c\u6210\uff1a 1. \u6587\u4ef6\u5185\u5bb9\u5df2\u7ecf\u8bfb\u53d6\u8fc7\u4e86\u3002\u73b0\u5728\u7ee7\u7eed\u5904\u7406\u3002",
			want:  "\u597d\u7684\uff0c\u4e4b\u524d\u5df2\u5b8c\u6210\uff1a 1. \u6587\u4ef6\u5185\u5bb9\u5df2\u7ecf\u8bfb\u53d6\u8fc7\u4e86\u3002",
		},
		{
			name:  "repairs incomplete numbered English progress",
			input: "Completed the following steps: 1.",
			want:  "I am continuing from the completed results.",
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

func TestLocalizedAgentProgressText(t *testing.T) {
	const chineseFallback = "\u6211\u5148\u786e\u8ba4\u5f53\u524d\u4fe1\u606f\uff0c\u518d\u7ee7\u7eed\u5904\u7406\u3002"

	tests := []struct {
		name     string
		userText string
		progress string
		want     string
	}{
		{
			name:     "keeps Chinese progress for Chinese request",
			userText: "\u5e2e\u6211\u4fee\u6539\u8fd9\u4e2a\u667a\u80fd\u4f53",
			progress: "\u6211\u5148\u786e\u8ba4\u5f53\u524d\u914d\u7f6e\uff0c\u518d\u7ee7\u7eed\u5904\u7406\u3002",
			want:     "\u6211\u5148\u786e\u8ba4\u5f53\u524d\u914d\u7f6e\uff0c\u518d\u7ee7\u7eed\u5904\u7406\u3002",
		},
		{
			name:     "localizes English progress for Chinese request",
			userText: "\u4fee\u6539\u8fd9\u4e2a\u667a\u80fd\u4f53\u7684\u5f00\u573a\u95ee\u9898",
			progress: "Let me start by loading the agent-management skill and reading the current config.",
			want:     chineseFallback,
		},
		{
			name:     "keeps English progress for English request",
			userText: "Update this agent's suggested questions",
			progress: "I will check the editable configuration first. Then I will apply the update.",
			want:     "I will check the editable configuration first.",
		},
		{
			name:     "still drops internal protocol progress",
			userText: "\u4fee\u6539\u8fd9\u4e2a\u667a\u80fd\u4f53",
			progress: "I will call get_agent_config before update_agent_config.",
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localizedAgentProgressText(tt.userText, tt.progress); got != tt.want {
				t.Fatalf("localizedAgentProgressText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgenticSkillLoopSystemMessageProgressGuidance(t *testing.T) {
	content := messageContent(AgenticSkillLoopSystemMessage().Content)
	for _, want := range []string{
		"at most one brief, high-level user-facing progress sentence",
		"same language as the user's latest request",
		"If the user writes in Chinese, progress text must be Chinese",
		"Do not narrate every tool call",
		"current page evidence",
		"Progress text must be one complete sentence",
		"Do not start every task by listing resources or navigating",
		"Do not announce that you need to navigate",
		"describe the outcome as executed and verified",
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
