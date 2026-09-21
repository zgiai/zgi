package connections

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

const (
	DecisionKeyCreation  = "key_creation"
	DecisionKeyReconnect = "key_reconnect"
)

// recordKeySetupAudit reuses the authoritative Connector audit repository on
// the caller's transaction. The evidence contains only stable identities and
// terminal non-secret state; candidate values, envelopes, request hashes, and
// provider responses never cross this boundary.
func recordKeySetupAudit(
	ctx context.Context,
	tx *gorm.DB,
	attempt connectorcore.KeySetupAttempt,
	connectionID uuid.UUID,
	now time.Time,
) error {
	if tx == nil || attempt.Validate() != nil || now.IsZero() {
		return invalidInputError("record connector key setup lifecycle")
	}
	status := connectorcore.DecisionStatusFailed
	switch attempt.State {
	case connectorcore.SetupStateReady:
		if connectionID == uuid.Nil || attempt.ResultConnectionID != connectionID {
			return invalidInputError("record connector key setup lifecycle")
		}
		status = connectorcore.DecisionStatusSuccessful
	case connectorcore.SetupStateFailed:
		if connectionID != uuid.Nil {
			return invalidInputError("record connector key setup lifecycle")
		}
	case connectorcore.SetupStateCancelled:
		if connectionID != uuid.Nil {
			return invalidInputError("record connector key setup lifecycle")
		}
		status = connectorcore.DecisionStatusCancelled
	default:
		return invalidInputError("record connector key setup lifecycle")
	}
	correlationID := "key-setup:" + attempt.ID.String()
	details := map[string]any{
		"setup_attempt_id": attempt.ID.String(),
		"setup_state":      string(attempt.State),
	}
	if attempt.ResultConnectionID != uuid.Nil {
		details["result_connection_id"] = attempt.ResultConnectionID.String()
	}
	if attempt.ErrorCode != "" {
		details["error_code"] = string(attempt.ErrorCode)
	}
	decisionName := DecisionKeyCreation
	auditConnectionID := connectionID
	if attempt.ConnectionID != uuid.Nil {
		decisionName = DecisionKeyReconnect
		auditConnectionID = attempt.ConnectionID
		details["existing_connection_id"] = attempt.ConnectionID.String()
	}
	decision := connectorcore.LifecycleDecision{
		ID:             uuid.NewSHA1(uuid.NameSpaceOID, []byte(decisionName+"\x00"+correlationID+"\x00"+string(status))),
		OrganizationID: attempt.OrganizationID, OwnerAccountID: attempt.OwnerAccountID,
		ActingPrincipalID: attempt.OwnerAccountID, ConnectorID: attempt.ConnectorID,
		ConnectionID: auditConnectionID, Target: cloneTarget(attempt.Target),
		Decision: decisionName, Status: status, CorrelationID: correlationID,
		Details: details, CreatedAt: now.UTC(),
	}
	if err := connectoraudit.NewGormRepository(tx).CreateDecision(ctx, decision); err != nil {
		return fmt.Errorf("record connector key setup lifecycle: %w", err)
	}
	return nil
}
