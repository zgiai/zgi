package connections

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This opt-in suite creates and removes only a uniquely named test schema.
// Credentials are read from the environment and never included in test output.
func isolatedPolicyPostgres(t *testing.T) *GormRepository {
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
	if err = db.AutoMigrate(&connectionRecord{}, &providerEvidenceRecord{}, &connectionLifecycleDecisionRecord{}); err != nil {
		t.Fatal("cannot initialize test schema")
	}
	if err = db.Exec("CREATE UNIQUE INDEX operation_policy_receipt_unique ON connector_lifecycle_decisions (organization_id, correlation_id, decision, status)").Error; err != nil {
		t.Fatal("cannot initialize receipt constraint")
	}
	return &GormRepository{db: db}
}

func TestPostgresOperationPolicyCASReplayAndRollback(t *testing.T) {
	repo := isolatedPolicyPostgres(t)
	s, _, _, _, _, _ := newMaintenanceFixture(t)
	s.repository = repo
	s.now = func() time.Time { return time.Now().UTC() }
	s.catalog, _ = catalog.New(catalog.WeComReadDefinition())
	c := readyConnection(uuid.New(), uuid.New(), uuid.New())
	c.ConnectorID = "wecom"
	c.ProviderID = "wecom"
	c.DriverID = "wecom-http-v1"
	c.AuthMethodID = "wecom.credentials"
	identity := "wecom:bot:fixture"
	c.ExternalAccountID = &identity
	c.UsableOperationIDs = []core.Identifier{"wecom.identity.get", "wecom.sessions.list", "wecom.messages.send"}
	c.Constraint = core.OwnerConstraint(c.OwnerAccountID, c.UsableOperationIDs)
	expiry := time.Now().Add(time.Hour)
	c.Constraint.ExpiresAt = &expiry
	c.Constraint.SourceDigest = strings.Repeat("a", 64)
	if err := repo.CreateConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	command := OperationPolicyCommand{OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectionID: c.ID, ExpectedRevision: 1, OperationIDs: []string{}, IdempotencyKey: uuid.NewString()}
	first, err := s.UpdateOperationPolicy(context.Background(), command)
	if err != nil || first.Revision != 2 || first.OperationIDs == nil || len(first.OperationIDs) != 0 {
		t.Fatal(first, err)
	}
	stored, err := repo.GetConnection(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID)
	if err != nil || len(stored.Constraint.AllowedOperationIDs) != 0 || stored.Constraint.SourceDigest != c.Constraint.SourceDigest || !stored.Constraint.ExpiresAt.Equal(expiry) || !reflect.DeepEqual(stored.UsableOperationIDs, c.UsableOperationIDs) {
		t.Fatal("constraint persistence", err)
	}
	if stored.Constraint.Allows(c.OwnerAccountID, "wecom.messages.send", core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, time.Now()) {
		t.Fatal("empty policy allowed execution")
	}
	replay, err := s.UpdateOperationPolicy(context.Background(), command)
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatal("receipt replay", err)
	}
	command.OperationIDs = []string{"wecom.identity.get"}
	if _, err = s.UpdateOperationPolicy(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("key reused with different input", err)
	}
	command.ExpectedRevision = 2
	command.IdempotencyKey = uuid.NewString()
	command.OperationIDs = []string{"wecom.documents.search"}
	if _, err = s.UpdateOperationPolicy(context.Background(), command); err == nil {
		t.Fatal("old connection gained unaccepted read")
	}
	command.OperationIDs = []string{"wecom.identity.get", "wecom.identity.get"}
	if _, err = s.UpdateOperationPolicy(context.Background(), command); err == nil {
		t.Fatal("duplicates accepted")
	}
	command.OperationIDs = []string{"wecom.identity.get"}
	wrong := command
	wrong.OwnerAccountID = uuid.New()
	if _, err = s.UpdateOperationPolicy(context.Background(), wrong); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatal("owner scope crossed")
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, op := range []string{"wecom.identity.get", "wecom.sessions.list"} {
		wg.Add(1)
		go func(op string) {
			defer wg.Done()
			cmd := command
			cmd.IdempotencyKey = uuid.NewString()
			cmd.OperationIDs = []string{op}
			_, err := s.UpdateOperationPolicy(context.Background(), cmd)
			outcomes <- err
		}(op)
	}
	wg.Wait()
	close(outcomes)
	success, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if core.IsErrorCode(err, core.ErrorCodeConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("CAS did not serialize writers")
	}
	stored, err = repo.GetConnection(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.db.Exec(`CREATE FUNCTION reject_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test audit failure'; END $$`).Error; err != nil {
		t.Fatal("cannot install test audit failure")
	}
	if err = repo.db.Exec(`CREATE TRIGGER reject_policy_audit BEFORE INSERT ON connector_lifecycle_decisions FOR EACH ROW EXECUTE FUNCTION reject_policy_audit()`).Error; err != nil {
		t.Fatal("cannot install test audit trigger")
	}
	command.ExpectedRevision = stored.Revision
	command.IdempotencyKey = uuid.NewString()
	command.OperationIDs = []string{}
	if _, err = s.UpdateOperationPolicy(context.Background(), command); err == nil {
		t.Fatal("audit failure committed")
	}
	after, err := repo.GetConnection(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID)
	if err != nil || after.Revision != stored.Revision || !reflect.DeepEqual(after.Constraint, stored.Constraint) {
		t.Fatal("failed audit changed policy", err)
	}
}

func TestPostgresMailAndMessagingEvidencePersistence(t *testing.T) {
	repo := isolatedPolicyPostgres(t)
	for _, id := range []string{"gmail", "qqmail", "netease163", "wecom"} {
		t.Run(id, func(t *testing.T) {
			c := readyConnection(uuid.New(), uuid.New(), uuid.New())
			c.ConnectorID = core.Identifier(id)
			c.ProviderID = c.ConnectorID
			identity := "wecom:bot:fixture"
			if profile, ok := catalog.MailProfileFor(id); ok {
				c.DriverID = core.Identifier(profile.DriverID)
				c.AuthMethodID = core.Identifier(profile.AuthMethodID)
				domain := profile.Domain
				if domain == "" {
					domain = "gmail.com"
				}
				identity = "User+tag@" + domain
				c.UsableOperationIDs = []core.Identifier{core.Identifier(id + ".message.send")}
				c.ProviderEvidence = &core.ProviderEvidence{Mail: &core.MailEvidence{Version: 1, Provider: id, IdentityKind: "authenticated_login", AuthenticatedLogin: identity, CredentialVerified: true, Usage: "send", SMTPStatus: "verified", IMAPStatus: "not_requested", Operations: []core.OperationEvidence{{ID: id + ".message.send", Availability: "conditional"}}, CheckedAt: time.Now().UTC()}}
			} else {
				c.DriverID = "wecom-http-v1"
				c.AuthMethodID = "wecom.credentials"
				c.UsableOperationIDs = []core.Identifier{"wecom.identity.get"}
				c.ProviderEvidence = &core.ProviderEvidence{Messaging: &core.MessagingEvidence{Version: 1, Provider: id, Identity: identity, CredentialVerified: true, Operations: []core.OperationEvidence{{ID: "wecom.identity.get", Availability: "verified"}}}}
			}
			c.ExternalAccountID = &identity
			c.Constraint = core.OwnerConstraint(c.OwnerAccountID, c.UsableOperationIDs)
			if err := repo.CreateConnection(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if err := writeProviderEvidence(context.Background(), repo.db, c); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetConnection(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID)
			if err != nil || stored.ProviderEvidence == nil || *stored.ExternalAccountID != identity {
				t.Fatal("evidence not persisted", err)
			}
			raw, _ := json.Marshal(stored)
			if strings.Contains(string(raw), encryptedCredentialSentinel) {
				t.Fatal("credential in projection")
			}
		})
	}
}
