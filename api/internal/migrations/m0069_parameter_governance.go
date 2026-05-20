package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0069_parameter_governance adds metadata-driven parameter governance support
func M0069_parameter_governance() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0069_parameter_governance",
		Migrate: func(tx *gorm.DB) error {
			// Add parameter-related columns to llm_models
			sql := `
				-- Boolean flags for standard parameter support
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS top_p BOOLEAN DEFAULT true;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS presence_penalty BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS frequency_penalty BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS logit_bias BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS parallel_tool_calls BOOLEAN DEFAULT true;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS reasoning_effort BOOLEAN DEFAULT false;

				-- Modalities
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS input_modalities JSONB DEFAULT '["text"]';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS output_modalities JSONB DEFAULT '["text"]';

				-- Parameter Metadata (Three-Layer Governance)
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS supported_parameters JSONB DEFAULT '[]';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS default_parameters JSONB DEFAULT '{}';

				-- Also update custom models
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS input_modalities JSONB DEFAULT '["text"]';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS output_modalities JSONB DEFAULT '["text"]';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS supported_parameters JSONB DEFAULT '[]';
			`
			return tx.Exec(sql).Error
		},
		Rollback: func(tx *gorm.DB) error {
			sql := `
				ALTER TABLE llm_models DROP COLUMN IF EXISTS top_p;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS presence_penalty;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS frequency_penalty;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS logit_bias;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS parallel_tool_calls;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS reasoning_effort;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS input_modalities;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS output_modalities;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS supported_parameters;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS default_parameters;

				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS input_modalities;
				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS output_modalities;
				ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS supported_parameters;
			`
			return tx.Exec(sql).Error
		},
	}
}
