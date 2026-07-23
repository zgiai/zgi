package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	paymentservice "github.com/zgiai/zgi/api/internal/modules/payment/service"
	"gorm.io/gorm"
)

func wrapBillingPredeductErrorForUser(err error, billingCtx *BillingContext, useSystemProvider bool) error {
	if err == nil {
		return nil
	}

	if billingCtx != nil && billingCtx.QuotaSubjectType == quotaSubjectTypeWorkspace && errors.Is(err, ErrInsufficientQuota) {
		return workspaceQuotaInsufficientError(err)
	}

	if errors.Is(err, ErrInsufficientBalance) {
		if useSystemProvider {
			return organizationBalanceInsufficientError()
		}
		return privateChannelBalanceInsufficientError()
	}

	return err
}

func (s *llmGatewayServiceImpl) PrecheckAppModels(ctx context.Context, organizationID string, appCtx *AppContext, models []AppModelRouteRef) (*AppModelRoutePrecheckResult, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return nil, fmt.Errorf("organization_id is required")
	}

	orgUUID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization_id: %w", err)
	}

	shadowOrganizationID, _, err := s.getShadowTenantInfo(ctx, orgUUID)
	if err != nil {
		return nil, err
	}

	if resolveQuotaSubjectType(appCtx) == quotaSubjectTypeWorkspace {
		workspaceID := ""
		if appCtx != nil && appCtx.WorkspaceID != nil {
			workspaceID = strings.TrimSpace(*appCtx.WorkspaceID)
		}
		if workspaceID != "" {
			workspaceWarning, err := s.buildWorkspaceQuotaWarning(ctx, shadowOrganizationID, workspaceID)
			if err != nil {
				return &AppModelRoutePrecheckResult{Status: AppModelRoutePrecheckStatusUnknown}, nil
			}
			if workspaceWarning != nil {
				return &AppModelRoutePrecheckResult{
					Status:   AppModelRoutePrecheckStatusWarning,
					Warnings: []AppModelRouteWarning{*workspaceWarning},
				}, nil
			}
		}
	}

	dedupedModels := dedupeAppModelRefsPreserveOrder(models)
	if len(dedupedModels) == 0 {
		return &AppModelRoutePrecheckResult{Status: AppModelRoutePrecheckStatusOK}, nil
	}

	aggregatedWarnings := make([]AppModelRouteWarning, 0)
	seenWarnings := make(map[AppModelRouteWarningKind]struct{})
	for _, model := range dedupedModels {
		modelWarnings, err := s.precheckSingleModelRoutes(ctx, shadowOrganizationID, model)
		if err != nil {
			return &AppModelRoutePrecheckResult{Status: AppModelRoutePrecheckStatusUnknown}, nil
		}
		for _, warning := range modelWarnings {
			if _, exists := seenWarnings[warning.Kind]; exists {
				continue
			}
			seenWarnings[warning.Kind] = struct{}{}
			aggregatedWarnings = append(aggregatedWarnings, warning)
		}
	}

	if len(aggregatedWarnings) == 0 {
		return &AppModelRoutePrecheckResult{Status: AppModelRoutePrecheckStatusOK}, nil
	}

	return &AppModelRoutePrecheckResult{
		Status:   AppModelRoutePrecheckStatusWarning,
		Warnings: aggregatedWarnings,
	}, nil
}

func (s *llmGatewayServiceImpl) precheckSingleModelRoutes(ctx context.Context, shadowOrganizationID uuid.UUID, model AppModelRouteRef) ([]AppModelRouteWarning, error) {
	routes, err := s.loadCandidateRoutesForModel(ctx, shadowOrganizationID, model.Provider, model.Model, 3)
	if err != nil {
		if errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable) {
			return []AppModelRouteWarning{{
				Kind:   AppModelRouteWarningKindPrivateChannelUpstreamUnavailable,
				Reason: privateChannelCredentialUnavailableReason,
				Scope:  AppModelRouteWarningScopeAll,
			}}, nil
		}
		return nil, err
	}

	healthy, warnings, err := s.evaluateCandidateRouteWarnings(ctx, shadowOrganizationID, routes)
	if err != nil {
		return nil, err
	}
	if healthy {
		return nil, nil
	}
	return warnings, nil
}

func (s *llmGatewayServiceImpl) evaluateCandidateRouteWarnings(ctx context.Context, organizationID uuid.UUID, routes []*channelmodel.LLMRoute) (bool, []AppModelRouteWarning, error) {
	if len(routes) == 0 {
		return false, nil, fmt.Errorf("no candidate routes")
	}

	states := make([]candidateRouteWarningState, 0, len(routes))
	var orgBalance *int64
	checker := s.officialCreditChecker
	if checker == nil {
		checker = paymentservice.NewConsoleOfficialCreditChecker()
	}

	for _, route := range routes {
		if route == nil {
			continue
		}

		if isOfficialRoute(route) {
			if orgBalance == nil {
				balance, err := checker.GetOfficialBalance(ctx, organizationID)
				if err != nil {
					return false, nil, err
				}
				orgBalance = &balance
			}
			if *orgBalance >= workflowOrganizationLowBalanceThreshold {
				states = append(states, candidateRouteWarningState{Route: route, Healthy: true})
				continue
			}
			states = append(states, candidateRouteWarningState{
				Route: route,
				Warning: &AppModelRouteWarning{
					Kind:         AppModelRouteWarningKindOrganizationBalanceLow,
					CurrentValue: *orgBalance,
					Threshold:    workflowOrganizationLowBalanceThreshold,
				},
			})
			continue
		}

		walletBalance, err := s.loadPrivateChannelWalletBalance(ctx, organizationID, route.ID)
		if err != nil {
			return false, nil, err
		}
		if walletBalance >= workflowPrivateChannelLowBalanceThreshold {
			states = append(states, candidateRouteWarningState{Route: route, Healthy: true})
			continue
		}
		states = append(states, candidateRouteWarningState{
			Route: route,
			Warning: &AppModelRouteWarning{
				Kind:         AppModelRouteWarningKindPrivateChannelBalanceLow,
				CurrentValue: walletBalance,
				Threshold:    workflowPrivateChannelLowBalanceThreshold,
			},
		})
	}

	if len(states) == 0 {
		return false, nil, fmt.Errorf("no candidate routes")
	}

	warnings := summarizeCandidateRouteWarnings(states)
	upstreamUnavailable, err := s.candidateUpstreamUnavailableWarning(ctx, organizationID, routes)
	if err != nil {
		return false, nil, err
	}
	if upstreamUnavailable != nil {
		warnings = append(warnings, *upstreamUnavailable)
	}
	upstreamLow, err := s.allCandidateUpstreamBalancesLow(ctx, organizationID, routes)
	if err != nil {
		return false, nil, err
	}
	if upstreamLow {
		warnings = append(warnings, AppModelRouteWarning{
			Kind: AppModelRouteWarningKindPrivateChannelUpstreamBalanceLow,
		})
	}
	if len(warnings) == 0 {
		return true, nil, nil
	}
	return false, warnings, nil
}

func (s *llmGatewayServiceImpl) candidateUpstreamUnavailableWarning(
	ctx context.Context,
	organizationID uuid.UUID,
	routes []*channelmodel.LLMRoute,
) (*AppModelRouteWarning, error) {
	if s.upstreamState == nil || len(routes) == 0 {
		return nil, nil
	}

	credentialIDs := make([]uuid.UUID, 0, len(routes))
	seenCredentialIDs := make(map[uuid.UUID]struct{}, len(routes))
	candidateCount := 0
	for _, route := range routes {
		if route == nil {
			continue
		}
		candidateCount++
		if isOfficialRoute(route) || route.CredentialID == nil {
			continue
		}
		if _, exists := seenCredentialIDs[*route.CredentialID]; exists {
			continue
		}
		seenCredentialIDs[*route.CredentialID] = struct{}{}
		credentialIDs = append(credentialIDs, *route.CredentialID)
	}
	if candidateCount == 0 || len(credentialIDs) == 0 {
		return nil, nil
	}

	states, err := s.upstreamState.GetMany(ctx, organizationID, credentialIDs)
	if err != nil {
		return nil, err
	}

	affectedCount := 0
	reason := upstreamstate.GuardReason("")
	mixedReasons := false
	for _, route := range routes {
		if route == nil || isOfficialRoute(route) || route.CredentialID == nil {
			continue
		}
		state := states[*route.CredentialID]
		if state == nil || state.BlockReason == "" {
			continue
		}
		affectedCount++
		if reason == "" {
			reason = state.BlockReason
			continue
		}
		if reason != state.BlockReason {
			mixedReasons = true
		}
	}
	if affectedCount == 0 {
		return nil, nil
	}

	warningReason := string(reason)
	if mixedReasons {
		warningReason = privateChannelCredentialUnavailableReason
	}
	scope := AppModelRouteWarningScopePartial
	if affectedCount == candidateCount {
		scope = AppModelRouteWarningScopeAll
	}
	return &AppModelRouteWarning{
		Kind:   AppModelRouteWarningKindPrivateChannelUpstreamUnavailable,
		Reason: warningReason,
		Scope:  scope,
	}, nil
}

func (s *llmGatewayServiceImpl) allCandidateUpstreamBalancesLow(
	ctx context.Context,
	organizationID uuid.UUID,
	routes []*channelmodel.LLMRoute,
) (bool, error) {
	if s.upstreamState == nil || len(routes) == 0 {
		return false, nil
	}
	credentialIDs := make([]uuid.UUID, 0, len(routes))
	for _, route := range routes {
		if route == nil || isOfficialRoute(route) || route.CredentialID == nil {
			return false, nil
		}
		credentialIDs = append(credentialIDs, *route.CredentialID)
	}
	states, err := s.upstreamState.GetMany(ctx, organizationID, credentialIDs)
	if err != nil {
		return false, err
	}
	now := time.Now()
	for _, credentialID := range credentialIDs {
		state := states[credentialID]
		if state == nil || upstreamstate.IsStale(state, now) || !upstreamstate.IsLow(state) {
			return false, nil
		}
	}
	return len(credentialIDs) > 0, nil
}

func summarizeCandidateRouteWarnings(states []candidateRouteWarningState) []AppModelRouteWarning {
	if len(states) == 0 {
		return nil
	}

	for _, state := range states {
		if state.Healthy {
			return nil
		}
	}

	warnings := make([]AppModelRouteWarning, 0, len(states))
	seen := make(map[AppModelRouteWarningKind]struct{}, len(states))
	for _, state := range states {
		if state.Warning == nil {
			continue
		}
		if _, exists := seen[state.Warning.Kind]; exists {
			continue
		}
		seen[state.Warning.Kind] = struct{}{}
		warnings = append(warnings, *state.Warning)
	}
	return warnings
}

func (s *llmGatewayServiceImpl) buildWorkspaceQuotaWarning(ctx context.Context, organizationID uuid.UUID, workspaceID string) (*AppModelRouteWarning, error) {
	quota, err := s.loadWorkspaceQuota(ctx, organizationID, workspaceID)
	if err != nil {
		return nil, err
	}
	if quota == nil || quota.QuotaLimit == nil {
		return nil, nil
	}
	if quota.RemainQuota >= workflowWorkspaceLowQuotaThreshold {
		return nil, nil
	}
	return &AppModelRouteWarning{
		Kind:         AppModelRouteWarningKindWorkspaceQuotaLow,
		CurrentValue: quota.RemainQuota,
		Threshold:    workflowWorkspaceLowQuotaThreshold,
	}, nil
}

func (s *llmGatewayServiceImpl) loadCandidateRoutesForModel(ctx context.Context, organizationID uuid.UUID, provider, modelName string, maxSelections int) ([]*channelmodel.LLMRoute, error) {
	if s.channelRouter == nil {
		return nil, fmt.Errorf("channel router is not configured")
	}
	return s.channelRouter.CandidateRoutesForProviderModel(ctx, organizationID, provider, modelName, maxSelections)
}

func (s *llmGatewayServiceImpl) loadWorkspaceQuota(ctx context.Context, organizationID uuid.UUID, workspaceID string) (*WorkspaceQuota, error) {
	var quota WorkspaceQuota
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND organization_id = ?", workspaceID, organizationID).
		First(&quota).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &WorkspaceQuota{WorkspaceID: workspaceID, OrganizationID: organizationID, QuotaLimit: nil}, nil
		}
		return nil, err
	}
	return &quota, nil
}

func (s *llmGatewayServiceImpl) loadPrivateChannelWalletBalance(ctx context.Context, organizationID, channelID uuid.UUID) (int64, error) {
	var wallet ChannelWallet
	err := s.db.WithContext(ctx).
		Where("channel_id = ? AND organization_id = ? AND status = ?", channelID, organizationID, channelWalletStatusActive).
		First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return wallet.Balance, nil
}

func dedupeAppModelRefsPreserveOrder(values []AppModelRouteRef) []AppModelRouteRef {
	result := make([]AppModelRouteRef, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		provider := strings.ToLower(strings.TrimSpace(value.Provider))
		model := strings.TrimSpace(value.Model)
		if model == "" {
			continue
		}
		key := provider + "\x00" + model
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, AppModelRouteRef{Provider: provider, Model: model})
	}
	return result
}
