package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNullableSnapshot(t *testing.T) {
	for _, raw := range []string{"", " \t\n", `{"status":"active"}`, ` {"status":"inactive"} `} {
		got := nullableSnapshot(raw)
		if strings.TrimSpace(raw) == "" {
			if got != nil || derefString(got) != "" {
				t.Fatalf("absent snapshot %q = %v, want nil", raw, got)
			}
		} else if got == nil || derefString(got) != raw {
			t.Fatalf("snapshot %q was changed: %v", raw, got)
		}
	}
}

func TestApplyFailurePersistsRealErrorAndNULLSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name, payload, want string
		noOrganization      bool
	}{
		{"InvalidPayload", `{"status":"BOGUS-INVALID","organization_id":"` + testOrganizationID + `"}`, `account.status.set received unsupported status "BOGUS-INVALID"`, false},
		{"MissingStatus", `{}`, "account.status.set requires a status", false},
		{"UnresolvedOrganization", `{"status":"inactive"}`, "no organization found for account.status.set", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			applier, _, db := newApplierFixture(t)
			if tc.noOrganization {
				if err := db.Exec("DELETE FROM organizations").Error; err != nil {
					t.Fatal(err)
				}
			}
			var logs []string
			previous := commandTargetWarn
			commandTargetWarn = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
			t.Cleanup(func() { commandTargetWarn = previous })
			command := envelope(CommandAccountStatusSet, tc.payload)
			command.IdempotencyKey = "failed-key"
			result, err := applier.Apply(context.Background(), &command)
			if result != nil || err == nil || err.Error() != tc.want || strings.Contains(err.Error(), "invalid input syntax for type json") {
				t.Fatalf("Apply() = %v, %v; want nil, %q", result, err, tc.want)
			}
			var ledger PreAppliedCommand
			if err := db.Where("command_id = ?", command.ID).First(&ledger).Error; err != nil {
				t.Fatalf("failed ledger row missing: %v", err)
			}
			if ledger.Status != StatusFailed || ledger.Error != tc.want || ledger.IdempotencyKey != nil {
				t.Fatalf("failed ledger lost cause/status or owns key: %+v", ledger)
			}
			// Read SQL NULL directly: SQLite accepts empty strings in JSONB columns.
			var beforeNULL, afterNULL bool
			if err := db.Raw("SELECT before_state IS NULL, after_state IS NULL FROM pre_applied_commands WHERE command_id = ?", command.ID).Row().Scan(&beforeNULL, &afterNULL); err != nil {
				t.Fatal(err)
			}
			if !beforeNULL || !afterNULL || ledger.BeforeState != nil || ledger.AfterState != nil {
				t.Fatalf("failed snapshots must be SQL NULL: before=%v after=%v ledger=%+v", beforeNULL, afterNULL, ledger)
			}
			wantLog := fmt.Sprintf("[consolesync] command apply failed id=%s seq=%d type=%s err=%s", command.ID, command.Seq, command.Type, tc.want)
			if !strings.Contains(strings.Join(logs, "\n"), wantLog) {
				t.Fatalf("real cause absent from logs: %v", logs)
			}
			if !tc.noOrganization {
				var status string
				if err := db.Raw("SELECT status FROM organizations WHERE id = ?", testOrganizationID).Row().Scan(&status); err != nil || status != "active" {
					t.Fatalf("failed command changed organization: %q, %v", status, err)
				}
			}
		})
	}
}

func TestApplyFailureRollsBackBusinessChangesButKeepsLedger(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	// FAIL preserves preceding writes within the statement; the applier must undo them.
	if err := db.Exec(`CREATE TRIGGER fail_status AFTER UPDATE OF status ON organizations BEGIN
 UPDATE organizations SET name = 'partial write' WHERE id = NEW.id;
 SELECT RAISE(FAIL, 'forced status failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	command := envelope(CommandAccountStatusSet, `{"status":"inactive","organization_id":"`+testOrganizationID+`"}`)
	if _, err := applier.Apply(context.Background(), &command); err == nil || !strings.Contains(err.Error(), "forced status failure") {
		t.Fatalf("Apply() = %v, want forced status failure", err)
	}
	var name, status string
	if err := db.Raw("SELECT name, status FROM organizations WHERE id = ?", testOrganizationID).Row().Scan(&name, &status); err != nil {
		t.Fatal(err)
	}
	if name != "Acme" || status != "active" {
		t.Fatalf("partial business writes survived: name=%q status=%q", name, status)
	}
	var ledger PreAppliedCommand
	if err := db.Where("command_id = ?", command.ID).First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.Status != StatusFailed || !strings.Contains(ledger.Error, "forced status failure") || ledger.BeforeState != nil || ledger.AfterState != nil {
		t.Fatalf("failed ledger = %+v", ledger)
	}
}

func TestApplyReplaysHistoricalNULLSnapshots(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	stored := envelope(CommandAccountStatusSet, `{}`)
	if err := db.Exec(`INSERT INTO pre_applied_commands (command_id, seq, type, idempotency_key, status, before_state, after_state, applied_at)
 VALUES (?, 1, ?, 'historical-key', 'applied', NULL, NULL, CURRENT_TIMESTAMP)`, stored.ID, stored.Type).Error; err != nil {
		t.Fatal(err)
	}
	command := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	command.IdempotencyKey = "historical-key"
	result, err := applier.Apply(context.Background(), &command)
	if !errors.Is(err, ErrAlreadyApplied) || result == nil || result.Before != "" || result.After != "" {
		t.Fatalf("NULL replay = %+v, %v", result, err)
	}
}

func TestApplySuccessPersistsValidJSONSnapshots(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	command := envelope(CommandAccountStatusSet, `{"status":"inactive","organization_id":"`+testOrganizationID+`"}`)
	if _, err := applier.Apply(context.Background(), &command); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err := db.Raw("SELECT before_state, after_state FROM pre_applied_commands WHERE command_id = ?", command.ID).Row().Scan(&before, &after); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(before)) || !json.Valid([]byte(after)) || !strings.Contains(before, "active") || !strings.Contains(after, "inactive") {
		t.Fatalf("success snapshots lost: before=%q after=%q", before, after)
	}
}
