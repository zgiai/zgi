package workflowtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGenerateCasesRunsOneGeneratorRequestPerCase(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 7, 7, 14, 40, 0, 0, time.UTC)
	scenarioRow := func() *sqlmock.Rows {
		return scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now)
	}

	expectListScenarios(mock, "agent-1", scenarioRow())
	expectListScenarios(mock, "agent-1", scenarioRow())
	expectListCases(mock, "agent-1", caseRows(now))
	for i := 1; i <= 3; i++ {
		expectListScenarios(mock, "agent-1", scenarioRow())
		expectCreateCase(mock, fmt.Sprintf("case-%d", i))
		expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-1", 1)
	}
	generator := &fakeCaseGenerator{}

	result, err := service.GenerateCases(ctx, "agent-1", GenerateCasesRequest{
		Count:       3,
		ScenarioIDs: []string{"scenario-1"},
	}, generator)

	require.NoError(t, err)
	require.Len(t, generator.requests, 3)
	for _, item := range generator.requests {
		require.Equal(t, 1, item.Count)
	}
	require.Len(t, result.Items, 3)
	require.NoError(t, mock.ExpectationsWereMet())
}
