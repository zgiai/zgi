package runtimeworkspace

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRuntimeWorkspaceAppCatalogPagesAndCapsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Runtime Workspace app catalog PostgreSQL integration")
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
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Catalog test')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Catalog owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'Catalog workspace', ?)", workspaceID, organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode) VALUES (?, ?, ?, 'active', 'attached')", workspaceID, organizationID, accountID)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	digitalHuman := func(name string, api bool) uuid.UUID {
		t.Helper()
		id, versionID := uuid.New(), uuid.New()
		exec("INSERT INTO digital_humans (id, web_app_id, organization_id, workspace_id, name, description, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, '', 'active', ?, ?)",
			id, uuid.New(), organizationID, workspaceID, name, now, now)
		exec("INSERT INTO digital_human_versions (id, digital_human_id, organization_id, workspace_id, state, revision, version_number, name, published_at, created_at, updated_at) VALUES (?, ?, ?, ?, 'published', 'sha256:test', 1, ?, ?, ?, ?)",
			versionID, id, organizationID, workspaceID, name, now, now, now)
		exec("UPDATE digital_humans SET published_version_id = ? WHERE id = ?", versionID, id)
		auth := runtimeauth.ResourceAuthorization{ResourceType: runtimeauth.PublishedRuntimeResourceDigitalHuman, ResourceID: id,
			OrganizationID: organizationID, WorkspaceID: &workspaceID, Surfaces: []runtimeauth.SurfaceAuthorization{{
				Surface: runtimeauth.PublishedRuntimeSurfaceAPI, Enabled: api, CompatibilitySource: "digital_human_access",
				Grants: []runtimeauth.SurfaceGrant{{SubjectType: runtimeauth.PublishedRuntimeSubjectPublic, Enabled: api}}}}}
		if err := runtimeauth.NewStore(tx).SaveResourceAuthorization(t.Context(), auth); err != nil {
			t.Fatal(err)
		}
		return id
	}
	alpha, beta := digitalHuman("Alpha helper", true), digitalHuman("Beta helper", true)
	digitalHuman("Closed helper", false)
	gamma := digitalHuman("Gamma helper", true)

	service := NewService(tx, WithClock(func() time.Time { return now }))
	candidates := func(query CandidateQuery) *AgentCandidateSet {
		t.Helper()
		set, err := service.ListAppCandidates(t.Context(), workspaceID, accountID, "current", nil, query)
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	names := func(set *AgentCandidateSet) string {
		out := ""
		for _, item := range set.Items {
			out += item.Name + ";"
		}
		return out
	}
	// The unauthorized one is skipped while filling a page, in name order.
	if first := candidates(CandidateQuery{Limit: 2}); names(first) != "Alpha helper;Beta helper;" || !first.HasMore {
		t.Fatalf("first page = %s more=%v", names(first), first.HasMore)
	}
	if second := candidates(CandidateQuery{Limit: 2, Page: 2}); names(second) != "Gamma helper;" || second.HasMore {
		t.Fatalf("second page = %s more=%v", names(second), second.HasMore)
	}
	if found := candidates(CandidateQuery{Search: "gam"}); names(found) != "Gamma helper;" {
		t.Fatalf("search = %s", names(found))
	}
	if agents := candidates(CandidateQuery{Type: AppTypeAgent}); len(agents.Items) != 0 {
		t.Fatalf("agent candidates = %s", names(agents))
	}
	if all, err := service.ListAgentCandidates(t.Context(), workspaceID, accountID, "current", nil); err != nil || len(all.Items) != 3 {
		t.Fatalf("all candidates = %+v %v", all, err)
	}

	bound, err := service.ChangeAppBindings(t.Context(), workspaceID, accountID, 0, []AppRef{{Type: AppTypeDigitalHuman, ID: alpha}, {Type: AppTypeDigitalHuman, ID: beta}}, nil)
	if err != nil || len(bound.Items) != 2 {
		t.Fatalf("add apps = %+v %v", bound, err)
	}
	if _, err := service.ChangeAppBindings(t.Context(), workspaceID, accountID, 0, []AppRef{{Type: AppTypeDigitalHuman, ID: gamma}}, nil); !errors.Is(err, ErrBindingRevisionConflict) {
		t.Fatalf("a stale change = %v", err)
	}
	if rest := candidates(CandidateQuery{ExcludeBound: true}); names(rest) != "Gamma helper;" {
		t.Fatalf("unbound candidates = %s", names(rest))
	}
	keys := map[uuid.UUID]string{}
	for _, app := range bound.Items {
		keys[uuid.MustParse(app.AgentID)] = app.AppKey
	}

	runtimeID, err := service.ResolveRuntimeID(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE runtime_workspaces SET policy_json = ? WHERE workspace_id = ?",
		fmt.Sprintf(`{"end_user_quota": {"apps": [{"app_key": %q, "limits": [{"window": "24h", "points": 1}]}], "plans": [{"key": "free", "limits": [], "apps": []}]}}`, keys[alpha]), workspaceID)
	register := func(user string) uuid.UUID {
		t.Helper()
		subject, err := EndUserSubject(runtimeID, user)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.EnsureEndUser(t.Context(), runtimeID, subject, user); err != nil {
			t.Fatal(err)
		}
		return subject
	}
	bill := func(subject, app uuid.UUID, points int64) {
		t.Helper()
		exec(`INSERT INTO llm_usage_bills (id, attempt_id, request_id, organization_id, app_id, app_type, api_key_id, model_id, model_name, provider_id, provider_name, status, private_points, total_points, end_user_subject_id, request_created_at, settled_at)
			VALUES (?, ?, ?, ?, ?, 'digital_human', ?, ?, 'model', ?, 'provider', 'success', ?, ?, ?, ?, ?)`,
			uuid.New(), uuid.NewString(), uuid.NewString(), organizationID, app, uuid.New(), uuid.New(), uuid.New(), points, points, subject, now.Add(-time.Hour), now)
	}
	capped, lifted := register("capped@example"), register("lifted@example")
	bill(capped, alpha, 2_000)
	bill(capped, beta, 500)
	bill(lifted, alpha, 2_000)
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, lifted, EndUserActor{Kind: EndUserActorAccount, ID: accountID}, EndUserPatch{Plan: &EndUserPlanAssignment{Key: "free"}}); err != nil {
		t.Fatal(err)
	}
	// Per-app caps count in quota states; a plan without them lifts them.
	over, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{QuotaState: QuotaStateOver})
	if err != nil || over.Total != 1 || over.Items[0].SubjectID != capped.String() {
		t.Fatalf("over an app cap = %+v %v", over, err)
	}

	page, err := service.ListBindingsPage(t.Context(), workspaceID, AppListQuery{Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || !page.HasMore || page.Counts.All != 2 || page.Counts.DigitalHumans != 2 || page.Limit != 100 {
		t.Fatalf("first app page = %+v %v", page, err)
	}
	found, err := service.ListBindingsPage(t.Context(), workspaceID, AppListQuery{Search: "alpha"})
	if err != nil || len(found.Items) != 1 {
		t.Fatalf("app search = %+v %v", found, err)
	}
	if usage := found.Items[0].Usage; usage.Points != 4 || usage.Calls != 2 || usage.EndUsers != 2 || len(found.Items[0].QuotaCaps) != 1 {
		t.Fatalf("alpha usage and caps = %+v %+v", usage, found.Items[0].QuotaCaps)
	}
	if callable, _ := service.ListBindingsPage(t.Context(), workspaceID, AppListQuery{State: AppStateUnavailable}); callable.Total != 0 {
		t.Fatalf("unavailable apps = %+v", callable)
	}
	appsPage, err := service.ListAppsPage(t.Context(), runtimeID, 2, 1)
	if err != nil || len(appsPage.Items) != 1 || appsPage.Total != 2 || appsPage.HasMore {
		t.Fatalf("external apps page = %+v %v", appsPage, err)
	}

	removed, err := service.ChangeAppBindings(t.Context(), workspaceID, accountID, page.Revision, nil, []uuid.UUID{beta})
	if err != nil || len(removed.Items) != 1 || removed.Items[0].AgentID != alpha.String() {
		t.Fatalf("remove app = %+v %v", removed, err)
	}
}
