package common

import "testing"

func TestEnrichStructuredOutput(t *testing.T) {
	doc := &DocumentResult{
		DocID:     "doc-1",
		FileName:  "a.pdf",
		PageCount: 1,
		Markdown:  "md",
		Source:    "mineru:pipeline",
		Chunks: []Chunk{
			{
				ID:        "c1",
				Type:      "text",
				Subtype:   "",
				Page:      0,
				Text:      "hello",
				Markdown:  "hello",
				Ordinal:   0,
				Precision: "reliable",
				ParentID:  "p1",
				Payload: map[string]any{
					"preview_data_url": "data:image/png;base64,abc",
					"vlm_caption":      "caption",
				},
			},
		},
	}

	got := EnrichStructuredOutput(doc)
	if got == nil || got.ExtractOutput == nil {
		t.Fatalf("extract_output should not be nil")
	}
	if got.ExtractOutput.Source != "mineru:pipeline" {
		t.Fatalf("source=%q", got.ExtractOutput.Source)
	}
	if len(got.ExtractOutput.Elements) != 1 {
		t.Fatalf("elements=%d", len(got.ExtractOutput.Elements))
	}
	el := got.ExtractOutput.Elements[0]
	if el.Ordinal != 1 {
		t.Fatalf("ordinal=%d want=1", el.Ordinal)
	}
	if el.Content != "hello" {
		t.Fatalf("content=%q", el.Content)
	}
	if el.Metadata == nil || el.Metadata["parent_id"] != "p1" {
		t.Fatalf("metadata.parent_id mismatch: %#v", el.Metadata)
	}
	payload, _ := el.Metadata["payload"].(map[string]any)
	if payload["preview_data_url"] != "data:image/png;base64,abc" {
		t.Fatalf("metadata.payload missing preview: %#v", el.Metadata)
	}
	if got.ExtractOutput.Metadata["doc_id"] != "doc-1" {
		t.Fatalf("metadata.doc_id=%v", got.ExtractOutput.Metadata["doc_id"])
	}
}
