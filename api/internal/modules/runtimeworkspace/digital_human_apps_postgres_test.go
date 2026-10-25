package runtimeworkspace

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRuntimeWorkspaceBindsAndAuthorizesDigitalHumansPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run Runtime Workspace DigitalHuman PostgreSQL integration")
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
	exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "Runtime DigitalHuman test")
	exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", accountID, "Runtime DigitalHuman owner", uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, ?, ?)", workspaceID, "Runtime DigitalHuman workspace", organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode) VALUES (?, ?, ?, 'active', 'attached')", workspaceID, organizationID, accountID)

	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	digitalHumanID, versionID := uuid.New(), uuid.New()
	exec("INSERT INTO digital_humans (id, web_app_id, organization_id, workspace_id, name, description, status, created_at, updated_at) VALUES (?, ?, ?, ?, 'Runtime DigitalHuman', 'Serves end users', 'active', ?, ?)",
		digitalHumanID, uuid.New(), organizationID, workspaceID, now, now)
	exec("INSERT INTO digital_human_versions (id, digital_human_id, organization_id, workspace_id, state, revision, version_number, name, published_at, created_at, updated_at) VALUES (?, ?, ?, ?, 'published', 'sha256:test', 3, 'Runtime DigitalHuman', ?, ?, ?)",
		versionID, digitalHumanID, organizationID, workspaceID, now, now, now)
	exec("UPDATE digital_humans SET published_version_id = ? WHERE id = ?", versionID, digitalHumanID)
	openAPI := func(enabled bool) {
		t.Helper()
		auth := runtimeauth.ResourceAuthorization{ResourceType: runtimeauth.PublishedRuntimeResourceDigitalHuman, ResourceID: digitalHumanID,
			OrganizationID: organizationID, WorkspaceID: &workspaceID, Surfaces: []runtimeauth.SurfaceAuthorization{{
				Surface: runtimeauth.PublishedRuntimeSurfaceAPI, Enabled: enabled, CompatibilitySource: "digital_human_access",
				Grants: []runtimeauth.SurfaceGrant{{SubjectType: runtimeauth.PublishedRuntimeSubjectPublic, Enabled: enabled}}}}}
		if err := runtimeauth.NewStore(tx).SaveResourceAuthorization(t.Context(), auth); err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(tx, WithClock(func() time.Time { return now }))
	candidates, err := service.ListAgentCandidates(t.Context(), workspaceID, accountID, "current", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range candidates.Items {
		if item.AgentID == digitalHumanID.String() {
			t.Fatal("a DigitalHuman without API access was offered for binding")
		}
	}
	if _, err := service.SyncAppBindings(t.Context(), workspaceID, accountID, 0, []AppRef{{Type: AppTypeDigitalHuman, ID: digitalHumanID}}); !errors.Is(err, ErrAppDisabled) {
		t.Fatalf("binding a DigitalHuman without API access = %v", err)
	}

	openAPI(true)
	candidates, err = service.ListAgentCandidates(t.Context(), workspaceID, accountID, "current", nil)
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, item := range candidates.Items {
		if item.AgentID == digitalHumanID.String() {
			offered = item.AppType == AppTypeDigitalHuman && item.VersionUUID == versionID.String()
		}
	}
	if !offered {
		t.Fatalf("published DigitalHuman candidate missing: %#v", candidates.Items)
	}
	bound, err := service.SyncAppBindings(t.Context(), workspaceID, accountID, 0, []AppRef{{Type: AppTypeDigitalHuman, ID: digitalHumanID}})
	if err != nil || len(bound.Items) != 1 || bound.Items[0].AppType != AppTypeDigitalHuman || !bound.Items[0].Callable {
		t.Fatalf("bind DigitalHuman = %#v, %v", bound, err)
	}
	if _, err := service.SyncAppBindings(t.Context(), workspaceID, accountID, bound.Revision, []AppRef{{Type: AppTypeAgent, ID: digitalHumanID}}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("rebinding a DigitalHuman as an Agent = %v", err)
	}

	runtimeWorkspace, err := service.loadRuntimeWorkspace(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	runtimeID := runtimeWorkspace.LogicalID()
	apps, err := service.ListApps(t.Context(), runtimeID)
	if err != nil || len(apps) != 1 || apps[0].AppType != AppTypeDigitalHuman {
		t.Fatalf("callable apps = %#v, %v", apps, err)
	}
	binding, err := service.ResolveApp(t.Context(), runtimeID, apps[0].AppKey)
	if err != nil || binding.AgentID != digitalHumanID || binding.AppType != AppTypeDigitalHuman {
		t.Fatalf("resolve DigitalHuman app = %#v, %v", binding, err)
	}

	credentialID, subjectID := uuid.New(), uuid.New()
	if err := tx.Create(&Credential{ID: credentialID, RuntimeWorkspaceID: runtimeID, KeyPrefix: "zgi_rt", KeyHash: uuid.NewString()[:32], Name: "test",
		Status: CredentialActive, CreatedByAccountID: accountID, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.CallerActive(t.Context(), runtimeID, credentialID, digitalHumanID); err != nil {
		t.Fatalf("active caller rejected: %v", err)
	}
	if err := service.CallerActive(t.Context(), uuid.New(), credentialID, digitalHumanID); err == nil {
		t.Fatal("a credential was accepted for another Runtime Workspace")
	}
	exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id, created_at, updated_at) VALUES (?, 'api_end_user', ?, ?, ?, ?)", subjectID, SubjectIssuerType, runtimeID, now, now)
	if err := service.SubjectBound(t.Context(), subjectID, digitalHumanID); err != nil {
		t.Fatalf("bound end user rejected: %v", err)
	}

	openAPI(false)
	if err := service.CallerActive(t.Context(), runtimeID, credentialID, digitalHumanID); !errors.Is(err, ErrAppDisabled) {
		t.Fatalf("caller of a DigitalHuman with API access off = %v", err)
	}
	if err := service.SubjectBound(t.Context(), subjectID, digitalHumanID); err == nil {
		t.Fatal("an end user stayed bound after API access was turned off")
	}
	openAPI(true)

	// The bound-app limit is configurable and refused with its own error.
	t.Setenv("ZGI_RUNTIME_WORKSPACE_MAX_APPS", "1")
	listed, err := service.ListBindings(t.Context(), workspaceID)
	if err != nil || listed.Limit != 1 {
		t.Fatalf("bindings limit = %#v, %v", listed, err)
	}
	if _, err := service.SyncAppBindings(t.Context(), workspaceID, accountID, listed.Revision,
		[]AppRef{{Type: AppTypeDigitalHuman, ID: digitalHumanID}, {Type: AppTypeAgent, ID: uuid.New()}}); !errors.Is(err, ErrAppLimitExceeded) {
		t.Fatalf("binding past the limit = %v", err)
	}

	// A selection that still names a deleted app drops it instead of failing.
	exec("UPDATE digital_humans SET deleted_at = ? WHERE id = ?", now, digitalHumanID)
	listed, err = service.ListBindings(t.Context(), workspaceID)
	if err != nil || len(listed.Items) != 1 || listed.Items[0].Callable {
		t.Fatalf("bindings with a deleted app = %#v, %v", listed, err)
	}
	saved, err := service.SyncAppBindings(t.Context(), workspaceID, accountID, listed.Revision, []AppRef{{Type: AppTypeDigitalHuman, ID: digitalHumanID}})
	if err != nil || len(saved.Items) != 0 {
		t.Fatalf("saving a selection with a deleted app = %#v, %v", saved, err)
	}
	if _, err := service.SyncAppBindings(t.Context(), workspaceID, accountID, saved.Revision, []AppRef{{Type: AppTypeDigitalHuman, ID: uuid.New()}}); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("binding an unknown app = %v", err)
	}
}
