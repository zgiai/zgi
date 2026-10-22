package runtimeworkspace

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEndUserQuotaGuardSumsPointsOverRollingWindowsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Runtime Workspace quota PostgreSQL integration")
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
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Quota test')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Quota owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'Quota workspace', ?)", workspaceID, organizationID)
	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode) VALUES (?, ?, ?, 'active', 'attached')", workspaceID, organizationID, accountID)
	var runtime RuntimeWorkspace
	if err := tx.Where("workspace_id = ?", workspaceID).First(&runtime).Error; err != nil {
		t.Fatal(err)
	}
	appID, otherAppID := uuid.New(), uuid.New()
	exec("INSERT INTO runtime_workspace_agents (runtime_workspace_id, agent_id, app_type, public_app_key, status) VALUES (?, ?, 'digital_human', 'app_capped', 'active')", runtime.LogicalID(), appID)
	setPolicy := func(policy string) {
		t.Helper()
		exec("UPDATE runtime_workspaces SET policy_json = ? WHERE workspace_id = ?", policy, workspaceID)
	}
	subjectID := uuid.New()
	exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id, created_at, updated_at) VALUES (?, 'api_end_user', ?, ?, now(), now())", subjectID, SubjectIssuerType, runtime.LogicalID())
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	bill := func(points int64, app uuid.UUID, at time.Time) {
		t.Helper()
		exec(`INSERT INTO llm_usage_bills (id, attempt_id, request_id, organization_id, app_id, app_type, api_key_id, model_id, model_name, provider_id, provider_name, status, private_points, total_points, end_user_subject_id, request_created_at, settled_at)
			VALUES (?, ?, ?, ?, ?, 'digital_human', ?, ?, 'model', ?, 'provider', 'success', ?, ?, ?, ?, ?)`,
			uuid.New(), uuid.NewString(), uuid.NewString(), organizationID, app, uuid.New(), uuid.New(), uuid.New(), points*1000, points*1000, subjectID, at, at)
	}
	bill(60, appID, now.Add(-2*time.Hour))
	bill(30, otherAppID, now.Add(-3*time.Hour))
	bill(500, appID, now.Add(-48*time.Hour)) // outside the 24h window
	guard := NewQuotaGuard(tx)
	guard.now = func() time.Time { return now }
	check := func(app uuid.UUID) error {
		return guard.CheckEndUserQuota(t.Context(), chatruntime.EndUserQuotaRef{SubjectID: subjectID, AppID: app.String()})
	}
	exhausted := func(err error) bool { return apperror.IsCode(err, catalog.CodeQuotaExceeded) }

	if err := check(appID); err != nil {
		t.Fatalf("no quota rejected a call: %v", err)
	}
	setPolicy(`{"requests_per_minute": 60, "end_user_quota": {"limits": [{"window": "24h", "points": 100}]}}`)
	if err := check(appID); err != nil {
		t.Fatalf("90 of 100 points rejected a call: %v", err)
	}
	bill(10, otherAppID, now.Add(-time.Minute))
	if err := check(otherAppID); !exhausted(err) {
		t.Fatalf("100 of 100 points allowed a call: %v", err)
	}
	setPolicy(`{"end_user_quota": {"limits": [{"window": "24h", "points": 1000}], "apps": [{"app_key": "app_capped", "limits": [{"window": "24h", "points": 50}]}]}}`)
	if err := check(appID); !exhausted(err) {
		t.Fatalf("the capped app allowed 60 of 50 points: %v", err)
	}
	if err := check(otherAppID); err != nil {
		t.Fatalf("another app was held to the cap: %v", err)
	}
	setPolicy(`{"end_user_quota": {"limits": [{"window": "24h", "points": 100.5}]}}`)
	if err := check(appID); err != nil {
		t.Fatalf("100 of 100.5 points rejected a call: %v", err)
	}
	setPolicy(`{"end_user_quota": {"limits": [{"window": "7d", "points": 600}]}}`)
	if err := check(appID); !exhausted(err) {
		t.Fatalf("the 7d window missed older usage: %v", err)
	}
	stranger := uuid.New()
	if err := guard.CheckEndUserQuota(t.Context(), chatruntime.EndUserQuotaRef{SubjectID: stranger}); err != nil {
		t.Fatalf("an end user of no Runtime Workspace was held to a quota: %v", err)
	}
}

func TestEndUserQuotaPolicyValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		policy string
		valid  bool
	}{
		"empty":            {`{}`, true},
		"rate limit only":  {`{"requests_per_minute": 10}`, true},
		"limits":           {`{"end_user_quota": {"limits": [{"window": "24h", "points": 10}, {"window": "30d", "points": 100}]}}`, true},
		"app cap":          {`{"end_user_quota": {"apps": [{"app_key": "app_a", "limits": [{"window": "7d", "points": 5}]}]}}`, true},
		"unknown window":   {`{"end_user_quota": {"limits": [{"window": "1h", "points": 10}]}}`, false},
		"zero points":      {`{"end_user_quota": {"limits": [{"window": "24h", "points": 0}]}}`, false},
		"fractional":       {`{"end_user_quota": {"limits": [{"window": "24h", "points": 0.05}]}}`, true},
		"below a milli":    {`{"end_user_quota": {"limits": [{"window": "24h", "points": 0.0001}]}}`, false},
		"negative":         {`{"end_user_quota": {"limits": [{"window": "24h", "points": -1}]}}`, false},
		"repeated window":  {`{"end_user_quota": {"limits": [{"window": "24h", "points": 1}, {"window": "24h", "points": 2}]}}`, false},
		"app without key":  {`{"end_user_quota": {"apps": [{"limits": [{"window": "7d", "points": 5}]}]}}`, false},
		"repeated app":     {`{"end_user_quota": {"apps": [{"app_key": "a", "limits": [{"window": "7d", "points": 5}]}, {"app_key": "a", "limits": [{"window": "24h", "points": 5}]}]}}`, false},
		"app without caps": {`{"end_user_quota": {"apps": [{"app_key": "a", "limits": []}]}}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateEndUserQuota([]byte(tc.policy))
			if (err == nil) != tc.valid {
				t.Fatalf("validate %s = %v", tc.policy, err)
			}
			if err != nil && !errors.Is(err, ErrInvalidWorkspace) {
				t.Fatalf("invalid quota error = %v", err)
			}
		})
	}
	if quota := EndUserQuota(`{"end_user_quota": {"limits": [{"window": "24h", "points": 7}]}}`); len(quota.Limits) != 1 || quota.Limits[0].MilliPoints() != 7000 {
		t.Fatalf("parsed quota = %+v", quota)
	}
}
