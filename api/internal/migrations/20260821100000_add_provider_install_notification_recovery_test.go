package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddProviderInstallNotificationRecoveryMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.integration_oauth_recovery_operations.*provider_install.*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up20260821100000(mschema.New(db)); err != nil {
		t.Fatalf("up20260821100000() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
