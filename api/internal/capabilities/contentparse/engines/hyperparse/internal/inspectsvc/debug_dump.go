package inspectsvc

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const envInspectDebugDumpDir = "CONTENT_PARSE_UI_INSPECT_DEBUG_DUMP_DIR"

var inspectDebugDumpStemSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type inspectFailureDebugState struct {
	Filename       string
	Mode           string
	Stage          string
	TaskID         string
	SizeBytes      int
	PageCount      int
	CountSource    string
	PDFVersion     string
	PreferFull     bool
	Progressive    bool
	Err            error
	FullDoc        map[string]any
	PlannedPages   []int
	ProcessedPages []int
	AppliedPages   []int
}

type inspectDebugDumpError struct {
	err      error
	stage    string
	dumpPath string
}

func InspectDebugDumpDir() string {
	return contentParseEnv(envInspectDebugDumpDir)
}

func (e *inspectDebugDumpError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	extras := make([]string, 0, 2)
	if e.stage != "" {
		extras = append(extras, "stage="+e.stage)
	}
	if e.dumpPath != "" {
		extras = append(extras, "debug_dump_path="+e.dumpPath)
	}
	if len(extras) == 0 {
		return e.err.Error()
	}
	return fmt.Sprintf("%s (%s)", e.err.Error(), strings.Join(extras, ", "))
}

func (e *inspectDebugDumpError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func maybeAttachInspectDebugArtifacts(result map[string]any, filename string, taskID string, progressive bool) {
	if len(result) == 0 {
		return
	}
	snapshot := buildInspectDebugSnapshot(result)
	result["debug_snapshot"] = snapshot
	dumpPath, err := writeInspectDebugDump(snapshot, filename, taskID, progressive)
	if err != nil {
		result["debug_dump_error"] = err.Error()
		log.Printf("[ui.inspect] debug_dump error progressive=%v task_id=%s err=%v", progressive, taskID, err)
		return
	}
	if dumpPath != "" {
		result["debug_dump_path"] = dumpPath
		log.Printf("[ui.inspect] debug_dump wrote progressive=%v task_id=%s path=%q", progressive, taskID, dumpPath)
	}
}

func attachInspectFailureDebugDump(state inspectFailureDebugState) error {
	if state.Err == nil {
		return nil
	}
	dumpPath := maybeWriteInspectFailureDebugDump(state)
	if dumpPath == "" {
		return state.Err
	}
	return &inspectDebugDumpError{
		err:      state.Err,
		stage:    state.Stage,
		dumpPath: dumpPath,
	}
}

func buildInspectDebugSnapshot(result map[string]any) map[string]any {
	snapshot := map[string]any{
		"version": "v1",
	}
	copySnapshotField(snapshot, result, "filename")
	copySnapshotField(snapshot, result, "mode")
	copySnapshotField(snapshot, result, "duration_ms")
	copySnapshotField(snapshot, result, "timing_breakdown")
	copySnapshotField(snapshot, result, "page_count")
	copySnapshotField(snapshot, result, "recognition_source")
	copySnapshotField(snapshot, result, "suggest_vlm")
	copySnapshotField(snapshot, result, "image_like_pdf")
	copySnapshotField(snapshot, result, "business_doc_vlm_hint")
	copySnapshotField(snapshot, result, "route_decision")
	copySnapshotField(snapshot, result, "route_debug")
	copySnapshotField(snapshot, result, "result_scope")
	copySnapshotField(snapshot, result, "coverage_pages")
	copySnapshotField(snapshot, result, "merge_report")

	vlm := map[string]any{}
	copySnapshotField(vlm, result, "vlm_model")
	copySnapshotField(vlm, result, "vlm_render_engine")
	copySnapshotField(vlm, result, "vlm_pipeline_error")
	copySnapshotField(vlm, result, "vlm_fast_preview")
	copySnapshotField(vlm, result, "vlm_fast_preview_pages")
	copySnapshotField(vlm, result, "vlm_fast_preview_total_pages")
	copySnapshotField(vlm, result, "vlm_rendered_pages")
	if len(vlm) > 0 {
		snapshot["vlm"] = vlm
	}

	if fullDoc, ok := result["full_document"].(map[string]any); ok {
		if doc, ok := fullDoc["document"].(map[string]any); ok {
			fullDocRoute := map[string]any{}
			copySnapshotField(fullDocRoute, doc, "route_decision")
			copySnapshotField(fullDocRoute, doc, "page_route_candidates")
			copySnapshotField(fullDocRoute, doc, "suggest_vlm")
			copySnapshotField(fullDocRoute, doc, "image_like_pdf")
			copySnapshotField(fullDocRoute, doc, "business_doc_vlm_hint")
			copySnapshotField(fullDocRoute, doc, "force_vlm")
			if len(fullDocRoute) > 0 {
				snapshot["full_document_route"] = fullDocRoute
			}
		}
	}

	return snapshot
}

func buildInspectFailureDebugSnapshot(state inspectFailureDebugState) map[string]any {
	result := map[string]any{
		"filename": state.Filename,
		"mode":     state.Mode,
	}
	if state.PageCount > 0 {
		result["page_count"] = state.PageCount
	}
	if state.FullDoc != nil {
		result["full_document"] = state.FullDoc
		if routeDecision := routeDecisionFromFullDoc(state.FullDoc); routeDecision != nil {
			result["route_decision"] = routeDecision
		}
		result["route_debug"] = buildRouteDebug(state.FullDoc, state.PlannedPages, state.ProcessedPages, state.AppliedPages)
	}
	snapshot := buildInspectDebugSnapshot(result)
	snapshot["failure"] = true
	if state.Stage != "" {
		snapshot["stage"] = state.Stage
	}
	if state.Err != nil {
		snapshot["error"] = state.Err.Error()
	}
	if state.SizeBytes >= 0 {
		snapshot["size_bytes"] = state.SizeBytes
	}
	if state.CountSource != "" {
		snapshot["count_source"] = state.CountSource
	}
	if state.PDFVersion != "" {
		snapshot["pdf_version"] = state.PDFVersion
	}
	if state.PreferFull {
		snapshot["prefer_full"] = true
	}
	return snapshot
}

func copySnapshotField(dst map[string]any, src map[string]any, key string) {
	if dst == nil || src == nil {
		return
	}
	val, ok := src[key]
	if !ok || val == nil {
		return
	}
	dst[key] = val
}

func writeInspectDebugDump(snapshot map[string]any, filename string, taskID string, progressive bool) (string, error) {
	dir := InspectDebugDumpDir()
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create debug dump dir: %w", err)
	}

	prefix := fmt.Sprintf("%s-%s-", sanitizeInspectDebugDumpStem(filename), inspectDebugRunKind(progressive))
	tmpFile, err := os.CreateTemp(dir, prefix+"*.json")
	if err != nil {
		return "", fmt.Errorf("create debug dump file: %w", err)
	}
	path := tmpFile.Name()
	payload := map[string]any{
		"version":     "v1",
		"captured_at": time.Now().UTC().Format(time.RFC3339Nano),
		"progressive": progressive,
		"snapshot":    snapshot,
	}
	if taskID != "" {
		payload["task_id"] = taskID
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("marshal debug dump: %w", err)
	}
	if _, err := tmpFile.Write(raw); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write debug dump: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close debug dump: %w", err)
	}
	return path, nil
}

func maybeWriteInspectFailureDebugDump(state inspectFailureDebugState) string {
	snapshot := buildInspectFailureDebugSnapshot(state)
	dumpPath, err := writeInspectDebugDump(snapshot, state.Filename, state.TaskID, state.Progressive)
	if err != nil {
		log.Printf("[ui.inspect] debug_dump error progressive=%v task_id=%s stage=%s err=%v", state.Progressive, state.TaskID, state.Stage, err)
		return ""
	}
	if dumpPath != "" {
		log.Printf("[ui.inspect] debug_dump wrote progressive=%v task_id=%s stage=%s path=%q", state.Progressive, state.TaskID, state.Stage, dumpPath)
	}
	return dumpPath
}

func sanitizeInspectDebugDumpStem(filename string) string {
	stem := strings.TrimSpace(filename)
	if stem != "" {
		stem = filepath.Base(stem)
		stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	}
	stem = inspectDebugDumpStemSanitizer.ReplaceAllString(stem, "_")
	stem = strings.Trim(stem, "._-")
	if stem == "" {
		stem = "inspect"
	}
	if len(stem) > 48 {
		stem = stem[:48]
	}
	return stem
}

func inspectDebugRunKind(progressive bool) string {
	if progressive {
		return "progressive"
	}
	return "sync"
}
