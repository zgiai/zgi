package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGetWorkspaceStatisticsUsesWorkspaceIDForDatasetCount(t *testing.T) {
	db, mock := newWorkspaceRepositoryMockDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT role, COUNT(role) as count FROM "workspace_members" WHERE workspace_id = $1 GROUP BY "role"`,
	)).
		WithArgs("ws-1").
		WillReturnRows(sqlmock.NewRows([]string{"role", "count"}).
			AddRow("owner", 1).
			AddRow("admin", 1).
			AddRow("normal", 1))
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT count(*) FROM "datasets" WHERE workspace_id = $1`,
	)).
		WithArgs("ws-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT count(*) FROM "agents" WHERE tenant_id = $1 AND deleted_at IS NULL AND is_universal = $2`,
	)).
		WithArgs("ws-1", false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	repo := &workspaceRepository{db: db}
	admins, members, datasets, agents, err := repo.GetWorkspaceStatistics(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceStatistics: %v", err)
	}

	if admins != 2 || members != 1 || datasets != 2 || agents != 1 {
		t.Fatalf(
			"counts = admins:%d members:%d datasets:%d agents:%d, want 2,1,2,1",
			admins,
			members,
			datasets,
			agents,
		)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func newWorkspaceRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock
}
