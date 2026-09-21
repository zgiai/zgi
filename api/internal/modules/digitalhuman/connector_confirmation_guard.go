package digitalhuman

import (
	"context"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/gorm"
)

// ConnectorConfirmationGuard is the GORM-backed, read-only, fail-closed gate
// that re-reads the durable DigitalHuman run named by a confirmed Connector
// continuation before the Connector Router may resolve credentials or invoke a
// provider. It carries no credential, provider value, or frozen operation
// input, and it never writes.
type ConnectorConfirmationGuard struct {
	db *gorm.DB
}

var _ connectorconfirmation.DigitalHumanContinuationGuard = (*ConnectorConfirmationGuard)(nil)

// NewConnectorConfirmationGuard builds the guard over the same database that
// owns digital_human_runs. A nil database fails construction so production
// wiring cannot silently start without the gate.
func NewConnectorConfirmationGuard(db *gorm.DB) (*ConnectorConfirmationGuard, error) {
	if db == nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return &ConnectorConfirmationGuard{db: db}, nil
}

// AuthorizeContinuation verifies that the exact durable run sealed into the
// continuation is still the waiting DigitalHuman host for this receipt. Every
// predicate is server-restored identity: tenant, owner, host, published
// version, conversation, message, waiting status, waiting reason, and the
// stored receipt. A missing, terminal, canceled, reassigned, or otherwise
// mismatched run fails closed with a sanitized conflict. A durable read error
// fails closed as internal. The query uses bound parameters only and never
// interpolates a value into SQL text.
//
// In-flight semantic: this is a point-in-time read. It denies a run that is
// already canceled when the confirmation arrives, but it cannot stop a
// cancellation requested after this read while the approved operation is
// already in flight. That race is governed by the existing run cancellation
// semantics; this guard must not hold a transaction open across provider
// access to close it.
func (guard *ConnectorConfirmationGuard) AuthorizeContinuation(
	ctx context.Context,
	request connectorconfirmation.DigitalHumanContinuationGuardRequest,
) error {
	if guard == nil || guard.db == nil || ctx == nil || !validConnectorConfirmationGuardRequest(request) {
		return connectorConfirmationGuardConflict()
	}
	receipt, err := uuid.Parse(string(request.RouteReceipt))
	if err != nil || receipt == uuid.Nil {
		return connectorConfirmationGuardConflict()
	}
	var matched int64
	queryErr := guard.db.WithContext(ctx).
		Model(&Run{}).
		Where(
			"id = ? AND organization_id = ? AND actor_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND conversation_id = ? AND message_id = ? AND status = ? AND waiting_reason = ? AND waiting_receipt = ? AND cancel_requested_at IS NULL",
			request.RunID, request.OrganizationID, request.OwnerAccountID,
			request.DigitalHumanID, request.DigitalHumanVersionID,
			request.ConversationID, request.MessageID,
			RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, receipt,
		).
		Count(&matched).Error
	if queryErr != nil {
		return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	if matched != 1 {
		return connectorConfirmationGuardConflict()
	}
	return nil
}

func connectorConfirmationGuardConflict() error {
	return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
}

func validConnectorConfirmationGuardRequest(
	request connectorconfirmation.DigitalHumanContinuationGuardRequest,
) bool {
	if request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.DigitalHumanID == uuid.Nil || request.DigitalHumanVersionID == uuid.Nil ||
		request.RunID == uuid.Nil || request.ConversationID == uuid.Nil || request.MessageID == uuid.Nil {
		return false
	}
	receipt, err := uuid.Parse(string(request.RouteReceipt))
	return err == nil && receipt != uuid.Nil && receipt.String() == string(request.RouteReceipt)
}
