package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0028_add_diagnosis_context_to_logs adds diagnosis context fields for debugging and trace
func M0028_add_diagnosis_context_to_logs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddDiagnosisContextToLogsID,
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE workflow_node_runtime_logs
				ADD COLUMN IF NOT EXISTS error_type VARCHAR(64),
				ADD COLUMN IF NOT EXISTS error_stack TEXT,
				ADD COLUMN IF NOT EXISTS diagnosis_result TEXT,
				ADD COLUMN IF NOT EXISTS diagnosis_model VARCHAR(128),
				ADD COLUMN IF NOT EXISTS diagnosis_tokens INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS diagnosis_latency_ms INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS is_llm_diagnosed BOOLEAN DEFAULT FALSE,
				ADD COLUMN IF NOT EXISTS diagnosis_node_config JSONB,
				ADD COLUMN IF NOT EXISTS diagnosis_upstream_config JSONB,
				ADD COLUMN IF NOT EXISTS diagnosis_input_snapshot JSONB,
				ADD COLUMN IF NOT EXISTS diagnosis_upstream_outputs JSONB;

				CREATE INDEX IF NOT EXISTS idx_wf_node_logs_error_type ON workflow_node_runtime_logs (error_type);
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE workflow_node_runtime_logs
				DROP COLUMN IF EXISTS error_type,
				DROP COLUMN IF EXISTS error_stack,
				DROP COLUMN IF EXISTS diagnosis_result,
				DROP COLUMN IF EXISTS diagnosis_model,
				DROP COLUMN IF EXISTS diagnosis_tokens,
				DROP COLUMN IF EXISTS diagnosis_latency_ms,
				DROP COLUMN IF EXISTS is_llm_diagnosed,
				DROP COLUMN IF EXISTS diagnosis_node_config,
				DROP COLUMN IF EXISTS diagnosis_upstream_config,
				DROP COLUMN IF EXISTS diagnosis_input_snapshot,
				DROP COLUMN IF EXISTS diagnosis_upstream_outputs;
			`).Error
		},
	}
}
