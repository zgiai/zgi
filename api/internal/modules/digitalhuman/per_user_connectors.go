package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Connector credential modes of a DigitalHuman Connector slot (ADR 0007).
// A shared slot acts as the DigitalHuman with its editor's delegated
// Connection; a per-user slot acts as the requester with their own.
const (
	ConnectorCredentialShared  = "shared"
	ConnectorCredentialPerUser = "per_user"
)

// Owner kinds of a per-user slot selection.
const (
	connectorOwnerAccount = "account"
	connectorOwnerEndUser = "end_user"
)

var (
	ErrConnectorSlotNotFound = errors.New("digital human connector slot not found")
	ErrConnectorSetupFailed  = errors.New("digital human connector setup failed")
)

// PerUserConnectorsEnabled reports whether per-user Connector slots are on.
func PerUserConnectorsEnabled() bool {
	return envFlagEnabled("ZGI_DIGITAL_HUMAN_PER_USER_CONNECTORS_ENABLED")
}

func connectorCredentialMode(row *ResourceBinding) string {
	if row == nil || row.BindingType != resourcebindings.BindingTypeConnector {
		return ""
	}
	var metadata struct {
		Mode string `json:"credential_mode"`
	}
	_ = json.Unmarshal(row.Metadata, &metadata)
	if metadata.Mode == ConnectorCredentialPerUser {
		return ConnectorCredentialPerUser
	}
	return ConnectorCredentialShared
}

// validatePerUserConnectorSlot admits a per-user slot: a key-based Connector
// the DigitalHuman surface supports, with known operations. The slot names
// no Connection; resource_id is its key, stable across published versions.
func (r *GormRepository) validatePerUserConnectorSlot(row *ResourceBinding) error {
	if !PerUserConnectorsEnabled() || r.connectorCatalog == nil || row.CredentialRef != "" {
		return ErrResourceBindingInvalid
	}
	if key, err := uuid.Parse(row.ResourceID); err != nil || key == uuid.Nil || key.String() != row.ResourceID {
		return ErrResourceBindingInvalid
	}
	definition, ok := r.connectorCatalog.Get(row.ParentResourceID)
	if !ok || !definition.Supports(catalog.SurfaceDigitalHuman) || catalog.Validate(definition) != nil || definition.SetupMethod != catalog.SetupMethodKey {
		return ErrResourceBindingInvalid
	}
	operations, err := bindingOperations(row)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		if !slices.ContainsFunc(definition.Operations, func(op catalog.Operation) bool { return op.ID == operation.String() }) {
			return ErrResourceBindingInvalid
		}
	}
	return nil
}

// ConnectorSelection records the Connection a requester supplies for one
// per-user slot of one DigitalHuman.
type ConnectorSelection struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	DigitalHumanID uuid.UUID `gorm:"type:uuid;not null"`
	OwnerKind      string    `gorm:"type:varchar(16);not null"`
	OwnerID        uuid.UUID `gorm:"type:uuid;not null"`
	SlotKey        string    `gorm:"type:varchar(64);not null"`
	ConnectorID    string    `gorm:"type:varchar(128);not null"`
	ConnectionID   uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ConnectorSelection) TableName() string { return "digital_human_connector_selections" }

// PerUserConnectorSlot is a per-user slot as its requester sees it.
type PerUserConnectorSlot struct {
	SlotKey      string     `json:"slot_key"`
	ConnectorID  string     `json:"connector_id"`
	Name         string     `json:"name"`
	Operations   []string   `json:"operation_ids"`
	State        string     `json:"state"`
	ConnectionID *uuid.UUID `json:"connection_id,omitempty"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
}

// Per-user slot states.
const (
	PerUserSlotConnected         = "connected"
	PerUserSlotNeedsConnection   = "needs_connection"
	PerUserSlotReconnectRequired = "reconnect_required"
)

// perUserSlotRows returns the per-user slots of a published version.
func perUserSlotRows(ctx context.Context, db *gorm.DB, organizationID, digitalHumanID, versionID uuid.UUID) ([]*ResourceBinding, error) {
	var rows []*ResourceBinding
	if err := db.WithContext(ctx).
		Where("organization_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND binding_type = ? AND metadata->>'credential_mode' = ?",
			organizationID, digitalHumanID, versionID, resourcebindings.BindingTypeConnector, ConnectorCredentialPerUser).
		Order("created_at").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list per-user connector slots: %w", err)
	}
	return rows, nil
}

// perUserRequester is who a run's per-user slots act as: the end user of an
// API run, or the member of a console run. Channel, task, and other runs
// have no requester for them.
func perUserRequester(run *Run) (uuid.UUID, string, bool) {
	if run == nil {
		return uuid.Nil, "", false
	}
	switch run.Surface {
	case SurfaceAPI:
		if run.UserSubjectKind == string(usersubject.APIEndUser) && run.UserSubjectID != nil && *run.UserSubjectID != uuid.Nil {
			return *run.UserSubjectID, connectorOwnerEndUser, true
		}
	case SurfaceConsole:
		if run.ActorID != nil && *run.ActorID != uuid.Nil {
			return *run.ActorID, connectorOwnerAccount, true
		}
	}
	return uuid.Nil, "", false
}

// preparePerUserConnectors gives the requester of a run their own binding
// for each per-user slot they connected, on the run's published version, so
// discovery and dispatch use their Connection like any binding they own. It
// returns the slots still needing a connection.
func preparePerUserConnectors(ctx context.Context, db *gorm.DB, connectorCatalog ConnectorCatalog, run *Run) ([]PerUserConnectorSlot, error) {
	if !PerUserConnectorsEnabled() || db == nil || connectorCatalog == nil || run == nil {
		return nil, nil
	}
	requester, _, ok := perUserRequester(run)
	if !ok {
		return nil, nil
	}
	rows, err := perUserSlotRows(ctx, db, run.OrganizationID, run.DigitalHumanID, run.DigitalHumanVersionID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	var missing []PerUserConnectorSlot
	for _, row := range rows {
		slot, ready, err := ensurePerUserBinding(ctx, db, connectorCatalog, run.OrganizationID, run.DigitalHumanID, run.DigitalHumanVersionID, requester, row)
		if err != nil {
			return nil, err
		}
		if !ready {
			missing = append(missing, slot)
		}
	}
	return missing, nil
}

func perUserSlotFor(connectorCatalog ConnectorCatalog, row *ResourceBinding) PerUserConnectorSlot {
	operations, _ := bindingOperations(row)
	slot := PerUserConnectorSlot{SlotKey: row.ResourceID, ConnectorID: row.ParentResourceID, Name: row.DisplayName, State: PerUserSlotNeedsConnection}
	for _, operation := range operations {
		slot.Operations = append(slot.Operations, operation.String())
	}
	if definition, ok := connectorCatalog.Get(row.ParentResourceID); ok && strings.TrimSpace(slot.Name) == "" {
		slot.Name = definition.Name
	}
	return slot
}

// ensurePerUserBinding binds the requester's selected Connection to one slot
// of a published version, or reports why it cannot.
func ensurePerUserBinding(ctx context.Context, db *gorm.DB, connectorCatalog ConnectorCatalog, organizationID, digitalHumanID, versionID, requester uuid.UUID, row *ResourceBinding) (PerUserConnectorSlot, bool, error) {
	slot := perUserSlotFor(connectorCatalog, row)
	var selection ConnectorSelection
	err := db.WithContext(ctx).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", digitalHumanID, requester, row.ResourceID).Take(&selection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return slot, false, nil
	}
	if err != nil {
		return slot, false, fmt.Errorf("load per-user connector selection: %w", err)
	}
	connectionID := selection.ConnectionID
	slot.ConnectionID = &connectionID
	connection, err := connections.NewGormRepository(db).GetConnection(ctx, organizationID, requester, selection.ConnectionID)
	if err != nil || connection.ConnectorID.String() != row.ParentResourceID || connection.State != core.ReadinessReady {
		slot.State = PerUserSlotReconnectRequired
		return slot, false, nil
	}
	verifiedAt := connection.LastVerifiedAt
	slot.VerifiedAt = &verifiedAt
	operations, err := bindingOperations(row)
	if err != nil {
		return slot, false, err
	}
	target := digitalHumanConnectorTarget(digitalHumanID, versionID, row.ID, core.ScopePublished)
	constraint := core.OwnerConstraint(requester, operations)
	constraint.TargetBoundaries = []core.TargetBoundary{{Type: target.Type, ID: target.ID, Scope: string(target.EffectiveScope()), VersionID: target.VersionID}}
	now := time.Now().UTC()
	usable := make([]core.Identifier, 0, len(operations))
	for _, operation := range operations {
		if slices.Contains(connection.UsableOperationIDs, operation) && core.ConstraintsAllow(connection.Constraint, constraint, requester, operation, target, now) {
			usable = append(usable, operation)
		}
	}
	if len(usable) == 0 {
		slot.State = PerUserSlotReconnectRequired
		return slot, false, nil
	}
	constraint.AllowedOperationIDs = usable
	bindingID := uuid.NewSHA1(row.ID, []byte("digital-human-per-user:"+requester.String()))
	var existing struct {
		Revision     int
		ConnectionID uuid.UUID
		Status       string
	}
	lookup := db.WithContext(ctx).Table("connector_target_bindings").Select("revision, connection_id, status").Where("id = ?", bindingID).Take(&existing)
	if lookup.Error != nil && !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
		return slot, false, fmt.Errorf("load per-user connector binding: %w", lookup.Error)
	}
	found := lookup.Error == nil
	if found && existing.Status == string(core.BindingStatusActive) && existing.ConnectionID == selection.ConnectionID {
		slot.State = PerUserSlotConnected
		return slot, true, nil
	}
	canonical := core.TargetBinding{
		ID: bindingID, OrganizationID: organizationID, OwnerAccountID: requester, ConnectionID: selection.ConnectionID,
		ConnectorID: connection.ConnectorID, Target: target, Constraint: constraint, Status: core.BindingStatusActive,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	revision := 0
	if found {
		revision, canonical.Revision = existing.Revision, existing.Revision
	}
	mutation := bindings.BindingLifecycleMutation{ID: uuid.New(), MutationType: bindings.MutationBindingUpserted, CorrelationID: "digital-human-per-user:" + uuid.NewString(), CreatedAt: now}
	if _, _, err := bindings.NewGormRepository(db).UpsertBindingWithLifecycle(ctx, organizationID, requester, canonical, revision, mutation); err != nil {
		return slot, false, fmt.Errorf("bind per-user connector slot: %w", err)
	}
	slot.State = PerUserSlotConnected
	return slot, true, nil
}

// perUserConnectorNote tells the model which per-user slots need the
// requester to connect their own account.
func perUserConnectorNote(missing []PerUserConnectorSlot) string {
	if len(missing) == 0 {
		return ""
	}
	names := make([]string, 0, len(missing))
	for _, slot := range missing {
		names = append(names, slot.Name)
	}
	return "Some connected apps act with the user's own account, and the user has not connected it yet: " +
		strings.Join(names, ", ") + ". If a request needs one of them, tell the user to connect their own account for it first. " +
		"Never use another connected app or account in its place."
}

// EndUserKeySetup verifies and stores a key-based Connection.
type EndUserKeySetup interface {
	StartKeySetup(context.Context, connections.KeySetupCommand) (connections.KeySetupResult, error)
}

var endUserKeySetup atomic.Pointer[EndUserKeySetup]

// UseEndUserKeySetup gives end-user connection setup the Connector key
// setup service.
func UseEndUserKeySetup(setup EndUserKeySetup) {
	if setup != nil {
		endUserKeySetup.Store(&setup)
	}
}

var endUserConnections atomic.Pointer[EndUserConnections]

// UseEndUserConnections registers the end-user connection manager the
// Runtime Workspace end-user endpoints use.
func UseEndUserConnections(manager *EndUserConnections) {
	if manager != nil {
		endUserConnections.Store(manager)
	}
}

// RegisteredEndUserConnections returns the registered manager, or nil.
func RegisteredEndUserConnections() *EndUserConnections { return endUserConnections.Load() }

// EndUserConnections manages the Connections an end user supplies for the
// per-user slots of the DigitalHumans they use (ADR 0007).
type EndUserConnections struct {
	db      *gorm.DB
	catalog ConnectorCatalog
}

// NewEndUserConnections builds the end-user connection manager.
func NewEndUserConnections(db *gorm.DB, connectorCatalog ConnectorCatalog) *EndUserConnections {
	return &EndUserConnections{db: db, catalog: connectorCatalog}
}

// EndUserApp names a DigitalHuman an end user reaches.
type EndUserApp struct {
	AppKey         string
	DigitalHumanID uuid.UUID
}

// EndUserAppSlots is the per-user slots of one DigitalHuman.
type EndUserAppSlots struct {
	AppKey string                 `json:"app_key"`
	Slots  []PerUserConnectorSlot `json:"slots"`
}

func (m *EndUserConnections) available() bool {
	return m != nil && m.db != nil && m.catalog != nil && PerUserConnectorsEnabled()
}

func (m *EndUserConnections) publishedVersion(ctx context.Context, organizationID, digitalHumanID uuid.UUID) (uuid.UUID, error) {
	var definition struct{ PublishedVersionID *uuid.UUID }
	if err := m.db.WithContext(ctx).Table("digital_humans").Select("published_version_id").
		Where("id = ? AND organization_id = ? AND deleted_at IS NULL", digitalHumanID, organizationID).Take(&definition).Error; err != nil ||
		definition.PublishedVersionID == nil {
		return uuid.Nil, ErrConnectorSlotNotFound
	}
	return *definition.PublishedVersionID, nil
}

// List reports the per-user slots of the given DigitalHumans for an end user.
func (m *EndUserConnections) List(ctx context.Context, organizationID, subjectID uuid.UUID, apps []EndUserApp) ([]EndUserAppSlots, error) {
	result := []EndUserAppSlots{}
	if !m.available() {
		return result, nil
	}
	for _, app := range apps {
		versionID, err := m.publishedVersion(ctx, organizationID, app.DigitalHumanID)
		if err != nil {
			continue
		}
		rows, err := perUserSlotRows(ctx, m.db, organizationID, app.DigitalHumanID, versionID)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		entry := EndUserAppSlots{AppKey: app.AppKey, Slots: []PerUserConnectorSlot{}}
		for _, row := range rows {
			slot := perUserSlotFor(m.catalog, row)
			var selection ConnectorSelection
			if err := m.db.WithContext(ctx).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", app.DigitalHumanID, subjectID, row.ResourceID).Take(&selection).Error; err == nil {
				connectionID := selection.ConnectionID
				slot.ConnectionID, slot.State = &connectionID, PerUserSlotReconnectRequired
				if connection, err := connections.NewGormRepository(m.db).GetConnection(ctx, organizationID, subjectID, selection.ConnectionID); err == nil && connection.State == core.ReadinessReady {
					verifiedAt := connection.LastVerifiedAt
					slot.State, slot.VerifiedAt = PerUserSlotConnected, &verifiedAt
				}
			}
			entry.Slots = append(entry.Slots, slot)
		}
		result = append(result, entry)
	}
	return result, nil
}

// Connect verifies an end user's key for one per-user slot, stores it as
// their Connection, and selects it for the slot, replacing a previous one.
// values are destroyed on every return path.
func (m *EndUserConnections) Connect(ctx context.Context, organizationID, subjectID uuid.UUID, app EndUserApp, slotKey, idempotencyKey string, values map[string]string) (PerUserConnectorSlot, error) {
	defer clear(values)
	setup := endUserKeySetup.Load()
	if !m.available() || setup == nil {
		return PerUserConnectorSlot{}, ErrConnectorSlotNotFound
	}
	versionID, err := m.publishedVersion(ctx, organizationID, app.DigitalHumanID)
	if err != nil {
		return PerUserConnectorSlot{}, err
	}
	rows, err := perUserSlotRows(ctx, m.db, organizationID, app.DigitalHumanID, versionID)
	if err != nil {
		return PerUserConnectorSlot{}, err
	}
	index := slices.IndexFunc(rows, func(row *ResourceBinding) bool { return row.ResourceID == slotKey })
	if index < 0 {
		return PerUserConnectorSlot{}, ErrConnectorSlotNotFound
	}
	row := rows[index]
	result, err := (*setup).StartKeySetup(ctx, connections.KeySetupCommand{
		OrganizationID: organizationID, OwnerAccountID: subjectID, ConnectorID: row.ParentResourceID,
		IdempotencyKey: idempotencyKey, Values: values,
	})
	if err != nil {
		return PerUserConnectorSlot{}, err
	}
	if result.Connection == nil {
		return PerUserConnectorSlot{}, ErrConnectorSetupFailed
	}
	connectionID := result.Connection.ID
	var previous *uuid.UUID
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing ConnectorSelection
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", app.DigitalHumanID, subjectID, slotKey).Take(&existing)
		now := time.Now().UTC()
		switch {
		case lookup.Error == nil:
			if existing.ConnectionID != connectionID {
				old := existing.ConnectionID
				previous = &old
			}
			return tx.Model(&ConnectorSelection{}).Where("id = ?", existing.ID).Updates(map[string]any{"connection_id": connectionID, "updated_at": now}).Error
		case errors.Is(lookup.Error, gorm.ErrRecordNotFound):
			return tx.Create(&ConnectorSelection{ID: uuid.New(), OrganizationID: organizationID, DigitalHumanID: app.DigitalHumanID,
				OwnerKind: connectorOwnerEndUser, OwnerID: subjectID, SlotKey: slotKey, ConnectorID: row.ParentResourceID,
				ConnectionID: connectionID, CreatedAt: now, UpdatedAt: now}).Error
		default:
			return lookup.Error
		}
	})
	if err != nil {
		return PerUserConnectorSlot{}, fmt.Errorf("select end-user connection: %w", err)
	}
	if previous != nil {
		_ = removeOwnerConnection(ctx, m.db, organizationID, subjectID, *previous)
	}
	slot := perUserSlotFor(m.catalog, row)
	verifiedAt := result.Connection.LastVerifiedAt
	slot.ConnectionID, slot.State, slot.VerifiedAt = &connectionID, PerUserSlotConnected, &verifiedAt
	return slot, nil
}

// Disconnect removes an end user's Connection for one per-user slot.
func (m *EndUserConnections) Disconnect(ctx context.Context, organizationID, subjectID, digitalHumanID uuid.UUID, slotKey string) error {
	if !m.available() {
		return ErrConnectorSlotNotFound
	}
	var selection ConnectorSelection
	if err := m.db.WithContext(ctx).Where("digital_human_id = ? AND owner_id = ? AND slot_key = ?", digitalHumanID, subjectID, slotKey).Take(&selection).Error; err != nil {
		return ErrConnectorSlotNotFound
	}
	if err := m.db.WithContext(ctx).Delete(&ConnectorSelection{}, "id = ?", selection.ID).Error; err != nil {
		return fmt.Errorf("remove end-user connection selection: %w", err)
	}
	return removeOwnerConnection(ctx, m.db, organizationID, subjectID, selection.ConnectionID)
}

// removeOwnerConnection removes a Connection the way a member removes
// theirs: its bindings need replacement and its credential is destroyed.
func removeOwnerConnection(ctx context.Context, db *gorm.DB, organizationID, ownerID, connectionID uuid.UUID) error {
	repository := connections.NewGormRepository(db)
	connection, err := repository.GetConnection(ctx, organizationID, ownerID, connectionID)
	if err != nil {
		return nil
	}
	var active []connections.BindingRevisionSnapshot
	if err := db.WithContext(ctx).Table("connector_target_bindings").Select("id, revision").
		Where("organization_id = ? AND owner_account_id = ? AND connection_id = ? AND status = ?", organizationID, ownerID, connectionID, core.BindingStatusActive).
		Order("id ASC").Scan(&active).Error; err != nil {
		return fmt.Errorf("list end-user connection bindings: %w", err)
	}
	return repository.ForceRemoveConnection(ctx, organizationID, ownerID, connectionID, connection.Revision, active)
}

// eraseEndUserConnections destroys an end user's Connections, disables
// their bindings, and drops their selections, inside an erasure.
func eraseEndUserConnections(tx *gorm.DB, subjectID uuid.UUID, now time.Time) error {
	for _, statement := range []string{
		"UPDATE connector_target_bindings SET status = 'disabled', disabled_at = ?, revision = revision + 1, updated_at = ? WHERE owner_account_id = ? AND owner_kind = 'end_user' AND status <> 'disabled'",
		"UPDATE connector_connections SET encrypted_credentials = 'removed', state = 'unavailable', usable_operation_ids = '[]', diagnostic_code = 'connection_removed', revision = revision + 1, deleted_at = ?, updated_at = ? WHERE owner_account_id = ? AND owner_kind = 'end_user' AND deleted_at IS NULL",
	} {
		if err := tx.Exec(statement, now, now, subjectID).Error; err != nil {
			return fmt.Errorf("erase end-user connections: %w", err)
		}
	}
	if err := tx.Exec("DELETE FROM digital_human_connector_selections WHERE owner_id = ?", subjectID).Error; err != nil {
		return fmt.Errorf("erase end-user connection selections: %w", err)
	}
	return nil
}
