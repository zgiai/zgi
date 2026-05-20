package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0114_simplify_dataset_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260214000100",
		Migrate: func(tx *gorm.DB) error {
			// Check if retrieval_model column exists
			var retrievalModelExists bool
			tx.Raw(`
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns 
					WHERE table_name = 'datasets' AND column_name = 'retrieval_model'
				);
			`).Scan(&retrievalModelExists)

			// Check if retrieval_config column exists
			var retrievalConfigExists bool
			tx.Raw(`
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns 
					WHERE table_name = 'datasets' AND column_name = 'retrieval_config'
				);
			`).Scan(&retrievalConfigExists)

			// 1. Handle column rename or creation
			if retrievalModelExists && !retrievalConfigExists {
				// Rename retrieval_model to retrieval_config
				if err := tx.Exec(`
					ALTER TABLE datasets 
					RENAME COLUMN retrieval_model TO retrieval_config;
				`).Error; err != nil {
					return err
				}
			} else if !retrievalConfigExists {
				// Create retrieval_config column if neither exists
				if err := tx.Exec(`
					ALTER TABLE datasets 
					ADD COLUMN retrieval_config JSONB DEFAULT NULL;
				`).Error; err != nil {
					return err
				}
			}

			// 2. Update retrieval_config structure - remove deprecated fields, keep only needed ones
			// This will update existing records to match the new simplified structure
			if err := tx.Exec(`
				UPDATE datasets 
				SET retrieval_config = jsonb_build_object(
					'search_method', COALESCE(retrieval_config->>'search_method', 'semantic_search'),
					'top_k', COALESCE((retrieval_config->>'top_k')::int, 4),
					'score_threshold_enabled', COALESCE((retrieval_config->>'score_threshold_enabled')::boolean, false),
					'score_threshold', COALESCE((retrieval_config->>'score_threshold')::float, 0.5),
					'reranking_enable', COALESCE((retrieval_config->>'reranking_enable')::boolean, false),
					'reranking_model', COALESCE(retrieval_config->'reranking_model', '{}'::jsonb)
				)
				WHERE retrieval_config IS NOT NULL;
			`).Error; err != nil {
				return err
			}

			// 3. Set default process_rule for datasets that don't have one
			if err := tx.Exec(`
				UPDATE datasets 
				SET process_rule = jsonb_build_object(
					'mode', 'hierarchical',
					'rules', jsonb_build_object(
						'pre_processing_rules', jsonb_build_array(
							jsonb_build_object('enabled', true, 'id', 'remove_extra_spaces')
						),
						'segmentation', jsonb_build_object(
							'separator', E'\n\n',
							'max_tokens', 500,
							'chunk_overlap', 50
						),
						'subchunk_segmentation', jsonb_build_object(
							'separator', E'\n',
							'max_tokens', 50
						)
					)
				)
				WHERE process_rule IS NULL;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Check if retrieval_config column exists
			var retrievalConfigExists bool
			tx.Raw(`
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns 
					WHERE table_name = 'datasets' AND column_name = 'retrieval_config'
				);
			`).Scan(&retrievalConfigExists)

			// Check if retrieval_model column exists
			var retrievalModelExists bool
			tx.Raw(`
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns 
					WHERE table_name = 'datasets' AND column_name = 'retrieval_model'
				);
			`).Scan(&retrievalModelExists)

			// Rollback: rename retrieval_config back to retrieval_model
			if retrievalConfigExists && !retrievalModelExists {
				if err := tx.Exec(`
					ALTER TABLE datasets 
					RENAME COLUMN retrieval_config TO retrieval_model;
				`).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
