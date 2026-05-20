package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestM0035CreateAIChatOrganizationSkillConfigs(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS public\.aichat_organization_skill_configs \(.+PRIMARY KEY \(organization_id, skill_id\).+\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_aichat_organization_skill_configs_enabled`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)INSERT INTO public\.aichat_organization_skill_configs .+VALUES \('time'\), \('calculator'\), \('file-generator'\).+ON CONFLICT \(organization_id, skill_id\) DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	if err := M0035_create_aichat_organization_skill_configs().Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestM0035CreateAIChatOrganizationSkillConfigsRollback(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sqlDB.Close()

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	mock.ExpectExec(`DROP INDEX IF EXISTS public\.idx_aichat_organization_skill_configs_enabled`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DROP TABLE IF EXISTS public\.aichat_organization_skill_configs`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0035_create_aichat_organization_skill_configs().Rollback(db); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
