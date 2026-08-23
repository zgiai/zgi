package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestListByOrganizationWithPermissionFilterUsesAuthorizedWorkspaceScope(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres mock: %v", err)
	}

	const query = `
		SELECT id, organization_id, workspace_id, name, schema_name, schema_id, description, permission, status, created_by, updated_by, created_at, updated_at, icon_type, icon, icon_background, guard_policy
		FROM data_sources
		WHERE organization_id = $1
	 AND workspace_id IN ($2) ORDER BY created_at DESC`
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "workspace_id", "name", "schema_name", "schema_id",
		"description", "permission", "status", "created_by", "updated_by", "created_at",
		"updated_at", "icon_type", "icon", "icon_background", "guard_policy",
	}).AddRow(
		"datasource-1", "organization-1", "workspace-1", "main", "schema_main", 1,
		"", "only_me", "active", "account-2", "account-2", now, now,
		nil, nil, nil, []byte(`{}`),
	)
	mock.ExpectQuery(query).
		WithArgs("organization-1", "workspace-1").
		WillReturnRows(rows)

	repo := NewPostgresDataSourceRepository(db)
	dataSources, err := repo.ListByOrganizationWithPermissionFilter(
		context.Background(),
		"organization-1",
		"organization-owner-without-membership",
		false,
		[]string{"workspace-1"},
	)
	if err != nil {
		t.Fatalf("list data sources: %v", err)
	}
	if len(dataSources) != 1 || dataSources[0].ID != "datasource-1" {
		t.Fatalf("data sources = %#v, want datasource-1", dataSources)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
