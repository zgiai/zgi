package bindings

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

// ReplaceDigitalHumanBindingWithLifecycle is the host-owned edit boundary for
// an existing draft resource. The host must validate its row identity, current
// editor and admitted catalog before calling it inside the resource transaction.
// Unlike an ambient upsert, this explicit edit can replace its operation subset;
// current personal-connection restrictions are rechecked under the lifecycle lock.
func (repository *GormRepository) ReplaceDigitalHumanBindingWithLifecycle(ctx context.Context, organizationID, ownerAccountID uuid.UUID, binding core.TargetBinding, expectedRevision int, mutation BindingLifecycleMutation) (core.TargetBinding, error) {
	if binding.Target.Type != core.TargetTypeDigitalHuman || binding.Target.EffectiveScope() != core.ScopeDraft || expectedRevision < 1 || binding.Constraint == nil {
		return core.TargetBinding{}, invalidInputError("replace DigitalHuman binding authority")
	}
	stored, _, err := repository.upsertBindingWithLifecycle(ctx, organizationID, ownerAccountID, binding, expectedRevision, mutation, true, false)
	return stored, err
}

func requireAuthorizedDigitalHumanConnection(ctx context.Context, tx *gorm.DB, binding core.TargetBinding) error {
	var row struct {
		Constraint []byte `gorm:"column:connection_constraint"`
		Operations []byte `gorm:"column:usable_operation_ids"`
	}
	err := tx.WithContext(ctx).Table("connector_connections").Select("connection_constraint, usable_operation_ids").
		Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND state = ? AND deleted_at IS NULL", binding.ConnectionID, binding.OrganizationID, binding.OwnerAccountID, binding.ConnectorID, core.ReadinessReady).Take(&row).Error
	if err != nil {
		return connectionNotReadyError("authorize DigitalHuman binding edit")
	}
	constraint, err := core.DecodeConstraint(row.Constraint)
	var operations []core.Identifier
	if err != nil || json.Unmarshal(row.Operations, &operations) != nil || len(binding.Constraint.AllowedOperationIDs) == 0 {
		return invalidInputError("authorize DigitalHuman binding edit")
	}
	for _, operation := range binding.Constraint.AllowedOperationIDs {
		if !slices.Contains(operations, operation) || !core.ConstraintsAllow(constraint, binding.Constraint, binding.OwnerAccountID, operation, binding.Target, time.Now().UTC()) {
			return core.NewError(core.ErrorCodeAccessDenied, false, nil)
		}
	}
	return nil
}

// replacedDigitalHumanConstraint keeps the existing expiry, deny reasons, owner
// and host boundaries, and takes the edit's explicit operation subset and
// approval policy (ADR 0026, ADR 0028) with its pinned snapshot. Direct
// authorization is never granted through this edit.
func replacedDigitalHumanConstraint(before, edited *core.AccessConstraint) (*core.AccessConstraint, error) {
	constraint, err := core.IntersectConstraints(before, edited)
	if err != nil {
		return nil, err
	}
	constraint.AllowedOperationIDs = append([]core.Identifier{}, edited.AllowedOperationIDs...)
	if mode := edited.AuthorizationModeValue(); mode != core.AuthorizationModeDirect {
		constraint.AuthorizationMode = mode
		constraint.AuthorizationConnectionRevision = 0
		constraint.AuthorizationCredentialVersion = 0
		constraint.AuthorizationContractDigest = ""
		if edited.Trusted() {
			constraint.AuthorizationConnectionRevision = edited.AuthorizationConnectionRevision
			constraint.AuthorizationCredentialVersion = edited.AuthorizationCredentialVersion
			constraint.AuthorizationContractDigest = edited.AuthorizationContractDigest
		}
	}
	return constraint, nil
}
