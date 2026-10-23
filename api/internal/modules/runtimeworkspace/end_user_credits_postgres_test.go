package runtimeworkspace

import (
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

func TestEndUserCreditsPayBeyondLimitsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Runtime Workspace end-user credits PostgreSQL integration")
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
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Credits test')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Credits owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'Credits workspace', ?)", workspaceID, organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode, policy_json) VALUES (?, ?, ?, 'active', 'attached', ?)",
		workspaceID, organizationID, accountID, `{"end_user_quota": {"limits": [{"window": "24h", "points": 10}], "plans": [{"key": "payg", "limits": [{"window": "24h", "points": 0}]}]}}`)
	var runtime RuntimeWorkspace
	if err := tx.Where("workspace_id = ?", workspaceID).First(&runtime).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	clock := now
	service := NewService(tx)
	service.now = func() time.Time { return clock }
	guard := NewQuotaGuard(tx)
	guard.now = func() time.Time { return clock }
	actor := EndUserActor{Kind: EndUserActorCredential, ID: uuid.New()}
	register := func(user string) uuid.UUID {
		t.Helper()
		subjectID, err := EndUserSubject(runtime.LogicalID(), user)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.EnsureEndUser(t.Context(), runtime.LogicalID(), subjectID, user); err != nil {
			t.Fatal(err)
		}
		return subjectID
	}
	bill := func(subject uuid.UUID, points int64, at time.Time) {
		t.Helper()
		exec(`INSERT INTO llm_usage_bills (id, attempt_id, request_id, organization_id, api_key_id, model_id, model_name, provider_id, provider_name, status, private_points, total_points, end_user_subject_id, request_created_at, settled_at)
			VALUES (?, ?, ?, ?, ?, ?, 'model', ?, 'provider', 'success', ?, ?, ?, ?, ?)`,
			uuid.New(), uuid.NewString(), uuid.NewString(), organizationID, uuid.New(), uuid.New(), uuid.New(), points, points, subject, at, at)
	}
	points := func(value string) decimal.Decimal { return decimal.RequireFromString(value) }
	grant := func(subject uuid.UUID, amount, externalID string) *CreditGrantResult {
		t.Helper()
		result, err := service.GrantCredits(t.Context(), workspaceID, subject, actor, CreditGrant{Points: points(amount), ExternalID: externalID})
		if err != nil {
			t.Fatalf("grant %s as %s: %v", amount, externalID, err)
		}
		return result
	}
	balance := func(subject uuid.UUID) float64 {
		t.Helper()
		ledger, err := service.ListCredits(t.Context(), workspaceID, subject, 1, 50)
		if err != nil {
			t.Fatal(err)
		}
		return ledger.Credits.Balance
	}
	check := func(subject uuid.UUID) error {
		return guard.CheckEndUserQuota(t.Context(), chatruntime.EndUserQuotaRef{SubjectID: subject})
	}

	alice := register("alice@example")
	bill(alice, 8_000, now.Add(-2*time.Hour)) // before credits: never charged
	if first := grant(alice, "5", "order-1"); first.Duplicate || first.Credits.Balance != 5 {
		t.Fatalf("first grant = %+v", first)
	}
	if again := grant(alice, "5", "order-1"); !again.Duplicate || again.Credits.Balance != 5 {
		t.Fatalf("a repeated grant = %+v", again)
	}
	if _, err := service.GrantCredits(t.Context(), workspaceID, alice, actor, CreditGrant{Points: points("6"), ExternalID: "order-1"}); !errors.Is(err, ErrCreditConflict) {
		t.Fatalf("an order reused for another amount = %v", err)
	}
	past := now.Add(-time.Minute)
	for name, invalid := range map[string]CreditGrant{
		"zero points":          {Points: points("0"), ExternalID: "x"},
		"no external id":       {Points: points("1")},
		"a negative with date": {Points: points("-1"), ExternalID: "y", ExpiresAt: ptrTime(now.Add(time.Hour))},
		"a past expiry":        {Points: points("1"), ExternalID: "z", ExpiresAt: &past},
	} {
		if _, err := service.GrantCredits(t.Context(), workspaceID, alice, actor, invalid); !errors.Is(err, ErrInvalidWorkspace) {
			t.Fatalf("%s = %v", name, err)
		}
	}

	// 8 of 10 points spent; a 4-point call goes 2 beyond the limit.
	bill(alice, 4_000, now.Add(time.Minute))
	clock = now.Add(2 * time.Minute)
	if got := balance(alice); got != 3 {
		t.Fatalf("balance after 2 points beyond the limit = %v", got)
	}
	if got := balance(alice); got != 3 {
		t.Fatalf("settling again changed the balance to %v", got)
	}
	if err := check(alice); err != nil {
		t.Fatalf("alice over her limit with credits left was refused: %v", err)
	}
	bill(alice, 5_000, now.Add(3*time.Minute))
	clock = now.Add(4 * time.Minute)
	if err := check(alice); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("alice with debt was admitted: %v", err)
	}
	if got := balance(alice); got != -2 {
		t.Fatalf("balance with debt = %v", got)
	}
	if paid := grant(alice, "1", "order-2"); paid.Credits.Balance != -1 || paid.Credits.Debt != 1 {
		t.Fatalf("a grant short of the debt = %+v", paid)
	}
	if topped := grant(alice, "3", "order-3"); topped.Credits.Balance != 2 || topped.Credits.Debt != 0 {
		t.Fatalf("a grant beyond the debt = %+v", topped)
	}
	if taken := grant(alice, "-5", "refund-3"); taken.Credits.Balance != -3 {
		t.Fatalf("a refund beyond the balance = %+v", taken)
	}
	expiry := clock.Add(time.Hour)
	expiring, err := service.GrantCredits(t.Context(), workspaceID, alice, actor, CreditGrant{Points: points("4"), ExternalID: "promo", ExpiresAt: &expiry})
	if err != nil || expiring.Credits.Balance != 1 {
		t.Fatalf("an expiring grant = %+v %v", expiring, err)
	}
	entitlement, err := service.EndUserEntitlement(t.Context(), workspaceID, alice)
	if err != nil || entitlement.Credits == nil || len(entitlement.Credits.Lots) != 1 || entitlement.Credits.Lots[0].ExpiresAt == nil ||
		!entitlement.CanRun || entitlement.Windows[0].Limit == nil || *entitlement.Windows[0].Limit != 10 || entitlement.Windows[0].NextReleaseAt == nil {
		t.Fatalf("entitlement = %+v %v", entitlement, err)
	}
	clock = expiry.Add(time.Minute)
	if got := balance(alice); got != 0 {
		t.Fatalf("balance after the promotion expired = %v", got)
	}
	ledger, err := service.ListCredits(t.Context(), workspaceID, alice, 1, 50)
	if err != nil || ledger.Total != 7 {
		t.Fatalf("ledger = %+v %v", ledger, err)
	}
	if view, err := service.GetEndUser(t.Context(), workspaceID, alice); err != nil || view.Credits == nil || view.Credits.Balance != 0 {
		t.Fatalf("alice's view = %+v %v", view, err)
	}

	// No limit: credits are never drawn.
	bob := register("bob@example")
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, bob, actor, EndUserPatch{Quota: &EndUserQuotaOverride{Unlimited: true}}); err != nil {
		t.Fatal(err)
	}
	grant(bob, "2", "bob-1")
	bill(bob, 50_000, clock.Add(time.Second))
	clock = clock.Add(time.Minute)
	if got := balance(bob); got != 2 {
		t.Fatalf("unlimited bob spent credits: %v", got)
	}

	// A zero limit includes nothing: every point is paid with credits.
	carol := register("carol@example")
	if _, err := service.UpdateEndUser(t.Context(), workspaceID, carol, actor, EndUserPatch{Plan: &EndUserPlanAssignment{Key: "payg"}}); err != nil {
		t.Fatal(err)
	}
	if err := check(carol); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("pay-as-you-go carol without credits was admitted: %v", err)
	}
	grant(carol, "1", "carol-1")
	if err := check(carol); err != nil {
		t.Fatalf("pay-as-you-go carol with credits was refused: %v", err)
	}
	bill(carol, 400, clock.Add(time.Second))
	clock = clock.Add(time.Minute)
	if got := balance(carol); got != 0.6 {
		t.Fatalf("carol after 0.4 points = %v", got)
	}

	if err := service.EraseEndUser(t.Context(), workspaceID, alice, actor); err != nil {
		t.Fatal(err)
	}
	var left int64
	if err := tx.Raw(`SELECT (SELECT count(*) FROM runtime_workspace_end_user_credit_lots WHERE subject_id = ?)
		+ (SELECT count(*) FROM runtime_workspace_end_user_credit_entries WHERE subject_id = ?)
		+ (SELECT count(*) FROM runtime_workspace_end_user_credit_bills WHERE subject_id = ?)`, alice, alice, alice).Scan(&left).Error; err != nil || left != 0 {
		t.Fatalf("erased alice keeps %d credit rows: %v", left, err)
	}
	for status, want := range map[string]int64{"": 2, "erased": 1, "all": 3} {
		if list, err := service.ListEndUsers(t.Context(), workspaceID, EndUserListQuery{Status: status}); err != nil || list.Total != want {
			t.Fatalf("end users with status %q = %+v %v", status, list, err)
		}
	}
}
