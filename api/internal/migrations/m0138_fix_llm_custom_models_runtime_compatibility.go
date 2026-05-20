package migrations

import (
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0138ID = "20260330000138"

const (
	customModelsTable       = "llm_custom_models"
	legacyCustomModelsTable = "llm_tenant_custom_models"
	customProvidersTable    = "llm_custom_providers"
	legacyProvidersTable    = "llm_tenant_custom_providers"
)

type migration0138Column struct {
	name       string
	definition string
}

// M0138_fix_llm_custom_models_runtime_compatibility repairs environments where
// compatibility migrations marked success before llm_custom_models reached the
// schema current runtime code expects.
func M0138_fix_llm_custom_models_runtime_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0138ID,
		Migrate: func(tx *gorm.DB) error {
			if err := ensureCustomModelTablesReady(tx); err != nil {
				return err
			}
			if !tx.Migrator().HasTable(customModelsTable) {
				return nil
			}

			for _, column := range runtimeCompatibilityColumns(tx.Dialector.Name()) {
				if err := addColumnIfMissing(tx, customModelsTable, column); err != nil {
					return err
				}
			}

			if err := backfillProviderSlug(tx); err != nil {
				return err
			}
			if err := syncBoolColumnFromLegacy(tx, customModelsTable, "vision", "supports_vision"); err != nil {
				return err
			}
			if err := syncBoolColumnFromLegacy(tx, customModelsTable, "function_calling", "supports_tool_call"); err != nil {
				return err
			}
			if err := syncBoolColumnFromLegacy(tx, customModelsTable, "reasoning", "supports_reasoning"); err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func ensureCustomModelTablesReady(tx *gorm.DB) error {
	if err := renameTableIfMissing(tx, legacyProvidersTable, customProvidersTable); err != nil {
		return err
	}
	if err := renameTableIfMissing(tx, legacyCustomModelsTable, customModelsTable); err != nil {
		return err
	}

	if err := renameColumnIfMissing(tx, customProvidersTable, "tenant_id", "organization_id"); err != nil {
		return err
	}
	if err := renameColumnIfMissing(tx, customModelsTable, "tenant_id", "organization_id"); err != nil {
		return err
	}
	if err := renameColumnIfMissing(tx, customProvidersTable, "name", "provider"); err != nil {
		return err
	}
	if err := renameColumnIfMissing(tx, customProvidersTable, "display_name", "provider_name"); err != nil {
		return err
	}

	return nil
}

func runtimeCompatibilityColumns(dialect string) []migration0138Column {
	jsonType := "JSONB"
	if dialect == "sqlite" {
		jsonType = "TEXT"
	}

	return []migration0138Column{
		{name: "provider", definition: "VARCHAR(100) DEFAULT ''"},
		{name: "vision", definition: "BOOLEAN DEFAULT false"},
		{name: "function_calling", definition: "BOOLEAN DEFAULT false"},
		{name: "reasoning", definition: "BOOLEAN DEFAULT false"},
		{name: "audio", definition: "BOOLEAN DEFAULT false"},
		{name: "image_generation", definition: "BOOLEAN DEFAULT false"},
		{name: "speech_generation", definition: "BOOLEAN DEFAULT false"},
		{name: "transcription", definition: "BOOLEAN DEFAULT false"},
		{name: "translation", definition: "BOOLEAN DEFAULT false"},
		{name: "moderation", definition: "BOOLEAN DEFAULT false"},
		{name: "realtime", definition: "BOOLEAN DEFAULT false"},
		{name: "batch", definition: "BOOLEAN DEFAULT false"},
		{name: "fine_tuning", definition: "BOOLEAN DEFAULT false"},
		{name: "assistants", definition: "BOOLEAN DEFAULT false"},
		{name: "responses", definition: "BOOLEAN DEFAULT false"},
		{name: "distillation", definition: "BOOLEAN DEFAULT false"},
		{name: "system_prompt", definition: "BOOLEAN DEFAULT true"},
		{name: "logprobs", definition: "BOOLEAN DEFAULT false"},
		{name: "web_search", definition: "BOOLEAN DEFAULT false"},
		{name: "file_search", definition: "BOOLEAN DEFAULT false"},
		{name: "code_interpreter", definition: "BOOLEAN DEFAULT false"},
		{name: "computer_use", definition: "BOOLEAN DEFAULT false"},
		{name: "mcp", definition: "BOOLEAN DEFAULT false"},
		{name: "reasoning_effort", definition: "BOOLEAN DEFAULT false"},
		{name: "parallel_tool_calls", definition: "BOOLEAN DEFAULT false"},
		{name: "temperature", definition: "BOOLEAN DEFAULT true"},
		{name: "top_p", definition: "BOOLEAN DEFAULT true"},
		{name: "presence_penalty", definition: "BOOLEAN DEFAULT false"},
		{name: "frequency_penalty", definition: "BOOLEAN DEFAULT false"},
		{name: "logit_bias", definition: "BOOLEAN DEFAULT false"},
		{name: "seed", definition: "BOOLEAN DEFAULT false"},
		{name: "stop", definition: "BOOLEAN DEFAULT true"},
		{name: "max_stop_sequences", definition: "INTEGER DEFAULT 4"},
		{name: "default_parameters", definition: fmt.Sprintf("%s DEFAULT '{}'", jsonType)},
	}
}

func addColumnIfMissing(tx *gorm.DB, table string, column migration0138Column) error {
	exists, err := hasExactColumn(tx, table, column.name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	sql := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column.name, column.definition)
	return tx.Exec(sql).Error
}

func backfillProviderSlug(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(customProvidersTable) ||
		!mustHaveColumn(tx, customProvidersTable, "id") ||
		!mustHaveColumn(tx, customProvidersTable, "provider") ||
		!mustHaveColumn(tx, customModelsTable, "provider") ||
		!mustHaveColumn(tx, customModelsTable, "provider_id") {
		return nil
	}

	sql := fmt.Sprintf(`
		UPDATE %s
		SET provider = (
			SELECT cp.provider
			FROM %s cp
			WHERE cp.id = %s.provider_id
		)
		WHERE COALESCE(provider, '') = ''
		  AND provider_id IS NOT NULL
	`, customModelsTable, customProvidersTable, customModelsTable)

	return tx.Exec(sql).Error
}

func syncBoolColumnFromLegacy(tx *gorm.DB, tableName, targetColumn, legacyColumn string) error {
	if !mustHaveColumn(tx, tableName, targetColumn) || !mustHaveColumn(tx, tableName, legacyColumn) {
		return nil
	}

	sql := fmt.Sprintf(`
		UPDATE %s
		SET %s = %s
		WHERE %s IS NOT NULL
		  AND (%s IS NULL OR %s <> %s)
	`, tableName, targetColumn, legacyColumn, legacyColumn, targetColumn, targetColumn, legacyColumn)

	return tx.Exec(sql).Error
}

func renameTableIfMissing(tx *gorm.DB, oldName, newName string) error {
	if !tx.Migrator().HasTable(oldName) || tx.Migrator().HasTable(newName) {
		return nil
	}
	return tx.Migrator().RenameTable(oldName, newName)
}

func renameColumnIfMissing(tx *gorm.DB, tableName, oldColumn, newColumn string) error {
	if !tx.Migrator().HasTable(tableName) ||
		!mustHaveColumn(tx, tableName, oldColumn) ||
		mustHaveColumn(tx, tableName, newColumn) {
		return nil
	}
	return tx.Migrator().RenameColumn(tableName, oldColumn, newColumn)
}

func hasExactColumn(tx *gorm.DB, tableName, columnName string) (bool, error) {
	switch tx.Dialector.Name() {
	case "postgres":
		var count int64
		err := tx.Raw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = CURRENT_SCHEMA()
			  AND table_name = ?
			  AND column_name = ?
		`, tableName, columnName).Scan(&count).Error
		return count > 0, err
	case "sqlite":
		var columns []struct {
			Name string `gorm:"column:name"`
		}
		if err := tx.Raw(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName)).Scan(&columns).Error; err != nil {
			return false, err
		}
		for _, column := range columns {
			if strings.EqualFold(column.Name, columnName) {
				return true, nil
			}
		}
		return false, nil
	default:
		columnTypes, err := tx.Migrator().ColumnTypes(tableName)
		if err != nil {
			return false, err
		}
		for _, columnType := range columnTypes {
			if strings.EqualFold(columnType.Name(), columnName) {
				return true, nil
			}
		}
		return false, nil
	}
}

func mustHaveColumn(tx *gorm.DB, tableName, columnName string) bool {
	exists, err := hasExactColumn(tx, tableName, columnName)
	return err == nil && exists
}
