package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

func TestNormalizeWorkspaceRoleName(t *testing.T) {
	t.Parallel()

	name, err := normalizeWorkspaceRoleName("  Project Maintainer  ")
	require.NoError(t, err)
	require.Equal(t, "Project Maintainer", name)

	_, err = normalizeWorkspaceRoleName("  ")
	require.ErrorIs(t, err, ErrWorkspaceRoleNameRequired)

	_, err = normalizeWorkspaceRoleName(strings.Repeat("角", workspaceRoleNameMaxRunes+1))
	require.ErrorIs(t, err, ErrWorkspaceRoleNameTooLong)

	for _, reservedName := range []string{"OWNER", "Admin", "member", "Viewer"} {
		_, err = normalizeWorkspaceRoleName(reservedName)
		require.ErrorIs(t, err, ErrWorkspaceRoleNameReserved)
	}
}

func TestNormalizeWorkspaceRoleDescription(t *testing.T) {
	t.Parallel()

	description := "  Can publish applications.  "
	normalized, err := normalizeWorkspaceRoleDescription(&description)
	require.NoError(t, err)
	require.NotNil(t, normalized)
	require.Equal(t, "Can publish applications.", *normalized)

	description = strings.Repeat("权", workspaceRoleDescriptionMaxRunes+1)
	_, err = normalizeWorkspaceRoleDescription(&description)
	require.ErrorIs(t, err, ErrWorkspaceRoleDescriptionTooLong)
}

func TestWorkspaceRoleNameExistsExcludesDeletedRoles(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "roles" WHERE group_id = $1 AND name = $2 AND status != $3`)).
		WithArgs("org-1", "Reusable", model.WorkspaceCustomRoleStatusDeleted).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	exists, err := workspaceRoleNameExists(context.Background(), db, "org-1", "Reusable", "")

	require.NoError(t, err)
	require.False(t, exists)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListWorkspaceRoleMembersRejectsUnknownTemplate(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3`)).
		WithArgs("role-missing", "org-1", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	result, err := svc.ListWorkspaceRoleMembers(
		context.Background(),
		"org-1",
		"role-missing",
		"owner-1",
		"",
		1,
		20,
	)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrWorkspaceRoleTemplateNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCustomWorkspaceRoleClearsLocalizedMetadata(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()
	description := "Updated description"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3`)).
		WithArgs("role-1", "org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "group_id", "name", "name_i18n", "description", "description_i18n", "status",
			"permissions", "system_key", "template_origin", "created_by", "created_at", "updated_at",
		}).AddRow(
			"role-1", "org-1", "Basic Member", []byte(`{"zh_Hans":"基础成员","en_US":"Basic Member"}`),
			"Default description", []byte(`{"zh_Hans":"默认描述","en_US":"Default description"}`),
			model.WorkspaceCustomRoleStatusActive, []byte(`[]`), model.WorkspaceDefaultRoleTemplateBasicKey,
			model.WorkspaceRoleTemplateOriginSystemDefault, "owner-1", now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "roles" WHERE (group_id = $1 AND name = $2 AND status != $3) AND id != $4`)).
		WithArgs("org-1", "Editors", model.WorkspaceCustomRoleStatusDeleted, "role-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`UPDATE "roles" SET .*"name_i18n"=\$[0-9]+.*"description_i18n"=\$[0-9]+.*WHERE "id" = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	name := " Editors "
	result, err := svc.UpdateCustomWorkspaceRole(context.Background(), &shared_dto.UpdateWorkspaceRoleRequest{
		OrganizationID: "org-1",
		RoleID:         "role-1",
		Name:           &name,
		Description:    &description,
	})

	require.NoError(t, err)
	require.Equal(t, "Editors", result.Name)
	require.Nil(t, result.NameI18n)
	require.Equal(t, "Updated description", *result.Description)
	require.Nil(t, result.DescriptionI18n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCustomWorkspaceRolePreservesLocalizationForUnchangedField(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()
	description := "Updated description"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3`)).
		WithArgs("role-1", "org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "group_id", "name", "name_i18n", "description", "description_i18n", "status",
			"permissions", "system_key", "template_origin", "created_by", "created_at", "updated_at",
		}).AddRow(
			"role-1", "org-1", "Basic Member", []byte(`{"zh_Hans":"基础成员","en_US":"Basic Member"}`),
			"Default description", []byte(`{"zh_Hans":"默认描述","en_US":"Default description"}`),
			model.WorkspaceCustomRoleStatusActive, []byte(`[]`), model.WorkspaceDefaultRoleTemplateBasicKey,
			model.WorkspaceRoleTemplateOriginSystemDefault, "owner-1", now, now,
		))
	mock.ExpectExec(`UPDATE "roles" SET .*"name_i18n"=\$[0-9]+.*"description_i18n"=\$[0-9]+.*WHERE "id" = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	name := "Basic Member"
	result, err := svc.UpdateCustomWorkspaceRole(context.Background(), &shared_dto.UpdateWorkspaceRoleRequest{
		OrganizationID: "org-1",
		RoleID:         "role-1",
		Name:           &name,
		Description:    &description,
	})

	require.NoError(t, err)
	require.NotNil(t, result.NameI18n)
	require.Equal(t, "Basic Member", result.NameI18n.EnUS)
	require.Equal(t, "基础成员", result.NameI18n.ZhHans)
	require.Nil(t, result.DescriptionI18n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceRoleUniqueConstraintViolation(t *testing.T) {
	t.Parallel()

	require.True(t, isWorkspaceRoleUniqueConstraintViolation(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "uk_roles_group_name",
	}))
	require.False(t, isWorkspaceRoleUniqueConstraintViolation(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "uk_roles_group_name",
	}))
	require.True(t, isWorkspaceRoleUniqueConstraintViolation(errors.New("UNIQUE constraint failed: roles.group_id, roles.name")))
	require.False(t, isWorkspaceRoleUniqueConstraintViolation(errors.New("unrelated database error")))
}

func TestEnsureDefaultWorkspaceRoleTemplatesSeedsNewOrganization(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	definitions := model.DefaultWorkspaceRoleTemplateDefinitions()
	rows := sqlmock.NewRows([]string{"id", "group_id", "system_key", "status"})
	mock.ExpectQuery(`SELECT \* FROM "roles" WHERE group_id = \$1 AND system_key IN \(\$2,\$3,\$4\)`).
		WithArgs("org-1", definitions[0].SystemKey, definitions[1].SystemKey, definitions[2].SystemKey).
		WillReturnRows(rows)

	for _, definition := range definitions {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "roles" WHERE group_id = $1 AND name = $2 AND status != $3`)).
			WithArgs("org-1", definition.NameEnUS, model.WorkspaceCustomRoleStatusDeleted).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`INSERT INTO "roles" .* RETURNING "id"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("role-" + definition.SystemKey))
	}

	err := ensureDefaultWorkspaceRoleTemplates(context.Background(), db, "org-1", "owner-1", "en-US")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
