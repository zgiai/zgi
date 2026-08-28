package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRuntimeInputConversationIdle = errors.New("chat runtime conversation is idle")
	ErrRuntimeInputTargetMismatch   = errors.New("chat runtime input target does not match the active message")
)

type runtimeInputRepository struct{ db *gorm.DB }

func NewRuntimeInputRepository(db *gorm.DB) RuntimeInputRepository {
	return &runtimeInputRepository{db: db}
}

func (r *runtimeInputRepository) Submit(ctx context.Context, input *runtimemodel.RuntimeInput) (*runtimemodel.RuntimeInput, bool, error) {
	if input == nil {
		return nil, false, errors.New("runtime input is required")
	}
	var result *runtimemodel.RuntimeInput
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing runtimemodel.RuntimeInput
		err := tx.Where("conversation_id = ? AND client_input_id = ?", input.ConversationID, input.ClientInputID).Take(&existing).Error
		if err == nil {
			result = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load idempotent chat runtime input: %w", err)
		}

		var conversation runtimemodel.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", input.ConversationID, input.OrganizationID, input.AccountID).
			Take(&conversation).Error; err != nil {
			return err
		}
		// A concurrent retry can pass the optimistic lookup before the first
		// request commits. Recheck after taking the conversation lock so both
		// requests resolve to the same durable input instead of surfacing a
		// unique-index error to the client.
		err = tx.Where("conversation_id = ? AND client_input_id = ?", input.ConversationID, input.ClientInputID).Take(&existing).Error
		if err == nil {
			result = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("reload idempotent chat runtime input: %w", err)
		}
		if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusStreaming || conversation.ActiveMessageID == nil {
			return ErrRuntimeInputConversationIdle
		}
		if *conversation.ActiveMessageID != input.TargetMessageID {
			return ErrRuntimeInputTargetMismatch
		}

		var next int64
		if err := tx.Model(&runtimemodel.RuntimeInput{}).
			Where("conversation_id = ?", input.ConversationID).
			Select("COALESCE(MAX(sequence), 0) + 1").Scan(&next).Error; err != nil {
			return fmt.Errorf("allocate chat runtime input sequence: %w", err)
		}
		now := time.Now().UTC()
		input.Sequence = next
		input.Status = runtimemodel.RuntimeInputStatusPending
		input.CreatedAt = now
		input.UpdatedAt = now
		if input.ID == uuid.Nil {
			input.ID = uuid.New()
		}
		if err := tx.Create(input).Error; err != nil {
			return fmt.Errorf("create chat runtime input: %w", err)
		}
		created = true
		copyValue := *input
		result = &copyValue
		return nil
	})
	return result, created, err
}

func (r *runtimeInputRepository) ListByConversation(ctx context.Context, conversationID, organizationID, accountID uuid.UUID) ([]*runtimemodel.RuntimeInput, error) {
	var items []*runtimemodel.RuntimeInput
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND organization_id = ? AND account_id = ?", conversationID, organizationID, accountID).
		Order("sequence ASC").Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list chat runtime inputs: %w", err)
	}
	return items, nil
}

func (r *runtimeInputRepository) ListByBatch(ctx context.Context, batchID uuid.UUID) ([]*runtimemodel.RuntimeInput, error) {
	var items []*runtimemodel.RuntimeInput
	if err := r.db.WithContext(ctx).Where("claim_batch_id = ?", batchID).Order("sequence ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list chat runtime input batch: %w", err)
	}
	return items, nil
}

func (r *runtimeInputRepository) ClaimPending(ctx context.Context, targetMessageID uuid.UUID, mode string, batchID *uuid.UUID, now, expiresAt time.Time) ([]*runtimemodel.RuntimeInput, error) {
	if mode != runtimemodel.RuntimeInputConsumptionCurrentRun && mode != runtimemodel.RuntimeInputConsumptionFollowUp {
		return nil, fmt.Errorf("claim chat runtime inputs: invalid consumption mode %q", mode)
	}
	var items []*runtimemodel.RuntimeInput
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("target_message_id = ? AND status = ?", targetMessageID, runtimemodel.RuntimeInputStatusPending)
		if mode == runtimemodel.RuntimeInputConsumptionCurrentRun {
			query = query.Where("consumption_mode IS NULL")
		}
		if err := query.Order("sequence ASC").Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		ids := runtimeInputIDs(items)
		updates := map[string]interface{}{
			"status":           runtimemodel.RuntimeInputStatusClaimed,
			"consumption_mode": mode,
			"claim_batch_id":   batchID,
			"claimed_at":       now,
			"claim_expires_at": expiresAt,
			"updated_at":       now,
		}
		if err := tx.Model(&runtimemodel.RuntimeInput{}).Where("id IN ? AND status = ?", ids, runtimemodel.RuntimeInputStatusPending).Updates(updates).Error; err != nil {
			return err
		}
		for _, item := range items {
			item.Status = runtimemodel.RuntimeInputStatusClaimed
			item.ConsumptionMode = stringPointer(mode)
			item.ClaimBatchID = cloneUUID(batchID)
			item.ClaimedAt = timePointer(now)
			item.ClaimExpiresAt = timePointer(expiresAt)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim chat runtime inputs: %w", err)
	}
	return items, nil
}

func (r *runtimeInputRepository) MarkBatchConsumed(ctx context.Context, batchID, consumedMessageID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.RuntimeInput{}).
		Where("claim_batch_id = ? AND status = ?", batchID, runtimemodel.RuntimeInputStatusClaimed).
		Updates(map[string]interface{}{
			"status": runtimemodel.RuntimeInputStatusConsumed, "consumed_message_id": consumedMessageID,
			"consumed_at": now, "claim_expires_at": nil, "updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("consume chat runtime input batch: %w", result.Error)
	}
	return nil
}

func (r *runtimeInputRepository) MarkBatchFailed(ctx context.Context, batchID uuid.UUID, failure string, now time.Time) ([]*runtimemodel.RuntimeInput, error) {
	var items []*runtimemodel.RuntimeInput
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("claim_batch_id = ? AND status IN ?", batchID, []string{
				runtimemodel.RuntimeInputStatusClaimed,
				runtimemodel.RuntimeInputStatusConsumed,
			}).Order("sequence ASC").Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Model(&runtimemodel.RuntimeInput{}).Where("id IN ?", runtimeInputIDs(items)).Updates(map[string]interface{}{
			"status": runtimemodel.RuntimeInputStatusFailed, "error": failure,
			"claim_expires_at": nil, "updated_at": now,
		}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("fail chat runtime input batch: %w", err)
	}
	for _, item := range items {
		item.Status = runtimemodel.RuntimeInputStatusFailed
		item.Error = stringPointer(failure)
	}
	return items, nil
}

func (r *runtimeInputRepository) CancelForStoppedMessage(ctx context.Context, conversationID, messageID uuid.UUID, now time.Time) ([]*runtimemodel.RuntimeInput, error) {
	var items []*runtimemodel.RuntimeInput
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("conversation_id = ?", conversationID).
			Where("(target_message_id = ? AND status IN ?) OR (consumed_message_id = ? AND status = ?)",
				messageID,
				[]string{runtimemodel.RuntimeInputStatusPending, runtimemodel.RuntimeInputStatusClaimed},
				messageID,
				runtimemodel.RuntimeInputStatusConsumed,
			)
		if err := query.Order("sequence ASC").Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Model(&runtimemodel.RuntimeInput{}).Where("id IN ?", runtimeInputIDs(items)).Updates(map[string]interface{}{
			"status": runtimemodel.RuntimeInputStatusCanceled, "canceled_at": now,
			"claim_expires_at": nil, "updated_at": now,
		}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("cancel stopped message chat runtime inputs: %w", err)
	}
	for _, item := range items {
		item.Status = runtimemodel.RuntimeInputStatusCanceled
		item.CanceledAt = timePointer(now)
	}
	return items, nil
}

func (r *runtimeInputRepository) CancelPendingFollowUps(ctx context.Context, conversationID uuid.UUID, now time.Time) ([]*runtimemodel.RuntimeInput, error) {
	var items []*runtimemodel.RuntimeInput
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("conversation_id = ? AND consumed_message_id IS NULL", conversationID).
			Where("status = ? OR (status = ? AND consumption_mode = ?)",
				runtimemodel.RuntimeInputStatusPending,
				runtimemodel.RuntimeInputStatusClaimed,
				runtimemodel.RuntimeInputConsumptionFollowUp,
			).
			Order("sequence ASC").Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Model(&runtimemodel.RuntimeInput{}).Where("id IN ?", runtimeInputIDs(items)).Updates(map[string]interface{}{
			"status": runtimemodel.RuntimeInputStatusCanceled, "canceled_at": now,
			"claim_expires_at": nil, "updated_at": now,
		}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("cancel pending chat runtime input follow-ups: %w", err)
	}
	for _, item := range items {
		item.Status = runtimemodel.RuntimeInputStatusCanceled
		item.CanceledAt = timePointer(now)
	}
	return items, nil
}

func (r *runtimeInputRepository) RecoverStaleClaims(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&runtimemodel.RuntimeInput{}).
		Where("status = ? AND consumed_message_id IS NULL AND claim_expires_at IS NOT NULL AND claim_expires_at < ? AND (consumption_mode IS NULL OR consumption_mode <> ?)",
			runtimemodel.RuntimeInputStatusClaimed, cutoff, runtimemodel.RuntimeInputConsumptionFollowUp).
		Updates(map[string]interface{}{
			"status": runtimemodel.RuntimeInputStatusPending, "consumption_mode": nil,
			"claim_batch_id": nil, "claimed_at": nil, "claim_expires_at": nil, "updated_at": cutoff,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("recover stale chat runtime input claims: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *runtimeInputRepository) HasPendingFollowUp(ctx context.Context, conversationID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&runtimemodel.RuntimeInput{}).
		Where("conversation_id = ?", conversationID).
		Where("status = ? AND consumption_mode = ? AND consumed_message_id IS NULL",
			runtimemodel.RuntimeInputStatusClaimed,
			runtimemodel.RuntimeInputConsumptionFollowUp,
		).
		Limit(1).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check pending chat runtime input follow-up: %w", err)
	}
	return count > 0, nil
}

func (r *runtimeInputRepository) ListPendingInactiveTargets(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct{ TargetMessageID uuid.UUID }
	if err := r.db.WithContext(ctx).Model(&runtimemodel.RuntimeInput{}).
		Select("DISTINCT chat_runtime_inputs.target_message_id").
		Joins("JOIN chat_runtime_conversations ON chat_runtime_conversations.id = chat_runtime_inputs.conversation_id").
		Where("chat_runtime_inputs.status = ?", runtimemodel.RuntimeInputStatusPending).
		Where("chat_runtime_conversations.deleted_at IS NULL").
		Where("chat_runtime_conversations.runtime_status <> ? OR chat_runtime_conversations.active_message_id IS NULL OR chat_runtime_conversations.active_message_id <> chat_runtime_inputs.target_message_id", runtimemodel.ConversationRuntimeStatusStreaming).
		Order("chat_runtime_inputs.target_message_id ASC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list inactive pending chat runtime input targets: %w", err)
	}
	targets := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.TargetMessageID != uuid.Nil {
			targets = append(targets, row.TargetMessageID)
		}
	}
	return targets, nil
}

func (r *runtimeInputRepository) ListPendingFollowUpBatches(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct{ ClaimBatchID uuid.UUID }
	if err := r.db.WithContext(ctx).Model(&runtimemodel.RuntimeInput{}).
		Select("DISTINCT claim_batch_id").
		Where("status = ? AND consumption_mode = ? AND claim_batch_id IS NOT NULL AND consumed_message_id IS NULL", runtimemodel.RuntimeInputStatusClaimed, runtimemodel.RuntimeInputConsumptionFollowUp).
		Order("claim_batch_id ASC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list pending chat runtime input follow-up batches: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.ClaimBatchID != uuid.Nil {
			out = append(out, row.ClaimBatchID)
		}
	}
	return out, nil
}

func runtimeInputIDs(items []*runtimemodel.RuntimeInput) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func stringPointer(value string) *string     { return &value }
func timePointer(value time.Time) *time.Time { return &value }
func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
