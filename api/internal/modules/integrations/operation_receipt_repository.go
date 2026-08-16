package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormOperationReceiptRepository struct{ db *gorm.DB }

func NewGormOperationReceiptRepository(db *gorm.DB) *GormOperationReceiptRepository {
	return &GormOperationReceiptRepository{db: db}
}

func (r *GormOperationReceiptRepository) Claim(ctx context.Context, receipt *OperationReceipt) (OperationReceiptClaim, error) {
	if r == nil || r.db == nil || receipt == nil {
		return OperationReceiptClaim{}, fmt.Errorf("integration operation receipt repository is unavailable")
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "operation_key"}},
		DoNothing: true,
	}).Create(receipt)
	if result.Error != nil {
		return OperationReceiptClaim{}, fmt.Errorf("claim integration operation: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return OperationReceiptClaim{Receipt: receipt, Claimed: true}, nil
	}
	existing := &OperationReceipt{}
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND operation_key = ?", receipt.OrganizationID, receipt.OperationKey).First(existing).Error; err != nil {
		return OperationReceiptClaim{}, fmt.Errorf("read claimed integration operation: %w", err)
	}
	if existing.Status == OperationReceiptStatusExecuting && !existing.LeaseExpiresAt.After(time.Now().UTC()) {
		if existing.ProviderStartedAt == nil {
			now := time.Now().UTC()
			updates := map[string]interface{}{
				"claim_token":      receipt.ClaimToken,
				"lease_expires_at": now.Add(operationReceiptLeaseDuration),
				"updated_at":       now,
			}
			takeover := r.db.WithContext(ctx).Model(&OperationReceipt{}).
				Where("id = ? AND status = ? AND provider_started_at IS NULL AND lease_expires_at <= CURRENT_TIMESTAMP", existing.ID, OperationReceiptStatusExecuting).
				Updates(updates)
			if takeover.Error != nil {
				return OperationReceiptClaim{}, fmt.Errorf("recover integration operation claim: %w", takeover.Error)
			}
			if takeover.RowsAffected == 1 {
				existing.ClaimToken = receipt.ClaimToken
				existing.LeaseExpiresAt = now.Add(operationReceiptLeaseDuration)
				return OperationReceiptClaim{Receipt: existing, Claimed: true}, nil
			}
			if err := r.db.WithContext(ctx).Where("organization_id = ? AND operation_key = ?", receipt.OrganizationID, receipt.OperationKey).First(existing).Error; err != nil {
				return OperationReceiptClaim{}, fmt.Errorf("reread integration operation claim: %w", err)
			}
		}
		if existing.Status != OperationReceiptStatusExecuting || existing.LeaseExpiresAt.After(time.Now().UTC()) {
			return OperationReceiptClaim{Receipt: existing}, nil
		}
		updates := map[string]interface{}{
			"status":     OperationReceiptStatusOutcomeUnknown,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}
		expire := r.db.WithContext(ctx).Model(&OperationReceipt{}).
			Where("id = ? AND status = ? AND lease_expires_at <= CURRENT_TIMESTAMP", existing.ID, OperationReceiptStatusExecuting).
			Updates(updates)
		if expire.Error != nil {
			return OperationReceiptClaim{}, fmt.Errorf("expire integration operation claim: %w", expire.Error)
		}
		if expire.RowsAffected == 0 {
			if err := r.db.WithContext(ctx).Where("organization_id = ? AND operation_key = ?", receipt.OrganizationID, receipt.OperationKey).First(existing).Error; err != nil {
				return OperationReceiptClaim{}, fmt.Errorf("reread expired integration operation claim: %w", err)
			}
			return OperationReceiptClaim{Receipt: existing}, nil
		}
		existing.Status = OperationReceiptStatusOutcomeUnknown
	}
	return OperationReceiptClaim{Receipt: existing}, nil
}

func (r *GormOperationReceiptRepository) MarkProviderStarted(ctx context.Context, id, claimToken, executionID uuid.UUID) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&OperationReceipt{}).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, OperationReceiptStatusExecuting).
		Updates(map[string]interface{}{"provider_started_at": now, "execution_id": executionID, "updated_at": now})
	return expectOneReceipt(result, "mark integration operation provider start")
}

func (r *GormOperationReceiptRepository) CompleteSuccess(ctx context.Context, id, claimToken uuid.UUID, actionResult *ActionResult) error {
	if actionResult == nil || actionResult.Output == nil {
		return fmt.Errorf("integration operation success result is required")
	}
	payload, err := json.Marshal(actionResult.Output)
	if err != nil {
		return fmt.Errorf("marshal integration operation result: %w", err)
	}
	if len(payload) > maxOperationReplayPayloadBytes {
		return fmt.Errorf("integration operation result exceeds replay limit")
	}
	providerRequestID := providerDiagnosticsForResult(actionResult, nil).RequestID
	result := r.db.WithContext(ctx).Model(&OperationReceipt{}).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, OperationReceiptStatusExecuting).
		Updates(map[string]interface{}{
			"status":              OperationReceiptStatusSucceeded,
			"provider_request_id": providerRequestID,
			"result_payload":      datatypes.JSON(payload),
			"result_count":        max(actionResult.ResultCount, 0),
			"updated_at":          gorm.Expr("CURRENT_TIMESTAMP"),
		})
	return expectOneReceipt(result, "complete integration operation")
}

func (r *GormOperationReceiptRepository) Release(ctx context.Context, id, claimToken uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, OperationReceiptStatusExecuting).
		Delete(&OperationReceipt{})
	return expectOneReceipt(result, "release integration operation claim")
}

func (r *GormOperationReceiptRepository) MarkOutcomeUnknown(ctx context.Context, id, claimToken, executionID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&OperationReceipt{}).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, OperationReceiptStatusExecuting).
		Updates(map[string]interface{}{
			"status":       OperationReceiptStatusOutcomeUnknown,
			"execution_id": executionID,
			"updated_at":   gorm.Expr("CURRENT_TIMESTAMP"),
		})
	return expectOneReceipt(result, "mark integration operation outcome unknown")
}

func expectOneReceipt(result *gorm.DB, operation string) error {
	if result == nil || result.Error != nil {
		if result == nil {
			return fmt.Errorf("%s: repository is unavailable", operation)
		}
		return fmt.Errorf("%s: %w", operation, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%s: receipt not found", operation)
	}
	return nil
}
