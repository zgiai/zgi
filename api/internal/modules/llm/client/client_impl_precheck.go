package client

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

type gatewayAppModelPrechecker interface {
	PrecheckAppModels(ctx context.Context, organizationID string, appCtx *gateway.AppContext, models []string) (*gateway.AppModelRoutePrecheckResult, error)
}

func (c *llmClientImpl) PrecheckAppModels(ctx context.Context, appCtx *AppContext, models []string) (*AppModelPrecheckResult, error) {
	if appCtx == nil {
		return &AppModelPrecheckResult{Status: AppModelPrecheckStatusUnknown}, nil
	}
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}
	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	prechecker, ok := c.gateway.(gatewayAppModelPrechecker)
	if !ok {
		return nil, fmt.Errorf("gateway does not support app model precheck")
	}

	result, err := prechecker.PrecheckAppModels(ctx, organizationID, gwAppCtx, models)
	if err != nil {
		return nil, err
	}
	return convertGatewayPrecheckResult(result), nil
}

func convertGatewayPrecheckResult(result *gateway.AppModelRoutePrecheckResult) *AppModelPrecheckResult {
	if result == nil {
		return &AppModelPrecheckResult{Status: AppModelPrecheckStatusUnknown}
	}

	warnings := make([]AppModelPrecheckWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		kind := AppModelPrecheckWarningKind("")
		switch warning.Kind {
		case gateway.AppModelRouteWarningKindOrganizationBalanceLow:
			kind = AppModelPrecheckWarningOrganizationBalanceLow
		case gateway.AppModelRouteWarningKindWorkspaceQuotaLow:
			kind = AppModelPrecheckWarningWorkspaceQuotaLow
		case gateway.AppModelRouteWarningKindPrivateChannelBalanceLow:
			kind = AppModelPrecheckWarningPrivateChannelBalanceLow
		case gateway.AppModelRouteWarningKindPrivateChannelUpstreamBalanceLow:
			kind = AppModelPrecheckWarningPrivateChannelUpstreamBalanceLow
		case gateway.AppModelRouteWarningKindPrivateChannelUpstreamUnavailable:
			kind = AppModelPrecheckWarningPrivateChannelUpstreamUnavailable
		default:
			continue
		}
		warnings = append(warnings, AppModelPrecheckWarning{
			Kind:         kind,
			CurrentValue: warning.CurrentValue,
			Threshold:    warning.Threshold,
			Reason:       warning.Reason,
		})
	}

	status := AppModelPrecheckStatusUnknown
	switch result.Status {
	case gateway.AppModelRoutePrecheckStatusOK:
		status = AppModelPrecheckStatusOK
	case gateway.AppModelRoutePrecheckStatusWarning:
		status = AppModelPrecheckStatusWarning
	case gateway.AppModelRoutePrecheckStatusUnknown:
		status = AppModelPrecheckStatusUnknown
	}

	return &AppModelPrecheckResult{Status: status, Warnings: warnings}
}
