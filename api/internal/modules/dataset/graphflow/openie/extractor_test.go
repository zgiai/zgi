package openie

import (
	"context"
	"strings"
	"testing"
)

type stubOpenIELLMClient struct {
	lastPrompt string
	response   string
}

func (s *stubOpenIELLMClient) Complete(ctx context.Context, promptText string) (string, error) {
	s.lastPrompt = promptText
	return s.response, nil
}

func TestExtractUsesGoTemplate(t *testing.T) {
	llmClient := &stubOpenIELLMClient{response: `{"entities":[],"triples":[]}`}
	extractor := NewExtractor(llmClient)
	extractor.SetSchema(&Schema{Entities: []EntityDef{{Name: "Person"}}}, true)

	_, err := extractor.Extract(context.Background(), "Example text")
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	for _, expected := range []string{"Example text", `"name": "Person"`, "STRICTLY ONLY"} {
		if !strings.Contains(llmClient.lastPrompt, expected) {
			t.Fatalf("prompt = %q, want %q", llmClient.lastPrompt, expected)
		}
	}
	if strings.Contains(llmClient.lastPrompt, "{{") {
		t.Fatalf("prompt still contains raw template markers: %q", llmClient.lastPrompt)
	}
}
