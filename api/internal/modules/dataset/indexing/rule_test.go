package indexing

import "testing"

func TestParseRuleUsesFineGrainedDefaultSubchunkSegmentation(t *testing.T) {
	rule, err := ParseRule(map[string]interface{}{
		"segmentation": map[string]interface{}{
			"separator":     "\n",
			"max_tokens":    500,
			"chunk_overlap": 50,
		},
	})
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}
	if rule.SubchunkSegmentation == nil {
		t.Fatal("SubchunkSegmentation is nil")
	}
	if rule.SubchunkSegmentation.MaxTokens != 100 {
		t.Fatalf("MaxTokens = %d, want 100", rule.SubchunkSegmentation.MaxTokens)
	}
	if rule.SubchunkSegmentation.ChunkOverlap != 20 {
		t.Fatalf("ChunkOverlap = %d, want 20", rule.SubchunkSegmentation.ChunkOverlap)
	}
	if rule.SubchunkSegmentation.Separator != "\n" {
		t.Fatalf("Separator = %q, want newline", rule.SubchunkSegmentation.Separator)
	}
}
