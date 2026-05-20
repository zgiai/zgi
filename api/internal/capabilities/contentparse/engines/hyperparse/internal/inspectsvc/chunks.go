package inspectsvc

import (
	"fmt"
	"strconv"
	"strings"
)

// CoerceChunkItems converts full_document chunks.items into []map[string]any.
func CoerceChunkItems(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

// JoinVLMChunkTexts joins VLM chunk text fields with newlines.
func JoinVLMChunkTexts(items []map[string]any) string {
	parts := make([]string, 0, len(items))
	for _, c := range items {
		t := strings.TrimSpace(fmt.Sprint(c["text"]))
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

// RemapVLMChunkPages rewrites model-relative page indexes onto actual document page numbers.
func RemapVLMChunkPages(items []map[string]any, pageNumbers []int) {
	if len(items) == 0 || len(pageNumbers) == 0 {
		return
	}
	for _, item := range items {
		page := IntFromChunkAny(item, "page_index")
		if page < 1 {
			page = 1
		}
		if page > len(pageNumbers) {
			page = len(pageNumbers)
		}
		actualPage := pageNumbers[page-1]
		item["page_index"] = actualPage
		sourceTrace, _ := item["source_trace"].(string)
		if strings.HasPrefix(sourceTrace, "vlm:page#") {
			item["source_trace"] = fmt.Sprintf("vlm:page#%d", actualPage)
		}
	}
}

// IntFromChunkAny reads an integer field from a chunk map.
func IntFromChunkAny(c map[string]any, key string) int {
	v, ok := c[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}
