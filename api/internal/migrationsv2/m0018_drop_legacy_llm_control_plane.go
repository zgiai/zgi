package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0018_drop_legacy_llm_control_plane() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropLegacyLLMControlPlaneID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP INDEX IF EXISTS public.idx_tenant_routes_group_id;
				ALTER TABLE IF EXISTS public.llm_routes
					DROP COLUMN IF EXISTS channel_group_id;

				DROP INDEX IF EXISTS public.idx_llm_channel_groups_is_active;
				DROP INDEX IF EXISTS public.idx_llm_channel_groups_name;
				DROP TABLE IF EXISTS public.llm_channel_groups;

				DROP INDEX IF EXISTS public.idx_sys_cred_active;
				DROP INDEX IF EXISTS public.idx_sys_cred_deleted_at;
				DROP INDEX IF EXISTS public.idx_sys_cred_hash;
				DROP INDEX IF EXISTS public.idx_sys_cred_provider;
				DROP TABLE IF EXISTS public.llm_system_credentials;

				DROP TABLE IF EXISTS public.llm_system_channels;
			`).Error; err != nil {
				return fmt.Errorf("drop legacy llm control-plane schema: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
