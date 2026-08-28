package indexing

import "testing"

func TestParseRuleUsesFineGrainedDefaultSubchunkSegmentation(t *testing.T) {
	rule, err := ParseRule(map[string]interface{}{
		"segmentation": map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}
	if rule.Segmentation == nil {
		t.Fatal("Segmentation is nil")
	}
	if rule.Segmentation.MaxTokens != DefaultParagraphParentMaxChars {
		t.Fatalf("parent MaxTokens = %d, want %d", rule.Segmentation.MaxTokens, DefaultParagraphParentMaxChars)
	}
	if rule.Segmentation.ChunkOverlap != DefaultParagraphParentOverlapChars {
		t.Fatalf("parent ChunkOverlap = %d, want %d", rule.Segmentation.ChunkOverlap, DefaultParagraphParentOverlapChars)
	}
	if rule.SubchunkSegmentation == nil {
		t.Fatal("SubchunkSegmentation is nil")
	}
	if rule.SubchunkSegmentation.MaxTokens != DefaultParagraphChildMaxChars {
		t.Fatalf("MaxTokens = %d, want %d", rule.SubchunkSegmentation.MaxTokens, DefaultParagraphChildMaxChars)
	}
	if rule.SubchunkSegmentation.ChunkOverlap != DefaultParagraphChildOverlapChars {
		t.Fatalf("ChunkOverlap = %d, want %d", rule.SubchunkSegmentation.ChunkOverlap, DefaultParagraphChildOverlapChars)
	}
	if rule.SubchunkSegmentation.Separator != "\n" {
		t.Fatalf("Separator = %q, want newline", rule.SubchunkSegmentation.Separator)
	}
}

func TestRuntimeRuleBuilderUsesParagraphFallbackSubchunkDefaults(t *testing.T) {
	_, rules := NewRuntimeRuleBuilder().BuildElementGroupRule()
	subchunkSegmentation := rules["subchunk_segmentation"].(map[string]interface{})

	if got := subchunkSegmentation["max_tokens"]; got != DefaultParagraphChildMaxChars {
		t.Fatalf("child max = %v, want %d", got, DefaultParagraphChildMaxChars)
	}
	if got := subchunkSegmentation["chunk_overlap"]; got != DefaultParagraphChildOverlapChars {
		t.Fatalf("child overlap = %v, want %d", got, DefaultParagraphChildOverlapChars)
	}
}
