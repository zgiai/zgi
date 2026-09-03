package workflow

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
)

func TestWorkflowDraftSaveRollsBackBeforeGraphWriteWhenTargetDeleted(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	org := uuid.New()
	w := &Workflow{ID: uuid.NewString(), AgentID: uuid.NewString(), TenantID: uuid.NewString(), Version: "draft", Graph: `{"nodes":[]}`}
	s := &WorkflowService{repo: NewWorkflowRepository(db), workflowDB: db, connectorBindings: bindings.NewGormRepository(db)}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "organization_id" FROM "workspaces"`).WithArgs(w.TenantID, "normal", 1).WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow(org))
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs("connector-workflow-target:" + org.String() + ":" + w.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT owner.id FROM agents AS owner`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if err := s.persistWorkflowWithBindings(t.Context(), w, nil, false); err == nil {
		t.Fatal("deleted target accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestWorkflowPublishedSaveIsImmutableBeforeTransaction(t *testing.T) {
	sqlDB, mock, _ := sqlmock.New()
	defer sqlDB.Close()
	db, _ := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	s := &WorkflowService{repo: NewWorkflowRepository(db), workflowDB: db, connectorBindings: bindings.NewGormRepository(db)}
	if err := s.persistWorkflowWithBindings(t.Context(), &Workflow{Version: "v1"}, nil, false); err == nil {
		t.Fatal("published mutation accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
