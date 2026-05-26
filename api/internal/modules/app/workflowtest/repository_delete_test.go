package workflowtest

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newWorkflowTestMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func TestRepositoryDeleteCasesScopesByAgentAndIDs(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "workflow_test_cases" WHERE agent_id = $1 AND id IN ($2,$3)`)).
		WithArgs("agent-1", "case-1", "case-2").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := repo.DeleteCases(context.Background(), "agent-1", []string{"case-1", "case-2"})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteCasesRejectsEmptySelection(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))

	err := service.DeleteCases(context.Background(), "agent-1", []string{" ", ""})

	require.Error(t, err)
	require.Contains(t, err.Error(), "case_ids is required")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveScenariosRejectsDeletingAssignedScenario(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	now := testNow()

	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "售前咨询", "", "manual", 2, now, now).
		AddRow("scenario-2", "agent-1", "订单查询", "", "manual", 0, now, now))

	items, err := service.SaveScenarios(context.Background(), "agent-1", SaveScenariosRequest{
		Scenarios: []SaveScenarioItemRequest{{
			ID:          "scenario-2",
			Name:        "订单查询",
			Description: "",
		}},
	})

	require.Nil(t, items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scenario has assigned cases")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveScenariosAllowsDeletingUnassignedScenario(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	now := testNow()

	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "售前咨询", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "订单查询", "", "manual", 0, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workflow_test_cases" WHERE agent_id = $1 AND scenario_id = $2`)).
		WithArgs("agent-1", "scenario-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenarios" SET "description"=$1,"name"=$2,"updated_at"=$3 WHERE agent_id = $4 AND id = $5`)).
		WithArgs("", "订单查询", sqlmock.AnyArg(), "agent-1", "scenario-2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "workflow_test_scenarios" WHERE agent_id = $1 AND id = $2`)).
		WithArgs("agent-1", "scenario-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenarios" SET "case_count"=$1,"updated_at"=$2 WHERE agent_id = $3`)).
		WithArgs(0, sqlmock.AnyArg(), "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_cases" WHERE agent_id = $1 ORDER BY created_at DESC`)).
		WithArgs("agent-1").
		WillReturnRows(caseRows(now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-2", "agent-1", "订单查询", "", "manual", 0, now, now))

	items, err := service.SaveScenarios(context.Background(), "agent-1", SaveScenariosRequest{
		Scenarios: []SaveScenarioItemRequest{{
			ID:          "scenario-2",
			Name:        "订单查询",
			Description: "",
		}},
	})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "scenario-2", items[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}
