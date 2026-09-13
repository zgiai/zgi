package v1

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTaskOwnerFromRunRequestRejectsMixedOwnership(t *testing.T) {
	subjectID := uuid.New()
	owner, err := taskOwnerFromRunRequest(automationaction.DigitalHumanRunRequest{
		OwnerKind: string(digitalhuman.TaskOwnerUserSubject), OwnerSubjectKind: string(usersubject.Account), OwnerSubjectID: subjectID.String(),
	})
	if err != nil || owner.Subject.ID != subjectID {
		t.Fatalf("account owner = %#v, error = %v", owner, err)
	}
	_, err = taskOwnerFromRunRequest(automationaction.DigitalHumanRunRequest{
		OwnerKind: string(digitalhuman.TaskOwnerUserSubject), OwnerSubjectKind: string(usersubject.Account), OwnerSubjectID: subjectID.String(), OwnerConversationID: uuid.NewString(),
	})
	if !errors.Is(err, errDigitalHumanTaskExecutionUnavailable) {
		t.Fatalf("mixed owner error = %v", err)
	}
}

func TestDigitalHumanTaskPreflightRejectsRevokedWorkspaceAccess(t *testing.T) {
	db, mock := taskRunnerMockDB(t)
	runner := &digitalHumanAutomationRunner{db: db}
	organizationID, workspaceID, digitalHumanID, accountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "digital_humans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "user_subjects"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM workspace_members AS members`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := runner.preflight(t.Context(), organizationID, workspaceID, digitalHumanID,
		digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountID)),
		automationaction.DigitalHumanRunRequest{OriginSurface: digitalhuman.SurfaceConsole},
	)
	if !errors.Is(err, errDigitalHumanTaskExecutionUnavailable) {
		t.Fatalf("preflight error = %v, want execution unavailable", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanTaskPreflightAllowsAccountOwnerOnWebApp(t *testing.T) {
	db, mock := taskRunnerMockDB(t)
	runner := &digitalHumanAutomationRunner{db: db}
	organizationID, workspaceID, digitalHumanID, accountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "digital_humans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "user_subjects"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM workspace_members AS members`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := runner.preflight(t.Context(), organizationID, workspaceID, digitalHumanID,
		digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountID)),
		automationaction.DigitalHumanRunRequest{OriginSurface: digitalhuman.SurfaceWebApp},
	)
	if err != nil {
		t.Fatalf("preflight error = %v, want account WebApp task to be executable", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanTaskPreflightAllowsAPIEndUserOwnerOnAPI(t *testing.T) {
	db, mock := taskRunnerMockDB(t)
	runner := &digitalHumanAutomationRunner{db: db}
	organizationID, workspaceID, digitalHumanID, subjectID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "digital_humans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "user_subjects"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := runner.preflight(t.Context(), organizationID, workspaceID, digitalHumanID,
		digitalhuman.UserTaskOwner(usersubject.Ref{Kind: usersubject.APIEndUser, ID: subjectID}),
		automationaction.DigitalHumanRunRequest{OriginSurface: digitalhuman.SurfaceAPI},
	)
	if err != nil {
		t.Fatalf("preflight error = %v, want API end-user task to be executable", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDigitalHumanTaskPreflightRejectsMismatchedPrivateOwnerSurface(t *testing.T) {
	tests := []struct {
		name    string
		owner   digitalhuman.TaskOwnerRef
		surface string
	}{
		{name: "account on api", owner: digitalhuman.UserTaskOwner(usersubject.AccountSubject(uuid.New())), surface: digitalhuman.SurfaceAPI},
		{name: "api user on console", owner: digitalhuman.UserTaskOwner(usersubject.Ref{Kind: usersubject.APIEndUser, ID: uuid.New()}), surface: digitalhuman.SurfaceConsole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := taskRunnerMockDB(t)
			runner := &digitalHumanAutomationRunner{db: db}
			mock.ExpectQuery(`SELECT count\(\*\) FROM "digital_humans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			err := runner.preflight(t.Context(), uuid.New(), uuid.New(), uuid.New(), tt.owner,
				automationaction.DigitalHumanRunRequest{OriginSurface: tt.surface})
			if !errors.Is(err, errDigitalHumanTaskExecutionUnavailable) {
				t.Fatalf("preflight error = %v, want execution unavailable", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDigitalHumanTaskPreflightRejectsDisabledChannelBinding(t *testing.T) {
	db, mock := taskRunnerMockDB(t)
	runner := &digitalHumanAutomationRunner{db: db}
	organizationID, workspaceID, digitalHumanID, installationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	conversationID := "chat-1"
	ownerID, err := digitalhuman.ChannelTaskConversationID(organizationID, workspaceID, digitalHumanID, "feishu", installationID.String(), conversationID)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM "digital_humans"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM channel_installations AS installations`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err = runner.preflight(t.Context(), organizationID, workspaceID, digitalHumanID,
		digitalhuman.ConversationTaskOwner(ownerID),
		automationaction.DigitalHumanRunRequest{
			OriginSurface: digitalhuman.SurfaceChannel, ChannelProvider: "feishu",
			ChannelInstallationID: installationID.String(), ChannelConversationID: conversationID,
		},
	)
	if !errors.Is(err, errDigitalHumanTaskExecutionUnavailable) {
		t.Fatalf("preflight error = %v, want execution unavailable", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func taskRunnerMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}
