package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
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
