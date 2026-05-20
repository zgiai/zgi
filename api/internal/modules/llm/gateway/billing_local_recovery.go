package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultLocalPredeductRecoverEvery = 30 * time.Second
	defaultLocalPredeductTimeout      = 10 * time.Minute
	defaultLocalPredeductRecoverBatch = 100
	defaultLocalRecoverMaxRetries     = 8
)

// StartLocalPredeductRecoveryWorker starts a background worker that rolls back
// stale local PREDEDUCTED attempts.
func (b *BillingService) StartLocalPredeductRecoveryWorker(ctx context.Context) {
	b.localRecoverOnce.Do(func() {
		go b.runLocalPredeductRecoveryWorker(ctx)
	})
}

func (b *BillingService) runLocalPredeductRecoveryWorker(ctx context.Context) {
	ticker := time.NewTicker(defaultLocalPredeductRecoverEvery)
	defer ticker.Stop()

	logger.InfoContext(ctx, "local pre-deduct recovery worker started",
		zap.Int64("sweep_every_ms", defaultLocalPredeductRecoverEvery.Milliseconds()),
		zap.Int64("timeout_ms", defaultLocalPredeductTimeout.Milliseconds()),
		zap.Int("batch_size", defaultLocalPredeductRecoverBatch),
	)

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "local pre-deduct recovery worker stopped")
			return
		case <-ticker.C:
			if err := b.recoverStaleLocalPredeductAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "local pre-deduct recovery sweep failed", err)
			}
		}
	}
}

func (b *BillingService) recoverStaleLocalPredeductAttempts(ctx context.Context) error {
	cutoff := time.Now().Add(-defaultLocalPredeductTimeout)
	var attempts []BillingAttempt
	if err := b.db.WithContext(ctx).
		Where(
			"lane = ? AND ((status = ? AND updated_at < ?) OR (status = ? AND reconcile_attempts < ? AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)))",
			billingAttemptLaneLocal,
			billingAttemptStatusPre,
			cutoff,
			billingAttemptStatusPartial,
			defaultLocalRecoverMaxRetries,
			time.Now(),
		).
		Order("COALESCE(next_reconcile_at, updated_at) ASC").
		Limit(defaultLocalPredeductRecoverBatch).
		Find(&attempts).Error; err != nil {
		return fmt.Errorf("list stale local pre-deduct attempts: %w", err)
	}
	for _, attempt := range attempts {
		if err := b.recoverLocalPredeductAttempt(ctx, attempt.AttemptID); err != nil {
			logger.ErrorContext(ctx, "local pre-deduct recovery failed",
				err,
				zap.String("attempt_id", attempt.AttemptID),
			)
		}
	}
	return nil
}

func (b *BillingService) recoverLocalPredeductAttempt(ctx context.Context, attemptID string) error {
	claimed, err := b.claimLocalPredeductAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	bc, err := b.buildLocalRecoveryBillingContext(ctx, attemptID)
	if err != nil {
		if markErr := b.scheduleLocalRecoveryFailure(ctx, attemptID, err); markErr != nil {
			return fmt.Errorf("build local recovery billing context: %v (additionally failed to mark attempt: %w)", err, markErr)
		}
		return fmt.Errorf("build local recovery billing context: %w", err)
	}

	if err := b.Settle(ctx, bc); err != nil {
		if markErr := b.scheduleLocalRecoveryFailure(ctx, attemptID, err); markErr != nil {
			return fmt.Errorf("local stale pre-deduct settle rollback failed: %v (additionally failed to mark attempt: %w)", err, markErr)
		}
		return fmt.Errorf("local stale pre-deduct settle rollback failed: %w", err)
	}

	return nil
}

func (b *BillingService) claimLocalPredeductAttempt(ctx context.Context, attemptID string) (bool, error) {
	invocation := "error"
	code := "LOCAL_PREDEDUCT_TIMEOUT_RECOVERY"
	msg := fmt.Sprintf("stale local pre-deduct timed out after %s, auto rollback in progress", defaultLocalPredeductTimeout)

	now := time.Now()
	res := b.db.WithContext(ctx).
		Model(&BillingAttempt{}).
		Where(
			"attempt_id = ? AND lane = ? AND (status = ? OR (status = ? AND reconcile_attempts < ? AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)))",
			attemptID,
			billingAttemptLaneLocal,
			billingAttemptStatusPre,
			billingAttemptStatusPartial,
			defaultLocalRecoverMaxRetries,
			now,
		).
		Updates(map[string]any{
			"status":             billingAttemptStatusSettlePending,
			"invocation_result":  invocation,
			"error_code":         code,
			"error_message":      msg,
			"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
			"last_reconcile_at":  now,
			"next_reconcile_at":  nil,
			"updated_at":         now,
		})
	if res.Error != nil {
		return false, fmt.Errorf("claim local stale pre-deduct attempt: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (b *BillingService) buildLocalRecoveryBillingContext(ctx context.Context, attemptID string) (*BillingContext, error) {
	var attempt BillingAttempt
	if err := b.db.WithContext(ctx).
		Where("attempt_id = ? AND lane = ?", attemptID, billingAttemptLaneLocal).
		First(&attempt).Error; err != nil {
		return nil, fmt.Errorf("load local attempt: %w", err)
	}
	var subjectEntry BillingAttemptEntry
	if err := b.db.WithContext(ctx).
		Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).
		First(&subjectEntry).Error; err != nil {
		return nil, fmt.Errorf("load local subject entry: %w", err)
	}
	var fundEntry BillingAttemptEntry
	if err := b.db.WithContext(ctx).
		Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", attemptID, billingEntryTypeFund, billingLedgerTypeChannelWallet).
		First(&fundEntry).Error; err != nil {
		return nil, fmt.Errorf("load local fund entry: %w", err)
	}
	if subjectEntry.ReservedAmount != fundEntry.ReservedAmount {
		return nil, fmt.Errorf(
			"reserved amount mismatch for local recovery: attempt_id=%s subject_reserved=%d fund_reserved=%d",
			attemptID, subjectEntry.ReservedAmount, fundEntry.ReservedAmount,
		)
	}
	channelID, err := uuid.Parse(fundEntry.LedgerRefID)
	if err != nil {
		return nil, fmt.Errorf("parse local fund ledger_ref_id as channel_id: %w", err)
	}
	channelIDCopy := channelID
	bc := &BillingContext{
		OrganizationID:    attempt.OrganizationID.String(),
		AttemptID:         attempt.AttemptID,
		RequestID:         attempt.RequestID,
		QuotaSubjectType:  attempt.QuotaSubjectType,
		QuotaSubjectID:    attempt.QuotaSubjectID,
		EstimatedCredits:  subjectEntry.ReservedAmount,
		ActualCredits:     0,
		ChannelID:         &channelIDCopy,
		BillingLane:       UsageBillingLanePrivate,
		UseSystemProvider: false,
		Status:            "error",
		ErrorMessage:      "stale local pre-deduct auto rollback",
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeAPIKey {
		bc.APIKeyID = attempt.QuotaSubjectID
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeWorkspace {
		bc.WorkspaceID = attempt.QuotaSubjectID
	}
	if attempt.ModelID != nil {
		bc.ModelID = *attempt.ModelID
	}
	if attempt.ProviderID != nil {
		bc.ProviderID = *attempt.ProviderID
	}
	if attempt.RouteID != nil {
		routeID := *attempt.RouteID
		if routeID.String() != fundEntry.LedgerRefID {
			return nil, fmt.Errorf(
				"local recovery channel mismatch: attempt_id=%s route_id=%s fund_ledger_ref_id=%s",
				attemptID,
				routeID.String(),
				fundEntry.LedgerRefID,
			)
		}
		bc.ChannelID = &routeID
	}
	return bc, nil
}

func (b *BillingService) scheduleLocalRecoveryFailure(ctx context.Context, attemptID string, recoverErr error) error {
	var attempt BillingAttempt
	if err := b.db.WithContext(ctx).
		Where("attempt_id = ? AND lane = ?", attemptID, billingAttemptLaneLocal).
		First(&attempt).Error; err != nil {
		return fmt.Errorf("load local recovery attempt: %w", err)
	}
	if attempt.Status != billingAttemptStatusSettlePending {
		return nil
	}

	invocation := "error"
	code := "LOCAL_PREDEDUCT_RECOVERY_FAILED"
	msg := recoverErr.Error()
	nextAt := time.Now().Add(localRecoveryBackoff(attempt.ReconcileAttempts))
	status := billingAttemptStatusPartial
	if attempt.ReconcileAttempts >= defaultLocalRecoverMaxRetries {
		status = billingAttemptStatusDeadLetter
		nextAt = time.Time{}
		code = "LOCAL_PREDEDUCT_RECOVERY_DEAD_LETTER"
		msg = fmt.Sprintf("max retries reached(%d): %v", attempt.ReconcileAttempts, recoverErr)
	}
	res := b.db.WithContext(ctx).
		Model(&BillingAttempt{}).
		Where("attempt_id = ? AND lane = ?", attemptID, billingAttemptLaneLocal).
		Updates(map[string]any{
			"status":            status,
			"invocation_result": invocation,
			"error_code":        code,
			"error_message":     msg,
			"next_reconcile_at": func() *time.Time {
				if nextAt.IsZero() {
					return nil
				}
				t := nextAt
				return &t
			}(),
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("schedule local recovery failure: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("schedule local recovery failure affected %d rows (attempt_id=%s)", res.RowsAffected, attemptID)
	}
	return nil
}

func localRecoveryBackoff(reconcileAttempts int) time.Duration {
	if reconcileAttempts <= 0 {
		return time.Minute
	}
	backoff := time.Minute
	for i := 1; i < reconcileAttempts; i++ {
		backoff *= 2
		if backoff >= 30*time.Minute {
			return 30 * time.Minute
		}
	}
	return backoff
}
