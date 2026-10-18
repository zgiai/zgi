package runtime

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBlockDigitalHumanTaskUsesSafeTerminalActivity(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	task := &automationmodel.AutomationTask{ID: uuid.NewString(), TaskKind: automationmodel.AutomationTaskKindDigitalHuman, Revision: 3}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "automation_tasks" SET .*work_status.* WHERE id = .*task_kind`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO "automation_task_activities"`).
		WithArgs(sqlmock.AnyArg(), task.ID, sqlmock.AnyArg(), "system", "scheduler", "blocked", "Task blocked before execution", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"changes", "created_at"}).AddRow(`{"reason":"execution_unavailable"}`, nil))
	mock.ExpectCommit()

	if err := (&Executor{db: db}).blockDigitalHumanTask(t.Context(), task); err != nil {
		t.Fatalf("blockDigitalHumanTask() error = %v", err)
	}
	if task.WorkStatus == nil || *task.WorkStatus != automationmodel.AutomationWorkStatusBlocked || task.Status != automationmodel.AutomationTaskStatusPaused || task.Revision != 4 {
		t.Fatalf("blocked task = %#v", task)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
