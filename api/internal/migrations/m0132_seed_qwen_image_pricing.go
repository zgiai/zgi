package migrations

import (
	"encoding/json"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func M0132_seed_qwen_image_pricing() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260310000132",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				INSERT INTO llm_protocols (name, display_name, version, adapter_type, description, is_active)
				VALUES ('dashscope', 'DashScope', 'v1', 'dashscope', 'Aliyun DashScope API', true)
				ON CONFLICT (name) DO NOTHING
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_models DROP CONSTRAINT IF EXISTS fk_model_provider
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				INSERT INTO llm_provider_protocols (provider_id, protocol_id, is_default, priority)
				SELECT p.id, prot.id, true, 1
				FROM llm_providers p, llm_protocols prot
				WHERE p.provider = 'qwen' AND prot.name = 'dashscope'
				ON CONFLICT DO NOTHING
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				INSERT INTO llm_provider_protocols (provider_id, protocol_id, is_default, priority)
				SELECT p.id, prot.id, false, 2
				FROM llm_providers p, llm_protocols prot
				WHERE p.provider = 'qwen' AND prot.name = 'openai'
				ON CONFLICT DO NOTHING
			`).Error; err != nil {
				return err
			}

			models := []struct {
				Name        string
				DisplayName string
				Price       float64
				Credits     int
			}{
				{"qwen-image-2.0", "Qwen Image 2.0", 0.16, 160},
			}

			for _, m := range models {
				rules := []map[string]interface{}{
					{
						"id":       m.Name + "_1024",
						"priority": 100,
						"conditions": map[string]interface{}{
							"size": []string{"1024x1024"},
						},
						"price": map[string]interface{}{
							"credits": m.Credits,
							"amount":  m.Price,
						},
					},
					{
						"id":         m.Name + "_default",
						"priority":   0,
						"conditions": map[string]interface{}{},
						"price": map[string]interface{}{
							"credits": m.Credits,
							"amount":  m.Price,
						},
					},
				}

				rulesJSON, err := json.Marshal(rules)
				if err != nil {
					return err
				}

				if err := tx.Exec(`
					INSERT INTO llm_models (
						provider,
						name,
						display_name,
						type,
						use_cases,
						image_generation,
						image_prices,
						is_active,
						is_configured,
						created_at,
						updated_at
					) VALUES (
						'qwen',
						?,
						?,
						'image',
						ARRAY['image-gen'],
						true,
						?,
						true,
						true,
						NOW(),
						NOW()
					)
					ON CONFLICT (provider, name) DO UPDATE SET
						image_prices = EXCLUDED.image_prices,
						updated_at = NOW()
				`, m.Name, m.DisplayName, datatypes.JSON(rulesJSON)).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Remove added models
			if err := tx.Exec(`DELETE FROM llm_models WHERE name IN ('qwen-image-2.0')`).Error; err != nil {
				return err
			}
			// Unlink dashscope protocol from qwen (optional, strict rollback might not be needed if it was added safely)
			return nil
		},
	}
}
