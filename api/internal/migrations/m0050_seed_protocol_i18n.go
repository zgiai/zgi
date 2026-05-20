package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0050SeedProtocolI18n seeds i18n data for protocols
func M0050SeedProtocolI18n() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000050",
		Migrate: func(tx *gorm.DB) error {
			// Update OpenAI protocol
			if err := tx.Exec(`
				UPDATE llm_protocols SET
					display_name_i18n = '{"en_US": "OpenAI Standard", "zh_Hans": "OpenAI 标准协议"}',
					description_i18n = '{"en_US": "OpenAI API standard protocol, compatible with many providers", "zh_Hans": "OpenAI API 标准协议，兼容多种提供商"}',
					icon_url = '/assets/provider_icons/openai/icon.svg'
				WHERE name = 'openai'
			`).Error; err != nil {
				return err
			}

			// Update Anthropic protocol
			if err := tx.Exec(`
				UPDATE llm_protocols SET
					display_name_i18n = '{"en_US": "Claude API", "zh_Hans": "Claude API 协议"}',
					description_i18n = '{"en_US": "Anthropic Claude API protocol", "zh_Hans": "Anthropic Claude API 协议"}',
					icon_url = '/assets/provider_icons/anthropic/icon.svg'
				WHERE name = 'anthropic'
			`).Error; err != nil {
				return err
			}

			// Update Google protocol
			if err := tx.Exec(`
				UPDATE llm_protocols SET
					display_name_i18n = '{"en_US": "Google AI", "zh_Hans": "Google AI 协议"}',
					description_i18n = '{"en_US": "Google Gemini API protocol", "zh_Hans": "Google Gemini API 协议"}',
					icon_url = '/assets/provider_icons/google/icon.svg'
				WHERE name = 'google'
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				UPDATE llm_protocols SET
					display_name_i18n = '{}',
					description_i18n = '{}',
					icon_url = ''
			`).Error
		},
	}
}
