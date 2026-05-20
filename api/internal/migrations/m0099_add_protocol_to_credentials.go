package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0099_add_protocol_to_credentials adds protocol column to llm_credentials table
func M0099_add_protocol_to_credentials() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260202000099",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				-- Add protocol column to llm_credentials
				ALTER TABLE llm_credentials 
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50);

				-- Add protocol column to llm_system_credentials
				ALTER TABLE llm_system_credentials 
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50);

				-- Add index for protocol
				CREATE INDEX IF NOT EXISTS idx_credential_protocol ON llm_credentials(protocol);
				CREATE INDEX IF NOT EXISTS idx_sys_credential_protocol ON llm_system_credentials(protocol);

				COMMENT ON COLUMN llm_credentials.protocol IS 'Protocol used by this credential (e.g., openai, anthropic)';
				COMMENT ON COLUMN llm_system_credentials.protocol IS 'Protocol used by this credential (e.g., openai, anthropic)';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				DROP INDEX IF EXISTS idx_credential_protocol;
				DROP INDEX IF EXISTS idx_sys_credential_protocol;
				ALTER TABLE llm_credentials DROP COLUMN IF EXISTS protocol;
				ALTER TABLE llm_system_credentials DROP COLUMN IF EXISTS protocol;
			`).Error
		},
	}
}
