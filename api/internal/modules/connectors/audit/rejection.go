package audit

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RejectExecution competes with confirmation finalization using the same lock.
// A resumed request must recheck its claim inside that lock before submitting.
func (repository *GormRepository) RejectExecution(ctx context.Context, decision core.LifecycleDecision) error {
	if repository == nil || repository.db == nil {
		return internalError()
	}
	if decision.Decision != "authorization" || decision.Status != core.DecisionStatusDenied ||
		decision.Target == nil || decision.LogicalOperationID == "" || decision.ReceiptID == "" {
		return invalidInputError()
	}
	details := make(map[string]any, len(decision.Details)+1)
	for key, value := range decision.Details {
		details[key] = value
	}
	details["rejection_finalized"] = true
	decision.Details = details
	record, err := newLifecycleDecisionRecord(decision)
	if err != nil {
		return err
	}
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", decision.OrganizationID.String()+"/"+decision.ReceiptID).Error; err != nil {
			return internalError()
		}
		var finalized int64
		if err := tx.Model(&lifecycleDecisionRecord{}).Where("organization_id = ? AND receipt_id = ? AND decision = ?", decision.OrganizationID, decision.ReceiptID, "confirmation_finalization").Count(&finalized).Error; err != nil {
			return internalError()
		}
		if finalized != 0 {
			return conflictError()
		}
		if err := requireOpenExecutionClaim(tx, decision); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return internalError()
		}
		if result.RowsAffected != 1 {
			return conflictError()
		}
		// The migration installs the identity-checked denied projection. Verify
		// it ran; an older schema must never report a durable rejection.
		var denied int64
		if err := matchingExecutionClaim(tx, decision).Where("state = ?", "denied").Count(&denied).Error; err != nil {
			return internalError()
		}
		if denied != 1 {
			return conflictError()
		}
		return nil
	})
	return preserveConnectorError(err)
}

func matchingExecutionClaim(tx *gorm.DB, decision core.LifecycleDecision) *gorm.DB {
	target := decision.Target
	return tx.Table("connector_invocation_claims").
		Where("organization_id = ? AND owner_account_id = ? AND acting_principal_id = ? AND receipt_id::text = ?", decision.OrganizationID, decision.OwnerAccountID, decision.ActingPrincipalID, decision.ReceiptID).
		Where("logical_operation_id = ? AND connector_id = ? AND connection_id = ? AND operation_id = ?", decision.LogicalOperationID, decision.ConnectorID, decision.ConnectionID, decision.OperationID).
		Where("target_type = ? AND target_id = ? AND target_scope = ? AND target_version_id = ? AND binding_instance_id = ?", target.Type, target.ID, target.EffectiveScope(), target.VersionID, target.BindingInstanceID)
}

func requireOpenExecutionClaim(tx *gorm.DB, decision core.LifecycleDecision) error {
	if decision.Target == nil {
		return invalidInputError()
	}
	var count int64
	if err := matchingExecutionClaim(tx, decision).Where("state IN ?", []string{"claimed", "awaiting_confirmation"}).Count(&count).Error; err != nil {
		return internalError()
	}
	if count != 1 {
		return conflictError()
	}
	return nil
}
