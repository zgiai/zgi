package connections

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func appValues() map[string]string {
	return map[string]string{"region": "feishu", "app_id": "cli_test", "app_secret": candidateKeySentinel}
}
func appCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.New(d)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func appVerification() CandidateVerificationResult {
	return CandidateVerificationResult{ExternalAccountID: "feishu:cli_test", ExternalDisplayName: "Test app", UsableOperationIDs: []string{"feishu.bot.get"}}
}
func TestAppSetupUsesExistingMultiConnectionAndIdempotencyLifecycle(t *testing.T) {
	s, v, sealer, repo := newKeyLifecycleTestService(t)
	s.catalog = appCatalog(t)
	v.result = appVerification()
	org, owner := uuid.New(), uuid.New()
	command := KeySetupCommand{OrganizationID: org, OwnerAccountID: owner, ConnectorID: "feishu", IdempotencyKey: uuid.NewString(), Values: appValues()}
	one, err := s.StartKeySetup(context.Background(), command)
	if err != nil || one.Connection == nil {
		t.Fatalf("app setup: %v", err)
	}
	if len(command.Values) != 0 || len(sealer.values) != 3 || len(v.credentials) != 3 {
		t.Fatal("credential fields lost or retained")
	}
	command.Values = appValues()
	replay, err := s.StartKeySetup(context.Background(), command)
	if err != nil || replay.Connection.ID != one.Connection.ID || v.calls != 1 {
		t.Fatal("replay created another connection")
	}
	command.Values = appValues()
	command.IdempotencyKey = uuid.NewString()
	two, err := s.StartKeySetup(context.Background(), command)
	if err != nil || two.Connection.ID == one.Connection.ID || len(repo.connections) != 2 {
		t.Fatal("multiple personal applications forbidden")
	}
	if _, err = s.GetConnection(context.Background(), org, uuid.New(), one.Connection.ID); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatal("cross-owner access")
	}
	assertLifecycleJSONHasNoCandidate(t, one)
}
func TestAppReconnectFailurePreservesOldCredentialAndSuccessRotates(t *testing.T) {
	s, repo, _, v, sealer, _ := newMaintenanceFixture(t)
	s.catalog = appCatalog(t)
	v.result = appVerification()
	d, _ := catalog.FeishuDefinition()
	existing := maintenanceConnection(s.now(), uuid.New(), uuid.New())
	existing.ConnectorID = core.Identifier(d.ID)
	existing.ProviderID = core.Identifier(d.ProviderID)
	existing.DriverID = core.Identifier(d.DriverID)
	existing.AuthMethodID = core.Identifier(d.AuthMethodID)
	identity := "feishu:cli_test"
	existing.ExternalAccountID = &identity
	existing.UsableOperationIDs = []core.Identifier{"feishu.bot.get"}
	repo.connections[existing.ID] = existing
	command := ReconnectCommand{OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID, IdempotencyKey: uuid.NewString(), Values: appValues()}
	v.err = core.NewError(core.ErrorCodeInvalidAppCredentials, false, nil)
	if _, err := s.Reconnect(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeInvalidAppCredentials) || sealer.calls != 0 || repo.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials {
		t.Fatal("failed candidate replaced active app")
	}
	v.err = nil
	command.IdempotencyKey = uuid.NewString()
	command.Values = appValues()
	result, err := s.Reconnect(context.Background(), command)
	if err != nil || !result.ReplacedExisting || result.Connection.ID != existing.ID || result.Connection.CredentialVersion != existing.CredentialVersion+1 {
		t.Fatalf("rotation: %v", err)
	}
	command.OwnerAccountID = uuid.New()
	command.Values = appValues()
	command.IdempotencyKey = uuid.NewString()
	if _, err = s.Reconnect(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeNotFound) || v.calls != 2 {
		t.Fatal("non-owner verified app")
	}
}
