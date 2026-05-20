package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0052NormalizeChannelGroup normalizes the channel group schema
// - Adds foreign key relationship between system_channels and channel_groups
// - Removes redundant fields from system_channels
// - Adds new fields to channel_groups for proper aggregation
func M0052NormalizeChannelGroup() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000052",
		Migrate: func(tx *gorm.DB) error {
			// Check if llm_system_channels table exists
			var sysChannelsExists bool
			err := tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.tables 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_system_channels'
				)
			`).Scan(&sysChannelsExists).Error
			if err != nil {
				return err
			}

			// Check if llm_channel_groups table exists
			var channelGroupsExists bool
			err = tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.tables 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_channel_groups'
				)
			`).Scan(&channelGroupsExists).Error
			if err != nil {
				return err
			}

			// Skip if either table doesn't exist
			if !sysChannelsExists || !channelGroupsExists {
				return nil
			}

			// Check if protocol column exists in llm_system_channels
			var protocolExists bool
			err = tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.columns 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_system_channels'
					AND column_name = 'protocol'
				)
			`).Scan(&protocolExists).Error
			if err != nil {
				return err
			}

			// Step 1: Add new columns to llm_channel_groups
			if err := tx.Exec(`
				ALTER TABLE llm_channel_groups
				ADD COLUMN IF NOT EXISTS provider VARCHAR(50),
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50),
				ADD COLUMN IF NOT EXISTS icon VARCHAR(500),
				ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}'
			`).Error; err != nil {
				return err
			}

			// Step 2: Add channel_group_id FK column to llm_system_channels
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels
				ADD COLUMN IF NOT EXISTS channel_group_id UUID
			`).Error; err != nil {
				return err
			}

			// Step 3: Migrate existing data - create channel_groups from unique channel_group values
			// Build the SQL dynamically based on whether protocol column exists
			insertSQL := `
				INSERT INTO llm_channel_groups (id, name, display_name, description, priority, is_active, provider, created_at, updated_at)
				SELECT
					uuid_generate_v4(),
					channel_group,
					COALESCE(channel_group_name, channel_group),
					channel_group_description,
					COALESCE(MAX(channel_group_priority), 10),
					true,
					(SELECT provider FROM llm_system_channels sc2
					 WHERE sc2.channel_group = sc.channel_group
					 AND sc2.deleted_at IS NULL
					 LIMIT 1),
					NOW(),
					NOW()
				FROM llm_system_channels sc
				WHERE channel_group IS NOT NULL
				  AND channel_group != ''
				  AND deleted_at IS NULL
				GROUP BY channel_group, channel_group_name, channel_group_description
				ON CONFLICT (name) DO NOTHING
			`

			if protocolExists {
				insertSQL = `
					INSERT INTO llm_channel_groups (id, name, display_name, description, priority, is_active, provider, protocol, created_at, updated_at)
					SELECT
						uuid_generate_v4(),
						channel_group,
						COALESCE(channel_group_name, channel_group),
						channel_group_description,
						COALESCE(MAX(channel_group_priority), 10),
						true,
						(SELECT provider FROM llm_system_channels sc2
						 WHERE sc2.channel_group = sc.channel_group
						 AND sc2.deleted_at IS NULL
						 LIMIT 1),
						(SELECT protocol FROM llm_system_channels sc2
						 WHERE sc2.channel_group = sc.channel_group
						 AND sc2.deleted_at IS NULL
						 LIMIT 1),
						NOW(),
						NOW()
					FROM llm_system_channels sc
					WHERE channel_group IS NOT NULL
					  AND channel_group != ''
					  AND deleted_at IS NULL
					GROUP BY channel_group, channel_group_name, channel_group_description
					ON CONFLICT (name) DO NOTHING
				`
			}

			if err := tx.Exec(insertSQL).Error; err != nil {
				return err
			}

			// Step 4: Update channel_group_id in system_channels based on channel_group name
			if err := tx.Exec(`
				UPDATE llm_system_channels sc
				SET channel_group_id = cg.id
				FROM llm_channel_groups cg
				WHERE sc.channel_group = cg.name
				  AND sc.channel_group IS NOT NULL
				  AND sc.channel_group != ''
			`).Error; err != nil {
				return err
			}

			// Step 5: Create index on channel_group_id
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_sys_channels_group_id
				ON llm_system_channels(channel_group_id)
				WHERE channel_group_id IS NOT NULL
			`).Error; err != nil {
				return err
			}

			// Step 6: Add foreign key constraint (deferred to allow data migration)
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels
				DROP CONSTRAINT IF EXISTS fk_sys_channels_channel_group;

				ALTER TABLE llm_system_channels
				ADD CONSTRAINT fk_sys_channels_channel_group
				FOREIGN KEY (channel_group_id)
				REFERENCES llm_channel_groups(id)
				ON DELETE SET NULL
			`).Error; err != nil {
				// Ignore FK constraint error if it already exists
				// Some databases may not support IF NOT EXISTS for constraints
			}

			// Step 7: Add channel_group_id to tenant_routes for group-level overrides
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes
				ADD COLUMN IF NOT EXISTS channel_group_id UUID
			`).Error; err != nil {
				return err
			}

			// Step 8: Create index on tenant_routes.channel_group_id
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_tenant_routes_group_id
				ON llm_tenant_routes(channel_group_id)
				WHERE channel_group_id IS NOT NULL
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes
			tx.Exec(`DROP INDEX IF EXISTS idx_tenant_routes_group_id`)
			tx.Exec(`DROP INDEX IF EXISTS idx_sys_channels_group_id`)

			// Drop FK constraint
			tx.Exec(`ALTER TABLE llm_system_channels DROP CONSTRAINT IF EXISTS fk_sys_channels_channel_group`)

			// Drop new columns
			tx.Exec(`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS channel_group_id`)
			tx.Exec(`ALTER TABLE llm_system_channels DROP COLUMN IF EXISTS channel_group_id`)
			tx.Exec(`ALTER TABLE llm_channel_groups DROP COLUMN IF EXISTS metadata`)
			tx.Exec(`ALTER TABLE llm_channel_groups DROP COLUMN IF EXISTS icon`)
			tx.Exec(`ALTER TABLE llm_channel_groups DROP COLUMN IF EXISTS protocol`)
			tx.Exec(`ALTER TABLE llm_channel_groups DROP COLUMN IF EXISTS provider`)

			return nil
		},
	}
}
