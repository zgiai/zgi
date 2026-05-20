package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0130_add_official_model_snapshot() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000130",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_official_model_snapshots (
					source_key VARCHAR(50) PRIMARY KEY,
					effective_models JSONB NOT NULL DEFAULT '[]'::jsonb,
					latest_models JSONB NOT NULL DEFAULT '[]'::jsonb,
					previous_models JSONB NOT NULL DEFAULT '[]'::jsonb,
					latest_event_version BIGINT NOT NULL DEFAULT 0,
					latest_synced_at TIMESTAMPTZ NULL,
					effective_updated_at TIMESTAMPTZ NULL,
					last_check_status VARCHAR(20) NOT NULL DEFAULT 'accepted',
					last_reject_reason TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				INSERT INTO llm_official_model_snapshots (
					source_key,
					effective_models,
					latest_models,
					previous_models,
					latest_event_version,
					latest_synced_at,
					effective_updated_at,
					last_check_status,
					created_at,
					updated_at
				)
				SELECT
					'ZGI_CLOUD',
					COALESCE(
						(
							SELECT jsonb_agg(model ORDER BY model)
							FROM (
								SELECT DISTINCT jsonb_array_elements_text(COALESCE(models, '[]'::jsonb)) AS model
								FROM llm_routes
								WHERE is_official = true AND deleted_at IS NULL
							) deduped
						),
						'[]'::jsonb
					),
					COALESCE(
						(
							SELECT jsonb_agg(model ORDER BY model)
							FROM (
								SELECT DISTINCT jsonb_array_elements_text(COALESCE(models, '[]'::jsonb)) AS model
								FROM llm_routes
								WHERE is_official = true AND deleted_at IS NULL
							) deduped
						),
						'[]'::jsonb
					),
					'[]'::jsonb,
					0,
					NOW(),
					NOW(),
					'accepted',
					NOW(),
					NOW()
				ON CONFLICT (source_key) DO NOTHING
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS llm_official_model_snapshots`).Error
		},
	}
}
