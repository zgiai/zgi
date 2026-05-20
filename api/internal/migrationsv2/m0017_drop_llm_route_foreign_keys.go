package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0017_drop_llm_route_foreign_keys() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropLLMRouteForeignKeysID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE IF EXISTS public.llm_system_channels
					DROP CONSTRAINT IF EXISTS llm_system_channels_credential_id_fkey;

				ALTER TABLE IF EXISTS public.llm_tenant_routes
					DROP CONSTRAINT IF EXISTS llm_tenant_routes_user_credential_id_fkey,
					DROP CONSTRAINT IF EXISTS fk_route_credential,
					DROP CONSTRAINT IF EXISTS fk_tenant_route_credential,
					DROP CONSTRAINT IF EXISTS fk_llm_routes_credential,
					DROP CONSTRAINT IF EXISTS llm_tenant_routes_system_channel_id_fkey;

				ALTER TABLE IF EXISTS public.llm_routes
					DROP CONSTRAINT IF EXISTS llm_routes_user_credential_id_fkey,
					DROP CONSTRAINT IF EXISTS fk_route_credential,
					DROP CONSTRAINT IF EXISTS fk_tenant_route_credential,
					DROP CONSTRAINT IF EXISTS fk_llm_routes_credential,
					DROP CONSTRAINT IF EXISTS llm_routes_system_channel_id_fkey,
					DROP CONSTRAINT IF EXISTS llm_tenant_routes_system_channel_id_fkey;
			`).Error; err != nil {
				return fmt.Errorf("drop llm route foreign keys: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
