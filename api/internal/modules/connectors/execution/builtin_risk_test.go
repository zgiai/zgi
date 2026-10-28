package execution

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// builtinRiskTestResolver builds the production platform policy set against the
// complete admitted built-in catalog.
func builtinRiskTestResolver(t *testing.T) *TrustedRiskResolver {
	t.Helper()
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	definitionCatalog, err := catalog.New(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTrustedRiskResolver(definitionCatalog, BuiltinRiskPolicies())
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func resolveBuiltinRisk(
	t *testing.T,
	resolver *TrustedRiskResolver,
	connector string,
	operation core.Identifier,
	target core.TargetRef,
) RiskClassification {
	t.Helper()
	owner := uuid.New()
	risk, err := resolver.ResolveRisk(context.Background(), RiskRequest{
		OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner,
		ConnectionID: uuid.New(), ConnectorID: core.Identifier(connector),
		OperationID: operation, Target: target, Interactive: true,
	})
	if err != nil {
		t.Fatalf("ResolveRisk(%s/%s) error = %v", connector, operation, err)
	}
	return risk
}

// TestServerOpsSSHOperationsAreHighRiskForEveryDigitalHumanUse pins the
// platform classification of the three read-only and four service write SSH
// Server Operations actions. Read-only is the operation Effect, not a risk
// claim: the platform classifies all seven RiskHigh so the DigitalHuman surface
// always requires a per-call approval. The policy identity is the complete stable leaf identity, and the
// tool name is checked against the admitted catalog contract.
func TestServerOpsSSHOperationsAreHighRiskForEveryDigitalHumanUse(t *testing.T) {
	definition := catalog.ServerOpsSSHDefinition()
	policies := serverOpsSSHRiskPolicies()
	if len(policies) != 7 || len(policies) != len(definition.Operations) {
		t.Fatalf("SSH policy count = %d, want the 7 admitted operations", len(policies))
	}
	toolByOperation := map[string]string{}
	for _, operation := range definition.Operations {
		toolByOperation[operation.ID] = operation.ToolName
	}
	seen := map[string]bool{}
	for _, policy := range policies {
		if seen[policy.OperationID.String()] {
			t.Fatalf("duplicate SSH policy for %s", policy.OperationID)
		}
		seen[policy.OperationID.String()] = true
		if policy.ConnectorID.String() != catalog.ServerOpsSSHConnectorID ||
			policy.ProviderID.String() != catalog.ServerOpsSSHProviderID ||
			policy.DriverID.String() != catalog.ServerOpsSSHDriverID ||
			policy.AuthMethodID.String() != catalog.ServerOpsSSHAuthMethodID {
			t.Fatalf("SSH policy identity drifted: %#v", policy)
		}
		if tool, ok := toolByOperation[policy.OperationID.String()]; !ok || tool != policy.ToolName {
			t.Fatalf("SSH policy tool identity = %q, admitted catalog tool = %q", policy.ToolName, tool)
		}
		if policy.Classification != RiskHigh {
			t.Fatalf("SSH operation %s classified %q, want %q", policy.OperationID, policy.Classification, RiskHigh)
		}
	}

	resolver := builtinRiskTestResolver(t)
	for _, operation := range []core.Identifier{
		serverops.ActionSystemSnapshot,
		serverops.ActionServiceStatus,
		serverops.ActionJournalTail,
		serverops.ActionServiceRestart,
		serverops.ActionServiceStart,
		serverops.ActionServiceStop,
		serverops.ActionServiceReload,
	} {
		if risk := resolveBuiltinRisk(t, resolver, catalog.ServerOpsSSHConnectorID, operation, validDigitalHumanTarget()); risk != RiskHigh {
			t.Fatalf("SSH operation %s resolved %q, want %q", operation, risk, RiskHigh)
		}
	}
}

// TestServerOpsSSHUnclassifiedOperationFailsClosed keeps the SSH surface
// fail-closed: only the seven exact operation identities have a policy, so any
// other operation under the SSH connector stays unclassified.
func TestServerOpsSSHUnclassifiedOperationFailsClosed(t *testing.T) {
	resolver := builtinRiskTestResolver(t)
	for _, operation := range []core.Identifier{
		"server.system.snapshot.extra",
		"server.read.arbitrary",
		"server.service.enable",
		"server.service.kill",
		"server.command.run",
		"github.user.get",
	} {
		if risk := resolveBuiltinRisk(t, resolver, catalog.ServerOpsSSHConnectorID, operation, validDigitalHumanTarget()); risk != RiskUnclassified {
			t.Fatalf("SSH operation %q resolved %q, want unclassified", operation, risk)
		}
	}
}

// TestBuiltinRiskPoliciesAddOnlyServerOpsSSHAndKeepExistingClassifications pins
// that the SSH policies are exactly the seven SSH operations and leave every
// pre-existing classification unchanged.
func TestBuiltinRiskPoliciesAddOnlyServerOpsSSHAndKeepExistingClassifications(t *testing.T) {
	policies := BuiltinRiskPolicies()
	// The github/exa static list is the only unexported-count group: it must
	// still contribute exactly its twelve reviewed entries, including the two
	// read-only GitHub MCP operations from the main branch.
	wantExisting := 12 + len(feishuRiskPolicies()) + len(mailRiskPolicies()) + len(messagingRiskPolicies())
	if len(policies) != wantExisting+7 {
		t.Fatalf("builtin policy count = %d, want %d existing + 7 SSH", len(policies), wantExisting)
	}
	sshPolicies := 0
	for _, policy := range policies {
		if policy.ConnectorID.String() == catalog.ServerOpsSSHConnectorID {
			sshPolicies++
		}
	}
	if sshPolicies != 7 {
		t.Fatalf("SSH policies in the builtin set = %d, want 7", sshPolicies)
	}

	resolver := builtinRiskTestResolver(t)
	chat := core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}
	for _, testCase := range []struct {
		connector string
		operation core.Identifier
		want      RiskClassification
	}{
		{"github", "github.user.get", RiskTrustedLow},
		{"github", "github.issue.create", RiskHigh},
		{"web-search", "web.search", RiskTrustedLow},
		{"feishu", "feishu.bot.get", RiskTrustedLow},
		{"feishu", "feishu.message.send", RiskHigh},
		{"gmail", "gmail.message.list", RiskTrustedLow},
		{"gmail", "gmail.message.send", RiskHigh},
		{"qqmail", "qqmail.message.send", RiskHigh},
		{"netease163", "netease163.message.list", RiskTrustedLow},
		{"wecom", "wecom.identity.get", RiskTrustedLow},
		{"wecom", "wecom.messages.send", RiskHigh},
		{"weixin", "weixin.messages.reply", RiskHigh},
	} {
		if risk := resolveBuiltinRisk(t, resolver, testCase.connector, testCase.operation, chat); risk != testCase.want {
			t.Fatalf("%s/%s = %q, want %q", testCase.connector, testCase.operation, risk, testCase.want)
		}
	}
}
