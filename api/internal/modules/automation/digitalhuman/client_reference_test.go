package digitalhumantask

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestTaskClientReferenceIsKeptFilteredAndEchoed(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	create := func(reference string) (*digitalhuman.Task, error) {
		return service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
			Name: "Weekly digest", Goal: "Summarize the week", ClientReference: reference,
			Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
		}, digitalhuman.TaskMutationContext{OperationID: uuid.New()})
	}

	task, err := create("  crm-ticket-42  ")
	if err != nil || task.ClientReference != "crm-ticket-42" {
		t.Fatalf("create with reference = %#v, %v", task, err)
	}
	if _, err := create("bad\nreference"); err == nil {
		t.Fatal("control characters accepted")
	}
	if _, err := create(strings.Repeat("x", maxClientReferenceRunes+1)); err == nil {
		t.Fatal("over-long reference accepted")
	}
	if _, err := create(""); err != nil {
		t.Fatalf("task without reference: %v", err)
	}

	listed, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{ClientReference: "crm-ticket-42"})
	if err != nil || listed.Total != 1 || listed.Items[0].ID != task.ID {
		t.Fatalf("filter by reference = %#v, %v", listed, err)
	}

	// The reference reaches the notice that feeds the inbox and webhooks.
	var row automationmodel.AutomationTask
	if err := service.db.First(&row, "id = ?", task.ID.String()).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	notice, _ := reportNotice(&taskReportRow{
		ID: uuid.NewString(), OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, DigitalHumanID: pointerValue(row.SubjectID), TaskID: row.ID,
	}, &row)
	if notice.ClientReference != "crm-ticket-42" {
		t.Fatalf("notice reference = %q", notice.ClientReference)
	}

	// An update replaces it; an empty value clears it.
	empty := ""
	updated, err := service.UpdateTask(t.Context(), access, task.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: task.Revision, ClientReference: &empty}, digitalhuman.TaskMutationContext{OperationID: uuid.New()})
	if err != nil || updated.ClientReference != "" {
		t.Fatalf("clear reference = %#v, %v", updated, err)
	}
}
