package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type privateChannelBalanceChecker interface {
	CheckPrivateChannelBalance(ctx context.Context, organizationID uuid.UUID, channelID uuid.UUID, estimatedCredits int64) (bool, error)
}

const (
	missingTokenUsageMessage   = "provider returned no token usage"
	billingFinalizationTimeout = 15 * time.Second
)

func billingFinalizationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), billingFinalizationTimeout)
}

func hasBillableTokenUsage(usage *adapter.Usage) bool {
	if usage == nil {
		return false
	}
	return usage.PromptTokens > 0 || usage.CompletionTokens > 0
}

func missingTokenUsageError(providerName, modelName string) error {
	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)
	switch {
	case providerName != "" && modelName != "":
		return fmt.Errorf("%s for provider %q model %q", missingTokenUsageMessage, providerName, modelName)
	case providerName != "":
		return fmt.Errorf("%s for provider %q", missingTokenUsageMessage, providerName)
	case modelName != "":
		return fmt.Errorf("%s for model %q", missingTokenUsageMessage, modelName)
	default:
		return errors.New(missingTokenUsageMessage)
	}
}

func resolveQuotaSubjectType(appCtx *AppContext) string {
	if appCtx == nil {
		return quotaSubjectTypeAPIKey
	}
	if appCtx.BillingSubjectType != nil {
		subjectType := strings.TrimSpace(*appCtx.BillingSubjectType)
		switch subjectType {
		case quotaSubjectTypeAPIKey, quotaSubjectTypeWorkspace, quotaSubjectTypeOrganization:
			return subjectType
		}
	}
	if appCtx.WorkspaceID != nil && strings.TrimSpace(*appCtx.WorkspaceID) != "" {
		return quotaSubjectTypeWorkspace
	}
	return quotaSubjectTypeAPIKey
}

// checkModelAuthorization checks whether current principal can use model.
// Workspace-scoped app calls are authorized by workspace policy, not API key model limits.
func (s *llmGatewayServiceImpl) checkModelAuthorization(apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, model string) error {
	if resolveQuotaSubjectType(appCtx) != quotaSubjectTypeAPIKey {
		return nil
	}
	if apiKey == nil {
		return ErrInvalidAPIKey
	}
	if !apiKey.ModelLimitsEnabled || apiKey.ModelLimits == nil || *apiKey.ModelLimits == "" {
		return nil
	}

	var allowedModels []string
	if err := json.Unmarshal([]byte(*apiKey.ModelLimits), &allowedModels); err != nil {
		return fmt.Errorf("invalid model limits configuration: %w", err)
	}

	for _, m := range allowedModels {
		if m == model {
			return nil
		}
	}

	return ErrModelNotAuthorized
}

// createBillingContext creates a billing context for a request
func (s *llmGatewayServiceImpl) createBillingContext(
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	shadowOrganizationID uuid.UUID,
	estimatedCredits int64,
	isStreaming bool,
	requestCreatedAt time.Time,
	requestID string,
	attemptID string,
) *BillingContext {
	if requestCreatedAt.IsZero() {
		requestCreatedAt = time.Now().UTC()
	} else {
		requestCreatedAt = requestCreatedAt.UTC()
	}

	var routeID *uuid.UUID
	if providerSelection.HasRoute() {
		routeID = &providerSelection.RouteID
	}
	billingLane, err := normalizeUsageBillingLane(providerSelection.BillingLane, providerSelection.UseSystemProvider)
	if err != nil {
		billingLane = usageBillingLaneFromSystemProvider(providerSelection.UseSystemProvider)
	}

	billingCtx := &BillingContext{
		APIKeyID:          apiKey.ID,
		OrganizationID:    shadowOrganizationID.String(),
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    apiKey.ID,
		ModelID:           providerSelection.Model.ID,
		ModelSource:       providerSelection.ModelSource,
		ModelName:         providerSelection.Model.Model,
		ProviderID:        providerSelection.Provider.ID,
		ProviderName:      providerSelection.Provider.Provider,
		RouteID:           routeID,
		ChannelID:         channelID,
		EstimatedCredits:  estimatedCredits,
		BillingLane:       billingLane,
		UseSystemProvider: usageBillingLaneUsesSystemProvider(billingLane),
		IsStreaming:       isStreaming,
		RequestID:         requestID,
		RequestCreatedAt:  requestCreatedAt,
		AttemptID:         attemptID,
	}

	if appCtx != nil {
		billingCtx.AppID = appCtx.AppID
		billingCtx.AppType = appCtx.AppType
		billingCtx.AccountID = appCtx.AccountID
		billingCtx.SessionID = appCtx.SessionID
		billingCtx.ConversationID = appCtx.ConversationID
		billingCtx.WorkflowID = appCtx.WorkflowID
		billingCtx.WorkflowRunID = appCtx.WorkflowRunID
		billingCtx.NodeID = appCtx.NodeID
		billingCtx.NodeType = appCtx.NodeType
		switch resolveQuotaSubjectType(appCtx) {
		case quotaSubjectTypeWorkspace:
			if appCtx.WorkspaceID != nil && *appCtx.WorkspaceID != "" {
				billingCtx.WorkspaceID = *appCtx.WorkspaceID
				billingCtx.QuotaSubjectType = quotaSubjectTypeWorkspace
				billingCtx.QuotaSubjectID = *appCtx.WorkspaceID
			}
		case quotaSubjectTypeOrganization:
			billingCtx.QuotaSubjectType = quotaSubjectTypeOrganization
			billingCtx.QuotaSubjectID = shadowOrganizationID.String()
		}
	}

	return billingCtx
}

// beginBillingAttempt centralizes the billing start phase:
// create context -> check balance (official only) -> pre-deduct on selected billing provider.
func (s *llmGatewayServiceImpl) beginBillingAttempt(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	providerSelection *ProviderSelection,
	shadowOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	estimatedCredits int64,
	isStreaming bool,
	requestCreatedAt time.Time,
	requestID string,
	attemptID string,
) (*BillingContext, error) {
	channelID := getChannelID(providerSelection)

	billingCtx := s.createBillingContext(
		apiKey,
		appCtx,
		providerSelection,
		channelID,
		shadowOrganizationID,
		estimatedCredits,
		isStreaming,
		requestCreatedAt,
		requestID,
		attemptID,
	)

	decision, err := s.resolveBillingDecision(providerSelection, billingCtx)
	if err != nil {
		return nil, wrapBillingLaneMismatchError(err)
	}

	logBillingEvent(
		billingCode("BILLING_LANE_SELECTED", decision.UseSystemProvider),
		billingCtx,
		decision.RouteID,
		decision.UseSystemProvider,
		"prededuct",
		string(decision.Lane),
		nil,
	)

	if err := s.checkBalanceAndPreDeduct(
		ctx,
		providerSelection,
		shadowOrganizationID,
		ownerID,
		estimatedCredits,
		billingCtx,
	); err != nil {
		return nil, err
	}

	return billingCtx, nil
}

// checkBalanceAndPreDeduct checks balance and pre-deducts credits
func (s *llmGatewayServiceImpl) checkBalanceAndPreDeduct(
	ctx context.Context,
	providerSelection *ProviderSelection,
	shadowOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	estimatedCredits int64,
	billingCtx *BillingContext,
) error {
	decision, laneErr := s.resolveBillingDecision(providerSelection, billingCtx)
	if laneErr != nil {
		wrappedLaneErr := wrapBillingLaneMismatchError(laneErr)

		logBillingEvent(
			billingCode("BILLING_LANE_MISMATCH", decision.UseSystemProvider),
			billingCtx,
			decision.RouteID,
			decision.UseSystemProvider,
			"prededuct",
			"error",
			wrappedLaneErr,
		)

		return wrappedLaneErr
	}

	useSystemProvider := decision.UseSystemProvider
	routeID := decision.RouteID

	billingProvider := s.billingProviderForDecision(decision)

	// Check balance if using system provider
	if useSystemProvider {
		canSpend, err := s.billing.CheckBalance(ctx, shadowOrganizationID, ownerID, estimatedCredits)
		if err != nil {
			wrappedErr := wrapBillingPreDeductError(
				fmt.Errorf("check credit balance: %w", err),
				billingCtx,
				providerSelection,
			)

			logBillingEvent(
				billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"prededuct",
				"error",
				wrappedErr,
			)

			return wrappedErr
		}

		if !canSpend {
			logBillingEvent(
				billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"prededuct",
				"error",
				ErrInsufficientBalance,
			)

			return organizationBalanceInsufficientError()
		}
	} else {
		if billingCtx.ChannelID == nil {
			err := wrapBillingPreDeductError(
				fmt.Errorf("missing channel_id for private channel balance check"),
				billingCtx,
				providerSelection,
			)
			logBillingEvent(
				billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"prededuct",
				"error",
				err,
			)
			return err
		}

		checker, ok := s.localBilling.(privateChannelBalanceChecker)
		if !ok {
			err := wrapBillingPreDeductError(
				fmt.Errorf("local billing does not support private channel balance check"),
				billingCtx,
				providerSelection,
			)
			logBillingEvent(
				billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"prededuct",
				"error",
				err,
			)
			return err
		}

		canSpend, err := checker.CheckPrivateChannelBalance(ctx, shadowOrganizationID, *billingCtx.ChannelID, estimatedCredits)
		if err != nil {
			wrappedErr := wrapBillingPreDeductError(
				fmt.Errorf("check private channel balance: %w", err),
				billingCtx,
				providerSelection,
			)
			logBillingEvent(
				billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"prededuct",
				"error",
				wrappedErr,
			)
			return wrappedErr
		}
		if !canSpend {
			logBillingEvent(
				billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"prededuct",
				"error",
				ErrInsufficientBalance,
			)
			return privateChannelBalanceInsufficientError()
		}
	}

	// Pre-deduct quota
	if err := billingProvider.PreDeduct(ctx, billingCtx); err != nil {
		wrappedErr := wrapBillingPreDeductError(
			fmt.Errorf("pre-deduct quota: %w", err),
			billingCtx,
			providerSelection,
		)

		logBillingEvent(
			billingCode("BILLING_PREDEDUCT_FAILED", useSystemProvider),
			billingCtx,
			routeID,
			useSystemProvider,
			"prededuct",
			"error",
			wrappedErr,
		)

		return wrappedErr
	}

	logBillingEvent(
		billingCode("BILLING_PREDEDUCT_OK", useSystemProvider),
		billingCtx,
		routeID,
		useSystemProvider,
		"prededuct",
		"ok",
		nil,
	)

	return nil
}

// handleProviderError handles provider call errors
func (s *llmGatewayServiceImpl) handleProviderError(
	ctx context.Context,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	responseTime int64,
	attemptIdx int,
	err error,
) error {
	billingCtx.Status = "error"
	billingCtx.ErrorMessage = err.Error()
	billingCtx.ResponseTime = responseTime
	billingCtx.ActualCredits = 0
	billingCtx.PromptTokens = 0
	billingCtx.CompletionTokens = 0
	billingCtx.TotalTokens = 0
	billingCtx.TotalCost = decimal.Zero
	decision, laneErr := s.resolveBillingDecision(providerSelection, billingCtx)
	if laneErr != nil {
		return wrapBillingLaneMismatchError(laneErr)
	}
	useSystemProvider := decision.UseSystemProvider
	routeID := decision.RouteID
	billingCtxForSettle, cancel := billingFinalizationContext(ctx)
	defer cancel()
	if err := s.billingProviderForDecision(decision).Settle(billingCtxForSettle, billingCtx); err != nil {
		wrappedErr := wrapBillingSettleError(err, billingCtx, useSystemProvider, routeID)
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", useSystemProvider),
			billingCtx,
			routeID,
			useSystemProvider,
			"settle",
			"error",
			wrappedErr,
		)
		return wrappedErr
	}
	logBillingEvent(
		billingCode("BILLING_SETTLE_OK", useSystemProvider),
		billingCtx,
		routeID,
		useSystemProvider,
		"settle",
		"ok",
		nil,
	)

	s.logProviderError(ctx, attemptIdx, providerSelection, err, "provider_call_failed")

	if channelID != nil {
		autoBan := providerSelection.HasRoute() && providerSelection.AutoBan
		if trackErr := s.healthTracker.RecordFailure(ctx, *channelID, autoBan); trackErr != nil {
			logger.WarnContext(ctx, "failed to record channel failure",
				"provider", providerSelection.Provider.Provider,
				"route_id", providerSelection.RouteID.String(),
				"auto_ban", autoBan,
				trackErr,
			)
		}
	}
	return nil
}

// settleChatSuccess settles billing for a successful chat/response completion
func (s *llmGatewayServiceImpl) settleChatSuccess(
	ctx context.Context,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	usage *adapter.Usage,
	settlement *adapter.SettlementResult,
	responseTime int64,
) error {
	decision, laneErr := s.resolveBillingDecision(providerSelection, billingCtx)
	if laneErr != nil {
		wrappedLaneErr := wrapBillingLaneMismatchError(laneErr)
		logBillingEvent(
			billingCode("BILLING_LANE_MISMATCH", decision.UseSystemProvider),
			billingCtx,
			decision.RouteID,
			decision.UseSystemProvider,
			"settle",
			"error",
			wrappedLaneErr,
		)
		return wrappedLaneErr
	}

	if decision.UseSystemProvider {
		if hasBillableTokenUsage(usage) {
			billingCtx.PromptTokens = usage.PromptTokens
			billingCtx.CompletionTokens = usage.CompletionTokens
			billingCtx.TotalTokens = usage.TotalTokens
		} else {
			clearBillingContextTokenUsage(billingCtx)
		}
		billingCtx.Status = "success"
		billingCtx.ResponseTime = responseTime
		billingCtxForSettle, cancel := billingFinalizationContext(ctx)
		defer cancel()
		if err := s.finalizePlatformProxySettlement(billingCtxForSettle, billingCtx, settlement, decision); err != nil {
			if channelID != nil {
				s.healthTracker.RecordSuccess(*channelID)
			}
			return err
		}
		if channelID != nil {
			s.healthTracker.RecordSuccess(*channelID)
		}
		return nil
	}

	if !hasBillableTokenUsage(usage) {
		err := missingTokenUsageError(providerSelection.Provider.Provider, providerSelection.Model.Model)
		if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, 0, err); settleErr != nil {
			return settleErr
		}
		return err
	}

	actualPromptTokens := usage.PromptTokens
	actualCompletionTokens := usage.CompletionTokens

	quote, err := s.quoteTokenPricing(ctx, pricingModelRefFromSelection(providerSelection), actualPromptTokens, actualCompletionTokens)
	if err != nil {
		return fmt.Errorf("failed to calculate credits: %w", err)
	}

	billingCtx.ActualCredits = quote.TotalCredits

	billingCtx.PromptTokens = actualPromptTokens
	billingCtx.CompletionTokens = actualCompletionTokens
	billingCtx.TotalTokens = usage.TotalTokens

	applyPricingQuoteToBillingContext(billingCtx, quote)

	billingCtx.Status = "success"
	billingCtx.ResponseTime = responseTime

	useSystemProvider := decision.UseSystemProvider
	routeID := decision.RouteID
	billingCtxForSettle, cancel := billingFinalizationContext(ctx)
	defer cancel()
	if err := s.billingProviderForDecision(decision).Settle(billingCtxForSettle, billingCtx); err != nil {
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", useSystemProvider),
			billingCtx,
			routeID,
			useSystemProvider,
			"settle",
			"error",
			err,
		)
		if channelID != nil {
			s.healthTracker.RecordSuccess(*channelID)
		}
		return wrapBillingSettleError(err, billingCtx, useSystemProvider, routeID)
	}
	logBillingEvent(
		billingCode("BILLING_SETTLE_OK", useSystemProvider),
		billingCtx,
		routeID,
		useSystemProvider,
		"settle",
		"ok",
		nil,
	)

	if channelID != nil {
		s.healthTracker.RecordSuccess(*channelID)
	}
	return nil
}

func clearBillingContextTokenUsage(billingCtx *BillingContext) {
	if billingCtx == nil {
		return
	}
	billingCtx.PromptTokens = 0
	billingCtx.CompletionTokens = 0
	billingCtx.TotalTokens = 0
}

// settleEmbeddingsSuccess settles billing for a successful embeddings/rerank completion
func (s *llmGatewayServiceImpl) settleEmbeddingsSuccess(
	ctx context.Context,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	actualTokens int,
	settlement *adapter.SettlementResult,
	responseTime int64,
) error {
	decision, laneErr := s.resolveBillingDecision(providerSelection, billingCtx)
	if laneErr != nil {
		wrappedLaneErr := wrapBillingLaneMismatchError(laneErr)
		logBillingEvent(
			billingCode("BILLING_LANE_MISMATCH", decision.UseSystemProvider),
			billingCtx,
			decision.RouteID,
			decision.UseSystemProvider,
			"settle",
			"error",
			wrappedLaneErr,
		)
		return wrappedLaneErr
	}

	if decision.UseSystemProvider {
		billingCtx.PromptTokens = actualTokens
		billingCtx.CompletionTokens = 0
		billingCtx.TotalTokens = actualTokens
		billingCtx.Status = "success"
		billingCtx.ResponseTime = responseTime
		if err := s.finalizePlatformProxySettlement(ctx, billingCtx, settlement, decision); err != nil {
			if channelID != nil {
				s.healthTracker.RecordSuccess(*channelID)
			}
			return err
		}
		if channelID != nil {
			s.healthTracker.RecordSuccess(*channelID)
		}
		return nil
	}

	if actualTokens <= 0 {
		providerName := ""
		modelName := ""
		if providerSelection != nil {
			providerName = providerSelection.Provider.Provider
			modelName = providerSelection.Model.Model
		}
		err := missingTokenUsageError(providerName, modelName)
		if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, 0, err); settleErr != nil {
			return settleErr
		}
		return err
	}

	quote, err := s.quoteTokenPricing(ctx, pricingModelRefFromSelection(providerSelection), actualTokens, 0)
	if err != nil {
		return fmt.Errorf("failed to calculate credits: %w", err)
	}

	billingCtx.ActualCredits = quote.TotalCredits
	billingCtx.PromptTokens = actualTokens
	billingCtx.CompletionTokens = 0
	billingCtx.TotalTokens = actualTokens
	applyPricingQuoteToBillingContext(billingCtx, quote)
	billingCtx.Status = "success"
	billingCtx.ResponseTime = responseTime

	useSystemProvider := decision.UseSystemProvider
	routeID := decision.RouteID
	if err := s.billingProviderForDecision(decision).Settle(ctx, billingCtx); err != nil {
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", useSystemProvider),
			billingCtx,
			routeID,
			useSystemProvider,
			"settle",
			"error",
			err,
		)
		if channelID != nil {
			s.healthTracker.RecordSuccess(*channelID)
		}
		return wrapBillingSettleError(err, billingCtx, useSystemProvider, routeID)
	}
	logBillingEvent(
		billingCode("BILLING_SETTLE_OK", useSystemProvider),
		billingCtx,
		routeID,
		useSystemProvider,
		"settle",
		"ok",
		nil,
	)

	if channelID != nil {
		s.healthTracker.RecordSuccess(*channelID)
	}
	return nil
}

type platformProxySettlementFinalizer interface {
	FinalizePlatformProxySettlement(context.Context, *BillingContext, *adapter.SettlementResult) error
}

func (s *llmGatewayServiceImpl) finalizePlatformProxySettlement(
	ctx context.Context,
	billingCtx *BillingContext,
	settlement *adapter.SettlementResult,
	decision BillingDecision,
) error {
	finalizer, ok := s.billingProviderForDecision(decision).(platformProxySettlementFinalizer)
	if !ok {
		err := fmt.Errorf("platform proxy settlement finalizer is not configured")
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", true),
			billingCtx,
			decision.RouteID,
			true,
			"settle",
			"error",
			err,
		)
		return wrapBillingSettleError(err, billingCtx, true, decision.RouteID)
	}
	if err := finalizer.FinalizePlatformProxySettlement(ctx, billingCtx, settlement); err != nil {
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", true),
			billingCtx,
			decision.RouteID,
			true,
			"settle",
			"error",
			err,
		)
		return wrapBillingSettleError(err, billingCtx, true, decision.RouteID)
	}
	logBillingEvent(
		billingCode("BILLING_SETTLE_OK", true),
		billingCtx,
		decision.RouteID,
		true,
		"settle",
		"ok",
		nil,
	)
	return nil
}

// getChannelID extracts channel ID from provider selection
func getChannelID(providerSelection *ProviderSelection) *uuid.UUID {
	if providerSelection.HasRoute() {
		return &providerSelection.RouteID
	}
	return nil
}

func buildAttemptID(requestID string, attemptIdx int) string {
	return fmt.Sprintf("%s-a%d", requestID, attemptIdx+1)
}
