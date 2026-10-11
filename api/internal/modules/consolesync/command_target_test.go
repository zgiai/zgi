package consolesync

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

const otherOrganizationID = "22222222-2222-4222-8222-222222222222"

// newMultiOrgApplierFixture hosts two organizations, each with one workspace and one API
// key, so a command that resolves the wrong organization is visible.
func newMultiOrgApplierFixture(t *testing.T) (*Applier, *gorm.DB, *[]string) {
	t.Helper()
	applier, _, db := newApplierFixture(t)
	now := time.Now().UTC()
	statements := []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO organizations (id, name, status, created_at) VALUES (?, 'Other', 'active', ?)`, []interface{}{otherOrganizationID, now.Add(time.Second)}},
		{`CREATE TABLE workspaces (id VARCHAR(255) PRIMARY KEY, organization_id VARCHAR(255))`, nil},
		{`INSERT INTO workspaces (id, organization_id) VALUES ('ws-acme', ?), ('ws-other', ?)`, []interface{}{testOrganizationID, otherOrganizationID}},
		{`INSERT INTO llm_organization_api_keys (id, organization_id, status, quota_limit, used_quota, remain_quota, allow_ips, created_at, updated_at)
		  VALUES ('key-other', ?, 'active', 100, 10, 90, '', ?, ?)`, []interface{}{otherOrganizationID, now, now}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seedAPIKey(t, db, "key-acme", 100)

	var warnings []string
	original := commandTargetWarn
	commandTargetWarn = func(format string, args ...interface{}) {
		warnings = append(warnings, format)
	}
	t.Cleanup(func() { commandTargetWarn = original })
	return applier, db, &warnings
}

func orgStatus(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var status string
	if err := db.Table("organizations").Select("status").Where("id = ?", id).Scan(&status).Error; err != nil {
		t.Fatalf("read status: %v", err)
	}
	return status
}

func apply(t *testing.T, applier *Applier, commandType, payload string) error {
	t.Helper()
	_, err := applier.Apply(context.Background(), ptrEnvelope(envelope(commandType, payload)))
	return err
}

// I3: the payload names the organization, so a multi-organization database changes only
// that one.
func TestAccountStatusTargetsPayloadOrganization(t *testing.T) {
	applier, db, warnings := newMultiOrgApplierFixture(t)

	if err := apply(t, applier, CommandAccountStatusSet, `{"status":"inactive","organization_id":"`+otherOrganizationID+`"}`); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := orgStatus(t, db, otherOrganizationID); got != "inactive" {
		t.Fatalf("target status = %q, want inactive", got)
	}
	if got := orgStatus(t, db, testOrganizationID); got != "active" {
		t.Fatalf("other organization changed: %q", got)
	}
	if len(*warnings) != 0 {
		t.Fatalf("explicit target logged a legacy warning: %v", *warnings)
	}
}

// An unknown organization is a hard failure, never a guess.
func TestAccountStatusFailsForUnknownOrganization(t *testing.T) {
	applier, db, _ := newMultiOrgApplierFixture(t)

	err := apply(t, applier, CommandAccountStatusSet, `{"status":"inactive","organization_id":"33333333-3333-4333-8333-333333333333"}`)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v, want 'does not exist'", err)
	}
	for _, id := range []string{testOrganizationID, otherOrganizationID} {
		if got := orgStatus(t, db, id); got != "active" {
			t.Fatalf("organization %s changed to %q", id, got)
		}
	}
}

// Older Console without a target: the single-organization fallback still works, with a
// warning; a multi-organization database still refuses to guess.
func TestAccountStatusLegacyFallbackWarns(t *testing.T) {
	t.Run("single organization", func(t *testing.T) {
		applier, db := newApplierFixtureWithWarnings(t)
		if err := apply(t, applier.applier, CommandAccountStatusSet, `{"status":"inactive"}`); err != nil {
			t.Fatalf("single organization legacy: %v", err)
		}
		if got := orgStatus(t, db, testOrganizationID); got != "inactive" {
			t.Fatalf("status = %q, want inactive", got)
		}
		if len(*applier.warnings) != 1 {
			t.Fatalf("warnings = %v, want 1", *applier.warnings)
		}
	})
	t.Run("multiple organizations", func(t *testing.T) {
		multi, _, _ := newMultiOrgApplierFixture(t)
		if err := apply(t, multi, CommandAccountStatusSet, `{"status":"inactive"}`); err == nil {
			t.Fatalf("multi organization legacy command must fail")
		}
	})
}

type warnedApplier struct {
	applier  *Applier
	warnings *[]string
}

func newApplierFixtureWithWarnings(t *testing.T) (warnedApplier, *gorm.DB) {
	t.Helper()
	applier, _, db := newApplierFixture(t)
	var warnings []string
	original := commandTargetWarn
	commandTargetWarn = func(format string, args ...interface{}) { warnings = append(warnings, format) }
	t.Cleanup(func() { commandTargetWarn = original })
	return warnedApplier{applier: applier, warnings: &warnings}, db
}

func workspaceQuotaRows(t *testing.T, db *gorm.DB, workspaceID string) []struct {
	OrganizationID string
	QuotaLimit     *int64
} {
	t.Helper()
	var rows []struct {
		OrganizationID string
		QuotaLimit     *int64
	}
	if err := db.Table("llm_workspace_quotas").Select("organization_id, quota_limit").
		Where("workspace_id = ?", workspaceID).Scan(&rows).Error; err != nil {
		t.Fatalf("read quota: %v", err)
	}
	return rows
}

func TestWorkspaceQuotaTargetsPayloadOrganization(t *testing.T) {
	applier, db, _ := newMultiOrgApplierFixture(t)

	if err := apply(t, applier, CommandQuotaSet, `{"scope":"workspace","scope_id":"ws-other","limit":50,"organization_id":"`+otherOrganizationID+`"}`); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	rows := workspaceQuotaRows(t, db, "ws-other")
	if len(rows) != 1 || rows[0].OrganizationID != otherOrganizationID || rows[0].QuotaLimit == nil || *rows[0].QuotaLimit != 50 {
		t.Fatalf("quota rows = %+v, want one row of %s with limit 50", rows, otherOrganizationID)
	}
}

// A workspace of another organization (or an unknown one) is refused before any write.
func TestWorkspaceQuotaRejectsWorkspaceOutsideOrganization(t *testing.T) {
	applier, db, _ := newMultiOrgApplierFixture(t)

	for _, workspace := range []string{"ws-acme", "ws-missing"} {
		err := apply(t, applier, CommandQuotaSet, `{"scope":"workspace","scope_id":"`+workspace+`","limit":50,"organization_id":"`+otherOrganizationID+`"}`)
		if err == nil {
			t.Fatalf("%s: quota.set for a workspace outside the organization succeeded", workspace)
		}
		if rows := workspaceQuotaRows(t, db, workspace); len(rows) != 0 {
			t.Fatalf("%s: quota row written: %+v", workspace, rows)
		}
	}
}

func apiKeyStatus(t *testing.T, db *gorm.DB, id string) (string, int64) {
	t.Helper()
	var row struct {
		Status     string
		QuotaLimit int64
	}
	if err := db.Table("llm_organization_api_keys").Select("status, quota_limit").Where("id = ?", id).Scan(&row).Error; err != nil {
		t.Fatalf("read key: %v", err)
	}
	return row.Status, row.QuotaLimit
}

func TestAPIKeyCommandsStayInsidePayloadOrganization(t *testing.T) {
	applier, db, _ := newMultiOrgApplierFixture(t)

	// The key belongs to the other organization: both commands must fail and change nothing.
	if err := apply(t, applier, CommandAPIKeySet, `{"key_id":"key-other","status":"disabled","organization_id":"`+testOrganizationID+`"}`); err == nil {
		t.Fatalf("apikey.set crossed organizations")
	}
	if err := apply(t, applier, CommandAPIKeyQuotaSet, `{"key_id":"key-other","quota_limit":5,"organization_id":"`+testOrganizationID+`"}`); err == nil {
		t.Fatalf("apikey.quota.set crossed organizations")
	}
	if status, limit := apiKeyStatus(t, db, "key-other"); status != "active" || limit != 100 {
		t.Fatalf("foreign key changed: status=%s limit=%d", status, limit)
	}

	// Inside its own organization both work.
	if err := apply(t, applier, CommandAPIKeySet, `{"key_id":"key-other","status":"disabled","organization_id":"`+otherOrganizationID+`"}`); err != nil {
		t.Fatalf("apikey.set: %v", err)
	}
	if err := apply(t, applier, CommandAPIKeyQuotaSet, `{"key_id":"key-other","quota_limit":5,"organization_id":"`+otherOrganizationID+`"}`); err != nil {
		t.Fatalf("apikey.quota.set: %v", err)
	}
	if status, limit := apiKeyStatus(t, db, "key-other"); status != "disabled" || limit != 5 {
		t.Fatalf("key not updated: status=%s limit=%d", status, limit)
	}
}
