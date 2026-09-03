package providers

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

type messagingDriverFixture struct {
	verification Verification
	id           string
}

func (d *messagingDriverFixture) ConnectorID() string { return d.id }
func (d *messagingDriverFixture) VerifyCredentials(context.Context, CredentialInput) (Verification, error) {
	return d.verification, nil
}
func (d *messagingDriverFixture) InvokeCredentials(context.Context, string, CredentialInput, map[string]any) (Response, error) {
	return Response{}, nil
}
func TestMessagingEvidenceCannotInventGrantOrOverrideDenial(t *testing.T) {
	d, _ := catalog.MessagingDefinition("dingtalk")
	driver := &messagingDriverFixture{id: d.ID}
	runtime, err := NewRuntime(&driverFixture{id: "github"}, &driverFixture{id: "web-search"}, &appDriverFixture{}, driver)
	if err != nil {
		t.Fatal(err)
	}
	request := connections.CandidateVerificationRequest{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: d.ID, DriverID: d.DriverID, AuthMethodID: d.AuthMethodID, Credentials: map[string]string{"client_id": "app1", "client_secret": "secret"}}
	op := d.Operations[0].ID
	driver.verification = Verification{AccountID: "dingtalk:app:app1", Operations: []string{op}}
	if _, err = runtime.Verify(context.Background(), request); err == nil {
		t.Fatal("missing evidence accepted")
	}
	driver.verification.Evidence = &core.ProviderEvidence{GrantedScopes: []string{"forged"}, ScopeStatus: "verified", Messaging: &core.MessagingEvidence{Version: 1, Provider: "dingtalk", Identity: driver.verification.AccountID, CredentialVerified: true, Operations: []core.OperationEvidence{{ID: op, Availability: "conditional", Permission: "granted"}}}}
	v, err := runtime.Verify(context.Background(), request)
	if err != nil || v.Evidence.ScopeStatus != "unknown" || len(v.Evidence.GrantedScopes) != 0 || v.Evidence.Messaging.Operations[0].Permission != "unknown" {
		t.Fatal("invented grant", err)
	}
	driver.verification.Evidence.Messaging.Operations = append(driver.verification.Evidence.Messaging.Operations, core.OperationEvidence{ID: op, Availability: "denied"})
	if _, err = runtime.Verify(context.Background(), request); err == nil {
		t.Fatal("denial overridden")
	}
	other, _ := catalog.MessagingDefinition("wecom")
	entry := registration{definition: other}
	e := driver.verification.Evidence.Clone()
	e.Messaging.Provider = "wecom"
	e.Messaging.Identity = "wecom:bot:b"
	e.Messaging.Operations = []core.OperationEvidence{{ID: other.Operations[0].ID, Availability: "conditional"}}
	safe := entry.safeEvidence(e, CredentialInput{Values: map[string]string{"bot_id": "b", "secret": "secret"}})
	if safe == nil || len(safe.Messaging.Operations) != 0 {
		t.Fatal("conditional leaked to another provider")
	}
}
