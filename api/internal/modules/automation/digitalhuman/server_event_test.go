package digitalhumantask

import (
	"testing"
	"time"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestTriggerServerEventStartsOneRunOfAnActiveTask(t *testing.T) {
	service, dispatcher, access, _ := newTaskService(t)
	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	created, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
		Name: "Server alert", Goal: "Find out why a service went down",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	event := map[string]any{"targets": []any{map[string]any{"name": "nginx", "observed": "failed", "failures": 2}}}

	started, reason, err := service.TriggerServerEvent(t.Context(), created.ID, event)
	if err != nil || !started || reason != "" {
		t.Fatalf("TriggerServerEvent() = %v, %q, %v", started, reason, err)
	}
	if len(dispatcher.runIDs) != 1 {
		t.Fatalf("dispatched runs = %v", dispatcher.runIDs)
	}
	var run automationmodel.AutomationTaskRun
	if err := service.db.Where("id = ?", dispatcher.runIDs[0]).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.TriggerSource != automationmodel.AutomationTriggerSourceServerEvent || run.RuntimeContext["server_event"] == nil {
		t.Fatalf("run = %+v", run)
	}

	started, reason, err = service.TriggerServerEvent(t.Context(), created.ID, event)
	if err != nil || started || reason != ServerEventRunInProgress {
		t.Fatalf("second trigger = %v, %q, %v", started, reason, err)
	}

	if err := service.db.Model(&automationmodel.AutomationTaskRun{}).Where("id = ?", run.ID).Update("status", automationmodel.AutomationTaskRunStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", created.ID).Update("status", automationmodel.AutomationTaskStatusPaused).Error; err != nil {
		t.Fatal(err)
	}
	started, reason, err = service.TriggerServerEvent(t.Context(), created.ID, event)
	if err != nil || started || reason != ServerEventTaskInactive {
		t.Fatalf("paused task trigger = %v, %q, %v", started, reason, err)
	}
}
