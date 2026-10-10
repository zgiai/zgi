package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestAddAppliedCommandIdempotencyKeyMigration(t *testing.T) {
	var migrate func(*gorm.DB) error
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationAddAppliedCommandIdempotencyKeyID {
			migrate = candidate.Migrate
			break
		}
	}
	if migrate == nil {
		t.Fatalf("migration %s is not registered", migrationAddAppliedCommandIdempotencyKeyID)
	}

	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s).*ALTER TABLE "public"\."pre_applied_commands" ADD COLUMN "idempotency_key" varchar\(128\).*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s).*CREATE UNIQUE INDEX "idx_pre_applied_commands_idempotency_key" ON "public"\."pre_applied_commands" \("idempotency_key"\).*`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
