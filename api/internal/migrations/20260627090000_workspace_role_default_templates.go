package migrations

import (
	"encoding/json"
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

const migration20260627090000ID = "20260627090000_workspace_role_default_templates"

const addWorkspaceRoleTemplateMetadataSQL = `
ALTER TABLE public.roles
	ADD COLUMN IF NOT EXISTS name_i18n jsonb DEFAULT '{}'::jsonb NOT NULL,
	ADD COLUMN IF NOT EXISTS description_i18n jsonb DEFAULT '{}'::jsonb NOT NULL,
	ADD COLUMN IF NOT EXISTS system_key varchar(64),
	ADD COLUMN IF NOT EXISTS template_origin varchar(32) DEFAULT 'custom' NOT NULL
`

const addWorkspaceRoleSystemKeyIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS uk_roles_group_system_key
ON public.roles (group_id, system_key)
WHERE system_key IS NOT NULL
`

func init() {
	registerSchemaMigration(
		migration20260627090000ID,
		upWorkspaceRoleDefaultTemplates,
		downWorkspaceRoleDefaultTemplates,
	)
}

func upWorkspaceRoleDefaultTemplates(schema *mschema.Builder) error {
	for _, statement := range []string{
		addWorkspaceRoleTemplateMetadataSQL,
		addWorkspaceRoleSystemKeyIndexSQL,
	} {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}

	if err := backfillWorkspaceRoleTemplateMetadata(schema); err != nil {
		return err
	}
	return seedWorkspaceDefaultRoleTemplates(schema)
}

func downWorkspaceRoleDefaultTemplates(schema *mschema.Builder) error {
	if err := schema.Raw(`DROP INDEX IF EXISTS public.uk_roles_group_system_key`); err != nil {
		return err
	}
	return schema.Table("roles", func(table *mschema.Blueprint) {
		table.DropColumn("template_origin")
		table.DropColumn("system_key")
		table.DropColumn("description_i18n")
		table.DropColumn("name_i18n")
	})
}

func backfillWorkspaceRoleTemplateMetadata(schema *mschema.Builder) error {
	return schema.DataFix("backfill workspace role template metadata", func(db *gorm.DB) error {
		var roles []struct {
			ID          string
			Name        string
			Description *string
			Permissions string
		}
		if err := db.Raw(`
			SELECT id::text AS id, name, description, COALESCE(permissions::text, '[]') AS permissions
			FROM public.roles
		`).Scan(&roles).Error; err != nil {
			return fmt.Errorf("failed to read workspace role templates: %w", err)
		}

		for _, role := range roles {
			permissions, err := decodeWorkspaceMemberPermissionSeedJSON(role.Permissions)
			if err != nil {
				return fmt.Errorf("failed to decode role permissions for %s: %w", role.ID, err)
			}
			sanitizedPermissions, err := json.Marshal(workspace_model.CanonicalAssignableWorkspacePermissionSnapshotStrings(permissions))
			if err != nil {
				return fmt.Errorf("failed to encode role permissions for %s: %w", role.ID, err)
			}
			nameI18n, err := json.Marshal(map[string]string{
				"zh_Hans": role.Name,
				"en_US":   role.Name,
			})
			if err != nil {
				return fmt.Errorf("failed to encode role name i18n for %s: %w", role.ID, err)
			}
			desc := ""
			if role.Description != nil {
				desc = *role.Description
			}
			descriptionI18n, err := json.Marshal(map[string]string{
				"zh_Hans": desc,
				"en_US":   desc,
			})
			if err != nil {
				return fmt.Errorf("failed to encode role description i18n for %s: %w", role.ID, err)
			}

			if err := db.Table("public.roles").
				Where("id = ?::uuid", role.ID).
				Updates(map[string]any{
					"name_i18n":        gorm.Expr("CASE WHEN name_i18n = '{}'::jsonb THEN ?::jsonb ELSE name_i18n END", string(nameI18n)),
					"description_i18n": gorm.Expr("CASE WHEN description_i18n = '{}'::jsonb THEN ?::jsonb ELSE description_i18n END", string(descriptionI18n)),
					"template_origin":  gorm.Expr("COALESCE(NULLIF(template_origin, ''), 'custom')"),
					"permissions":      gorm.Expr("?::jsonb", string(sanitizedPermissions)),
				}).Error; err != nil {
				return fmt.Errorf("failed to persist role template metadata for %s: %w", role.ID, err)
			}
		}

		return nil
	})
}

type workspaceDefaultRoleTemplateOrganizationSeed struct {
	OrganizationID string
	CreatedBy      string
	Language       string
}

func seedWorkspaceDefaultRoleTemplates(schema *mschema.Builder) error {
	return schema.DataFix("seed workspace default role templates", func(db *gorm.DB) error {
		var organizations []workspaceDefaultRoleTemplateOrganizationSeed
		if err := db.Raw(`
			SELECT
				o.id::text AS organization_id,
				creator.account_id::text AS created_by,
				COALESCE(a.interface_language, '') AS language
			FROM public.organizations AS o
			JOIN LATERAL (
				SELECT m.account_id
				FROM public.members AS m
				WHERE m.organization_id = o.id
				  AND m.status = 'active'
				ORDER BY
					CASE m.role
						WHEN 'owner' THEN 0
						WHEN 'admin' THEN 1
						ELSE 2
					END,
					m.created_at ASC
				LIMIT 1
			) AS creator ON true
			LEFT JOIN public.accounts AS a ON a.id = creator.account_id
			WHERE o.status != 'deleted'
		`).Scan(&organizations).Error; err != nil {
			return fmt.Errorf("failed to read organizations for default role templates: %w", err)
		}

		for _, organization := range organizations {
			for _, definition := range workspace_model.DefaultWorkspaceRoleTemplateDefinitions() {
				if err := seedWorkspaceDefaultRoleTemplate(db, organization, definition); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func seedWorkspaceDefaultRoleTemplate(
	db *gorm.DB,
	organization workspaceDefaultRoleTemplateOrganizationSeed,
	definition workspace_model.WorkspaceDefaultRoleTemplateDefinition,
) error {
	var exists int64
	if err := db.Table("public.roles").
		Where("group_id = ?::uuid AND system_key = ?", organization.OrganizationID, definition.SystemKey).
		Count(&exists).Error; err != nil {
		return fmt.Errorf("failed to check default role template %s for organization %s: %w", definition.SystemKey, organization.OrganizationID, err)
	}
	if exists > 0 {
		return nil
	}

	name := workspaceDefaultRoleTemplatePrimaryName(definition, organization.Language)
	description := workspaceDefaultRoleTemplatePrimaryDescription(definition, organization.Language)
	uniqueName, err := uniqueWorkspaceRoleTemplateName(db, organization.OrganizationID, name, organization.Language)
	if err != nil {
		return err
	}

	nameI18n, err := json.Marshal(map[string]string{
		"zh_Hans": definition.NameZhHans,
		"en_US":   definition.NameEnUS,
	})
	if err != nil {
		return fmt.Errorf("failed to encode default role name i18n: %w", err)
	}
	descriptionI18n, err := json.Marshal(map[string]string{
		"zh_Hans": definition.DescZhHans,
		"en_US":   definition.DescEnUS,
	})
	if err != nil {
		return fmt.Errorf("failed to encode default role description i18n: %w", err)
	}
	permissions, err := json.Marshal(workspace_model.CanonicalAssignableWorkspacePermissionSnapshotStrings(definition.Permissions))
	if err != nil {
		return fmt.Errorf("failed to encode default role permissions: %w", err)
	}

	if err := db.Exec(`
		INSERT INTO public.roles (
			group_id,
			name,
			name_i18n,
			description,
			description_i18n,
			status,
			created_by,
			permissions,
			system_key,
			template_origin,
			created_at,
			updated_at
		)
		VALUES (
			?::uuid,
			?,
			?::jsonb,
			?,
			?::jsonb,
			'active',
			?::uuid,
			?::jsonb,
			?,
			?,
			NOW(),
			NOW()
		)
	`, organization.OrganizationID, uniqueName, string(nameI18n), description, string(descriptionI18n), organization.CreatedBy, string(permissions), definition.SystemKey, string(workspace_model.WorkspaceRoleTemplateOriginSystemDefault)).Error; err != nil {
		return fmt.Errorf("failed to seed default role template %s for organization %s: %w", definition.SystemKey, organization.OrganizationID, err)
	}

	return nil
}

func workspaceDefaultRoleTemplatePrimaryName(definition workspace_model.WorkspaceDefaultRoleTemplateDefinition, language string) string {
	if usesEnglishWorkspaceRoleTemplateName(language) {
		return definition.NameEnUS
	}
	return definition.NameZhHans
}

func workspaceDefaultRoleTemplatePrimaryDescription(definition workspace_model.WorkspaceDefaultRoleTemplateDefinition, language string) string {
	if usesEnglishWorkspaceRoleTemplateName(language) {
		return definition.DescEnUS
	}
	return definition.DescZhHans
}

func usesEnglishWorkspaceRoleTemplateName(language string) bool {
	language = strings.ToLower(strings.TrimSpace(language))
	return strings.HasPrefix(language, "en")
}

func uniqueWorkspaceRoleTemplateName(db *gorm.DB, organizationID, preferredName, language string) (string, error) {
	name := strings.TrimSpace(preferredName)
	if name == "" {
		name = "Default Member"
	}
	if workspaceRoleTemplateNameAvailable(db, organizationID, name) {
		return name, nil
	}

	base := name + "（默认）"
	if usesEnglishWorkspaceRoleTemplateName(language) {
		base = name + " (Default)"
	}
	if workspaceRoleTemplateNameAvailable(db, organizationID, base) {
		return base, nil
	}

	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s %d", base, i)
		if workspaceRoleTemplateNameAvailable(db, organizationID, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique workspace role template name for organization %s", organizationID)
}

func workspaceRoleTemplateNameAvailable(db *gorm.DB, organizationID, name string) bool {
	var count int64
	if err := db.Table("public.roles").
		Where("group_id = ?::uuid AND name = ?", organizationID, name).
		Count(&count).Error; err != nil {
		return false
	}
	return count == 0
}
