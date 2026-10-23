package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MemberConnectorSlot is a per-user slot as a workspace member sees it, with
// their own ready Connections that may serve it.
type MemberConnectorSlot struct {
	PerUserConnectorSlot
	Options []MemberConnectionOption `json:"options"`
}

// MemberConnectionOption is one of the member's own Connections.
type MemberConnectionOption struct {
	ConnectionID uuid.UUID `json:"connection_id"`
	Name         string    `json:"name"`
	Account      string    `json:"account,omitempty"`
}

func (s *RuntimeService) memberSlotRepository(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) (*GormRepository, uuid.UUID, error) {
	repository, ok := s.store.(*GormRepository)
	if !ok || repository.connectorCatalog == nil || !PerUserConnectorsEnabled() || scope.ActorID == uuid.Nil {
		return nil, uuid.Nil, ErrConnectorSlotNotFound
	}
	definition, err := repository.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	if definition.PublishedVersionID == nil {
		return nil, uuid.Nil, ErrConnectorSlotNotFound
	}
	return repository, *definition.PublishedVersionID, nil
}

// ListMemberConnectorSlots lists the per-user slots of a published
// DigitalHuman for the member, with the Connections they may choose.
func (s *RuntimeService) ListMemberConnectorSlots(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) ([]MemberConnectorSlot, error) {
	result := []MemberConnectorSlot{}
	repository, versionID, err := s.memberSlotRepository(ctx, scope, digitalHumanID)
	if errors.Is(err, ErrConnectorSlotNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := perUserSlotRows(ctx, repository.db, scope.OrganizationID, digitalHumanID, versionID)
	if err != nil {
		return nil, err
	}
	owned := connections.NewGormRepository(repository.db)
	for _, row := range rows {
		slot := MemberConnectorSlot{PerUserConnectorSlot: perUserSlotFor(repository.connectorCatalog, row), Options: []MemberConnectionOption{}}
		ready, err := owned.ListConnections(ctx, scope.OrganizationID, scope.ActorID, connections.ConnectionListFilter{
			ConnectorID: core.Identifier(row.ParentResourceID), States: []core.Readiness{core.ReadinessReady},
		})
		if err != nil {
			return nil, err
		}
		for _, connection := range ready {
			slot.Options = append(slot.Options, MemberConnectionOption{ConnectionID: connection.ID, Name: connection.Name, Account: derefString(connection.ExternalDisplayName)})
		}
		var selection ConnectorSelection
		if err := repository.db.WithContext(ctx).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", digitalHumanID, scope.ActorID, row.ResourceID).Take(&selection).Error; err == nil {
			connectionID := selection.ConnectionID
			slot.ConnectionID, slot.State = &connectionID, PerUserSlotReconnectRequired
			if slices.ContainsFunc(ready, func(connection core.PersonalConnection) bool { return connection.ID == selection.ConnectionID }) {
				slot.State = PerUserSlotConnected
			}
		}
		result = append(result, slot)
	}
	return result, nil
}

// SelectMemberConnectorSlot selects one of the member's own ready
// Connections for a per-user slot. A member never selects another's.
func (s *RuntimeService) SelectMemberConnectorSlot(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, slotKey string, connectionID uuid.UUID) error {
	repository, versionID, err := s.memberSlotRepository(ctx, scope, digitalHumanID)
	if err != nil {
		return err
	}
	rows, err := perUserSlotRows(ctx, repository.db, scope.OrganizationID, digitalHumanID, versionID)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(rows, func(row *ResourceBinding) bool { return row.ResourceID == slotKey })
	if index < 0 {
		return ErrConnectorSlotNotFound
	}
	connection, err := connections.NewGormRepository(repository.db).GetConnection(ctx, scope.OrganizationID, scope.ActorID, connectionID)
	if err != nil || connection.ConnectorID.String() != rows[index].ParentResourceID || connection.State != core.ReadinessReady {
		return ErrInvalidRequest
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var existing ConnectorSelection
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", digitalHumanID, scope.ActorID, slotKey).Take(&existing)
		if lookup.Error == nil {
			return tx.Model(&ConnectorSelection{}).Where("id = ?", existing.ID).Updates(map[string]any{"connection_id": connectionID, "updated_at": now}).Error
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load member connector selection: %w", lookup.Error)
		}
		return tx.Create(&ConnectorSelection{ID: uuid.New(), OrganizationID: scope.OrganizationID, DigitalHumanID: digitalHumanID,
			OwnerKind: connectorOwnerAccount, OwnerID: scope.ActorID, SlotKey: slotKey, ConnectorID: rows[index].ParentResourceID,
			ConnectionID: connectionID, CreatedAt: now, UpdatedAt: now}).Error
	})
}

// ClearMemberConnectorSlot drops the member's selection for a slot; their
// Connection itself stays.
func (s *RuntimeService) ClearMemberConnectorSlot(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, slotKey string) error {
	repository, _, err := s.memberSlotRepository(ctx, scope, digitalHumanID)
	if err != nil {
		return err
	}
	if err := repository.db.WithContext(ctx).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", digitalHumanID, scope.ActorID, slotKey).
		Delete(&ConnectorSelection{}).Error; err != nil {
		return fmt.Errorf("clear member connector selection: %w", err)
	}
	return nil
}

type memberConnectorSlotService interface {
	ListMemberConnectorSlots(context.Context, Scope, uuid.UUID) ([]MemberConnectorSlot, error)
	SelectMemberConnectorSlot(context.Context, Scope, uuid.UUID, string, uuid.UUID) error
	ClearMemberConnectorSlot(context.Context, Scope, uuid.UUID, string) error
}

func (h *RuntimeHandler) memberSlots(c *gin.Context) (memberConnectorSlotService, Scope, uuid.UUID, bool) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return nil, Scope{}, uuid.Nil, false
	}
	service, ok := h.service.(memberConnectorSlotService)
	if !ok {
		h.base.writeError(c, applicationError(ErrNotFound, "digital_human.connector_slots"))
		return nil, Scope{}, uuid.Nil, false
	}
	return service, scope, digitalHumanID, true
}

// ListMemberConnectorSlots lists the per-user slots of a DigitalHuman for
// the signed-in member.
func (h *RuntimeHandler) ListMemberConnectorSlots(c *gin.Context) {
	service, scope, digitalHumanID, ok := h.memberSlots(c)
	if !ok {
		return
	}
	slots, err := service.ListMemberConnectorSlots(c.Request.Context(), scope, digitalHumanID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.connector_slots.list"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"items": slots})
}

// SelectMemberConnectorSlot selects the member's Connection for a slot.
func (h *RuntimeHandler) SelectMemberConnectorSlot(c *gin.Context) {
	service, scope, digitalHumanID, ok := h.memberSlots(c)
	if !ok {
		return
	}
	var request struct {
		ConnectionID string `json:"connection_id"`
	}
	if !h.base.decode(c, &request) {
		return
	}
	connectionID, err := uuid.Parse(strings.TrimSpace(request.ConnectionID))
	if err != nil || connectionID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.connector_slots.select"))
		return
	}
	if err := service.SelectMemberConnectorSlot(c.Request.Context(), scope, digitalHumanID, c.Param("slot_key"), connectionID); err != nil {
		h.base.writeError(c, applicationError(memberSlotError(err), "digital_human.connector_slots.select"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"result": "success"})
}

// ClearMemberConnectorSlot clears the member's selection for a slot.
func (h *RuntimeHandler) ClearMemberConnectorSlot(c *gin.Context) {
	service, scope, digitalHumanID, ok := h.memberSlots(c)
	if !ok {
		return
	}
	if err := service.ClearMemberConnectorSlot(c.Request.Context(), scope, digitalHumanID, c.Param("slot_key")); err != nil {
		h.base.writeError(c, applicationError(memberSlotError(err), "digital_human.connector_slots.clear"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, gin.H{"result": "success"})
}

func memberSlotError(err error) error {
	if errors.Is(err, ErrConnectorSlotNotFound) {
		return ErrNotFound
	}
	return err
}
