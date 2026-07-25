package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestQueryModelUsageDailyTrendIncludesChannelTokens(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)

	mock.ExpectQuery(`(?s)official_tokens.*private_tokens`).
		WithArgs("org-1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"date",
			"attempt_count",
			"success_count",
			"failed_count",
			"partial_count",
			"prompt_tokens",
			"completion_tokens",
			"total_tokens",
			"official_tokens",
			"private_tokens",
			"official_points",
			"private_points",
			"total_points",
		}).AddRow("2026-07-23", 2, 2, 0, 0, 400, 600, 1000, 650, 350, 70, 30, 100))

	repository := &statisticsRepositoryImpl{db: db}
	rows, err := repository.queryModelUsageDailyTrend(context.Background(), modelUsageFilters{
		OrganizationID: "org-1",
		StartTime:      1753171200,
		EndTime:        1753257599,
	})

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(650), rows[0].OfficialTokens)
	require.Equal(t, int64(350), rows[0].PrivateTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildModelUsageDailyItemsIncludesChannelTokens(t *testing.T) {
	items := buildModelUsageDailyItems([]modelUsageDailyRow{{
		Date:           "2026-07-23",
		TotalTokens:    1000,
		OfficialTokens: 650,
		PrivateTokens:  350,
	}})

	require.Len(t, items, 1)
	require.Equal(t, int64(650), items[0].OfficialTokens)
	require.Equal(t, int64(350), items[0].PrivateTokens)
}
