package inspectsvc

import (
	"testing"

	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

func TestBuildInspectNativeTimingDetail(t *testing.T) {
	detail := buildInspectNativeTimingDetail(11, pdforchestrator.FullDocumentTimingBreakdown{
		PageInfosMs:          22,
		RenderSpecsMs:        33,
		TextExtractMs:        44,
		TextExtractDetail:    map[string]any{"geom_scanned_pages": 2},
		ImageExtractMs:       55,
		ParallelExtractMs:    66,
		ChunkingMs:           77,
		TotalMs:              200,
		OutlineExtractMs:     88,
		AnnotationsExtractMs: 99,
		FormsExtractMs:       111,
		AttachmentsExtractMs: 123,
	}, 219)

	if got, _ := detail["inspect_basic_ms"].(int64); got != 11 {
		t.Fatalf("inspect_basic_ms=%d", got)
	}
	if got, _ := detail["text_extract_ms"].(int64); got != 44 {
		t.Fatalf("text_extract_ms=%d", got)
	}
	if got, _ := detail["parse_full_document_ms"].(int64); got != 200 {
		t.Fatalf("parse_full_document_ms=%d", got)
	}
	textExtractDetail, ok := detail["text_extract_detail"].(map[string]any)
	if !ok {
		t.Fatalf("text_extract_detail=%T", detail["text_extract_detail"])
	}
	if got, _ := textExtractDetail["geom_scanned_pages"].(int); got != 2 {
		t.Fatalf("geom_scanned_pages=%d", got)
	}
	if got, _ := detail["other_ms"].(int64); got != 8 {
		t.Fatalf("other_ms=%d", got)
	}
}

func TestBuildInspectNativeTimingDetailOmitsNonPositive(t *testing.T) {
	detail := buildInspectNativeTimingDetail(0, pdforchestrator.FullDocumentTimingBreakdown{}, 0)
	if len(detail) != 0 {
		t.Fatalf("detail=%v", detail)
	}
}
