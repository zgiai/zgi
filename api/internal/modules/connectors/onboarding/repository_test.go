package onboarding

import (
	"context"
	"database/sql/driver"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"testing"
	"time"
)

func flowRepositoryFixture(t *testing.T) (*GormRepository, sqlmock.Sqlmock, Flow) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	r := NewGormRepository(db, nil)
	r.now = func() time.Time { return now }
	f := Flow{ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "feishu", Intent: "create_app", Region: "feishu", RequestedRegion: "feishu", TemplateID: "feishu-app-template-v1", TemplateDigest: strings.Repeat("a", 64), AcceptedDefinitionDigest: strings.Repeat("b", 64), IdempotencyKeyHash: strings.Repeat("c", 64), RequestFingerprint: strings.Repeat("d", 64), State: Starting, Generation: 1, Revision: 1, PollIntervalSeconds: 5, ExpiresAt: now.Add(10 * time.Minute), NextPollAt: now, CreatedAt: now, UpdatedAt: now}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return r, mock, f
}
func flowRow(f Flow) *sqlmock.Rows {
	var worker, until driver.Value
	if f.LeaseOwner != nil {
		worker = f.LeaseOwner.String()
	}
	if f.LeaseUntil != nil {
		until = *f.LeaseUntil
	}
	var pending, deadline, app driver.Value
	if f.PendingSetupID != nil {
		pending = f.PendingSetupID.String()
	}
	if f.VerificationDeadline != nil {
		deadline = *f.VerificationDeadline
	}
	if f.ExpectedAppID != nil {
		app = *f.ExpectedAppID
	}
	return sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connector_id", "intent", "region", "requested_region", "state", "generation", "revision", "fencing", "lease_owner", "lease_until", "expires_at", "idempotency_key_hash", "request_fingerprint", "begin_dispatched", "pending_setup_id", "verification_deadline", "expected_app_id", "template_id", "template_digest", "accepted_definition_digest"}).AddRow(f.ID, f.OrganizationID, f.OwnerAccountID, f.ConnectorID, f.Intent, f.Region, f.RequestedRegion, f.State, f.Generation, f.Revision, f.Fencing, worker, until, f.ExpiresAt, f.IdempotencyKeyHash, f.RequestFingerprint, f.BeginDispatched, pending, deadline, app, f.TemplateID, f.TemplateDigest, f.AcceptedDefinitionDigest)
}
func expectBudget(mock sqlmock.Sqlmock) {
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
}
func expectCleanup(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE SKIP LOCKED`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`DELETE FROM "connector_onboarding_secrets" WHERE expires_at`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM "connector_onboarding_flows" WHERE terminal_at`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT .*connector_pending_setups.*FOR UPDATE SKIP LOCKED`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`DELETE FROM "connector_pending_setups"`).WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestRepositoryAdmissionLimitsAreLockedAndDoNotDiscardCancelledHistory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		counts []driver.Value
	}{{"owner capacity", []driver.Value{3, 3, 0, 0, 0}}, {"org capacity", []driver.Value{0, 20, 0, 0, 0}}, {"cancelled owner rate", []driver.Value{0, 0, 5, 5, 0}}, {"cancelled org rate", []driver.Value{0, 0, 0, 20, 0}}, {"same create or update", []driver.Value{0, 0, 0, 0, 1}}} {
		t.Run(tc.name, func(t *testing.T) {
			r, m, f := flowRepositoryFixture(t)
			m.ExpectBegin()
			expectBudget(m)
			m.ExpectQuery(`SELECT .*connector_onboarding_flows.*organization_id = .*owner_account_id = .*idempotency_key_hash =`).WithArgs(f.OrganizationID, f.OwnerAccountID, f.IdempotencyKeyHash, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			expectCleanup(m)
			m.ExpectQuery(`SELECT\s+count\(\*\) FILTER.*created_at >.*FROM connector_onboarding_flows WHERE organization_id`).WillReturnRows(sqlmock.NewRows([]string{"owner_active", "org_active", "owner_recent", "org_recent", "same_intent"}).AddRow(tc.counts...))
			m.ExpectRollback()
			if _, err := r.Admit(context.Background(), f); !core.IsErrorCode(err, core.ErrorCodeRateLimited) {
				t.Fatalf("limit: %v", err)
			}
		})
	}
}
func TestRepositoryIdempotentAdmissionRecoversBeforeCapacityCheck(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "replay", true: "different intent"}[conflict], func(t *testing.T) {
			r, m, f := flowRepositoryFixture(t)
			prior := f
			if conflict {
				prior.RequestFingerprint = strings.Repeat("e", 64)
			}
			m.ExpectBegin()
			expectBudget(m)
			m.ExpectQuery(`SELECT .*connector_onboarding_flows.*idempotency_key_hash`).WillReturnRows(flowRow(prior))
			if conflict {
				m.ExpectRollback()
			} else {
				m.ExpectCommit()
			}
			got, err := r.Admit(context.Background(), f)
			if conflict {
				if !core.IsErrorCode(err, core.ErrorCodeConflict) {
					t.Fatal(err)
				}
			} else if err != nil || got.ID != f.ID {
				t.Fatalf("replay %v", err)
			}
		})
	}
}
func TestRepositoryDeploymentConcurrencyBudgetPrecedesLeaseSelection(t *testing.T) {
	r, m, _ := flowRepositoryFixture(t)
	m.ExpectBegin()
	expectBudget(m)
	expectCleanup(m)
	m.ExpectQuery(`SELECT count\(\*\) FROM "connector_onboarding_flows" WHERE lease_until`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(32))
	m.ExpectCommit()
	_, found, err := r.Claim(context.Background(), uuid.New())
	if err != nil || found {
		t.Fatalf("budget %v %v", found, err)
	}
}
func TestRepositoryCancelledOrStaleLeaseCannotCommitCredentials(t *testing.T) {
	for _, change := range []string{"cancelled", "fence", "owner", "generation", "expired"} {
		t.Run(change, func(t *testing.T) {
			r, m, f := flowRepositoryFixture(t)
			w := uuid.New()
			until := r.now().Add(time.Minute)
			f.LeaseOwner = &w
			f.LeaseUntil = &until
			f.State = Awaiting
			f.Fencing = 4
			stored := f
			switch change {
			case "cancelled":
				stored.State = Cancelled
			case "fence":
				stored.Fencing++
			case "owner":
				other := uuid.New()
				stored.LeaseOwner = &other
			case "generation":
				stored.Generation++
			case "expired":
				past := r.now().Add(-time.Second)
				stored.LeaseUntil = &past
			}
			r.commit = func(context.Context, *gorm.DB, Flow, map[string]string) (uuid.UUID, error) {
				t.Fatal("stale lease committed")
				return uuid.Nil, nil
			}
			m.ExpectBegin()
			m.ExpectQuery(`SELECT .*connector_onboarding_flows.*organization_id = .*owner_account_id = .*id = .*FOR UPDATE`).WithArgs(f.OrganizationID, f.OwnerAccountID, f.ID, 1).WillReturnRows(flowRow(stored))
			m.ExpectRollback()
			values := map[string]string{"app_secret": "fixture"}
			_, err := r.Complete(context.Background(), f, values)
			if !core.IsErrorCode(err, core.ErrorCodeConflict) || len(values) != 0 {
				t.Fatalf("fence/wipe %v", err)
			}
		})
	}
}
func TestRepositoryCandidateFailureRollsBackVerificationTransaction(t *testing.T) {
	r, m, f := flowRepositoryFixture(t)
	w := uuid.New()
	until := r.now().Add(time.Minute)
	f.LeaseOwner = &w
	f.LeaseUntil = &until
	f.State = Awaiting
	m.ExpectBegin()
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*FOR UPDATE`).WillReturnRows(flowRow(f))
	m.ExpectExec(`UPDATE "connector_onboarding_flows"`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectRollback()
	r.commit = func(_ context.Context, tx *gorm.DB, _ Flow, _ map[string]string) (uuid.UUID, error) {
		if tx.Statement.ConnPool == r.db.Statement.ConnPool {
			t.Fatal("candidate is outside transaction")
		}
		return uuid.Nil, registrationError(core.ErrorCodeInvalidAppCredentials)
	}
	if _, err := r.Complete(context.Background(), f, map[string]string{"app_secret": "fixture"}); !core.IsErrorCode(err, core.ErrorCodeInvalidAppCredentials) {
		t.Fatal(err)
	}
}
func TestRepositoryGetAndRequestRecoveryAreOwnerScoped(t *testing.T) {
	r, m, f := flowRepositoryFixture(t)
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*organization_id = .*owner_account_id = .*id =`).WithArgs(f.OrganizationID, f.OwnerAccountID, f.ID, 1).WillReturnRows(flowRow(f))
	if got, err := r.Get(context.Background(), f.OrganizationID, f.OwnerAccountID, f.ID); err != nil || got.ID != f.ID {
		t.Fatal(err)
	}
	m.ExpectQuery(`SELECT .*connector_onboarding_flows.*organization_id = .*owner_account_id = .*idempotency_key_hash =`).WithArgs(f.OrganizationID, sqlmock.AnyArg(), f.IdempotencyKeyHash, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, found, err := r.FindRequest(context.Background(), f.OrganizationID, uuid.New(), f.IdempotencyKeyHash); err != nil || found {
		t.Fatal("foreign receipt exposed")
	}
}
