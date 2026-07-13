package landingai

import "testing"

func TestToExtractOutputMarksStructuredChunks(t *testing.T) {
	extractor := &LandingAIPDFExtractor{filePath: "/tmp/sample.pdf"}
	output := extractor.toExtractOutput(&LandingAIResponse{
		Chunks: []LandingAIChunk{{
			ID:       "chunk-1",
			Markdown: "| Name | Value |\n| --- | --- |\n| A | 1 |",
			Metadata: map[string]interface{}{"type": "table"},
		}},
	})

	if output == nil || len(output.Elements) != 1 {
		t.Fatalf("unexpected output: %#v", output)
	}
	if output.Metadata["structured_elements"] != true {
		t.Fatalf("expected structured element marker: %#v", output.Metadata)
	}
}

func TestToExtractOutputMarkdownFallbackIsNotStructured(t *testing.T) {
	extractor := &LandingAIPDFExtractor{filePath: "/tmp/sample.pdf"}
	output := extractor.toExtractOutput(&LandingAIResponse{Markdown: "plain markdown"})

	if output == nil || len(output.Elements) != 1 {
		t.Fatalf("unexpected output: %#v", output)
	}
	if output.Metadata["structured_elements"] != nil {
		t.Fatalf("markdown fallback must not be marked structured: %#v", output.Metadata)
	}
}
