package audit_test

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
	"time"
)

func TestExecutionDeniedDecisionPreservesActorAndCannotBecomeSubmission(t *testing.T) {
	request := audit.ExecutionDecisionRequest{ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ActingPrincipalID: uuid.New(),
		ConnectorID: "github", ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"}, OperationID: "github.list_repositories",
		LogicalOperationID: "logical-1", CorrelationID: "correlation-1", ReceiptID: uuid.NewString(), ExecutionPath: "connector_native", RiskClassification: "unclassified", Interactive: true, CreatedAt: time.Now().UTC()}
	decision, err := audit.NewExecutionDecision(request, core.DecisionStatusDenied)
	if err != nil || decision.ActingPrincipalID != request.ActingPrincipalID || decision.Status != core.DecisionStatusDenied {
		t.Fatalf("denied=%#v err=%v", decision, err)
	}
	request.Confirmation = true
	request.ExecutionLeaseExpiresAt = request.CreatedAt.Add(time.Minute)
	if _, err := audit.NewExecutionDecision(request, core.DecisionStatusSubmitted); err == nil {
		t.Fatal("foreign actor submitted")
	}
	if _, err := audit.NewConfirmationFinalizationDecision(request, true); err == nil {
		t.Fatal("foreign actor finalized confirmation")
	}
}
