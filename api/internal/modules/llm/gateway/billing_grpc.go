package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RemoteBilling implements BillingProvider by forwarding PreDeduct/Settle
// to console-api via gRPC. CalculateCreditsFromTokens is handled locally
// since it only needs read access to zgi-api DB.
// This is used in CLOUD mode where console-api is the billing authority.
type RemoteBilling struct {
	localService      *BillingService
	grpcClient        quotaClient
	consoleAPIBaseURL string
	reconcileWorkerID string
}

type quotaClient interface {
	PreDeductQuota(ctx context.Context, req *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error)
	PreDeductMeteredUsage(ctx context.Context, req *PreDeductMeteredUsageRequest) (*PreDeductMeteredUsageResponse, error)
	SettleQuota(ctx context.Context, req *SettleQuotaRequest) (*SettleQuotaResponse, error)
	CheckCreditBalance(ctx context.Context, organizationID string, estimatedCredits int64) (bool, int64, error)
	Close() error
}

type authoritativeTokenCostClient interface {
	CalculateDualCost(ctx context.Context, modelID, provider, model string, promptTokens, completionTokens int) (*DualCostQuotaResponse, error)
}

type billingCapabilityClient interface {
	GetBillingCapabilities(context.Context) (bool, error)
}

func (s *RemoteBilling) SupportsToolPostpaid(ctx context.Context) (bool, error) {
	client, ok := s.grpcClient.(billingCapabilityClient)
	if !ok {
		logger.Warn("billing capability downgrade", zap.String("capability", "tool_postpaid_v2"), zap.String("reason", "client_method_unavailable"))
		return false, nil
	}
	supported, err := client.GetBillingCapabilities(ctx)
	if err != nil {
		logger.Warn("billing capability negotiation failed", zap.String("capability", "tool_postpaid_v2"), zap.Error(err))
		return false, err
	}
	if !supported {
		logger.Warn("billing capability downgrade", zap.String("capability", "tool_postpaid_v2"), zap.String("reason", "console_unsupported"))
	}
	return supported, nil
}

const (
	defaultReconcileSweepEvery            = 30 * time.Second
	defaultReconcileBatchSize             = 50
	defaultReconcileMaxRetries            = 8
	defaultReconcileBaseBackoff           = 30 * time.Second
	defaultReconcileMaxBackoff            = 10 * time.Minute
	defaultSettlePendingTimeout           = 2 * time.Minute
	defaultRemotePredeductTimeout         = 3 * time.Minute
	defaultRemoteInitTimeout              = 10 * time.Minute
	defaultRemoteCleanupTimeout           = 10 * time.Second
	reservationPolicyAuthoritativeQuoteV1 = "authoritative_quote_v1"
)

var (
	errReconcileMissingDeductionID = errors.New("missing deduction_id for remote reconcile")
	errRemoteAttemptNotBindable    = errors.New("remote billing attempt is no longer awaiting deduction binding")
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
		localService:      localService,
		grpcClient:        client,
		reconcileWorkerID: "pre-settlement-" + uuid.NewString(),
	}
	if base, baseErr := resolveOfficialRouteBaseURL(); baseErr == nil {
		rb.consoleAPIBaseURL = base
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

func (s *RemoteBilling) QuotePlatformTokenPricing(
	ctx context.Context,
	model PricingModelRef,
	promptTokens int,
	completionTokens int,
) (PricingQuote, error) {
	model = normalizePricingModelRef(model)
	if model.Provider == "" || model.Model == "" {
		return PricingQuote{}, fmt.Errorf("canonical provider and model are required for authoritative token pricing")
	}
	client, ok := s.grpcClient.(authoritativeTokenCostClient)
	if !ok {
		return PricingQuote{}, fmt.Errorf("remote billing does not support authoritative token pricing")
	}
	modelID := ""
	if model.ModelID != uuid.Nil {
		modelID = model.ModelID.String()
	}
	// Console resolves the authoritative reservation ceiling with the same
	// normalized pricing path used by proxy settlement, including the highest
	// sale price across every catalog provider row for the logical model.
	resp, err := client.CalculateDualCost(ctx, modelID, model.Provider, model.Model, promptTokens, completionTokens)
	if err != nil {
		// A stable Console ErrorInfo.Reason must classify the failure (for
		// example billing.model.pricing_not_configured -> 402) instead of
		// collapsing into a generic internal_error at the projector.
		if mapped, ok := mapBillingRPCErrorCode(billingRPCErrorReason(err), ""); ok {
			return PricingQuote{}, wrapPricingNotConfiguredError(mapped, pricingErrorParamsFromModelRef(model))
		}
		return PricingQuote{}, fmt.Errorf("calculate authoritative token cost: %w", err)
	}
	if resp == nil || !resp.Success {
		message := "authoritative token pricing failed"
		if resp != nil && strings.TrimSpace(resp.ErrorMessage) != "" {
			message = resp.ErrorMessage
		}
		return PricingQuote{}, fmt.Errorf("%s", message)
	}
	if resp.TotalCredits <= 0 && (promptTokens > 0 || completionTokens > 0) {
		return PricingQuote{}, fmt.Errorf("authoritative token pricing returned no credits")
	}
	inputPrice, inputResolved, err := deriveAuthoritativeTokenPrice(resp.InputUSD, promptTokens)
	if err != nil {
		return PricingQuote{}, fmt.Errorf("derive authoritative input token price: %w", err)
	}
	outputPrice, outputResolved, err := deriveAuthoritativeTokenPrice(resp.OutputUSD, completionTokens)
	if err != nil {
		return PricingQuote{}, fmt.Errorf("derive authoritative output token price: %w", err)
	}
	return PricingQuote{
		InputCredits:      resp.InputCredits,
		OutputCredits:     resp.OutputCredits,
		TotalCredits:      resp.TotalCredits,
		InputUSD:          resp.InputUSD,
		OutputUSD:         resp.OutputUSD,
		TotalUSD:          resp.TotalUSD,
		PricingSource:     PricingSourceUpstreamModelPrice,
		UsageSource:       UsageSourceEstimatedUsage,
		ReservationPolicy: reservationPolicyAuthoritativeQuoteV1,

		InputTokenPriceUSDPer1M:  inputPrice,
		OutputTokenPriceUSDPer1M: outputPrice,
		InputTokenPriceResolved:  inputResolved,
		OutputTokenPriceResolved: outputResolved,
		PricingSnapshot: buildPricingSnapshot(map[string]interface{}{
			"pricing_source":                 PricingSourceUpstreamModelPrice,
			"usage_source":                   UsageSourceEstimatedUsage,
			"reservation_policy":             reservationPolicyAuthoritativeQuoteV1,
			"provider":                       model.Provider,
			"model":                          model.Model,
			"prompt_tokens":                  promptTokens,
			"completion_tokens":              completionTokens,
			"input_credits":                  resp.InputCredits,
			"output_credits":                 resp.OutputCredits,
			"total_credits":                  resp.TotalCredits,
			"input_usd":                      resp.InputUSD.String(),
			"output_usd":                     resp.OutputUSD.String(),
			"total_usd":                      resp.TotalUSD.String(),
			"input_price_usd_per_1m_tokens":  inputPrice.String(),
			"output_price_usd_per_1m_tokens": outputPrice.String(),
		}),
	}, nil
}

func deriveAuthoritativeTokenPrice(costUSD decimal.Decimal, tokenCount int) (decimal.Decimal, bool, error) {
	if tokenCount < 0 || costUSD.IsNegative() {
		return decimal.Zero, false, fmt.Errorf("invalid authoritative token cost")
	}
	if tokenCount == 0 {
		if !costUSD.IsZero() {
			return decimal.Zero, false, fmt.Errorf("non-zero cost without tokens")
		}
		return decimal.Zero, false, nil
	}
	price := costUSD.
		Mul(decimal.NewFromInt(1_000_000)).
		Div(decimal.NewFromInt(int64(tokenCount)))
	return price, true, nil
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

	fact, hasMeteredFact := shared.MeteredUsageFromContext(ctx)
	if meteredPredeductEligible(bc, fact, hasMeteredFact) {
		return s.preDeductMeteredViaGRPC(ctx, bc, fact)
	}

	req := &PreDeductQuotaRequest{
		ToolPostpaid:      bc.ToolPostpaid,
		OrganizationID:    bc.OrganizationID,
		EstimatedCredits:  bc.EstimatedCredits,
		ModelID:           bc.ModelID.String(),
		ModelName:         bc.ModelName,
		ProviderID:        bc.ProviderID.String(),
		ProviderName:      bc.ProviderName,
		RequestID:         bc.RequestID,
		AttemptID:         bc.AttemptID,
		ReservationPolicy: bc.ReservationPolicy,
		PricingSnapshot:   pricingSnapshotJSON(bc.PricingSnapshot),
	}

	resp, err := s.grpcClient.PreDeductQuota(ctx, req)
	if err != nil {
		if rollbackErr := s.rollbackLocalSubjectQuotaAndMarkFailed(ctx, bc, "PREDEDUCT_FAILED", err.Error()); rollbackErr != nil {
			return fmt.Errorf("grpc pre-deduct failed: %v (local subject rollback failed: %w)", err, rollbackErr)
		}
		if mapped, ok := mapBillingRPCErrorCode(billingRPCErrorReason(err), ""); ok {
			return mapped
		}
		return fmt.Errorf("grpc pre-deduct failed: %w", err)
	}

	if !resp.Success {
		if rollbackErr := s.rollbackLocalSubjectQuotaAndMarkFailed(ctx, bc, "PREDEDUCT_FAILED", resp.ErrorMessage); rollbackErr != nil {
			return fmt.Errorf("pre-deduct failed: %s (local subject rollback failed: %w)", resp.ErrorMessage, rollbackErr)
		}
		if mapped, ok := mapBillingRPCErrorCode(resp.ErrorCode, resp.ErrorMessage); ok {
			return mapped
		}
		return fmt.Errorf("pre-deduct failed: %s", resp.ErrorMessage)
	}

	if err := s.finalizeRemotePreDeductBinding(ctx, bc, resp.DeductionID); err != nil {
		return err
	}

	return nil
}

// finalizeRemotePreDeductBinding is the shared tail of both remote pre-deduct
// paths: it validates the non-empty deduction id, binds it to the local attempt,
// and compensates the remote reservation when that binding cannot commit.
func (s *RemoteBilling) finalizeRemotePreDeductBinding(ctx context.Context, bc *BillingContext, deductionID string) error {
	if strings.TrimSpace(deductionID) == "" {
		invalidResponseErr := fmt.Errorf("pre-deduct succeeded but deduction_id is empty (request_id=%s)", bc.RequestID)
		if rollbackErr := s.rollbackLocalSubjectQuotaAndMarkFailed(ctx, bc, "PREDEDUCT_INVALID_RESPONSE", invalidResponseErr.Error()); rollbackErr != nil {
			return fmt.Errorf("%v (local subject rollback failed: %w)", invalidResponseErr, rollbackErr)
		}
		return invalidResponseErr
	}
	bc.DeductionID = strings.TrimSpace(deductionID)
	if err := s.localService.db.Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.WithContext(ctx).
			Select("attempt_id", "status").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ? AND lane = ?", bc.AttemptID, billingAttemptLaneRemote).
			First(&attempt).Error; err != nil {
			return err
		}
		if attempt.Status != billingAttemptStatusInit {
			return fmt.Errorf("%w: attempt_id=%s status=%s", errRemoteAttemptNotBindable, bc.AttemptID, attempt.Status)
		}
		if err := s.localService.bindRemoteDeductionID(ctx, tx, bc); err != nil {
			return err
		}
		if err := s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusPre, nil, nil, nil); err != nil {
			return err
		}
		if bc.ToolPostpaid {
			markToolPostpaid(bc)
			return s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusPartial, nil, nil)
		}
		return nil
	}); err != nil {
		// Provider pre-deduct has already committed remotely. Finish or durably
		// queue the cleanup even if the client request was canceled meanwhile.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), defaultRemoteCleanupTimeout)
		remoteCompErr := s.compensateRemoteReservationAfterBindFailure(cleanupCtx, bc)
		cancelCleanup()
		if remoteCompErr != nil {
			persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), defaultRemoteCleanupTimeout)
			persistErr := s.persistRemoteCompensationPending(persistCtx, bc, err, remoteCompErr)
			cancelPersist()
			if persistErr != nil {
				return fmt.Errorf(
					"persist remote deduction binding failed: %v (remote_compensation_err=%v, compensation_persistence_err=%w)",
					err,
					remoteCompErr,
					persistErr,
				)
			}
			return fmt.Errorf(
				"persist remote deduction binding failed: %v (remote compensation queued for retry: %w)",
				err,
				remoteCompErr,
			)
		}
		if errors.Is(err, errRemoteAttemptNotBindable) {
			return fmt.Errorf("remote deduction arrived after local attempt closed and was compensated: %w", err)
		}
		rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), defaultRemoteCleanupTimeout)
		localRollbackErr := s.rollbackLocalSubjectQuotaAndMarkFailed(rollbackCtx, bc, "PREDEDUCT_BIND_FAILED", err.Error())
		cancelRollback()
		if localRollbackErr != nil {
			return fmt.Errorf(
				"persist remote deduction binding failed: %v (local_rollback_err=%v)",
				err,
				localRollbackErr,
			)
		}
		return fmt.Errorf("persist remote deduction binding failed: %w", err)
	}
	return nil
}

func pricingSnapshotJSON(snapshot datatypes.JSON) string {
	raw := strings.TrimSpace(string(snapshot))
	if raw == "" || raw == "null" {
		return ""
	}
	if !json.Valid(snapshot) {
		return ""
	}
	return raw
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
	if bc.ToolPostpaid || isToolPostpaidSnapshot(bc.PricingSnapshot) {
		bc.ToolPostpaid = true
		return s.FinalizePlatformProxySettlement(ctx, bc, nil)
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

	alreadyFinalized, err := s.prepareRemoteSettlement(ctx, bc)
	if err != nil {
		return fmt.Errorf("prepare settle pending state failed: %w", err)
	}
	if alreadyFinalized {
		return nil
	}

	req := &SettleQuotaRequest{
		OrganizationID:    bc.OrganizationID,
		DeductionID:       bc.DeductionID,
		EstimatedCredits:  bc.EstimatedCredits,
		ActualCredits:     bc.ActualCredits,
		PromptTokens:      bc.PromptTokens,
		CompletionTokens:  bc.CompletionTokens,
		TotalTokens:       bc.TotalTokens,
		InputCost:         bc.InputCost,
		OutputCost:        bc.OutputCost,
		TotalCost:         bc.TotalCost,
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
	if strings.TrimSpace(resp.ToolSettlementJSON) != "" {
		if err := s.mirrorPlatformTools(ctx, bc, json.RawMessage(resp.ToolSettlementJSON), bc.PricingSnapshot); err != nil {
			if markErr := s.markAttemptSettleFailed(ctx, bc, "TOOL_MIRROR_PENDING", err.Error()); markErr != nil {
				return fmt.Errorf("tool mirror failed: %v; persist retry: %w", err, markErr)
			}
			return err
		}
		return nil
	}
	if bc.ToolPostpaid || isToolPostpaidSnapshot(bc.PricingSnapshot) {
		supported, capabilityErr := s.SupportsToolPostpaid(ctx)
		if capabilityErr != nil {
			return fmt.Errorf("verify billing capability after missing tool projection: %w", capabilityErr)
		}
		if supported {
			return fmt.Errorf("tool settlement response omitted authoritative projection")
		}
		logger.Warn("billing settlement downgraded to legacy token contract", zap.String("attempt_id", bc.AttemptID))
		clearToolPostpaid(bc)
	}
	if err := applyRemoteSettlementResult(bc, resp); err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "SETTLE_INVALID_RESPONSE", err.Error()); markErr != nil {
			return fmt.Errorf("settle invalid response: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return err
	}

	if err := s.finalizeRemoteSettlement(ctx, bc); err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "LOCAL_SUBJECT_SETTLE_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("finalize settle failed: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return fmt.Errorf("finalize settle failed: %w", err)
	}

	return nil
}

func (s *RemoteBilling) prepareRemoteSettlement(ctx context.Context, bc *BillingContext) (bool, error) {
	alreadyFinalized := false
	err := s.localService.db.Transaction(func(tx *gorm.DB) error {
		terminal, err := s.localService.lockAttemptForFinalization(ctx, tx, bc.AttemptID)
		if err != nil {
			return err
		}
		if terminal {
			alreadyFinalized = true
			return nil
		}
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		if err := s.localService.recordAttemptSettleInput(ctx, tx, bc); err != nil {
			return err
		}
		invocation := invocationResultFromBillingStatus(bc.Status)
		if err := s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusSettlePending, &invocation, nil, nil); err != nil {
			return err
		}
		return tx.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"settlement_state": billingAttemptStatusSettling, "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": time.Now()}).Error
	})
	return alreadyFinalized, err
}

func (s *RemoteBilling) finalizeRemoteSettlement(ctx context.Context, bc *BillingContext) error {
	if (bc.ToolPostpaid || isToolPostpaidSnapshot(bc.PricingSnapshot)) && billingContextStatusIsSuccess(bc.Status) {
		return fmt.Errorf("tool settlement requires authoritative JSON before finalizing mirror")
	}
	// Resolve route metadata before locking the billing transaction.
	s.localService.applyRouteAttributionWithLookup(ctx, bc)
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		terminal, err := s.localService.lockAttemptForFinalization(ctx, tx, bc.AttemptID)
		if err != nil {
			return err
		}
		if terminal {
			return nil
		}
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
		settlementState := billingAttemptStatusSettled
		if status == billingAttemptStatusRolledBack {
			settlementState = billingAttemptStatusRolledBack
		}
		if err := tx.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"settlement_state": settlementState, "console_settlement_id": strings.TrimSpace(bc.DeductionID), "console_settlement_status": settlementState, "settlement_next_retry_at": nil, "settlement_last_error_code": nil, "settlement_last_error_message": nil, "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		finalizeMediaTimingSnapshot(bc)
		if shouldMirrorRemoteUsageBill(bc) {
			if err := s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusFromBillingContext(bc.Status), nil, nil); err != nil {
				return fmt.Errorf("upsert remote usage bill: %w", err)
			}
		}
		return nil
	})
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
			if err := s.reconcilePendingSettlementQueries(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing settlement query sweep failed", err)
			}
			if err := s.recoverStaleRemoteInitAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing stale init recovery failed", err)
			}
			if err := s.recoverStalePredeductedRemoteAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing stale prededuct recovery failed", err)
			}
			if err := s.recoverStaleSettlePendingAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing stale settle pending recovery failed", err)
			}
			if err := s.reconcilePendingRemoteCompensations(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing compensation reconcile sweep failed", err)
			}
			if err := s.reconcilePartialSettledAttempts(ctx); err != nil {
				logger.ErrorContext(ctx, "remote billing reconcile sweep failed", err)
			}
		}
	}
}

// reconcilePendingSettlementQueries confirms attempts whose stream did not
// carry a settlement event. A positive Console response is recorded as
// authoritative evidence, but local ledger mirroring remains a separate
// transaction and is never assumed complete.
func (s *RemoteBilling) reconcilePendingSettlementQueries(ctx context.Context) error {
	now := time.Now()
	attempts, err := s.claimPendingSettlementAttempts(ctx, now)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		logger.InfoContext(ctx, "settlement query reconcile attempt", zap.String("attempt_id", attempt.AttemptID), zap.String("request_id", attempt.RequestID), zap.String("settlement_state", attempt.SettlementState), zap.Int("retry_count", attempt.SettlementRetryCount))
		result, err := queryConsoleSettlement(ctx, s.consoleAPIBaseURL+"/billing/settlements/by-attempt", attempt.OrganizationID.String(), attempt.AttemptID)
		if err != nil {
			logger.WarnContext(ctx, "settlement query failed; queued for retry", zap.String("attempt_id", attempt.AttemptID), zap.String("request_id", attempt.RequestID), zap.Int("retry_count", attempt.SettlementRetryCount+1), zap.Error(err))
			_ = s.localService.db.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ? AND settlement_claimed_by = ?", attempt.AttemptID, s.reconcileWorkerID).Updates(map[string]interface{}{"settlement_state": billingAttemptStatusFailedRetryable, "settlement_last_error_code": "SETTLEMENT_QUERY_FAILED", "settlement_last_error_message": err.Error(), "settlement_retry_count": gorm.Expr("settlement_retry_count + 1"), "settlement_next_retry_at": now.Add(reconcileBackoff(attempt.SettlementRetryCount + 1)), "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": now})
			s.releaseSettlementClaim(ctx, attempt.AttemptID)
			continue
		}
		updates := map[string]interface{}{"console_settlement_status": result.SettlementStatus, "settlement_next_retry_at": nil, "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": now}
		if strings.TrimSpace(result.SettlementID) != "" {
			updates["console_settlement_id"] = result.SettlementID
		}
		if result.Found && strings.EqualFold(result.SettlementStatus, "SETTLED") {
			updates["settlement_state"] = billingAttemptStatusPartial
			// Console is authoritative, but local mirroring is still required.
			// Rebuild the minimal billing context from the durable attempt ledger;
			// no stream metadata is trusted for this recovery path.
			if err := s.mirrorQueriedSettlement(ctx, &attempt, result); err != nil {
				logger.ErrorContext(ctx, "authoritative settlement found but local mirror failed", err, zap.String("attempt_id", attempt.AttemptID), zap.String("settlement_id", result.SettlementID), zap.String("result", "partial"))
				updates["settlement_last_error_code"] = "LOCAL_MIRROR_FAILED"
				updates["settlement_last_error_message"] = err.Error()
				updates["settlement_retry_count"] = gorm.Expr("settlement_retry_count + 1")
			} else {
				logger.InfoContext(ctx, "settlement reconciliation completed", zap.String("attempt_id", attempt.AttemptID), zap.String("settlement_id", result.SettlementID), zap.String("result", "mirrored"))
				updates["settlement_state"] = billingAttemptStatusSettled
			}
		} else if !result.Found {
			retry := attempt.SettlementRetryCount + 1
			updates["settlement_state"] = billingAttemptStatusFailedRetryable
			updates["settlement_last_error_code"] = "SETTLEMENT_NOT_FOUND"
			updates["settlement_last_error_message"] = "console settlement not found"
			updates["settlement_retry_count"] = gorm.Expr("settlement_retry_count + 1")
			updates["settlement_next_retry_at"] = now.Add(reconcileBackoff(retry))
			if retry >= defaultReconcileMaxRetries {
				updates["settlement_state"] = billingAttemptStatusFailedFinal
			}
		} else if result.Found && !result.Retryable {
			updates["settlement_state"] = billingAttemptStatusFailedFinal
		}
		updates["settlement_claimed_by"] = nil
		updates["settlement_claimed_until"] = nil
		_ = s.localService.db.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ? AND settlement_claimed_by = ?", attempt.AttemptID, s.reconcileWorkerID).Updates(updates)
		s.releaseSettlementClaim(ctx, attempt.AttemptID)
	}
	return nil
}

func (s *RemoteBilling) claimPendingSettlementAttempts(ctx context.Context, now time.Time) ([]BillingAttempt, error) {
	var attempts []BillingAttempt
	err := s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("lane = ? AND settlement_state IN ? AND (settlement_next_retry_at IS NULL OR settlement_next_retry_at <= ?) AND (settlement_claimed_until IS NULL OR settlement_claimed_until < ?)", billingAttemptLaneRemote, []string{"PENDING", billingAttemptStatusSettlePending, billingAttemptStatusSettling, billingAttemptStatusPartial, billingAttemptStatusFailedRetryable}, now, now).Order("updated_at ASC").Limit(defaultReconcileBatchSize).Find(&attempts).Error; err != nil {
			return err
		}
		leaseUntil := now.Add(2 * time.Minute)
		for i := range attempts {
			if err := tx.Model(&BillingAttempt{}).Where("attempt_id = ?", attempts[i].AttemptID).Updates(map[string]interface{}{"settlement_claimed_by": s.reconcileWorkerID, "settlement_claimed_until": leaseUntil, "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return attempts, err
}

func (s *RemoteBilling) releaseSettlementClaim(ctx context.Context, attemptID string) {
	_ = s.localService.db.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ? AND settlement_claimed_by = ?", attemptID, s.reconcileWorkerID).Updates(map[string]interface{}{"settlement_claimed_by": nil, "settlement_claimed_until": nil})
}

func (s *RemoteBilling) mirrorQueriedSettlement(ctx context.Context, attempt *BillingAttempt, result *settlementQueryResponse) error {
	if attempt == nil || result == nil || strings.TrimSpace(result.SettlementID) == "" {
		return fmt.Errorf("incomplete authoritative settlement result")
	}
	if result.AttemptID != "" && result.AttemptID != attempt.AttemptID {
		return fmt.Errorf("settlement attempt mismatch: got=%s want=%s", result.AttemptID, attempt.AttemptID)
	}
	if result.OrganizationID != "" && !strings.EqualFold(result.OrganizationID, attempt.OrganizationID.String()) {
		return fmt.Errorf("settlement organization mismatch: got=%s want=%s", result.OrganizationID, attempt.OrganizationID)
	}
	if result.DeductionID != "" && result.DeductionID != result.SettlementID {
		return fmt.Errorf("settlement deduction mismatch: settlement_id=%s deduction_id=%s", result.SettlementID, result.DeductionID)
	}
	if result.SettledCredits < 0 || result.RefundedCredits < 0 {
		return fmt.Errorf("negative settlement amounts")
	}
	if result.RequestID != "" && result.RequestID != attempt.RequestID {
		return fmt.Errorf("settlement request mismatch: got=%s want=%s", result.RequestID, attempt.RequestID)
	}
	if result.ActualQuantity < 0 || result.EstimatedQuantity < 0 {
		return fmt.Errorf("negative settlement quantity")
	}
	if len(result.PricingSnapshot) > 0 && !json.Valid(result.PricingSnapshot) {
		return fmt.Errorf("invalid settlement pricing snapshot")
	}
	projection, err := toolProjectionFromQuery(result)
	if err != nil {
		return err
	}
	if len(projection) > 0 && string(projection) != "null" && strings.EqualFold(result.SettlementStatus, billingAttemptStatusRolledBack) {
		return s.mirrorReleasedToolHold(ctx, attempt, result, projection)
	}
	if len(projection) > 0 && string(projection) != "null" {
		if !strings.EqualFold(result.SettlementStatus, billingAttemptStatusSettled) {
			return fmt.Errorf("authoritative tool settlement is still pending")
		}
		var projected toolSettlement
		if json.Unmarshal(projection, &projected) != nil || projected.TotalCredits != result.SettledCredits {
			return fmt.Errorf("authoritative settlement amount mismatch")
		}
		bc := &BillingContext{OrganizationID: attempt.OrganizationID.String(), AttemptID: attempt.AttemptID, RequestID: attempt.RequestID, DeductionID: result.SettlementID, BillingLane: UsageBillingLanePlatform}
		if attempt.ModelID != nil {
			bc.ModelID = *attempt.ModelID
		}
		if attempt.ProviderID != nil {
			bc.ProviderID = *attempt.ProviderID
		}
		applyPlatformSettlementTokenUsage(bc, settlementFromQueryResult(result))
		s.localService.enrichBillingContextCatalogFields(ctx, bc)
		return s.mirrorPlatformTools(ctx, bc, projection, datatypes.JSON(result.PricingSnapshot))
	}
	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current BillingAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("attempt_id = ?", attempt.AttemptID).First(&current).Error; err != nil {
			return err
		}
		if current.ConsoleSettlementID != nil && strings.TrimSpace(*current.ConsoleSettlementID) != "" && strings.TrimSpace(*current.ConsoleSettlementID) != strings.TrimSpace(result.SettlementID) {
			return fmt.Errorf("settlement_id mismatch: stored=%s received=%s", *current.ConsoleSettlementID, result.SettlementID)
		}
		var fund BillingAttemptEntry
		if err := tx.Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", current.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).First(&fund).Error; err != nil {
			return err
		}
		if !consoleSettlementAmountsValid(fund.ReservedAmount.Int64(), result.SettledCredits, result.RefundedCredits) {
			return fmt.Errorf("settlement amounts exceed reserved credits: reserved=%d settled=%d refunded=%d", fund.ReservedAmount, result.SettledCredits, result.RefundedCredits)
		}
		bc := &BillingContext{
			OrganizationID:    current.OrganizationID.String(),
			RequestID:         current.RequestID,
			AttemptID:         current.AttemptID,
			QuotaSubjectType:  current.QuotaSubjectType,
			QuotaSubjectID:    current.QuotaSubjectID,
			DeductionID:       strings.TrimSpace(firstNonEmptyString(result.SettlementID, derefString(fund.IdempotencyKey))),
			EstimatedCredits:  fund.ReservedAmount.Int64(),
			ActualCredits:     result.SettledCredits,
			Status:            "success",
			BillingLane:       UsageBillingLanePlatform,
			UseSystemProvider: true,
			InvocationSource:  current.InvocationSource,
			RequestCreatedAt:  current.CreatedAt,
			SettledAt:         time.Now().UTC(),
		}
		if len(result.PricingSnapshot) > 0 && string(result.PricingSnapshot) != "null" {
			bc.PricingSnapshot = datatypes.JSON(result.PricingSnapshot)
		}
		restoreBillingContextAttribution(bc, &current)
		if current.RouteID != nil {
			bc.RouteID = current.RouteID
			bc.ChannelID = current.RouteID
		}
		if current.ModelID != nil {
			bc.ModelID = *current.ModelID
		}
		if current.ProviderID != nil {
			bc.ProviderID = *current.ProviderID
		}
		if err := s.restorePartialUsageBillContext(ctx, bc, tx); err != nil {
			return err
		}
		if bc.ToolPostpaid {
			return fmt.Errorf("authoritative tool settlement JSON is still pending")
		}

		s.localService.enrichBillingContextCatalogFields(ctx, bc)
		settlement := settlementFromQueryResult(result)
		applyPlatformSettlementTokenUsage(bc, settlement)
		if err := applyPlatformSettlementCostSnapshot(bc, settlement); err != nil {
			return fmt.Errorf("apply platform settlement currency snapshot: %w", err)
		}
		recordSettlementOverageSnapshot(bc, fund.ReservedAmount.Int64(), result.SettledCredits)
		s.localService.applyRouteAttributionWithLookup(ctx, bc)
		finalizeMediaTimingSnapshot(bc)
		if err := s.localService.settleSubjectQuota(ctx, tx, bc); err != nil {
			return err
		}
		if err := s.localService.updateAttemptEntriesAfterSettle(ctx, tx, bc, billingLedgerTypeOrgFunds, current.OrganizationID.String()); err != nil {
			return err
		}
		if err := tx.Model(&BillingAttempt{}).Where("attempt_id = ?", current.AttemptID).Updates(map[string]interface{}{"status": billingAttemptStatusSettled, "settlement_state": billingAttemptStatusSettled, "console_settlement_id": result.SettlementID, "console_settlement_status": result.SettlementStatus, "next_reconcile_at": nil, "settlement_next_retry_at": nil, "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if hasUsageBillProjectionIdentity(bc) {
			if err := s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusSuccess, nil, nil); err != nil {
				return fmt.Errorf("upsert reconciled usage bill: %w", err)
			}
		}
		return nil
	})
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *RemoteBilling) persistRemoteCompensationPending(
	ctx context.Context,
	bc *BillingContext,
	bindErr error,
	compensationErr error,
) error {
	deductionID := strings.TrimSpace(bc.DeductionID)
	if deductionID == "" {
		return fmt.Errorf("missing deduction_id for compensation persistence")
	}
	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ? AND lane = ?", bc.AttemptID, billingAttemptLaneRemote).
			First(&attempt).Error; err != nil {
			return err
		}
		if attempt.Status == billingAttemptStatusCompensated {
			return nil
		}
		if attempt.Status != billingAttemptStatusInit &&
			attempt.Status != billingAttemptStatusPredeductFailed &&
			attempt.Status != billingAttemptStatusCompensationPending {
			return fmt.Errorf("attempt cannot queue remote compensation from status %s", attempt.Status)
		}

		var subjectEntry BillingAttemptEntry
		if err := tx.WithContext(ctx).
			Where("attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeSubject).
			First(&subjectEntry).Error; err != nil {
			return fmt.Errorf("load compensation subject entry: %w", err)
		}
		var fundEntry BillingAttemptEntry
		if err := tx.WithContext(ctx).
			Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", bc.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
			First(&fundEntry).Error; err != nil {
			return fmt.Errorf("load compensation fund entry: %w", err)
		}
		if fundEntry.IdempotencyKey != nil {
			boundID := strings.TrimSpace(*fundEntry.IdempotencyKey)
			if boundID != "" && boundID != deductionID {
				return fmt.Errorf("remote deduction id conflict: stored=%s received=%s", boundID, deductionID)
			}
		}

		// INIT still owns a local subject reservation. A stale-INIT recovery may
		// already have closed it, so only refund entries that remain pending.
		if attempt.Status == billingAttemptStatusInit && subjectEntry.Status == billingEntryStatusPending {
			rollbackCtx := *bc
			rollbackCtx.ActualCredits = 0
			rollbackCtx.Status = "error"
			if err := s.localService.settleSubjectQuota(ctx, tx, &rollbackCtx); err != nil {
				return fmt.Errorf("rollback compensation subject quota: %w", err)
			}
		}

		now := time.Now()
		code := "REMOTE_COMPENSATION_PENDING"
		message := fmt.Sprintf("binding failed: %v; remote compensation failed: %v", bindErr, compensationErr)
		if err := tx.WithContext(ctx).Model(&BillingAttemptEntry{}).
			Where("id = ?", subjectEntry.ID).
			Updates(map[string]any{
				"actual_amount": 0, "refunded_amount": subjectEntry.ReservedAmount,
				"status": billingEntryStatusRolled, "error_code": code, "error_message": message, "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("close compensation subject entry: %w", err)
		}
		if err := tx.WithContext(ctx).Model(&BillingAttemptEntry{}).
			Where("id = ?", fundEntry.ID).
			Updates(map[string]any{
				"idempotency_key": deductionID, "actual_amount": 0, "refunded_amount": 0,
				"status": billingEntryStatusPending, "error_code": code, "error_message": message, "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("persist compensation deduction id: %w", err)
		}
		invocation := "error"
		if err := s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusCompensationPending, &invocation, &code, &message); err != nil {
			return err
		}
		return tx.WithContext(ctx).Model(&BillingAttempt{}).
			Where("attempt_id = ?", bc.AttemptID).
			Updates(map[string]any{
				"next_reconcile_at": now,
				"updated_at":        now,
			}).Error
	})
}

func (s *RemoteBilling) reconcilePendingRemoteCompensations(ctx context.Context) error {
	now := time.Now()
	var attempts []BillingAttempt
	if err := s.localService.db.WithContext(ctx).
		Where("status = ? AND lane = ? AND reconcile_attempts < ? AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)",
			billingAttemptStatusCompensationPending, billingAttemptLaneRemote, defaultReconcileMaxRetries, now).
		Order("COALESCE(next_reconcile_at, updated_at) ASC").
		Limit(defaultReconcileBatchSize).
		Find(&attempts).Error; err != nil {
		return fmt.Errorf("query pending remote compensations: %w", err)
	}
	for _, attempt := range attempts {
		claimed, err := s.claimRemoteCompensationForReconcile(ctx, attempt.AttemptID, now)
		if err != nil {
			logger.ErrorContext(ctx, "remote compensation reconcile claim failed", err, zap.String("attempt_id", attempt.AttemptID))
			continue
		}
		if !claimed {
			continue
		}
		if err := s.reconcileRemoteCompensationAttempt(ctx, attempt.AttemptID); err != nil {
			logger.ErrorContext(ctx, "remote compensation reconcile attempt failed", err, zap.String("attempt_id", attempt.AttemptID))
			if scheduleErr := s.scheduleRemoteCompensationFailure(ctx, attempt.AttemptID, err); scheduleErr != nil {
				logger.ErrorContext(ctx, "remote compensation retry scheduling failed", scheduleErr, zap.String("attempt_id", attempt.AttemptID))
			}
		}
	}
	return nil
}

func (s *RemoteBilling) claimRemoteCompensationForReconcile(ctx context.Context, attemptID string, now time.Time) (bool, error) {
	res := s.localService.db.WithContext(ctx).Model(&BillingAttempt{}).
		Where("attempt_id = ? AND status = ? AND lane = ? AND reconcile_attempts < ? AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)",
			attemptID, billingAttemptStatusCompensationPending, billingAttemptLaneRemote, defaultReconcileMaxRetries, now).
		Updates(map[string]any{
			"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
			"last_reconcile_at":  now,
			"next_reconcile_at":  now.Add(defaultSettlePendingTimeout),
			"updated_at":         now,
		})
	return res.RowsAffected == 1, res.Error
}

func (s *RemoteBilling) reconcileRemoteCompensationAttempt(ctx context.Context, attemptID string) error {
	var attempt BillingAttempt
	if err := s.localService.db.WithContext(ctx).Where("attempt_id = ?", attemptID).First(&attempt).Error; err != nil {
		return fmt.Errorf("load compensation attempt: %w", err)
	}
	var fundEntry BillingAttemptEntry
	if err := s.localService.db.WithContext(ctx).
		Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", attemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
		First(&fundEntry).Error; err != nil {
		return fmt.Errorf("load compensation fund entry: %w", err)
	}
	if fundEntry.IdempotencyKey == nil || strings.TrimSpace(*fundEntry.IdempotencyKey) == "" {
		return errReconcileMissingDeductionID
	}
	bc := &BillingContext{
		OrganizationID: attempt.OrganizationID.String(), AttemptID: attempt.AttemptID, RequestID: attempt.RequestID,
		DeductionID: strings.TrimSpace(*fundEntry.IdempotencyKey), EstimatedCredits: fundEntry.ReservedAmount.Int64(),
		BillingLane: UsageBillingLanePlatform, UseSystemProvider: true, Status: "error",
	}
	restoreBillingContextAttribution(bc, &attempt)
	if attempt.ModelID != nil {
		bc.ModelID = *attempt.ModelID
	}
	if attempt.ProviderID != nil {
		bc.ProviderID = *attempt.ProviderID
	}
	if err := s.compensateRemoteReservationAfterBindFailure(ctx, bc); err != nil {
		return err
	}
	return s.finalizeRemoteCompensation(ctx, attemptID)
}

func (s *RemoteBilling) finalizeRemoteCompensation(ctx context.Context, attemptID string) error {
	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ?", attemptID).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.Status == billingAttemptStatusCompensated {
			return nil
		}
		if attempt.Status != billingAttemptStatusCompensationPending {
			return fmt.Errorf("attempt cannot finalize remote compensation from status %s", attempt.Status)
		}
		now := time.Now()
		if err := tx.WithContext(ctx).Model(&BillingAttemptEntry{}).
			Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", attemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
			Updates(map[string]any{
				"actual_amount": 0, "refunded_amount": gorm.Expr("reserved_amount"),
				"status": billingEntryStatusRolled, "error_code": nil, "error_message": nil, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		invocation := "error"
		bc := &BillingContext{AttemptID: attemptID}
		if err := s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusCompensated, &invocation, nil, nil); err != nil {
			return err
		}
		return tx.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ?", attemptID).
			Updates(map[string]any{"next_reconcile_at": nil, "updated_at": now}).Error
	})
}

func (s *RemoteBilling) scheduleRemoteCompensationFailure(ctx context.Context, attemptID string, reconcileErr error) error {
	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ?", attemptID).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.Status != billingAttemptStatusCompensationPending {
			return nil
		}
		now := time.Now()
		if attempt.ReconcileAttempts >= defaultReconcileMaxRetries {
			return tx.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ?", attemptID).
				Updates(map[string]any{
					"status": billingAttemptStatusDeadLetter, "error_code": "REMOTE_COMPENSATION_FAILED",
					"error_message":     fmt.Sprintf("max retries reached(%d): %v", attempt.ReconcileAttempts, reconcileErr),
					"next_reconcile_at": nil, "updated_at": now,
				}).Error
		}
		return tx.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ?", attemptID).
			Updates(map[string]any{
				"error_code": "REMOTE_COMPENSATION_FAILED", "error_message": reconcileErr.Error(),
				"next_reconcile_at": now.Add(reconcileBackoff(attempt.ReconcileAttempts)), "updated_at": now,
			}).Error
	})
}

func (s *RemoteBilling) recoverStaleRemoteInitAttempts(ctx context.Context) error {
	cutoff := time.Now().Add(-defaultRemoteInitTimeout)
	var attempts []BillingAttempt
	if err := s.localService.db.WithContext(ctx).
		Where("status = ? AND lane = ? AND updated_at < ?", billingAttemptStatusInit, billingAttemptLaneRemote, cutoff).
		Order("updated_at ASC").
		Limit(defaultReconcileBatchSize).
		Find(&attempts).Error; err != nil {
		return fmt.Errorf("query stale remote init attempts: %w", err)
	}
	for _, attempt := range attempts {
		if err := s.recoverStaleRemoteInitAttempt(ctx, attempt.AttemptID, cutoff); err != nil {
			logger.ErrorContext(ctx, "remote billing stale init attempt recovery failed", err, zap.String("attempt_id", attempt.AttemptID))
		}
	}
	return nil
}

func (s *RemoteBilling) recoverStaleRemoteInitAttempt(ctx context.Context, attemptID string, cutoff time.Time) error {
	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("attempt_id = ? AND status = ? AND lane = ? AND updated_at < ?", attemptID, billingAttemptStatusInit, billingAttemptLaneRemote, cutoff).
			First(&attempt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		var fundEntry BillingAttemptEntry
		if err := tx.WithContext(ctx).
			Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", attemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
			First(&fundEntry).Error; err != nil {
			return fmt.Errorf("load stale remote init fund entry: %w", err)
		}
		if fundEntry.IdempotencyKey != nil && strings.TrimSpace(*fundEntry.IdempotencyKey) != "" {
			return nil
		}
		var subjectEntry BillingAttemptEntry
		if err := tx.WithContext(ctx).
			Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).
			First(&subjectEntry).Error; err != nil {
			return fmt.Errorf("load stale remote init subject entry: %w", err)
		}

		bc := &BillingContext{
			OrganizationID: attempt.OrganizationID.String(), AttemptID: attempt.AttemptID, RequestID: attempt.RequestID,
			InvocationSource: normalizeInvocationSource(attempt.InvocationSource), QuotaSubjectType: attempt.QuotaSubjectType,
			QuotaSubjectID: attempt.QuotaSubjectID, EstimatedCredits: fundEntry.ReservedAmount.Int64(),
			SubjectReservedCredits: subjectEntry.ReservedAmount.Int64(), ActualCredits: 0,
			BillingLane: UsageBillingLanePlatform, UseSystemProvider: true, Status: "error",
			ErrorMessage: "stale remote pre-deduct without a bound deduction id was rolled back",
		}
		restoreBillingContextAttribution(bc, &attempt)
		if attempt.QuotaSubjectType == quotaSubjectTypeAPIKey {
			bc.APIKeyID = attempt.QuotaSubjectID
		}
		if attempt.QuotaSubjectType == quotaSubjectTypeAccessGrant {
			bc.AccessGrantID = attempt.QuotaSubjectID
		}
		if attempt.QuotaSubjectType == quotaSubjectTypeWorkspace {
			bc.WorkspaceID = attempt.QuotaSubjectID
		}
		if err := s.localService.settleSubjectQuota(ctx, tx, bc); err != nil {
			return fmt.Errorf("rollback stale remote init subject quota: %w", err)
		}
		now := time.Now()
		code := "REMOTE_PREDEDUCT_TIMEOUT_NO_DEDUCTION_ID"
		message := fmt.Sprintf("stale remote pre-deduct timed out after %s without a bound deduction id", defaultRemoteInitTimeout)
		if err := tx.WithContext(ctx).Model(&BillingAttemptEntry{}).
			Where("id = ?", subjectEntry.ID).
			Updates(map[string]any{
				"actual_amount": 0, "refunded_amount": subjectEntry.ReservedAmount,
				"status": billingEntryStatusRolled, "error_code": code, "error_message": message, "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("close stale remote init subject entry: %w", err)
		}
		invocation := "error"
		return s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusPredeductFailed, &invocation, &code, &message)
	})
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

func (s *RemoteBilling) recoverStalePredeductedRemoteAttempts(ctx context.Context) error {
	cutoff := time.Now().Add(-defaultRemotePredeductTimeout)
	var attempts []BillingAttempt
	if err := s.localService.db.WithContext(ctx).
		Where("status = ? AND lane = ? AND updated_at < ?", billingAttemptStatusPre, billingAttemptLaneRemote, cutoff).
		Order("updated_at ASC").
		Limit(defaultReconcileBatchSize).
		Find(&attempts).Error; err != nil {
		return fmt.Errorf("query stale remote prededucted attempts: %w", err)
	}
	for _, attempt := range attempts {
		if err := s.recoverStalePredeductedRemoteAttempt(ctx, attempt.AttemptID, cutoff); err != nil {
			logger.ErrorContext(ctx, "remote billing stale prededuct attempt recovery failed", err, zap.String("attempt_id", attempt.AttemptID))
		}
	}
	return nil
}

func (s *RemoteBilling) recoverStalePredeductedRemoteAttempt(ctx context.Context, attemptID string, cutoff time.Time) error {
	var attempt BillingAttempt
	if err := s.localService.db.WithContext(ctx).
		Where("attempt_id = ? AND status = ? AND lane = ? AND updated_at < ?", attemptID, billingAttemptStatusPre, billingAttemptLaneRemote, cutoff).
		First(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	var fundEntry BillingAttemptEntry
	if err := s.localService.db.WithContext(ctx).
		Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", attemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
		First(&fundEntry).Error; err != nil {
		return fmt.Errorf("load stale prededuct fund entry: %w", err)
	}
	if fundEntry.IdempotencyKey == nil || strings.TrimSpace(*fundEntry.IdempotencyKey) == "" {
		return nil
	}

	var subjectEntry BillingAttemptEntry
	if err := s.localService.db.WithContext(ctx).
		Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).
		First(&subjectEntry).Error; err != nil {
		return fmt.Errorf("load stale prededuct subject entry: %w", err)
	}

	bc := &BillingContext{
		OrganizationID:         attempt.OrganizationID.String(),
		DeductionID:            strings.TrimSpace(*fundEntry.IdempotencyKey),
		AttemptID:              attempt.AttemptID,
		RequestID:              attempt.RequestID,
		InvocationSource:       normalizeInvocationSource(attempt.InvocationSource),
		EstimatedCredits:       fundEntry.ReservedAmount.Int64(),
		SubjectReservedCredits: subjectEntry.ReservedAmount.Int64(),
		ActualCredits:          0,
		QuotaSubjectType:       attempt.QuotaSubjectType,
		QuotaSubjectID:         attempt.QuotaSubjectID,
		BillingLane:            UsageBillingLanePlatform,
		UseSystemProvider:      true,
		Status:                 "error",
		ErrorMessage:           fmt.Sprintf("stale remote prededuct timed out after %s without provider settlement", defaultRemotePredeductTimeout),
	}
	restoreBillingContextAttribution(bc, &attempt)
	if attempt.QuotaSubjectType == quotaSubjectTypeAPIKey {
		bc.APIKeyID = attempt.QuotaSubjectID
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeAccessGrant {
		bc.AccessGrantID = attempt.QuotaSubjectID
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeWorkspace {
		bc.WorkspaceID = attempt.QuotaSubjectID
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
	// The attempt ledger keeps only catalog ids; restore the names so the
	// outcome is mirrored into a usage bill like any other settlement.
	s.localService.enrichBillingContextCatalogFields(ctx, bc)

	// Console owns the hold. An async call (a video whose client never polls)
	// is settled there, or is still running, long after this timeout, so
	// Console's state decides the outcome: rolling back blindly marks a charged
	// call ROLLED_BACK without a bill, and settling it at zero releases a hold
	// whose task is still running.
	if s.consoleAPIBaseURL != "" {
		result, err := queryConsoleSettlement(ctx, s.consoleAPIBaseURL+"/billing/settlements/by-attempt", attempt.OrganizationID.String(), attempt.AttemptID)
		if err != nil {
			return fmt.Errorf("query console settlement for stale prededuct: %w", err)
		}
		switch {
		case result.Found && strings.EqualFold(result.SettlementStatus, billingAttemptStatusSettled):
			return s.mirrorQueriedSettlement(ctx, &attempt, result)
		case result.Found && result.Retryable:
			// Still open on Console: its task settlement or hold expiry resolves
			// it, and a later sweep mirrors whichever outcome it reaches.
			return nil
		case result.Found && strings.EqualFold(result.SettlementStatus, billingAttemptStatusRolledBack):
			// Console already released the hold (for example it expired), so a
			// settle call would be refused; mirror the release locally.
			bc.ErrorMessage = "console released the remote prededuct before provider settlement"
			return s.finalizeRemoteSettlement(ctx, bc)
		}
	}
	return s.settleViaGRPC(ctx, bc)
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

	// Organization funds and member quota are separate ledgers. The member
	// charge may be capped (or zero for a stale grant period), but remote
	// reconciliation must always use the provider cost recorded on the fund
	// entry, including a legitimate zero-cost failed invocation.
	actualCredits := fundEntry.ActualAmount

	settleStatus := "success"
	if attempt.InvocationResult != nil && strings.EqualFold(*attempt.InvocationResult, "error") {
		settleStatus = "error"
	}

	bc := &BillingContext{
		OrganizationID:   attempt.OrganizationID.String(),
		DeductionID:      strings.TrimSpace(*fundEntry.IdempotencyKey),
		AttemptID:        attempt.AttemptID,
		RequestID:        attempt.RequestID,
		InvocationSource: normalizeInvocationSource(attempt.InvocationSource),
		EstimatedCredits: fundEntry.ReservedAmount.Int64(),
		SubjectReservedCredits: func() int64 {
			if subjectEntry != nil {
				return subjectEntry.ReservedAmount.Int64()
			}
			return fundEntry.ReservedAmount.Int64()
		}(),
		ActualCredits:     actualCredits.Int64(),
		QuotaSubjectType:  attempt.QuotaSubjectType,
		QuotaSubjectID:    attempt.QuotaSubjectID,
		BillingLane:       UsageBillingLanePlatform,
		UseSystemProvider: true,
		Status:            settleStatus,
	}
	restoreBillingContextAttribution(bc, &attempt)
	if attempt.QuotaSubjectType == quotaSubjectTypeAPIKey && strings.TrimSpace(attempt.QuotaSubjectID) != "" {
		bc.APIKeyID = strings.TrimSpace(attempt.QuotaSubjectID)
	}
	if attempt.QuotaSubjectType == quotaSubjectTypeAccessGrant && strings.TrimSpace(attempt.QuotaSubjectID) != "" {
		if strings.TrimSpace(bc.AccessGrantID) == "" {
			bc.AccessGrantID = strings.TrimSpace(attempt.QuotaSubjectID)
		}
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
	if err := s.restorePartialUsageBillContext(ctx, bc); err != nil {
		return err
	}

	return s.settleViaGRPC(ctx, bc)
}

// restorePartialUsageBillContext reloads request/model/usage metadata that is
// intentionally not duplicated in billing_attempts. A failed remote settle has
// already persisted that metadata in the partial usage bill, so reconciliation
// can use it to replace the partial audit record with the final result.
func (s *RemoteBilling) restorePartialUsageBillContext(ctx context.Context, bc *BillingContext, databases ...*gorm.DB) error {
	db := s.localService.db
	if len(databases) > 0 {
		db = databases[0]
	}
	var bill UsageBill
	err := db.WithContext(ctx).Where("attempt_id = ?", bc.AttemptID).First(&bill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load partial usage bill: %w", err)
	}

	bc.RequestID = bill.RequestID
	bc.OrganizationID = bill.OrganizationID
	bc.AppID, bc.AppType = bill.AppID, bill.AppType
	bc.InvocationSource = bill.InvocationSource
	if bill.WorkspaceID != nil {
		bc.WorkspaceID = *bill.WorkspaceID
	}
	bc.AccountID = bill.AccountID
	if bill.PrincipalType != nil {
		bc.PrincipalType = *bill.PrincipalType
	}
	if bill.PrincipalID != nil {
		bc.PrincipalID = *bill.PrincipalID
	}
	if bill.AccessGrantID != nil {
		bc.AccessGrantID = bill.AccessGrantID.String()
	}
	bc.AuthMethod = bill.AuthMethod
	bc.APIKeyID = bill.APIKeyID
	if bill.QuotaSubjectType != nil {
		bc.QuotaSubjectType = *bill.QuotaSubjectType
	}
	if bill.QuotaSubjectID != nil {
		bc.QuotaSubjectID = *bill.QuotaSubjectID
	}
	bc.ModelID, bc.ModelName = bill.ModelID, bill.ModelName
	bc.ProviderID, bc.ProviderName = bill.ProviderID, bill.ProviderName
	bc.RouteID, bc.ChannelID = bill.RouteID, bill.ChannelID
	bc.BillingLane, bc.UseSystemProvider = bill.BillingLane, bill.UseSystemProvider
	if bill.RemoteDeductionID != nil {
		bc.DeductionID = *bill.RemoteDeductionID
	}
	bc.PromptTokens = int(bill.PromptTokens)
	bc.CacheReadTokens = int(bill.CacheReadTokens)
	bc.CacheWriteTokens = int(bill.CacheWriteTokens)
	bc.CompletionTokens = int(bill.CompletionTokens)
	bc.TotalTokens = int(bill.TotalTokens)
	bc.PricingSource, bc.UsageSource = bill.PricingSource, bill.UsageSource
	bc.PricingSnapshot = bill.PricingSnapshot
	bc.ToolPostpaid = bc.ToolPostpaid || isToolPostpaidSnapshot(bill.PricingSnapshot)
	bc.ResponseTime = bill.ResponseTimeMS
	bc.RequestCreatedAt = bill.RequestCreatedAt
	return nil
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

// preDeductMeteredViaGRPC reserves a metered price on the platform billing
// authority. It mirrors preDeductViaGRPC: local subject pre-deduct first,
// remote metered reservation second, then the shared binding tail with the
// same compensation and rollback behavior.
func (s *RemoteBilling) preDeductMeteredViaGRPC(ctx context.Context, bc *BillingContext, fact shared.MeteredUsageFact) error {
	// The caller already reserved the local subject quota (preDeductViaGRPC does
	// that before branching), so this function owns only the remote reservation
	// and must not reserve again.
	req := &PreDeductMeteredUsageRequest{
		OrganizationID: bc.OrganizationID,
		ModelID:        bc.ModelID.String(),
		ProviderName:   bc.ProviderName,
		ModelName:      bc.ModelName,
		RequestID:      bc.RequestID,
		AttemptID:      bc.AttemptID,
		Usage: MeteredUsageFact{
			Operation:  fact.Operation,
			Meter:      fact.Meter,
			BaseUnit:   fact.BaseUnit,
			Quantity:   fact.Quantity,
			Dimensions: fact.Dimensions,
		},
	}

	resp, err := s.grpcClient.PreDeductMeteredUsage(ctx, req)
	if err != nil {
		if rollbackErr := s.rollbackLocalSubjectQuotaAndMarkFailed(ctx, bc, "PREDEDUCT_FAILED", err.Error()); rollbackErr != nil {
			return fmt.Errorf("grpc metered pre-deduct failed: %v (local subject rollback failed: %w)", err, rollbackErr)
		}
		if mapped, ok := mapBillingRPCErrorCode(billingRPCErrorReason(err), ""); ok {
			return mapped
		}
		return fmt.Errorf("grpc metered pre-deduct failed: %w", err)
	}
	if !resp.Success {
		if rollbackErr := s.rollbackLocalSubjectQuotaAndMarkFailed(ctx, bc, "PREDEDUCT_FAILED", resp.ErrorMessage); rollbackErr != nil {
			return fmt.Errorf("metered pre-deduct failed: %s (local subject rollback failed: %w)", resp.ErrorMessage, rollbackErr)
		}
		return mapMeteredPreDeductError(resp.ErrorCode, resp.ErrorMessage)
	}
	return s.finalizeRemotePreDeductBinding(ctx, bc, resp.DeductionID)
}

// mapMeteredPreDeductError maps a metered pre-deduct failure to a Pre sentinel,
// preferring the stable error_code over the free-text message.
func mapMeteredPreDeductError(errorCode, message string) error {
	if mapped, ok := mapBillingRPCErrorCode(errorCode, message); ok {
		return mapped
	}
	return fmt.Errorf("pre-deduct failed: %s", message)
}

// mapBillingRPCErrorCode maps a Console billing failure to a Pre sentinel. It
// prefers the stable error_code / ErrorInfo.Reason and keeps a temporary
// substring compatibility branch for Console builds that only populate the
// free-text message. T21 removes the compatibility branch once every Console
// failure carries a stable code.
func mapBillingRPCErrorCode(errorCode, errorMessage string) (error, bool) {
	switch strings.TrimSpace(errorCode) {
	case "billing.organization.balance_insufficient",
		"billing.private_channel.balance_insufficient",
		"llm.balance.insufficient",
		"INSUFFICIENT_CREDITS":
		return ErrInsufficientBalance, true
	case "billing.workspace.quota_insufficient",
		"llm.quota.exceeded":
		return ErrInsufficientQuota, true
	case "billing.model.pricing_not_configured":
		return ErrPricingNotConfigured, true
	case "billing.prededuct.failed":
		return ErrBillingPreDeductFailed, true
	case "billing.deduction.not_found", "ORG_NOT_FOUND":
		return ErrBalanceNotFound, true
	case "billing.request.invalid", "INVALID_REQUEST":
		return ErrInvalidRequest, true
	}

	// Temporary compatibility branch (T21 removal). Older Console builds only
	// populated error_message, so the sentinels are detected by substring.
	lowered := strings.ToLower(strings.TrimSpace(errorMessage))
	switch {
	case strings.Contains(lowered, "insufficient"):
		return ErrInsufficientBalance, true
	case strings.Contains(lowered, "organization") && strings.Contains(lowered, "not found"):
		return ErrBalanceNotFound, true
	case strings.Contains(lowered, "invalid"):
		return ErrInvalidRequest, true
	}
	return nil, false
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

		var preDeductErr error
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
			if !apiKey.IsActive() {
				return ErrAPIKeyInactive
			}
			preDeductErr = s.localService.preDeductSubjectQuota(ctx, tx, bc, &apiKey)

		case quotaSubjectTypeAccessGrant:
			if strings.TrimSpace(bc.QuotaSubjectID) == "" || strings.TrimSpace(bc.AccessGrantID) != strings.TrimSpace(bc.QuotaSubjectID) {
				return fmt.Errorf("missing or mismatched developer grant subject")
			}
			preDeductErr = s.localService.preDeductSubjectQuota(ctx, tx, bc, nil)
			if preDeductErr == nil {
				apiKeyID := strings.TrimSpace(bc.APIKeyID)
				if bc.AuthMethod != "personal_api_key" || apiKeyID == "" {
					return fmt.Errorf("developer grant subject requires a personal api key")
				}
				// Keep the same Grant -> Key lock order as local billing and
				// developer-access mutations. A lifecycle change committed after
				// authentication must stop this request before the remote reservation.
				var apiKey apikeymodel.TenantAPIKey
				if err := tx.WithContext(ctx).
					Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("id = ? AND organization_id = ?", apiKeyID, bc.OrganizationID).
					First(&apiKey).Error; err != nil {
					return fmt.Errorf("load personal api key for developer grant pre-deduct: %w", err)
				}
				if !apiKey.IsActive() || apiKey.AccessGrantID == nil || strings.TrimSpace(*apiKey.AccessGrantID) != strings.TrimSpace(bc.AccessGrantID) ||
					bc.GrantAuthorizationVersion == nil || apiKey.AuthorizationVersion != *bc.GrantAuthorizationVersion {
					return ErrAPIKeyInactive
				}
			}

		case quotaSubjectTypeWorkspace:
			if strings.TrimSpace(bc.QuotaSubjectID) == "" {
				return fmt.Errorf("missing workspace_id for subject pre-deduct")
			}
			preDeductErr = s.localService.preDeductSubjectQuota(ctx, tx, bc, nil)
		case quotaSubjectTypeOrganization:
			if strings.TrimSpace(bc.QuotaSubjectID) == "" {
				return fmt.Errorf("missing organization_id for subject pre-deduct")
			}
			preDeductErr = s.localService.preDeductSubjectQuota(ctx, tx, bc, nil)

		default:
			return fmt.Errorf("unsupported quota subject type: %s", subjectType)
		}
		if preDeductErr != nil {
			return preDeductErr
		}
		return s.localService.upsertAttemptInit(ctx, tx, bc)
	})
}

func (s *RemoteBilling) rollbackLocalSubjectQuotaAndMarkFailed(ctx context.Context, bc *BillingContext, code, msg string) error {
	rollbackCtx := *bc
	rollbackCtx.ActualCredits = 0
	rollbackCtx.Status = "error"
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		terminal, err := s.localService.lockAttemptForFinalization(ctx, tx, rollbackCtx.AttemptID)
		if err != nil {
			return err
		}
		if terminal {
			return nil
		}
		if err := s.localService.settleSubjectQuota(ctx, tx, &rollbackCtx); err != nil {
			return err
		}
		now := time.Now()
		if err := tx.WithContext(ctx).Model(&BillingAttemptEntry{}).
			Where("attempt_id = ? AND entry_type = ?", rollbackCtx.AttemptID, billingEntryTypeSubject).
			Updates(map[string]any{
				"actual_amount": 0, "refunded_amount": millipoints.MilliPoints(subjectReservedCredits(&rollbackCtx)),
				"status": billingEntryStatusRolled, "error_code": code, "error_message": msg, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		invocation := "error"
		return s.localService.updateAttemptStatus(ctx, tx, &rollbackCtx, billingAttemptStatusPredeductFailed, &invocation, &code, &msg)
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
		terminal, err := s.localService.lockAttemptForFinalization(ctx, tx, bc.AttemptID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if terminal {
			return nil
		}
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
		terminal, err := s.localService.lockAttemptForFinalization(ctx, tx, bc.AttemptID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if terminal {
			return nil
		}
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		if bc.RequestCreatedAt.IsZero() {
			bc.RequestCreatedAt = time.Now().UTC()
		}
		bc.SettledAt = time.Now().UTC()
		invocation := invocationResultFromBillingStatus(bc.Status)
		if err := s.localService.updateAttemptStatus(
			ctx,
			tx,
			bc,
			billingAttemptStatusPartial,
			&invocation,
			&code,
			&msg,
		); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"settlement_state": billingAttemptStatusFailedRetryable, "settlement_last_error_code": code, "settlement_last_error_message": msg, "settlement_retry_count": gorm.Expr("settlement_retry_count + 1"), "settlement_version": gorm.Expr("settlement_version + 1"), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusPartial, &code, &msg)
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
	return billingAttemptStatusIsFinalized(attempt.Status), nil
}

func (s *RemoteBilling) Close() error {
	if s.grpcClient != nil {
		return s.grpcClient.Close()
	}
	return nil
}
