package migrations

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestMessagingReliabilityForwardMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public.connector_onboarding_flows.*candidate_expires_at.*verification_waiting.*CREATE TABLE public.connector_messaging_read_leases`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upMessagingReliability(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Runs only inside TestMessagingProtocolPostgresIsolation's guarded fresh DB.
func testMessagingReliabilityPostgres(t *testing.T, db *gorm.DB) {
	ctx := context.Background()
	cipher, _ := credentials.NewStore("test", map[string]string{"test": strings.Repeat("x", 32)})
	store := messaging.NewStore(db, cipher)
	commits := 0
	repo := onboarding.NewGormRepository(db, nil).WithMessagingCommit(func(ctx context.Context, tx *gorm.DB, f onboarding.Flow, _ map[string]string, _ connections.CandidateVerificationResult) (uuid.UUID, error) {
		commits++
		id := *f.CandidateConnectionID
		err := tx.Exec("INSERT INTO connector_connections VALUES (?,?,?,?)", id, f.OrganizationID, f.OwnerAccountID, f.ConnectorID).Error
		return id, err
	})
	fresh := func(t *testing.T) onboarding.Flow {
		t.Helper()
		if err := db.Model(&onboarding.Flow{}).Where("state IN ?", []onboarding.State{onboarding.Starting, onboarding.Awaiting, onboarding.Verifying}).Update("next_poll_at", time.Now().Add(time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
		f := flowFixture("weixin")
		f.State = onboarding.Awaiting
		f.Step = "verify"
		accepted := time.Now().UTC()
		expiry := accepted.Add(15 * time.Minute)
		id := uuid.New()
		f.CandidateAcceptedAt = &accepted
		f.CandidateExpiresAt = &expiry
		f.CandidateConnectionID = &id
		// A QR can already be expired while its accepted candidate remains valid.
		f.ExpiresAt = accepted.Add(-time.Second)
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&onboarding.DeviceRecord{FlowID: f.ID, OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, Generation: f.Generation, EncryptedDevice: "c1.fixture", ExpiresAt: expiry}).Error; err != nil {
			t.Fatal(err)
		}
		got, found, err := repo.Claim(ctx, uuid.New())
		if err != nil || !found || got.ID != f.ID {
			t.Fatal("claim candidate", err)
		}
		return got
	}
	t.Run("candidate retry CAS and fixed lifetime", func(t *testing.T) {
		f := fresh(t)
		deadline := *f.CandidateExpiresAt
		if err := repo.WaitMessaging(ctx, f, core.ErrorCodeProviderUnavailable, "operation", uuid.NewString()); err != nil {
			t.Fatal(err)
		}
		waiting, _ := repo.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID)
		if waiting.State != onboarding.VerificationWaiting || !waiting.CandidateExpiresAt.Equal(deadline) {
			t.Fatal("lost candidate deadline")
		}
		if _, err := repo.Device(ctx, waiting); err != nil {
			t.Fatal("QR expiry incorrectly destroyed candidate", err)
		}
		if _, err := repo.RetryMessaging(ctx, waiting, waiting.Revision-1, strings.Repeat("a", 64)); core.ErrorCodeOf(err) != core.ErrorCodeConflict {
			t.Fatal("stale retry accepted")
		}
		retried, err := repo.RetryMessaging(ctx, waiting, waiting.Revision, strings.Repeat("b", 64))
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := repo.RetryMessaging(ctx, waiting, waiting.Revision, strings.Repeat("b", 64))
		if err != nil || repeated.Revision != retried.Revision {
			t.Fatal("retry idempotency")
		}
		if _, err = repo.Cancel(ctx, f.OrganizationID, f.OwnerAccountID, f.ID, retried.Revision); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cancel during consuming probe holds no flow transaction", func(t *testing.T) {
		f := fresh(t)
		scope := providers.ResourceScope{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectionID: *f.CandidateConnectionID}
		err := store.Update(ctx, scope, "weixin", strings.Repeat("c", 64), f.ID, func(callback context.Context, state *messaging.State) error {
			bounded, cancel := context.WithTimeout(callback, time.Second)
			defer cancel()
			if _, err := repo.Cancel(bounded, f.OrganizationID, f.OwnerAccountID, f.ID, f.Revision); err != nil {
				return err
			}
			state.Cursor = "private-cursor"
			state.Messages = []messaging.Message{{ID: "first", Text: "private-message"}}
			return nil
		})
		if err == nil {
			t.Fatal("cancelled probe checkpointed")
		}
		var n int64
		db.Table("connector_messaging_states").Where("flow_id = ?", f.ID).Count(&n)
		if n != 0 {
			t.Fatal("cancelled candidate retained")
		}
	})
	t.Run("checkpoint promotion preserves first message and cursor", func(t *testing.T) {
		f := fresh(t)
		scope := providers.ResourceScope{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectionID: *f.CandidateConnectionID}
		digest := strings.Repeat("e", 64)
		if err := store.Update(ctx, scope, "weixin", digest, f.ID, func(_ context.Context, state *messaging.State) error {
			state.Cursor = "first-cursor"
			state.Verified = true
			state.Messages = []messaging.Message{{ID: "first", Text: "private-first-message"}}
			state.Seen = []string{"first"}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		device := onboarding.DeviceRecord{FlowID: f.ID, OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, Generation: f.Generation, EncryptedDevice: "c1.checkpoint", ExpiresAt: *f.CandidateExpiresAt}
		if err := repo.CheckpointMessaging(ctx, f, device); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CompleteMessaging(ctx, f, nil, connections.CandidateVerificationResult{}); core.ErrorCodeOf(err) != core.ErrorCodeConflict {
			t.Fatal("old producer fence promoted")
		}
		db.Model(&onboarding.Flow{}).Where("id = ?", f.ID).Update("next_poll_at", time.Now().Add(-time.Second))
		commit, found, err := repo.Claim(ctx, uuid.New())
		if err != nil || !found || commit.Step != "commit" {
			t.Fatal("claim checkpoint", err)
		}
		id, err := repo.CompleteMessaging(ctx, commit, nil, connections.CandidateVerificationResult{})
		if err != nil || id != scope.ConnectionID || commits != 1 {
			t.Fatal("local promotion", err)
		}
		ready, _ := repo.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID)
		if ready.State != onboarding.Ready {
			t.Fatal("not ready")
		}
		if err = store.Update(ctx, scope, "weixin", digest, uuid.Nil, func(_ context.Context, state *messaging.State) error {
			if state.Cursor != "first-cursor" || len(state.Messages) != 1 || state.Messages[0].ID != "first" || len(state.Seen) != 1 {
				t.Fatal("first batch lost")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("uncertain consuming read never replays", func(t *testing.T) {
		f := fresh(t)
		scope := providers.ResourceScope{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectionID: *f.CandidateConnectionID}
		calls := 0
		callback := func(context.Context, *messaging.State) error {
			calls++
			return &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: true}
		}
		_ = store.Update(ctx, scope, "weixin", strings.Repeat("f", 64), f.ID, callback)
		if err := store.Update(ctx, scope, "weixin", strings.Repeat("f", 64), f.ID, callback); err == nil || calls != 1 {
			t.Fatal("uncertain pull replayed")
		}
	})
}
