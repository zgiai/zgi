package tools

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
)

func openWorkflowConnectorVerifierDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	previous := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	return db, mock
}
