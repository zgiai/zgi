package migrationsv2

import (
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var legacyLLMUsageRelations = []string{
	"llm_organization_usage_logs",
	"llm_usage_logs",
	"llm_tenant_usage_logs",
}

func M0031_cleanup_llm_usage_logs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2CleanupLLMUsageLogsID,
		Migrate: func(tx *gorm.DB) error {
			if err := cleanupLegacyLLMUsageRelations(tx); err != nil {
				return err
			}
			return deleteLegacyLLMUsageRetentionPolicy(tx)
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func cleanupLegacyLLMUsageRelations(tx *gorm.DB) error {
	for _, name := range legacyLLMUsageRelations {
		kind, exists, err := legacyLLMUsageRelationKind(tx, name)
		if err != nil {
			return fmt.Errorf("check legacy llm usage relation %s: %w", name, err)
		}
		if !exists {
			continue
		}

		if err := dropLegacyLLMUsageRelation(tx, name, kind); err != nil {
			return fmt.Errorf("drop legacy llm usage relation %s: %w", name, err)
		}
	}
	return nil
}

func deleteLegacyLLMUsageRetentionPolicy(tx *gorm.DB) error {
	kind, exists, err := legacyLLMUsageRelationKind(tx, "data_retention_policies")
	if err != nil {
		return fmt.Errorf("check data_retention_policies: %w", err)
	}
	if !exists || kind != "table" {
		return nil
	}
	return tx.Exec(`DELETE FROM data_retention_policies WHERE data_type = ?`, "usage_logs").Error
}

func legacyLLMUsageRelationKind(tx *gorm.DB, name string) (string, bool, error) {
	if tx.Dialector.Name() == "sqlite" {
		var relationType string
		err := tx.Raw(`
			SELECT type
			FROM sqlite_master
			WHERE name = ?
			  AND type IN ('table', 'view')
			ORDER BY CASE type WHEN 'view' THEN 1 ELSE 2 END
			LIMIT 1
		`, name).Scan(&relationType).Error
		if err != nil {
			return "", false, err
		}
		if relationType == "" {
			return "", false, nil
		}
		return relationType, true, nil
	}

	var relKind string
	err := tx.Raw(`
		SELECT c.relkind::text
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema()
		  AND c.relname = ?
		LIMIT 1
	`, name).Scan(&relKind).Error
	if err != nil {
		return "", false, err
	}

	switch relKind {
	case "":
		return "", false, nil
	case "r", "p":
		return "table", true, nil
	case "v":
		return "view", true, nil
	case "m":
		return "materialized_view", true, nil
	default:
		return relKind, true, nil
	}
}

func dropLegacyLLMUsageRelation(tx *gorm.DB, name, kind string) error {
	identifier := quoteLegacyLLMUsageIdentifier(name)

	switch kind {
	case "view":
		return tx.Exec("DROP VIEW IF EXISTS " + identifier).Error
	case "materialized_view":
		return tx.Exec("DROP MATERIALIZED VIEW IF EXISTS " + identifier).Error
	case "table":
		return tx.Exec("DROP TABLE IF EXISTS " + identifier).Error
	default:
		return fmt.Errorf("unsupported relation kind %s", kind)
	}
}

func quoteLegacyLLMUsageIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
