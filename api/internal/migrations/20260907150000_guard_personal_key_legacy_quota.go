package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationGuardPersonalKeyLegacyQuotaID = "20260907150000_guard_personal_key_legacy_quota"

const allowZeroOnlyForPrincipalBoundKeysSQL = `
	ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS chk_llm_tenant_api_keys_quota_limit;
	ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT chk_llm_tenant_api_keys_quota_limit CHECK (
			quota_limit IS NULL
			OR quota_limit > 0
			OR (
				quota_limit = 0
				AND (principal_type IS NOT NULL OR principal_id IS NOT NULL OR access_grant_id IS NOT NULL)
			)
		);
`

const backfillPersonalKeyLegacyQuotaSQL = `
	UPDATE public.llm_organization_api_keys
	SET quota_limit = 0, remain_quota = 0
	WHERE principal_type IS NOT NULL OR principal_id IS NOT NULL OR access_grant_id IS NOT NULL;
`

const guardPersonalKeyLegacyQuotaSQL = `
	ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT llm_api_keys_principal_legacy_quota_check CHECK (
			(principal_type IS NULL AND principal_id IS NULL AND access_grant_id IS NULL)
			OR (quota_limit IS NOT NULL AND quota_limit = 0 AND remain_quota = 0)
		);
`

const rollbackPersonalKeyLegacyQuotaSQL = `
	DO $$
	BEGIN
		IF EXISTS (
			SELECT 1 FROM public.llm_organization_api_keys
			WHERE principal_type IS NOT NULL OR principal_id IS NOT NULL OR access_grant_id IS NOT NULL
		) THEN
			RAISE EXCEPTION 'cannot remove legacy quota guard while personal key records exist';
		END IF;
	END
	$$;
	ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS llm_api_keys_principal_legacy_quota_check;
	ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS chk_llm_tenant_api_keys_quota_limit;
	ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT chk_llm_tenant_api_keys_quota_limit CHECK (
			quota_limit IS NULL OR quota_limit > 0
		);
`

func init() {
	registerSchemaMigration(migrationGuardPersonalKeyLegacyQuotaID, upGuardPersonalKeyLegacyQuota, downGuardPersonalKeyLegacyQuota)
}

func upGuardPersonalKeyLegacyQuota(schema *mschema.Builder) error {
	return schema.DataFix("atomically deny legacy quota authentication for principal-bound keys without changing grant balances or usage", func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// The legacy table rejects zero quota limits. Replace that constraint
			// first, while still allowing zero only for principal-bound rows.
			if err := tx.Exec(allowZeroOnlyForPrincipalBoundKeysSQL).Error; err != nil {
				return err
			}
			if err := tx.Exec(backfillPersonalKeyLegacyQuotaSQL).Error; err != nil {
				return err
			}
			return tx.Exec(guardPersonalKeyLegacyQuotaSQL).Error
		})
	})
}

func downGuardPersonalKeyLegacyQuota(schema *mschema.Builder) error {
	return schema.Raw(rollbackPersonalKeyLegacyQuotaSQL)
}
