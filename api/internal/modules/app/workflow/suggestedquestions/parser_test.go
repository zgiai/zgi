package suggestedquestions

import "testing"

func TestParseQuestionsNormalizesAndDeduplicates(t *testing.T) {
	raw := "```json\n{\"questions\":[{\"text\":\"How do I check order status?\",\"reason\":\"Covers the status lookup entry point\"},{\"text\":\"How do I check order status?\"},{\"text\":\"Help me escalate my service request\"}],\"warnings\":[\"Requires a configured knowledge base\"]}\n```"

	questions, warnings, err := ParseQuestions(raw, 4, []string{"Existing question"})
	if err != nil {
		t.Fatal(err)
	}

	if len(questions) != 2 {
		t.Fatalf("len(questions) = %d, want 2: %#v", len(questions), questions)
	}
	if questions[0].Text != "How do I check order status?" {
		t.Fatalf("first question = %q", questions[0].Text)
	}
	if len(warnings) != 1 || warnings[0] != "Requires a configured knowledge base" {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestParseQuestionsSupportsStringArray(t *testing.T) {
	questions, _, err := ParseQuestions(`["How do I summarize this report?"]`, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 1 || questions[0].Text != "How do I summarize this report?" {
		t.Fatalf("questions = %#v", questions)
	}
}
