package bindings

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/datatypes"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkflowProjectionScope identifies one authoritative Workflow authorization
// snapshot. Draft scopes use the stable workflows.id host identity. Published
// scopes use workflows.version_uuid, never the mutable/head row pointer.
type WorkflowProjectionScope struct {
	Scope          core.TargetScope
	OrganizationID uuid.UUID
	TargetID       uuid.UUID
}

// WorkflowPlacement is the owner-scoped Connection selected by one stable
// Workflow node placement. It intentionally contains no graph presentation or
// member-configurable policy state.
type WorkflowPlacement struct {
	Constraint        *core.AccessConstraint
	OwnerAccountID    uuid.UUID
	ConnectionID      uuid.UUID
	ConnectorID       core.Identifier
	BindingInstanceID string
}

type workflowProjectionMutation struct {
	operationID uuid.UUID
	createdAt   time.Time
}

func newWorkflowProjectionMutation(operationID uuid.UUID) workflowProjectionMutation {
	if operationID == uuid.Nil {
		operationID = uuid.New()
	}
	return workflowProjectionMutation{
		operationID: operationID,
		createdAt:   time.Now().UTC(),
	}
}

// LockWorkflowLineage serializes operations that can add or revoke published
// targets for one Workflow-owning Agent. Callers acquire this before any
// Workflow target lock. Draft-only saves do not change the target set and
// remain serialized by their draft target lock.
func (repository *GormRepository) LockWorkflowLineage(
	ctx context.Context,
	tx *gorm.DB,
	organizationID, lineageID uuid.UUID,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if tx == nil || organizationID == uuid.Nil || lineageID == uuid.Nil {
		return invalidInputError("lock workflow connector lineage")
	}
	key := fmt.Sprintf("connector-workflow-lineage:%s:%s", organizationID, lineageID)
	if err := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", key).Error; err != nil {
		return internalError("lock workflow connector lineage")
	}
	return nil
}

// LockWorkflowProjection serializes graph, ledger, and canonical Target
// Binding changes for one Workflow draft or published version. Callers that
// also lock a workflows row must take this advisory lock first so save,
// publish, delete, and startup backfill share one global lock order.
func (repository *GormRepository) LockWorkflowProjection(
	ctx context.Context,
	tx *gorm.DB,
	scope WorkflowProjectionScope,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if tx == nil {
		return invalidInputError("lock workflow connector projection")
	}
	if err := validateWorkflowProjection(scope, nil); err != nil {
		return err
	}
	return lockWorkflowProjectionScopes(ctx, tx, scope)
}

// ReplaceWorkflowDraftProjection reconciles the native canonical bindings for
// one draft graph. Removed or replaced placements are disabled rather than
// deleted so re-adding the same logical placement retains its binding UUID.
// legacy_read_only rows are never selected or mutated by this method.
func (repository *GormRepository) ReplaceWorkflowDraftProjection(
	ctx context.Context,
	tx *gorm.DB,
	scope WorkflowProjectionScope,
	desired []WorkflowPlacement,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if err := validateWorkflowProjection(scope, desired); err != nil {
		return err
	}
	db := repository.db
	if tx != nil {
		db = tx
	}
	if db == nil {
		return internalError("replace workflow connector projection")
	}
	replace := func(current *gorm.DB) error {
		if err := lockWorkflowProjectionScopes(ctx, current, scope); err != nil {
			return err
		}
		authorities, err := lockWorkflowProjectionAuthorities(ctx, current, []WorkflowProjectionScope{scope}, desired)
		if err != nil {
			return err
		}
		return repository.replaceWorkflowProjection(
			ctx, current, scope, desired, false, newWorkflowProjectionMutation(uuid.New()), authorities,
		)
	}
	if tx != nil {
		return replace(db)
	}
	return db.WithContext(ctx).Transaction(replace)
}

// SnapshotWorkflowPublishedProjection copies the exact active draft
// authorization snapshot to a new immutable published version target. Existing
// native destination rows must be byte-equivalent at the authorization level;
// a historical version is never rewritten in place.
func (repository *GormRepository) SnapshotWorkflowPublishedProjection(
	ctx context.Context,
	tx *gorm.DB,
	source, destination WorkflowProjectionScope,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if source.targetScope() != core.ScopeDraft || destination.targetScope() != core.ScopePublished {
		return invalidInputError("snapshot scope")
	}
	if err := validateWorkflowProjection(source, nil); err != nil {
		return err
	}
	if err := validateWorkflowProjection(destination, nil); err != nil || source == destination ||
		source.OrganizationID != destination.OrganizationID {
		return invalidInputError("snapshot workflow connector projection")
	}
	scopes := []WorkflowProjectionScope{source, destination}
	db := repository.db
	if tx != nil {
		db = tx
	}
	if db == nil {
		return internalError("snapshot workflow connector projection")
	}
	snapshot := func(current *gorm.DB) error {
		if err := lockWorkflowProjectionScopes(ctx, current, scopes...); err != nil {
			return err
		}
		authorities, err := lockWorkflowProjectionAuthorities(ctx, current, scopes, nil)
		if err != nil {
			return err
		}
		mutation := newWorkflowProjectionMutation(destination.TargetID)
		sourceRows, err := listWorkflowProjectionRecords(ctx, current, source, true)
		if err != nil {
			return err
		}
		if err := requireWorkflowProjectionAuthorityCoverage(authorities, source.OrganizationID, sourceRows, nil); err != nil {
			return err
		}
		desired := make([]WorkflowPlacement, 0, len(sourceRows))
		for _, row := range sourceRows {
			if row.Status != string(core.BindingStatusActive) {
				continue
			}
			connectorID, normalizeErr := core.NormalizeIdentifier(row.ConnectorID)
			if normalizeErr != nil || row.BindingInstanceID == nil {
				return internalError("decode workflow connector projection")
			}
			constraint, err := core.DecodeConstraint(row.Constraint)
			if err != nil {
				return err
			}
			desired = append(desired, WorkflowPlacement{Constraint: constraint,
				OwnerAccountID: row.OwnerAccountID, ConnectionID: row.ConnectionID,
				ConnectorID: connectorID, BindingInstanceID: *row.BindingInstanceID,
			})
		}
		if err := validateWorkflowProjection(destination, desired); err != nil {
			return err
		}
		if err := repository.replaceWorkflowProjection(
			ctx, current, destination, desired, true, mutation, authorities,
		); err != nil {
			return err
		}
		return nil
	}
	if tx != nil {
		return snapshot(db)
	}
	return db.WithContext(ctx).Transaction(snapshot)
}

// RevokeWorkflowProjection disables every native binding in one exact draft or
// published target. It does not affect other published versions.
func (repository *GormRepository) RevokeWorkflowProjection(
	ctx context.Context,
	tx *gorm.DB,
	scope WorkflowProjectionScope,
) error {
	return repository.RevokeWorkflowProjections(ctx, tx, []WorkflowProjectionScope{scope})
}

// RevokeWorkflowProjections disables every native binding across one complete
// Workflow lineage after acquiring every target lock in a global order. The
// canonical rows and lifecycle evidence commit or roll back as one unit.
func (repository *GormRepository) RevokeWorkflowProjections(
	ctx context.Context,
	tx *gorm.DB,
	scopes []WorkflowProjectionScope,
) error {
	if err := repository.available(); err != nil {
		return err
	}
	if len(scopes) == 0 {
		return invalidInputError("revoke workflow connector projections")
	}
	organizationID := scopes[0].OrganizationID
	unique := make(map[uuid.UUID]WorkflowProjectionScope, len(scopes))
	for _, scope := range scopes {
		if err := validateWorkflowProjection(scope, nil); err != nil || scope.OrganizationID != organizationID {
			return invalidInputError("revoke workflow connector projections")
		}
		unique[scope.TargetID] = scope
	}
	normalized := make([]WorkflowProjectionScope, 0, len(unique))
	for _, scope := range unique {
		normalized = append(normalized, scope)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].TargetID.String() < normalized[j].TargetID.String() })
	db := repository.db
	if tx != nil {
		db = tx
	}
	revoke := func(current *gorm.DB) error {
		if err := lockWorkflowProjectionScopes(ctx, current, normalized...); err != nil {
			return err
		}
		authorities, err := lockWorkflowProjectionAuthorities(ctx, current, normalized, nil)
		if err != nil {
			return err
		}
		mutation := newWorkflowProjectionMutation(uuid.New())
		for _, scope := range normalized {
			if err := repository.replaceWorkflowProjection(ctx, current, scope, nil, false, mutation, authorities); err != nil {
				return err
			}
		}
		return nil
	}
	if tx != nil {
		return revoke(db)
	}
	return db.WithContext(ctx).Transaction(revoke)
}

// ListActiveWorkflowProjection returns the exact canonical native snapshot.
// It is intended for graph/ledger consistency checks; normal member-facing
// reads remain owner-scoped through Repository.ListBindingsByTarget.
func (repository *GormRepository) ListActiveWorkflowProjection(
	ctx context.Context,
	tx *gorm.DB,
	scope WorkflowProjectionScope,
) ([]core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if err := validateWorkflowProjection(scope, nil); err != nil {
		return nil, err
	}
	db := repository.db
	if tx != nil {
		db = tx
	}
	rows, err := listWorkflowProjectionRecords(ctx, db, scope, false)
	if err != nil {
		return nil, err
	}
	result := make([]core.TargetBinding, 0, len(rows))
	for _, row := range rows {
		if row.Status != string(core.BindingStatusActive) {
			continue
		}
		binding, conversionErr := row.toDomain()
		if conversionErr != nil {
			return nil, conversionErr
		}
		result = append(result, binding)
	}
	return result, nil
}

func (repository *GormRepository) replaceWorkflowProjection(
	ctx context.Context,
	tx *gorm.DB,
	scope WorkflowProjectionScope,
	desired []WorkflowPlacement,
	immutable bool,
	mutation workflowProjectionMutation,
	authorities bindingLifecycleAuthoritySet,
) error {
	existing, err := listWorkflowProjectionRecords(ctx, tx, scope, true)
	if err != nil {
		return err
	}
	if err := requireWorkflowProjectionAuthorityCoverage(authorities, scope.OrganizationID, existing, desired); err != nil {
		return err
	}
	if immutable && len(existing) > 0 {
		if workflowProjectionEquivalent(existing, desired) {
			return nil
		}
		return conflictError("snapshot workflow connector projection")
	}

	now := mutation.createdAt
	desiredByKey := make(map[string]WorkflowPlacement, len(desired))
	for _, placement := range desired {
		if err := requireReadyConnection(
			ctx, tx, scope.OrganizationID, placement.OwnerAccountID,
			placement.ConnectionID, placement.ConnectorID,
		); err != nil {
			return err
		}
		desiredByKey[workflowPlacementKey(placement)] = placement
	}

	for _, row := range existing {
		if row.BindingInstanceID == nil {
			return internalError("decode workflow connector projection")
		}
		connectorID, normalizeErr := core.NormalizeIdentifier(row.ConnectorID)
		if normalizeErr != nil {
			return internalError("decode workflow connector projection")
		}
		placement := WorkflowPlacement{
			OwnerAccountID: row.OwnerAccountID, ConnectionID: row.ConnectionID,
			ConnectorID: connectorID, BindingInstanceID: *row.BindingInstanceID,
		}
		key := workflowPlacementKey(placement)
		desiredPlacement, keep := desiredByKey[key]
		var narrowedJSON []byte
		if keep {
			previous, err := core.DecodeConstraint(row.Constraint)
			if err != nil {
				return err
			}
			desiredConstraint := desiredPlacement.Constraint
			if desiredConstraint == nil {
				desiredConstraint = NewBindingConstraint(desiredPlacement.OwnerAccountID, desiredPlacement.ConnectorID)
			}
			narrowed, err := core.IntersectConstraints(previous, desiredConstraint)
			if err != nil {
				return err
			}
			narrowedJSON, err = json.Marshal(narrowed)
			if err != nil {
				return err
			}
		}

		if keep && equivalentConstraintJSON(row.Constraint, narrowedJSON) && row.ConnectionID == desiredPlacement.ConnectionID && row.Status == string(core.BindingStatusActive) {
			delete(desiredByKey, key)
			continue
		}
		if keep {
			before, conversionErr := row.toDomain()
			if conversionErr != nil {
				return conversionErr
			}
			result := tx.WithContext(ctx).Model(&bindingRecord{}).
				Where("id = ? AND organization_id = ? AND revision = ?", row.ID, scope.OrganizationID, row.Revision).
				Updates(map[string]any{
					"connection_id":      desiredPlacement.ConnectionID,
					"binding_constraint": datatypes.JSON(narrowedJSON),
					"status":             core.BindingStatusActive,
					"disabled_at":        nil,
					"revision":           row.Revision + 1,
					"updated_at":         now,
				})
			if result.Error != nil {
				return internalError("replace workflow connector projection")
			}
			if result.RowsAffected != 1 {
				return conflictError("replace workflow connector projection")
			}
			updated := row
			updated.Constraint = datatypes.JSON(narrowedJSON)
			updated.ConnectionID = desiredPlacement.ConnectionID
			updated.Status = string(core.BindingStatusActive)
			updated.DisabledAt = nil
			updated.Revision = row.Revision + 1
			updated.UpdatedAt = now
			after, conversionErr := updated.toDomain()
			if conversionErr != nil {
				return conversionErr
			}
			if before.ConnectionID != after.ConnectionID {
				detached := before
				detached.Status = core.BindingStatusDisabled
				detached.DisabledAt = &now
				detached.Revision = after.Revision
				detached.UpdatedAt = now
				if err := appendWorkflowProjectionLifecycle(ctx, tx, detached, mutation, MutationBindingDetached, authorities); err != nil {
					return err
				}
			}
			if err := appendWorkflowProjectionLifecycle(ctx, tx, after, mutation, MutationBindingUpserted, authorities); err != nil {
				return err
			}
			delete(desiredByKey, key)
			continue
		}
		if row.Status == string(core.BindingStatusDisabled) {
			continue
		}
		result := tx.WithContext(ctx).Model(&bindingRecord{}).
			Where("id = ? AND organization_id = ? AND revision = ?", row.ID, scope.OrganizationID, row.Revision).
			Updates(map[string]any{
				"status":      core.BindingStatusDisabled,
				"disabled_at": now,
				"revision":    row.Revision + 1,
				"updated_at":  now,
			})
		if result.Error != nil {
			return internalError("replace workflow connector projection")
		}
		if result.RowsAffected != 1 {
			return conflictError("replace workflow connector projection")
		}
		disabled := row
		disabled.Status = string(core.BindingStatusDisabled)
		disabled.DisabledAt = &now
		disabled.Revision = row.Revision + 1
		disabled.UpdatedAt = now
		after, conversionErr := disabled.toDomain()
		if conversionErr != nil {
			return conversionErr
		}
		if err := appendWorkflowProjectionLifecycle(ctx, tx, after, mutation, MutationBindingDetached, authorities); err != nil {
			return err
		}
	}

	keys := make([]string, 0, len(desiredByKey))
	for key := range desiredByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		placement := desiredByKey[key]
		target := core.TargetRef{
			Type: core.TargetTypeWorkflow, ID: scope.TargetID.String(), Scope: scope.targetScope(), VersionID: scope.versionID(),
			BindingInstanceID: strings.TrimSpace(placement.BindingInstanceID),
		}
		binding := core.TargetBinding{
			Constraint: placement.Constraint, ID: stableNativeBindingID(scope.OrganizationID, placement.OwnerAccountID, placement.ConnectorID, target),
			OrganizationID: scope.OrganizationID, OwnerAccountID: placement.OwnerAccountID,
			ConnectionID: placement.ConnectionID, ConnectorID: placement.ConnectorID,
			Target: target, Status: core.BindingStatusActive,

			Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		if binding.Constraint == nil {
			binding.Constraint = NewBindingConstraint(binding.OwnerAccountID, binding.ConnectorID)
		}
		record, conversionErr := bindingRecordFromDomain(binding)
		if conversionErr != nil {
			return conversionErr
		}
		if createErr := tx.WithContext(ctx).Create(&record).Error; createErr != nil {
			if isDuplicateDatabaseError(createErr) {
				return conflictError("replace workflow connector projection")
			}
			return internalError("replace workflow connector projection")
		}
		if err := appendWorkflowProjectionLifecycle(ctx, tx, binding, mutation, MutationBindingUpserted, authorities); err != nil {
			return err
		}
	}
	return nil
}

func appendWorkflowProjectionLifecycle(
	ctx context.Context,
	tx *gorm.DB,
	binding core.TargetBinding,
	mutation workflowProjectionMutation,
	mutationType string,
	authorities bindingLifecycleAuthoritySet,
) error {
	correlationID := strings.Join([]string{
		"workflow-projection", mutation.operationID.String(), mutationType, binding.ID.String(),
	}, ":")
	_, locked := authorities[bindingLifecycleAuthorityIdentity(binding)]
	if !locked {
		return conflictError("lock workflow connector binding lifecycle authority")
	}
	return appendBindingLifecycleMutationLocked(ctx, tx, binding, BindingLifecycleMutation{
		ID:            uuid.NewSHA1(mutation.operationID, []byte(mutationType+"\x00"+binding.ID.String()+"\x00"+binding.ConnectionID.String())),
		MutationType:  mutationType,
		CorrelationID: correlationID,
		CreatedAt:     mutation.createdAt,
	})
}

func lockWorkflowProjectionAuthorities(
	ctx context.Context,
	tx *gorm.DB,
	scopes []WorkflowProjectionScope,
	desired []WorkflowPlacement,
) (bindingLifecycleAuthoritySet, error) {
	identities := make([]connectionLockIdentity, 0, len(desired)+len(scopes))
	for _, scope := range scopes {
		rows, err := listWorkflowProjectionRecords(ctx, tx, scope, false)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			identity, identityErr := workflowProjectionAuthorityIdentity(scope.OrganizationID, row.OwnerAccountID, row.ConnectionID, row.ConnectorID)
			if identityErr != nil {
				return nil, identityErr
			}
			identities = append(identities, identity)
		}
	}
	if len(scopes) == 0 && len(desired) > 0 {
		return nil, invalidInputError("lock workflow connector projection authority")
	}
	for _, placement := range desired {
		identity, err := workflowProjectionAuthorityIdentity(
			scopes[0].OrganizationID,
			placement.OwnerAccountID,
			placement.ConnectionID,
			placement.ConnectorID.String(),
		)
		if err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return lockBindingLifecycleIdentities(ctx, tx, identities)
}

func requireWorkflowProjectionAuthorityCoverage(
	authorities bindingLifecycleAuthoritySet,
	organizationID uuid.UUID,
	existing []bindingRecord,
	desired []WorkflowPlacement,
) error {
	for _, row := range existing {
		identity, err := workflowProjectionAuthorityIdentity(
			organizationID, row.OwnerAccountID, row.ConnectionID, row.ConnectorID,
		)
		if err != nil {
			return err
		}
		if _, locked := authorities[identity]; !locked {
			return conflictError("verify workflow connector projection authority")
		}
	}
	for _, placement := range desired {
		identity, err := workflowProjectionAuthorityIdentity(
			organizationID,
			placement.OwnerAccountID,
			placement.ConnectionID,
			placement.ConnectorID.String(),
		)
		if err != nil {
			return err
		}
		if _, locked := authorities[identity]; !locked {
			return conflictError("verify workflow connector placement authority")
		}
	}
	return nil
}

func workflowProjectionAuthorityIdentity(
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectorText string,
) (connectionLockIdentity, error) {
	connectorID, err := core.NormalizeIdentifier(connectorText)
	if err != nil {
		return connectionLockIdentity{}, internalError("decode workflow connector projection authority")
	}
	identity := connectionLockIdentity{
		OrganizationID: organizationID,
		OwnerAccountID: ownerAccountID,
		ConnectionID:   connectionID,
		ConnectorID:    connectorID,
	}
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil {
		return connectionLockIdentity{}, internalError("decode workflow connector projection authority")
	}
	return identity, nil
}

func listWorkflowProjectionRecords(
	ctx context.Context,
	db *gorm.DB,
	scope WorkflowProjectionScope,
	lock bool,
) ([]bindingRecord, error) {
	if db == nil {
		return nil, internalError("list workflow connector projection")
	}
	query := db.WithContext(ctx).Where(
		"organization_id = ? AND target_type = ? AND target_id = ? AND target_scope = ? AND target_version_id = ?",
		scope.OrganizationID, core.TargetTypeWorkflow, scope.TargetID.String(), scope.targetScope(), scope.versionID(),
	)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []bindingRecord
	if err := query.Order("owner_account_id ASC, connector_id ASC, binding_instance_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, internalError("list workflow connector projection")
	}
	return rows, nil
}

func validateWorkflowProjection(scope WorkflowProjectionScope, placements []WorkflowPlacement) error {
	if scope.OrganizationID == uuid.Nil || scope.TargetID == uuid.Nil {
		return invalidInputError("validate workflow connector projection")
	}
	seenNodes := make(map[string]struct{}, len(placements))
	for _, placement := range placements {
		nodeID := strings.TrimSpace(placement.BindingInstanceID)
		if placement.OwnerAccountID == uuid.Nil || placement.ConnectionID == uuid.Nil ||
			!placement.ConnectorID.Valid() || nodeID == "" || nodeID != placement.BindingInstanceID || len(nodeID) > 255 {
			return invalidInputError("validate workflow connector projection")
		}
		if _, duplicate := seenNodes[nodeID]; duplicate {
			return conflictError("validate workflow connector projection")
		}
		seenNodes[nodeID] = struct{}{}
	}
	return nil
}

func workflowProjectionEquivalent(existing []bindingRecord, desired []WorkflowPlacement) bool {
	if len(existing) != len(desired) {
		return false
	}
	want := make(map[string]WorkflowPlacement, len(desired))
	for _, placement := range desired {
		want[workflowPlacementKey(placement)] = placement
	}
	for _, row := range existing {
		if row.BindingInstanceID == nil || row.Status != string(core.BindingStatusActive) || row.DisabledAt != nil {
			return false
		}
		connectorID, err := core.NormalizeIdentifier(row.ConnectorID)
		if err != nil {
			return false
		}
		placement := WorkflowPlacement{
			OwnerAccountID: row.OwnerAccountID, ConnectionID: row.ConnectionID,
			ConnectorID: connectorID, BindingInstanceID: *row.BindingInstanceID,
		}
		candidate, ok := want[workflowPlacementKey(placement)]
		storedConstraint, constraintErr := core.DecodeConstraint(row.Constraint)
		if !ok || constraintErr != nil || !core.EquivalentConstraints(storedConstraint, candidate.Constraint) || candidate.ConnectionID != row.ConnectionID {
			return false
		}
	}
	return true
}

func workflowPlacementKey(placement WorkflowPlacement) string {
	return strings.Join([]string{
		placement.OwnerAccountID.String(), placement.ConnectorID.String(),
		strings.TrimSpace(placement.BindingInstanceID),
	}, "\x00")
}

func lockWorkflowProjectionScopes(ctx context.Context, db *gorm.DB, scopes ...WorkflowProjectionScope) error {
	keys := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		keys = append(keys, fmt.Sprintf("connector-workflow-target:%s:%s", scope.OrganizationID, scope.TargetID))
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", key).Error; err != nil {
			return internalError("lock workflow connector projection")
		}
	}
	return nil
}

func (scope WorkflowProjectionScope) targetScope() core.TargetScope {
	if scope.Scope == core.ScopePublished {
		return core.ScopePublished
	}
	return core.ScopeDraft
}
func (scope WorkflowProjectionScope) versionID() string {
	if scope.targetScope() == core.ScopePublished {
		return scope.TargetID.String()
	}
	return ""
}
