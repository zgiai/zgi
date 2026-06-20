package prompt

import (
	"strings"
	"testing"
)

func TestGraphFlowQueryEntityExtractionTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(GraphFlowQueryEntityExtraction)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(struct {
		Query string
	}{
		Query: "quantum computing",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(rendered, "quantum computing") {
		t.Fatalf("rendered template does not contain query value: %s", rendered)
	}
	if strings.Contains(rendered, "{{") {
		t.Fatalf("rendered template still contains raw Go placeholders: %s", rendered)
	}
}

func TestGraphFlowGraphExtractionTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(GraphFlowGraphExtraction)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(struct {
		GlobalContext string
		DocumentTitle string
		SegmentText   string
	}{
		GlobalContext: "- Alpha\n- Beta",
		DocumentTitle: "Doc Title",
		SegmentText:   "Segment body",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	for _, expected := range []string{"Alpha", "Beta", "Doc Title", "Segment body"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered template does not contain %q: %s", expected, rendered)
		}
	}
}

func TestWorkflowSQLGeneratorSystemTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(WorkflowSQLGeneratorSystem)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(struct{}{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(rendered, "Return ONLY the raw SQL query.") {
		t.Fatalf("unexpected rendered workflow SQL system template: %s", rendered)
	}
}

func TestAIChatSystemTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(AIChatSystem)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(map[string]interface{}{"Surface": "work_chat"})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	for _, expected := range []string{
		"ZGI workbench assistant",
		"Do not claim to see or operate the current page",
		"High-risk asset operations require",
		"AIChat account memory and Agent memory are separate",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered aichat system template does not contain %q: %s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "helpful AI assistant") {
		t.Fatalf("unexpected rendered aichat system template: %s", rendered)
	}

	contextual, err := tmpl.Render(map[string]interface{}{"Surface": "contextual_sidebar"})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	for _, expected := range []string{
		"ZGI sidebar operation assistant",
		"navigating to relevant internal console modules",
	} {
		if !strings.Contains(contextual, expected) {
			t.Fatalf("rendered contextual aichat system template does not contain %q: %s", expected, contextual)
		}
	}
}

func TestCommonConversationTitleTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(CommonConversationTitle)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(struct {
		MessagesLast string
	}{
		MessagesLast: "User: 帮我规划周末旅行",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(rendered, "帮我规划周末旅行") {
		t.Fatalf("rendered title template does not contain conversation: %s", rendered)
	}
}

func TestWorkflowParameterExtractorChatUserTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(WorkflowParameterExtractorChatUser)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(struct {
		Structure   string
		Instruction string
		Text        string
	}{
		Structure:   "{\"type\":\"object\"}",
		Instruction: "Use the provided schema",
		Text:        "Book a flight to Paris",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	for _, expected := range []string{"{\"type\":\"object\"}", "Use the provided schema", "Book a flight to Paris"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered template does not contain %q: %s", expected, rendered)
		}
	}
}

func TestWorkflowLLMCompletionDefaultTemplateRender(t *testing.T) {
	tmpl, err := GetTemplate(WorkflowLLMCompletionDefault)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	rendered, err := tmpl.Render(struct {
		HistoriesPlaceholder string
		SysQueryPlaceholder  string
	}{
		HistoriesPlaceholder: "{{#histories#}}",
		SysQueryPlaceholder:  "{{#sys.query#}}",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	for _, expected := range []string{"{{#histories#}}", "{{#sys.query#}}"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered template does not preserve workflow placeholder %q: %s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "{{.HistoriesPlaceholder}}") || strings.Contains(rendered, "{{.SysQueryPlaceholder}}") {
		t.Fatalf("rendered template still contains Go placeholders: %s", rendered)
	}
}

func TestGoTemplateRenderingStillWorks(t *testing.T) {
	tmpl, err := GetTemplate(DatasetQuestionGeneration)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	_, err = tmpl.Render(struct {
		Count   int
		Content string
	}{
		Count:   2,
		Content: "Prompt center migration test",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
}
