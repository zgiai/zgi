package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0037_llm_tenant_routes extends the existing llm_tenant_routes table
// Table was originally created in m0031_add_channel_architecture.go
// This migration adds new columns for:
// - Multi-credential load balancing per route
// - Rate limiting and circuit breaker configuration
// - Status management with state machine
func M0037_llm_tenant_routes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000037",
		Migrate: func(tx *gorm.DB) error {
			// Step 1: Add new columns to existing llm_tenant_routes table
			// Note: Table was created in m0031 with different schema
			alterStatements := []string{
				// Add match_models column (TEXT[] for model pattern matching)
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS match_models TEXT[] DEFAULT '{}'`,

				// Add route_type if not exists (m0031 uses 'type' column)
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS route_type VARCHAR(20) DEFAULT 'PRIVATE'`,

				// Add credential_ids for multi-credential support
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS credential_ids UUID[] DEFAULT '{}'`,

				// Add load_balance_strategy
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS load_balance_strategy VARCHAR(20) DEFAULT 'round_robin'`,

				// Add model_rewrite_map
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS model_rewrite_map JSONB DEFAULT '{}'`,

				// Add base_url if not exists (m0031 uses api_base_url)
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS base_url VARCHAR(255)`,

				// Add is_official
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS is_official BOOLEAN DEFAULT false`,

				// Add status management columns
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active'`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS status_reason TEXT`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS status_changed_at TIMESTAMPTZ`,

				// Add circuit breaker columns
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS circuit_breaker_threshold INT DEFAULT 5`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS circuit_breaker_window_seconds INT DEFAULT 60`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS circuit_breaker_cooldown_seconds INT DEFAULT 30`,

				// Add rate limiting columns
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS rate_limit_rpm INT`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS rate_limit_tpm BIGINT`,

				// Add timeout & retry columns
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS timeout_seconds INT DEFAULT 60`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS retry_count INT DEFAULT 3`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS retry_delay_ms INT DEFAULT 1000`,

				// Add audit columns
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS created_by UUID`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS updated_by UUID`,
			}

			for _, sql := range alterStatements {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			// Step 2: Migrate data from old columns to new columns if needed
			// Copy 'type' to 'route_type' if route_type is empty
			tx.Exec(`
				UPDATE llm_tenant_routes 
				SET route_type = type 
				WHERE route_type IS NULL AND type IS NOT NULL
			`)

			// Copy 'api_base_url' to 'base_url' if base_url is empty
			tx.Exec(`
				UPDATE llm_tenant_routes 
				SET base_url = api_base_url 
				WHERE base_url IS NULL AND api_base_url IS NOT NULL
			`)

			// Copy 'models' JSONB to 'match_models' TEXT[] if match_models is empty
			// Note: models is JSONB array, match_models is TEXT array
			tx.Exec(`
				UPDATE llm_tenant_routes 
				SET match_models = (
					SELECT ARRAY_AGG(elem::text)
					FROM jsonb_array_elements_text(COALESCE(models, '[]'::jsonb)) AS elem
				)
				WHERE (match_models IS NULL OR match_models = '{}') 
				AND models IS NOT NULL AND models != '[]'::jsonb
			`)

			// Step 3: Create indexes (only if column exists)
			indexStatements := []string{
				`CREATE INDEX IF NOT EXISTS idx_routes_tenant_new ON llm_tenant_routes(tenant_id) WHERE deleted_at IS NULL AND is_enabled = true`,
				`CREATE INDEX IF NOT EXISTS idx_routes_models ON llm_tenant_routes USING GIN(match_models) WHERE match_models IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_routes_tenant_priority ON llm_tenant_routes(tenant_id, priority DESC) WHERE deleted_at IS NULL AND is_enabled = true`,
				`CREATE INDEX IF NOT EXISTS idx_routes_status ON llm_tenant_routes(status) WHERE deleted_at IS NULL`,
			}

			for _, sql := range indexStatements {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			// Step 4: Add constraints (drop first to avoid conflicts)
			constraintStatements := []string{
				`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_route_type_new`,
				`ALTER TABLE llm_tenant_routes ADD CONSTRAINT chk_route_type_new CHECK (route_type IS NULL OR route_type IN ('PRIVATE', 'ZGI_CLOUD'))`,

				`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_route_status`,
				`ALTER TABLE llm_tenant_routes ADD CONSTRAINT chk_route_status CHECK (status IS NULL OR status IN ('active', 'disabled', 'banned', 'maintenance'))`,

				`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_load_balance`,
				`ALTER TABLE llm_tenant_routes ADD CONSTRAINT chk_load_balance CHECK (load_balance_strategy IS NULL OR load_balance_strategy IN ('round_robin', 'random', 'weighted'))`,
			}

			for _, sql := range constraintStatements {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop constraints
			tx.Exec("ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_route_type_new")
			tx.Exec("ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_route_status")
			tx.Exec("ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_load_balance")

			// Drop indexes
			tx.Exec("DROP INDEX IF EXISTS idx_routes_tenant_priority")
			tx.Exec("DROP INDEX IF EXISTS idx_routes_models")
			tx.Exec("DROP INDEX IF EXISTS idx_routes_tenant_new")
			tx.Exec("DROP INDEX IF EXISTS idx_routes_status")

			// Drop added columns
			dropColumns := []string{
				"match_models", "route_type", "credential_ids", "load_balance_strategy",
				"model_rewrite_map", "base_url", "is_official", "status", "status_reason",
				"status_changed_at", "circuit_breaker_threshold", "circuit_breaker_window_seconds",
				"circuit_breaker_cooldown_seconds", "rate_limit_rpm", "rate_limit_tpm",
				"timeout_seconds", "retry_count", "retry_delay_ms", "created_by", "updated_by",
			}

			for _, col := range dropColumns {
				tx.Exec("ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS " + col)
			}

			return nil
		},
	}
}
