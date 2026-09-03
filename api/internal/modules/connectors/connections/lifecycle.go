package connections

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MutationConnectionRenamed = "connection_renamed"
	MutationCredentialRotated = "credential_rotated"
	MutationReadinessChanged  = "readiness_changed"
	MutationConnectionRemoved = "connection_removed"

	DecisionConnectionReconnected = "connection_reconnected"
	DecisionConnectionDiagnosed   = "connection_diagnosed"

	connectionLifecycleSuccessful = "successful"
)

// ConnectionChange records safe facts for a native maintenance event.
type ConnectionChange struct {
	Type              string
	Name              string
	Readiness         connectorcore.Readiness
	CredentialVersion int
}

// ConnectionLifecycleMutation binds the canonical write to its audit event.
type ConnectionLifecycleMutation struct {
	ID            uuid.UUID
	Decision      string
	CorrelationID string
	CreatedAt     time.Time
	Changes       []ConnectionChange
}

// MaintenanceLifecycleRepository commits the canonical write and audit together.
type MaintenanceLifecycleRepository interface {
	UpdateConnectionWithLifecycle(context.Context, uuid.UUID, uuid.UUID, connectorcore.PersonalConnection, int, ConnectionLifecycleMutation) (connectorcore.PersonalConnection, error)
	CompleteReconnectReadyWithLifecycle(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, ReconnectCompletion, ConnectionLifecycleMutation) (connectorcore.KeySetupAttempt, error)
	ForceRemoveConnectionWithLifecycle(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, []BindingRevisionSnapshot, ConnectionLifecycleMutation) error
}

type connectionLifecycleDecisionRecord struct {
	ID                 uuid.UUID      `gorm:"column:id"`
	OrganizationID     uuid.UUID      `gorm:"column:organization_id"`
	OwnerAccountID     uuid.UUID      `gorm:"column:owner_account_id"`
	ActingPrincipalID  uuid.UUID      `gorm:"column:acting_principal_id"`
	ConnectorID        string         `gorm:"column:connector_id"`
	ConnectionID       uuid.UUID      `gorm:"column:connection_id"`
	TargetType         *string        `gorm:"column:target_type"`
	TargetID           *string        `gorm:"column:target_id"`
	BindingInstanceID  *string        `gorm:"column:binding_instance_id"`
	OperationID        *string        `gorm:"column:operation_id"`
	LogicalOperationID *string        `gorm:"column:logical_operation_id"`
	Decision           string         `gorm:"column:decision"`
	Status             string         `gorm:"column:status"`
	CorrelationID      string         `gorm:"column:correlation_id"`
	ReceiptID          *string        `gorm:"column:receipt_id"`
	Details            datatypes.JSON `gorm:"column:details"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
}

func (connectionLifecycleDecisionRecord) TableName() string {
	return "connector_lifecycle_decisions"
}

var _ MaintenanceLifecycleRepository = (*GormRepository)(nil)

func (repository *GormRepository) UpdateConnectionWithLifecycle(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	connection connectorcore.PersonalConnection,
	expectedRevision int,
	mutation ConnectionLifecycleMutation,
) (connectorcore.PersonalConnection, error) {
	if err := repository.available(); err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	if !validConnectionLifecycleMutation(mutation) {
		return connectorcore.PersonalConnection{}, invalidInputError("update connector connection lifecycle")
	}
	var stored connectorcore.PersonalConnection
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nested := &GormRepository{db: tx.Session(&gorm.Session{DisableNestedTransaction: true})}
		var writeErr error
		stored, writeErr = nested.UpdateConnection(ctx, organizationID, ownerAccountID, connection, expectedRevision)
		if writeErr != nil {
			return writeErr
		}
		if err := writeProviderEvidence(ctx, tx, stored); err != nil {
			return err
		}
		return appendConnectionLifecycleMutation(ctx, tx, stored, mutation)
	})
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	return stored, nil
}

func (repository *GormRepository) CompleteReconnectReadyWithLifecycle(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
	completion ReconnectCompletion,
	mutation ConnectionLifecycleMutation,
) (connectorcore.KeySetupAttempt, error) {
	if err := repository.available(); err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	if !validConnectionLifecycleMutation(mutation) || mutation.Decision != DecisionConnectionReconnected {
		return connectorcore.KeySetupAttempt{}, invalidInputError("complete connector reconnect lifecycle")
	}
	var completed connectorcore.KeySetupAttempt
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nested := &GormRepository{db: tx.Session(&gorm.Session{DisableNestedTransaction: true})}
		var writeErr error
		completed, writeErr = nested.CompleteReconnectReady(ctx, organizationID, ownerAccountID, attemptID, completion)
		if writeErr != nil {
			return writeErr
		}
		stored := completion.Result
		if completion.ReplaceExisting {
			stored.Revision = completion.ExpectedRevision + 1
		}
		return appendConnectionLifecycleMutation(ctx, tx, stored, mutation)
	})
	if err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	return completed, nil
}

func (repository *GormRepository) ForceRemoveConnectionWithLifecycle(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	expectedRevision int,
	expectedBindings []BindingRevisionSnapshot,
	mutation ConnectionLifecycleMutation,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if !validConnectionLifecycleMutation(mutation) || mutation.Decision != MutationConnectionRemoved {
		return invalidInputError("remove connector connection lifecycle")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := getConnectionRecord(
			ctx, tx, organizationID, ownerAccountID, connectionID,
		)
		if err != nil {
			return err
		}
		connection, err := record.toDomain()
		if err != nil {
			return err
		}
		if mutation.Decision == MutationConnectionRemoved {
			if err := cancelAppOnboarding(ctx, tx, connection); err != nil {
				return err
			}
		}
		nested := &GormRepository{db: tx.Session(&gorm.Session{DisableNestedTransaction: true})}
		if err := nested.ForceRemoveConnection(
			ctx, organizationID, ownerAccountID, connectionID, expectedRevision, expectedBindings,
		); err != nil {
			return err
		}
		removedAt := mutation.CreatedAt.UTC()
		connection.State = connectorcore.ReadinessUnavailable
		connection.ProviderEvidence = nil
		if err := writeProviderEvidence(ctx, tx, connection); err != nil {
			return err
		}
		connection.Revision = expectedRevision + 1
		connection.DeletedAt = &removedAt
		connection.UpdatedAt = removedAt
		return appendConnectionLifecycleMutation(ctx, tx, connection, mutation)
	})
}

func appendConnectionLifecycleMutation(
	ctx context.Context,
	tx *gorm.DB,
	connection connectorcore.PersonalConnection,
	mutation ConnectionLifecycleMutation,
) error {
	if tx == nil || connection.Validate() != nil || !validConnectionLifecycleMutation(mutation) {
		return invalidInputError("record connector connection lifecycle")
	}
	detailsJSON, err := json.Marshal(connectionLifecycleDetails(connection))
	if err != nil {
		return internalError("record connector connection lifecycle")
	}
	decision := connectionLifecycleDecisionRecord{
		ID: mutation.ID, OrganizationID: connection.OrganizationID, OwnerAccountID: connection.OwnerAccountID,
		ActingPrincipalID: connection.OwnerAccountID, ConnectorID: connection.ConnectorID.String(), ConnectionID: connection.ID,
		Decision: mutation.Decision, Status: connectionLifecycleSuccessful, CorrelationID: mutation.CorrelationID,
		Details: datatypes.JSON(detailsJSON), CreatedAt: mutation.CreatedAt.UTC(),
	}
	if err := createOrVerifyConnectionLifecycleDecision(ctx, tx, decision); err != nil {
		return err
	}
	return nil
}

func createOrVerifyConnectionLifecycleDecision(
	ctx context.Context,
	tx *gorm.DB,
	decision connectionLifecycleDecisionRecord,
) error {
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&decision)
	if result.Error != nil {
		return internalError("record connector connection lifecycle")
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing connectionLifecycleDecisionRecord
	err := tx.WithContext(ctx).Where(
		"organization_id = ? AND correlation_id = ? AND decision = ? AND status = ?",
		decision.OrganizationID, decision.CorrelationID, decision.Decision, decision.Status,
	).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !sameConnectionLifecycleDecision(existing, decision) {
		return conflictError("replay connector connection lifecycle")
	}
	if err != nil {
		return internalError("verify connector connection lifecycle replay")
	}
	return nil
}

func sameConnectionLifecycleDecision(left, right connectionLifecycleDecisionRecord) bool {
	left.ID, right.ID = uuid.Nil, uuid.Nil
	leftCreatedAt, rightCreatedAt := left.CreatedAt.UTC().Truncate(time.Microsecond), right.CreatedAt.UTC().Truncate(time.Microsecond)
	left.CreatedAt, right.CreatedAt = time.Time{}, time.Time{}
	leftDetails, rightDetails := left.Details, right.Details
	left.Details, right.Details = nil, nil
	return leftCreatedAt.Equal(rightCreatedAt) && reflect.DeepEqual(left, right) && equivalentLifecycleJSON(leftDetails, rightDetails)
}

func equivalentLifecycleJSON(left, right datatypes.JSON) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func validConnectionLifecycleMutation(mutation ConnectionLifecycleMutation) bool {
	correlationID := strings.TrimSpace(mutation.CorrelationID)
	if mutation.ID == uuid.Nil || strings.TrimSpace(mutation.Decision) == "" || mutation.Decision != strings.TrimSpace(mutation.Decision) ||
		correlationID == "" || correlationID != mutation.CorrelationID || len(correlationID) > 128 || mutation.CreatedAt.IsZero() {
		return false
	}
	for _, change := range mutation.Changes {
		switch change.Type {
		case MutationConnectionRenamed, MutationCredentialRotated, MutationReadinessChanged, MutationConnectionRemoved:
		default:
			return false
		}
	}
	return true
}

func connectionLifecycleDetails(connection connectorcore.PersonalConnection) map[string]any {
	details := map[string]any{
		"connection_id": connection.ID.String(), "name": connection.Name, "state": string(connection.State),
		"credential_version": connection.CredentialVersion, "revision": connection.Revision,
	}
	if connection.DeletedAt != nil {
		details["removed_at"] = connection.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	return details
}

func newConnectionLifecycleMutation(
	decision, correlationID string,
	createdAt time.Time,
	change ...ConnectionChange,
) ConnectionLifecycleMutation {
	return ConnectionLifecycleMutation{
		ID:            uuid.NewSHA1(uuid.NameSpaceOID, []byte(decision+"\x00"+correlationID)),
		Decision:      decision,
		CorrelationID: correlationID,
		CreatedAt:     createdAt.UTC(),
		Changes:       append([]ConnectionChange(nil), change...),
	}
}
