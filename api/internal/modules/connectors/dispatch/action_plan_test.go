package dispatch

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestActionPlanStepsSurviveModelRenamingAndSeparateRepeatedIntent(t *testing.T) {
	identity := Identity{ConnectorID: "github", OperationID: "github.issue.create"}
	refs := []PlanPhaseRef{{"first", identity.ConnectorID, identity.OperationID}, {"second", identity.ConnectorID, identity.OperationID}}
	ledger := ActionLedger{}
	first, created, err := ResolveActionPlanStep(&ledger, refs, "first", identity)
	if err != nil || !created {
		t.Fatal(err)
	}
	second, created, err := ResolveActionPlanStep(&ledger, refs, "second", identity)
	if err != nil || created || second == first {
		t.Fatal("independent accepted steps share identity", err)
	}
	encoded, _ := json.Marshal(ledger)
	restored := ActionLedger{}
	if json.Unmarshal(encoded, &restored) != nil {
		t.Fatal("restore ledger")
	}
	refs[0].ID, refs[1].ID = "renamed-first", "renamed-second"
	retry, created, err := ResolveActionPlanStep(&restored, refs, "renamed-first", identity)
	if err != nil || created || retry != first {
		t.Fatal("model rename minted a new identity", err)
	}
	refs = append(refs, PlanPhaseRef{"invented-third", identity.ConnectorID, identity.OperationID})
	if _, _, err := ResolveActionPlanStep(&restored, refs, "invented-third", identity); err == nil {
		t.Fatal("model expanded a committed plan")
	}
	if _, _, err := ResolveActionPlanStep(&restored, nil, "", identity); err == nil {
		t.Fatal("removing the plan bypassed its committed identity")
	}
}

func TestCommittedActionPlanStepCannotChangeInput(t *testing.T) {
	owner := uuid.New()
	identity := Identity{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", OperationID: "github.issue.create", Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}}
	identity.LogicalOperationID, identity.CorrelationID = "lookup", "correlation"
	ledger := ActionLedger{}
	refs := []PlanPhaseRef{{"first", identity.ConnectorID, identity.OperationID}, {"second", identity.ConnectorID, identity.OperationID}}
	step, _, err := ResolveActionPlanStep(&ledger, refs, "first", identity)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := ResolveActionInstance(&ledger, step, "correlation", identity, map[string]any{"title": "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveActionInstance(&ledger, step, "correlation", identity, map[string]any{"title": "changed"}); err == nil {
		t.Fatal("changed input minted a second action for the same step")
	}
	retry, created, err := ResolveActionInstance(&ledger, step, "correlation", identity, map[string]any{"title": "first"})
	if err != nil || created || retry.LogicalOperationID != first.LogicalOperationID {
		t.Fatal("original action did not survive", err)
	}
	secondStep, _, err := ResolveActionPlanStep(&ledger, refs, "second", identity)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := ResolveActionInstance(&ledger, secondStep, "correlation", identity, map[string]any{"title": "first"})
	if err != nil || second.LogicalOperationID == first.LogicalOperationID {
		t.Fatal("explicit second step cannot execute independently", err)
	}
}
