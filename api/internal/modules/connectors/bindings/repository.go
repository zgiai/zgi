package bindings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MutationBindingUpserted = "binding_upserted"
	MutationBindingDetached = "binding_detached"

	bindingLifecycleDecisionSuccessful = "successful"
)

// BindingLifecycleMutation binds a canonical write to its audit event.
type BindingLifecycleMutation struct {
	ID            uuid.UUID
	MutationType  string
	CorrelationID string
	CreatedAt     time.Time
}

// LifecycleMutationRepository guarantees that the canonical binding write,
// automatic audit decision commit
// or roll back together. Service-layer mutations intentionally require this
// contract; the lower-level Repository methods remain available to host
// projections that already own a broader transaction.
type LifecycleMutationRepository interface {
	UpsertBindingWithLifecycle(context.Context, uuid.UUID, uuid.UUID, core.TargetBinding, int, BindingLifecycleMutation) (core.TargetBinding, bool, error)
	SetBindingStatusWithLifecycle(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, core.BindingStatus, int, BindingLifecycleMutation) (core.TargetBinding, error)
}

// AuthorizationMutationRepository updates only the binding constraint. It is
// separate from UpsertBindingWithLifecycle because generic upsert intersects
// constraints (correct for replacement) and therefore cannot represent an
// explicit direct -> ask_each_time revocation.
type AuthorizationMutationRepository interface {
	UpdateBindingAuthorizationWithLifecycle(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *core.AccessConstraint, int, BindingLifecycleMutation) (core.TargetBinding, error)
}

// HostDelegationCandidateRepository reads active published bindings that have
// explicitly delegated a target to one host audience. It is deliberately
// separate from Repository because this lookup is organization-scoped rather
// than owner-scoped and must only be exposed through the audited service
// boundary.
type HostDelegationCandidateRepository interface {
	ListPublishedHostDelegations(context.Context, uuid.UUID, core.TargetRef, core.HostAudience) ([]core.TargetBinding, error)
}

type bindingLifecycleDecisionRecord struct {
	TargetScope        string         `gorm:"column:target_scope"`
	TargetVersionID    string         `gorm:"column:target_version_id"`
	ID                 uuid.UUID      `gorm:"column:id"`
	OrganizationID     uuid.UUID      `gorm:"column:organization_id"`
	OwnerAccountID     uuid.UUID      `gorm:"column:owner_account_id"`
	ActingPrincipalID  uuid.UUID      `gorm:"column:acting_principal_id"`
	ConnectorID        string         `gorm:"column:connector_id"`
	ConnectionID       uuid.UUID      `gorm:"column:connection_id"`
	TargetType         string         `gorm:"column:target_type"`
	TargetID           string         `gorm:"column:target_id"`
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

func (bindingLifecycleDecisionRecord) TableName() string {
	return "connector_lifecycle_decisions"
}

// ListOptions narrows an owner-scoped binding list. Empty slices mean all
// statuses. It contains no credential-bearing fields.
type ListOptions struct {
	Scope     core.TargetScope
	VersionID string
	Statuses  []core.BindingStatus
}

// Repository owns the canonical Target Binding rows. Every operation is
// scoped by organization and owner; a globally supplied binding UUID is never
// sufficient to read or mutate a row.
//
// Mutations preserve owner, target, constraint and revision checks.
type Repository interface {
	UpsertBinding(context.Context, uuid.UUID, uuid.UUID, core.TargetBinding, int) (core.TargetBinding, bool, error)
	GetBinding(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.TargetBinding, error)
	ListBindingsByTarget(context.Context, uuid.UUID, uuid.UUID, core.TargetType, string, ListOptions) ([]core.TargetBinding, error)
	ListBindingsByConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, ListOptions) ([]core.TargetBinding, error)
	ListBindingsByConnector(context.Context, uuid.UUID, uuid.UUID, core.Identifier, ListOptions) ([]core.TargetBinding, error)
	SetBindingStatus(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, core.BindingStatus, int) (core.TargetBinding, error)
}

type GormRepository struct {
	db *gorm.DB
}

var (
	_ Repository                      = (*GormRepository)(nil)
	_ LifecycleMutationRepository     = (*GormRepository)(nil)
	_ AuthorizationMutationRepository = (*GormRepository)(nil)
)

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

// UpsertBindingWithLifecycle keeps the canonical binding, automatic audit,
// in one database transaction.
func (repository *GormRepository) UpsertBindingWithLifecycle(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	binding core.TargetBinding,
	expectedRevision int,
	mutation BindingLifecycleMutation,
) (core.TargetBinding, bool, error) {
	return repository.upsertBindingWithLifecycle(ctx, organizationID, ownerAccountID, binding, expectedRevision, mutation, false)
}

func (repository *GormRepository) upsertBindingWithLifecycle(ctx context.Context, organizationID, ownerAccountID uuid.UUID, binding core.TargetBinding, expectedRevision int, mutation BindingLifecycleMutation, replaceAuthorizedConstraint bool) (core.TargetBinding, bool, error) {
	if err := repository.available(); err != nil {
		return core.TargetBinding{}, false, err
	}
	prepared, err := prepareNativeBinding(organizationID, ownerAccountID, binding, expectedRevision)
	if err != nil || !validBindingLifecycleMutation(mutation, MutationBindingUpserted) {
		return core.TargetBinding{}, false, invalidInputError("upsert connector target binding lifecycle")
	}

	var stored core.TargetBinding
	created := false
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		authorityInputs := []core.TargetBinding{prepared}
		observed, observedErr := getBindingByLogicalKey(ctx, tx, prepared, false)
		if observedErr == nil {
			observedBinding, conversionErr := observed.toDomain()
			if conversionErr != nil {
				return conversionErr
			}
			authorityInputs = append(authorityInputs, observedBinding)
		} else if !core.IsErrorCode(observedErr, core.ErrorCodeNotFound) {
			return observedErr
		}
		authorities, authorityErr := lockBindingLifecycleAuthorities(ctx, tx, authorityInputs...)
		if authorityErr != nil {
			return authorityErr
		}

		var before *core.TargetBinding
		existing, lookupErr := getBindingByLogicalKey(ctx, tx, prepared, true)
		if lookupErr == nil {
			value, conversionErr := existing.toDomain()
			if conversionErr != nil {
				return conversionErr
			}
			if _, locked := authorities[bindingLifecycleAuthorityIdentity(value)]; !locked {
				return conflictError("lock connector target binding replacement authority")
			}
			before = &value
		} else if !core.IsErrorCode(lookupErr, core.ErrorCodeNotFound) {
			return lookupErr
		}

		nested := &GormRepository{db: tx.Session(&gorm.Session{DisableNestedTransaction: true})}
		if replaceAuthorizedConstraint {
			if before == nil || before.ID != prepared.ID || before.ConnectionID != prepared.ConnectionID || before.Revision != expectedRevision {
				return conflictError("replace DigitalHuman binding authority")
			}
			// Only the explicitly selected operation subset is replaceable. Existing
			// expiry, deny reasons, owner and host boundaries remain restrictive.
			constraint, err := core.IntersectConstraints(before.Constraint, prepared.Constraint)
			if err != nil {
				return err
			}
			constraint.AllowedOperationIDs = append([]core.Identifier{}, prepared.Constraint.AllowedOperationIDs...)
			prepared.Constraint = constraint
			if err := requireAuthorizedDigitalHumanConnection(ctx, tx, prepared); err != nil {
				return err
			}
		}
		var writeErr error
		stored, created, writeErr = nested.upsertBinding(
			ctx, organizationID, ownerAccountID, prepared, expectedRevision, replaceAuthorizedConstraint,
		)
		if writeErr != nil {
			return writeErr
		}
		changed := created || (before != nil && bindingLifecycleChanged(*before, stored))
		if !changed {
			return nil
		}
		// Record both sides of a replacement in the same transaction.
		if before != nil && before.ConnectionID != stored.ConnectionID {
			detached, detachedMutation := replacementDetachLifecycle(*before, stored, mutation)
			_, locked := authorities[bindingLifecycleAuthorityIdentity(detached)]
			if !locked {
				return conflictError("lock replaced connector target binding authority")
			}
			if err := appendBindingLifecycleMutationLocked(ctx, tx, detached, detachedMutation); err != nil {
				return err
			}
		}
		_, locked := authorities[bindingLifecycleAuthorityIdentity(stored)]
		if !locked {
			return conflictError("lock connector target binding authority")
		}
		return appendBindingLifecycleMutationLocked(ctx, tx, stored, mutation)
	})
	if err != nil {
		return core.TargetBinding{}, false, err
	}
	return stored, created, nil
}

// SetBindingStatusWithLifecycle is the status-CAS equivalent of
// UpsertBindingWithLifecycle. A successful detach is never observable without
// its automatic audit and, when applicable, migration tombstone.
func (repository *GormRepository) SetBindingStatusWithLifecycle(
	ctx context.Context,
	organizationID, ownerAccountID, bindingID uuid.UUID,
	status core.BindingStatus,
	expectedRevision int,
	mutation BindingLifecycleMutation,
) (core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return core.TargetBinding{}, err
	}
	if status != core.BindingStatusDisabled ||
		!validBindingLifecycleMutation(mutation, MutationBindingDetached) {
		return core.TargetBinding{}, invalidInputError("detach connector target binding lifecycle")
	}

	var stored core.TargetBinding
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		observed, lookupErr := getBindingByID(ctx, tx, organizationID, ownerAccountID, bindingID, false)
		if lookupErr != nil {
			return lookupErr
		}
		observedBinding, conversionErr := observed.toDomain()
		if conversionErr != nil {
			return conversionErr
		}
		authorities, authorityErr := lockBindingLifecycleAuthorities(ctx, tx, observedBinding)
		if authorityErr != nil {
			return authorityErr
		}
		nested := &GormRepository{db: tx.Session(&gorm.Session{DisableNestedTransaction: true})}
		var writeErr error
		stored, writeErr = nested.SetBindingStatus(
			ctx, organizationID, ownerAccountID, bindingID, status, expectedRevision,
		)
		if writeErr != nil {
			return writeErr
		}
		_, locked := authorities[bindingLifecycleAuthorityIdentity(stored)]
		if !locked {
			return conflictError("lock detached connector target binding authority")
		}
		return appendBindingLifecycleMutationLocked(ctx, tx, stored, mutation)
	})
	if err != nil {
		return core.TargetBinding{}, err
	}
	return stored, nil
}

// UpdateBindingAuthorizationWithLifecycle persists an explicit authorization
// mode change with owner scope, revision CAS, and the normal lifecycle audit.
func (repository *GormRepository) UpdateBindingAuthorizationWithLifecycle(
	ctx context.Context,
	organizationID, ownerAccountID, bindingID uuid.UUID,
	constraint *core.AccessConstraint,
	expectedRevision int,
	mutation BindingLifecycleMutation,
) (core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return core.TargetBinding{}, err
	}
	if constraint == nil || constraint.Validate() != nil || expectedRevision < 1 ||
		!validBindingLifecycleMutation(mutation, MutationBindingUpserted) {
		return core.TargetBinding{}, invalidInputError("update connector target binding authorization lifecycle")
	}
	encoded, err := json.Marshal(constraint)
	if err != nil {
		return core.TargetBinding{}, internalError("encode connector target binding authorization")
	}
	var stored core.TargetBinding
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		observed, lookupErr := getBindingByID(ctx, tx, organizationID, ownerAccountID, bindingID, true)
		if lookupErr != nil {
			return lookupErr
		}
		before, conversionErr := observed.toDomain()
		if conversionErr != nil {
			return conversionErr
		}
		if before.Status != core.BindingStatusActive || before.Revision != expectedRevision {
			return conflictError("update connector target binding authorization")
		}
		authorities, authorityErr := lockBindingLifecycleAuthorities(ctx, tx, before)
		if authorityErr != nil {
			return authorityErr
		}
		now := time.Now().UTC()
		result := tx.Model(&bindingRecord{}).
			Where("organization_id = ? AND owner_account_id = ? AND id = ? AND revision = ?", organizationID, ownerAccountID, bindingID, expectedRevision).
			Updates(map[string]any{"binding_constraint": datatypes.JSON(encoded), "revision": expectedRevision + 1, "updated_at": now})
		if result.Error != nil {
			return internalError("update connector target binding authorization")
		}
		if result.RowsAffected != 1 {
			return conflictError("update connector target binding authorization")
		}
		before.Constraint = constraint
		before.Revision = expectedRevision + 1
		before.UpdatedAt = now
		stored = before
		if _, locked := authorities[bindingLifecycleAuthorityIdentity(stored)]; !locked {
			return conflictError("lock connector target binding authorization authority")
		}
		return appendBindingLifecycleMutationLocked(ctx, tx, stored, mutation)
	})
	if err != nil {
		return core.TargetBinding{}, err
	}
	return stored, nil
}

// UpsertBinding creates or replaces one native binding by its stable logical
// key: organization, owner, target type/id, Connector, and (for Workflows)
// binding instance. expectedRevision is zero for creation/idempotent replay and
// must match the stored revision for a material replacement. An existing UUID
// always wins over a different UUID supplied by a retry.
func (repository *GormRepository) UpsertBinding(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	binding core.TargetBinding,
	expectedRevision int,
) (core.TargetBinding, bool, error) {
	return repository.upsertBinding(ctx, organizationID, ownerAccountID, binding, expectedRevision, false)
}

func (repository *GormRepository) upsertBinding(ctx context.Context, organizationID, ownerAccountID uuid.UUID, binding core.TargetBinding, expectedRevision int, replaceAuthorizedConstraint bool) (core.TargetBinding, bool, error) {
	if err := repository.available(); err != nil {
		return core.TargetBinding{}, false, err
	}
	prepared, err := prepareNativeBinding(organizationID, ownerAccountID, binding, expectedRevision)
	if err != nil {
		return core.TargetBinding{}, false, err
	}
	if prepared.Constraint == nil {
		prepared.Constraint = NewBindingConstraint(prepared.OwnerAccountID, prepared.ConnectorID)
	}
	record, err := bindingRecordFromDomain(prepared)
	if err != nil {
		return core.TargetBinding{}, false, err
	}

	var stored core.TargetBinding
	created := false
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireReadyConnection(ctx, tx, organizationID, ownerAccountID, prepared.ConnectionID, prepared.ConnectorID); err != nil {
			return err
		}

		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			if isDuplicateDatabaseError(result.Error) {
				return conflictError("upsert connector target binding")
			}
			return internalError("upsert connector target binding")
		}
		if result.RowsAffected == 1 {
			created = true
			var conversionErr error
			stored, conversionErr = record.toDomain()
			return conversionErr
		}

		existing, lookupErr := getBindingByLogicalKey(ctx, tx, prepared, true)
		if lookupErr != nil {
			// A different unique identity (for example the requested UUID) won
			// the conflict but the logical row does not exist.
			if core.IsErrorCode(lookupErr, core.ErrorCodeNotFound) {
				return conflictError("upsert connector target binding")
			}
			return lookupErr
		}
		existingConstraint, constraintErr := core.DecodeConstraint(existing.Constraint)
		if constraintErr != nil {
			return constraintErr
		}
		narrowed, constraintErr := core.IntersectConstraints(existingConstraint, prepared.Constraint)
		if constraintErr != nil {
			return constraintErr
		}
		if replaceAuthorizedConstraint {
			narrowed = prepared.Constraint
		} else {
			// The prepared binding is the current explicit authorization choice.
			// Constraint intersection still narrows operations/targets, but the
			// mode itself must support an intentional direct grant or revocation;
			// otherwise a legacy ask_each_time row could never be changed.
			narrowed.AuthorizationMode = prepared.Constraint.AuthorizationModeValue()
			if narrowed.AuthorizationMode == core.AuthorizationModeDirect {
				narrowed.AuthorizationConnectionRevision = prepared.Constraint.AuthorizationConnectionRevision
				narrowed.AuthorizationCredentialVersion = prepared.Constraint.AuthorizationCredentialVersion
				narrowed.AuthorizationContractDigest = prepared.Constraint.AuthorizationContractDigest
			} else {
				narrowed.AuthorizationConnectionRevision = 0
				narrowed.AuthorizationCredentialVersion = 0
				narrowed.AuthorizationContractDigest = ""
			}
		}
		record.Constraint, constraintErr = json.Marshal(narrowed)
		if constraintErr != nil {
			return internalError("encode Connector restrictions")
		}
		if equivalentNativeBinding(existing, record) && !replaceAuthorizedConstraint {
			stored, lookupErr = existing.toDomain()
			return lookupErr
		}
		if prepared.Target.EffectiveScope() == core.ScopePublished {
			return conflictError("published Connector snapshot is immutable")
		}
		if expectedRevision < 1 || existing.Revision != expectedRevision {
			return conflictError("upsert connector target binding")
		}

		now := time.Now().UTC()
		disabledAt := disabledTimeForStatus(prepared.Status, prepared.DisabledAt, now)
		result = tx.Model(&bindingRecord{}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND id = ? AND revision = ?",
				organizationID,
				ownerAccountID,
				existing.ID,
				expectedRevision,
			).
			Updates(map[string]any{
				"connection_id":      prepared.ConnectionID,
				"binding_constraint": record.Constraint,
				"status":             prepared.Status,
				"disabled_at":        disabledAt,
				"revision":           expectedRevision + 1,
				"updated_at":         now,
			})
		if result.Error != nil {
			return internalError("upsert connector target binding")
		}
		if result.RowsAffected != 1 {
			return conflictError("upsert connector target binding")
		}

		existing.Constraint = record.Constraint
		existing.ConnectionID = prepared.ConnectionID
		existing.Status = string(prepared.Status)
		existing.DisabledAt = cloneTime(disabledAt)
		existing.Revision = expectedRevision + 1
		existing.UpdatedAt = now
		stored, lookupErr = existing.toDomain()
		return lookupErr
	})
	if err != nil {
		return core.TargetBinding{}, false, err
	}
	return stored, created, nil
}

func (repository *GormRepository) GetBinding(
	ctx context.Context,
	organizationID, ownerAccountID, bindingID uuid.UUID,
) (core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return core.TargetBinding{}, err
	}
	if !validOwnerScope(organizationID, ownerAccountID) || bindingID == uuid.Nil {
		return core.TargetBinding{}, invalidInputError("get connector target binding")
	}
	record, err := getBindingByID(ctx, repository.db, organizationID, ownerAccountID, bindingID, false)
	if err != nil {
		return core.TargetBinding{}, err
	}
	return record.toDomain()
}

// ListBindingsByTarget returns all binding instances for one concrete target.
// For a Workflow this intentionally includes every node/placement so editor
// round trips and impact displays cannot silently drop repeated placements.
func (repository *GormRepository) ListBindingsByTarget(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	targetType core.TargetType,
	targetID string,
	options ListOptions,
) ([]core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if !validOwnerScope(organizationID, ownerAccountID) || !targetType.Valid() || !validExactText(targetID) || !validListOptions(options) {
		return nil, invalidInputError("list connector target bindings by target")
	}
	query := repository.db.WithContext(ctx).Where(
		"organization_id = ? AND owner_account_id = ? AND target_type = ? AND target_id = ?",
		organizationID,
		ownerAccountID,
		targetType,
		targetID,
	)
	query = applyListOptions(query, options)
	return findBindings(query.Order("connector_id ASC, binding_instance_id ASC NULLS FIRST, created_at ASC, id ASC"), "list connector target bindings by target")
}

func (repository *GormRepository) ListPublishedHostDelegations(ctx context.Context, organizationID uuid.UUID, target core.TargetRef, audience core.HostAudience) ([]core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if organizationID == uuid.Nil || (target.Type != core.TargetTypeAgent && target.Type != core.TargetTypeDigitalHuman) ||
		target.EffectiveScope() != core.ScopePublished || target.Validate() != nil ||
		(audience != core.HostAudienceWorkspace && audience != core.HostAudienceAPI && audience != core.HostAudienceChannel) {
		return nil, invalidInputError("list published host delegations")
	}
	grant, err := json.Marshal(map[string]any{"host_delegation": map[string]any{"audiences": []core.HostAudience{audience}}})
	if err != nil {
		return nil, internalError("encode host delegation audience")
	}
	var records []bindingRecord
	err = repository.db.WithContext(ctx).Where(
		"organization_id = ? AND target_type = ? AND target_id = ? AND target_scope = ? AND target_version_id = ? AND status = ?",
		organizationID, target.Type, target.ID, core.ScopePublished, target.VersionID, core.BindingStatusActive,
	).Where("binding_constraint @> ?::jsonb", datatypes.JSON(grant)).
		Order("connector_id ASC, binding_instance_id ASC NULLS FIRST, id ASC").Limit(51).Find(&records).Error
	if err != nil || len(records) > 50 {
		return nil, internalError("list published host delegations")
	}
	bindings := make([]core.TargetBinding, 0, len(records))
	for _, record := range records {
		binding, err := record.toDomain()
		if err != nil || binding.OrganizationID != organizationID || binding.Target.Type != target.Type ||
			binding.Target.ID != target.ID || binding.Target.EffectiveScope() != core.ScopePublished || binding.Target.VersionID != target.VersionID ||
			binding.Status != core.BindingStatusActive || binding.Constraint == nil || binding.Constraint.HostDelegation == nil ||
			binding.Constraint.HostDelegation.GrantedBy != binding.OwnerAccountID {
			return nil, internalError("validate published host delegation")
		}
		allowed := false
		for _, operation := range binding.Constraint.AllowedOperationIDs {
			if binding.Constraint.AllowsHostAudience(binding.OwnerAccountID, operation, binding.Target, audience, time.Now().UTC()) {
				allowed = true
				break
			}
		}
		if allowed {
			bindings = append(bindings, binding)
		}
	}
	return bindings, nil
}

// ListBindingsByConnection supplies the complete owner-visible removal impact;
// callers can restrict it to active states with ListOptions when required.
func (repository *GormRepository) ListBindingsByConnection(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	options ListOptions,
) ([]core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if !validOwnerScope(organizationID, ownerAccountID) || connectionID == uuid.Nil || !validListOptions(options) {
		return nil, invalidInputError("list connector target bindings by connection")
	}
	query := repository.db.WithContext(ctx).Where(
		"organization_id = ? AND owner_account_id = ? AND connection_id = ?",
		organizationID,
		ownerAccountID,
		connectionID,
	)
	query = applyListOptions(query, options)
	return findBindings(query.Order("target_type ASC, target_id ASC, binding_instance_id ASC NULLS FIRST, created_at ASC, id ASC"), "list connector target bindings by connection")
}

// ListBindingsByConnector supplies Connector disable/remove impact without
// widening the Personal Connection owner boundary.
func (repository *GormRepository) ListBindingsByConnector(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	connectorID core.Identifier,
	options ListOptions,
) ([]core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return nil, err
	}
	if !validOwnerScope(organizationID, ownerAccountID) || !connectorID.Valid() || !validListOptions(options) {
		return nil, invalidInputError("list connector target bindings by connector")
	}
	query := repository.db.WithContext(ctx).Where(
		"organization_id = ? AND owner_account_id = ? AND connector_id = ?",
		organizationID,
		ownerAccountID,
		connectorID,
	)
	query = applyListOptions(query, options)
	return findBindings(query.Order("target_type ASC, target_id ASC, binding_instance_id ASC NULLS FIRST, created_at ASC, id ASC"), "list connector target bindings by connector")
}

// SetBindingStatus performs a native-only compare-and-swap. Re-enabling a
// binding also rechecks that its exact owner-scoped Connection is ready.
func (repository *GormRepository) SetBindingStatus(
	ctx context.Context,
	organizationID, ownerAccountID, bindingID uuid.UUID,
	status core.BindingStatus,
	expectedRevision int,
) (core.TargetBinding, error) {
	if err := repository.available(); err != nil {
		return core.TargetBinding{}, err
	}
	if !validOwnerScope(organizationID, ownerAccountID) || bindingID == uuid.Nil || !status.Valid() || expectedRevision < 1 {
		return core.TargetBinding{}, invalidInputError("set connector target binding status")
	}

	var binding core.TargetBinding
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := getBindingByID(ctx, tx, organizationID, ownerAccountID, bindingID, true)
		if err != nil {
			return err
		}
		if record.Revision != expectedRevision {
			return conflictError("set connector target binding status")
		}
		if record.Status == string(status) {
			binding, err = record.toDomain()
			return err
		}
		if status == core.BindingStatusActive {
			connectorID, normalizeErr := core.NormalizeIdentifier(record.ConnectorID)
			if normalizeErr != nil {
				return internalError("set connector target binding status")
			}
			if err := requireReadyConnection(ctx, tx, organizationID, ownerAccountID, record.ConnectionID, connectorID); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		disabledAt := disabledTimeForStatus(status, record.DisabledAt, now)
		result := tx.Model(&bindingRecord{}).
			Where(
				"organization_id = ? AND owner_account_id = ? AND id = ? AND revision = ?",
				organizationID,
				ownerAccountID,
				bindingID,
				expectedRevision,
			).
			Updates(map[string]any{
				"status":      status,
				"disabled_at": disabledAt,
				"revision":    expectedRevision + 1,
				"updated_at":  now,
			})
		if result.Error != nil {
			return internalError("set connector target binding status")
		}
		if result.RowsAffected != 1 {
			return conflictError("set connector target binding status")
		}

		record.Status = string(status)
		record.DisabledAt = cloneTime(disabledAt)
		record.Revision = expectedRevision + 1
		record.UpdatedAt = now
		binding, err = record.toDomain()
		return err
	})
	if err != nil {
		return core.TargetBinding{}, err
	}
	return binding, nil
}

func validBindingLifecycleMutation(mutation BindingLifecycleMutation, expectedType string) bool {
	correlationID := strings.TrimSpace(mutation.CorrelationID)
	return mutation.ID != uuid.Nil && mutation.MutationType == expectedType &&
		correlationID == mutation.CorrelationID && correlationID != "" && len(correlationID) <= 128 &&
		!mutation.CreatedAt.IsZero()
}

func bindingLifecycleChanged(before, after core.TargetBinding) bool {
	return before.ID != after.ID || before.ConnectionID != after.ConnectionID ||
		before.Status != after.Status || before.Revision != after.Revision ||
		(before.DisabledAt == nil) != (after.DisabledAt == nil)
}

func replacementDetachLifecycle(
	before, after core.TargetBinding,
	upsert BindingLifecycleMutation,
) (core.TargetBinding, BindingLifecycleMutation) {
	detachedAt := upsert.CreatedAt.UTC()
	before.Status = core.BindingStatusDisabled
	before.DisabledAt = &detachedAt
	before.Revision = after.Revision
	before.UpdatedAt = detachedAt
	detach := BindingLifecycleMutation{
		ID:            uuid.NewSHA1(upsert.ID, []byte("binding-replacement-detached:"+before.ConnectionID.String())),
		MutationType:  MutationBindingDetached,
		CorrelationID: "binding-replacement-detached:" + upsert.ID.String(),
		CreatedAt:     detachedAt,
	}
	return before, detach
}

func bindingLifecycleAuthorityIdentity(binding core.TargetBinding) connectionLockIdentity {
	return connectionLockIdentity{
		OrganizationID: binding.OrganizationID,
		OwnerAccountID: binding.OwnerAccountID,
		ConnectionID:   binding.ConnectionID,
		ConnectorID:    binding.ConnectorID,
	}
}

type connectionLockIdentity struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectionID   uuid.UUID
	ConnectorID    core.Identifier
}
type bindingLifecycleAuthoritySet map[connectionLockIdentity]struct{}

func lockBindingLifecycleAuthorities(
	ctx context.Context,
	tx *gorm.DB,
	bindings ...core.TargetBinding,
) (bindingLifecycleAuthoritySet, error) {
	identities := make([]connectionLockIdentity, 0, len(bindings))
	for _, binding := range bindings {
		identities = append(identities, bindingLifecycleAuthorityIdentity(binding))
	}
	return lockBindingLifecycleIdentities(ctx, tx, identities)
}

func lockBindingLifecycleIdentities(
	ctx context.Context,
	tx *gorm.DB,
	identities []connectionLockIdentity,
) (bindingLifecycleAuthoritySet, error) {
	if len(identities) == 0 {
		return bindingLifecycleAuthoritySet{}, nil
	}
	ordered := append([]connectionLockIdentity(nil), identities...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ConnectionID.String() < ordered[j].ConnectionID.String() })
	locked := bindingLifecycleAuthoritySet{}
	for _, identity := range ordered {
		if _, ok := locked[identity]; ok {
			continue
		}
		var row struct{ ID uuid.UUID }
		err := tx.WithContext(ctx).Table("connector_connections").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ?", identity.ConnectionID, identity.OrganizationID, identity.OwnerAccountID, identity.ConnectorID).Take(&row).Error
		if err != nil || row.ID != identity.ConnectionID {
			return nil, connectionNotReadyError("lock native connector connection")
		}
		locked[identity] = struct{}{}
	}
	return locked, nil
}

func appendBindingLifecycleMutationLocked(
	ctx context.Context,
	tx *gorm.DB,
	binding core.TargetBinding,
	mutation BindingLifecycleMutation,
) error {
	if binding.Validate() != nil ||
		!validBindingLifecycleMutation(mutation, mutation.MutationType) ||
		(mutation.MutationType != MutationBindingUpserted && mutation.MutationType != MutationBindingDetached) {
		return invalidInputError("record connector target binding lifecycle")
	}
	details := bindingLifecycleDetails(binding)
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return internalError("record connector target binding lifecycle")
	}
	decision := bindingLifecycleDecisionRecord{
		TargetScope: string(binding.Target.EffectiveScope()), TargetVersionID: binding.Target.VersionID,
		ID: mutation.ID, OrganizationID: binding.OrganizationID, OwnerAccountID: binding.OwnerAccountID,
		ActingPrincipalID: binding.OwnerAccountID, ConnectorID: binding.ConnectorID.String(),
		ConnectionID: binding.ConnectionID, TargetType: string(binding.Target.Type), TargetID: binding.Target.ID,
		BindingInstanceID: optionalBindingText(binding.Target.BindingInstanceID), Decision: mutation.MutationType,
		Status: bindingLifecycleDecisionSuccessful, CorrelationID: mutation.CorrelationID,
		Details: datatypes.JSON(detailsJSON), CreatedAt: mutation.CreatedAt.UTC(),
	}
	if err := createOrVerifyBindingLifecycleDecision(ctx, tx, decision); err != nil {
		return err
	}
	return nil
}

func createOrVerifyBindingLifecycleDecision(
	ctx context.Context,
	tx *gorm.DB,
	decision bindingLifecycleDecisionRecord,
) error {
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&decision)
	if result.Error != nil {
		return internalError("record connector target binding lifecycle")
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing bindingLifecycleDecisionRecord
	err := tx.WithContext(ctx).Where(
		"organization_id = ? AND correlation_id = ? AND decision = ? AND status = ?",
		decision.OrganizationID, decision.CorrelationID, decision.Decision, decision.Status,
	).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !sameBindingLifecycleDecision(existing, decision) {
		return conflictError("replay connector target binding lifecycle")
	}
	if err != nil {
		return internalError("verify connector target binding lifecycle replay")
	}
	return nil
}

func sameBindingLifecycleDecision(left, right bindingLifecycleDecisionRecord) bool {
	left.ID, right.ID = uuid.Nil, uuid.Nil
	leftCreatedAt, rightCreatedAt := left.CreatedAt.UTC().Truncate(time.Microsecond), right.CreatedAt.UTC().Truncate(time.Microsecond)
	left.CreatedAt, right.CreatedAt = time.Time{}, time.Time{}
	leftDetails, rightDetails := left.Details, right.Details
	left.Details, right.Details = nil, nil
	return leftCreatedAt.Equal(rightCreatedAt) && reflect.DeepEqual(left, right) && equivalentBindingLifecycleJSON(leftDetails, rightDetails)
}

func equivalentBindingLifecycleJSON(left, right datatypes.JSON) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func bindingLifecycleDetails(binding core.TargetBinding) map[string]any {
	details := map[string]any{
		"binding_id": binding.ID.String(), "target_type": string(binding.Target.Type), "target_id": binding.Target.ID,
		"binding_instance_id": binding.Target.BindingInstanceID, "status": string(binding.Status),
		"authorization_mode": binding.Constraint.AuthorizationModeValue(),
		"revision":           binding.Revision, "created_at": binding.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": binding.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if binding.DisabledAt != nil {
		details["disabled_at"] = binding.DisabledAt.UTC().Format(time.RFC3339Nano)
	}
	return details
}

func optionalBindingText(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func (repository *GormRepository) available() error {
	if repository == nil || repository.db == nil {
		return internalError("connector target binding repository unavailable")
	}
	return nil
}

func prepareNativeBinding(
	organizationID, ownerAccountID uuid.UUID,
	binding core.TargetBinding,
	expectedRevision int,
) (core.TargetBinding, error) {
	if !validOwnerScope(organizationID, ownerAccountID) ||
		binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerAccountID ||
		expectedRevision < 0 ||
		!validExactTarget(binding.Target) {
		return core.TargetBinding{}, invalidInputError("upsert connector target binding")
	}
	if binding.ID == uuid.Nil {
		binding.ID = uuid.New()
	}
	if expectedRevision == 0 {
		if binding.Revision == 0 {
			binding.Revision = 1
		}
		if binding.Revision != 1 {
			return core.TargetBinding{}, invalidInputError("upsert connector target binding")
		}
	} else if binding.Revision != expectedRevision {
		return core.TargetBinding{}, invalidInputError("upsert connector target binding")
	}
	if binding.Status == core.BindingStatusActive && binding.DisabledAt != nil {
		return core.TargetBinding{}, invalidInputError("upsert connector target binding")
	}
	now := time.Now().UTC()
	if binding.Status != core.BindingStatusActive && binding.DisabledAt == nil {
		binding.DisabledAt = &now
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = binding.CreatedAt
	}
	if err := binding.Validate(); err != nil {
		return core.TargetBinding{}, invalidInputError("upsert connector target binding")
	}
	return binding, nil
}

func requireReadyConnection(
	ctx context.Context,
	db *gorm.DB,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectorID core.Identifier,
) error {
	if connectionID == uuid.Nil || !connectorID.Valid() {
		return invalidInputError("validate connector target binding connection")
	}
	var connection struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	err := db.WithContext(ctx).
		Table("connector_connections").
		Select("id").
		Where(
			"id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND state = ? AND deleted_at IS NULL",
			connectionID,
			organizationID,
			ownerAccountID,
			connectorID,
			core.ReadinessReady,
		).
		Take(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return connectionNotReadyError("validate connector target binding connection")
	}
	if err != nil {
		return internalError("validate connector target binding connection")
	}
	return nil
}

func getBindingByID(
	ctx context.Context,
	db *gorm.DB,
	organizationID, ownerAccountID, bindingID uuid.UUID,
	lock bool,
) (bindingRecord, error) {
	query := db.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record bindingRecord
	err := query.Where(
		"organization_id = ? AND owner_account_id = ? AND id = ?",
		organizationID,
		ownerAccountID,
		bindingID,
	).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return bindingRecord{}, notFoundError("get connector target binding")
	}
	if err != nil {
		return bindingRecord{}, internalError("get connector target binding")
	}
	return record, nil
}

func getBindingByLogicalKey(ctx context.Context, db *gorm.DB, binding core.TargetBinding, lock bool) (bindingRecord, error) {
	query := db.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	query = query.Where(
		"organization_id = ? AND owner_account_id = ? AND target_type = ? AND target_id = ? AND connector_id = ? AND target_scope = ? AND target_version_id = ?",
		binding.OrganizationID,
		binding.OwnerAccountID,
		binding.Target.Type,
		binding.Target.ID,
		binding.ConnectorID, binding.Target.EffectiveScope(), binding.Target.VersionID,
	)
	if binding.Target.BindingInstanceID == "" {
		query = query.Where("binding_instance_id IS NULL")
	} else {
		query = query.Where("binding_instance_id = ?", binding.Target.BindingInstanceID)
	}
	var record bindingRecord
	err := query.Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return bindingRecord{}, notFoundError("get connector target binding by logical key")
	}
	if err != nil {
		return bindingRecord{}, internalError("get connector target binding by logical key")
	}
	return record, nil
}

func applyListOptions(query *gorm.DB, options ListOptions) *gorm.DB {
	if options.Scope != "" {
		query = query.Where("target_scope = ? AND target_version_id = ?", options.Scope, options.VersionID)
	}
	if len(options.Statuses) > 0 {
		query = query.Where("status IN ?", options.Statuses)
	}
	return query
}

func findBindings(query *gorm.DB, operation string) ([]core.TargetBinding, error) {
	var records []bindingRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, internalError(operation)
	}
	bindings := make([]core.TargetBinding, 0, len(records))
	for _, record := range records {
		binding, err := record.toDomain()
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func validOwnerScope(organizationID, ownerAccountID uuid.UUID) bool {
	return organizationID != uuid.Nil && ownerAccountID != uuid.Nil
}

func validExactText(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 255
}

func validExactTarget(target core.TargetRef) bool {
	if err := target.Validate(); err != nil || !validExactText(target.ID) {
		return false
	}
	return target.BindingInstanceID == "" || validExactText(target.BindingInstanceID)
}

func validListOptions(options ListOptions) bool {
	for _, status := range options.Statuses {
		if !status.Valid() {
			return false
		}
	}
	return true
}

func equivalentNativeBinding(existing, desired bindingRecord) bool {
	return equivalentConstraintJSON(existing.Constraint, desired.Constraint) && existing.ConnectionID == desired.ConnectionID &&
		existing.Status == desired.Status
}

func disabledTimeForStatus(status core.BindingStatus, current *time.Time, now time.Time) *time.Time {
	if status == core.BindingStatusActive {
		return nil
	}
	if current != nil {
		return cloneTime(current)
	}
	return &now
}

func invalidInputError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeInvalidInput, false, nil))
}

func notFoundError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeNotFound, false, nil))
}

func conflictError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeConflict, false, nil))
}

func connectionNotReadyError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeConnectionNotReady, false, nil))
}

func internalError(operation string) error {
	return fmt.Errorf("%s: %w", operation, core.NewError(core.ErrorCodeInternal, true, nil))
}

func isDuplicateDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "violates unique constraint") ||
		strings.Contains(message, "unique constraint failed")
}

type bindingRecord struct {
	TargetScope       string         `gorm:"column:target_scope;not null"`
	TargetVersionID   string         `gorm:"column:target_version_id;not null"`
	Constraint        datatypes.JSON `gorm:"column:binding_constraint;type:jsonb" json:"-"`
	ID                uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	OrganizationID    uuid.UUID      `gorm:"column:organization_id;type:uuid;not null;uniqueIndex:idx_connector_target_bindings_logical_key,priority:1;index:idx_connector_target_bindings_connection,priority:1;index:idx_connector_target_bindings_target,priority:1"`
	OwnerAccountID    uuid.UUID      `gorm:"column:owner_account_id;type:uuid;not null;uniqueIndex:idx_connector_target_bindings_logical_key,priority:2;index:idx_connector_target_bindings_connection,priority:2"`
	ConnectionID      uuid.UUID      `gorm:"column:connection_id;type:uuid;not null;index:idx_connector_target_bindings_connection,priority:3"`
	ConnectorID       string         `gorm:"column:connector_id;type:varchar(128);not null;uniqueIndex:idx_connector_target_bindings_logical_key,priority:5"`
	TargetType        string         `gorm:"column:target_type;type:varchar(16);not null;uniqueIndex:idx_connector_target_bindings_logical_key,priority:3;index:idx_connector_target_bindings_target,priority:2"`
	TargetID          string         `gorm:"column:target_id;type:varchar(255);not null;uniqueIndex:idx_connector_target_bindings_logical_key,priority:4;index:idx_connector_target_bindings_target,priority:3"`
	BindingInstanceID *string        `gorm:"column:binding_instance_id;type:varchar(255)"`
	Status            string         `gorm:"column:status;type:varchar(32);not null;index:idx_connector_target_bindings_connection,priority:4;index:idx_connector_target_bindings_target,priority:4"`

	Revision   int        `gorm:"column:revision;not null"`
	DisabledAt *time.Time `gorm:"column:disabled_at"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;not null"`
}

func (bindingRecord) TableName() string { return "connector_target_bindings" }

func bindingRecordFromDomain(binding core.TargetBinding) (bindingRecord, error) {
	constraint, err := json.Marshal(binding.Constraint)
	if err != nil {
		return bindingRecord{}, invalidInputError("encode native constraint")
	}
	if err := binding.Validate(); err != nil || !validExactTarget(binding.Target) {
		return bindingRecord{}, invalidInputError("persist connector target binding")
	}
	record := bindingRecord{
		Constraint:     datatypes.JSON(constraint),
		ID:             binding.ID,
		OrganizationID: binding.OrganizationID,
		OwnerAccountID: binding.OwnerAccountID,
		ConnectionID:   binding.ConnectionID,
		ConnectorID:    binding.ConnectorID.String(),
		TargetType:     string(binding.Target.Type),
		TargetID:       binding.Target.ID,
		TargetScope:    string(binding.Target.EffectiveScope()), TargetVersionID: binding.Target.VersionID,
		Status: string(binding.Status),

		Revision:   binding.Revision,
		DisabledAt: cloneTime(binding.DisabledAt),
		CreatedAt:  binding.CreatedAt,
		UpdatedAt:  binding.UpdatedAt,
	}
	if binding.Target.BindingInstanceID != "" {
		instanceID := binding.Target.BindingInstanceID
		record.BindingInstanceID = &instanceID
	}
	return record, nil
}

func (record bindingRecord) toDomain() (core.TargetBinding, error) {
	var constraint *core.AccessConstraint
	if len(record.Constraint) > 0 && string(record.Constraint) != "null" {
		var err error
		constraint, err = core.DecodeConstraint(record.Constraint)
		if err != nil {
			return core.TargetBinding{}, internalError("decode native constraint")
		}
	}
	connectorID, err := core.NormalizeIdentifier(record.ConnectorID)
	if err != nil {
		return core.TargetBinding{}, internalError("decode connector target binding identity")
	}
	binding := core.TargetBinding{
		Constraint:     constraint,
		ID:             record.ID,
		OrganizationID: record.OrganizationID,
		OwnerAccountID: record.OwnerAccountID,
		ConnectionID:   record.ConnectionID,
		ConnectorID:    connectorID,
		Target: core.TargetRef{
			Type: core.TargetType(record.TargetType),
			ID:   record.TargetID, Scope: core.TargetScope(record.TargetScope), VersionID: record.TargetVersionID,
		},
		Status: core.BindingStatus(record.Status),

		Revision:   record.Revision,
		DisabledAt: cloneTime(record.DisabledAt),
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
	if record.BindingInstanceID != nil {
		binding.Target.BindingInstanceID = *record.BindingInstanceID
	}
	if err := binding.Validate(); err != nil || !validExactTarget(binding.Target) {
		return core.TargetBinding{}, internalError("decode connector target binding")
	}
	return binding, nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func equivalentConstraintJSON(a, b []byte) bool {
	left, e := core.DecodeConstraint(a)
	if e != nil {
		return false
	}
	right, e := core.DecodeConstraint(b)
	return e == nil && core.EquivalentConstraints(left, right)
}
