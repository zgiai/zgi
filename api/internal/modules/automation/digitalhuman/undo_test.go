package digitalhumantask

import (
	"testing"
	"time"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestADirectChangeCanBeUndoneOnce(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	direct := func() digitalhuman.TaskMutationContext {
		return digitalhuman.TaskMutationContext{ActorType: "digital_human", ActorRef: uuid.NewString(), OperationID: uuid.New(), Source: "console", Undoable: true}
	}
	runAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	once := digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"}
	task, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Lunch break", Goal: "Remind me", Schedule: once}, direct())
	if err != nil || task.WorkStatus != digitalhuman.TaskWorkStatusPending {
		t.Fatalf("direct create = %#v, %v", task, err)
	}
	name := "Lunch break at noon"
	updated, err := service.UpdateTask(t.Context(), access, task.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: task.Revision, Name: &name}, direct())
	if err != nil || updated.Name != name {
		t.Fatalf("direct update = %#v, %v", updated, err)
	}
	undone, err := service.UndoTaskChange(t.Context(), access, task.ID, direct())
	if err != nil || undone.Name != "Lunch break" || undone.Revision != updated.Revision+1 {
		t.Fatalf("undo of the update = %#v, %v", undone, err)
	}
	if _, err := service.UndoTaskChange(t.Context(), access, task.ID, direct()); err == nil {
		t.Fatal("an undo was undone")
	}

	// Undoing a creation cancels the task.
	other, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Stretch", Goal: "Remind me", Schedule: once}, direct())
	if err != nil {
		t.Fatal(err)
	}
	canceled, err := service.UndoTaskChange(t.Context(), access, other.ID, direct())
	if err != nil || canceled.WorkStatus != digitalhuman.TaskWorkStatusCanceled {
		t.Fatalf("undo of the creation = %#v, %v", canceled, err)
	}

	// A change made without undo, or one past the window, cannot be undone.
	kept, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Report", Goal: "Write it", Schedule: once},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UndoTaskChange(t.Context(), access, kept.ID, direct()); err == nil {
		t.Fatal("a change made without undo was undone")
	}
	old, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Later", Goal: "Remind me", Schedule: once}, direct())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.db.Model(&automationmodel.AutomationTaskActivity{}).Where("task_id = ?", old.ID.String()).
		Update("created_at", time.Now().Add(-digitalhuman.TaskUndoWindow-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.UndoTaskChange(t.Context(), access, old.ID, direct()); err == nil {
		t.Fatal("a change past the undo window was undone")
	}
}
