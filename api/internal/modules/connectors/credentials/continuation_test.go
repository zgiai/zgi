package credentials

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

func TestContinuationRetainsFrozenSkillIdentityWithoutCatalogAlias(t *testing.T) {
	store, err := NewStore("native", map[string]string{"native": strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	protector, err := NewContinuationPayloadProtector(store)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	definition := definitions[0]
	for i := range definition.Operations {
		definition.Operations[i].SkillID = "external-apps"
	}
	lifecycle, err := catalog.NewPluginLifecycle([]catalog.Definition{definition}, nil)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := lifecycle.Pin(definition.ID, definition.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	org, owner, receipt := uuid.New(), uuid.New(), execution.RouteClaimReceipt(uuid.NewString())
	frozen := execution.FrozenConfirmationRequest{RouteReceipt: receipt, Request: execution.ExecutionRequest{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier(definition.ID), OperationID: core.Identifier(accepted.Operation.ID), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, Input: map[string]any{"body": "PRIVATE-FROZEN-INPUT"}}, AcceptedOperation: accepted}
	envelope, err := protector.Seal(t.Context(), org, receipt, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(envelope), "PRIVATE-FROZEN-INPUT") {
		t.Fatal("plaintext escaped envelope")
	}
	// Opening a continuation uses its authenticated accepted snapshot only.
	// The current catalog and its hidden Skill do not participate in decoding.
	for i := range definition.Operations {
		definition.Operations[i].SkillID = "connector-runtime"
	}
	opened, err := protector.Open(t.Context(), org, receipt, envelope)
	if err != nil || opened.AcceptedOperation.Operation.SkillID != "external-apps" || opened.AcceptedOperation.ContractDigest != accepted.ContractDigest || opened.Request.Input.(map[string]any)["body"] != "PRIVATE-FROZEN-INPUT" {
		t.Fatal("accepted snapshot changed during restore", err)
	}
	for _, subject := range []struct {
		org     uuid.UUID
		receipt execution.RouteClaimReceipt
	}{{uuid.New(), receipt}, {org, execution.RouteClaimReceipt(uuid.NewString())}} {
		if _, err := protector.Open(t.Context(), subject.org, subject.receipt, envelope); err == nil {
			t.Fatal("cross-owner/receipt envelope opened")
		}
	}
}
