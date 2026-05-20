package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0046_seed_special_models_v2 seeds Rerank, Vision/Multimodal, and Audio models (fixed version)
func M0046_seed_special_models_v2() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251215000046",
		Migrate: func(tx *gorm.DB) error {
			// First, ensure providers exist
			providers := []struct {
				Name        string
				DisplayName string
			}{
				{"cohere", "Cohere"},
				{"openai", "OpenAI"},
				{"google", "Google"},
				{"anthropic", "Anthropic"},
			}

			for _, p := range providers {
				tx.Exec(`
					INSERT INTO llm_providers (name, display_name, is_active, created_at, updated_at)
					VALUES (?, ?, true, NOW(), NOW())
					ON CONFLICT (name) DO NOTHING
				`, p.Name, p.DisplayName)
			}

			// Seed Cohere Rerank models
			rerankModels := []struct {
				Name         string
				DisplayName  string
				ContextWindow int
				InputPrice    string
			}{
				{"rerank-v3.5", "Cohere Rerank v3.5", 4096, "0.002"},
				{"rerank-english-v3.0", "Cohere Rerank English v3.0", 4096, "0.002"},
				{"rerank-multilingual-v3.0", "Cohere Rerank Multilingual v3.0", 4096, "0.002"},
				{"rerank-english-v2.0", "Cohere Rerank English v2.0", 512, "0.001"},
				{"rerank-multilingual-v2.0", "Cohere Rerank Multilingual v2.0", 512, "0.001"},
			}

			for _, m := range rerankModels {
				tx.Exec(`
					INSERT INTO llm_models (provider, name, display_name, type, context_window, input_price, is_active, created_at, updated_at)
					VALUES ('cohere', ?, ?, 'rerank', ?, ?, true, NOW(), NOW())
					ON CONFLICT (provider, name) DO NOTHING
				`, m.Name, m.DisplayName, m.ContextWindow, m.InputPrice)
			}

			// Add supports_vision column if not exists
			tx.Exec(`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS supports_vision BOOLEAN DEFAULT false`)

			// Seed Vision models (update existing models to mark vision support)
			tx.Exec(`UPDATE llm_models SET supports_vision = true WHERE provider = 'openai' AND name IN ('gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo', 'gpt-4-vision-preview', 'o1', 'o1-mini', 'o1-preview')`)
			tx.Exec(`UPDATE llm_models SET supports_vision = true WHERE provider = 'google' AND name LIKE 'gemini%'`)
			tx.Exec(`UPDATE llm_models SET supports_vision = true WHERE provider = 'anthropic' AND name LIKE 'claude-3%'`)

			// Add type column if not exists (for backward compatibility)
			tx.Exec(`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS type VARCHAR(50) DEFAULT 'llm'`)

			// Seed OpenAI Audio models (TTS and STT)
			audioModels := []struct {
				Name        string
				DisplayName string
				Type        string
				InputPrice   string
			}{
				{"tts-1", "OpenAI TTS-1", "tts", "0.015"},
				{"tts-1-hd", "OpenAI TTS-1 HD", "tts", "0.030"},
				{"whisper-1", "OpenAI Whisper-1", "speech2text", "0.006"},
			}

			for _, m := range audioModels {
				tx.Exec(`
					INSERT INTO llm_models (provider, name, display_name, type, input_price, is_active, created_at, updated_at)
					VALUES ('openai', ?, ?, ?, ?, true, NOW(), NOW())
					ON CONFLICT (provider, name) DO NOTHING
				`, m.Name, m.DisplayName, m.Type, m.InputPrice)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Remove seeded models
			tx.Exec(`DELETE FROM llm_models WHERE provider = 'cohere' AND type = 'rerank'`)
			tx.Exec(`UPDATE llm_models SET supports_vision = false WHERE provider IN ('openai', 'google', 'anthropic')`)
			tx.Exec(`DELETE FROM llm_models WHERE provider = 'openai' AND type IN ('tts', 'speech2text')`)
			return nil
		},
	}
}
