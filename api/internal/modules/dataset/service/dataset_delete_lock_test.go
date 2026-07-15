package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type datasetDeleteLockOrganizationService struct {
	interfaces.OrganizationService
	permissionsByWorkspace map[string]bool
	checkedWorkspaces      []string
}

func (s *datasetDeleteLockOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, _ ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkedWorkspaces = append(s.checkedWorkspaces, workspaceID)
	return s.permissionsByWorkspace[workspaceID], nil
}

func TestLockDatasetForDeletionReauthorizesCurrentWorkspace(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	organizationID := "98d25d61-119d-4366-977e-1ebda9ca0bbb"
	lockedWorkspaceID := "da5466f9-371b-4502-92a2-20396f15ff6e"
	organization := &datasetDeleteLockOrganizationService{
		permissionsByWorkspace: map[string]bool{lockedWorkspaceID: false},
	}
	svc := &datasetService{enterpriseService: organization}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "datasets" WHERE id = \$1 LIMIT \$2 FOR UPDATE`).
		WithArgs("dataset-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id", "name", "created_by"}).
			AddRow("dataset-1", organizationID, lockedWorkspaceID, "Knowledge", "account-1"))
	mock.ExpectRollback()

	err = db.Transaction(func(tx *gorm.DB) error {
		_, lockErr := svc.lockDatasetForDeletionInTx(context.Background(), tx, "dataset-1", organizationID, "account-1")
		return lockErr
	})
	if !errors.Is(err, ErrDatasetAccessDenied) {
		t.Fatalf("lockDatasetForDeletionInTx() error = %v, want ErrDatasetAccessDenied", err)
	}
	if len(organization.checkedWorkspaces) != 1 || organization.checkedWorkspaces[0] != lockedWorkspaceID {
		t.Fatalf("checked workspaces = %#v, want locked workspace %s", organization.checkedWorkspaces, lockedWorkspaceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
