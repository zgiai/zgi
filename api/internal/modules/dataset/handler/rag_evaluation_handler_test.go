package handler

import (
	"strings"
	"testing"
)

func TestRAGEvaluationAnswerPromptUsesEnhancedEnglishAnswerContract(t *testing.T) {
	prompt := buildRAGEvaluationAnswerPrompt("MultiHop-RAG", "A report says the event happened first.", "Did it happen first?")

	for _, want := range []string{
		"Return an enhanced answer in English",
		"first write \"Answer: <short answer>.\"",
		"Then write 2 to 4 concise English sentences",
		"Answer: yes. The retrieved context states",
		"Answer: no. The retrieved context dates",
		"Answer: consistent. Both retrieved reports",
		"Answer: YouTube. Each retrieved report",
		"Answer: insufficient information. The retrieved context",
	} {
		if !strings.Contains(ragEvaluationAnswerSystemPrompt, want) {
			t.Fatalf("system prompt is missing %q", want)
		}
	}
	for _, want := range []string{
		"Knowledge base: MultiHop-RAG",
		"<knowledge_base_context>",
		"A report says the event happened first.",
		"Question: Did it happen first?",
		"Answer:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("user prompt is missing %q: %s", want, prompt)
		}
	}
}

func TestRAGEvaluationNoInformationAnswerIsEnglishReferenceStyle(t *testing.T) {
	if ragEvaluationNoInformation != "insufficient information" {
		t.Fatalf("unexpected no-information answer: %q", ragEvaluationNoInformation)
	}
}
