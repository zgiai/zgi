package messaging

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func isolatedLeasePostgres(t *testing.T) *DBStore {
	t.Helper()
	dsn := os.Getenv("ZGI_CONNECTOR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ZGI_CONNECTOR_TEST_POSTGRES_DSN is not configured")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	admin := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = admin.Close() })
	schema := "connector_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal("cannot create isolated test schema")
	}
	t.Cleanup(func() {
		if !strings.HasPrefix(schema, "connector_test_") || len(schema) != 47 {
			t.Error("unsafe test schema")
			return
		}
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error("cannot remove isolated test schema")
		}
	})
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = pool.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect test database")
	}
	return &DBStore{db: db}
}

func TestPostgresMessagingLeaseBudgetConcurrencyAndUncertainty(t *testing.T) {
	repo := isolatedLeasePostgres(t)
	// Exercise the real table constraints in the isolated search_path only.
	for file, table := range map[string]string{
		"20260903120000_messaging_connector_protocol.go": "connector_messaging_states",
		"20260906120000_messaging_auth_reliability.go":   "connector_messaging_read_leases",
	} {
		raw, err := os.ReadFile("../../../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		marker := "CREATE TABLE public." + table + " ("
		_, rest, ok := strings.Cut(string(raw), marker)
		if !ok {
			t.Fatal("table migration missing")
		}
		definition, _, ok := strings.Cut(rest, "\n);")
		if !ok {
			t.Fatal("table definition missing")
		}
		if err = repo.db.Exec("CREATE TABLE " + table + " (" + definition + "\n);").Error; err != nil {
			t.Fatal("isolated table creation failed")
		}
	}
	secrets, err := credentials.NewStore("test", map[string]string{"test": strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	scope := providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New()}
	id, digest := uuid.New(), strings.Repeat("a", 64)
	sealed, err := secrets.Protect(credentials.Subject{Purpose: credentials.MessagingState, OrganizationID: scope.OrganizationID, ID: id, ConnectorID: "weixin", Version: 1}, []byte(`{"Messages":[],"Seen":[],"Cursor":"before"}`))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = repo.db.Exec(`INSERT INTO connector_messaging_states (id,organization_id,owner_account_id,connector_id,connection_id,credential_digest,revision,encrypted_state,expires_at,updated_at) VALUES (?,?,?,'weixin',?,?,1,?,?,?)`, id, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, digest, sealed, now.Add(time.Hour), now).Error
	if err != nil {
		t.Fatal("state fixture failed")
	}
	store := NewStore(repo.db, secrets)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	go func() {
		finished <- store.Update(context.Background(), scope, "weixin", digest, uuid.Nil, func(_ context.Context, state *State) error {
			close(entered)
			<-release
			state.Cursor = "after"
			state.LongPollingTimeoutMS = 45000
			return nil
		})
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatal("lease claim failed", err)
	case <-time.After(5 * time.Second):
		t.Fatal("lease claim timeout")
	}
	var lease struct {
		Until     time.Time
		Fence     int64
		Uncertain bool
	}
	if err := repo.db.Table("connector_messaging_read_leases").Take(&lease).Error; err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(lease.Until)
	if remaining < 60*time.Second || remaining > 65*time.Second || lease.Fence != 1 {
		t.Fatal("lease does not cover poll and checkpoint", remaining)
	}
	called := false
	err = store.Update(context.Background(), scope, "weixin", digest, uuid.Nil, func(context.Context, *State) error { called = true; return nil })
	if called || providers.FailureOf(err).Code != core.ErrorCodeVerificationWaiting {
		t.Fatal("concurrent consuming read dispatched")
	}
	close(release)
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("checkpoint timeout")
	}
	err = store.Update(context.Background(), scope, "weixin", digest, uuid.Nil, func(_ context.Context, state *State) error {
		if state.Cursor != "after" || state.LongPollingTimeoutMS != 45000 {
			t.Error("checkpoint did not survive reload")
		}
		return &providers.Failure{Code: core.ErrorCodeVerificationUncertain, Unknown: true}
	})
	if !providers.FailureOf(err).Unknown {
		t.Fatal("uncertainty lost")
	}
	err = store.Update(context.Background(), scope, "weixin", digest, uuid.Nil, func(context.Context, *State) error { called = true; return nil })
	if called || providers.FailureOf(err).Code != core.ErrorCodeVerificationUncertain {
		t.Fatal("uncertain consumption replayed")
	}
}
