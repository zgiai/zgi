package gateway

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// RT-04 step 2: place the gateway in exactly the state its own failure path
// produces when the authority has already charged but the local mirror was never
// written, i.e. an attempt whose settlement_state is a non-terminal value the
// reconcile worker claims (PARTIAL / SETTLE_PENDING / FAILED_RETRYABLE) and which
// has no usage bill.
//
// The boundary is CONSTRUCTED here, not injected mid-request: the gateway's gRPC
// dialer exposes no fault-injection point, and modifying production code to add
// one is out of scope. Reports must say "constructed boundary" accordingly.

type rt4PartialSpec struct {
	AttemptID        string `json:"attempt_id"`
	RequestID        string `json:"request_id"`
	OrganizationID   string `json:"organization_id"`
	QuotaSubjectID   string `json:"quota_subject_id"`
	AuthorityAttempt string `json:"authority_attempt_id"`
	StateBefore      string `json:"state_before"`
	BillExistsBefore bool   `json:"bill_exists_before"`
}

func rt4OpenPre(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("PRE_PG_TEST_DSN")
	if dsn == "" {
		t.Skip("set PRE_PG_TEST_DSN to the isolated gateway database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gateway database: %v", err)
	}
	return db
}

// TestRT04PlaceUnmirroredAttempt creates the "authority charged, mirror missing"
// row and proves nothing has been mirrored for it yet.
func TestRT04PlaceUnmirroredAttempt(t *testing.T) {
	db := rt4OpenPre(t)
	statePath := os.Getenv("PRE_PG_TEST_STATE")
	if statePath == "" {
		t.Skip("set PRE_PG_TEST_STATE to record the RT-04 gateway state")
	}
	orgRaw := os.Getenv("PRE_PG_TEST_ORG")
	keyRaw := os.Getenv("PRE_PG_TEST_KEY")
	if orgRaw == "" || keyRaw == "" {
		t.Skip("set PRE_PG_TEST_ORG and PRE_PG_TEST_KEY to the gateway organisation and api key")
	}
	org := uuid.MustParse(orgRaw)
	keyID, err := uuid.Parse(keyRaw)
	if err != nil {
		t.Fatalf("parse api key id: %v", err)
	}

	// The organisation must really exist on this side, otherwise the attempt is
	// meaningless for the gateway.
	var orgRows int64
	db.Table("organizations").Where("id = ?", org).Count(&orgRows)
	if orgRows == 0 {
		t.Fatalf("organisation %s does not exist in the gateway database", org)
	}

	attempt := "rt4-mirror-a1"
	request := "rt4-mirror-r1"
	now := time.Now().UTC()

	// Nothing may be mirrored yet.
	var billsBefore int64
	db.Table("llm_usage_bills").Where("attempt_id = ?", attempt).Count(&billsBefore)
	if billsBefore != 0 {
		t.Fatalf("attempt %s already has %d usage bill(s); it must be unmirrored", attempt, billsBefore)
	}

	// Clear any leftover row from an earlier run so the run is repeatable.
	if err := db.Exec("DELETE FROM billing_attempts WHERE attempt_id = ?", attempt).Error; err != nil {
		t.Fatalf("clear previous attempt: %v", err)
	}
	row := &BillingAttempt{
		AttemptID:        attempt,
		RequestID:        request,
		OrganizationID:   org,
		Lane:             billingAttemptLaneRemote,
		InvocationSource: "api",
		QuotaSubjectType: "key",
		QuotaSubjectID:   keyRaw,
		// A key-subject settle needs the api key identity itself, otherwise the
		// mirror refuses with "missing api_key_id for key subject settle".
		APIKeyID:             &keyID,
		Status:               billingAttemptStatusSettlePending,
		AuthMethod:           "legacy_api_key",
		SettlementState:      billingAttemptStatusPartial,
		SettlementRetryCount: 0,
		SettlementVersion:    1,
		ReconcileAttempts:    0,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("place unmirrored attempt: %v", err)
	}

	// Re-read to prove the persisted state and that it is claimable, and that the
	// mirror is still absent.
	var stored BillingAttempt
	db.Where("attempt_id = ?", attempt).First(&stored)
	var billsAfter int64
	db.Table("llm_usage_bills").Where("attempt_id = ?", attempt).Count(&billsAfter)
	if stored.SettlementState != billingAttemptStatusPartial || stored.Lane != billingAttemptLaneRemote {
		t.Fatalf("unexpected persisted attempt: state=%s lane=%s", stored.SettlementState, stored.Lane)
	}
	if billsAfter != 0 {
		t.Fatalf("the attempt must still be unmirrored, got %d bill(s)", billsAfter)
	}

	spec := rt4PartialSpec{
		AttemptID: attempt, RequestID: request, OrganizationID: org.String(),
		QuotaSubjectID: keyRaw, AuthorityAttempt: attempt,
		StateBefore: stored.SettlementState, BillExistsBefore: false,
	}
	blob := fmt.Sprintf(`{
  "attempt_id": %q,
  "request_id": %q,
  "organization_id": %q,
  "quota_subject_id": %q,
  "authority_attempt_id": %q,
  "state_before": %q,
  "bill_exists_before": false
}
`, spec.AttemptID, spec.RequestID, spec.OrganizationID, spec.QuotaSubjectID,
		spec.AuthorityAttempt, spec.StateBefore)
	if err := os.WriteFile(statePath, []byte(blob), 0o600); err != nil {
		t.Fatalf("record state: %v", err)
	}
	fmt.Printf("RT4_GATEWAY_UNMIRRORED attempt=%s org=%s state=%s bills=%d claimable=true\n",
		attempt, org, stored.SettlementState, billsAfter)
}
