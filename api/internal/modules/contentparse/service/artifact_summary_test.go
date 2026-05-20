package service

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestParseArtifactStorageSummary(t *testing.T) {
	if got := ParseArtifactStorageSummary(nil); len(got) != 0 {
		t.Fatalf("nil artifact summary=%v", got)
	}

	artifact := &contracts.ParseArtifact{
		ArtifactID:   "artifact-1",
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityHigh,
		EngineUsed:   contracts.ParseEngineLocal,
		FallbackUsed: true,
		Text:         "hello",
		Markdown:     "# hello",
		Elements: []contracts.ParsedElement{
			{ID: "el-1"},
			{ID: "el-2"},
		},
		Diagnostics: map[string]any{
			"recognition_source": "native+ocr",
		},
	}
	summary := ParseArtifactStorageSummary(artifact)
	if summary["artifact_id"] != "artifact-1" {
		t.Fatalf("artifact_id=%v", summary["artifact_id"])
	}
	if summary["text_length"] != 5 {
		t.Fatalf("text_length=%v", summary["text_length"])
	}
	if summary["markdown_length"] != 7 {
		t.Fatalf("markdown_length=%v", summary["markdown_length"])
	}
	if summary["element_count"] != 2 {
		t.Fatalf("element_count=%v", summary["element_count"])
	}
	diagnostics, ok := summary["diagnostics"].(map[string]interface{})
	if !ok || diagnostics["recognition_source"] != "native+ocr" {
		t.Fatalf("diagnostics=%v", summary["diagnostics"])
	}
}

func TestParseArtifactDiagnosticsSummary(t *testing.T) {
	if got := ParseArtifactDiagnosticsSummary(nil); got != nil {
		t.Fatalf("nil diagnostics summary=%v", got)
	}
	summary := ParseArtifactDiagnosticsSummary(map[string]any{
		"z_extra":            true,
		"recognition_source": "native+ocr",
		"local_vlm_fallback": map[string]any{"status": "skipped"},
		"ocr_fallback":       map[string]any{"applied": true},
		"ocr_engine":         "tesseract",
		"ocr_strategy":       "auto",
		"ocr_retry_used":     false,
		"ocr_preprocess":     "deskew",
		"local_image_parse":  map[string]any{"figures": 2},
	})
	if summary["recognition_source"] != "native+ocr" {
		t.Fatalf("recognition_source=%v", summary["recognition_source"])
	}
	if !reflect.DeepEqual(summary["keys"], []string{
		"local_image_parse",
		"local_vlm_fallback",
		"ocr_engine",
		"ocr_fallback",
		"ocr_preprocess",
		"ocr_retry_used",
		"ocr_strategy",
		"recognition_source",
		"z_extra",
	}) {
		t.Fatalf("keys=%v", summary["keys"])
	}
}
