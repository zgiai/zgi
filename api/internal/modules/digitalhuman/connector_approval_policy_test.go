package digitalhuman

import (
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
)

func TestBindingAuthorizationModeMapsTheApprovalPolicy(t *testing.T) {
	ssh := catalog.ServerOpsSSHDefinition()
	for metadata, want := range map[string]string{
		`{"allowed_operation_ids":["server.command.run"]}`:                                    core.AuthorizationModeAsk,
		`{"approval_policy":"always_ask"}`:                                                    core.AuthorizationModeAsk,
		`{"approval_policy":"read_only_auto","allowed_operation_ids":["server.command.run"]}`: core.AuthorizationModeReadOnlyAuto,
		`{"approval_policy":"trusted"}`:                                                       core.AuthorizationModeTrusted,
		`{"approval_policy":"trusted_unattended"}`:                                            core.AuthorizationModeTrustedUnattended,
	} {
		got, err := bindingAuthorizationMode(&ResourceBinding{Metadata: datatypes.JSON(metadata)}, ssh)
		if err != nil || got != want {
			t.Errorf("bindingAuthorizationMode(%s) = %q, %v, want %q", metadata, got, err, want)
		}
	}
	if _, err := bindingAuthorizationMode(&ResourceBinding{Metadata: datatypes.JSON(`{"approval_policy":"direct"}`)}, ssh); !errors.Is(err, ErrResourceBindingInvalid) {
		t.Errorf("an unknown policy was accepted: %v", err)
	}
	other := ssh
	other.ID = "github"
	if _, err := bindingAuthorizationMode(&ResourceBinding{Metadata: datatypes.JSON(`{"approval_policy":"read_only_auto"}`)}, other); !errors.Is(err, ErrResourceBindingInvalid) {
		t.Errorf("a connector that cannot prove reads accepted read_only_auto: %v", err)
	}
}

func TestConnectorRequesterIsOnlyTheAccountInAConsoleConversation(t *testing.T) {
	account := invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: "account-1"}
	for name, test := range map[string]struct {
		event invocation.Event
		want  string
	}{
		"console":  {invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginManual}, Sender: account}, "account-1"},
		"channel":  {invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginChannel}, Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_1"}}, ""},
		"api":      {invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}, Sender: account}, ""},
		"schedule": {invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}, Sender: account}, ""},
	} {
		if got := connectorRequester(test.event); got != test.want {
			t.Errorf("%s: connectorRequester() = %q, want %q", name, got, test.want)
		}
	}
}

func TestConnectorApprovalLineDescribesWhatThisRunAllows(t *testing.T) {
	for name, test := range map[string]struct {
		policy                    string
		interactive, ownerPresent bool
		want                      string
	}{
		"trusted owner present":       {ConnectorApprovalTrusted, true, true, "every call runs without approval"},
		"trusted someone else":        {ConnectorApprovalTrusted, true, false, "read-only run without approval"},
		"trusted scheduled":           {ConnectorApprovalTrusted, false, false, "only with the owner's task grant"},
		"trusted unattended schedule": {ConnectorApprovalTrustedUnattended, false, false, "every call runs without approval"},
		"read-only console":           {ConnectorApprovalReadOnlyAuto, true, true, "read-only run without approval"},
		"always ask console":          {ConnectorApprovalAlwaysAsk, true, true, ""},
	} {
		got := connectorApprovalLine("prod", test.policy, test.interactive, test.ownerPresent)
		if (test.want == "" && got != "") || !strings.Contains(got, test.want) {
			t.Errorf("%s: connectorApprovalLine() = %q, want it to contain %q", name, got, test.want)
		}
	}
}
