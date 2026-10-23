package runtimeworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEndUserRegistryListsCapsAndSuspendsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Runtime Workspace end-user PostgreSQL integration")
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
	exec("INSERT INTO organizations (id, name) VALUES (?, 'End users test')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'End users owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'End users workspace', ?)", workspaceID, organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode, policy_json) VALUES (?, ?, ?, 'active', 'attached', ?)",
		workspaceID, organizationID, accountID, `{"end_user_quota": {"limits": [{"window": "24h", "points": 10}], "apps": [{"app_key": "app_one", "limits": [{"window": "30d", "points": 40}]}]}}`)
	var runtime RuntimeWorkspace
	if err := tx.Where("workspace_id = ?", workspaceID).First(&runtime).Error; err != nil {
		t.Fatal(err)
	}
	appID := uuid.New()
	exec("INSERT INTO runtime_workspace_agents (runtime_workspace_id, agent_id, app_type, public_app_key, status) VALUES (?, ?, 'agent', 'app_one', 'active')", runtime.LogicalID(), appID)

	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	clock := now
	service := NewService(tx)
	service.now = func() time.Time { return clock }
	touch := func(user string) uuid.UUID {
		t.Helper()
		subjectID, err := EndUserSubject(runtime.LogicalID(), user)
		if err != nil {
			t.Fatal(err)
		}
		exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id, created_at, updated_at) VALUES (?, 'api_end_user', ?, ?, now(), now()) ON CONFLICT DO NOTHING", subjectID, SubjectIssuerType, runtime.LogicalID())
		if err := service.touchEndUser(t.Context(), runtime.LogicalID(), subjectID, user); err != nil {
			t.Fatal(err)
		}
		return subjectID
	}
	alice := touch("alice@example")
	clock = now.Add(30 * time.Second)
	touch("alice@example")
	var stored EndUser
	if err := tx.Where("subject_id = ?", alice).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.LastSeenAt.Equal(now) || stored.ExternalUserID != "alice@example" || stored.Status != EndUserActive {
		t.Fatalf("a request within a minute rewrote the end user: %+v", stored)
	}
	clock = now.Add(2 * time.Minute)
	bob := touch("bob@example")
	clock = now

	bill := func(subject uuid.UUID, app *uuid.UUID, points int64, at time.Time) {
		t.Helper()
		appType := any(nil)
		if app != nil {
			appType = "agent"
		}
		exec(`INSERT INTO llm_usage_bills (id, attempt_id, request_id, organization_id, app_id, app_type, api_key_id, model_id, model_name, provider_id, provider_name, status, private_points, total_points, end_user_subject_id, request_created_at, settled_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'model', ?, 'provider', 'success', ?, ?, ?, ?, ?)`,
			uuid.New(), uuid.NewString(), uuid.NewString(), organizationID, app, appType, uuid.New(), uuid.New(), uuid.New(), points, points, subject, at, at)
	}
	bill(alice, &appID, 4_000, now.Add(-time.Hour))
	bill(alice, &appID, 30_000, now.Add(-10*24*time.Hour))
	bill(alice, nil, 1_500, now.Add(-2*time.Hour))

	list, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || list.Items[0].SubjectID != bob.String() || list.Items[1].ExternalUserID != "alice@example" {
		t.Fatalf("end users = %+v", list)
	}
	view := list.Items[1]
	if view.QuotaSource != "default" || view.Usage[0].Points != 5.5 || view.Usage[0].Limit == nil || *view.Usage[0].Remaining != 4.5 || view.Usage[2].Points != 35.5 || view.Usage[2].Limit != nil {
		t.Fatalf("alice usage = %+v", view.Usage)
	}
	if len(view.Apps) != 1 || view.Apps[0].AppKey != "app_one" || view.Apps[0].Usage[2].Points != 34 || *view.Apps[0].Usage[2].Remaining != 6 {
		t.Fatalf("alice apps = %+v", view.Apps)
	}
	if found, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{Search: "ali"}); err != nil || found.Total != 1 {
		t.Fatalf("search = %+v %v", found, err)
	}
	if found, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{Search: "%"}); err != nil || found.Total != 0 {
		t.Fatalf("a wildcard search matched = %+v %v", found, err)
	}

	guard := NewQuotaGuard(tx)
	guard.now = func() time.Time { return now }
	check := func(subject uuid.UUID) error {
		return guard.CheckEndUserQuota(t.Context(), chatruntime.EndUserQuotaRef{SubjectID: subject, AppID: appID.String()})
	}
	if err := check(alice); err != nil {
		t.Fatalf("alice within quota was refused: %v", err)
	}
	actor := EndUserActor{Kind: EndUserActorAccount, ID: accountID}
	five := QuotaLimit{Window: "24h", Points: decimal.NewFromInt(5)}
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, EndUserPatch{Quota: &EndUserQuotaOverride{Limits: []QuotaLimit{five}}}); err != nil {
		t.Fatal(err)
	}
	if err := check(alice); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("alice's own 5-point limit let 5.5 points through: %v", err)
	}
	updated, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, EndUserPatch{Quota: &EndUserQuotaOverride{Unlimited: true}})
	if err != nil || updated.QuotaSource != "unlimited" || updated.Usage[0].Limit != nil {
		t.Fatalf("unlimited alice = %+v %v", updated, err)
	}
	if err := check(alice); err != nil {
		t.Fatalf("unlimited alice was refused: %v", err)
	}
	bill(alice, &appID, 10_000, now.Add(-time.Minute))
	if err := check(alice); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("the per-app cap no longer held unlimited alice: %v", err)
	}

	suspended := EndUserSuspended
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, bob, actor, EndUserPatch{Status: &suspended}); err != nil {
		t.Fatal(err)
	}
	if err := check(bob); !apperror.IsCode(err, chatruntime.CodeEndUserSuspended) {
		t.Fatalf("suspended bob was admitted: %v", err)
	}
	active := EndUserActive
	if view, err := service.UpdateEndUser(t.Context(), workspaceID, bob, actor, EndUserPatch{Status: &active}); err != nil || view.Status != EndUserActive || view.SuspendedAt != nil {
		t.Fatalf("resumed bob = %+v %v", view, err)
	}
	if err := check(bob); err != nil {
		t.Fatalf("resumed bob was refused: %v", err)
	}
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, EndUserPatch{ResetQuota: true}); err != nil {
		t.Fatal(err)
	}
	eventPage, err := service.ListEndUserEvents(t.Context(), workspaceID, alice, 1, 2)
	if err != nil || len(eventPage.Items) != 2 || !eventPage.HasMore {
		t.Fatalf("first page of alice events = %+v %v", eventPage, err)
	}
	eventPage, err = service.ListEndUserEvents(t.Context(), workspaceID, alice, 1, 50)
	events := eventPage.Items
	actions := map[string]int{}
	for _, event := range events {
		actions[event.Action]++
	}
	if err != nil || len(events) != 3 || actions[EndUserEventQuotaSet] != 2 || actions[EndUserEventQuotaReset] != 1 {
		t.Fatalf("alice events = %+v %v", events, err)
	}
	if bobPage, _ := service.ListEndUserEvents(t.Context(), workspaceID, bob, 1, 50); len(bobPage.Items) != 2 || bobPage.Items[0].ActorKind != EndUserActorAccount || bobPage.Items[0].ActorID == nil {
		bobEvents := bobPage.Items
		t.Fatalf("bob events = %+v", bobEvents)
	}

	bad := "erased"
	for name, patch := range map[string]EndUserPatch{
		"erase by patch":   {Status: &bad},
		"empty quota":      {Quota: &EndUserQuotaOverride{}},
		"unlimited limits": {Quota: &EndUserQuotaOverride{Unlimited: true, Limits: []QuotaLimit{five}}},
		"quota and reset":  {Quota: &EndUserQuotaOverride{Unlimited: true}, ResetQuota: true},
	} {
		if _, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, patch); !errors.Is(err, ErrInvalidWorkspace) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	if _, err := service.GetEndUser(t.Context(), workspaceID, uuid.New()); !errors.Is(err, ErrEndUserNotFound) {
		t.Fatalf("an unknown end user = %v", err)
	}

	// The admin views: usage order, quota states, the summary, and the series.
	if ranked, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{Sort: "usage_30d"}); err != nil || ranked.Items[0].SubjectID != alice.String() {
		t.Fatalf("by 30-day usage = %+v %v", ranked, err)
	}
	if ranked, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{Sort: "usage_30d", Ascending: true}); err != nil || ranked.Items[0].SubjectID != bob.String() {
		t.Fatalf("by 30-day usage, ascending = %+v %v", ranked, err)
	}
	eighteen := QuotaLimit{Window: "24h", Points: decimal.NewFromInt(18)}
	unknown := uuid.New()
	batch, err := service.UpdateEndUsers(t.Context(), workspaceID, []uuid.UUID{alice, bob, alice, unknown}, actor, EndUserPatch{Quota: &EndUserQuotaOverride{Limits: []QuotaLimit{eighteen}}})
	if err != nil || batch.Updated != 2 || len(batch.Failed) != 1 || batch.Failed[0] != unknown.String() {
		t.Fatalf("batch = %+v %v", batch, err)
	}
	if _, err := service.UpdateEndUsers(t.Context(), workspaceID, []uuid.UUID{alice}, actor, EndUserPatch{Status: &bad}); !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("an invalid batch = %v", err)
	}
	states := func(state string) int64 {
		t.Helper()
		list, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{QuotaState: state})
		if err != nil {
			t.Fatal(err)
		}
		return list.Total
	}
	// alice is near her own 18 points, but has spent app_one's 30-day cap
	// (47 of 40), which counts too.
	if near, over, custom := states(QuotaStateNear), states(QuotaStateOver), states(QuotaStateCustom); near != 0 || over != 1 || custom != 2 {
		t.Fatalf("quota states near=%d over=%d custom=%d", near, over, custom)
	}
	summary, err := service.SummarizeEndUsers(t.Context(), workspaceID)
	if err != nil || summary.Usage[0].Points != 15.5 || summary.Usage[0].EndUsers != 1 || summary.Usage[2].Points != 45.5 ||
		summary.Usage[1].PreviousPoints != 30 || summary.Usage[0].PreviousPoints != 0 ||
		summary.EndUsers.Total != 2 || summary.EndUsers.NearQuota != 0 || summary.EndUsers.OverQuota != 1 || summary.EndUsers.CustomQuota != 2 || summary.EndUsers.Active7d != 2 ||
		len(summary.Top) != 2 || summary.Top[0].SubjectID != alice.String() {
		t.Fatalf("summary = %+v %v", summary, err)
	}
	bill(alice, &appID, 3_000, now.Add(-time.Minute))
	if near, over := states(QuotaStateNear), states(QuotaStateOver); near != 0 || over != 1 {
		t.Fatalf("over the limit near=%d over=%d", near, over)
	}
	series, err := service.EndUserUsageSeries(t.Context(), workspaceID, nil, 30, "Asia/Shanghai")
	if err != nil || len(series.Days) != 30 || series.Days[29].Date != "2026-10-23" || series.Days[29].Points != 18.5 ||
		series.Days[19].Points != 30 || series.Days[29].Apps[appID.String()] != 17 || len(series.Apps) != 1 || series.Apps[0].AppKey != "app_one" ||
		series.Days[29].EndUsers != 1 || series.Days[29].Calls != 4 || series.Days[19].Calls != 1 {
		t.Fatalf("series = %+v %v", series, err)
	}
	if own, err := service.EndUserUsageSeries(t.Context(), workspaceID, &bob, 7, ""); err != nil || len(own.Days) != 7 || own.Days[6].Points != 0 {
		t.Fatalf("bob's series = %+v %v", own, err)
	}

	// Plans (ADR 0011): a plan replaces the defaults until it expires, and an
	// end user's own quota still wins.
	exec("UPDATE runtime_workspaces SET policy_json = ? WHERE workspace_id = ?",
		`{"end_user_quota": {"limits": [{"window": "24h", "points": 10}], "apps": [{"app_key": "app_one", "limits": [{"window": "30d", "points": 40}]}],
		  "plans": [{"key": "pro", "name": "Pro", "limits": [{"window": "24h", "points": 18}], "apps": []}]}}`, workspaceID)
	if _, err := service.UpdateEndUsers(t.Context(), workspaceID, []uuid.UUID{alice, bob}, actor, EndUserPatch{ResetQuota: true}); err != nil {
		t.Fatal(err)
	}
	for name, plan := range map[string]EndUserPlanAssignment{
		"an unknown plan": {Key: "gold"},
		"a past expiry":   {Key: "pro", ExpiresAt: ptrTime(now.Add(-time.Hour))},
	} {
		if _, err := service.UpdateEndUser(t.Context(), workspaceID, bob, actor, EndUserPatch{Plan: &plan}); !errors.Is(err, ErrInvalidWorkspace) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	guard.now = func() time.Time { return clock }
	if err := check(alice); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("alice on the defaults was admitted: %v", err)
	}
	expiry := now.Add(time.Hour)
	held, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, EndUserPatch{Plan: &EndUserPlanAssignment{Key: "pro", ExpiresAt: &expiry}})
	if err != nil || held.QuotaSource != QuotaSourcePlan || held.Plan == nil || !held.Plan.Active || held.Plan.Name != "Pro" || *held.Usage[0].Limit != 18 || len(held.Apps) != 1 || held.Apps[0].Usage[2].Limit != nil {
		t.Fatalf("alice on pro = %+v %v", held, err)
	}
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, bob, actor, EndUserPatch{Plan: &EndUserPlanAssignment{Key: "pro"}}); err != nil {
		t.Fatal(err)
	}
	if err := check(alice); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("alice spent 18.5 of pro's 18 points and was admitted: %v", err)
	}
	if err := check(bob); err != nil {
		t.Fatalf("bob on pro was refused: %v", err)
	}
	if over := states(QuotaStateOver); over != 1 {
		t.Fatalf("over a plan limit = %d", over)
	}
	plans := func(plan string) int64 {
		t.Helper()
		list, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{Plan: plan})
		if err != nil {
			t.Fatal(err)
		}
		return list.Total
	}
	if pro, none := plans("pro"), plans("none"); pro != 2 || none != 0 {
		t.Fatalf("plan filter pro=%d none=%d", pro, none)
	}
	if summary, err := service.SummarizeEndUsers(t.Context(), workspaceID); err != nil || len(summary.Plans) != 1 || summary.Plans[0].EndUsers != 2 {
		t.Fatalf("summary plans = %+v %v", summary, err)
	}
	unlimited, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, EndUserPatch{Quota: &EndUserQuotaOverride{Unlimited: true}})
	if err != nil || unlimited.QuotaSource != QuotaSourceUnlimited || unlimited.Plan == nil || !unlimited.Plan.Active {
		t.Fatalf("alice's own quota over pro = %+v %v", unlimited, err)
	}
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, alice, actor, EndUserPatch{ResetQuota: true}); err != nil {
		t.Fatal(err)
	}
	clock = now.Add(2 * time.Hour)
	if err := check(alice); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("alice's expired plan still applied: %v", err)
	}
	if expired, err := service.GetEndUser(t.Context(), workspaceID, alice); err != nil || expired.QuotaSource != QuotaSourceDefault || expired.Plan == nil || expired.Plan.Active {
		t.Fatalf("alice after expiry = %+v %v", expired, err)
	}
	if pro, none := plans("pro"), plans("none"); pro != 1 || none != 1 {
		t.Fatalf("plan filter after expiry pro=%d none=%d", pro, none)
	}
	clock = now
	if cleared, err := service.UpdateEndUser(t.Context(), workspaceID, bob, actor, EndUserPatch{ClearPlan: true}); err != nil || cleared.Plan != nil {
		t.Fatalf("bob without a plan = %+v %v", cleared, err)
	}
	bobPage, _ := service.ListEndUserEvents(t.Context(), workspaceID, bob, 1, 50)
	bobEvents := bobPage.Items
	planActions := map[string]bool{}
	for _, event := range bobEvents {
		planActions[event.Action] = true
	}
	if !planActions[EndUserEventPlanCleared] || !planActions[EndUserEventPlanSet] {
		t.Fatalf("bob plan events = %+v", bobEvents)
	}

	// The integrating server can give a plan before the first request.
	carol, err := EndUserSubject(runtime.LogicalID(), "carol@example")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureEndUser(t.Context(), runtime.LogicalID(), carol, "carol@example"); err != nil {
		t.Fatal(err)
	}
	if view, err := service.UpdateEndUser(t.Context(), workspaceID, carol, actor, EndUserPatch{Plan: &EndUserPlanAssignment{Key: "pro"}}); err != nil || view.ExternalUserID != "carol@example" || view.QuotaSource != QuotaSourcePlan {
		t.Fatalf("carol before her first request = %+v %v", view, err)
	}
	if err := service.EnsureEndUser(t.Context(), runtime.LogicalID(), carol, "carol@example"); err != nil {
		t.Fatal(err)
	}
	if view, _ := service.GetEndUser(t.Context(), workspaceID, carol); view.Plan == nil || view.Plan.Key != "pro" {
		t.Fatalf("ensuring carol again changed her = %+v", view)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestParseEndUserPatch(t *testing.T) {
	patch, err := ParseEndUserPatch([]byte(`{"status": "suspended", "quota": null}`))
	if err != nil || patch.Status == nil || *patch.Status != EndUserSuspended || !patch.ResetQuota || patch.Quota != nil {
		t.Fatalf("patch = %+v %v", patch, err)
	}
	patch, err = ParseEndUserPatch([]byte(`{"quota": {"limits": [{"window": "7d", "points": 2.5}]}}`))
	if err != nil || patch.Status != nil || patch.Quota == nil || patch.Quota.Limits[0].MilliPoints() != 2500 {
		t.Fatalf("patch = %+v %v", patch, err)
	}
	if patch, err := ParseEndUserPatch([]byte(`{}`)); err != nil || patch.Status != nil || patch.Quota != nil || patch.ResetQuota {
		t.Fatalf("empty patch = %+v %v", patch, err)
	}
	if raw, err := json.Marshal(patch.Quota); err != nil || string(raw) != `{"limits":[{"window":"7d","points":2.5}]}` {
		t.Fatalf("quota JSON = %s %v", raw, err)
	}
	if _, err := ParseEndUserPatch([]byte(`[`)); !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("broken patch = %v", err)
	}
	patch, err = ParseEndUserPatch([]byte(`{"plan": {"key": "pro", "expires_at": "2026-12-01T00:00:00Z"}}`))
	if err != nil || patch.Plan == nil || patch.Plan.Key != "pro" || patch.Plan.ExpiresAt == nil || patch.ClearPlan {
		t.Fatalf("plan patch = %+v %v", patch, err)
	}
	if patch, err := ParseEndUserPatch([]byte(`{"plan": null}`)); err != nil || !patch.ClearPlan || patch.Plan != nil {
		t.Fatalf("plan removal = %+v %v", patch, err)
	}
	for _, body := range []string{`{"plan": {"key": "Pro"}}`, `{"plan": {"key": ""}}`, `{"plan": "pro"}`} {
		if _, err := ParseEndUserPatch([]byte(body)); !errors.Is(err, ErrInvalidWorkspace) {
			t.Fatalf("%s = %v", body, err)
		}
	}
}
