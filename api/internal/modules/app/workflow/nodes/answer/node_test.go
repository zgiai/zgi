package answer

import (
	"context"
	"testing"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestExecuteRun_RendersWorkflowImageFilesAsMarkdown(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"imagegen", "files"}, []*workflowfile.File{
		workflowfile.NewFile(
			"tenant-1",
			workflowfile.FileTypeImage,
			workflowfile.FileTransferMethodRemoteURL,
			workflowfile.WithFilename("cat-1.png"),
			workflowfile.WithMimeType("image/png"),
			workflowfile.WithRemoteURL("https://example.com/cat-1.png"),
		),
		workflowfile.NewFile(
			"tenant-1",
			workflowfile.FileTypeImage,
			workflowfile.FileTransferMethodRemoteURL,
			workflowfile.WithFilename("cat-2.png"),
			workflowfile.WithMimeType("image/png"),
			workflowfile.WithRemoteURL("https://example.com/cat-2.png"),
		),
	})

	node := newTestAnswerNode(vp, "{{#imagegen.files#}}")
	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	answer, ok := result.Outputs["answer"].(string)
	if !ok {
		t.Fatalf("answer output type = %T, want string", result.Outputs["answer"])
	}

	expected := "![cat-1.png](https://example.com/cat-1.png)\n![cat-2.png](https://example.com/cat-2.png)"
	if answer != expected {
		t.Fatalf("answer = %q, want %q", answer, expected)
	}
	if containsPointerAddress(answer) {
		t.Fatalf("answer = %q, should not contain pointer-like output", answer)
	}
}

func TestExecuteRun_RendersWorkflowNonImageFileAsLink(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"imagegen", "files"}, []*workflowfile.File{
		workflowfile.NewFile(
			"tenant-1",
			workflowfile.FileTypeDocument,
			workflowfile.FileTransferMethodRemoteURL,
			workflowfile.WithFilename("report.pdf"),
			workflowfile.WithMimeType("application/pdf"),
			workflowfile.WithRemoteURL("https://example.com/report.pdf"),
		),
	})

	node := newTestAnswerNode(vp, "{{#imagegen.files#}}")
	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	answer := result.Outputs["answer"].(string)
	expected := "[report.pdf](https://example.com/report.pdf)"
	if answer != expected {
		t.Fatalf("answer = %q, want %q", answer, expected)
	}
}

func TestExecuteRun_EmptyWorkflowFileArrayRendersEmptyString(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"imagegen", "files"}, []*workflowfile.File{})

	node := newTestAnswerNode(vp, "{{#imagegen.files#}}")
	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	answer := result.Outputs["answer"].(string)
	if answer != "" {
		t.Fatalf("answer = %q, want empty string", answer)
	}
	if containsPointerAddress(answer) {
		t.Fatalf("answer = %q, should not contain pointer-like output", answer)
	}
}

func TestExecuteRun_PreservesJSONRenderingForNonFileArrays(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"iter", "items"}, []map[string]interface{}{
		{"name": "alpha"},
	})

	node := newTestAnswerNode(vp, "{{#iter.items#}}")
	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	answer := result.Outputs["answer"].(string)
	expected := `[{"name":"alpha"}]`
	if answer != expected {
		t.Fatalf("answer = %q, want %q", answer, expected)
	}
}

func TestTemplateStreamParts_PreservesTemplateOrderAcrossStaticAndVariables(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"llm", "text"}, "LLM result")
	vp.Add([]string{"code", "result"}, "Code result")

	node := newTestAnswerNode(vp, "prefix\n{{#llm.text#}}\nmiddle\n{{#code.result#}}\nsuffix")

	parts := node.templateStreamParts(node.NodeData.Answer, "")

	expected := []string{
		"prefix\n",
		"LLM result",
		"\nmiddle\n",
		"Code result",
		"\nsuffix",
	}
	assertStringSlicesEqual(t, parts, expected)
}

func TestStreamAnswer_EmitsChunksInTemplateOrder(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"llm", "text"}, "ABCDE")
	vp.Add([]string{"code", "result"}, "FGHIJ")

	node := newTestAnswerNode(vp, "P{{#llm.text#}}M{{#code.result#}}S")
	node.NodeData.Streaming = &StreamingConfig{
		Enabled:   true,
		ChunkSize: 2,
	}

	eventChan := make(chan *shared.NodeEventCh, 32)
	err := node.streamAnswer(context.Background(), eventChan, node.NodeData.Answer, "PABCDEFGHIS")
	if err != nil {
		t.Fatalf("streamAnswer returned error: %v", err)
	}
	close(eventChan)

	var got []string
	for event := range eventChan {
		streamEvent, ok := event.Data.(*shared.RunStreamChunkEvent)
		if !ok {
			t.Fatalf("event.Data type = %T, want *shared.RunStreamChunkEvent", event.Data)
		}
		got = append(got, streamEvent.ChunkContent)
	}

	expected := []string{"P", "AB", "CD", "E", "M", "FG", "HI", "J", "S"}
	assertStringSlicesEqual(t, got, expected)
}

func TestTemplateStreamParts_AcademicAnalysisTemplatePreservesBusinessSectionOrder(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "name"}, "Alice")
	vp.Add([]string{"start", "nianji"}, "Grade 7")
	vp.Add([]string{"start", "xueke"}, "Math")
	vp.Add([]string{"llm", "text"}, "Logic: 92\nExpression: 88")
	vp.Add([]string{"image", "files"}, []*workflowfile.File{
		workflowfile.NewFile(
			"tenant-1",
			workflowfile.FileTypeImage,
			workflowfile.FileTransferMethodRemoteURL,
			workflowfile.WithFilename("radar.png"),
			workflowfile.WithMimeType("image/png"),
			workflowfile.WithRemoteURL("https://example.com/radar.png"),
		),
	})
	vp.Add([]string{"code", "result"}, "Keep algebra drills twice a week.")

	template := `# One-Click Academic Analysis

## Student Information
Name: {{#start.name#}}
Grade: {{#start.nianji#}}
Subject: {{#start.xueke#}}

## Six-Dimension Scores
{{#llm.text#}}

## Radar Chart
{{#image.files#}}

## Recommendations
{{#code.result#}}`

	node := newTestAnswerNode(vp, template)
	parts := node.templateStreamParts(template, "")

	expected := []string{
		"# One-Click Academic Analysis\n\n## Student Information\nName: ",
		"Alice",
		"\nGrade: ",
		"Grade 7",
		"\nSubject: ",
		"Math",
		"\n\n## Six-Dimension Scores\n",
		"Logic: 92\nExpression: 88",
		"\n\n## Radar Chart\n",
		"![radar.png](https://example.com/radar.png)",
		"\n\n## Recommendations\n",
		"Keep algebra drills twice a week.",
	}
	assertStringSlicesEqual(t, parts, expected)
}

func TestExecuteRun_AcademicAnalysisTemplateKeepsStaticSectionsWhenOptionalValueMissing(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "name"}, "Alice")

	template := `## Student Information
Name: {{#start.name#}}

## Recommendations
{{#code.result#}}`

	node := newTestAnswerNode(vp, template)
	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	answer, ok := result.Outputs["answer"].(string)
	if !ok {
		t.Fatalf("answer output type = %T, want string", result.Outputs["answer"])
	}

	expected := `## Student Information
Name: Alice

## Recommendations
`
	if answer != expected {
		t.Fatalf("answer = %q, want %q", answer, expected)
	}
}

func newTestAnswerNode(vp *entities.VariablePool, answerTemplate string) *Node {
	return &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "answer-node",
			NodeType:          shared.Answer,
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Answer: answerTemplate,
		},
	}
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%q want=%q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; got=%q want=%q", i, got[i], want[i], got, want)
		}
	}
}

func containsPointerAddress(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '0' && s[i+1] == 'x' {
			return true
		}
	}
	return false
}
