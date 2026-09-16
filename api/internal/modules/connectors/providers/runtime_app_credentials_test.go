package providers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

type appDriverFixture struct {
	calls int
	got   map[string]string
}

func (*appDriverFixture) ConnectorID() string { return "feishu" }
func (d *appDriverFixture) VerifyCredentials(_ context.Context, input CredentialInput) (Verification, error) {
	d.calls++
	d.got = input.Values
	return Verification{AccountID: "feishu:cli_test", DisplayName: "name " + input.Values["app_secret"], Operations: []string{"feishu.bot.get"}, Evidence: &core.ProviderEvidence{AppName: input.Values["app_secret"], GrantedScopes: []string{input.Values["app_secret"]}, ScopeStatus: "verified", EventsEnabled: true, UserAuthorizationEnabled: true}}, nil
}
func (d *appDriverFixture) InvokeCredentials(_ context.Context, _ string, input CredentialInput, _ map[string]any) (Response, error) {
	d.calls++
	d.got = input.Values
	return Response{Value: map[string]any{"text": input.Values["app_secret"]}}, nil
}
func TestAppRuntimeCarriesFullCredentialsAndRedactsEverySecret(t *testing.T) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	driver := &appDriverFixture{}
	runtime := &Runtime{drivers: map[string]registration{d.ID: {definition: d, driver: driver}}}
	values := map[string]string{"region": "feishu", "app_id": "cli_test", "app_secret": "APP-SECRET-SENTINEL"}
	org, owner, id := uuid.New(), uuid.New(), uuid.New()
	verified, err := runtime.Verify(context.Background(), connections.CandidateVerificationRequest{OrganizationID: org, OwnerAccountID: owner, ConnectionID: id, ConnectorID: d.ID, DriverID: d.DriverID, AuthMethodID: d.AuthMethodID, Credentials: values})
	if err != nil || !reflect.DeepEqual(driver.got, values) || verified.Evidence == nil {
		t.Fatalf("typed verification: %v", err)
	}
	b, _ := json.Marshal(verified)
	if strings.Contains(string(b), values["app_secret"]) || verified.Evidence.EventsEnabled || verified.Evidence.UserAuthorizationEnabled {
		t.Fatal("unsafe evidence")
	}
	request := execution.ProviderInvocation{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: id, ConnectorID: core.Identifier(d.ID), ProviderID: core.Identifier(d.ProviderID), DriverID: core.Identifier(d.DriverID), AuthMethodID: core.Identifier(d.AuthMethodID), CredentialVersion: 1, Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, OperationID: "feishu.bot.get", LogicalOperationID: "typed-1", Credentials: values}
	result, err := runtime.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(result)
	if strings.Contains(string(b), values["app_secret"]) {
		t.Fatal("secret in output")
	}
	request.ActingPrincipalID = uuid.New()
	if _, err = runtime.Invoke(context.Background(), request); err == nil || driver.calls != 2 {
		t.Fatal("non-owner reached driver")
	}
}
