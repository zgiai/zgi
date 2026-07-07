package migrations

import (
	"encoding/json"
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

const migration20260626090000ID = "20260626090000_add_workspace_member_direct_permissions"

const addWorkspaceMemberDirectPermissionsColumnsSQL = `
ALTER TABLE public.workspace_members
	ADD COLUMN IF NOT EXISTS permissions jsonb DEFAULT '[]'::jsonb NOT NULL,
	ADD COLUMN IF NOT EXISTS permission_source varchar(32) DEFAULT 'role_template' NOT NULL,
	ADD COLUMN IF NOT EXISTS permission_template_role_id uuid
`

const addWorkspaceMemberPermissionSourceConstraintSQL = `
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'workspace_members_permission_source_check'
	) THEN
		ALTER TABLE public.workspace_members
			ADD CONSTRAINT workspace_members_permission_source_check
			CHECK (permission_source IN ('owner', 'role_template', 'direct', 'legacy_role'));
	END IF;
END $$;
`

const addWorkspaceMemberPermissionIndexesSQL = `
CREATE INDEX IF NOT EXISTS idx_workspace_members_permission_template_role_id
ON public.workspace_members (permission_template_role_id)
`

func init() {
	registerSchemaMigration(
		migration20260626090000ID,
		upAddWorkspaceMemberDirectPermissions,
		downAddWorkspaceMemberDirectPermissions,
	)
}

func upAddWorkspaceMemberDirectPermissions(schema *mschema.Builder) error {
	for _, statement := range []string{
		addWorkspaceMemberDirectPermissionsColumnsSQL,
		addWorkspaceMemberPermissionSourceConstraintSQL,
		addWorkspaceMemberPermissionIndexesSQL,
	} {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return seedWorkspaceMemberDirectPermissions(schema)
}

func downAddWorkspaceMemberDirectPermissions(schema *mschema.Builder) error {
	for _, statement := range []string{
		`DROP INDEX IF EXISTS public.idx_workspace_members_permission_template_role_id`,
		`ALTER TABLE public.workspace_members DROP CONSTRAINT IF EXISTS workspace_members_permission_source_check`,
	} {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return schema.Table("workspace_members", func(table *mschema.Blueprint) {
		table.DropColumn("permission_template_role_id")
		table.DropColumn("permission_source")
		table.DropColumn("permissions")
	})
}

type workspaceMemberPermissionSeedRow struct {
	ID              string
	Role            string
	RoleID          *string
	RolePermissions string
}

func seedWorkspaceMemberDirectPermissions(schema *mschema.Builder) error {
	return schema.DataFix("seed workspace member direct permissions", func(db *gorm.DB) error {
		var rows []workspaceMemberPermissionSeedRow
		if err := db.Raw(`
			SELECT
				wm.id,
				wm.role,
				wm.role_id::text AS role_id,
				COALESCE(roles.permissions::text, '[]') AS role_permissions
			FROM public.workspace_members AS wm
			LEFT JOIN public.roles AS roles
			  ON roles.id = wm.role_id
			 AND roles.status = 'active'
			WHERE wm.permission_source IN ('role_template', 'legacy_role', 'owner')
		`).Scan(&rows).Error; err != nil {
			return fmt.Errorf("failed to read workspace members for permission seed: %w", err)
		}

		for _, row := range rows {
			source, templateRoleID, permissions, err := buildWorkspaceMemberPermissionSeed(row)
			if err != nil {
				return fmt.Errorf("failed to build workspace member permission seed for %s: %w", row.ID, err)
			}

			permissionsJSON, err := json.Marshal(permissions)
			if err != nil {
				return fmt.Errorf("failed to encode workspace member permissions for %s: %w", row.ID, err)
			}

			updates := map[string]any{
				"permission_source": string(source),
				"permissions":       gorm.Expr("?::jsonb", string(permissionsJSON)),
			}
			if templateRoleID == "" {
				updates["permission_template_role_id"] = nil
			} else {
				updates["permission_template_role_id"] = gorm.Expr("?::uuid", templateRoleID)
			}

			if err := db.Table("public.workspace_members").
				Where("id = ?", row.ID).
				Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to seed workspace member permissions for %s: %w", row.ID, err)
			}
		}

		return nil
	})
}

func buildWorkspaceMemberPermissionSeed(row workspaceMemberPermissionSeedRow) (workspace_model.WorkspaceMemberPermissionSource, string, []string, error) {
	role := workspace_model.WorkspaceMemberRole(strings.TrimSpace(row.Role))
	roleID := ""
	if row.RoleID != nil {
		roleID = strings.TrimSpace(*row.RoleID)
	}

	if role == workspace_model.WorkspaceRoleOwner {
		return workspace_model.WorkspaceMemberPermissionSourceOwner,
			workspace_model.WorkspaceBuiltinRoleOwnerID,
			[]string{},
			nil
	}

	source := workspace_model.WorkspaceMemberPermissionSourceRoleTemplate
	if roleID == "" {
		source = workspace_model.WorkspaceMemberPermissionSourceLegacyRole
		roleID = workspace_model.DefaultWorkspaceRoleID(role)
		if roleID == "" {
			roleID = workspace_model.WorkspaceBuiltinRoleMemberID
		}
	}

	if workspace_model.IsBuiltinRole(roleID) {
		return source,
			roleID,
			workspace_model.CanonicalAssignableWorkspacePermissionSnapshotStrings(
				workspace_model.DefaultWorkspaceMemberPermissionStrings(role, &roleID),
			),
			nil
	}

	rolePermissions, err := decodeWorkspaceMemberPermissionSeedJSON(row.RolePermissions)
	if err != nil {
		return "", "", nil, err
	}

	return source,
		roleID,
		workspace_model.CanonicalAssignableWorkspacePermissionSnapshotStrings(rolePermissions),
		nil
}

func decodeWorkspaceMemberPermissionSeedJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}

	var permissions []string
	if err := json.Unmarshal([]byte(raw), &permissions); err != nil {
		return nil, err
	}
	return permissions, nil
}
