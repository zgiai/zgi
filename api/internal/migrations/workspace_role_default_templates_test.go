package migrations

import (
	"os"
	"strings"
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWorkspaceRoleDefaultTemplatesMigrationUpPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL migration up test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	mustExec(t, db, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	mustExec(t, db, `DROP TABLE IF EXISTS public.roles`)
	mustExec(t, db, `DROP TABLE IF EXISTS public.members`)
	mustExec(t, db, `DROP TABLE IF EXISTS public.organizations`)
	mustExec(t, db, `DROP TABLE IF EXISTS public.accounts`)
	mustExec(t, db, `
		CREATE TABLE public.accounts (
			id uuid PRIMARY KEY,
			name text NOT NULL,
			email text NOT NULL,
			interface_language text
		)
	`)
	mustExec(t, db, `
		CREATE TABLE public.organizations (
			id uuid PRIMARY KEY,
			name text NOT NULL,
			status text NOT NULL
		)
	`)
	mustExec(t, db, `
		CREATE TABLE public.members (
			organization_id uuid NOT NULL,
			account_id uuid NOT NULL,
			role text NOT NULL,
			status text NOT NULL,
			created_at timestamptz NOT NULL
		)
	`)
	mustExec(t, db, `
		CREATE TABLE public.roles (
			id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
			group_id uuid NOT NULL,
			name text NOT NULL,
			description text,
			status text NOT NULL,
			created_by uuid NOT NULL,
			created_at timestamptz NOT NULL DEFAULT NOW(),
			updated_at timestamptz NOT NULL DEFAULT NOW(),
			permissions jsonb DEFAULT '[]'::jsonb
		)
	`)
	mustExec(t, db, `
		INSERT INTO public.accounts (id, name, email, interface_language)
		VALUES ('10000000-0000-0000-0000-000000000001', 'Owner', 'owner@example.com', 'en-US')
	`)
	mustExec(t, db, `
		INSERT INTO public.organizations (id, name, status)
		VALUES ('20000000-0000-0000-0000-000000000001', 'Org', 'active')
	`)
	mustExec(t, db, `
		INSERT INTO public.members (organization_id, account_id, role, status, created_at)
		VALUES ('20000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001', 'owner', 'active', NOW())
	`)
	mustExec(t, db, `
		INSERT INTO public.roles (group_id, name, description, status, created_by, permissions)
		VALUES (
			'20000000-0000-0000-0000-000000000001',
			'Advanced Member',
			'legacy custom',
			'active',
			'10000000-0000-0000-0000-000000000001',
			'["workspace.manage","agent.manage"]'::jsonb
		)
	`)

	if err := upWorkspaceRoleDefaultTemplates(mschema.New(db)); err != nil {
		t.Fatalf("run migration up: %v", err)
	}
	if err := upWorkspaceRoleDefaultTemplates(mschema.New(db)); err != nil {
		t.Fatalf("run migration up second time: %v", err)
	}
	mustExec(t, db, `
		UPDATE public.roles
		SET name = 'Renamed Template', description = 'Renamed description'
		WHERE system_key = 'default_basic'
	`)
	mustExec(t, db, `
		INSERT INTO public.roles (group_id, name, description, status, created_by, permissions)
		VALUES (
			'20000000-0000-0000-0000-000000000001',
			'Reusable',
			'deleted template',
			'deleted',
			'10000000-0000-0000-0000-000000000001',
			'[]'::jsonb
		)
	`)
	if err := upFixWorkspaceRoleTemplateLifecycle(mschema.New(db)); err != nil {
		t.Fatalf("run role template lifecycle migration: %v", err)
	}
	mustExec(t, db, `
		INSERT INTO public.roles (group_id, name, description, status, created_by, permissions)
		VALUES (
			'20000000-0000-0000-0000-000000000001',
			'Reusable',
			'new template',
			'active',
			'10000000-0000-0000-0000-000000000001',
			'[]'::jsonb
		)
	`)

	var defaultCount int64
	if err := db.Table("public.roles").
		Where("group_id = ? AND system_key IN ?", "20000000-0000-0000-0000-000000000001", []string{
			workspace_model.WorkspaceDefaultRoleTemplateAdvancedKey,
			workspace_model.WorkspaceDefaultRoleTemplateBasicKey,
			workspace_model.WorkspaceDefaultRoleTemplateReadonlyKey,
		}).
		Count(&defaultCount).Error; err != nil {
		t.Fatalf("count default templates: %v", err)
	}
	if defaultCount != 3 {
		t.Fatalf("default templates count = %d, want 3", defaultCount)
	}

	var localizedMetadata struct {
		NameI18n        string
		DescriptionI18n string
	}
	if err := db.Table("public.roles").
		Select("name_i18n::text AS name_i18n, description_i18n::text AS description_i18n").
		Where("system_key = ?", workspace_model.WorkspaceDefaultRoleTemplateBasicKey).
		Scan(&localizedMetadata).Error; err != nil {
		t.Fatalf("read renamed template localized metadata: %v", err)
	}
	if localizedMetadata.NameI18n != "{}" || localizedMetadata.DescriptionI18n != "{}" {
		t.Fatalf("renamed template retained stale localized metadata: %#v", localizedMetadata)
	}

	var permissions string
	if err := db.Table("public.roles").
		Select("permissions::text").
		Where("name = ?", "Advanced Member").
		Scan(&permissions).Error; err != nil {
		t.Fatalf("read sanitized custom role permissions: %v", err)
	}
	if strings.Contains(permissions, "workspace.permission.manage") || strings.Contains(permissions, "workspace.manage") {
		t.Fatalf("custom role permissions should not contain governance permissions: %s", permissions)
	}
	if !strings.Contains(permissions, "agent.create") {
		t.Fatalf("custom role permissions should keep expanded asset permissions: %s", permissions)
	}
}

func TestDefaultWorkspaceRoleTemplateDefinitionsExcludeNonConfigurablePermissions(t *testing.T) {
	assignablePermissions := make(map[string]struct{})
	assignableCodes := workspace_model.CanonicalAssignableWorkspacePermissionSnapshotStrings(
		workspace_model.WorkspacePermissionStringsFromCodes(workspace_model.AllWorkspacePermissionCodes()),
	)
	for _, permission := range assignableCodes {
		assignablePermissions[permission] = struct{}{}
	}
	for _, definition := range workspace_model.DefaultWorkspaceRoleTemplateDefinitions() {
		for _, permission := range definition.Permissions {
			code := workspace_model.WorkspacePermissionCode(permission)
			if workspace_model.IsWorkspaceGovernancePermission(code) {
				t.Fatalf("default template %s contains governance permission %s", definition.SystemKey, permission)
			}
			if strings.HasPrefix(permission, "prompt.") || strings.HasPrefix(permission, "content_parse.") {
				t.Fatalf("default template %s contains retired tool permission %s", definition.SystemKey, permission)
			}
			if _, ok := assignablePermissions[permission]; !ok {
				t.Fatalf("default template %s contains non-configurable permission %s", definition.SystemKey, permission)
			}
		}
	}
}

func TestWorkspaceRoleTemplateLifecycleMigrationUsesActiveOnlyUniqueness(t *testing.T) {
	t.Parallel()

	for name, statement := range map[string]string{
		"name":       addActiveWorkspaceRoleNameIndexSQL,
		"system_key": addActiveWorkspaceRoleSystemKeyIndexSQL,
	} {
		if !strings.Contains(statement, "status != 'deleted'") {
			t.Fatalf("%s unique index must exclude deleted role templates: %s", name, statement)
		}
	}
	if !strings.Contains(addActiveWorkspaceRoleNameIndexSQL, "(group_id, name)") {
		t.Fatalf("name unique index must remain scoped to an organization: %s", addActiveWorkspaceRoleNameIndexSQL)
	}
}

func TestWorkspaceRoleTemplateLifecycleMigrationClearsStaleLocalizedOverrides(t *testing.T) {
	t.Parallel()

	for name, statement := range map[string]string{
		"name":        clearStaleWorkspaceRoleNameI18nSQL,
		"description": clearStaleWorkspaceRoleDescriptionI18nSQL,
	} {
		if !strings.Contains(statement, "jsonb_each_text") || !strings.Contains(statement, "NOT EXISTS") {
			t.Fatalf("%s localized metadata repair must preserve matching defaults and clear stale overrides: %s", name, statement)
		}
	}
}
