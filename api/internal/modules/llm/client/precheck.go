package client

import "context"

type AppModelPrecheckStatus string

type AppModelPrecheckWarningKind string

const (
	AppModelPrecheckStatusOK      AppModelPrecheckStatus = "ok"
	AppModelPrecheckStatusWarning AppModelPrecheckStatus = "warning"
	AppModelPrecheckStatusUnknown AppModelPrecheckStatus = "unknown"
)

const (
	AppModelPrecheckWarningOrganizationBalanceLow            AppModelPrecheckWarningKind = "organization_balance_low"
	AppModelPrecheckWarningWorkspaceQuotaLow                 AppModelPrecheckWarningKind = "workspace_quota_low"
	AppModelPrecheckWarningPrivateChannelBalanceLow          AppModelPrecheckWarningKind = "private_channel_balance_low"
	AppModelPrecheckWarningPrivateChannelUpstreamBalanceLow  AppModelPrecheckWarningKind = "private_channel_upstream_balance_low"
	AppModelPrecheckWarningPrivateChannelUpstreamUnavailable AppModelPrecheckWarningKind = "private_channel_upstream_unavailable"
)

type AppModelPrecheckWarning struct {
	Kind         AppModelPrecheckWarningKind
	CurrentValue int64
	Threshold    int64
	Reason       string
}

type AppModelPrecheckResult struct {
	Status   AppModelPrecheckStatus
	Warnings []AppModelPrecheckWarning
}

type AppModelPrechecker interface {
	PrecheckAppModels(ctx context.Context, appCtx *AppContext, models []string) (*AppModelPrecheckResult, error)
}
