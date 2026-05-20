package inspectsvc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildInspectDebugSnapshot(t *testing.T) {
	result := map[string]any{
		"filename":              "claim-form.pdf",
		"mode":                  "relaxed",
		"duration_ms":           int64(9123),
		"timing_breakdown":      map[string]any{"native_ms": int64(2100), "render_ms": int64(3300), "total_ms": int64(9123)},
		"page_count":            3,
		"recognition_source":    "vlm",
		"suggest_vlm":           true,
		"image_like_pdf":        false,
		"business_doc_vlm_hint": true,
		"route_decision": map[string]any{
			"recommended_mode": "vlm_candidate",
		},
		"route_debug": map[string]any{
			"candidate_pages": []int{2, 3},
		},
		"result_scope": "preview_partial",
		"coverage_pages": map[string]any{
			"processed": []int{2},
		},
		"merge_report": map[string]any{
			"strategy": "preview_partial_vlm",
		},
		"vlm_model":          "qwen-vl-plus",
		"vlm_render_engine":  "pdftoppm",
		"vlm_pipeline_error": "partial failure",
		"vlm_fast_preview":   true,
		"vlm_rendered_pages": 2,
		"full_document": map[string]any{
			"document": map[string]any{
				"route_decision": map[string]any{
					"recommended_mode": "vlm_candidate",
				},
				"page_route_candidates": []map[string]any{
					{"page_index": 2, "recommended_mode": "vlm_candidate"},
				},
			},
		},
	}

	snapshot := buildInspectDebugSnapshot(result)
	if got, _ := snapshot["version"].(string); got != "v1" {
		t.Fatalf("version=%q", got)
	}
	if got, _ := snapshot["filename"].(string); got != "claim-form.pdf" {
		t.Fatalf("filename=%q", got)
	}
	if got, _ := snapshot["duration_ms"].(int64); got != 9123 {
		t.Fatalf("duration_ms=%d", got)
	}
	timing, ok := snapshot["timing_breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("timing_breakdown=%T", snapshot["timing_breakdown"])
	}
	if got, _ := timing["render_ms"].(int64); got != 3300 {
		t.Fatalf("timing_breakdown.render_ms=%d", got)
	}
	vlm, ok := snapshot["vlm"].(map[string]any)
	if !ok {
		t.Fatalf("vlm=%T", snapshot["vlm"])
	}
	if got, _ := vlm["vlm_model"].(string); got != "qwen-vl-plus" {
		t.Fatalf("vlm_model=%q", got)
	}
	fullDocRoute, ok := snapshot["full_document_route"].(map[string]any)
	if !ok {
		t.Fatalf("full_document_route=%T", snapshot["full_document_route"])
	}
	if _, ok := fullDocRoute["page_route_candidates"]; !ok {
		t.Fatalf("page_route_candidates missing: %v", fullDocRoute)
	}
}

func TestWriteInspectDebugDumpDisabled(t *testing.T) {
	t.Setenv(envInspectDebugDumpDir, "")
	path, err := writeInspectDebugDump(map[string]any{"version": "v1"}, "sample.pdf", "", false)
	if err != nil {
		t.Fatalf("writeInspectDebugDump error: %v", err)
	}
	if path != "" {
		t.Fatalf("path=%q, want empty", path)
	}
}

func TestWriteInspectDebugDump(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envInspectDebugDumpDir, dir)

	snapshot := map[string]any{
		"version":  "v1",
		"filename": "medical claim #1.pdf",
	}
	path, err := writeInspectDebugDump(snapshot, "medical claim #1.pdf", "task-123", true)
	if err != nil {
		t.Fatalf("writeInspectDebugDump error: %v", err)
	}
	if path == "" {
		t.Fatal("expected dump path")
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("dir=%q", filepath.Dir(path))
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "medical_claim_1-progressive-") {
		t.Fatalf("basename=%q", base)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal dump: %v", err)
	}
	if got, _ := payload["task_id"].(string); got != "task-123" {
		t.Fatalf("task_id=%q", got)
	}
	if got, _ := payload["progressive"].(bool); !got {
		t.Fatalf("progressive=%v", payload["progressive"])
	}
	inner, ok := payload["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot=%T", payload["snapshot"])
	}
	if got, _ := inner["filename"].(string); got != "medical claim #1.pdf" {
		t.Fatalf("snapshot.filename=%q", got)
	}
}

func TestMaybeAttachInspectDebugArtifacts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envInspectDebugDumpDir, dir)
	result := map[string]any{
		"filename":           "statement.pdf",
		"mode":               "relaxed",
		"recognition_source": "native",
		"route_debug": map[string]any{
			"candidate_pages": []int{},
		},
	}

	maybeAttachInspectDebugArtifacts(result, "statement.pdf", "", false)

	if _, ok := result["debug_snapshot"].(map[string]any); !ok {
		t.Fatalf("debug_snapshot=%T", result["debug_snapshot"])
	}
	path, ok := result["debug_dump_path"].(string)
	if !ok || path == "" {
		t.Fatalf("debug_dump_path=%v", result["debug_dump_path"])
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat debug_dump_path: %v", err)
	}
}

func TestBuildInspectFailureDebugSnapshot(t *testing.T) {
	state := inspectFailureDebugState{
		Filename:    "claim.pdf",
		Mode:        "relaxed",
		Stage:       "render_pages",
		SizeBytes:   2048,
		PageCount:   4,
		CountSource: "catalog",
		PDFVersion:  "1.7",
		PreferFull:  true,
		Progressive: true,
		Err:         errors.New("renderer failed"),
		FullDoc: map[string]any{
			"document": map[string]any{
				"route_decision": map[string]any{
					"recommended_mode": "vlm_candidate",
				},
				"page_route_candidates": []map[string]any{
					{"page_index": 2, "recommended_mode": "vlm_candidate"},
				},
			},
		},
		PlannedPages: []int{2, 3},
	}

	snapshot := buildInspectFailureDebugSnapshot(state)
	if failure, _ := snapshot["failure"].(bool); !failure {
		t.Fatalf("failure=%v", snapshot["failure"])
	}
	if got, _ := snapshot["stage"].(string); got != "render_pages" {
		t.Fatalf("stage=%q", got)
	}
	if got, _ := snapshot["error"].(string); got != "renderer failed" {
		t.Fatalf("error=%q", got)
	}
	if got, _ := snapshot["count_source"].(string); got != "catalog" {
		t.Fatalf("count_source=%q", got)
	}
	if got, _ := snapshot["pdf_version"].(string); got != "1.7" {
		t.Fatalf("pdf_version=%q", got)
	}
	debug, ok := snapshot["route_debug"].(map[string]any)
	if !ok {
		t.Fatalf("route_debug=%T", snapshot["route_debug"])
	}
	if got := debug["planned_vlm_pages"].([]int); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("planned_vlm_pages=%v", got)
	}
}

func TestAttachInspectFailureDebugDump(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envInspectDebugDumpDir, dir)

	wrapped := attachInspectFailureDebugDump(inspectFailureDebugState{
		Filename:    "statement.pdf",
		Mode:        "strict",
		Stage:       "inspect_basic",
		TaskID:      "task-456",
		SizeBytes:   12,
		Progressive: true,
		Err:         errors.New("bad xref"),
	})
	if wrapped == nil {
		t.Fatal("wrapped error is nil")
	}
	if !strings.Contains(wrapped.Error(), "debug_dump_path=") {
		t.Fatalf("wrapped error=%q", wrapped.Error())
	}
	if !strings.Contains(wrapped.Error(), "stage=inspect_basic") {
		t.Fatalf("wrapped error=%q", wrapped.Error())
	}
	var dumpErr *inspectDebugDumpError
	if !errors.As(wrapped, &dumpErr) {
		t.Fatalf("errors.As failed for %T", wrapped)
	}
	if dumpErr.dumpPath == "" {
		t.Fatalf("dumpPath=%q", dumpErr.dumpPath)
	}
	if _, err := os.Stat(dumpErr.dumpPath); err != nil {
		t.Fatalf("stat dump path: %v", err)
	}
}
