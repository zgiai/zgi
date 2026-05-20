package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0059_gdpr_compliance creates tables for GDPR compliance
// - gdpr_audit_logs: Audit trail for GDPR operations
// - data_retention_policies: Configurable data retention rules
// - user_consents: User consent management
// All tables are NEW, no modifications to existing tables
func M0059_gdpr_compliance() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251222000059",
		Migrate: func(tx *gorm.DB) error {
			// 1. GDPR Audit Logs - tracks all GDPR-related operations
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS gdpr_audit_logs (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					action_type VARCHAR(50) NOT NULL,
					actor_id UUID,
					actor_email VARCHAR(255),
					subject_id UUID NOT NULL,
					subject_email VARCHAR(255),
					tenant_id UUID,
					details JSONB DEFAULT '{}',
					ip_address VARCHAR(45),
					user_agent TEXT,
					status VARCHAR(20) NOT NULL DEFAULT 'completed',
					error_message TEXT,
					created_at TIMESTAMP NOT NULL DEFAULT NOW()
				)
			`).Error; err != nil {
				return err
			}

			// Indexes for gdpr_audit_logs
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_gdpr_audit_logs_action_type ON gdpr_audit_logs(action_type);
				CREATE INDEX IF NOT EXISTS idx_gdpr_audit_logs_actor_id ON gdpr_audit_logs(actor_id);
				CREATE INDEX IF NOT EXISTS idx_gdpr_audit_logs_subject_id ON gdpr_audit_logs(subject_id);
				CREATE INDEX IF NOT EXISTS idx_gdpr_audit_logs_tenant_id ON gdpr_audit_logs(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_gdpr_audit_logs_created_at ON gdpr_audit_logs(created_at);
			`).Error; err != nil {
				return err
			}

			// 2. Data Retention Policies - configurable retention rules
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS data_retention_policies (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					data_type VARCHAR(50) NOT NULL,
					description TEXT,
					retention_days INT NOT NULL,
					anonymize_after_days INT,
					hard_delete_after_days INT,
					is_active BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMP NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_data_retention_policies_data_type UNIQUE (data_type)
				)
			`).Error; err != nil {
				return err
			}

			// Seed default retention policies
			if err := tx.Exec(`
				INSERT INTO data_retention_policies (data_type, description, retention_days, anonymize_after_days, hard_delete_after_days, is_active)
				VALUES
					('transactions', 'Wallet transactions (financial records)', 2555, NULL, NULL, true),
					('audit_logs', 'System audit logs', 1095, NULL, NULL, true),
					('api_keys', 'Deleted API keys', 90, 30, 180, true)
				ON CONFLICT (data_type) DO NOTHING
			`).Error; err != nil {
				return err
			}

			// 3. User Consents - consent management
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS user_consents (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					account_id UUID NOT NULL,
					consent_type VARCHAR(50) NOT NULL,
					is_granted BOOLEAN NOT NULL,
					granted_at TIMESTAMP,
					revoked_at TIMESTAMP,
					ip_address VARCHAR(45),
					user_agent TEXT,
					version VARCHAR(20) DEFAULT '1.0',
					created_at TIMESTAMP NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP NOT NULL DEFAULT NOW()
				)
			`).Error; err != nil {
				return err
			}

			// Indexes for user_consents
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_user_consents_account_id ON user_consents(account_id);
				CREATE INDEX IF NOT EXISTS idx_user_consents_consent_type ON user_consents(consent_type);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_user_consents_account_type ON user_consents(account_id, consent_type);
			`).Error; err != nil {
				return err
			}

			// 4. Add gdpr_purged_at column to existing credential tables (safe addition)
			// Using DO $$ block for idempotent column addition
			if err := tx.Exec(`
				DO $$
				BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'llm_system_credentials' AND column_name = 'gdpr_purged_at'
					) THEN
						ALTER TABLE llm_system_credentials ADD COLUMN gdpr_purged_at TIMESTAMP;
					END IF;
				END $$;
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				DO $$
				BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'llm_tenant_credentials' AND column_name = 'gdpr_purged_at'
					) THEN
						ALTER TABLE llm_tenant_credentials ADD COLUMN gdpr_purged_at TIMESTAMP;
					END IF;
				END $$;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop in reverse order
			if err := tx.Exec(`DROP TABLE IF EXISTS user_consents`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS data_retention_policies`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS gdpr_audit_logs`).Error; err != nil {
				return err
			}
			// Note: We don't drop the gdpr_purged_at columns as they may contain data
			return nil
		},
	}
}
