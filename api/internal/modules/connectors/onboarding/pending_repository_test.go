package onboarding

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

func pendingRow(p PendingSetup) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connector_id", "region", "app_id", "source_flow_id", "template_id", "template_digest", "accepted_definition_digest", "state", "revision", "resume_expires_at", "created_at"}).AddRow(p.ID, p.OrganizationID, p.OwnerAccountID, p.ConnectorID, p.Region, p.AppID, p.SourceFlowID, p.TemplateID, p.TemplateDigest, p.AcceptedDefinitionDigest, p.State, p.Revision, p.ResumeExpiresAt, p.CreatedAt)
}
func leasedPendingFixture(t *testing.T) (*GormRepository, sqlmock.Sqlmock, Flow, PendingSetup) {
	r, m, f := flowRepositoryFixture(t)
	worker := uuid.New()
	until := r.now().Add(time.Minute)
	deadline := r.now().Add(25 * time.Second)
	f.LeaseOwner = &worker
	f.LeaseUntil = &until
	f.VerificationDeadline = &deadline
	f.State = Verifying
	f.Fencing = 2
	p := PendingSetup{ID: uuid.New(), OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectorID: f.ConnectorID, Region: f.Region, AppID: "cli_original", SourceFlowID: f.ID, TemplateID: f.TemplateID, TemplateDigest: f.TemplateDigest, AcceptedDefinitionDigest: f.AcceptedDefinitionDigest, State: VerificationRequired, Revision: 1, CreatedAt: r.now(), ResumeExpiresAt: r.now().Add(30 * 24 * time.Hour)}
	f.PendingSetupID = &p.ID
	return r, m, f, p
}
func TestObservedIdentityCommitsBeforeAnySecretVerification(t *testing.T) {
	r, m, f, _ := leasedPendingFixture(t)
	f.PendingSetupID = nil
	f.VerificationDeadline = nil
	f.State = Awaiting
	m.ExpectBegin()
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE`).WillReturnRows(flowRow(f))
	m.ExpectExec(`INSERT INTO "connector_pending_setups"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`DELETE FROM "connector_onboarding_secrets"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	r.commit = func(context.Context, *gorm.DB, Flow, map[string]string) (uuid.UUID, error) {
		t.Fatal("identity staging invoked candidate verification")
		return uuid.Nil, nil
	}
	got, err := r.ObserveApplication(context.Background(), f, "cli_original")
	if err != nil || got.PendingSetupID == nil || got.State != Verifying || got.VerificationDeadline == nil || got.VerificationDeadline.Sub(r.now()) != 30*time.Second {
		t.Fatalf("staging: %+v %v", got, err)
	}
}
func TestPendingCompletionAfterQRExpiryIsAtomicAndSecretIsDestroyed(t *testing.T) {
	r, m, f, p := leasedPendingFixture(t)
	f.ExpiresAt = r.now().Add(-time.Second)
	m.ExpectBegin()
	m.ExpectQuery(`SELECT .*connector_pending_setups.*FOR UPDATE`).WillReturnRows(pendingRow(p))
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE`).WillReturnRows(flowRow(f))
	m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`UPDATE "connector_pending_setups"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`DELETE FROM "connector_onboarding_secrets"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	id := uuid.New()
	r.commit = func(_ context.Context, tx *gorm.DB, flow Flow, values map[string]string) (uuid.UUID, error) {
		if tx.Statement.ConnPool == r.db.Statement.ConnPool || flow.PendingSetupID == nil || values["app_id"] != p.AppID {
			t.Fatal("not atomic or identity changed")
		}
		return id, nil
	}
	values := map[string]string{"region": p.Region, "app_id": p.AppID, "app_secret": "SECRET"}
	got, err := r.Complete(context.Background(), f, values)
	if err != nil || got != id || len(values) != 0 {
		t.Fatal("received credentials discarded with QR expiry", err)
	}
}
func TestPendingCandidateFailureRollsBackThenDefersWithoutCredentialPersistence(t *testing.T) {
	r, m, f, p := leasedPendingFixture(t)
	m.ExpectBegin()
	m.ExpectQuery(`SELECT .*connector_pending_setups.*FOR UPDATE`).WillReturnRows(pendingRow(p))
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE`).WillReturnRows(flowRow(f))
	m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectRollback()
	r.commit = func(context.Context, *gorm.DB, Flow, map[string]string) (uuid.UUID, error) {
		return uuid.Nil, registrationError(core.ErrorCodeApplicationNotReady)
	}
	if _, err := r.Complete(context.Background(), f, map[string]string{"region": p.Region, "app_id": p.AppID, "app_secret": "SECRET"}); !core.IsErrorCode(err, core.ErrorCodeApplicationNotReady) {
		t.Fatal(err)
	}
	m.ExpectBegin()
	m.ExpectQuery(`SELECT .*connector_pending_setups.*FOR UPDATE`).WillReturnRows(pendingRow(p))
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE`).WillReturnRows(flowRow(f))
	m.ExpectExec(`UPDATE "connector_pending_setups"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`DELETE FROM "connector_onboarding_secrets"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	if err := r.DeferApplication(context.Background(), f, core.ErrorCodeApplicationNotReady); err != nil {
		t.Fatal(err)
	}
}
func TestPendingCancellationExpiryAndIdentityFenceLateCompletion(t *testing.T) {
	for _, change := range []string{"cancelled", "expired", "identity", "deadline"} {
		t.Run(change, func(t *testing.T) {
			r, m, f, p := leasedPendingFixture(t)
			switch change {
			case "cancelled":
				p.State = PendingCancelled
			case "expired":
				p.ResumeExpiresAt = r.now().Add(-time.Second)
			case "identity":
				p.Region = "lark"
			case "deadline":
				past := r.now().Add(-time.Second)
				f.VerificationDeadline = &past
			}
			m.ExpectBegin()
			m.ExpectQuery(`SELECT .*connector_pending_setups.*FOR UPDATE`).WillReturnRows(pendingRow(p))
			if change == "deadline" {
				m.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE`).WillReturnRows(flowRow(f))
			}
			m.ExpectRollback()
			r.commit = func(context.Context, *gorm.DB, Flow, map[string]string) (uuid.UUID, error) {
				t.Fatal("late response created connection")
				return uuid.Nil, nil
			}
			if _, err := r.Complete(context.Background(), f, map[string]string{"region": p.Region, "app_id": p.AppID, "app_secret": "SECRET"}); !core.IsErrorCode(err, core.ErrorCodeConflict) {
				t.Fatal(err)
			}
		})
	}
}
func TestPendingSchemaCannotStoreCredentialsOrBindings(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(PendingSetup{}), reflect.TypeOf(PendingSnapshot{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, blocked := range []string{"secret", "credential", "device", "token", "binding", "default", "url", "qrcode"} {
				if strings.Contains(name, blocked) {
					t.Fatalf("forbidden pending field %s", name)
				}
			}
		}
	}
}

func TestPendingCapacityReservedBeforeStartingAnotherApplication(t *testing.T) {
	for _, counts := range [][2]int64{{20, 20}, {0, 200}} {
		r, m, f := flowRepositoryFixture(t)
		m.ExpectBegin()
		expectBudget(m)
		m.ExpectQuery(`SELECT .*connector_onboarding_flows.*idempotency_key_hash`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		expectCleanup(m)
		m.ExpectQuery(`SELECT\s+count\(\*\) FILTER.*created_at >.*FROM connector_onboarding_flows WHERE organization_id`).WillReturnRows(sqlmock.NewRows([]string{"owner_active", "org_active", "owner_recent", "org_recent", "same_intent"}).AddRow(0, 0, 0, 0, 0))
		m.ExpectQuery(`SELECT count\(\*\) FILTER.*FROM \(.*connector_pending_setups.*UNION ALL.*connector_onboarding_flows.*reserved`).WillReturnRows(sqlmock.NewRows([]string{"owner_pending", "org_pending"}).AddRow(counts[0], counts[1]))
		m.ExpectRollback()
		if _, err := r.Admit(context.Background(), f); !core.IsErrorCode(err, core.ErrorCodeRateLimited) {
			t.Fatalf("pending capacity admitted a new app: %v", err)
		}
	}
}

func TestCancelPendingLocksParentThenFencesActiveAttempt(t *testing.T) {
	for _, stale := range []bool{false, true} {
		r, m, f, p := leasedPendingFixture(t)
		m.ExpectBegin()
		m.ExpectQuery(`SELECT .*connector_pending_setups.*organization_id = .*owner_account_id = .*FOR UPDATE`).WithArgs(p.OrganizationID, p.OwnerAccountID, p.ID, 1).WillReturnRows(pendingRow(p))
		revision := p.Revision
		if stale {
			revision++
			m.ExpectRollback()
		} else {
			m.ExpectQuery(`SELECT .*connector_onboarding_flows.*pending_setup_id.*FOR UPDATE`).WillReturnRows(flowRow(f))
			m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`DELETE FROM "connector_onboarding_secrets"`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`UPDATE "connector_pending_setups"`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
		}
		got, err := r.CancelPending(context.Background(), p.OrganizationID, p.OwnerAccountID, p.ID, revision)
		if stale {
			if !core.IsErrorCode(err, core.ErrorCodeConflict) {
				t.Fatal(err)
			}
		} else if err != nil || got.State != PendingCancelled || got.Revision != p.Revision+1 || got.TerminalAt == nil {
			t.Fatal("cancel failed to fence the original app", err)
		}
	}
}

func TestPendingCleanupUsesItsOwnRetentionAndKeepsReferencedReceipts(t *testing.T) {
	r, m, _, p := leasedPendingFixture(t)
	p.ResumeExpiresAt = r.now().Add(-time.Second)
	m.ExpectQuery(`SELECT .*connector_pending_setups.*resume_expires_at <=.*FOR UPDATE SKIP LOCKED`).WithArgs(r.now(), 128).WillReturnRows(pendingRow(p))
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*pending_setup_id.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	m.ExpectExec(`UPDATE "connector_pending_setups"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`DELETE FROM "connector_pending_setups" WHERE terminal_at < .*NOT EXISTS .*connector_onboarding_flows`).WithArgs(r.now().Add(-7 * 24 * time.Hour)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.cleanupPending(r.db); err != nil {
		t.Fatal(err)
	}
}
