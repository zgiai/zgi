package digitalhuman

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"gorm.io/datatypes"
)

func TestTaskReportCarriesOnlyItsRunsFiles(t *testing.T) {
	run, protected, _ := newBackgroundTestFile(t, SurfaceTask)
	manager := toolfile.GlobalToolFileManager
	create := func(tenant, name string) *toolfile.ToolFile {
		t.Helper()
		file, err := manager.CreateFileByRaw(t.Context(), toolfile.CreateFileByRawParams{UserID: uuid.NewString(), TenantID: tenant,
			Filename: &name, FileData: []byte(name), MimeType: "text/plain"})
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	generated := create(run.OrganizationID.String(), "chart.txt")
	foreign := create(uuid.NewString(), "other-tenant.txt")
	trace, _ := json.Marshal(map[string]any{"generated_files": []map[string]any{
		{"tool_file_id": protected.ID}, {"artifact_id": "tool_file:" + generated.ID}, {"tool_file_id": foreign.ID},
	}})
	run.ExecutionTrace = datatypes.JSON(trace)
	runs := newFakeRunStore()
	runs.runs[run.ID] = run
	service := &RuntimeService{store: runs}
	scope := Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID}
	report := &TaskReport{ID: uuid.New(), DigitalHumanID: run.DigitalHumanID, DigitalHumanRunID: &run.ID}

	files, err := service.TaskReportFiles(t.Context(), scope, report)
	if err != nil || len(files) != 2 || files[0].ID != protected.ID || files[1].ID != generated.ID {
		t.Fatalf("files = %#v, %v", files, err)
	}
	for _, file := range []*toolfile.ToolFile{protected, generated} {
		read, data, err := service.ReadTaskReportFile(t.Context(), scope, report, file.ID)
		if err != nil || read == nil || len(data) == 0 {
			t.Fatalf("read %s = %v", file.Name, err)
		}
	}
	if _, _, err := service.ReadTaskReportFile(t.Context(), scope, report, foreign.ID); err == nil {
		t.Fatal("a file of another tenant was readable through the report")
	}

	// A report that chose files carries only those.
	report.FileIDs = []uuid.UUID{uuid.MustParse(generated.ID)}
	files, err = service.TaskReportFiles(t.Context(), scope, report)
	if err != nil || len(files) != 1 || files[0].ID != generated.ID {
		t.Fatalf("chosen files = %#v, %v", files, err)
	}
	if _, _, err := service.ReadTaskReportFile(t.Context(), scope, report, protected.ID); err == nil {
		t.Fatal("a file the report left out was readable")
	}

	// Another workspace cannot reach the run, so the report carries nothing.
	if files, _ := service.TaskReportFiles(t.Context(), Scope{OrganizationID: run.OrganizationID, WorkspaceID: uuid.New()}, report); len(files) != 0 {
		t.Fatalf("files from another workspace = %#v", files)
	}
}
