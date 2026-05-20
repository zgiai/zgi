package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (b *BillingService) ensureAttemptID(bc *BillingContext) (string, error) {
	if bc == nil {
		return "", fmt.Errorf("billing context is nil")
	}
	attemptID := strings.TrimSpace(bc.AttemptID)
	if attemptID == "" {
		return "", fmt.Errorf("missing attempt_id for billing ledger (request_id=%s)", strings.TrimSpace(bc.RequestID))
	}
	return attemptID, nil
}

func (b *BillingService) upsertAttemptInit(ctx context.Context, tx *gorm.DB, bc *BillingContext) error {
	orgID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization id in billing context: %w", err)
	}

	attemptID, err := b.ensureAttemptID(bc)
	if err != nil {
		return err
	}
	bc.AttemptID = attemptID

	usageLane, err := normalizeBillingContextUsageLane(bc)
	if err != nil {
		return err
	}
	lane := billingAttemptLaneLocal
	if usageBillingLaneUsesSystemProvider(usageLane) {
		lane = billingAttemptLaneRemote
	}

	subjectType := strings.TrimSpace(bc.QuotaSubjectType)
	if subjectType == "" {
		return fmt.Errorf("missing quota_subject_type for billing ledger (attempt_id=%s request_id=%s)", attemptID, strings.TrimSpace(bc.RequestID))
	}
	if subjectType != quotaSubjectTypeAPIKey && subjectType != quotaSubjectTypeWorkspace && subjectType != quotaSubjectTypeOrganization {
		return fmt.Errorf("unsupported quota_subject_type for billing ledger: %s (attempt_id=%s request_id=%s)", subjectType, attemptID, strings.TrimSpace(bc.RequestID))
	}
	subjectID := strings.TrimSpace(bc.QuotaSubjectID)
	if subjectID == "" {
		return fmt.Errorf("missing quota_subject_id for billing ledger (quota_subject_type=%s attempt_id=%s request_id=%s)", subjectType, attemptID, strings.TrimSpace(bc.RequestID))
	}
	bc.QuotaSubjectType = subjectType
	bc.QuotaSubjectID = subjectID

	now := time.Now()
	attempt := &BillingAttempt{
		AttemptID:        attemptID,
		RequestID:        bc.RequestID,
		OrganizationID:   orgID,
		Lane:             lane,
		QuotaSubjectType: subjectType,
		QuotaSubjectID:   subjectID,
		Status:           billingAttemptStatusInit,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if bc.ChannelID != nil {
		attempt.RouteID = bc.ChannelID
	}
	if bc.ProviderID != uuid.Nil {
		providerID := bc.ProviderID
		attempt.ProviderID = &providerID
	}
	if bc.ModelID != uuid.Nil {
		modelID := bc.ModelID
		attempt.ModelID = &modelID
	}

	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "attempt_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"request_id":         attempt.RequestID,
				"organization_id":    attempt.OrganizationID,
				"lane":               attempt.Lane,
				"route_id":           attempt.RouteID,
				"provider_id":        attempt.ProviderID,
				"model_id":           attempt.ModelID,
				"quota_subject_type": attempt.QuotaSubjectType,
				"quota_subject_id":   attempt.QuotaSubjectID,
				"updated_at":         now,
			}),
		}).
		Create(attempt).Error; err != nil {
		return fmt.Errorf("upsert billing attempt: %w", err)
	}

	return b.upsertAttemptBaseEntries(ctx, tx, bc, subjectType, subjectID, lane, now)
}

func (b *BillingService) upsertAttemptBaseEntries(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
	subjectType string,
	subjectID string,
	lane string,
	now time.Time,
) error {
	attemptID, err := b.ensureAttemptID(bc)
	if err != nil {
		return err
	}
	estimated := bc.EstimatedCredits

	subjectLedgerType := billingLedgerTypeAPIKeyQuota
	if subjectType != quotaSubjectTypeAPIKey {
		subjectLedgerType = subjectType + "_quota"
	}
	subjectEntry := &BillingAttemptEntry{
		AttemptID:      attemptID,
		EntryType:      billingEntryTypeSubject,
		LedgerType:     subjectLedgerType,
		LedgerRefID:    subjectID,
		ReservedAmount: estimated,
		Status:         billingEntryStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := b.upsertAttemptEntry(ctx, tx, subjectEntry, now); err != nil {
		return err
	}

	switch lane {
	case billingAttemptLaneRemote:
		fundEntry := &BillingAttemptEntry{
			AttemptID:      attemptID,
			EntryType:      billingEntryTypeFund,
			LedgerType:     billingLedgerTypeOrgFunds,
			LedgerRefID:    bc.OrganizationID,
			ReservedAmount: estimated,
			Status:         billingEntryStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return b.upsertAttemptEntry(ctx, tx, fundEntry, now)
	case billingAttemptLaneLocal:
		if bc.ChannelID == nil {
			return fmt.Errorf(
				"missing channel_id for local billing ledger entry (attempt_id=%s request_id=%s)",
				attemptID,
				strings.TrimSpace(bc.RequestID),
			)
		}
		fundEntry := &BillingAttemptEntry{
			AttemptID:      attemptID,
			EntryType:      billingEntryTypeFund,
			LedgerType:     billingLedgerTypeChannelWallet,
			LedgerRefID:    bc.ChannelID.String(),
			ReservedAmount: estimated,
			Status:         billingEntryStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return b.upsertAttemptEntry(ctx, tx, fundEntry, now)
	default:
		return fmt.Errorf(
			"unsupported billing lane for attempt entries: %s (attempt_id=%s request_id=%s)",
			lane,
			attemptID,
			strings.TrimSpace(bc.RequestID),
		)
	}
}

func (b *BillingService) upsertAttemptEntry(
	ctx context.Context,
	tx *gorm.DB,
	entry *BillingAttemptEntry,
	now time.Time,
) error {
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "attempt_id"},
				{Name: "entry_type"},
				{Name: "ledger_type"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"ledger_ref_id":   entry.LedgerRefID,
				"reserved_amount": entry.ReservedAmount,
				"actual_amount":   entry.ActualAmount,
				"refunded_amount": entry.RefundedAmount,
				"status":          entry.Status,
				"error_code":      entry.ErrorCode,
				"error_message":   entry.ErrorMessage,
				"idempotency_key": entry.IdempotencyKey,
				"updated_at":      now,
			}),
		}).
		Create(entry).Error; err != nil {
		return fmt.Errorf("upsert billing attempt entry: %w", err)
	}
	return nil
}

func (b *BillingService) updateAttemptStatus(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
	status string,
	invocationResult *string,
	errCode *string,
	errMsg *string,
) error {
	attemptID, err := b.ensureAttemptID(bc)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"status":            status,
		"invocation_result": invocationResult,
		"error_code":        errCode,
		"error_message":     errMsg,
		"updated_at":        time.Now(),
	}
	res := tx.WithContext(ctx).
		Model(&BillingAttempt{}).
		Where("attempt_id = ?", attemptID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("update billing attempt status affected %d rows (attempt_id=%s)", res.RowsAffected, attemptID)
	}
	return nil
}

func (b *BillingService) updateAttemptEntriesAfterSettle(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
	fundLedgerType string,
	fundLedgerRefID string,
) error {
	attemptID, err := b.ensureAttemptID(bc)
	if err != nil {
		return err
	}
	if strings.TrimSpace(fundLedgerType) == "" || strings.TrimSpace(fundLedgerRefID) == "" {
		return fmt.Errorf(
			"missing fund ledger identity for settle entry update (attempt_id=%s request_id=%s)",
			attemptID,
			strings.TrimSpace(bc.RequestID),
		)
	}

	refunded := bc.EstimatedCredits - bc.ActualCredits
	if refunded < 0 {
		refunded = 0
	}
	entryStatus := billingEntryStatusSettled
	if !billingContextStatusIsSuccess(bc.Status) {
		entryStatus = billingEntryStatusRolled
	}

	subjectRes := tx.WithContext(ctx).
		Model(&BillingAttemptEntry{}).
		Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).
		Updates(map[string]interface{}{
			"actual_amount":   bc.ActualCredits,
			"refunded_amount": refunded,
			"status":          entryStatus,
			"updated_at":      time.Now(),
		})
	if subjectRes.Error != nil {
		return fmt.Errorf("update subject attempt entry: %w", subjectRes.Error)
	}
	if subjectRes.RowsAffected != 1 {
		return fmt.Errorf("update subject attempt entry affected %d rows (attempt_id=%s)", subjectRes.RowsAffected, attemptID)
	}

	fundRes := tx.WithContext(ctx).
		Model(&BillingAttemptEntry{}).
		Where("attempt_id = ? AND entry_type = ? AND ledger_type = ? AND ledger_ref_id = ?", attemptID, billingEntryTypeFund, fundLedgerType, fundLedgerRefID).
		Updates(map[string]interface{}{
			"actual_amount":   bc.ActualCredits,
			"refunded_amount": refunded,
			"status":          entryStatus,
			"updated_at":      time.Now(),
		})
	if fundRes.Error != nil {
		return fundRes.Error
	}
	if fundRes.RowsAffected != 1 {
		return fmt.Errorf("update fund attempt entry affected %d rows (attempt_id=%s ledger_type=%s ledger_ref_id=%s)", fundRes.RowsAffected, attemptID, fundLedgerType, fundLedgerRefID)
	}
	return nil
}

func (b *BillingService) recordAttemptSettleInput(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) error {
	attemptID, err := b.ensureAttemptID(bc)
	if err != nil {
		return err
	}

	refunded := bc.EstimatedCredits - bc.ActualCredits
	if refunded < 0 {
		refunded = 0
	}

	updates := map[string]interface{}{
		"actual_amount":   bc.ActualCredits,
		"refunded_amount": refunded,
		"updated_at":      time.Now(),
	}

	subjectRes := tx.WithContext(ctx).
		Model(&BillingAttemptEntry{}).
		Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).
		Updates(updates)
	if subjectRes.Error != nil {
		return fmt.Errorf("record subject settle input: %w", subjectRes.Error)
	}
	if subjectRes.RowsAffected != 1 {
		return fmt.Errorf("record subject settle input affected %d rows (attempt_id=%s)", subjectRes.RowsAffected, attemptID)
	}

	fundRes := tx.WithContext(ctx).
		Model(&BillingAttemptEntry{}).
		Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeFund).
		Updates(updates)
	if fundRes.Error != nil {
		return fmt.Errorf("record fund settle input: %w", fundRes.Error)
	}
	if fundRes.RowsAffected != 1 {
		return fmt.Errorf("record fund settle input affected %d rows (attempt_id=%s)", fundRes.RowsAffected, attemptID)
	}

	return nil
}

func (b *BillingService) bindRemoteDeductionID(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) error {
	attemptID, err := b.ensureAttemptID(bc)
	if err != nil {
		return err
	}
	deductionID := strings.TrimSpace(bc.DeductionID)
	if deductionID == "" {
		return fmt.Errorf("missing deduction_id for remote binding (attempt_id=%s request_id=%s)", attemptID, strings.TrimSpace(bc.RequestID))
	}
	res := tx.WithContext(ctx).
		Model(&BillingAttemptEntry{}).
		Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", attemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
		Updates(map[string]interface{}{
			"idempotency_key": deductionID,
			"updated_at":      time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("bind remote deduction id affected %d rows (attempt_id=%s)", res.RowsAffected, attemptID)
	}
	return nil
}

func (b *BillingService) preDeductPrivateChannelWallet(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) error {
	if bc.ChannelID == nil {
		return fmt.Errorf("missing channel id for private billing pre-deduct")
	}
	orgID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization id in billing context: %w", err)
	}

	wallet, err := b.getOrCreateChannelWalletForUpdate(ctx, tx, *bc.ChannelID, orgID)
	if err != nil {
		return err
	}
	if wallet.Status == channelWalletStatusDebt {
		return ErrInsufficientBalance
	}
	if wallet.Balance < bc.EstimatedCredits {
		return ErrInsufficientBalance
	}

	before := wallet.Balance
	after := before - bc.EstimatedCredits
	updateRes := tx.WithContext(ctx).
		Model(&ChannelWallet{}).
		Where("channel_id = ? AND organization_id = ?", wallet.ChannelID, wallet.OrganizationID).
		Updates(map[string]interface{}{
			"balance":    after,
			"status":     channelWalletStatusActive,
			"updated_at": time.Now(),
		})
	if updateRes.Error != nil {
		return fmt.Errorf("update channel wallet pre-deduct: %w", updateRes.Error)
	}
	if updateRes.RowsAffected != 1 {
		return fmt.Errorf(
			"update channel wallet pre-deduct affected %d rows (channel_id=%s organization_id=%s)",
			updateRes.RowsAffected,
			wallet.ChannelID,
			wallet.OrganizationID,
		)
	}
	if err := b.syncRouteBalanceSnapshot(ctx, tx, wallet.ChannelID, after); err != nil {
		return fmt.Errorf("sync route balance snapshot: %w", err)
	}

	return b.createChannelWalletTransaction(
		ctx,
		tx,
		wallet.ChannelID,
		bc.AttemptID,
		channelWalletTxTypePreDeduct,
		-bc.EstimatedCredits,
		before,
		after,
		map[string]interface{}{
			"phase":             billingPhasePreDeduct,
			"request_id":        bc.RequestID,
			"attempt_id":        bc.AttemptID,
			"estimated_credits": bc.EstimatedCredits,
		},
	)
}

func (b *BillingService) CheckPrivateChannelBalance(
	ctx context.Context,
	organizationID uuid.UUID,
	channelID uuid.UUID,
	estimatedCredits int64,
) (bool, error) {
	if estimatedCredits <= 0 {
		return true, nil
	}

	var wallet ChannelWallet
	err := b.db.WithContext(ctx).
		Where("channel_id = ? AND organization_id = ?", channelID, organizationID).
		First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load private channel wallet balance: %w", err)
	}

	return wallet.Balance >= estimatedCredits, nil
}

func (b *BillingService) settlePrivateChannelWallet(
	ctx context.Context,
	tx *gorm.DB,
	bc *BillingContext,
) (string, string, error) {
	if bc.ChannelID == nil {
		return "", "", fmt.Errorf("missing channel id for private billing settle")
	}
	orgID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return "", "", fmt.Errorf("invalid organization id in billing context: %w", err)
	}

	wallet, err := b.getOrCreateChannelWalletForUpdate(ctx, tx, *bc.ChannelID, orgID)
	if err != nil {
		return "", "", err
	}

	delta := bc.EstimatedCredits - bc.ActualCredits
	before := wallet.Balance
	after := before + delta
	newStatus := channelWalletStatusActive
	if after < 0 {
		newStatus = channelWalletStatusDebt
		logger.WarnContext(ctx, "private channel wallet entered debt",
			zap.String("channel_id", wallet.ChannelID.String()),
			zap.String("attempt_id", bc.AttemptID),
			zap.String("request_id", bc.RequestID),
			zap.Int64("balance_before", before),
			zap.Int64("delta", delta),
			zap.Int64("balance_after", after),
		)
	}

	updateRes := tx.WithContext(ctx).
		Model(&ChannelWallet{}).
		Where("channel_id = ? AND organization_id = ?", wallet.ChannelID, wallet.OrganizationID).
		Updates(map[string]interface{}{
			"balance":    after,
			"status":     newStatus,
			"updated_at": time.Now(),
		})
	if updateRes.Error != nil {
		return "", "", fmt.Errorf("update channel wallet settle: %w", updateRes.Error)
	}
	if updateRes.RowsAffected != 1 {
		return "", "", fmt.Errorf(
			"update channel wallet settle affected %d rows (channel_id=%s organization_id=%s)",
			updateRes.RowsAffected,
			wallet.ChannelID,
			wallet.OrganizationID,
		)
	}
	if err := b.syncRouteBalanceSnapshot(ctx, tx, wallet.ChannelID, after); err != nil {
		return "", "", fmt.Errorf("sync route balance snapshot: %w", err)
	}

	if delta != 0 {
		txType := channelWalletTxTypeSettleAdjustment
		if delta > 0 {
			txType = channelWalletTxTypeRefund
			if !billingContextStatusIsSuccess(bc.Status) {
				txType = channelWalletTxTypeRollback
			}
		}
		if err := b.createChannelWalletTransaction(
			ctx,
			tx,
			wallet.ChannelID,
			bc.AttemptID,
			txType,
			delta,
			before,
			after,
			map[string]interface{}{
				"phase":             billingPhaseSettle,
				"request_id":        bc.RequestID,
				"attempt_id":        bc.AttemptID,
				"estimated_credits": bc.EstimatedCredits,
				"actual_credits":    bc.ActualCredits,
				"status":            bc.Status,
			},
		); err != nil {
			return "", "", err
		}
	}

	return billingLedgerTypeChannelWallet, wallet.ChannelID.String(), nil
}

func (b *BillingService) getOrCreateChannelWalletForUpdate(
	ctx context.Context,
	tx *gorm.DB,
	channelID uuid.UUID,
	organizationID uuid.UUID,
) (*ChannelWallet, error) {
	var wallet ChannelWallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("channel_id = ?", channelID).
		First(&wallet).Error
	if err == nil {
		if wallet.OrganizationID != organizationID {
			return nil, fmt.Errorf(
				"channel wallet organization mismatch: channel_id=%s wallet_org=%s request_org=%s",
				channelID,
				wallet.OrganizationID,
				organizationID,
			)
		}
		return &wallet, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	var route channelmodel.LLMRoute
	if routeErr := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND organization_id = ?", channelID, organizationID).
		First(&route).Error; routeErr != nil {
		return nil, fmt.Errorf("load private channel route: %w", routeErr)
	}

	initialBalance := route.Balance.Round(0).IntPart()
	status := channelWalletStatusActive
	if initialBalance < 0 {
		status = channelWalletStatusDebt
	}
	wallet = ChannelWallet{
		ChannelID:      channelID,
		OrganizationID: organizationID,
		Balance:        initialBalance,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if createErr := tx.WithContext(ctx).Create(&wallet).Error; createErr != nil {
		return nil, fmt.Errorf("create channel wallet: %w", createErr)
	}
	return &wallet, nil
}

func (b *BillingService) createChannelWalletTransaction(
	ctx context.Context,
	tx *gorm.DB,
	channelID uuid.UUID,
	attemptID string,
	txType string,
	amount int64,
	before int64,
	after int64,
	metadata map[string]interface{},
) error {
	aid := strings.TrimSpace(attemptID)
	if aid == "" {
		return fmt.Errorf("missing attempt_id for channel wallet transaction (channel_id=%s)", channelID)
	}
	record := &ChannelWalletTransaction{
		ChannelID:     channelID,
		AttemptID:     &aid,
		Type:          txType,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
		Metadata:      metadata,
		CreatedAt:     time.Now(),
	}
	return tx.WithContext(ctx).Create(record).Error
}

func (b *BillingService) syncRouteBalanceSnapshot(
	ctx context.Context,
	tx *gorm.DB,
	channelID uuid.UUID,
	balance int64,
) error {
	res := tx.WithContext(ctx).
		Model(&channelmodel.LLMRoute{}).
		Where("id = ?", channelID).
		Updates(map[string]interface{}{
			"balance":    decimal.NewFromInt(balance),
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("sync route balance snapshot affected %d rows (channel_id=%s)", res.RowsAffected, channelID)
	}
	return nil
}
