package execution

import (
	"context"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestMailSendTimeoutIsScopedToAcceptedDriver(t *testing.T) {
	service := &Service{providerTimeout: defaultProviderInvocationTimeout}
	connection := core.PersonalConnection{ConnectorID: "gmail", ProviderID: "gmail", DriverID: "gmail-mail-v1", AuthMethodID: "gmail.app_password"}
	if got := service.invocationTimeout(connection, "gmail.message.send"); got != 12*time.Minute {
		t.Fatalf("send timeout = %s", got)
	}
	if got := service.invocationTimeout(connection, "gmail.message.get"); got != defaultProviderInvocationTimeout {
		t.Fatalf("read timeout = %s", got)
	}
	connection.DriverID = "other"
	if got := service.invocationTimeout(connection, "gmail.message.send"); got != defaultProviderInvocationTimeout {
		t.Fatalf("unaccepted driver timeout = %s", got)
	}
	connection.DriverID = "gmail-mail-v1"
	service.providerTimeout = time.Second
	if got := service.invocationTimeout(connection, "gmail.message.send"); got != time.Second {
		t.Fatalf("explicit timeout overridden: %s", got)
	}
}

func TestMailConfirmedSendUsesMatchingLeaseAndProviderDeadline(t *testing.T) {
	harness := newExecutionHarness(t)
	definition := catalog.GmailDefinition()
	harness.service.catalog = &definitionCatalogStub{definition: definition}
	harness.pinned.Request.ConnectorID = "gmail"
	harness.pinned.Request.OperationID = "gmail.message.send"
	harness.pinned.Request.Input = map[string]any{"to": []string{"recipient@example.com"}, "subject": "测试", "text": "local fixture only"}
	harness.bindings.binding.ConnectorID = "gmail"
	harness.bindings.binding.Constraint = core.OwnerConstraint(harness.pinned.Request.OwnerAccountID, []core.Identifier{"gmail.message.send"})
	connection := &harness.connections.connection
	connection.ConnectorID = "gmail"
	connection.ProviderID = "gmail"
	connection.DriverID = "gmail-mail-v1"
	connection.AuthMethodID = "gmail.app_password"
	connection.UsableOperationIDs = []core.Identifier{"gmail.message.send"}
	connection.Constraint = harness.bindings.binding.Constraint
	resume := frozenConfirmationRequest(t, harness, ConfirmationDecisionConfirmed)
	if harness.provider.calls != 0 {
		t.Fatal("send invoked before confirmation")
	}
	result, err := harness.service.InvokeConnector(context.Background(), resume)
	if err != nil || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("confirmed invocation=%#v, %v", result, err)
	}
	lease, err := time.Parse(time.RFC3339Nano, harness.audit.decisions[1].Details["execution_lease_expires_at"].(string))
	if err != nil || !lease.Equal(harness.service.now().Add(12*time.Minute)) || !harness.provider.deadline.Equal(lease) {
		t.Fatalf("lease/provider mismatch: %v / %v", lease, harness.provider.deadline)
	}
}
