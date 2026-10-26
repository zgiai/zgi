package providers

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"testing"
)

type scopedKeyFixture struct {
	driverFixture
	scope ResourceScope
	token string
}

func (d *scopedKeyFixture) InvokeWithScope(_ context.Context, _ string, token string, _ map[string]any, scope ResourceScope) (Response, error) {
	d.scope = scope
	d.token = token
	return Response{Value: map[string]any{"ok": true}}, nil
}
func TestRuntimePassesRestoredScopeToOptionalKeyDriver(t *testing.T) {
	d := &scopedKeyFixture{driverFixture: driverFixture{id: "github"}}
	runtime, err := NewRuntime(d, &driverFixture{id: "github-mcp"}, &driverFixture{id: "web-search"}, &appDriverFixture{}, &sshDriverFixture{})
	if err != nil {
		t.Fatal(err)
	}
	defs, _ := catalog.BuiltinDefinitions()
	var def catalog.Definition
	for _, entry := range defs {
		if entry.ID == "github" {
			def = entry
		}
	}
	owner := uuid.New()
	request := execution.ProviderInvocation{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: uuid.New(), ConnectorID: "github", ProviderID: core.Identifier(def.ProviderID), DriverID: core.Identifier(def.DriverID), AuthMethodID: core.Identifier(def.AuthMethodID), CredentialVersion: 3, BindingID: uuid.New(), BindingRevision: 4, LogicalOperationID: "request-1", OperationID: "github.user.get", Target: core.TargetRef{Type: core.TargetTypeAgent, ID: uuid.NewString()}, Credentials: map[string]string{def.KeyFields[0].Name: "secret"}, Input: map[string]any{"connection_id": "spoofed"}}
	if _, err = runtime.Invoke(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if d.calls != 0 || d.scope.ConnectionID != request.ConnectionID || d.scope.OwnerAccountID != owner || d.scope.OrganizationID != request.OrganizationID || d.scope.CredentialVersion != 3 || d.scope.BindingID != request.BindingID || d.scope.BindingRevision != 4 || d.scope.LogicalOperationID != "request-1" || d.scope.Target != request.Target || d.token != "secret" {
		t.Fatal("scope did not come from runtime request")
	}
}
