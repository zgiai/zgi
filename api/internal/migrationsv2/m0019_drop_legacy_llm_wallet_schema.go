package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0019_drop_legacy_llm_wallet_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropLegacyLLMWalletID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS public.llm_redeem_usages;
				DROP TABLE IF EXISTS public.llm_redeem_codes;
				DROP TABLE IF EXISTS public.llm_transactions;
				DROP TABLE IF EXISTS public.llm_organization_balances;
				DROP TABLE IF EXISTS public.llm_tenant_transactions;
				DROP TABLE IF EXISTS public.llm_tenant_balances;
			`).Error; err != nil {
				return fmt.Errorf("drop legacy llm wallet schema: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
