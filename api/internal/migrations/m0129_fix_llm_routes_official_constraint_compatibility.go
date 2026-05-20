package migrations

import (
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0129_fix_llm_routes_official_constraint_compatibility repairs llm_routes
// official-route check constraints for environments whose schema drifted away
// from the runtime expectation.
func M0129_fix_llm_routes_official_constraint_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000129",
		Migrate: func(tx *gorm.DB) error {
			exists, err := tableExists(tx, "llm_routes")
			if err != nil {
				return err
			}
			if !exists {
				return nil
			}

			hasSystemChannelID, err := columnExists(tx, "llm_routes", "system_channel_id")
			if err != nil {
				return err
			}

			sqls := []string{
				`ALTER TABLE llm_routes DROP CONSTRAINT IF EXISTS chk_system_ref`,
				`ALTER TABLE llm_routes DROP CONSTRAINT IF EXISTS chk_route_ref`,
				buildOfficialRouteConstraintSQL(hasSystemChannelID),
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return fmt.Errorf("apply llm_routes official constraint compatibility: %w", err)
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Compatibility migration is intentionally one-way.
			return nil
		},
	}
}

func buildOfficialRouteConstraintSQL(hasSystemChannelID bool) string {
	const officialWithSystemRef = `
ALTER TABLE llm_routes
ADD CONSTRAINT chk_system_ref CHECK (
	(type = 'ZGI_CLOUD' AND (system_channel_id IS NOT NULL OR is_official = true)) OR
	(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
)`

	const officialWithoutSystemRef = `
ALTER TABLE llm_routes
ADD CONSTRAINT chk_route_ref CHECK (
	(type = 'ZGI_CLOUD' AND is_official = true) OR
	(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
)`

	if hasSystemChannelID {
		return strings.TrimSpace(officialWithSystemRef)
	}
	return strings.TrimSpace(officialWithoutSystemRef)
}
