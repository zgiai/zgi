package repository

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestQueryModelUsageByAppTypeKeepsKnownTypesAndBucketsUnknownTypes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE llm_usage_bills (
			organization_id TEXT NOT NULL,
			app_type TEXT,
			request_created_at DATETIME NOT NULL,
			status TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL,
			completion_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			official_points INTEGER NOT NULL,
			private_points INTEGER NOT NULL,
			total_points INTEGER NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create usage bill table: %v", err)
	}

	requestTime := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	knownTypes := []string{
		"workflow",
		"dataset",
		"agent",
		"aichat",
		"image-runtime",
		"data_library_file",
		"prompt_optimizer",
		"prompt_playground",
		"automation_task_draft",
	}
	for _, appType := range knownTypes {
		insertModelUsageAppTypeBill(t, db, requestTime, appType)
	}
	for _, appType := range []any{nil, "", "unknown", "web-app", "future_app_type"} {
		insertModelUsageAppTypeBill(t, db, requestTime, appType)
	}

	repo := &statisticsRepositoryImpl{db: db}
	filters := modelUsageFilters{
		OrganizationID: "org-1",
		StartTime:      requestTime.Add(-time.Hour).Unix(),
		EndTime:        requestTime.Add(time.Hour).Unix(),
	}
	rows, err := repo.queryModelUsageByAppType(context.Background(), filters)
	if err != nil {
		t.Fatalf("query usage by app type: %v", err)
	}

	got := make(map[string]modelUsageAppTypeRow, len(rows))
	var totalAttempts, totalTokens, totalPoints int64
	for _, row := range rows {
		got[row.AppType] = row
		totalAttempts += row.AttemptCount
		totalTokens += row.TotalTokens
		totalPoints += row.TotalPoints
	}
	if len(got) != len(knownTypes)+1 {
		t.Fatalf("app type groups = %#v, want %d known groups plus unknown", got, len(knownTypes))
	}
	for _, appType := range knownTypes {
		if got[appType].AttemptCount != 1 {
			t.Fatalf("%s attempt count = %d, want 1", appType, got[appType].AttemptCount)
		}
	}
	if got["unknown"].AttemptCount != 5 {
		t.Fatalf("unknown attempt count = %d, want 5", got["unknown"].AttemptCount)
	}
	if got["unknown"].TotalTokens != 30 || got["unknown"].TotalPoints != 50 {
		t.Fatalf(
			"unknown totals = tokens:%d points:%d, want tokens:30 points:50",
			got["unknown"].TotalTokens,
			got["unknown"].TotalPoints,
		)
	}
	if totalAttempts != 14 || totalTokens != 84 || totalPoints != 140 {
		t.Fatalf(
			"group totals = attempts:%d tokens:%d points:%d, want attempts:14 tokens:84 points:140",
			totalAttempts,
			totalTokens,
			totalPoints,
		)
	}

	items := buildModelUsageByAppTypeItems(rows, totalPoints)
	var shareTotal float64
	for _, item := range items {
		shareTotal += item.PointsShare
	}
	if math.Abs(shareTotal-1) > 1e-9 {
		t.Fatalf("points share total = %f, want 1", shareTotal)
	}

	unknown := "unknown"
	filters.AppType = &unknown
	rows, err = repo.queryModelUsageByAppType(context.Background(), filters)
	if err != nil {
		t.Fatalf("query unknown app type: %v", err)
	}
	if len(rows) != 1 || rows[0].AppType != "unknown" || rows[0].AttemptCount != 5 {
		t.Fatalf("unknown rows = %#v, want one aggregated group with 5 attempts", rows)
	}
}

func insertModelUsageAppTypeBill(t *testing.T, db *gorm.DB, requestTime time.Time, appType any) {
	t.Helper()
	if err := db.Table(usageBillTable).Create(map[string]any{
		"organization_id":    "org-1",
		"app_type":           appType,
		"request_created_at": requestTime,
		"status":             "success",
		"prompt_tokens":      2,
		"completion_tokens":  4,
		"total_tokens":       6,
		"official_points":    10,
		"private_points":     0,
		"total_points":       10,
	}).Error; err != nil {
		t.Fatalf("insert usage bill: %v", err)
	}
}

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
