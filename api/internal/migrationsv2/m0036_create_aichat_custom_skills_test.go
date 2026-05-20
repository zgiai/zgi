package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestM0036CreateAIChatCustomSkills_Migrate(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS public\.aichat_custom_skills \(.+skill_id VARCHAR\(128\) NOT NULL.+display JSONB NOT NULL DEFAULT '\{\}'::jsonb.+manifest JSONB NOT NULL DEFAULT '\{\}'::jsonb.+deleted_at TIMESTAMPTZ.+\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE UNIQUE INDEX IF NOT EXISTS idx_aichat_custom_skills_org_skill_active.+ON public\.aichat_custom_skills\(organization_id, skill_id\).+WHERE deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE INDEX IF NOT EXISTS idx_aichat_custom_skills_org_status.+ON public\.aichat_custom_skills\(organization_id, status\).+WHERE deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0036_create_aichat_custom_skills().Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestM0036CreateAIChatCustomSkills_Rollback(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	mock.ExpectExec(`DROP INDEX IF EXISTS public\.idx_aichat_custom_skills_org_status`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DROP INDEX IF EXISTS public\.idx_aichat_custom_skills_org_skill_active`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DROP TABLE IF EXISTS public\.aichat_custom_skills`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := M0036_create_aichat_custom_skills().Rollback(db); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
