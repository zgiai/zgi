package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/gorm"
)

func TestWorkspaceArchiveDisablesAttachedRuntime(t *testing.T) {
	db, mock := newWorkspaceManagementMockDB(t)
	workspaceID := "10000000-0000-0000-0000-000000000001"
	repo := &runtimeLifecycleWorkspaceRepo{db: db, workspace: &model.Workspace{ID: workspaceID, Name: "Attached", Status: model.WorkspaceStatusNormal}}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "runtime_workspaces" SET .*"status"=\$[0-9]+.*WHERE workspace_id = \$[0-9]+`).
		WithArgs("disabled", workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	archived := model.WorkspaceStatusArchived
	if _, err := NewWorkspaceService(repo).UpdateWorkspace(t.Context(), workspaceID, "", &archived, "account-1", true); err != nil {
		t.Fatal(err)
	}
	if repo.updatedStatus != model.WorkspaceStatusArchived {
		t.Fatalf("workspace status = %q, want archived", repo.updatedStatus)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceArchiveRollsBackWhenRuntimeDisableFails(t *testing.T) {
	db, mock := newWorkspaceManagementMockDB(t)
	workspaceID := "10000000-0000-0000-0000-000000000003"
	repo := &runtimeLifecycleWorkspaceRepo{db: db, workspace: &model.Workspace{ID: workspaceID, Name: "Attached", Status: model.WorkspaceStatusNormal}}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "runtime_workspaces" SET .*"status"=\$[0-9]+.*WHERE workspace_id = \$[0-9]+`).
		WithArgs("disabled", workspaceID).
		WillReturnError(errors.New("runtime update unavailable"))
	mock.ExpectRollback()
	archived := model.WorkspaceStatusArchived
	if _, err := NewWorkspaceService(repo).UpdateWorkspace(t.Context(), workspaceID, "", &archived, "account-1", true); err == nil {
		t.Fatal("archive succeeded while Runtime disable failed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceArchiveRollsBackBeforeRuntimeDisableWhenStatusUpdateFails(t *testing.T) {
	db, mock := newWorkspaceManagementMockDB(t)
	workspaceID := "10000000-0000-0000-0000-000000000004"
	repo := &runtimeLifecycleWorkspaceRepo{db: db, workspace: &model.Workspace{ID: workspaceID, Name: "Attached", Status: model.WorkspaceStatusNormal}, updateStatusErr: errors.New("workspace status unavailable")}
	mock.ExpectBegin()
	mock.ExpectRollback()
	archived := model.WorkspaceStatusArchived
	if _, err := NewWorkspaceService(repo).UpdateWorkspace(t.Context(), workspaceID, "", &archived, "account-1", true); err == nil {
		t.Fatal("archive succeeded while Workspace status update failed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceDeleteRejectsActiveRuntime(t *testing.T) {
	db, mock := newWorkspaceManagementMockDB(t)
	workspaceID := "10000000-0000-0000-0000-000000000002"
	mock.ExpectQuery(`SELECT count\(\*\) FROM "runtime_workspaces" WHERE workspace_id = \$1 AND status = \$2`).
		WithArgs(workspaceID, "active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	service := &WorkspaceManagementServiceImpl{db: db, workspaceRepo: &runtimeLifecycleWorkspaceRepo{db: db}}
	if err := service.DeleteWorkspace(t.Context(), workspaceID); !errors.Is(err, ErrWorkspaceHasActiveRuntime) {
		t.Fatalf("DeleteWorkspace error = %v, want ErrWorkspaceHasActiveRuntime", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type runtimeLifecycleWorkspaceRepo struct {
	repository.WorkspaceRepository
	db              *gorm.DB
	workspace       *model.Workspace
	updatedStatus   model.WorkspaceStatus
	updateStatusErr error
}

func (repo *runtimeLifecycleWorkspaceRepo) GetWorkspaceByID(context.Context, string) (*model.Workspace, error) {
	if repo.workspace == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return repo.workspace, nil
}

func (repo *runtimeLifecycleWorkspaceRepo) UpdateWorkspaceStatus(_ context.Context, _ string, status model.WorkspaceStatus) error {
	if repo.updateStatusErr != nil {
		return repo.updateStatusErr
	}
	repo.updatedStatus = status
	return nil
}

func (repo *runtimeLifecycleWorkspaceRepo) GetDB() *gorm.DB {
	return repo.db
}

func (repo *runtimeLifecycleWorkspaceRepo) WithTx(*gorm.DB) repository.WorkspaceRepository {
	return repo
}
