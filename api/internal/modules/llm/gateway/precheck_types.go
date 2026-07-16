package gateway

import channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"

type AppModelRoutePrecheckStatus string

type AppModelRouteWarningKind string

const (
	AppModelRoutePrecheckStatusOK      AppModelRoutePrecheckStatus = "ok"
	AppModelRoutePrecheckStatusWarning AppModelRoutePrecheckStatus = "warning"
	AppModelRoutePrecheckStatusUnknown AppModelRoutePrecheckStatus = "unknown"
)

const (
	AppModelRouteWarningKindOrganizationBalanceLow            AppModelRouteWarningKind = "organization_balance_low"
	AppModelRouteWarningKindWorkspaceQuotaLow                 AppModelRouteWarningKind = "workspace_quota_low"
	AppModelRouteWarningKindPrivateChannelBalanceLow          AppModelRouteWarningKind = "private_channel_balance_low"
	AppModelRouteWarningKindPrivateChannelUpstreamBalanceLow  AppModelRouteWarningKind = "private_channel_upstream_balance_low"
	AppModelRouteWarningKindPrivateChannelUpstreamUnavailable AppModelRouteWarningKind = "private_channel_upstream_unavailable"
)

const (
	workflowOrganizationLowBalanceThreshold   = int64(500000)
	workflowWorkspaceLowQuotaThreshold        = int64(100000)
	workflowPrivateChannelLowBalanceThreshold = int64(500000)
)

type AppModelRouteWarning struct {
	Kind         AppModelRouteWarningKind
	CurrentValue int64
	Threshold    int64
	Reason       string
}

type AppModelRoutePrecheckResult struct {
	Status   AppModelRoutePrecheckStatus
	Warnings []AppModelRouteWarning
}

type AppModelRouteRef struct {
	Provider string
	Model    string
}

type candidateRouteWarningState struct {
	Route   *channelmodel.LLMRoute
	Healthy bool
	Warning *AppModelRouteWarning
}
