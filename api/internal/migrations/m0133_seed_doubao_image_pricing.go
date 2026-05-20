package migrations

import (
	"encoding/json"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func M0133_seed_doubao_image_pricing() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260310000133",
		Migrate: func(tx *gorm.DB) error {
			// Ensure volcengine protocol exists (mapped to volcengine adapter)
			if err := tx.Exec(`
				INSERT INTO llm_protocols (name, display_name, version, adapter_type, description, is_active)
				VALUES ('volcengine', 'Volcengine Ark', 'v3', 'volcengine', 'Volcengine Ark API', true)
				ON CONFLICT (name) DO NOTHING
			`).Error; err != nil {
				return err
			}

			// Link provider to protocol (provider must already exist)
			if err := tx.Exec(`
				INSERT INTO llm_provider_protocols (provider_id, protocol_id, base_url, is_default, priority)
				SELECT p.id, prot.id, 'https://ark.cn-beijing.volces.com/api/v3', true, 1
				FROM llm_providers p, llm_protocols prot
				WHERE p.provider = 'doubao' AND prot.name = 'volcengine'
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
				{"doubao-seedream-5-0-lite-260128", "Doubao Seedream 5.0 Lite", 0.10, 100},
			}

			for _, m := range models {
				rules := []map[string]interface{}{
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
						'doubao',
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
			if err := tx.Exec(`DELETE FROM llm_models WHERE name IN ('doubao-seedream-5-0-lite-260128')`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
