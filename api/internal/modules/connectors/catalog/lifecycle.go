package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrPluginLifecycleBlocked   = errors.New("Connector plugin lifecycle change is blocked by active references")
	ErrPluginDefinitionNotFound = errors.New("Connector plugin definition not found")
	ErrPluginDefinitionInactive = errors.New("Connector plugin definition is inactive")
	ErrPluginOperationNotFound  = errors.New("Connector plugin operation not found")
)

type PluginDefinitionStatus string

const (
	PluginDefinitionStatusActive      PluginDefinitionStatus = "active"
	PluginDefinitionStatusSuperseded  PluginDefinitionStatus = "superseded"
	PluginDefinitionStatusDisabled    PluginDefinitionStatus = "disabled"
	PluginDefinitionStatusUnavailable PluginDefinitionStatus = "unavailable"
)

type DefinitionImpact struct {
	ConnectorID string
	Status      PluginDefinitionStatus
}

type ConnectionImpact struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectorID    string
	State          string
}

type BindingImpact struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	OwnerAccountID    uuid.UUID
	ConnectionID      uuid.UUID
	ConnectorID       string
	TargetType        string
	TargetID          string
	BindingInstanceID string
	Status            string
}

type PluginImpact struct {
	PluginID    string
	Definitions []DefinitionImpact
	Connections []ConnectionImpact
	Bindings    []BindingImpact
}

func (impact PluginImpact) HasReferences() bool {
	return len(impact.Connections) > 0 || len(impact.Bindings) > 0
}

// PluginLifecycleStore owns the atomic database boundary for impact checking
// and forced state projection. Implementations must not delete Connections,
// Target Bindings, or encrypted credential envelopes.
type PluginLifecycleStore interface {
	InspectPluginImpact(context.Context, string) (PluginImpact, error)
	TransitionPlugin(context.Context, string, PluginDefinitionStatus, bool) (PluginImpact, error)
}

type pinnedVersionKey struct {
	ConnectorID    string
	PluginVersion  string
	PluginDigest   string
	ContractDigest string
}

type versionedDefinition struct {
	definition     Definition
	contractDigest string
}

// PinnedOperation is a defensive immutable snapshot. Holding this value is the
// runtime contract pin: activation, disablement, or removal can prevent new
// pins but cannot switch the definition used by an operation already started.
type PinnedOperation struct {
	ConnectorID    string
	PluginID       string
	PluginVersion  string
	PluginDigest   string
	ContractDigest string
	Definition     Definition
	Operation      Operation
}

type PluginLifecycle struct {
	mu           sync.RWMutex
	store        PluginLifecycleStore
	active       map[string]pinnedVersionKey
	versions     map[pinnedVersionKey]versionedDefinition
	pluginStatus map[string]PluginDefinitionStatus
}

func NewPluginLifecycle(definitions []Definition, store PluginLifecycleStore) (*PluginLifecycle, error) {
	return NewPluginLifecycleWithStatuses(definitions, nil, store)
}

// NewPluginLifecycleWithStatuses restores the durable current plugin state at
// process startup. Definitions for disabled/unavailable plugins remain loaded
// only so maintainers can inspect or transition them; Pin always fails closed.
func NewPluginLifecycleWithStatuses(
	definitions []Definition,
	statuses map[string]PluginDefinitionStatus,
	store PluginLifecycleStore,
) (*PluginLifecycle, error) {
	lifecycle := &PluginLifecycle{
		store: store, active: make(map[string]pinnedVersionKey, len(definitions)),
		versions:     make(map[pinnedVersionKey]versionedDefinition, len(definitions)),
		pluginStatus: make(map[string]PluginDefinitionStatus),
	}
	for _, definition := range definitions {
		if err := lifecycle.activateLocked(definition, false); err != nil {
			return nil, err
		}
	}
	for pluginID, status := range statuses {
		if !stableIdentifier(pluginID) || (status != PluginDefinitionStatusActive &&
			status != PluginDefinitionStatusDisabled && status != PluginDefinitionStatusUnavailable) {
			return nil, ErrPluginDefinitionNotFound
		}
		if lifecycle.hasPluginLocked(pluginID) {
			lifecycle.pluginStatus[pluginID] = status
		}
	}
	return lifecycle, nil
}

// Activate accepts an idempotent or backward-compatible version. Existing
// operation pins remain addressable in versions and are never rewritten.
func (lifecycle *PluginLifecycle) Activate(definition Definition) error {
	return lifecycle.ActivatePackage([]Definition{definition})
}

// ActivatePackage validates and swaps every Connector definition from one
// plugin as a single in-memory operation. A rejected component leaves the
// prior active package completely unchanged.
func (lifecycle *PluginLifecycle) ActivatePackage(definitions []Definition) error {
	if lifecycle == nil {
		return ErrPluginDefinitionNotFound
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	candidates, pluginID, err := validatePluginPackageDefinitions(definitions)
	if err != nil {
		return err
	}
	if status, known := lifecycle.pluginStatus[pluginID]; known && status != PluginDefinitionStatusActive {
		return ErrPluginDefinitionInactive
	}
	current := make(map[string]Definition)
	for connectorID, key := range lifecycle.active {
		definition := lifecycle.versions[key].definition
		if definition.PluginID == pluginID {
			current[connectorID] = definition
		}
	}
	if err := validateActivePackageReplacement(current, candidates, pluginID); err != nil {
		return err
	}
	staged := lifecycle.cloneLocked()
	for _, definition := range definitions {
		if err := staged.activateLocked(definition, true); err != nil {
			return err
		}
	}
	lifecycle.active = staged.active
	lifecycle.versions = staged.versions
	lifecycle.pluginStatus = staged.pluginStatus
	return nil
}

func (lifecycle *PluginLifecycle) ValidatePackageActivation(definitions []Definition) error {
	if lifecycle == nil {
		return ErrPluginDefinitionNotFound
	}
	lifecycle.mu.RLock()
	staged := lifecycle.cloneLocked()
	lifecycle.mu.RUnlock()
	return staged.ActivatePackage(definitions)
}

func (lifecycle *PluginLifecycle) cloneLocked() *PluginLifecycle {
	cloned := &PluginLifecycle{
		store: lifecycle.store, active: make(map[string]pinnedVersionKey, len(lifecycle.active)),
		versions:     make(map[pinnedVersionKey]versionedDefinition, len(lifecycle.versions)),
		pluginStatus: make(map[string]PluginDefinitionStatus, len(lifecycle.pluginStatus)),
	}
	for connectorID, key := range lifecycle.active {
		cloned.active[connectorID] = key
	}
	for key, version := range lifecycle.versions {
		cloned.versions[key] = versionedDefinition{
			definition: cloneDefinition(version.definition), contractDigest: version.contractDigest,
		}
	}
	for pluginID, status := range lifecycle.pluginStatus {
		cloned.pluginStatus[pluginID] = status
	}
	return cloned
}

func (lifecycle *PluginLifecycle) activateLocked(definition Definition, compare bool) error {
	if err := Validate(definition); err != nil {
		return err
	}
	if status, knownPlugin := lifecycle.pluginStatus[definition.PluginID]; compare && knownPlugin && status != PluginDefinitionStatusActive {
		return ErrPluginDefinitionInactive
	}
	if definition.PluginDigest == "" {
		digest, err := digestJSON(struct {
			PluginID      string `json:"plugin_id"`
			PluginVersion string `json:"plugin_version"`
			Dialect       string `json:"source_dialect"`
			SchemaVersion string `json:"schema_version"`
		}{definition.PluginID, definition.PluginVersion, definition.SourceDialect, definition.SourceSchemaVersion})
		if err != nil {
			return err
		}
		definition.PluginDigest = digest
	}
	contractDigest, err := DefinitionContractDigest(definition)
	if err != nil {
		return err
	}
	if existingKey, exists := lifecycle.active[definition.ID]; exists {
		existing := lifecycle.versions[existingKey].definition
		if existing.PluginID != definition.PluginID {
			return ErrDuplicateConnector
		}
		if compare {
			if err := CheckCompatibleUpdate(existing, definition); err != nil {
				return err
			}
		} else {
			return ErrDuplicateConnector
		}
	}
	key := pinnedVersionKey{
		ConnectorID: definition.ID, PluginVersion: definition.PluginVersion,
		PluginDigest: definition.PluginDigest, ContractDigest: contractDigest,
	}
	lifecycle.versions[key] = versionedDefinition{definition: cloneDefinition(definition), contractDigest: contractDigest}
	lifecycle.active[definition.ID] = key
	if _, exists := lifecycle.pluginStatus[definition.PluginID]; !exists {
		lifecycle.pluginStatus[definition.PluginID] = PluginDefinitionStatusActive
	}
	return nil
}

func (lifecycle *PluginLifecycle) Pin(connectorID, operationID string) (PinnedOperation, error) {
	if lifecycle == nil {
		return PinnedOperation{}, ErrPluginDefinitionNotFound
	}
	lifecycle.mu.RLock()
	defer lifecycle.mu.RUnlock()
	key, ok := lifecycle.active[connectorID]
	if !ok {
		return PinnedOperation{}, ErrPluginDefinitionNotFound
	}
	version := lifecycle.versions[key]
	if lifecycle.pluginStatus[version.definition.PluginID] != PluginDefinitionStatusActive {
		return PinnedOperation{}, ErrPluginDefinitionInactive
	}
	for _, operation := range version.definition.Operations {
		if operation.ID != operationID {
			continue
		}
		definition := cloneDefinition(version.definition)
		var pinnedOperation Operation
		for _, candidate := range definition.Operations {
			if candidate.ID == operationID {
				pinnedOperation = candidate
				break
			}
		}
		return PinnedOperation{
			ConnectorID: definition.ID, PluginID: definition.PluginID,
			PluginVersion: definition.PluginVersion, PluginDigest: definition.PluginDigest,
			ContractDigest: version.contractDigest, Definition: definition, Operation: pinnedOperation,
		}, nil
	}
	return PinnedOperation{}, ErrPluginOperationNotFound
}

func (lifecycle *PluginLifecycle) Impact(ctx context.Context, pluginID string) (PluginImpact, error) {
	if lifecycle == nil || !stableIdentifier(pluginID) {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	if lifecycle.store != nil {
		return lifecycle.store.InspectPluginImpact(ctx, pluginID)
	}
	lifecycle.mu.RLock()
	defer lifecycle.mu.RUnlock()
	return lifecycle.memoryImpactLocked(pluginID)
}

func (lifecycle *PluginLifecycle) Disable(
	ctx context.Context,
	pluginID string,
	force bool,
) (PluginImpact, error) {
	return lifecycle.transition(ctx, pluginID, PluginDefinitionStatusDisabled, force)
}

// Remove never physically deletes authoritative records. It marks definitions
// unavailable so affected Connections, bindings, IDs, and Keys remain
// inspectable and recoverable.
func (lifecycle *PluginLifecycle) Remove(
	ctx context.Context,
	pluginID string,
	force bool,
) (PluginImpact, error) {
	return lifecycle.transition(ctx, pluginID, PluginDefinitionStatusUnavailable, force)
}

func (lifecycle *PluginLifecycle) transition(
	ctx context.Context,
	pluginID string,
	status PluginDefinitionStatus,
	force bool,
) (PluginImpact, error) {
	if lifecycle == nil || !stableIdentifier(pluginID) ||
		(status != PluginDefinitionStatusDisabled && status != PluginDefinitionStatusUnavailable) {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.hasPluginLocked(pluginID) {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	var (
		impact PluginImpact
		err    error
	)
	if lifecycle.store != nil {
		impact, err = lifecycle.store.TransitionPlugin(ctx, pluginID, status, force)
	} else {
		impact, err = lifecycle.memoryImpactLocked(pluginID)
		if err == nil && impact.HasReferences() && !force {
			err = ErrPluginLifecycleBlocked
		}
	}
	if err != nil {
		return impact, err
	}
	lifecycle.pluginStatus[pluginID] = status
	return impact, nil
}

func (lifecycle *PluginLifecycle) hasPluginLocked(pluginID string) bool {
	for _, version := range lifecycle.versions {
		if version.definition.PluginID == pluginID {
			return true
		}
	}
	return false
}

func (lifecycle *PluginLifecycle) memoryImpactLocked(pluginID string) (PluginImpact, error) {
	impact := PluginImpact{PluginID: pluginID}
	seen := make(map[string]struct{})
	for connectorID, key := range lifecycle.active {
		definition := lifecycle.versions[key].definition
		if definition.PluginID != pluginID {
			continue
		}
		if _, exists := seen[connectorID]; exists {
			continue
		}
		seen[connectorID] = struct{}{}
		impact.Definitions = append(impact.Definitions, DefinitionImpact{
			ConnectorID: connectorID, Status: lifecycle.pluginStatus[pluginID],
		})
	}
	if len(impact.Definitions) == 0 {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	sort.Slice(impact.Definitions, func(left, right int) bool {
		return impact.Definitions[left].ConnectorID < impact.Definitions[right].ConnectorID
	})
	return impact, nil
}

type GormPluginLifecycleStore struct {
	db *gorm.DB
}

func NewGormPluginLifecycleStore(db *gorm.DB) *GormPluginLifecycleStore {
	return &GormPluginLifecycleStore{db: db}
}

func (store *GormPluginLifecycleStore) InspectPluginImpact(
	ctx context.Context,
	pluginID string,
) (PluginImpact, error) {
	if store == nil || store.db == nil || !stableIdentifier(pluginID) {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	return inspectPluginImpact(store.db.WithContext(ctx), pluginID)
}

func (store *GormPluginLifecycleStore) TransitionPlugin(
	ctx context.Context,
	pluginID string,
	status PluginDefinitionStatus,
	force bool,
) (PluginImpact, error) {
	if store == nil || store.db == nil || !stableIdentifier(pluginID) ||
		(status != PluginDefinitionStatusDisabled && status != PluginDefinitionStatusUnavailable) {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	var impact PluginImpact
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "connector-plugin:"+pluginID); result.Error != nil {
			return fmt.Errorf("lock Connector plugin lifecycle: %w", result.Error)
		}
		var err error
		impact, err = inspectPluginImpact(tx, pluginID)
		if err != nil {
			return err
		}
		fromStatus, consistent := pluginImpactStatus(impact)
		if !consistent {
			return fmt.Errorf("inspect current Connector plugin lifecycle: %w", ErrIncompatibleStoredDefinition)
		}
		// A replay that already reached the requested state is a no-op; the
		// immutable event committed with the first transition remains authority.
		if fromStatus == status {
			return nil
		}
		if fromStatus == PluginDefinitionStatusUnavailable {
			return ErrPluginDefinitionInactive
		}
		if impact.HasReferences() && !force {
			return ErrPluginLifecycleBlocked
		}
		var version pluginVersionEvidence
		if err := tx.Table("connector_plugin_versions").
			Select("plugin_version, plugin_digest").
			Where("plugin_id = ? AND status = ?", pluginID, fromStatus).
			Order("accepted_at DESC, plugin_version DESC").
			Take(&version).Error; err != nil {
			return fmt.Errorf("load current Connector plugin version: %w", err)
		}
		now := time.Now().UTC()
		definitionUpdate := tx.Table("connector_definitions").
			Where("plugin_id = ?", pluginID).
			Updates(map[string]interface{}{"status": status, "updated_at": now})
		if definitionUpdate.Error != nil {
			return fmt.Errorf("update Connector plugin definitions: %w", definitionUpdate.Error)
		}
		if definitionUpdate.RowsAffected == 0 {
			return ErrPluginDefinitionNotFound
		}
		pluginVersionUpdate := tx.Table("connector_plugin_versions").
			Where("plugin_id = ? AND status IN ?", pluginID, []PluginDefinitionStatus{
				PluginDefinitionStatusActive,
				PluginDefinitionStatusDisabled,
				PluginDefinitionStatusUnavailable,
			}).
			Updates(map[string]interface{}{
				"status": status, "retired_at": now, "updated_at": now,
			})
		if pluginVersionUpdate.Error != nil {
			return fmt.Errorf("update current Connector plugin version: %w", pluginVersionUpdate.Error)
		}
		if pluginVersionUpdate.RowsAffected == 0 {
			return fmt.Errorf("update current Connector plugin version: %w", ErrIncompatibleStoredDefinition)
		}
		definitionVersionUpdate := tx.Table("connector_definition_versions").
			Where("plugin_id = ? AND status IN ?", pluginID, []PluginDefinitionStatus{
				PluginDefinitionStatusActive,
				PluginDefinitionStatusDisabled,
				PluginDefinitionStatusUnavailable,
			}).
			Updates(map[string]interface{}{
				"status": status, "retired_at": now, "updated_at": now,
			})
		if definitionVersionUpdate.Error != nil {
			return fmt.Errorf("update current Connector definition versions: %w", definitionVersionUpdate.Error)
		}
		if definitionVersionUpdate.RowsAffected == 0 {
			return fmt.Errorf("update current Connector definition versions: %w", ErrIncompatibleStoredDefinition)
		}

		diagnosticCode := "connector_plugin_disabled"
		if status == PluginDefinitionStatusUnavailable {
			diagnosticCode = "connector_plugin_removed"
		}
		connectorIDs := tx.Table("connector_definitions").Select("connector_id").Where("plugin_id = ?", pluginID)
		if result := tx.Table("connector_connections").
			Where("connector_id IN (?) AND deleted_at IS NULL", connectorIDs).
			Updates(map[string]interface{}{
				"state": "unavailable", "diagnostic_code": diagnosticCode,
				"revision": gorm.Expr("revision + 1"), "updated_at": now,
			}); result.Error != nil {
			return fmt.Errorf("make Connector plugin Connections unavailable: %w", result.Error)
		}
		if result := tx.Table("connector_target_bindings").
			Where("connector_id IN (?) AND status <> ?", connectorIDs, "disabled").
			Updates(map[string]interface{}{
				"status": "needs_replacement", "revision": gorm.Expr("revision + 1"), "updated_at": now,
			}); result.Error != nil {
			return fmt.Errorf("mark Connector plugin targets for replacement: %w", result.Error)
		}
		return recordPluginTransitionAudit(ctx, tx, pluginID, fromStatus, status, force, impact, version, now)
	})
	return impact, err
}

func inspectPluginImpact(db *gorm.DB, pluginID string) (PluginImpact, error) {
	impact := PluginImpact{PluginID: pluginID}
	if err := db.Table("connector_definitions").
		Select("connector_id, status").
		Where("plugin_id = ?", pluginID).
		Order("connector_id ASC").
		Scan(&impact.Definitions).Error; err != nil {
		return PluginImpact{}, fmt.Errorf("inspect Connector plugin definitions: %w", err)
	}
	if len(impact.Definitions) == 0 {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	connectorIDs := db.Table("connector_definitions").Select("connector_id").Where("plugin_id = ?", pluginID)
	if err := db.Table("connector_connections").
		Select("id, organization_id, owner_account_id, connector_id, state").
		Where("connector_id IN (?) AND deleted_at IS NULL", connectorIDs).
		Order("connector_id ASC, organization_id ASC, owner_account_id ASC, id ASC").
		Scan(&impact.Connections).Error; err != nil {
		return PluginImpact{}, fmt.Errorf("inspect Connector plugin Connections: %w", err)
	}
	if err := db.Table("connector_target_bindings").
		Select("id, organization_id, owner_account_id, connection_id, connector_id, target_type, target_id, COALESCE(binding_instance_id, '') AS binding_instance_id, status").
		Where("connector_id IN (?) AND status <> ?", connectorIDs, "disabled").
		Order("connector_id ASC, target_type ASC, target_id ASC, binding_instance_id ASC, id ASC").
		Scan(&impact.Bindings).Error; err != nil {
		return PluginImpact{}, fmt.Errorf("inspect Connector plugin Target Bindings: %w", err)
	}
	return impact, nil
}
