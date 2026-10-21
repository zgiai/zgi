package connections

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type mailVerifierFunc func(context.Context, CandidateVerificationRequest) (CandidateVerificationResult, error)

func (f mailVerifierFunc) Verify(ctx context.Context, r CandidateVerificationRequest) (CandidateVerificationResult, error) {
	return f(ctx, r)
}
func mailValues() map[string]string {
	return map[string]string{"email": "User.Name+tag@gmail.com", "app_password": "Abcd efgh ijkl MNOP"}
}
func mailResult(usage string) CandidateVerificationResult {
	return CandidateVerificationResult{ExternalAccountID: "User.Name+tag@gmail.com", ExternalDisplayName: "User.Name+tag@gmail.com", UsableOperationIDs: catalog.MailOperations(usage), Evidence: &core.ProviderEvidence{Mail: &core.MailEvidence{Version: 1, Provider: "gmail", IdentityKind: "authenticated_login", AuthenticatedLogin: "User.Name+tag@gmail.com", Usage: usage, CredentialVerified: true, CheckedAt: time.Now()}}}
}
func mailCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.New(catalog.GmailDefinition())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestMailSetupScopesVerificationAndIdempotencyBeforeSaving(t *testing.T) {
	s, _, sealer, repo := newKeyLifecycleTestService(t)
	s.catalog = mailCatalog(t)
	calls := 0
	s.verifier = mailVerifierFunc(func(_ context.Context, r CandidateVerificationRequest) (CandidateVerificationResult, error) {
		calls++
		if r.MailUsage != "send" || !reflect.DeepEqual(r.OperationIDs, []string{"gmail.message.send"}) || r.Credentials["app_password"] != "AbcdefghijklMNOP" {
			t.Fatal("unscoped or unnormalized verification")
		}
		return mailResult("send"), nil
	})
	command := KeySetupCommand{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "gmail", IdempotencyKey: uuid.NewString(), Values: mailValues(), MailUsage: "send"}
	result, err := s.StartKeySetup(context.Background(), command)
	if err != nil || result.Connection == nil || calls != 1 || sealer.calls != 1 {
		t.Fatalf("setup: %v", err)
	}
	if len(command.Values) != 0 || len(sealer.values) != 2 {
		t.Fatal("credential lifetime or field contract")
	}
	if len(result.Connection.Constraint.AllowedOperationIDs) != 1 || result.Connection.Constraint.AllowedOperationIDs[0] != "gmail.message.send" {
		t.Fatal("local send-only constraint widened")
	}
	command.Values = mailValues()
	replay, err := s.StartKeySetup(context.Background(), command)
	if err != nil || replay.Connection.ID != result.Connection.ID || calls != 1 {
		t.Fatal("duplicate reverified")
	}
	command.Values = mailValues()
	command.MailUsage = "read_send"
	if _, err = s.StartKeySetup(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) || calls != 1 || len(repo.connections) != 1 {
		t.Fatal("usage change reused prior idempotent command")
	}
	command.IdempotencyKey = uuid.NewString()
	command.Values = mailValues()
	command.MailUsage = ""
	if _, err = s.StartKeySetup(context.Background(), command); err == nil || calls != 1 {
		t.Fatal("missing usage accepted")
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "Abcd") {
		t.Fatal("password leaked")
	}
}
func TestMailPartialVerificationNeverSealsOrCreates(t *testing.T) {
	s, _, sealer, repo := newKeyLifecycleTestService(t)
	s.catalog = mailCatalog(t)
	s.verifier = mailVerifierFunc(func(context.Context, CandidateVerificationRequest) (CandidateVerificationResult, error) {
		return CandidateVerificationResult{MailVerification: &core.MailVerification{Usage: "read_send", IMAPStatus: "verified", SMTPStatus: "failed"}}, core.NewError(core.ErrorCodeMailAuthenticationRejected, false, nil)
	})
	values := mailValues()
	result, err := s.StartKeySetup(context.Background(), KeySetupCommand{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "gmail", IdempotencyKey: uuid.NewString(), Values: values, MailUsage: "read_send"})
	if err == nil || result.Connection != nil || sealer.calls != 0 || len(repo.connections) != 0 || len(values) != 0 || result.MailVerification == nil || result.MailVerification.SMTPStatus != "failed" {
		t.Fatal("partial verification saved or lost safe status")
	}
}
func TestMailReconnectLocksLoginAndDoesNotExpandPasswordMaintenance(t *testing.T) {
	s, repo, _, _, _, _ := newMaintenanceFixture(t)
	s.catalog = mailCatalog(t)
	existing := maintenanceConnection(s.now(), uuid.New(), uuid.New())
	existing.ConnectorID = "gmail"
	existing.ProviderID = "gmail"
	existing.DriverID = "gmail-mail-v1"
	existing.AuthMethodID = "gmail.app_password"
	identity := "User.Name+tag@gmail.com"
	existing.ExternalAccountID = &identity
	existing.UsableOperationIDs = []core.Identifier{"gmail.message.send"}
	existing.Constraint = core.OwnerConstraint(existing.OwnerAccountID, existing.UsableOperationIDs)
	existing.Constraint.DenyReasons = []string{"restricted"}
	repo.connections[existing.ID] = existing
	calls := 0
	s.verifier = mailVerifierFunc(func(_ context.Context, r CandidateVerificationRequest) (CandidateVerificationResult, error) {
		calls++
		if r.MailUsage != "send" {
			t.Fatal("password maintenance expanded usage")
		}
		return mailResult("send"), nil
	})
	command := ReconnectCommand{OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID, IdempotencyKey: uuid.NewString(), Values: mailValues()}
	command.Values["email"] = "username@gmail.com"
	if _, err := s.Reconnect(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) || calls != 0 {
		t.Fatal("alternate login accepted")
	}
	command.Values = mailValues()
	result, err := s.Reconnect(context.Background(), command)
	if err != nil || !result.ReplacedExisting || result.Connection.ID != existing.ID || result.Connection.CredentialVersion != existing.CredentialVersion+1 || calls != 1 {
		t.Fatalf("same login rotation: %v", err)
	}
	if len(result.Connection.Constraint.DenyReasons) != 1 || len(result.Connection.Constraint.AllowedOperationIDs) != 1 {
		t.Fatal("maintenance dropped other constraints")
	}
}
func TestMailLifecycleRejectsVerifierPrivilegeDrift(t *testing.T) {
	s, _, sealer, _ := newKeyLifecycleTestService(t)
	s.catalog = mailCatalog(t)
	s.verifier = mailVerifierFunc(func(context.Context, CandidateVerificationRequest) (CandidateVerificationResult, error) {
		return mailResult("read_send"), nil
	})
	_, err := s.StartKeySetup(context.Background(), KeySetupCommand{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "gmail", IdempotencyKey: uuid.NewString(), Values: mailValues(), MailUsage: "send"})
	if err == nil || sealer.calls != 0 {
		t.Fatal("verifier added unrequested operations")
	}
}
