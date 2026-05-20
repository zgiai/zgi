package migrations

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestM0132_seed_qwen_image_pricing_DropsModelProviderFKBeforeInsert(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	mock.ExpectExec(`INSERT INTO llm_protocols`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`ALTER TABLE llm_models DROP CONSTRAINT IF EXISTS fk_model_provider`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO llm_provider_protocols`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO llm_provider_protocols`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO llm_models`).
		WithArgs("qwen-image-2.0", "Qwen Image 2.0", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := M0132_seed_qwen_image_pricing().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
