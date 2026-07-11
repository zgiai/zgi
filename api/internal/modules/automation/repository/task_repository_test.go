package repository

import (
	"context"
	"testing"

	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaskRepositoryCountByStatusScopesToOrganizationAndWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&automationmodel.AutomationTask{}); err != nil {
		t.Fatalf("migrate automation tasks: %v", err)
	}

	insertAutomationTask(t, db, "task-1", "org-1", "workspace-1", automationmodel.AutomationTaskStatusActive)
	insertAutomationTask(t, db, "task-2", "org-1", "workspace-1", automationmodel.AutomationTaskStatusActive)
	insertAutomationTask(t, db, "task-3", "org-1", "workspace-1", automationmodel.AutomationTaskStatusPaused)
	insertAutomationTask(t, db, "task-4", "org-1", "workspace-2", automationmodel.AutomationTaskStatusCompleted)
	insertAutomationTask(t, db, "task-5", "org-2", "workspace-1", automationmodel.AutomationTaskStatusArchived)

	counts, err := NewTaskRepository(db).CountByStatus(context.Background(), nil, automationdto.TaskScope{
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
	})
	if err != nil {
		t.Fatalf("count tasks by status: %v", err)
	}

	if got, want := counts[automationmodel.AutomationTaskStatusActive], int64(2); got != want {
		t.Fatalf("active count = %d, want %d", got, want)
	}
	if got, want := counts[automationmodel.AutomationTaskStatusPaused], int64(1); got != want {
		t.Fatalf("paused count = %d, want %d", got, want)
	}
	if got := counts[automationmodel.AutomationTaskStatusCompleted]; got != 0 {
		t.Fatalf("completed count = %d, want 0", got)
	}
	if got := counts[automationmodel.AutomationTaskStatusArchived]; got != 0 {
		t.Fatalf("archived count = %d, want 0", got)
	}
}

func insertAutomationTask(
	t *testing.T,
	db *gorm.DB,
	id, organizationID, workspaceID string,
	status automationmodel.AutomationTaskStatus,
) {
	t.Helper()
	task := &automationmodel.AutomationTask{
		ID:             id,
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Name:           id,
		Status:         status,
		TriggerType:    automationmodel.AutomationTriggerTypeSchedule,
		ScheduleType:   automationmodel.AutomationScheduleTypeCron,
		Timezone:       "UTC",
		ScheduleConfig: map[string]interface{}{"cron_expr": "0 * * * *"},
		SourceType:     automationmodel.AutomationSourceTypeManual,
		CreatedBy:      "account-1",
		UpdatedBy:      "account-1",
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task %s: %v", id, err)
	}
}
