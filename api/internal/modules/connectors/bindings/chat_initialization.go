package bindings

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const MaxInitialChatConnections = 64

type InitialChatConnection struct {
	ConnectorID  core.Identifier
	ConnectionID uuid.UUID
}

func ValidateInitialChatConnections(selections []InitialChatConnection) error {
	if len(selections) > MaxInitialChatConnections {
		return invalidInputError("initialize chat connections")
	}
	seen := make(map[core.Identifier]bool, len(selections))
	for _, selection := range selections {
		if !selection.ConnectorID.Valid() || selection.ConnectionID == uuid.Nil || seen[selection.ConnectorID] {
			return invalidInputError("initialize chat connections")
		}
		seen[selection.ConnectorID] = true
	}
	return nil
}

// InitializeChatConnections runs inside the authenticated Chat host's new-
// conversation transaction. The caller owns creation of that host; an existing
// conversation must use Service.UpsertBinding and its edit/revision checks.
// Any failed selection rolls back the conversation, bindings and lifecycle audit.
func InitializeChatConnections(ctx context.Context, tx *gorm.DB, org, owner, conversationID uuid.UUID, selections []InitialChatConnection) error {
	if tx == nil || org == uuid.Nil || owner == uuid.Nil || conversationID == uuid.Nil {
		return invalidInputError("initialize chat connections")
	}
	if err := ValidateInitialChatConnections(selections); err != nil {
		return err
	}
	ordered := append([]InitialChatConnection(nil), selections...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ConnectionID.String() < ordered[j].ConnectionID.String() })
	repository := NewGormRepository(tx.Session(&gorm.Session{DisableNestedTransaction: true}))
	for _, selection := range ordered {
		target := core.TargetRef{Type: core.TargetTypeChat, ID: conversationID.String()}
		now := time.Now().UTC()
		candidate := core.TargetBinding{
			ID:             stableNativeBindingID(org, owner, selection.ConnectorID, target),
			OrganizationID: org, OwnerAccountID: owner, ConnectionID: selection.ConnectionID,
			ConnectorID: selection.ConnectorID, Target: target, Status: core.BindingStatusActive,

			Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		// Lock in the same connection-ID order as maintenance and other host saves.
		if _, err := lockBindingLifecycleAuthorities(ctx, tx, candidate); err != nil {
			return err
		}
		var connection struct {
			Constraint datatypes.JSON `gorm:"column:connection_constraint"`
			Operations datatypes.JSON `gorm:"column:usable_operation_ids"`
		}
		err := tx.WithContext(ctx).Table("connector_connections").Select("connection_constraint, usable_operation_ids").
			Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND state = ? AND deleted_at IS NULL",
				selection.ConnectionID, org, owner, selection.ConnectorID, core.ReadinessReady).Take(&connection).Error
		if err != nil {
			return connectionNotReadyError("initialize chat connections")
		}
		var operations []core.Identifier
		if json.Unmarshal(connection.Operations, &operations) != nil || len(operations) == 0 {
			return connectionNotReadyError("initialize chat connections")
		}
		constraint, err := core.DecodeConstraint(connection.Constraint)
		if err != nil {
			return err
		}
		candidate.Constraint, err = core.IntersectConstraints(core.OwnerConstraint(owner, operations), constraint)
		if err != nil {
			return err
		}
		_, _, err = repository.UpsertBindingWithLifecycle(ctx, org, owner, candidate, 0, BindingLifecycleMutation{
			ID: uuid.New(), MutationType: MutationBindingUpserted,
			CorrelationID: "chat-initial:" + candidate.ID.String(), CreatedAt: now,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
