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
