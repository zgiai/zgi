package runtimeworkspace

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAccessLimitsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Runtime Workspace access limits PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Access limits test')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Access limits owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'Access limits workspace', ?)", workspaceID, organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode, policy_json) VALUES (?, ?, ?, 'active', 'attached', '{}')",
		workspaceID, organizationID, accountID)
	var runtime RuntimeWorkspace
	if err := tx.Where("workspace_id = ?", workspaceID).First(&runtime).Error; err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO runtime_workspace_agents (runtime_workspace_id, agent_id, app_type, public_app_key, status) VALUES (?, ?, 'agent', 'app_scoped', 'active')", runtime.LogicalID(), uuid.New())

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	clock := now
	service := NewService(tx)
	service.now = func() time.Time { return clock }
	subjectID, err := EndUserSubject(runtime.LogicalID(), "alice@example")
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id, created_at, updated_at) VALUES (?, 'api_end_user', ?, ?, now(), now())", subjectID, SubjectIssuerType, runtime.LogicalID())
	if err := service.touchEndUser(t.Context(), runtime.LogicalID(), subjectID, "alice@example"); err != nil {
		t.Fatal(err)
	}

	t.Run("exports every conversation and message as a job", func(t *testing.T) {
		// More conversations and messages than one page, so nothing may be cut off.
		conversations := endUserExportPageSize + 3
		var lastConversation uuid.UUID
		for index := range conversations {
			lastConversation = uuid.New()
			exec("INSERT INTO chat_runtime_conversations (id, organization_id, account_id, title, created_at) VALUES (?, ?, ?, 'c', ?)",
				lastConversation, organizationID, subjectID, now.Add(time.Duration(index)*time.Second))
		}
		messages := endUserExportPageSize + 5
		for index := range messages {
			exec("INSERT INTO chat_runtime_messages (id, conversation_id, query, answer, status, model_name, created_at) VALUES (?, ?, 'q', 'a', 'completed', '', ?)",
				uuid.New(), lastConversation, now.Add(time.Duration(index)*time.Second))
		}
		actor := EndUserActor{Kind: EndUserActorAccount, ID: accountID}
		queued, err := service.RequestEndUserExport(t.Context(), workspaceID, subjectID, actor)
		if err != nil || queued.Status != EndUserExportPending {
			t.Fatalf("queued = %+v, %v", queued, err)
		}
		again, err := service.RequestEndUserExport(t.Context(), workspaceID, subjectID, actor)
		if err != nil || again.ID != queued.ID {
			t.Fatalf("a second request while one is queued = %+v, %v", again, err)
		}
		if _, err := service.OpenEndUserExport(t.Context(), workspaceID, subjectID, queued.ID); !errors.Is(err, ErrExportNotReady) {
			t.Fatalf("download before it ran = %v", err)
		}
		worker := &MaintenanceWorker{service: service, db: tx, now: func() time.Time { return clock }}
		if err := worker.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
		done, err := service.GetEndUserExport(t.Context(), workspaceID, subjectID, queued.ID)
		if err != nil || done.Status != EndUserExportSucceeded || done.SizeBytes == 0 || done.FinishedAt == nil {
			t.Fatalf("export after the worker = %+v, %v", done, err)
		}
		content, err := service.OpenEndUserExport(t.Context(), workspaceID, subjectID, queued.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(content)
		_ = content.Close()
		if err != nil {
			t.Fatal(err)
		}
		var export EndUserExport
		if err := json.Unmarshal(raw, &export); err != nil {
			t.Fatalf("export is not JSON: %v", err)
		}
		if len(export.Conversations) != conversations || len(export.Conversations[conversations-1].Messages) != messages ||
			export.Conversations[conversations-1].ID != lastConversation || export.EndUser.ExternalUserID != "alice@example" {
			t.Fatalf("export has %d conversations, the last with %d messages", len(export.Conversations), len(export.Conversations[conversations-1].Messages))
		}
		next, err := service.RequestEndUserExport(t.Context(), workspaceID, subjectID, actor)
		if err != nil || next.ID == queued.ID {
			t.Fatalf("a request after one finished = %+v, %v", next, err)
		}

		clock = now.Add(endUserExportRetention + time.Hour)
		if _, err := service.GetEndUserExport(t.Context(), workspaceID, subjectID, queued.ID); !errors.Is(err, ErrExportNotFound) {
			t.Fatalf("an expired export = %v", err)
		}
		clock = now
		exec("DELETE FROM runtime_workspace_end_user_exports WHERE id = ?", next.ID)
	})

	t.Run("purges access events past the retention", func(t *testing.T) {
		t.Setenv("ZGI_RUNTIME_WORKSPACE_ACCESS_EVENT_RETENTION_DAYS", "30")
		insert := func(at time.Time) uuid.UUID {
			id := uuid.New()
			exec("INSERT INTO runtime_workspace_access_events (id, runtime_workspace_id, organization_id, workspace_id, method, route, status_code, result, created_at) VALUES (?, ?, ?, ?, 'GET', '/apps', 200, 'success', ?)",
				id, runtime.LogicalID(), organizationID, workspaceID, at)
			return id
		}
		old, recent := insert(now.Add(-31*24*time.Hour)), insert(now.Add(-29*24*time.Hour))
		worker := &MaintenanceWorker{service: service, db: tx, now: func() time.Time { return now }}
		if err := worker.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
		var left []uuid.UUID
		if err := tx.Table("runtime_workspace_access_events").Where("id IN ?", []uuid.UUID{old, recent}).Pluck("id", &left).Error; err != nil {
			t.Fatal(err)
		}
		if len(left) != 1 || left[0] != recent {
			t.Fatalf("access events left = %v", left)
		}
	})

	t.Run("credentials keep their scopes and apps", func(t *testing.T) {
		service.replayProtector, err = NewCredentialReplayProtector("0123456789abcdef0123456789abcdef")
		if err != nil {
			t.Fatal(err)
		}
		create := func(input CredentialInput) (*CreatedCredential, error) {
			return service.CreateScopedCredential(t.Context(), workspaceID, accountID, input, uuid.NewString())
		}
		for _, input := range []CredentialInput{
			{Scopes: []string{"admin"}},
			{AppKeys: []string{"app_missing"}},
			{Scopes: []string{CredentialScopeEndUsersRead}, AppKeys: []string{"app_scoped"}},
		} {
			if _, err := create(input); !errors.Is(err, ErrInvalidCredential) {
				t.Fatalf("create %+v = %v", input, err)
			}
		}
		created, err := create(CredentialInput{Name: "scoped", Scopes: []string{CredentialScopeRuntime, CredentialScopeRuntime}, AppKeys: []string{"app_scoped"}})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := service.AuthenticateCredential(t.Context(), created.Secret)
		if err != nil {
			t.Fatal(err)
		}
		key := principal.Credential
		if len(key.Scopes) != 1 || !key.Allows(CredentialScopeRuntime) || key.Allows(CredentialScopeEndUsersRead) || !key.AllowsApp("app_scoped") || key.AllowsApp("app_other") {
			t.Fatalf("scoped credential = %+v", key)
		}
		full, err := create(CredentialInput{Name: "full"})
		if err != nil {
			t.Fatal(err)
		}
		if full.Credential.Scopes != nil || !full.Credential.Allows(CredentialScopeEndUsersManage) || !full.Credential.AllowsApp("app_other") {
			t.Fatalf("an unscoped credential = %+v", full.Credential)
		}
	})

	t.Run("erasing an end user deletes their exports", func(t *testing.T) {
		queued, err := service.RequestEndUserExport(t.Context(), workspaceID, subjectID, EndUserActor{Kind: EndUserActorAccount, ID: accountID})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.EraseEndUser(t.Context(), workspaceID, subjectID, EndUserActor{Kind: EndUserActorAccount, ID: accountID}); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := tx.Table("runtime_workspace_end_user_exports").Where("id = ?", queued.ID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("exports after erasure = %d, %v", count, err)
		}
	})
}

func TestCredentialScopeRules(t *testing.T) {
	manager := &Credential{Scopes: []string{CredentialScopeEndUsersManage}}
	if !manager.Allows(CredentialScopeEndUsersRead) || manager.Allows(CredentialScopeRuntime) {
		t.Fatal("end_users.manage implies read and nothing else")
	}
	apps := []App{{AppKey: "a"}, {AppKey: "b"}}
	if got := (&Credential{AppKeys: []string{"b"}}).AppsAllowed(apps); len(got) != 1 || got[0].AppKey != "b" {
		t.Fatalf("apps allowed = %+v", got)
	}
	if got := (*Credential)(nil).AppsAllowed(apps); len(got) != 2 {
		t.Fatal("no credential keeps every app")
	}
	if _, _, err := normalizeCredentialLimits(nil, []string{"a"}); err != nil {
		t.Fatal("apps without scopes keep full scopes")
	}
	if RuntimeEndUserRequestsPerMinute(`{"rate_limit":{"requests_per_minute":100,"per_end_user_requests_per_minute":10}}`) != 10 ||
		RuntimeEndUserRequestsPerMinute(`{"rate_limit":{"requests_per_minute":100}}`) != 0 || RuntimeEndUserRequestsPerMinute(`{`) != 0 {
		t.Fatal("per-end-user rate limit policy")
	}
}
