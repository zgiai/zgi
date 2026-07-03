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

func TestParseQuestionsSupportsObjectStringArray(t *testing.T) {
	questions, _, err := ParseQuestions(`{"questions":["帮我查询订单状态","总结这份报告"]}`, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 2 || questions[0].Text != "帮我查询订单状态" {
		t.Fatalf("questions = %#v", questions)
	}
}

func TestParseQuestionsSupportsAliasesAndThinkingContent(t *testing.T) {
	raw := `<think>{"questions":[{"text":"bad example"}]}</think>
{"suggested_questions":[{"question":"帮我分析知识库里的政策","description":"Uses the knowledge base"}]}`

	questions, _, err := ParseQuestions(raw, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 1 || questions[0].Text != "帮我分析知识库里的政策" {
		t.Fatalf("questions = %#v", questions)
	}
	if questions[0].Reason != "Uses the knowledge base" {
		t.Fatalf("reason = %q", questions[0].Reason)
	}
}

func TestParseQuestionsSupportsMarkdownListFallback(t *testing.T) {
	raw := "当然可以：\n1. 帮我检索知识库里的产品介绍\n2. 总结最近上传的合同"

	questions, _, err := ParseQuestions(raw, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 2 || questions[1].Text != "总结最近上传的合同" {
		t.Fatalf("questions = %#v", questions)
	}
}
