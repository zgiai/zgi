package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMessagingProtocolForwardMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)ALTER TABLE public.connector_onboarding_flows.*CREATE TABLE public.connector_messaging_states.*CREATE TABLE public.connector_messaging_contexts`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upMessagingConnectorProtocol(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestMessagingProtocolPostgresIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_CONNECTOR_MESSAGING_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires a fresh isolated connector007_* PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated database")
	}
	var name string
	if db.Raw("SELECT current_database()").Scan(&name).Error != nil || !strings.HasPrefix(name, "connector007_") {
		t.Fatal("refusing non-test database")
	}
	var count int64
	db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&count)
	if count != 0 {
		t.Fatal("requires fresh empty test database")
	}
	if err = db.Exec(`CREATE TABLE public.connector_connections(id uuid PRIMARY KEY,organization_id uuid NOT NULL,owner_account_id uuid NOT NULL,connector_id varchar(128) NOT NULL,UNIQUE(id,organization_id,owner_account_id,connector_id))`).Error; err != nil {
		t.Fatal(err)
	}
	if err = upCreateConnectorOnboarding(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	legacy := flowFixture("feishu")
	legacy.ProtocolVersion = 0
	legacy.Handler = ""
	legacy.Step = ""
	legacy.Intent = "create_app"
	legacy.Region = "feishu"
	legacy.RequestedRegion = "feishu"
	legacy.TemplateID = "legacy"
	legacy.TemplateDigest = strings.Repeat("a", 64)
	legacy.PollIntervalSeconds = 5
	// Insert through the previous schema before the new columns exist.
	if err = db.Exec(`INSERT INTO connector_onboarding_flows(id,organization_id,owner_account_id,connector_id,intent,region,requested_region,template_id,template_digest,accepted_definition_digest,idempotency_key_hash,request_fingerprint,state,poll_interval_seconds,expires_at,next_poll_at,created_at,updated_at) VALUES (?,?,?,'feishu','create_app','feishu','feishu','legacy',?,?,?,?,'starting',5,?,?,?,?)`, legacy.ID, legacy.OrganizationID, legacy.OwnerAccountID, legacy.TemplateDigest, legacy.AcceptedDefinitionDigest, legacy.IdempotencyKeyHash, legacy.RequestFingerprint, legacy.ExpiresAt, legacy.NextPollAt, legacy.CreatedAt, legacy.UpdatedAt).Error; err != nil {
		t.Fatal(err)
	}
	if err = upMessagingConnectorProtocol(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err = upConnectorPendingSetups(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err = upMessagingReliability(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	var kept onboarding.Flow
	if db.First(&kept, "id = ?", legacy.ID).Error != nil || kept.ProtocolVersion != 0 || kept.TemplateDigest != legacy.TemplateDigest {
		t.Fatal("legacy row changed")
	}
	db.Model(&onboarding.Flow{}).Where("id = ?", legacy.ID).Update("next_poll_at", time.Now().Add(time.Hour))
	ctx := context.Background()
	var commits atomic.Int32
	repo := onboarding.NewGormRepository(db, func(context.Context, *gorm.DB, onboarding.Flow, map[string]string) (uuid.UUID, error) {
		commits.Add(1)
		return uuid.New(), nil
	})
	t.Run("all intents share quotas", func(t *testing.T) {
		org, owner := uuid.New(), uuid.New()
		var success atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				f := flowFixture([]string{"dingtalk", "wecom", "weixin"}[i%3])
				f.OrganizationID = org
				f.OwnerAccountID = owner
				if _, err := repo.Admit(ctx, f); err == nil {
					success.Add(1)
				} else if core.ErrorCodeOf(err) != core.ErrorCodeRateLimited {
					t.Errorf("admission: %v", err)
				}
			}(i)
		}
		wg.Wait()
		if success.Load() != 3 {
			t.Fatalf("active quota = %d", success.Load())
		}
	})
	t.Run("cancel does not refund begin budget", func(t *testing.T) {
		org, owner := uuid.New(), uuid.New()
		for i := 0; i < 6; i++ {
			f := flowFixture("dingtalk")
			f.OrganizationID = org
			f.OwnerAccountID = owner
			got, err := repo.Admit(ctx, f)
			if i == 5 {
				if core.ErrorCodeOf(err) != core.ErrorCodeRateLimited {
					t.Fatal("budget refunded")
				}
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = repo.Cancel(ctx, org, owner, got.ID, got.Revision); err != nil {
				t.Fatal(err)
			}
		}
	})
	lease := func(t *testing.T, provider string) onboarding.Flow {
		t.Helper()
		db.Model(&onboarding.Flow{}).Where("state IN ?", []onboarding.State{onboarding.Starting, onboarding.Awaiting}).Update("next_poll_at", time.Now().Add(time.Hour))
		f := flowFixture(provider)
		if _, err := repo.Admit(ctx, f); err != nil {
			t.Fatal(err)
		}
		got, found, err := repo.Claim(ctx, uuid.New())
		if err != nil || !found || got.ID != f.ID {
			t.Fatal("claim", err)
		}
		return got
	}
	t.Run("global lease budget covers every protocol", func(t *testing.T) {
		ids := []uuid.UUID{}
		for i := 0; i < 32; i++ {
			f := flowFixture([]string{"dingtalk", "wecom", "weixin"}[i%3])
			worker, until := uuid.New(), time.Now().Add(time.Minute)
			f.LeaseOwner, f.LeaseUntil, f.StepDispatched = &worker, &until, true
			if err := db.Create(&f).Error; err != nil {
				t.Fatal(err)
			}
			ids = append(ids, f.ID)
		}
		if _, found, err := repo.Claim(ctx, uuid.New()); err != nil || found {
			t.Fatal("global budget exceeded", err)
		}
		if err := db.Where("id IN ?", ids).Delete(&onboarding.Flow{}).Error; err != nil {
			t.Fatal(err)
		}
	})
	device := func(f onboarding.Flow) onboarding.DeviceRecord {
		return onboarding.DeviceRecord{FlowID: f.ID, OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, Generation: f.Generation, EncryptedDevice: "c1.encrypted-fixture", ExpiresAt: f.ExpiresAt}
	}
	t.Run("durable init and stale completion fence", func(t *testing.T) {
		f := lease(t, "dingtalk")
		if !f.StepDispatched {
			t.Fatal("no dispatch marker")
		}
		if err := repo.SaveProtocolStep(ctx, f, device(f), onboarding.ProtocolResult{Step: "begin", Interval: time.Second}); err != nil {
			t.Fatal(err)
		}
		current, _ := repo.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID)
		if current.Step != "begin" || current.StepDispatched || current.LeaseOwner != nil {
			t.Fatal("step not durable")
		}
		if _, err := repo.Cancel(ctx, f.OrganizationID, f.OwnerAccountID, f.ID, current.Revision); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Complete(ctx, f, map[string]string{"secret": "late"}); core.ErrorCodeOf(err) != core.ErrorCodeConflict {
			t.Fatal("late completion")
		}
		if commits.Load() != 0 {
			t.Fatal("cancelled flow committed")
		}
		var n int64
		db.Model(&onboarding.DeviceRecord{}).Where("flow_id = ?", f.ID).Count(&n)
		if n != 0 {
			t.Fatal("secret retained")
		}
	})
	t.Run("challenge CAS and lease recovery", func(t *testing.T) {
		f := lease(t, "weixin")
		if err := repo.SaveProtocolStep(ctx, f, device(f), onboarding.ProtocolResult{Step: "poll", Interval: time.Second}); err != nil {
			t.Fatal(err)
		}
		db.Model(&onboarding.Flow{}).Where("id = ?", f.ID).Update("next_poll_at", time.Now().Add(-time.Second))
		f, _, err = repo.Claim(ctx, uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.SaveProtocolStep(ctx, f, device(f), onboarding.ProtocolResult{Step: "awaiting_code", Interval: time.Second}); err != nil {
			t.Fatal(err)
		}
		f, _ = repo.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID)
		d := device(f)
		next, err := repo.SubmitChallenge(ctx, f, d, f.Revision, f.ChallengeRevision)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = repo.SubmitChallenge(ctx, f, d, f.Revision, f.ChallengeRevision); core.ErrorCodeOf(err) != core.ErrorCodeConflict {
			t.Fatal("challenge replay")
		}
		f, _, err = repo.Claim(ctx, uuid.New())
		if err != nil || f.ID != next.ID {
			t.Fatal("challenge claim")
		}
		db.Model(&onboarding.Flow{}).Where("id = ?", f.ID).Update("lease_until", time.Now().Add(-time.Second))
		_, _, _ = repo.Claim(ctx, uuid.New())
		got, _ := repo.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID)
		if got.State != onboarding.RecoveryRequired {
			t.Fatal("unknown dispatch resumed")
		}
	})
	t.Run("encrypted state transaction and target scoped handles", func(t *testing.T) {
		cipher, _ := credentials.NewStore("test", map[string]string{"test": strings.Repeat("x", 32)})
		store := messaging.NewStore(db, cipher)
		scope := providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New(), CredentialVersion: 1, BindingID: uuid.New(), BindingRevision: 1, Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}}
		db.Exec("INSERT INTO connector_connections VALUES (?,?,?,'weixin')", scope.ConnectionID, scope.OrganizationID, scope.OwnerAccountID)
		digest := strings.Repeat("d", 64)
		stop := errors.New("rollback fixture")
		err := db.Transaction(func(tx *gorm.DB) error {
			e := store.Update(messaging.WithTransaction(ctx, tx), scope, "weixin", digest, uuid.New(), func(_ context.Context, s *messaging.State) error {
				s.Cursor = "PRIVATE_CURSOR"
				s.Messages = []messaging.Message{{ID: "1", Recipient: "PRIVATE_RECIPIENT", ContextToken: "PRIVATE_CONTEXT"}}
				return nil
			})
			if providers.FailureOf(e).Code != core.ErrorCodeConflict {
				t.Fatal("nested consuming read allowed", e)
			}
			return stop
		})
		if !errors.Is(err, stop) {
			t.Fatal(err)
		}
		var n int64
		db.Table("connector_messaging_states").Count(&n)
		if n != 0 {
			t.Fatal("verification buffer escaped rollback")
		}
		handle, err := store.Put(ctx, scope, "weixin", messaging.Conversation{Recipient: "private-user", ContextToken: "private-context"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.Resolve(ctx, scope, "weixin", handle); err != nil {
			t.Fatal(err)
		}
		for _, mutate := range []func(*providers.ResourceScope){func(s *providers.ResourceScope) { s.OwnerAccountID = uuid.New() }, func(s *providers.ResourceScope) { s.OrganizationID = uuid.New() }, func(s *providers.ResourceScope) { s.ConnectionID = uuid.New() }, func(s *providers.ResourceScope) { s.CredentialVersion++ }, func(s *providers.ResourceScope) { s.BindingRevision++ }, func(s *providers.ResourceScope) { s.Target.ID = uuid.NewString() }} {
			forged := scope
			mutate(&forged)
			if _, err = store.Resolve(ctx, forged, "weixin", handle); err == nil {
				t.Fatal("context escaped scope")
			}
		}
		var sealed string
		db.Raw("SELECT encrypted_context FROM connector_messaging_contexts WHERE id = ?", handle).Scan(&sealed)
		if strings.Contains(sealed, "private-") || !strings.HasPrefix(sealed, "c1.") {
			t.Fatal("plaintext context")
		}
		// Expiry is enforced on reads before the periodic physical cleanup.
		if err = db.Table("connector_messaging_contexts").Where("id = ?", handle).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = store.Resolve(ctx, scope, "weixin", handle); err == nil {
			t.Fatal("expired context usable")
		}
		if err = store.Cleanup(ctx); err != nil {
			t.Fatal("cleanup", err)
		}
		db.Table("connector_messaging_contexts").Where("id = ?", handle).Count(&n)
		if n != 0 {
			t.Fatal("expired encrypted context retained")
		}

		// A committed verification receipt authorizes the first explicit pull.
		f := flowFixture("weixin")
		f.OrganizationID, f.OwnerAccountID = scope.OrganizationID, scope.OwnerAccountID
		f.State, f.Step = onboarding.Verifying, "verify"
		accepted := time.Now().UTC()
		expires := accepted.Add(15 * time.Minute)
		f.CandidateConnectionID, f.CandidateAcceptedAt, f.CandidateExpiresAt = &scope.ConnectionID, &accepted, &expires
		if err = db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		if err = store.Update(ctx, scope, "weixin", digest, f.ID, func(_ context.Context, s *messaging.State) error {
			s.Cursor = "0"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		terminal := time.Now().UTC()
		if err = db.Model(&onboarding.Flow{}).Where("id = ?", f.ID).Updates(map[string]any{"state": onboarding.Ready, "result_connection_id": scope.ConnectionID, "terminal_at": terminal}).Error; err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for attempt := 0; attempt < 100; attempt++ {
					e := store.Update(ctx, scope, "weixin", digest, uuid.Nil, func(_ context.Context, s *messaging.State) error {
						value, e := strconv.Atoi(s.Cursor)
						if e != nil {
							return e
						}
						s.Cursor = strconv.Itoa(value + 1)
						return nil
					})
					if e == nil {
						return
					}
					if providers.FailureOf(e).Code != core.ErrorCodeVerificationWaiting {
						t.Error(e)
						return
					}
					time.Sleep(time.Millisecond)
				}
				t.Error("read lease did not release")
			}()
		}
		wg.Wait()
		if err = store.Update(ctx, scope, "weixin", digest, uuid.Nil, func(_ context.Context, s *messaging.State) error {
			if s.Cursor != "10" {
				t.Errorf("cursor lost an update: %s", s.Cursor)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("messaging reliability", func(t *testing.T) { testMessagingReliabilityPostgres(t, db) })
}

func flowFixture(provider string) onboarding.Flow {
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(uuid.NewString()))
	hash := hex.EncodeToString(sum[:])
	intent, step := "register_bot", "begin"
	if provider == "weixin" {
		intent = "login_session"
	}
	if provider == "dingtalk" {
		step = "init"
	}
	return onboarding.Flow{ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: provider, Intent: intent, ProtocolVersion: 1, Handler: provider + "-registration-v1", Step: step, ExpirySource: "local_limit", AcceptedDefinitionDigest: hash, IdempotencyKeyHash: hash, RequestFingerprint: hash, State: onboarding.Starting, Generation: 1, Revision: 1, PollIntervalSeconds: 3, ExpiresAt: now.Add(5 * time.Minute), NextPollAt: now, CreatedAt: now, UpdatedAt: now}
}
