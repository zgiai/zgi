package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
)

func TestDefaultDocumentProcessRulesUseParagraphSlidingWindowDefaults(t *testing.T) {
	rules := defaultDocumentProcessRules()
	segmentation := rules["segmentation"].(map[string]interface{})
	subchunkSegmentation := rules["subchunk_segmentation"].(map[string]interface{})

	if got := segmentation["max_tokens"]; got != indexing.DefaultParagraphParentMaxChars {
		t.Fatalf("parent max = %v, want %d", got, indexing.DefaultParagraphParentMaxChars)
	}
	if got := segmentation["chunk_overlap"]; got != indexing.DefaultParagraphParentOverlapChars {
		t.Fatalf("parent overlap = %v, want %d", got, indexing.DefaultParagraphParentOverlapChars)
	}
	if got := subchunkSegmentation["max_tokens"]; got != indexing.DefaultParagraphChildMaxChars {
		t.Fatalf("child max = %v, want %d", got, indexing.DefaultParagraphChildMaxChars)
	}
	if got := subchunkSegmentation["chunk_overlap"]; got != indexing.DefaultParagraphChildOverlapChars {
		t.Fatalf("child overlap = %v, want %d", got, indexing.DefaultParagraphChildOverlapChars)
	}
}
