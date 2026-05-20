package migrations

import (
	"encoding/json"
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0088_merge_role_permissions_to_roles merges enterprise_group_role_permissions into roles.permissions (jsonb)
func M0088_merge_role_permissions_to_roles() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601170088",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add permissions column to roles
			if err := tx.Exec("ALTER TABLE roles ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT '[]'::jsonb").Error; err != nil {
				return err
			}

			// 2. Data migration: Aggregate permissions and update roles
			type RolePermission struct {
				RoleID         string
				PermissionCode string
			}
			var rolePerms []RolePermission
			// Use the old table name directly
			if err := tx.Raw("SELECT role_id, permission_code FROM enterprise_group_role_permissions").Scan(&rolePerms).Error; err != nil {
				// If table doesn't exist (e.g. fresh install skipped it?), we can ignore
				// But here we assume it exists as per migration history
				fmt.Printf("Warning: failed to read enterprise_group_role_permissions: %v\n", err)
			}

			// Group permissions by role
			permsByRole := make(map[string][]string)
			for _, rp := range rolePerms {
				permsByRole[rp.RoleID] = append(permsByRole[rp.RoleID], rp.PermissionCode)
			}

			// Update roles
			for roleID, perms := range permsByRole {
				permsJSON, err := json.Marshal(perms)
				if err != nil {
					return err
				}
				if err := tx.Exec("UPDATE roles SET permissions = ? WHERE id = ?", permsJSON, roleID).Error; err != nil {
					return err
				}
			}

			// 3. Drop enterprise_group_role_permissions table
			if err := tx.Exec("DROP TABLE IF EXISTS enterprise_group_role_permissions").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Recreate enterprise_group_role_permissions table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS "enterprise_group_role_permissions" (
					"role_id" uuid NOT NULL,
					"permission_code" varchar(128) NOT NULL,
					"created_by" uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000', -- Default dummy UUID for rollback
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("role_id", "permission_code"),
					CONSTRAINT fk_eg_role_perm_role FOREIGN KEY (role_id)
						REFERENCES roles(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_eg_role_perm_role_id ON enterprise_group_role_permissions(role_id);
				CREATE INDEX IF NOT EXISTS idx_eg_role_perm_perm_code ON enterprise_group_role_permissions(permission_code);
			`).Error; err != nil {
				return err
			}

			// 2. Data migration: Extract permissions from roles and insert into enterprise_group_role_permissions
			type Role struct {
				ID          string
				Permissions []byte // JSONB
			}
			var roles []Role
			if err := tx.Raw("SELECT id, permissions FROM roles").Scan(&roles).Error; err != nil {
				return err
			}

			for _, r := range roles {
				if len(r.Permissions) == 0 {
					continue
				}
				var perms []string
				if err := json.Unmarshal(r.Permissions, &perms); err != nil {
					continue // Ignore bad data
				}

				for _, p := range perms {
					// Use raw SQL to avoid model dependencies
					if err := tx.Exec("INSERT INTO enterprise_group_role_permissions (role_id, permission_code) VALUES (?, ?) ON CONFLICT DO NOTHING", r.ID, p).Error; err != nil {
						return err
					}
				}
			}

			// 3. Drop permissions column from roles
			if err := tx.Exec("ALTER TABLE roles DROP COLUMN IF EXISTS permissions").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
