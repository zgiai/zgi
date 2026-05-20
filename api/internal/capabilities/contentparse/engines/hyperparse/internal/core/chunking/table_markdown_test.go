package chunking

import (
	"strings"
	"testing"
)

func TestMarkdownTableFromNativePayload(t *testing.T) {
	p := map[string]any{
		"row_count":    2,
		"column_count": 2,
		"cells": []any{
			map[string]any{"row": 0, "col": 0, "text": "Name"},
			map[string]any{"row": 0, "col": 1, "text": "Value"},
			map[string]any{"row": 1, "col": 0, "text": "a|b"},
			map[string]any{"row": 1, "col": 1, "text": "1"},
		},
	}
	md := MarkdownTableFromNativePayload(p)
	if !strings.Contains(md, "| Name |") || !strings.Contains(md, "| --- |") {
		t.Fatalf("unexpected md:\n%s", md)
	}
	if !strings.Contains(md, `a\|b`) {
		t.Fatalf("pipe in cell should be escaped: %s", md)
	}
}
