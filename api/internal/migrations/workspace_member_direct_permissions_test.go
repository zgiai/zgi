package migrations

import (
	"os"
	"strings"
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWorkspaceMemberDirectPermissionsMigrationUpDownPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL migration up/down test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	mustExec(t, db, `DROP TABLE IF EXISTS public.workspace_members`)
	mustExec(t, db, `DROP TABLE IF EXISTS public.roles`)
	mustExec(t, db, `
		CREATE TABLE public.roles (
			id uuid PRIMARY KEY,
			group_id uuid,
			name text,
			status text NOT NULL,
			permissions jsonb NOT NULL DEFAULT '[]'::jsonb
		)
	`)
	mustExec(t, db, `
		CREATE TABLE public.workspace_members (
			id uuid PRIMARY KEY,
			workspace_id uuid NOT NULL,
			account_id uuid NOT NULL,
			role varchar(16) NOT NULL DEFAULT 'normal',
			role_id uuid
		)
	`)
	mustExec(t, db, `
		INSERT INTO public.roles (id, group_id, name, status, permissions)
		VALUES (
			'10000000-0000-0000-0000-000000000001',
			'20000000-0000-0000-0000-000000000001',
			'Custom Agent Builder',
			'active',
			'["agent.manage"]'::jsonb
		)
	`)
	mustExec(t, db, `
		INSERT INTO public.workspace_members (id, workspace_id, account_id, role, role_id)
		VALUES
			('30000000-0000-0000-0000-000000000001', '40000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000001', 'owner', NULL),
			('30000000-0000-0000-0000-000000000002', '40000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000002', 'admin', '00000000-0000-0000-0000-000000000002'),
			('30000000-0000-0000-0000-000000000003', '40000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000003', 'normal', '10000000-0000-0000-0000-000000000001')
	`)

	if err := upAddWorkspaceMemberDirectPermissions(mschema.New(db)); err != nil {
		t.Fatalf("run migration up: %v", err)
	}

	assertWorkspaceMemberPermissionRow(t, db, "50000000-0000-0000-0000-000000000001", "owner", "00000000-0000-0000-0000-000000000001", "[]", "")
	assertWorkspaceMemberPermissionRow(t, db, "50000000-0000-0000-0000-000000000002", "role_template", "00000000-0000-0000-0000-000000000002", "agent.create", "agent.manage")
	assertWorkspaceMemberPermissionRow(t, db, "50000000-0000-0000-0000-000000000003", "role_template", "10000000-0000-0000-0000-000000000001", "agent.create", "agent.manage")

	if err := downAddWorkspaceMemberDirectPermissions(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatalf("run migration down: %v", err)
	}

	for _, column := range []string{"permissions", "permission_source", "permission_template_role_id"} {
		var count int64
		if err := db.Raw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'workspace_members'
			  AND column_name = ?
		`, column).Scan(&count).Error; err != nil {
			t.Fatalf("check dropped column %s: %v", column, err)
		}
		if count != 0 {
			t.Fatalf("expected column %s to be dropped, got count %d", column, count)
		}
	}
}

func TestBuildWorkspaceMemberPermissionSeedCanonicalizesLegacyCoarsePermissions(t *testing.T) {
	customRoleID := "10000000-0000-0000-0000-000000000001"

	tests := []struct {
		name        string
		row         workspaceMemberPermissionSeedRow
		wantSource  string
		wantRoleID  string
		wantContain string
		wantMissing string
	}{
		{
			name: "legacy admin role",
			row: workspaceMemberPermissionSeedRow{
				ID:   "member-admin",
				Role: "admin",
			},
			wantSource:  "legacy_role",
			wantRoleID:  "00000000-0000-0000-0000-000000000002",
			wantContain: "agent.create",
			wantMissing: "agent.manage",
		},
		{
			name: "custom role",
			row: workspaceMemberPermissionSeedRow{
				ID:              "member-custom",
				Role:            "normal",
				RoleID:          &customRoleID,
				RolePermissions: `["database.manage","file.view"]`,
			},
			wantSource:  "role_template",
			wantRoleID:  customRoleID,
			wantContain: "database.schema.manage",
			wantMissing: "database.manage",
		},
		{
			name: "owner role",
			row: workspaceMemberPermissionSeedRow{
				ID:   "member-owner",
				Role: "owner",
			},
			wantSource:  "owner",
			wantRoleID:  "00000000-0000-0000-0000-000000000001",
			wantContain: "",
			wantMissing: "agent.manage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, templateRoleID, permissions, err := buildWorkspaceMemberPermissionSeed(tt.row)
			if err != nil {
				t.Fatalf("build seed: %v", err)
			}
			if string(source) != tt.wantSource {
				t.Fatalf("source = %q, want %q", source, tt.wantSource)
			}
			if templateRoleID != tt.wantRoleID {
				t.Fatalf("template role id = %q, want %q", templateRoleID, tt.wantRoleID)
			}
			if tt.wantContain != "" && !containsString(permissions, tt.wantContain) {
				t.Fatalf("permissions = %#v, want contain %q", permissions, tt.wantContain)
			}
			if tt.wantMissing != "" && containsString(permissions, tt.wantMissing) {
				t.Fatalf("permissions = %#v, should not contain %q", permissions, tt.wantMissing)
			}
		})
	}
}

func assertWorkspaceMemberPermissionRow(t *testing.T, db *gorm.DB, accountID, wantSource, wantTemplateRoleID, wantPermissionSubstring, wantMissingPermissionSubstring string) {
	t.Helper()

	var row struct {
		PermissionSource         string
		PermissionTemplateRoleID string
		Permissions              string
	}
	if err := db.Table("public.workspace_members").
		Select("permission_source, permission_template_role_id::text AS permission_template_role_id, permissions::text AS permissions").
		Where("account_id = ?", accountID).
		Scan(&row).Error; err != nil {
		t.Fatalf("read workspace member permissions for %s: %v", accountID, err)
	}
	if row.PermissionSource != wantSource {
		t.Fatalf("permission source for %s = %q, want %q", accountID, row.PermissionSource, wantSource)
	}
	if row.PermissionTemplateRoleID != wantTemplateRoleID {
		t.Fatalf("template role id for %s = %q, want %q", accountID, row.PermissionTemplateRoleID, wantTemplateRoleID)
	}
	if !strings.Contains(row.Permissions, wantPermissionSubstring) {
		t.Fatalf("permissions for %s = %s, want substring %q", accountID, row.Permissions, wantPermissionSubstring)
	}
	if wantMissingPermissionSubstring != "" && strings.Contains(row.Permissions, wantMissingPermissionSubstring) {
		t.Fatalf("permissions for %s = %s, should not contain substring %q", accountID, row.Permissions, wantMissingPermissionSubstring)
	}
}

func mustExec(t *testing.T, db *gorm.DB, sql string) {
	t.Helper()
	if err := db.Exec(sql).Error; err != nil {
		t.Fatalf("exec SQL failed: %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
