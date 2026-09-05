package gateway

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

const (
	headerZGIBillingOrganizationID = "X-ZGI-Billing-Organization-ID"
	headerZGIRequestID             = "X-ZGI-Request-ID"
	headerZGIAttemptID             = "X-ZGI-Attempt-ID"
	headerZGIDeductionID           = "X-ZGI-Deduction-ID"
	headerZGIAPIKeyID              = "X-ZGI-API-Key-ID"
	headerZGIWorkspaceID           = "X-ZGI-Workspace-ID"
	headerZGIAppID                 = "X-ZGI-App-ID"
	headerZGIAppType               = "X-ZGI-App-Type"
	headerZGIModelName             = "X-ZGI-Model-Name"
	headerZGIProviderName          = "X-ZGI-Provider-Name"
	headerZGIIsStreaming           = "X-ZGI-Is-Streaming"
	headerZGIRoutingStrategy       = "X-ZGI-Routing-Strategy"
	headerZGIEstimatedInputTokens  = "X-ZGI-Estimated-Input-Tokens"
	headerZGIEstimatedOutputTokens = "X-ZGI-Estimated-Output-Tokens"
)

type platformProxyContextKey struct{}

type platformProxyMetadata struct {
	BillingOrganizationID string
	RequestID             string
	AttemptID             string
	DeductionID           string
	APIKeyID              string
	WorkspaceID           string
	AppID                 string
	AppType               string
	ModelName             string
	ProviderName          string
	IsStreaming           bool
	RoutingStrategy       string
	EstimatedInputTokens  int
	EstimatedOutputTokens int
}

func withPlatformProxyMetadata(ctx context.Context, bc *BillingContext) context.Context {
	if ctx == nil || bc == nil {
		return ctx
	}
	lane, err := normalizeBillingContextUsageLane(bc)
	if err != nil || lane != UsageBillingLanePlatform {
		return ctx
	}

	meta := platformProxyMetadata{
		BillingOrganizationID: strings.TrimSpace(bc.OrganizationID),
		RequestID:             strings.TrimSpace(bc.RequestID),
		AttemptID:             strings.TrimSpace(bc.AttemptID),
		DeductionID:           strings.TrimSpace(bc.DeductionID),
		APIKeyID:              strings.TrimSpace(bc.APIKeyID),
		WorkspaceID:           strings.TrimSpace(bc.WorkspaceID),
		ModelName:             strings.TrimSpace(bc.ModelName),
		ProviderName:          strings.TrimSpace(bc.ProviderName),
		IsStreaming:           bc.IsStreaming,
		RoutingStrategy:       strings.TrimSpace(bc.RoutingStrategy),
		EstimatedInputTokens:  bc.EstimatedInputTokens,
		EstimatedOutputTokens: bc.EstimatedOutputTokens,
	}
	if bc.AppID != nil {
		meta.AppID = bc.AppID.String()
	}
	if bc.AppType != nil {
		meta.AppType = strings.TrimSpace(*bc.AppType)
	}

	return context.WithValue(ctx, platformProxyContextKey{}, meta)
}

func applyPlatformProxyHeaders(req *http.Request) {
	if req == nil {
		return
	}
	meta, ok := req.Context().Value(platformProxyContextKey{}).(platformProxyMetadata)
	if !ok {
		return
	}
	setHeaderIfNotEmpty(req, headerZGIBillingOrganizationID, meta.BillingOrganizationID)
	setHeaderIfNotEmpty(req, headerZGIRequestID, meta.RequestID)
	setHeaderIfNotEmpty(req, headerZGIAttemptID, meta.AttemptID)
	setHeaderIfNotEmpty(req, headerZGIDeductionID, meta.DeductionID)
	setHeaderIfNotEmpty(req, headerZGIAPIKeyID, meta.APIKeyID)
	setHeaderIfNotEmpty(req, headerZGIWorkspaceID, meta.WorkspaceID)
	setHeaderIfNotEmpty(req, headerZGIAppID, meta.AppID)
	setHeaderIfNotEmpty(req, headerZGIAppType, meta.AppType)
	setHeaderIfNotEmpty(req, headerZGIModelName, meta.ModelName)
	setHeaderIfNotEmpty(req, headerZGIProviderName, meta.ProviderName)
	setHeaderIfNotEmpty(req, headerZGIRoutingStrategy, meta.RoutingStrategy)
	if meta.EstimatedInputTokens > 0 || meta.EstimatedOutputTokens > 0 {
		req.Header.Set(headerZGIEstimatedInputTokens, strconv.Itoa(meta.EstimatedInputTokens))
		req.Header.Set(headerZGIEstimatedOutputTokens, strconv.Itoa(meta.EstimatedOutputTokens))
	}
	if meta.IsStreaming {
		req.Header.Set(headerZGIIsStreaming, "true")
	}
}

func setHeaderIfNotEmpty(req *http.Request, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	req.Header.Set(key, value)
}
