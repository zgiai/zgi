package migrations

import (
	"os"
	"strings"
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPersonalKeyLegacyQuotaMigrationContract(t *testing.T) {
	for _, required := range []string{
		"DROP CONSTRAINT IF EXISTS chk_llm_tenant_api_keys_quota_limit",
		"quota_limit > 0",
		"SET quota_limit = 0, remain_quota = 0",
		"principal_type IS NOT NULL OR principal_id IS NOT NULL OR access_grant_id IS NOT NULL",
		"quota_limit IS NOT NULL AND quota_limit = 0 AND remain_quota = 0",
	} {
		if !strings.Contains(allowZeroOnlyForPrincipalBoundKeysSQL+backfillPersonalKeyLegacyQuotaSQL+guardPersonalKeyLegacyQuotaSQL, required) {
			t.Fatalf("missing legacy quota protection: %s", required)
		}
	}
	if !strings.Contains(rollbackPersonalKeyLegacyQuotaSQL, "RAISE EXCEPTION") || strings.Contains(rollbackPersonalKeyLegacyQuotaSQL, "UPDATE") {
		t.Fatal("rollback must not restore unlimited personal quotas")
	}
}

// Run only against a disposable PostgreSQL database. No existing table is
// dropped; the entire fixture and migration are rolled back after the test.
func TestPersonalKeyLegacyQuotaMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to a disposable PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	mustExec(t, tx, `CREATE TABLE public.llm_organization_api_keys (
		id text PRIMARY KEY, principal_type text, principal_id text, access_grant_id text,
		quota_limit bigint, remain_quota bigint NOT NULL DEFAULT 0, used_quota bigint NOT NULL DEFAULT 0,
		CONSTRAINT chk_llm_tenant_api_keys_quota_limit CHECK (quota_limit IS NULL OR quota_limit > 0)
	)`)
	mustExec(t, tx, `INSERT INTO public.llm_organization_api_keys VALUES
		('personal', 'user', 'user-a', 'grant-a', NULL, 9, 7),
		('legacy-unlimited', NULL, NULL, NULL, NULL, 0, 11),
		('legacy-bounded', NULL, NULL, NULL, 100, 75, 25),
		('orphan-grant', NULL, NULL, 'grant-b', NULL, 0, 0)`)
	// Force the final ADD CONSTRAINT to fail and prove the earlier constraint
	// replacement and backfill roll back together.
	mustExec(t, tx, `ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT llm_api_keys_principal_legacy_quota_check CHECK (TRUE)`)
	if err := upGuardPersonalKeyLegacyQuota(mschema.New(tx)); err == nil {
		t.Fatal("migration unexpectedly succeeded with a conflicting guard constraint")
	}
	var unchanged int64
	if err := tx.Raw(`SELECT COUNT(*) FROM public.llm_organization_api_keys
		WHERE id = 'personal' AND quota_limit IS NULL AND remain_quota = 9`).Scan(&unchanged).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged != 1 {
		t.Fatal("failed migration did not atomically restore the personal key row")
	}
	mustExec(t, tx, `ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS llm_api_keys_principal_legacy_quota_check`)
	if err := upGuardPersonalKeyLegacyQuota(mschema.New(tx)); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := tx.Raw(`SELECT COUNT(*) FROM public.llm_organization_api_keys WHERE
		(id = 'personal' AND quota_limit = 0 AND remain_quota = 0 AND used_quota = 7) OR
		(id = 'legacy-unlimited' AND quota_limit IS NULL AND used_quota = 11) OR
		(id = 'legacy-bounded' AND quota_limit = 100 AND remain_quota = 75 AND used_quota = 25) OR
		(id = 'orphan-grant' AND quota_limit = 0 AND remain_quota = 0)`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("backfill changed legacy quotas or usage counters: matched %d rows", count)
	}
	for _, update := range []string{"quota_limit = NULL", "quota_limit = 100", "remain_quota = 1"} {
		mustExec(t, tx, "SAVEPOINT denied_write")
		if err := tx.Exec("UPDATE public.llm_organization_api_keys SET " + update + " WHERE id = 'personal'").Error; err == nil {
			t.Fatalf("unsafe personal quota write succeeded: %s", update)
		}
		mustExec(t, tx, "ROLLBACK TO SAVEPOINT denied_write")
	}
	mustExec(t, tx, "SAVEPOINT denied_down")
	if err := downGuardPersonalKeyLegacyQuota(mschema.New(tx).AllowDestructive()); err == nil {
		t.Fatal("rollback removed guard while personal records exist")
	}
	mustExec(t, tx, "ROLLBACK TO SAVEPOINT denied_down")
	// Only fixture rows in this rolled-back transaction are removed to test
	// the empty-personal-table down path; never a deployment cleanup step.
	mustExec(t, tx, "DELETE FROM public.llm_organization_api_keys WHERE id IN ('personal', 'orphan-grant')")
	if err := downGuardPersonalKeyLegacyQuota(mschema.New(tx).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	mustExec(t, tx, "SAVEPOINT denied_legacy_zero")
	if err := tx.Exec("UPDATE public.llm_organization_api_keys SET quota_limit = 0 WHERE id = 'legacy-bounded'").Error; err == nil {
		t.Fatal("rollback did not restore the legacy positive quota constraint")
	}
	mustExec(t, tx, "ROLLBACK TO SAVEPOINT denied_legacy_zero")
}
