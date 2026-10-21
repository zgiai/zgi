package core

import (
	"github.com/google/uuid"
	"testing"
)

func TestOnlyDeniedLifecycleDecisionCanRecordRealNonOwnerActor(t *testing.T) {
	decision := LifecycleDecision{ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ActingPrincipalID: uuid.New(),
		ConnectorID: "github", Decision: "authorization", Status: DecisionStatusDenied, CorrelationID: "denied-owner-check", Details: map[string]any{}}
	if err := decision.Validate(); err != nil {
		t.Fatal("denied decision lost actual actor identity", err)
	}
	for _, status := range []DecisionStatus{DecisionStatusSubmitted, DecisionStatusBlocked, DecisionStatusCancelled, DecisionStatusFailed, DecisionStatusSuccessful, DecisionStatusUnknown} {
		decision.Status = status
		if decision.Validate() == nil {
			t.Fatalf("non-owner accepted with status %s", status)
		}
	}
	decision.Status = DecisionStatusDenied
	decision.ActingPrincipalID = uuid.Nil
	if decision.Validate() == nil {
		t.Fatal("denied decision accepted an absent actor")
	}
}
