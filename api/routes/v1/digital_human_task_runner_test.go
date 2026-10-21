package v1

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTaskRunScopeKeepsSystemPrincipalSeparateFromAccountActor(t *testing.T) {
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	shared := taskRunScope(organizationID, workspaceID, digitalhuman.SharedTaskOwner(), accountID)
	if shared.ActorID != accountID || shared.PrincipalID != accountID || shared.PrincipalType != string(invocation.PrincipalSystem) || shared.Surface != digitalhuman.SurfaceTask || shared.UserSubjectID != nil {
		t.Fatal("shared task must use the verified creator account for owner-scoped capabilities")
	}
	account := taskRunScope(organizationID, workspaceID, digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountID)), uuid.Nil)
	if account.ActorID != accountID || account.PrincipalID != accountID || account.UserSubjectID == nil || *account.UserSubjectID != accountID {
		t.Fatal("account-owned task must keep its real account actor and system principal")
	}
	apiSubjectID := uuid.New()
	api := taskRunScope(organizationID, workspaceID, digitalhuman.UserTaskOwner(usersubject.Ref{Kind: usersubject.APIEndUser, ID: apiSubjectID}), uuid.Nil)
	if api.ActorID != apiSubjectID || api.PrincipalID != apiSubjectID || api.UserSubjectID == nil || *api.UserSubjectID != apiSubjectID {
		t.Fatal("API task must retain its end-user subject without an account actor")
	}
	channelAccountID := uuid.New()
	channel := taskRunScope(organizationID, workspaceID, digitalhuman.ConversationTaskOwner(uuid.New()), channelAccountID)
	if channel.ActorID != channelAccountID || channel.PrincipalID != channelAccountID || channel.UserSubjectID != nil {
		t.Fatal("channel task must use its verified connection owner without changing conversation ownership")
	}
	for _, owner := range []digitalhuman.TaskOwnerRef{digitalhuman.SharedTaskOwner(), digitalhuman.ConversationTaskOwner(uuid.New())} {
		invalid := taskRunScope(organizationID, workspaceID, owner, uuid.Nil)
		if invalid.ActorID != uuid.Nil || invalid.PrincipalID != uuid.Nil {
			t.Fatal("unresolved account ownership must not fall back to the digital human identity")
		}
	}
}

func TestTaskCreatorAccountRequiresCurrentWorkspaceOwnership(t *testing.T) {
	organizationID, workspaceID, digitalHumanID, accountID, taskID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, testCase := range []struct {
		name  string
		count int
		ok    bool
	}{
		{"current member", 1, true},
		{"revoked member", 0, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := taskRunnerMockDB(t)
			runner := &digitalHumanAutomationRunner{db: db}
			mock.ExpectQuery(`SELECT count\(\*\) FROM automation_tasks AS tasks`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(testCase.count))
			got, err := runner.taskCreatorAccount(t.Context(), organizationID, workspaceID, digitalHumanID,
				automationaction.DigitalHumanRunRequest{TaskID: taskID.String(), CreatorAccountID: accountID.String()})
			if testCase.ok && (err != nil || got != accountID) || !testCase.ok && err == nil {
				t.Fatalf("creator account = %s, error = %v", got, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChannelTaskAccountRequiresCurrentInstallationOwner(t *testing.T) {
	organizationID, workspaceID, digitalHumanID, ownerID, installationID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, testCase := range []struct {
		name  string
		owner string
		ok    bool
	}{
		{"current owner", ownerID.String(), true},
		{"revoked owner", "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := taskRunnerMockDB(t)
			runner := &digitalHumanAutomationRunner{db: db}
			rows := sqlmock.NewRows([]string{"account_id"})
			if testCase.owner != "" {
				rows.AddRow(testCase.owner)
			}
			mock.ExpectQuery(`SELECT installations.connection_owner_id AS account_id FROM channel_installations AS installations`).WillReturnRows(rows)
			got, err := runner.channelTaskAccount(t.Context(), organizationID, workspaceID, digitalHumanID,
				automationaction.DigitalHumanRunRequest{ChannelInstallationID: installationID.String(), ChannelProvider: "feishu", ChannelConversationID: "conversation-1"})
			if testCase.ok && (err != nil || got != ownerID) || !testCase.ok && err == nil {
				t.Fatalf("channel account = %s, error = %v", got, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

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

func TestScheduledTaskEventContextCarriesVerifiedChannelOwner(t *testing.T) {
	conversationID := uuid.New()
	req := automationaction.DigitalHumanRunRequest{TaskID: "task", OriginSurface: digitalhuman.SurfaceChannel, DeliveryMode: "task_only"}
	channel := scheduledTaskEventContext(req, digitalhuman.ConversationTaskOwner(conversationID))
	if channel[digitalhuman.TaskOwnerConversationContextKey] != conversationID.String() {
		t.Fatalf("channel task context = %#v", channel)
	}
	console := scheduledTaskEventContext(req, digitalhuman.SharedTaskOwner())
	if _, ok := console[digitalhuman.TaskOwnerConversationContextKey]; ok {
		t.Fatalf("shared task context carries a channel owner: %#v", console)
	}
}
