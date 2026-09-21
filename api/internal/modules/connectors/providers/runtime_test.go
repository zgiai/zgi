package providers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

type driverFixture struct {
	id         string
	calls      int
	failure    error
	operations []string
}

func (d *driverFixture) ConnectorID() string { return d.id }
func (d *driverFixture) Verify(context.Context, string) (Verification, error) {
	d.calls++
	return Verification{Operations: d.operations}, d.failure
}
func (d *driverFixture) Invoke(context.Context, string, string, map[string]any) (Response, error) {
	d.calls++
	return Response{Value: map[string]any{"ok": true}}, d.failure
}

// sshDriverFixture is the narrow, network-free CredentialDriver stand-in for
// the required SSH Server Operations leaf. NewRuntime only needs its frozen
// connector identity and the CredentialDriver method set. Construction must not
// call the driver, so the fixture counts calls; every method fails closed, which
// keeps a test that accidentally reaches it off the network without weakening
// the required-driver invariant for any other built-in.
type sshDriverFixture struct {
	verifyCalls int
	invokeCalls int
}

func (*sshDriverFixture) ConnectorID() string { return "serverops_ssh" }
func (d *sshDriverFixture) VerifyCredentials(context.Context, CredentialInput) (Verification, error) {
	d.verifyCalls++
	return Verification{}, Invalid()
}
func (d *sshDriverFixture) InvokeCredentials(context.Context, string, CredentialInput, map[string]any) (Response, error) {
	d.invokeCalls++
	return Response{}, Invalid()
}
func (d *sshDriverFixture) calls() int { return d.verifyCalls + d.invokeCalls }

func TestRuntimeRejectsForgedLeafIdentityWithoutProviderCall(t *testing.T) {
	github := &driverFixture{id: "github", operations: []string{"github.user.get"}}
	ssh := &sshDriverFixture{}
	runtime, err := NewRuntime(github, &driverFixture{id: "web-search"}, &appDriverFixture{}, ssh)
	if err != nil {
		t.Fatal(err)
	}
	if ssh.calls() != 0 {
		t.Fatal("runtime construction reached the SSH Server Operations driver")
	}
	definitions, _ := catalog.BuiltinDefinitions()
	d := definitions[0]
	owner := uuid.New()
	request := execution.ProviderInvocation{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: uuid.New(), ConnectorID: core.Identifier(d.ID), ProviderID: core.Identifier(d.ProviderID), DriverID: core.Identifier(d.DriverID), AuthMethodID: core.Identifier(d.AuthMethodID), CredentialVersion: 1, Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, OperationID: "github.user.get", LogicalOperationID: "call-1", Credentials: map[string]string{d.KeyFields[0].Name: "fixture-key"}}
	for _, field := range []string{"provider", "driver", "auth", "owner", "operation"} {
		forged := request
		switch field {
		case "provider":
			forged.ProviderID = "other"
		case "driver":
			forged.DriverID = "other"
		case "auth":
			forged.AuthMethodID = "other"
		case "owner":
			forged.ActingPrincipalID = uuid.New()
		case "operation":
			forged.OperationID = "web.search"
		}
		if _, err = runtime.Invoke(context.Background(), forged); err == nil {
			t.Fatalf("accepted forged %s", field)
		}
	}
	if github.calls != 0 {
		t.Fatal("invalid identity reached provider")
	}
	github.failure = &Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	result, err := runtime.Invoke(context.Background(), request)
	if err == nil || result.Status != core.OutcomeStatusUnknown || github.calls != 1 {
		t.Fatal("unknown incorrectly mapped")
	}
	github.failure = nil
	verified, err := runtime.Verify(context.Background(), connections.CandidateVerificationRequest{OrganizationID: request.OrganizationID, OwnerAccountID: owner, ConnectionID: request.ConnectionID, ConnectorID: d.ID, DriverID: d.DriverID, AuthMethodID: d.AuthMethodID, Credentials: request.Credentials, OperationIDs: []string{"github.user.get"}})
	if err != nil || len(verified.UsableOperationIDs) != 1 {
		t.Fatalf("verification: %v", err)
	}
}

// TestRuntimeKeepsRequiredDriverInvariantForSSHServerOps pins that admitting the
// SSH Server Operations contract does not weaken NewRuntime: a non-messaging,
// non-mail built-in that has no driver still fails initialization instead of
// being silently skipped.
func TestRuntimeKeepsRequiredDriverInvariantForSSHServerOps(t *testing.T) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	admitsSSH := false
	for _, definition := range definitions {
		if definition.ID == "serverops_ssh" {
			admitsSSH = true
		}
	}
	if !admitsSSH {
		t.Skip("SSH Server Operations contract is not admitted")
	}
	if _, err = NewRuntime(&driverFixture{id: "github"}, &driverFixture{id: "web-search"}, &appDriverFixture{}); err == nil {
		t.Fatal("runtime accepted the catalog without the required SSH Server Operations driver")
	}
}
