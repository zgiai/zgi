package connections

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestMailExplicitReadSendUpgrade(t *testing.T)         { testMailUpgrade(t, false) }
func TestPostgresMailExplicitReadSendUpgrade(t *testing.T) { testMailUpgrade(t, true) }

func testMailUpgrade(t *testing.T, postgres bool) {
	for _, id := range []string{"gmail", "qqmail", "netease163"} {
		for _, oldUsage := range []string{"read", "send"} {
			t.Run(id+"/"+oldUsage, func(t *testing.T) {
				ctx := context.Background()
				s, memory, bindings, _, sealer, _ := newMaintenanceFixture(t)
				s.now = func() time.Time { return time.Now().UTC() }
				definition, _ := catalog.MailDefinition(id)
				s.catalog, _ = catalog.New(definition)
				c := maintenanceConnection(s.now(), uuid.New(), uuid.New())
				c.ConnectorID, c.ProviderID, c.DriverID, c.AuthMethodID = core.Identifier(definition.ID), core.Identifier(definition.ProviderID), core.Identifier(definition.DriverID), core.Identifier(definition.AuthMethodID)
				domain := map[string]string{"gmail": "gmail.com", "qqmail": "qq.com", "netease163": "163.com"}[id]
				login := "User+tag@" + domain
				c.ExternalAccountID, c.ExternalDisplayName = &login, &login
				c.Name = "Saved alias"
				c.UsableOperationIDs = coreIDs(catalog.MailOperationsFor(id, oldUsage))
				c.Constraint = core.OwnerConstraint(c.OwnerAccountID, c.UsableOperationIDs)
				expiry := s.now().Add(time.Hour).Truncate(time.Microsecond)
				c.Constraint.ExpiresAt = &expiry
				c.Constraint.DenyReasons = []string{"saved restriction"}
				c.Constraint.SourceDigest = strings.Repeat("a", 64)
				bindings.items = []core.TargetBinding{maintenanceBinding(c, core.BindingStatusActive)}
				savedBindings := append([]core.TargetBinding(nil), bindings.items...)
				var pg *GormRepository
				if postgres {
					pg = isolatedPolicyPostgres(t)
					if err := pg.db.AutoMigrate(&setupAttemptRecord{}); err != nil {
						t.Fatal("test attempt schema")
					}
					if err := pg.CreateConnection(ctx, c); err != nil {
						t.Fatal(err)
					}
					s.repository = pg
				} else {
					memory.connections[c.ID] = c
				}
				read := func() core.PersonalConnection {
					result, err := s.repository.GetConnection(ctx, c.OrganizationID, c.OwnerAccountID, c.ID)
					if err != nil {
						t.Fatal(err)
					}
					return result
				}
				before := read()
				values := func() map[string]string {
					field := "authorization_code"
					if id == "gmail" {
						field = "app_password"
					}
					return map[string]string{"email": login, field: "Abcdefghijklmnop"}
				}
				command := func() ReconnectCommand {
					return ReconnectCommand{OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectionID: c.ID, IdempotencyKey: uuid.NewString(), Values: values(), MailUsage: "read_send"}
				}
				failing := "smtp"
				calls := 0
				s.verifier = mailVerifierFunc(func(_ context.Context, request CandidateVerificationRequest) (CandidateVerificationResult, error) {
					calls++
					if request.ConnectorID != id || request.MailUsage != "read_send" || !reflect.DeepEqual(request.OperationIDs, catalog.MailOperationsFor(id, "read_send")) {
						t.Fatal("upgrade did not request both directions on the real provider")
					}
					if failing != "" {
						verification := &core.MailVerification{Usage: "read_send", IMAPStatus: "verified", SMTPStatus: "failed"}
						if failing == "imap" {
							verification.IMAPStatus = "failed"
							verification.SMTPStatus = "not_requested"
						}
						return CandidateVerificationResult{MailVerification: verification}, core.NewError(core.ErrorCodeMailAuthenticationRejected, false, nil)
					}
					result := mailResult("read_send")
					result.ExternalAccountID, result.ExternalDisplayName = login, login
					result.UsableOperationIDs = catalog.MailOperationsFor(id, "read_send")
					result.Evidence.Mail.Provider, result.Evidence.Mail.AuthenticatedLogin = id, login
					return result, nil
				})
				for _, phase := range []string{"imap", "smtp"} {
					failing = phase
					failed := command()
					if result, err := s.Reconnect(ctx, failed); err == nil || result.Connection != nil || sealer.calls != 0 || len(failed.Values) != 0 {
						t.Fatal("partial verification persisted an upgrade or retained credentials")
					}
					if !reflect.DeepEqual(before, read()) {
						t.Fatal("partial verification changed saved connection")
					}
				}
				failing = ""
				if pg != nil {
					if err := pg.db.Exec(`CREATE FUNCTION reject_mail_upgrade() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test audit failure'; END $$`).Error; err != nil {
						t.Fatal("test trigger function")
					}
					if err := pg.db.Exec(`CREATE TRIGGER reject_mail_upgrade BEFORE INSERT ON connector_lifecycle_decisions FOR EACH ROW EXECUTE FUNCTION reject_mail_upgrade()`).Error; err != nil {
						t.Fatal("test trigger")
					}
					if _, err := s.Reconnect(ctx, command()); err == nil {
						t.Fatal("audit failure accepted upgrade")
					}
					if !reflect.DeepEqual(before, read()) {
						t.Fatal("failed commit did not roll back credentials, constraints or evidence")
					}
					if err := pg.db.Exec(`DROP TRIGGER reject_mail_upgrade ON connector_lifecycle_decisions`).Error; err != nil {
						t.Fatal("test trigger cleanup")
					}
				}
				success := command()
				result, err := s.Reconnect(ctx, success)
				if err != nil || result.Connection == nil || !result.ReplacedExisting || result.Connection.ID != c.ID {
					t.Fatal("upgrade failed", err)
				}
				after := read()
				if after.Name != c.Name || after.OwnerAccountID != c.OwnerAccountID || after.ConnectorID != c.ConnectorID || after.CredentialVersion != c.CredentialVersion+1 || after.Revision != c.Revision+1 {
					t.Fatal("upgrade changed identity, alias or revision incorrectly")
				}
				if !reflect.DeepEqual(after.Constraint.AllowedOperationIDs, coreIDs(catalog.MailOperationsFor(id, "read_send"))) || after.Constraint.SourceDigest != c.Constraint.SourceDigest || !after.Constraint.ExpiresAt.Equal(expiry) || !reflect.DeepEqual(after.Constraint.DenyReasons, c.Constraint.DenyReasons) {
					t.Fatal("upgrade lost unrelated constraints")
				}
				if !reflect.DeepEqual(savedBindings, bindings.items) {
					t.Fatal("upgrade changed saved bindings")
				}
				success.Values = values()
				beforeReplay := calls
				if _, err := s.Reconnect(ctx, success); err != nil || calls != beforeReplay {
					t.Fatal("upgrade replay reverified")
				}
			})
		}
	}
}

func coreIDs(ids []string) []core.Identifier {
	result := make([]core.Identifier, len(ids))
	for i, id := range ids {
		result[i] = core.Identifier(id)
	}
	return result
}
