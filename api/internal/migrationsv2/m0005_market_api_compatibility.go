package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0005_market_api_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2MarketAPICompatibilityID,
		Migrate: func(tx *gorm.DB) error {
			applied, err := migrationIDApplied(tx, legacyMarketAPIRuntimeTablesID)
			if err != nil {
				return err
			}

			if !applied &&
				!tableExists(tx, "market_api_installations") &&
				!tableExists(tx, "market_api_daily_usage_counters") &&
				!tableExists(tx, "market_api_call_logs") {
				return nil
			}

			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS market_api_installations (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					api_id UUID NOT NULL,
					status VARCHAR(20) NOT NULL DEFAULT 'enabled',
					installed_by UUID NOT NULL,
					installed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
					config_json JSONB
				);

				CREATE UNIQUE INDEX IF NOT EXISTS uniq_market_api_installations_org_api
					ON market_api_installations(organization_id, api_id);

				CREATE INDEX IF NOT EXISTS idx_market_api_installations_org_status
					ON market_api_installations(organization_id, status);

				CREATE TABLE IF NOT EXISTS market_api_daily_usage_counters (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					api_id UUID NOT NULL,
					usage_date DATE NOT NULL,
					success_count INTEGER NOT NULL DEFAULT 0,
					free_quota_used_count INTEGER NOT NULL DEFAULT 0,
					paid_count INTEGER NOT NULL DEFAULT 0,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE UNIQUE INDEX IF NOT EXISTS uniq_market_api_daily_usage_org_api_date
					ON market_api_daily_usage_counters(organization_id, api_id, usage_date);

				CREATE INDEX IF NOT EXISTS idx_market_api_daily_usage_org_date
					ON market_api_daily_usage_counters(organization_id, usage_date DESC);

				CREATE TABLE IF NOT EXISTS market_api_call_logs (
					id UUID PRIMARY KEY,
					request_id VARCHAR(64) NOT NULL,
					organization_id UUID NOT NULL,
					api_id UUID NOT NULL,
					api_slug VARCHAR(100) NOT NULL,
					api_key_id VARCHAR(64) NOT NULL,
					installation_id UUID,
					call_status VARCHAR(30) NOT NULL,
					http_status_code INTEGER,
					base_credit_cost BIGINT NOT NULL DEFAULT 0,
					discount_credit_cost BIGINT NOT NULL DEFAULT 0,
					final_credit_cost BIGINT NOT NULL DEFAULT 0,
					pricing_rule_code VARCHAR(60),
					quota_window_date DATE,
					credit_transaction_id UUID,
					upstream_latency_ms INTEGER,
					error_message TEXT,
					metadata_json JSONB,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE UNIQUE INDEX IF NOT EXISTS uniq_market_api_call_logs_request_id
					ON market_api_call_logs(request_id);

				CREATE INDEX IF NOT EXISTS idx_market_api_call_logs_org_created
					ON market_api_call_logs(organization_id, created_at DESC);

				CREATE INDEX IF NOT EXISTS idx_market_api_call_logs_api_created
					ON market_api_call_logs(api_id, created_at DESC);

				CREATE INDEX IF NOT EXISTS idx_market_api_call_logs_api_key_created
					ON market_api_call_logs(api_key_id, created_at DESC);
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
