package approval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSubmitByTokenForWorkflowRunRejectsDifferentRunBeforeUpdate(t *testing.T) {
	db, mock, closeDB := openApprovalServiceMockDB(t)
	defer closeDB()

	now := time.Now()
	mock.ExpectQuery(`SELECT .*FROM "workflow_approval_forms".*access_token`).
		WillReturnRows(sqlmock.NewRows(approvalFormColumns()).AddRow(
			"form-1",
			"tenant-1",
			"agent-1",
			"other-run",
			"approval-node",
			"Approval",
			"approval-token",
			`{"content":"Approve","fields":[],"actions":[{"id":"approve","label":"Approve"}],"rendered_content":"Approve","default_values":{},"expiration_at":"2099-01-01T00:00:00Z"}`,
			"Approve",
			FormStatusWaiting,
			now.Add(time.Hour),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			now,
			now,
		))

	service := NewServiceWithDependencies(db, nil, nil, nil)
	_, err := service.SubmitByTokenForWorkflowRun(context.Background(), "approval-token", "owned-run", SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{},
	}, nil, nil)

	if !errors.Is(err, ErrFormNotFound) {
		t.Fatalf("error = %v, want ErrFormNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations were not met: %v", err)
	}
}

func TestEnsureFormWorkflowRunAllowsTrimmedMatch(t *testing.T) {
	err := ensureFormWorkflowRun(&Form{WorkflowRunID: " owned-run "}, "owned-run")
	if err != nil {
		t.Fatalf("ensureFormWorkflowRun returned error: %v", err)
	}
}

func TestEnsureFormWorkflowRunRejectsMissingOrDifferentRun(t *testing.T) {
	for name, form := range map[string]*Form{
		"missing_form": nil,
		"missing_run":  {WorkflowRunID: ""},
		"different":    {WorkflowRunID: "other-run"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ensureFormWorkflowRun(form, "owned-run")
			if !errors.Is(err, ErrFormNotFound) {
				t.Fatalf("error = %v, want ErrFormNotFound", err)
			}
		})
	}
}

func openApprovalServiceMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm sqlmock: %v", err)
	}
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func approvalFormColumns() []string {
	return []string{
		"id",
		"tenant_id",
		"app_id",
		"workflow_run_id",
		"node_id",
		"node_title",
		"access_token",
		"form_definition",
		"rendered_content",
		"status",
		"expiration_time",
		"selected_action_id",
		"submitted_data",
		"submitted_at",
		"submission_user_id",
		"submission_end_user_id",
		"completed_by_recipient_id",
		"created_at",
		"updated_at",
	}
}
