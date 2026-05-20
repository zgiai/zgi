package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RemoteBilling implements BillingProvider by forwarding PreDeduct/Settle
// to console-api via gRPC. CalculateCreditsFromTokens is handled locally
// since it only needs read access to zgi-api DB.
// This is used in CLOUD mode where console-api is the billing authority.
type RemoteBilling struct {
	localService *BillingService
	grpcClient   quotaClient
}

type quotaClient interface {
	PreDeductQuota(ctx context.Context, req *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error)
	SettleQuota(ctx context.Context, req *SettleQuotaRequest) (*SettleQuotaResponse, error)
	CheckCreditBalance(ctx context.Context, organizationID string, estimatedCredits int64) (bool, int64, error)
	Close() error
}

const (
	defaultReconcileSweepEvery  = 30 * time.Second
	defaultReconcileBatchSize   = 50
	defaultReconcileMaxRetries  = 8
	defaultReconcileBaseBackoff = 30 * time.Second
	defaultReconcileMaxBackoff  = 10 * time.Minute
	defaultSettlePendingTimeout = 2 * time.Minute
)

var (
	errReconcileMissingDeductionID = errors.New("missing deduction_id for remote reconcile")
)

// NewRemoteBilling creates a RemoteBilling that routes PreDeduct/Settle via gRPC.
// grpcAddr is the console-api gRPC address (e.g. "console-api:50051").
// localService is used for CalculateCreditsFromTokens (needs local model pricing data).
func NewRemoteBilling(grpcAddr string, localService *BillingService) (*RemoteBilling, error) {
	client, err := NewQuotaClient(grpcAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to console-api billing at %s: %w", grpcAddr, err)
	}
	logger.Info("remote billing connected", "grpc_addr", grpcAddr)

	rb := &RemoteBilling{
		localService: localService,
		grpcClient:   client,
	}
	go rb.startPartialSettleReconcileWorker(context.Background())

	return rb, nil
}

func (s *RemoteBilling) PreDeduct(ctx context.Context, bc *BillingContext) error {
	return s.preDeductViaGRPC(ctx, bc)
}

func (s *RemoteBilling) Settle(ctx context.Context, bc *BillingContext) error {
	return s.settleViaGRPC(ctx, bc)
}

// CalculateCreditsFromTokens uses local DB (model pricing is in zgi-api DB)
func (s *RemoteBilling) CalculateCreditsFromTokens(
	promptTokens, completionTokens int,
	modelID uuid.UUID,
) (inputCredits, outputCredits, totalCredits int64, err error) {
	// via grpc console

	return s.localService.CalculateCreditsFromTokens(promptTokens, completionTokens, modelID)
}

// CalculateImageCredits uses local DB (model pricing is in zgi-api DB)
func (s *RemoteBilling) CalculateImageCredits(req *adapter.ImageRequest, modelID uuid.UUID) (int64, error) {
	return s.localService.CalculateImageCredits(req, modelID)
}

// CheckBalance checks credit balance via gRPC to console-api
func (s *RemoteBilling) CheckBalance(ctx context.Context, groupID uuid.UUID, ownerID uuid.UUID, estimatedCredits int64) (bool, error) {
	sufficient, _, err := s.grpcClient.CheckCreditBalance(ctx, groupID.String(), estimatedCredits)
	if err != nil {
		return false, fmt.Errorf("grpc check credit balance failed: %w", err)
	}
	return sufficient, nil
}

func (s *RemoteBilling) preDeductViaGRPC(ctx context.Context, bc *BillingContext) error {
	usageLane, err := normalizeBillingContextUsageLane(bc)
	if err != nil {
		return err
	}
	if usageLane != UsageBillingLanePlatform {
		return fmt.Errorf("remote billing requires platform lane, got %s (request_id=%s)", usageLane, bc.RequestID)
	}
	if strings.TrimSpace(bc.AttemptID) == "" {
		return fmt.Errorf("missing attempt_id for pre-deduct (request_id=%s)", bc.RequestID)
	}
	if err := s.preDeductLocalSubjectQuota(ctx, bc); err != nil {
		if markErr := s.markAttemptPreDeductFailed(ctx, bc, "PREDEDUCT_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("pre-deduct subject quota failed: %v (additionally failed to mark attempt: %w)", err, markErr)
		}
		return fmt.Errorf("pre-deduct subject quota failed: %w", err)
	}

	req := &PreDeductQuotaRequest{
		OrganizationID:   bc.OrganizationID,
		EstimatedCredits: bc.EstimatedCredits,
		ModelID:          bc.ModelID.String(),
		ModelName:        bc.ModelName,
		ProviderID:       bc.ProviderID.String(),
		ProviderName:     bc.ProviderName,
		RequestID:        bc.RequestID,
		AttemptID:        bc.AttemptID,
	}

	resp, err := s.grpcClient.PreDeductQuota(ctx, req)
	if err != nil {
		rollbackErr := s.rollbackLocalSubjectQuota(ctx, bc)
		if markErr := s.markAttemptPreDeductFailed(ctx, bc, "PREDEDUCT_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("grpc pre-deduct failed: %v (rollback_err=%v, mark_err=%w)", err, rollbackErr, markErr)
		}
		if rollbackErr != nil {
			return fmt.Errorf("grpc pre-deduct failed: %v (local subject rollback failed: %w)", err, rollbackErr)
		}
		return fmt.Errorf("grpc pre-deduct failed: %w", err)
	}

	if !resp.Success {
		rollbackErr := s.rollbackLocalSubjectQuota(ctx, bc)
		if markErr := s.markAttemptPreDeductFailed(ctx, bc, "PREDEDUCT_FAILED", resp.ErrorMessage); markErr != nil {
			return fmt.Errorf("pre-deduct failed: %s (rollback_err=%v, mark_err=%w)", resp.ErrorMessage, rollbackErr, markErr)
		}
		if rollbackErr != nil {
			return fmt.Errorf("pre-deduct failed: %s (local subject rollback failed: %w)", resp.ErrorMessage, rollbackErr)
		}
		switch resp.ErrorCode {
		case "INSUFFICIENT_CREDITS":
			return ErrInsufficientBalance
		case "ORG_NOT_FOUND":
			return ErrBalanceNotFound
		case "INVALID_REQUEST":
			return ErrInvalidRequest
		default:
			return fmt.Errorf("pre-deduct failed: %s", resp.ErrorMessage)
		}
	}

	if resp.DeductionID == "" {
		return fmt.Errorf("pre-deduct succeeded but deduction_id is empty (request_id=%s)", bc.RequestID)
	}
	bc.DeductionID = resp.DeductionID
	if err := s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.bindRemoteDeductionID(ctx, tx, bc); err != nil {
			return err
		}
		return s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusPre, nil, nil, nil)
	}); err != nil {
		localRollbackErr := s.rollbackLocalSubjectQuota(ctx, bc)
		remoteCompErr := s.compensateRemoteReservationAfterBindFailure(ctx, bc)
		markErr := s.markAttemptPreDeductFailed(ctx, bc, "PREDEDUCT_BIND_FAILED", err.Error())
		if markErr != nil {
			return fmt.Errorf(
				"persist remote deduction binding failed: %v (local_rollback_err=%v, remote_compensation_err=%v, mark_err=%w)",
				err,
				localRollbackErr,
				remoteCompErr,
				markErr,
			)
		}
		if localRollbackErr != nil || remoteCompErr != nil {
			return fmt.Errorf(
				"persist remote deduction binding failed: %v (local_rollback_err=%v, remote_compensation_err=%v)",
				err,
				localRollbackErr,
				remoteCompErr,
			)
		}
		return fmt.Errorf("persist remote deduction binding failed: %w", err)
	}

	return nil
}

func (s *RemoteBilling) settleViaGRPC(ctx context.Context, bc *BillingContext) error {
	usageLane, err := normalizeBillingContextUsageLane(bc)
	if err != nil {
		return err
	}
	if usageLane != UsageBillingLanePlatform {
		return fmt.Errorf("remote billing requires platform lane, got %s (request_id=%s)", usageLane, bc.RequestID)
	}
	if bc.DeductionID == "" {
		return fmt.Errorf("missing deduction_id for settle (request_id=%s)", bc.RequestID)
	}
	if strings.TrimSpace(bc.AttemptID) == "" {
		return fmt.Errorf("missing attempt_id for settle (request_id=%s)", bc.RequestID)
	}
	terminal, err := s.isAttemptTerminal(ctx, bc.AttemptID)
	if err != nil {
		return err
	}
	if terminal {
		return nil
	}
	if bc.RequestCreatedAt.IsZero() {
		bc.RequestCreatedAt = time.Now().UTC()
	} else {
		bc.RequestCreatedAt = bc.RequestCreatedAt.UTC()
	}
	bc.SettledAt = time.Now().UTC()

	if err := s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		if err := s.localService.recordAttemptSettleInput(ctx, tx, bc); err != nil {
			return err
		}
		invocation := invocationResultFromBillingStatus(bc.Status)
		return s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusSettlePending, &invocation, nil, nil)
	}); err != nil {
		return fmt.Errorf("prepare settle pending state failed: %w", err)
	}

	req := &SettleQuotaRequest{
		OrganizationID:    bc.OrganizationID,
		DeductionID:       bc.DeductionID,
		EstimatedCredits:  bc.EstimatedCredits,
		ActualCredits:     bc.ActualCredits,
		PromptTokens:      bc.PromptTokens,
		CompletionTokens:  bc.CompletionTokens,
		TotalTokens:       bc.TotalTokens,
		InputCost:         bc.InputCost.InexactFloat64(),
		OutputCost:        bc.OutputCost.InexactFloat64(),
		TotalCost:         bc.TotalCost.InexactFloat64(),
		ModelID:           bc.ModelID.String(),
		ModelName:         bc.ModelName,
		ProviderID:        bc.ProviderID.String(),
		ProviderName:      bc.ProviderName,
		RequestID:         bc.RequestID,
		ResponseTime:      bc.ResponseTime,
		Status:            bc.Status,
		ErrorMessage:      bc.ErrorMessage,
		IsStreaming:       bc.IsStreaming,
		UseSystemProvider: bc.UseSystemProvider,
		IPAddress:         bc.IPAddress,
		UserAgent:         bc.UserAgent,
		AttemptID:         bc.AttemptID,
	}

	if bc.ChannelID != nil {
		req.ChannelID = bc.ChannelID.String()
	}

	if bc.AccountID != nil {
		req.AccountID = bc.AccountID.String()
	}

	if bc.AppID != nil {
		req.AppID = bc.AppID.String()
	}

	if bc.AppType != nil {
		req.AppType = *bc.AppType
	}

	resp, err := s.grpcClient.SettleQuota(ctx, req)
	if err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "SETTLE_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("grpc settle failed: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return fmt.Errorf("grpc settle failed: %w", err)
	}

	if !resp.Success {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "SETTLE_FAILED", resp.ErrorMessage); markErr != nil {
			return fmt.Errorf("settle failed: %s (additionally failed to mark partial: %w)", resp.ErrorMessage, markErr)
		}
		return fmt.Errorf("settle failed: %s", resp.ErrorMessage)
	}
	if err := applyRemoteSettlementResult(bc, resp); err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "SETTLE_INVALID_RESPONSE", err.Error()); markErr != nil {
			return fmt.Errorf("settle invalid response: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return err
	}

	if err := s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.settleSubjectQuota(ctx, tx, bc); err != nil {
			return fmt.Errorf("settle subject quota: %w", err)
		}
		invocation := "success"
		status := billingAttemptStatusSettled
		if !billingContextStatusIsSuccess(bc.Status) {
			invocation = "error"
			status = billingAttemptStatusRolledBack
		}
		if err := s.localService.updateAttemptEntriesAfterSettle(ctx, tx, bc, billingLedgerTypeOrgFunds, bc.OrganizationID); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).
			Model(&BillingAttempt{}).
			Where("attempt_id = ?", bc.AttemptID).
			Updates(map[string]interface{}{
				"reconcile_attempts": 0,
				"next_reconcile_at":  nil,
				"last_reconcile_at":  time.Now(),
			}).Error; err != nil {
			return err
		}
		if err := s.localService.updateAttemptStatus(ctx, tx, bc, status, &invocation, nil, nil); err != nil {
			return err
		}
		if shouldMirrorRemoteUsageBill(bc) {
			if err := s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusFromBillingContext(bc.Status), nil, nil); err != nil {
				return fmt.Errorf("upsert remote usage bill: %w", err)
			}
		}
		return nil
	}); err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "LOCAL_SUBJECT_SETTLE_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("finalize settle failed: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return fmt.Errorf("finalize settle failed: %w", err)
	}

	return nil
}

func shouldMirrorRemoteUsageBill(bc *BillingContext) bool {
	if bc == nil {
		return false
	}
	if strings.TrimSpace(bc.AttemptID) == "" || strings.TrimSpace(bc.RequestID) == "" {
		return false
	}
	// Main official traffic has full request/model/provider context; partial reconcile paths currently do not.
	return strings.TrimSpace(bc.ModelName) != "" && strings.TrimSpace(bc.ProviderName) != ""
}

func applyRemoteSettlementResult(bc *BillingContext, resp *SettleQuotaResponse) error {
	if bc == nil || resp == nil {
		return nil
	}
	if resp.SettledCredits < 0 {
		return fmt.Errorf("remote settle returned negative settled_credits (request_id=%s attempt_id=%s)", bc.RequestID, bc.AttemptID)
	}
	expectedCredits := maxInt64(bc.ActualCredits, 0)
	if billingContextStatusIsSuccess(bc.Status) && expectedCredits > 0 && resp.SettledCredits <= 0 {
		return fmt.Errorf("remote settle succeeded without settled_credits (request_id=%s attempt_id=%s)", bc.RequestID, bc.AttemptID)
	}
	bc.BillingLane = UsageBillingLanePlatform
	bc.UseSystemProvider = true
	bc.ActualCredits = resp.SettledCredits
	return nil
}

func (s *RemoteBilling) startPartialSettleReconcileWorker(ctx context.Context) {
	ticker := time.NewTicker(defaultReconcileSweepEvery)
	defer ticker.Stop()

	logger.InfoContext(ctx, "remote billing reconcile worker started",
		zap.Int64("sweep_every_ms", defaultReconcileSweepEvery.Milliseconds()),
		zap.Int("batch_size", defaultReconcileBatchSize),
	)
	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "remote billing reconcile worker stopped")
			return
		case <-ticker.C:
			if err := s.recoverStaleSettlePendingAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing stale settle pending recovery failed", err)
			}
			if err := s.reconcilePartialSettledAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing reconcile sweep failed", err)
			}
		}
	}
}

func (s *RemoteBilling) reconcilePartialSettledAttempts(ctx context.Context) error {
	now := time.Now()
	var attempts []BillingAttempt
	if err := s.localService.db.WithContext(ctx).
		Where("status = ? AND lane = ? AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)", billingAttemptStatusPartial, billingAttemptLaneRemote, now).
		Order("COALESCE(next_reconcile_at, updated_at) ASC").
		Limit(defaultReconcileBatchSize).
		Find(&attempts).Error; err != nil {
		return fmt.Errorf("query partial attempts: %w", err)
	}

	for _, attempt := range attempts {
		claimed, err := s.claimAttemptForReconcile(ctx, attempt.AttemptID, now)
		if err != nil {
			logger.ErrorContext(ctx, "remote billing reconcile claim failed",
				err,
				zap.String("attempt_id", attempt.AttemptID),
			)
			continue
		}
		if !claimed {
			continue
		}
		if err := s.reconcileAttempt(ctx, attempt.AttemptID); err != nil {
			logger.ErrorContext(ctx, "remote billing reconcile attempt failed",
				err,
				zap.String("attempt_id", attempt.AttemptID),
			)
			if scheduleErr := s.scheduleReconcileFailure(ctx, attempt.AttemptID, err); scheduleErr != nil {
				logger.ErrorContext(ctx, "remote billing reconcile failure scheduling failed",
					scheduleErr,
					zap.String("attempt_id", attempt.AttemptID),
				)
			}
		}
	}

	return nil
}

func (s *RemoteBilling) claimAttemptForReconcile(ctx context.Context, attemptID string, now time.Time) (bool, error) {
	res := s.localService.db.WithContext(ctx).
		Model(&BillingAttempt{}).
		Where(
			"attempt_id = ? AND status = ? AND lane = ? AND reconcile_attempts < ? AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)",
			attemptID,
			billingAttemptStatusPartial,
			billingAttemptLaneRemote,
			defaultReconcileMaxRetries,
			now,
		).
		Updates(map[string]interface{}{
			"status":             billingAttemptStatusSettlePending,
			"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
			"last_reconcile_at":  now,
			"next_reconcile_at":  nil,
			"updated_at":         now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (s *RemoteBilling) recoverStaleSettlePendingAttempts(ctx context.Context) error {
	cutoff := time.Now().Add(-defaultSettlePendingTimeout)
	return s.localService.db.WithContext(ctx).
		Model(&BillingAttempt{}).
		Where("status = ? AND lane = ? AND updated_at < ?", billingAttemptStatusSettlePending, billingAttemptLaneRemote, cutoff).
		Updates(map[string]interface{}{
			"status":            billingAttemptStatusPartial,
			"next_reconcile_at": time.Now(),
			"error_code":        "RECONCILE_STALE_SETTLE_PENDING",
			"error_message":     fmt.Sprintf("auto-recovered stale settle pending after %s", defaultSettlePendingTimeout),
			"updated_at":        time.Now(),
		}).Error
}

func (s *RemoteBilling) reconcileAttempt(ctx context.Context, attemptID string) error {
	var attempt BillingAttempt
	if err := s.localService.db.WithContext(ctx).
		Where("attempt_id = ?", attemptID).
		First(&attempt).Error; err != nil {
		return fmt.Errorf("load attempt: %w", err)
	}

	var entries []BillingAttemptEntry
	if err := s.localService.db.WithContext(ctx).
		Where("attempt_id = ?", attemptID).
		Find(&entries).Error; err != nil {
		return fmt.Errorf("load attempt entries: %w", err)
	}

	var fundEntry *BillingAttemptEntry
	var subjectEntry *BillingAttemptEntry
	for i := range entries {
		entry := &entries[i]
		if entry.EntryType == billingEntryTypeFund && entry.LedgerType == billingLedgerTypeOrgFunds {
			fundEntry = entry
		}
		if entry.EntryType == billingEntryTypeSubject {
			subjectEntry = entry
		}
	}

	if fundEntry == nil {
		return s.markAttemptDeadLetter(ctx, attemptID, "missing remote fund entry")
	}
	if fundEntry.IdempotencyKey == nil || strings.TrimSpace(*fundEntry.IdempotencyKey) == "" {
		return fmt.Errorf("%w: attempt_id=%s", errReconcileMissingDeductionID, attemptID)
	}

	actualCredits := fundEntry.ActualAmount
	if subjectEntry != nil && subjectEntry.ActualAmount > 0 {
		actualCredits = subjectEntry.ActualAmount
	}
	if actualCredits == 0 {
		actualCredits = fundEntry.ReservedAmount
	}

	settleStatus := "success"
	if attempt.InvocationResult != nil && strings.EqualFold(*attempt.InvocationResult, "error") {
		settleStatus = "error"
	}

	bc := &BillingContext{
		OrganizationID:    attempt.OrganizationID.String(),
		DeductionID:       strings.TrimSpace(*fundEntry.IdempotencyKey),
		AttemptID:         attempt.AttemptID,
		RequestID:         attempt.RequestID,
		EstimatedCredits:  fundEntry.ReservedAmount,
		ActualCredits:     actualCredits,
		QuotaSubjectType:  attempt.QuotaSubjectType,
		QuotaSubjectID:    attempt.QuotaSubjectID,
		BillingLane:       UsageBillingLanePlatform,
		UseSystemProvider: true,
		Status:            settleStatus,
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeAPIKey && strings.TrimSpace(attempt.QuotaSubjectID) != "" {
		bc.APIKeyID = strings.TrimSpace(attempt.QuotaSubjectID)
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeWorkspace && strings.TrimSpace(attempt.QuotaSubjectID) != "" {
		bc.WorkspaceID = strings.TrimSpace(attempt.QuotaSubjectID)
	}
	if attempt.ProviderID != nil {
		bc.ProviderID = *attempt.ProviderID
	}
	if attempt.ModelID != nil {
		bc.ModelID = *attempt.ModelID
	}
	if attempt.RouteID != nil {
		bc.ChannelID = attempt.RouteID
	}

	return s.settleViaGRPC(ctx, bc)
}

func (s *RemoteBilling) markAttemptDeadLetter(ctx context.Context, attemptID string, reason string) error {
	return s.localService.db.WithContext(ctx).
		Model(&BillingAttempt{}).
		Where("attempt_id = ?", attemptID).
		Updates(map[string]interface{}{
			"status":            billingAttemptStatusDeadLetter,
			"error_code":        "ATTEMPT_RECONCILE_FAILED",
			"error_message":     reason,
			"next_reconcile_at": nil,
			"updated_at":        time.Now(),
		}).Error
}

func (s *RemoteBilling) scheduleReconcileFailure(ctx context.Context, attemptID string, reconcileErr error) error {
	errorCode := "ATTEMPT_RECONCILE_FAILED"
	if errors.Is(reconcileErr, errReconcileMissingDeductionID) {
		errorCode = "RECONCILE_MISSING_DEDUCTION_ID"
	}

	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ?", attemptID).
			First(&attempt).Error; err != nil {
			return err
		}

		if attempt.Status != billingAttemptStatusSettlePending {
			return nil
		}

		if attempt.ReconcileAttempts >= defaultReconcileMaxRetries {
			return tx.WithContext(ctx).
				Model(&BillingAttempt{}).
				Where("attempt_id = ?", attemptID).
				Updates(map[string]interface{}{
					"status":            billingAttemptStatusDeadLetter,
					"error_code":        errorCode,
					"error_message":     fmt.Sprintf("max retries reached(%d): %v", attempt.ReconcileAttempts, reconcileErr),
					"next_reconcile_at": nil,
					"updated_at":        time.Now(),
				}).Error
		}

		backoff := reconcileBackoff(attempt.ReconcileAttempts)
		return tx.WithContext(ctx).
			Model(&BillingAttempt{}).
			Where("attempt_id = ?", attemptID).
			Updates(map[string]interface{}{
				"status":            billingAttemptStatusPartial,
				"error_code":        errorCode,
				"error_message":     reconcileErr.Error(),
				"next_reconcile_at": time.Now().Add(backoff),
				"updated_at":        time.Now(),
			}).Error
	})
}

func reconcileBackoff(reconcileAttempts int) time.Duration {
	if reconcileAttempts <= 0 {
		return defaultReconcileBaseBackoff
	}
	backoff := defaultReconcileBaseBackoff
	for i := 1; i < reconcileAttempts; i++ {
		backoff *= 2
		if backoff >= defaultReconcileMaxBackoff {
			return defaultReconcileMaxBackoff
		}
	}
	if backoff > defaultReconcileMaxBackoff {
		return defaultReconcileMaxBackoff
	}
	return backoff
}

func (s *RemoteBilling) preDeductLocalSubjectQuota(ctx context.Context, bc *BillingContext) error {
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}

		subjectType := strings.TrimSpace(bc.QuotaSubjectType)
		if subjectType == "" {
			return fmt.Errorf(
				"missing quota_subject_type for subject pre-deduct (attempt_id=%s request_id=%s)",
				strings.TrimSpace(bc.AttemptID),
				strings.TrimSpace(bc.RequestID),
			)
		}
		bc.QuotaSubjectType = subjectType

		switch subjectType {
		case quotaSubjectTypeAPIKey:
			apiKeyID := strings.TrimSpace(bc.APIKeyID)
			if apiKeyID == "" {
				return fmt.Errorf(
					"missing api_key_id for subject pre-deduct (attempt_id=%s request_id=%s)",
					strings.TrimSpace(bc.AttemptID),
					strings.TrimSpace(bc.RequestID),
				)
			}
			subjectID := strings.TrimSpace(bc.QuotaSubjectID)
			if subjectID == "" {
				return fmt.Errorf(
					"missing quota_subject_id for key subject pre-deduct (attempt_id=%s request_id=%s)",
					strings.TrimSpace(bc.AttemptID),
					strings.TrimSpace(bc.RequestID),
				)
			}
			if subjectID != apiKeyID {
				return fmt.Errorf(
					"quota subject mismatch for key subject pre-deduct: quota_subject_id=%s api_key_id=%s attempt_id=%s request_id=%s",
					subjectID,
					apiKeyID,
					strings.TrimSpace(bc.AttemptID),
					strings.TrimSpace(bc.RequestID),
				)
			}
			bc.APIKeyID = apiKeyID
			bc.QuotaSubjectID = subjectID

			var apiKey apikeymodel.TenantAPIKey
			if err := tx.WithContext(ctx).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ? AND organization_id = ?", apiKeyID, bc.OrganizationID).
				First(&apiKey).Error; err != nil {
				return fmt.Errorf("load api key for subject pre-deduct: %w", err)
			}
			if apiKey.Status != "active" {
				return ErrAPIKeyInactive
			}
			return s.localService.preDeductSubjectQuota(ctx, tx, bc, &apiKey)

		case quotaSubjectTypeWorkspace:
			if strings.TrimSpace(bc.QuotaSubjectID) == "" {
				return fmt.Errorf("missing workspace_id for subject pre-deduct")
			}
			return s.localService.preDeductSubjectQuota(ctx, tx, bc, nil)
		case quotaSubjectTypeOrganization:
			if strings.TrimSpace(bc.QuotaSubjectID) == "" {
				return fmt.Errorf("missing organization_id for subject pre-deduct")
			}
			return s.localService.preDeductSubjectQuota(ctx, tx, bc, nil)

		default:
			return fmt.Errorf("unsupported quota subject type: %s", subjectType)
		}
	})
}

func (s *RemoteBilling) rollbackLocalSubjectQuota(ctx context.Context, bc *BillingContext) error {
	rollbackCtx := *bc
	rollbackCtx.ActualCredits = 0
	rollbackCtx.Status = "error"
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.upsertAttemptInit(ctx, tx, &rollbackCtx); err != nil {
			return err
		}
		if err := s.localService.settleSubjectQuota(ctx, tx, &rollbackCtx); err != nil {
			return err
		}
		return nil
	})
}

func (s *RemoteBilling) compensateRemoteReservationAfterBindFailure(ctx context.Context, bc *BillingContext) error {
	deductionID := strings.TrimSpace(bc.DeductionID)
	if deductionID == "" {
		return fmt.Errorf("missing deduction_id for remote reservation compensation")
	}
	req := &SettleQuotaRequest{
		OrganizationID:    bc.OrganizationID,
		DeductionID:       deductionID,
		EstimatedCredits:  bc.EstimatedCredits,
		ActualCredits:     0,
		ModelID:           bc.ModelID.String(),
		ModelName:         bc.ModelName,
		ProviderID:        bc.ProviderID.String(),
		ProviderName:      bc.ProviderName,
		RequestID:         bc.RequestID,
		Status:            "error",
		ErrorMessage:      "prededuct binding failed before provider invocation",
		UseSystemProvider: true,
		AttemptID:         bc.AttemptID,
	}
	resp, err := s.grpcClient.SettleQuota(ctx, req)
	if err != nil {
		return fmt.Errorf("grpc compensate remote reservation failed: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("grpc compensate remote reservation failed: %s", resp.ErrorMessage)
	}
	return nil
}

func (s *RemoteBilling) markAttemptPreDeductFailed(ctx context.Context, bc *BillingContext, code, msg string) error {
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		invocation := "error"
		return s.localService.updateAttemptStatus(
			ctx,
			tx,
			bc,
			billingAttemptStatusPredeductFailed,
			&invocation,
			&code,
			&msg,
		)
	})
}

func (s *RemoteBilling) markAttemptSettleFailed(ctx context.Context, bc *BillingContext, code, msg string) error {
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		invocation := invocationResultFromBillingStatus(bc.Status)
		return s.localService.updateAttemptStatus(
			ctx,
			tx,
			bc,
			billingAttemptStatusPartial,
			&invocation,
			&code,
			&msg,
		)
	})
}

func (s *RemoteBilling) isAttemptTerminal(ctx context.Context, attemptID string) (bool, error) {
	var attempt BillingAttempt
	err := s.localService.db.WithContext(ctx).
		Select("status").
		Where("attempt_id = ?", attemptID).
		First(&attempt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load attempt status: %w", err)
	}
	return attempt.Status == billingAttemptStatusSettled || attempt.Status == billingAttemptStatusRolledBack, nil
}

func (s *RemoteBilling) Close() error {
	if s.grpcClient != nil {
		return s.grpcClient.Close()
	}
	return nil
}
