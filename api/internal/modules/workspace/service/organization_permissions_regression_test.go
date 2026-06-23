package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type organizationPermissionTestAccountService struct {
	interfaces.AccountService
}

func (organizationPermissionTestAccountService) GetAccountByID(ctx context.Context, id string) (*auth_model.Account, error) {
	if id == "" {
		return nil, errors.New("account not found")
	}
	return &auth_model.Account{
		ID:     id,
		Name:   id,
		Email:  id + "@example.com",
		Status: auth_model.AccountStatusActive,
	}, nil
}

func TestUpdateMemberStatusCannotDisableOwner(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organizations" WHERE id = $1 ORDER BY "organizations"."id" LIMIT $2`)).
		WithArgs("org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "created_at", "updated_at"}).
			AddRow("org-1", "Org", model.OrganizationStatusActive, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "members" WHERE organization_id = $1 AND account_id = $2 ORDER BY "members"."organization_id" LIMIT $3`)).
		WithArgs("org-1", "owner-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "account_id", "role", "status", "created_at", "updated_at"}).
			AddRow("org-1", "owner-1", model.OrganizationRoleOwner, model.OrganizationMemberStatusActive, now, now))

	err := svc.UpdateMemberStatus(context.Background(), &shared_dto.UpdateOrganizationMemberStatusRequest{
		OrganizationID: "org-1",
		AccountID:      "owner-1",
		Status:         model.OrganizationMemberStatusInactive,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot disable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteCustomWorkspaceRoleRejectsAssignedRole(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	svc := newOrganizationPermissionRegressionService(db)
	now := time.Now().UTC()
	roleID := "role-1"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3`)).
		WithArgs(roleID, "org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "name", "description", "status", "permissions", "created_by", "created_at", "updated_at"}).
			AddRow(roleID, "org-1", "Custom", nil, model.WorkspaceCustomRoleStatusActive, "[]", "owner-1", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id IN (SELECT id FROM "workspaces" WHERE organization_id = $1) AND role_id = $2`)).
		WithArgs("org-1", roleID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := svc.DeleteCustomWorkspaceRole(context.Background(), "org-1", roleID, "owner-1")

	require.ErrorIs(t, err, ErrWorkspaceRoleInUse)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newOrganizationPermissionRegressionService(db *gorm.DB) *organizationService {
	return &organizationService{
		organizationRepo: workspace_repo.NewOrganizationRepository(db),
		accountService:   organizationPermissionTestAccountService{},
	}
}

func newOrganizationPermissionRegressionMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(true)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db, mock
}
