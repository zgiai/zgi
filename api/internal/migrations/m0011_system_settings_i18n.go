package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0011_system_settings_i18n adds internationalization columns to system_settings table
func M0011_system_settings_i18n() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251114101000",
		Migrate: func(tx *gorm.DB) error {
			// Add i18n columns to system_settings table
			if err := tx.Exec(`
				ALTER TABLE system_settings 
				ADD COLUMN IF NOT EXISTS category_name_zh_hans VARCHAR(100),
				ADD COLUMN IF NOT EXISTS category_name_en_us VARCHAR(100)
			`).Error; err != nil {
				return err
			}

			// Populate existing rows with default translations
			translations := map[string]struct {
				zhHans string
				enUS   string
			}{
				"email": {
					zhHans: "邮件设置",
					enUS:   "Email Configuration",
				},
				"storage": {
					zhHans: "存储设置",
					enUS:   "Storage Configuration",
				},
				"file_upload": {
					zhHans: "文件上传设置",
					enUS:   "File Upload Configuration",
				},
				"workflow": {
					zhHans: "工作流设置",
					enUS:   "Workflow Configuration",
				},
				"etl": {
					zhHans: "ETL设置",
					enUS:   "ETL Configuration",
				},
				"embedding": {
					zhHans: "嵌入模型设置",
					enUS:   "Embedding Configuration",
				},
				"model": {
					zhHans: "模型设置",
					enUS:   "Model Configuration",
				},
				"cdn": {
					zhHans: "CDN设置",
					enUS:   "CDN Configuration",
				},
				"datasource": {
					zhHans: "数据源设置",
					enUS:   "Data Source Configuration",
				},
				"branding": {
					zhHans: "品牌设置",
					enUS:   "Branding Configuration",
				},
				"sms": {
					zhHans: "短信设置",
					enUS:   "SMS Configuration",
				},
			}

			// Update each category with its translations
			for category, trans := range translations {
				if err := tx.Exec(`
					UPDATE system_settings 
					SET category_name_zh_hans = ?, category_name_en_us = ?
					WHERE category = ?
				`, trans.zhHans, trans.enUS, category).Error; err != nil {
					return err
				}
			}

			// Add indexes for query performance
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_system_settings_category_name_zh_hans ON system_settings(category_name_zh_hans)
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_system_settings_category_name_en_us ON system_settings(category_name_en_us)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes first
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_system_settings_category_name_zh_hans`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`DROP INDEX IF EXISTS idx_system_settings_category_name_en_us`).Error; err != nil {
				return err
			}

			// Drop columns
			if err := tx.Exec(`
				ALTER TABLE system_settings 
				DROP COLUMN IF EXISTS category_name_zh_hans,
				DROP COLUMN IF EXISTS category_name_en_us
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
