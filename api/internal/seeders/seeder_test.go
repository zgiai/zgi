package seeders

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListExcludesRemovedSystemSettingsSeed(t *testing.T) {
	seeder := NewSeeder(nil, "development")

	files, err := seeder.List()
	if err != nil {
		t.Fatalf("list seeds: %v", err)
	}

	for _, file := range files {
		if strings.Contains(file, "03_system_settings.sql") {
			t.Fatalf("unexpected retired seed file in list: %s", file)
		}
	}
}

func TestHasHistoricalInitialSeedData_UsesLocalBuiltInWorkflowSeeds(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	seeder := NewSeeder(sqlDB, "development")
	expectedSeedCount := len(BuiltInWorkflowSeedScenarios())
	tableExistsQuery := regexp.QuoteMeta(`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name = $1
		)`)

	mock.ExpectQuery(tableExistsQuery).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(DISTINCT id)
		 FROM agents
		 WHERE tenant_id::text = $1
		   AND id::text = ANY($2)`)).
		WithArgs(BuiltInTenantID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedSeedCount))
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("workflows").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(DISTINCT agent_id)
		 FROM workflows
		 WHERE tenant_id::text = $1
		   AND agent_id::text = ANY($2)`)).
		WithArgs(BuiltInTenantID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedSeedCount))

	ok, err := seeder.hasHistoricalInitialSeedData(context.Background())
	if err != nil {
		t.Fatalf("hasHistoricalInitialSeedData returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected historical seed detection to succeed from local built-in workflow tables")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
