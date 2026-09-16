package runtimeauth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// InitialChatConnectionValidator is a narrow first-turn transaction port, not a
// default lookup or an alternate execution authority. The host has just created
// its own conversation and bindings and already holds lifecycle connection locks.
type InitialChatConnectionValidator interface {
	ValidateInitialChatConnections(context.Context, *gorm.DB, ChatCapabilityRequest, []bindings.InitialChatConnection) error
}

var _ InitialChatConnectionValidator = (*ChatResolver)(nil)

type initialChatConnectionRecord struct {
	ConnectionID          uuid.UUID      `gorm:"column:connection_id"`
	ConnectorID           string         `gorm:"column:connector_id"`
	BindingConstraint     datatypes.JSON `gorm:"column:binding_constraint"`
	ConnectionConstraint  datatypes.JSON `gorm:"column:connection_constraint"`
	State                 core.Readiness `gorm:"column:state"`
	DriverID              string         `gorm:"column:driver_id"`
	AuthMethodID          string         `gorm:"column:auth_method_id"`
	UsableOperationIDs    datatypes.JSON `gorm:"column:usable_operation_ids"`
	UnavailableOperations datatypes.JSON `gorm:"column:unavailable_operations"`
}

// ValidateInitialChatConnections reads only safe metadata from the same
// transaction as initialization. It fails closed if any explicitly selected
// connection no longer has a permitted operation in the current plugin catalog.
func (resolver *ChatResolver) ValidateInitialChatConnections(ctx context.Context, tx *gorm.DB, request ChatCapabilityRequest, selections []bindings.InitialChatConnection) error {
	if resolver == nil || resolver.catalog == nil || tx == nil || request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	conversationID, err := uuid.Parse(request.ConversationID)
	if err != nil || conversationID == uuid.Nil || conversationID.String() != request.ConversationID {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if err := bindings.ValidateInitialChatConnections(selections); err != nil {
		return err
	}
	if len(selections) == 0 {
		return nil
	}
	expected := make(map[string]uuid.UUID, len(selections))
	ids := make([]uuid.UUID, 0, len(selections))
	for _, selection := range selections {
		expected[selection.ConnectorID.String()] = selection.ConnectionID
		ids = append(ids, selection.ConnectionID)
	}
	var records []initialChatConnectionRecord
	err = tx.WithContext(ctx).Table("connector_target_bindings AS b").
		Select("b.connection_id, b.connector_id, b.binding_constraint, c.connection_constraint, c.state, c.driver_id, c.auth_method_id, c.usable_operation_ids, c.unavailable_operations").
		Joins("JOIN connector_connections AS c ON c.id = b.connection_id AND c.organization_id = b.organization_id AND c.owner_account_id = b.owner_account_id AND c.connector_id = b.connector_id").
		Where("b.organization_id = ? AND b.owner_account_id = ? AND b.target_type = ? AND b.target_id = ? AND b.target_scope = ? AND b.target_version_id = ? AND b.binding_instance_id IS NULL AND b.status = ? AND c.deleted_at IS NULL", request.OrganizationID, request.OwnerAccountID, core.TargetTypeChat, request.ConversationID, core.ScopeLive, "", core.BindingStatusActive).
		Where("b.connection_id IN ?", ids).Scan(&records).Error
	if err != nil {
		return core.NewError(core.ErrorCodeInternal, false, err)
	}
	if len(records) != len(expected) {
		return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	target := core.TargetRef{Type: core.TargetTypeChat, ID: request.ConversationID}
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if expected[record.ConnectorID] != record.ConnectionID || seen[record.ConnectorID] || record.State != core.ReadinessReady {
			return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		seen[record.ConnectorID] = true
		definition, found := resolver.catalog.Get(record.ConnectorID)
		if !found || definition.ID != record.ConnectorID || catalog.Validate(definition) != nil || !definition.Supports(catalog.SurfaceChat) || definition.DriverID != record.DriverID || definition.AuthMethodID != record.AuthMethodID {
			return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		connectionConstraint, err := core.DecodeConstraint(record.ConnectionConstraint)
		if err != nil {
			return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		bindingConstraint, err := core.DecodeConstraint(record.BindingConstraint)
		if err != nil {
			return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		connection := core.PersonalConnection{Constraint: connectionConstraint}
		if json.Unmarshal(record.UsableOperationIDs, &connection.UsableOperationIDs) != nil || json.Unmarshal(record.UnavailableOperations, &connection.UnavailableOperations) != nil {
			return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		if len(permittedOperations(definition, connection, bindingConstraint, request.OwnerAccountID, target, time.Now())) == 0 {
			return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
	}
	return nil
}
