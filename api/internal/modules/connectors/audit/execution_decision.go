package audit

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ExecutionDecisionRequest contains the stable, non-secret identity used to
// build a Connector operation decision. Risk and confirmation are runtime
// facts supplied by trusted platform code; this contract contains no member
// policy or setup-time override.
type ExecutionDecisionRequest struct {
	ID                      uuid.UUID
	OrganizationID          uuid.UUID
	OwnerAccountID          uuid.UUID
	ActingPrincipalID       uuid.UUID
	ConnectorID             core.Identifier
	ConnectionID            uuid.UUID
	Target                  core.TargetRef
	OperationID             core.Identifier
	LogicalOperationID      string
	CorrelationID           string
	ReceiptID               string
	ExecutionPath           string
	RiskClassification      string
	Interactive             bool
	Confirmation            bool
	Preauthorized           bool
	CreatedAt               time.Time
	ExecutionLeaseExpiresAt time.Time
}

// NewExecutionDecision constructs the only pre-submission Connector operation
// decisions. Submitted means a provider call is about to be attempted;
// blocked and cancelled mean no provider call and therefore no outcome row.
func NewExecutionDecision(request ExecutionDecisionRequest, status core.DecisionStatus) (core.LifecycleDecision, error) {
	if !validExecutionDecisionRequest(request, status == core.DecisionStatusDenied) {
		return core.LifecycleDecision{}, invalidInputError()
	}
	decisionName := ""
	switch status {
	case core.DecisionStatusDenied:
		decisionName = "authorization"
		if request.Confirmation {
			return core.LifecycleDecision{}, invalidInputError()
		}
	case core.DecisionStatusSubmitted:
		decisionName = "dispatch"
		if (request.RiskClassification != "trusted_low" && !request.Confirmation && !request.Preauthorized) ||
			request.ExecutionLeaseExpiresAt.IsZero() ||
			!request.ExecutionLeaseExpiresAt.After(request.CreatedAt) {
			return core.LifecycleDecision{}, invalidInputError()
		}
	case core.DecisionStatusBlocked:
		decisionName = "confirmation"
		if request.Interactive || request.Confirmation || request.RiskClassification == "trusted_low" {
			return core.LifecycleDecision{}, invalidInputError()
		}
	case core.DecisionStatusCancelled:
		decisionName = "confirmation"
		if !request.Interactive || request.Confirmation || request.RiskClassification == "trusted_low" {
			return core.LifecycleDecision{}, invalidInputError()
		}
	default:
		return core.LifecycleDecision{}, invalidInputError()
	}
	if status != core.DecisionStatusSubmitted && !request.ExecutionLeaseExpiresAt.IsZero() {
		return core.LifecycleDecision{}, invalidInputError()
	}

	target := request.Target
	decision := core.LifecycleDecision{
		ID: request.ID, OrganizationID: request.OrganizationID,
		OwnerAccountID: request.OwnerAccountID, ActingPrincipalID: request.ActingPrincipalID,
		ConnectorID: request.ConnectorID, ConnectionID: request.ConnectionID,
		Target: &target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, Decision: decisionName,
		Status: status, CorrelationID: request.CorrelationID, ReceiptID: request.ReceiptID,
		Details: map[string]any{
			"execution_path":      request.ExecutionPath,
			"risk_classification": request.RiskClassification,
			"interactive":         request.Interactive,
			"confirmation":        request.Confirmation,
			"preauthorized":       request.Preauthorized,
		},
		CreatedAt: request.CreatedAt.UTC(),
	}
	if status == core.DecisionStatusSubmitted {
		decision.Details["execution_lease_expires_at"] = request.ExecutionLeaseExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if err := decision.Validate(); err != nil {
		return core.LifecycleDecision{}, invalidInputError()
	}
	return decision, nil
}

// NewConfirmationFinalizationDecision builds the immutable marker used by the
// repository's route-receipt CAS. Confirm and cancel intentionally share the
// same decision identity and status; only one can finalize a prompt.
func NewConfirmationFinalizationDecision(request ExecutionDecisionRequest, confirmed bool) (core.LifecycleDecision, error) {
	if !validExecutionDecisionRequest(request, false) || !request.Interactive || request.RiskClassification == "trusted_low" {
		return core.LifecycleDecision{}, invalidInputError()
	}
	target := request.Target
	decision := core.LifecycleDecision{
		ID: request.ID, OrganizationID: request.OrganizationID,
		OwnerAccountID: request.OwnerAccountID, ActingPrincipalID: request.ActingPrincipalID,
		ConnectorID: request.ConnectorID, ConnectionID: request.ConnectionID,
		Target: &target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, Decision: "confirmation_finalization",
		Status: core.DecisionStatusSuccessful, CorrelationID: request.CorrelationID, ReceiptID: request.ReceiptID,
		Details: map[string]any{
			"execution_path":      request.ExecutionPath,
			"risk_classification": request.RiskClassification,
			"interactive":         true,
			"confirmation":        confirmed,
		},
		CreatedAt: request.CreatedAt.UTC(),
	}
	if err := decision.Validate(); err != nil {
		return core.LifecycleDecision{}, invalidInputError()
	}
	return decision, nil
}

func validConfirmationFinalization(decision core.LifecycleDecision) bool {
	if decision.Validate() != nil || decision.Decision != "confirmation_finalization" ||
		decision.Status != core.DecisionStatusSuccessful || strings.TrimSpace(decision.ReceiptID) == "" {
		return false
	}
	_, confirmationOK := decision.Details["confirmation"].(bool)
	interactive, interactiveOK := decision.Details["interactive"].(bool)
	if !confirmationOK || !interactiveOK || !interactive {
		return false
	}
	risk, riskOK := decision.Details["risk_classification"].(string)
	path, pathOK := decision.Details["execution_path"].(string)
	return riskOK && risk != "trusted_low" && (risk == "high" || risk == "unclassified") &&
		pathOK && path == "connector_native"
}

func validConfirmationFinalizationPair(
	finalization core.LifecycleDecision,
	operationDecision core.LifecycleDecision,
) bool {
	if !validConfirmationFinalization(finalization) || operationDecision.Validate() != nil ||
		finalization.ID == operationDecision.ID || finalization.Target == nil || operationDecision.Target == nil ||
		*finalization.Target != *operationDecision.Target ||
		finalization.OrganizationID != operationDecision.OrganizationID ||
		finalization.OwnerAccountID != operationDecision.OwnerAccountID ||
		finalization.ActingPrincipalID != operationDecision.ActingPrincipalID ||
		finalization.ConnectorID != operationDecision.ConnectorID ||
		finalization.ConnectionID != operationDecision.ConnectionID ||
		finalization.OperationID != operationDecision.OperationID ||
		finalization.LogicalOperationID != operationDecision.LogicalOperationID ||
		finalization.CorrelationID != operationDecision.CorrelationID ||
		finalization.ReceiptID != operationDecision.ReceiptID ||
		!finalization.CreatedAt.Equal(operationDecision.CreatedAt) {
		return false
	}
	confirmed, confirmedOK := finalization.Details["confirmation"].(bool)
	operationConfirmed, operationConfirmedOK := operationDecision.Details["confirmation"].(bool)
	finalRisk, finalRiskOK := finalization.Details["risk_classification"].(string)
	operationRisk, operationRiskOK := operationDecision.Details["risk_classification"].(string)
	finalPath, finalPathOK := finalization.Details["execution_path"].(string)
	operationPath, operationPathOK := operationDecision.Details["execution_path"].(string)
	interactive, interactiveOK := operationDecision.Details["interactive"].(bool)
	if !confirmedOK || !operationConfirmedOK || !finalRiskOK || !operationRiskOK ||
		!finalPathOK || !operationPathOK || !interactiveOK || !interactive ||
		finalRisk != operationRisk || finalPath != operationPath {
		return false
	}
	if confirmed {
		lease, leaseOK := operationDecision.Details["execution_lease_expires_at"].(string)
		leaseExpiresAt, leaseErr := time.Parse(time.RFC3339Nano, lease)
		return operationConfirmed && operationDecision.Decision == "dispatch" &&
			operationDecision.Status == core.DecisionStatusSubmitted && leaseOK && leaseErr == nil &&
			leaseExpiresAt.After(operationDecision.CreatedAt)
	}
	_, hasLease := operationDecision.Details["execution_lease_expires_at"]
	return !operationConfirmed && !hasLease && operationDecision.Decision == "confirmation" &&
		operationDecision.Status == core.DecisionStatusCancelled
}

func validExecutionDecisionRequest(request ExecutionDecisionRequest, denied bool) bool {
	if request.ID == uuid.Nil || request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.ActingPrincipalID == uuid.Nil || (!denied && request.ActingPrincipalID != request.OwnerAccountID) || !request.ConnectorID.Valid() ||
		request.ConnectionID == uuid.Nil || request.Target.Validate() != nil || !request.OperationID.Valid() ||
		request.CreatedAt.IsZero() || !validDecisionText(request.LogicalOperationID, 128) ||
		!validDecisionText(request.CorrelationID, 128) || !validDecisionText(request.ReceiptID, 128) {
		return false
	}
	if request.ExecutionPath != "connector_native" {
		return false
	}
	switch request.RiskClassification {
	case "trusted_low", "high", "unclassified":
		return true
	default:
		return false
	}
}

func validDecisionText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum &&
		!strings.ContainsAny(value, "\x00\r\n")
}
