package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestConsoleFilesActionDecisionMatchesChineseReadIntentWithSelectedFile(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "\u8bf7\u603b\u7ed3\u8fd9\u4e2a\u6587\u4ef6",
		RuntimeContext: "route=/console/files capabilities=file.read",
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "page",
					"resource_id":   "console.files",
					"metadata": map[string]interface{}{
						"selected_file_ids": "file-1",
					},
					"capability_ids": []interface{}{"file.read"},
				},
				map[string]interface{}{
					"resource_type": "file",
					"resource_id":   "file-1",
					"title":         "notes.txt",
					"metadata": map[string]interface{}{
						"selected": true,
						"file_id":  "file-1",
					},
				},
				map[string]interface{}{
					"resource_type": "file",
					"resource_id":   "file-2",
					"title":         "other.txt",
					"metadata": map[string]interface{}{
						"selected": false,
						"file_id":  "file-2",
					},
				},
			},
			"capabilities": []interface{}{
				map[string]interface{}{"id": "file.read", "resource_id": "console.files"},
			},
		},
	}

	decision := consoleFilesActionDecisionForParts(parts)
	if !decision.Matched {
		t.Fatalf("Matched = false, want true")
	}
	if got, want := strings.Join(decision.FileIDs, ","), "file-1"; got != want {
		t.Fatalf("FileIDs = %q, want %q", got, want)
	}
}

func TestConsoleFilesActionDecisionAsksWhenNoSelectedFileAmongManyVisible(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "read the selected file",
		RuntimeContext: "route=/console/files capabilities=file.read",
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "page",
					"resource_id":   "console.files",
					"metadata": map[string]interface{}{
						"selected_file_ids": "",
					},
					"capability_ids": []interface{}{"file.read"},
				},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "notes.txt"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "other.txt"},
			},
			"capabilities": []interface{}{
				map[string]interface{}{"id": "file.read", "resource_id": "console.files"},
			},
		},
	}

	decision := consoleFilesActionDecisionForParts(parts)
	if !decision.Matched {
		t.Fatalf("Matched = false, want true")
	}
	if len(decision.FileIDs) != 0 {
		t.Fatalf("FileIDs = %#v, want empty", decision.FileIDs)
	}
}

func TestConsoleFilesActionDecisionMatchesExactVisibleFileName(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "read notes.txt",
		RuntimeContext: "route=/console/files capabilities=file.read",
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "page", "resource_id": "console.files", "capability_ids": []interface{}{"file.read"}},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "notes.txt"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "other.txt"},
			},
			"capabilities": []interface{}{
				map[string]interface{}{"id": "file.read", "resource_id": "console.files"},
			},
		},
	}

	decision := consoleFilesActionDecisionForParts(parts)
	if !decision.Matched {
		t.Fatalf("Matched = false, want true")
	}
	if got, want := strings.Join(decision.FileIDs, ","), "file-1"; got != want {
		t.Fatalf("FileIDs = %q, want %q", got, want)
	}
}

func TestConsoleFilesSemanticActionDecisionResolvesVisibleFileOrdinals(t *testing.T) {
	files := []consoleFilesTestFile{
		{ID: "file-1", Name: "meeting-notes.txt", Extension: "txt", MimeType: "text/plain"},
		{ID: "file-2", Name: "sales-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-3", Name: "product-spec.pdf", Extension: "pdf", MimeType: "application/pdf"},
		{ID: "file-4", Name: "customer-letter.docx", Extension: "docx", MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{ID: "file-5", Name: "sales-q2.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-6", Name: "signed-contract.pdf", Extension: "pdf", MimeType: "application/pdf"},
	}
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "fourth visible file",
			query: "\u5e2e\u6211\u8bfb\u4e00\u4e0b\u7b2c\u56db\u4e2a\u6587\u4ef6\u7684\u5185\u5bb9\uff0c\u7ffb\u8bd1\u4e00\u4e0b",
			want:  "file-4",
		},
		{
			name:  "second visible Excel file",
			query: "\u8bfb\u53d6\u7b2c\u4e8c\u4e2a Excel",
			want:  "file-5",
		},
		{
			name:  "last visible PDF file",
			query: "\u8bfb\u53d6\u6700\u540e\u4e00\u4e2a PDF",
			want:  "file-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := consoleFilesActionDecisionForParts(consoleFilesSemanticTestParts(tt.query, files))
			if !decision.Matched {
				t.Fatalf("Matched = false, want true for query %q", tt.query)
			}
			if got := strings.Join(decision.FileIDs, ","); got != tt.want {
				t.Fatalf("FileIDs = %q, want %q for query %q", got, tt.want, tt.query)
			}
		})
	}
}

func TestResolveConsoleFileIDsFromActionDecisionFallsBackToQueryOrdinal(t *testing.T) {
	files := []consoleFilesTestFile{
		{ID: "file-1", Name: "meeting-notes.txt", Extension: "txt", MimeType: "text/plain"},
		{ID: "file-2", Name: "sales-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-3", Name: "product-spec.pdf", Extension: "pdf", MimeType: "application/pdf"},
		{ID: "file-4", Name: "customer-letter.pdf", Extension: "pdf", MimeType: "application/pdf"},
	}
	parts := consoleFilesSemanticTestParts(
		"\u5e2e\u6211\u7ffb\u8bd1\u4e00\u4e0b\u7b2c\u56db\u4e2a\u6587\u4ef6\u7684\u5185\u5bb9\uff0c\u6458\u8981\u4e00\u4e0b\u4e3b\u8981\u5185\u5bb9\u5373\u53ef",
		files,
	)
	confidence := 0.91
	ids := resolveConsoleFileIDsFromActionDecision(parts, AIChatActionDecision{
		Matched:      true,
		Confidence:   &confidence,
		CapabilityID: consoleFilesActionCapabilityID,
		Intent:       "read_then_translate_and_summarize",
		Postprocess:  []AIChatActionPostprocess{{Type: "translate"}, {Type: "summarize"}},
	})

	if got, want := strings.Join(ids, ","), "file-4"; got != want {
		t.Fatalf("FileIDs = %q, want %q", got, want)
	}
}

func TestConsoleFilesActionPlanRequestPreservesTranslatePostprocess(t *testing.T) {
	files := []consoleFilesTestFile{
		{ID: "file-1", Name: "meeting-notes.txt", Extension: "txt", MimeType: "text/plain"},
		{ID: "file-2", Name: "sales-q1.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-3", Name: "product-spec.pdf", Extension: "pdf", MimeType: "application/pdf"},
		{ID: "file-4", Name: "customer-letter.docx", Extension: "docx", MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	}
	parts := consoleFilesSemanticTestParts(
		"\u5e2e\u6211\u8bfb\u4e00\u4e0b\u7b2c\u56db\u4e2a\u6587\u4ef6\u7684\u5185\u5bb9\uff0c\u7ffb\u8bd1\u4e00\u4e0b",
		files,
	)
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		parts:        parts,
	}

	req := consoleFilesActionPlanRequest(prepared, []string{"file-4"}, AIChatActionDecision{
		Matched:      true,
		CapabilityID: consoleFilesActionCapabilityID,
		Intent:       "read_then_translate",
		Postprocess:  []AIChatActionPostprocess{{Type: "translate"}},
	})
	if req.CapabilityID != consoleFilesActionCapabilityID {
		t.Fatalf("CapabilityID = %q, want %q", req.CapabilityID, consoleFilesActionCapabilityID)
	}
	if got, want := strings.Join(stringSliceFromAny(req.Arguments["file_ids"]), ","), "file-4"; got != want {
		t.Fatalf("file_ids = %q, want %q", got, want)
	}
	if includeContent, ok := req.Arguments["include_content"].(bool); !ok || !includeContent {
		t.Fatalf("include_content = %#v, want true", req.Arguments["include_content"])
	}
	if !containsTranslateInstruction(req.Arguments) && !containsTranslateInstruction(req.Metadata) {
		t.Fatalf("arguments/metadata = %#v / %#v, want retained translate postprocess instruction", req.Arguments, req.Metadata)
	}
}

func TestConsoleFilesSemanticActionDecisionDoesNotExecuteAmbiguousVisibleCandidates(t *testing.T) {
	parts := consoleFilesSemanticTestParts("\u8bfb\u53d6 report", []consoleFilesTestFile{
		{ID: "file-pdf", Name: "report.pdf", Extension: "pdf", MimeType: "application/pdf"},
		{ID: "file-xlsx", Name: "report.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{ID: "file-txt", Name: "notes.txt", Extension: "txt", MimeType: "text/plain"},
	})

	decision := consoleFilesActionDecisionForParts(parts)
	if !decision.Matched {
		t.Fatalf("Matched = false, want true so the UI can ask for confirmation")
	}
	if len(decision.FileIDs) != 0 {
		t.Fatalf("FileIDs = %#v, want empty until the user confirms one candidate", decision.FileIDs)
	}
}

func TestConsoleFilesActionDecisionIgnoresOtherPages(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "read this file",
		RuntimeContext: "route=/console/agents capabilities=file.read",
		RawOperationContext: map[string]interface{}{
			"selected_file_ids": "file-1",
			"capabilities": []interface{}{
				map[string]interface{}{"id": "file.read"},
			},
		},
	}

	decision := consoleFilesActionDecisionForParts(parts)
	if decision.Matched {
		t.Fatalf("Matched = true, want false")
	}
}

func TestConsoleFilesActionDecisionRequiresFileReadCapability(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "read this file",
		RuntimeContext: "route=/console/files",
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "page", "resource_id": "console.files"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "notes.txt"},
			},
		},
	}

	decision := consoleFilesActionDecisionForParts(parts)
	if decision.Matched {
		t.Fatalf("Matched = true, want false")
	}
}

func TestConsoleFilesReadActionRouterYieldsToFileReaderSkill(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		parts: consoleFilesSemanticTestParts("read the fourth file", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-2", Name: "two.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-3", Name: "three.txt", Extension: "txt", MimeType: "text/plain"},
			{ID: "file-4", Name: "four.txt", Extension: "txt", MimeType: "text/plain"},
		}),
	}

	prepared.parts.SkillMode = skillModeAuto
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	if !shouldRouteConsoleFilesReadThroughSkillRuntime(prepared) {
		t.Fatal("shouldRouteConsoleFilesReadThroughSkillRuntime() = false, want true with file-reader skill enabled")
	}
	actionRuntime := &failingConsoleFilesActionRuntime{t: t}
	svc := &service{actionRuntime: actionRuntime}
	result, handled, err := svc.runConsoleFilesActionIfMatched(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("runConsoleFilesActionIfMatched() error = %v, want nil", err)
	}
	if handled {
		t.Fatalf("handled = true, want false so skill runtime can process file.read")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if actionRuntime.planCalls != 0 || actionRuntime.executeCalls != 0 {
		t.Fatalf("action runtime calls = plan:%d execute:%d, want 0", actionRuntime.planCalls, actionRuntime.executeCalls)
	}

	prepared.parts.SkillIDs = []string{skills.SkillCalculator}
	if shouldRouteConsoleFilesReadThroughSkillRuntime(prepared) {
		t.Fatal("shouldRouteConsoleFilesReadThroughSkillRuntime() = true, want false without file-reader skill")
	}

	prepared.parts.SkillMode = skillModeDisabled
	prepared.parts.SkillIDs = []string{skills.SkillFileReader}
	if shouldRouteConsoleFilesReadThroughSkillRuntime(prepared) {
		t.Fatal("shouldRouteConsoleFilesReadThroughSkillRuntime() = true, want false when skills are disabled")
	}
}

type failingConsoleFilesActionRuntime struct {
	t            *testing.T
	planCalls    int
	executeCalls int
}

func (f *failingConsoleFilesActionRuntime) ListCapabilities(ctx context.Context, scope actionservice.Scope) ([]actionservice.CapabilityManifest, error) {
	return nil, nil
}

func (f *failingConsoleFilesActionRuntime) PlanAction(ctx context.Context, scope actionservice.Scope, req actiondto.ActionPlanRequest) (*actionservice.ActionRunView, error) {
	f.planCalls++
	f.t.Fatalf("PlanAction called for file read despite file-reader skill being enabled")
	return nil, nil
}

func (f *failingConsoleFilesActionRuntime) GetActionRun(ctx context.Context, scope actionservice.Scope, id uuid.UUID) (*actionservice.ActionRunView, error) {
	return nil, nil
}

func (f *failingConsoleFilesActionRuntime) ConfirmAction(ctx context.Context, scope actionservice.Scope, id uuid.UUID, req actiondto.ConfirmActionRequest) (*actionservice.ActionRunView, error) {
	return nil, nil
}

func (f *failingConsoleFilesActionRuntime) ExecuteAction(ctx context.Context, scope actionservice.Scope, id uuid.UUID, req actiondto.ExecuteActionRequest) (*actionservice.ActionRunView, error) {
	f.executeCalls++
	f.t.Fatalf("ExecuteAction called for file read despite file-reader skill being enabled")
	return nil, nil
}

func TestConsoleFilesAssetCapabilityMatchesDeleteCapability(t *testing.T) {
	operationContext := map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "file",
				"resource_id":   "file-1",
				"title":         "old.pdf",
				"capabilities": []interface{}{
					map[string]interface{}{"id": "file.delete", "risk": "high"},
				},
			},
		},
	}

	if !hasConsoleFilesAssetCapability("route=/console/files", operationContext) {
		t.Fatal("hasConsoleFilesAssetCapability() = false, want true for file.delete")
	}
	if hasConsoleFilesReadCapability("route=/console/files", operationContext) {
		t.Fatal("hasConsoleFilesReadCapability() = true, want false for delete-only capability")
	}
}

func TestConsoleFilesActionDecisionDoesNotMatchProfileReadCapability(t *testing.T) {
	parts := &chatRequestParts{
		Query:          "read this file",
		RuntimeContext: "route=/console/files capabilities=profile.read",
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "page", "resource_id": "console.files", "capability_ids": []interface{}{"profile.read"}},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "notes.txt"},
			},
			"capabilities": []interface{}{
				map[string]interface{}{"id": "profile.read", "resource_id": "console.files"},
			},
		},
	}

	decision := consoleFilesActionDecisionForParts(parts)
	if decision.Matched {
		t.Fatalf("Matched = true, want false")
	}
}

func TestActionRunResponseForMetadataIsParseableByFrontend(t *testing.T) {
	now := time.Unix(1700000000, 0)
	conversationID := uuid.New()
	messageID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	run := &actionmodel.ActionRun{
		ID:                   runID,
		OrganizationID:       uuid.New(),
		AccountID:            uuid.New(),
		ConversationID:       &conversationID,
		MessageID:            &messageID,
		Intent:               consoleFilesActionIntent,
		CapabilityID:         consoleFilesActionCapabilityID,
		Title:                "Read selected file",
		Status:               actionmodel.ActionRunStatusCompleted,
		RiskLevel:            actionmodel.RiskLevelLow,
		RequiresConfirmation: false,
		Resources:            map[string]interface{}{"items": []interface{}{map[string]interface{}{"type": "file", "id": "file-1"}}},
		Arguments:            map[string]interface{}{"file_ids": []interface{}{"file-1"}},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	step := &actionmodel.ActionStep{
		ID:                   stepID,
		RunID:                runID,
		StepKey:              "execute",
		CapabilityID:         consoleFilesActionCapabilityID,
		Title:                "Read selected file",
		Status:               actionmodel.ActionStepStatusDone,
		RiskLevel:            actionmodel.RiskLevelLow,
		RequiresConfirmation: false,
		Output: map[string]interface{}{"files": []map[string]interface{}{
			{"id": "file-1", "name": "notes.txt", "content_preview": "hello"},
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	view := &actionservice.ActionRunView{Run: run, Steps: []*actionmodel.ActionStep{step}}

	resp := actionRunResponseForMetadata(view)
	if resp.ID != runID.String() {
		t.Fatalf("ID = %q, want %q", resp.ID, runID.String())
	}
	if resp.Status == "" || resp.Title == "" || len(resp.Steps) != 1 {
		t.Fatalf("frontend required fields missing: %#v", resp)
	}
	if resp.ConfirmationStatus != "not_required" {
		t.Fatalf("ConfirmationStatus = %q, want not_required", resp.ConfirmationStatus)
	}
	if got := resp.Steps[0].Output["files"]; got == nil {
		t.Fatalf("step output files missing: %#v", resp.Steps[0].Output)
	}
}

func TestConsoleFilesAnswerFromFailedRunKeepsUsefulError(t *testing.T) {
	errText := "file file-1 not found"
	view := &actionservice.ActionRunView{Run: &actionmodel.ActionRun{
		Status: actionmodel.ActionRunStatusFailed,
		Error:  &errText,
	}}

	answer := consoleFilesAnswerFromRun(view)
	if !strings.Contains(answer, errText) {
		t.Fatalf("answer = %q, want to contain %q", answer, errText)
	}
}

type consoleFilesTestFile struct {
	ID        string
	Name      string
	Extension string
	MimeType  string
	Selected  bool
}

func consoleFilesSemanticTestParts(query string, files []consoleFilesTestFile) *chatRequestParts {
	resources := make([]interface{}, 0, len(files)+1)
	selectedIDs := make([]string, 0)
	for _, file := range files {
		if file.Selected {
			selectedIDs = append(selectedIDs, file.ID)
		}
	}
	resources = append(resources, map[string]interface{}{
		"resource_type": "page",
		"resource_id":   "console.files",
		"title":         "console.files",
		"href":          "/console/files",
		"capability_ids": []interface{}{
			consoleFilesActionCapabilityID,
		},
		"metadata": map[string]interface{}{
			"page":               "console.files",
			"route":              "/console/files",
			"selected_file_ids":  strings.Join(selectedIDs, ","),
			"visible_file_count": len(files),
		},
	})
	for _, file := range files {
		resources = append(resources, map[string]interface{}{
			"resource_type": "file",
			"resource_id":   file.ID,
			"title":         file.Name,
			"subtitle":      file.Extension,
			"href":          "/console/files",
			"source":        "Files page",
			"status":        "available",
			"capability_ids": []interface{}{
				consoleFilesActionCapabilityID,
			},
			"metadata": map[string]interface{}{
				"page":      "console.files",
				"file_id":   file.ID,
				"selected":  file.Selected,
				"name":      file.Name,
				"extension": file.Extension,
				"mime_type": file.MimeType,
			},
		})
	}
	operationContext := map[string]interface{}{
		"schema":    "zgi.aichat.operation_context.v1",
		"version":   1,
		"resources": resources,
		"capabilities": []interface{}{
			map[string]interface{}{
				"id":            consoleFilesActionCapabilityID,
				"title":         "Read file",
				"resource_id":   "console.files",
				"resource_type": "page",
				"risk":          "low",
				"status":        "available",
			},
		},
		"risk_summary": map[string]interface{}{
			"level":                 "low",
			"requires_confirmation": false,
		},
	}
	return &chatRequestParts{
		Query:               query,
		RuntimeContext:      "route=/console/files capabilities=file.read",
		RawOperationContext: operationContext,
		OperationContext:    operationContext,
	}
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringMetadataValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		text := stringMetadataValue(value)
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func containsTranslateInstruction(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		return strings.Contains(text, "translate") || strings.Contains(text, "translation") || strings.Contains(text, "\u7ffb\u8bd1")
	case map[string]interface{}:
		for key, item := range typed {
			if containsTranslateInstruction(key) || containsTranslateInstruction(item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if containsTranslateInstruction(item) {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if containsTranslateInstruction(item) {
				return true
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			if containsTranslateInstruction(item) {
				return true
			}
		}
	}
	return false
}
