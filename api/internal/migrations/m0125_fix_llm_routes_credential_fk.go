package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0125_fix_llm_routes_credential_fk points route credentials at llm_credentials.
// Some environments still carry the legacy foreign key to llm_tenant_credentials.
func M0125_fix_llm_routes_credential_fk() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000125",
		Migrate: func(tx *gorm.DB) error {
			routeTable, err := firstExistingTable(tx, "llm_routes", "llm_tenant_routes")
			if err != nil {
				return err
			}
			if routeTable == "" {
				return nil
			}

			hasUserCredentialID, err := columnExists(tx, routeTable, "user_credential_id")
			if err != nil {
				return err
			}
			if !hasUserCredentialID {
				return nil
			}

			sqls := []string{
				`ALTER TABLE ` + routeTable + ` DROP CONSTRAINT IF EXISTS fk_tenant_route_credential`,
				`ALTER TABLE ` + routeTable + ` DROP CONSTRAINT IF EXISTS fk_route_credential`,
				`ALTER TABLE ` + routeTable + ` DROP CONSTRAINT IF EXISTS llm_tenant_routes_user_credential_id_fkey`,
				`ALTER TABLE ` + routeTable + ` DROP CONSTRAINT IF EXISTS llm_routes_user_credential_id_fkey`,
				`ALTER TABLE ` + routeTable + ` ADD CONSTRAINT fk_llm_routes_credential
					FOREIGN KEY (user_credential_id) REFERENCES llm_credentials(id) ON DELETE SET NULL`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func firstExistingTable(tx *gorm.DB, tableNames ...string) (string, error) {
	for _, tableName := range tableNames {
		exists, err := tableExists(tx, tableName)
		if err != nil {
			return "", err
		}
		if exists {
			return tableName, nil
		}
	}

	return "", nil
}
