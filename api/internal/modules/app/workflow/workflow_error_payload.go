package workflow

import (
	"context"
	"errors"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/response"
)

var errWorkflowEventPersistenceFailed = errors.New("workflow event persistence failed")

func workflowNodeFailureReport(err error) (observability.ErrorClassification, observability.Level) {
	classification := observability.ErrorClassification{
		Category: observability.ErrorCategoryApplication,
		Source:   observability.ErrorSourceZGI,
		Code:     "workflow_node_failed",
	}
	if errors.Is(err, errWorkflowEventPersistenceFailed) || gateway.IsProviderSelectionPreparationError(err) {
		if databaseClassification := database.ClassifyError(err); databaseClassification.Source == observability.ErrorSourceInfrastructure {
			return databaseClassification, observability.LevelError
		}
		code := "provider_selection_preparation_failed"
		if errors.Is(err, errWorkflowEventPersistenceFailed) {
			code = "workflow_persistence_failed"
		}
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryApplication,
			Source:   observability.ErrorSourceZGI,
			Code:     code,
		}, observability.LevelError
	}
	if errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable) {
		return observability.ErrorClassification{
			Category:  observability.ErrorCategoryDependency,
			Source:    observability.ErrorSourceProvider,
			Code:      "private_channel_upstream_unavailable",
			Retryable: true,
		}, observability.LevelError
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, adapter.ErrTimeout) || errors.Is(err, llmerrors.DomainErrUpstreamTimeout) {
		return observability.ErrorClassification{
			Category:  observability.ErrorCategoryTimeout,
			Source:    observability.ErrorSourceProvider,
			Code:      "upstream_timeout",
			Retryable: true,
		}, observability.LevelError
	}
	if errors.Is(err, adapter.ErrRateLimited) {
		return observability.ErrorClassification{
			Category:  observability.ErrorCategoryDependency,
			Source:    observability.ErrorSourceProvider,
			Code:      "upstream_rate_limited",
			Retryable: true,
		}, observability.LevelError
	}
	if errors.Is(err, llmerrors.DomainErrRateLimitExceeded) {
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "rate_limit_exceeded",
		}, observability.LevelWarning
	}
	if errors.Is(err, adapter.ErrUpstreamError) || errors.Is(err, adapter.ErrProxyError) || errors.Is(err, adapter.ErrPlatformChannelUnavailable) || errors.Is(err, llmerrors.DomainErrUpstreamUnavailable) {
		return observability.ErrorClassification{
			Category:  observability.ErrorCategoryDependency,
			Source:    observability.ErrorSourceProvider,
			Code:      "upstream_unavailable",
			Retryable: true,
		}, observability.LevelError
	}
	if adapter.IsDeterministicRejection(err) {
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "llm_request_rejected",
		}, observability.LevelWarning
	}
	if errors.Is(err, gateway.ErrNoProviderAvailable) || errors.Is(err, llmerrors.DomainErrNoProviderAvailable) {
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "no_provider_available",
		}, observability.LevelWarning
	}
	if errors.Is(err, llmerrors.DomainErrModelNotFound) || errors.Is(err, llmerrors.DomainErrRouteNotFound) {
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "model_route_not_configured",
		}, observability.LevelWarning
	}

	var billingErr *gateway.BillingUserError
	if errors.As(err, &billingErr) && billingErr != nil {
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     string(billingErr.Kind),
		}, observability.LevelWarning
	}
	if errors.Is(err, llmerrors.DomainErrInsufficientBalance) {
		return observability.ErrorClassification{
			Category: observability.ErrorCategoryConfiguration,
			Source:   observability.ErrorSourceTenant,
			Code:     "insufficient_balance",
		}, observability.LevelWarning
	}
	return classification, observability.LevelError
}

func shouldReportWorkflowNodeFailure(err error) bool {
	// Credential ownership is only known at the selected Gateway channel. The
	// Gateway already emits the authoritative tenant- or platform-owned event;
	// the workflow runtime log remains available without creating a conflicting
	// owner-agnostic Sentry incident here.
	return !gateway.IsPersistentChannelFailure(err)
}

func buildWorkflowStreamErrorPayload(err error) map[string]any {
	if errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
		return map[string]any{
			"message": "Workflow execution ownership was lost.",
			"code":    "workflow_execution_ownership_lost",
		}
	}
	if errors.Is(err, errWorkflowEventPersistenceFailed) {
		return map[string]any{
			"message": "Workflow event persistence failed.",
			"code":    "workflow_event_persistence_failed",
		}
	}
	if errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable) {
		return map[string]any{
			"message": response.ErrWorkflowPrivateChannelUpstreamUnavailable.Message,
			"code":    response.ErrWorkflowPrivateChannelUpstreamUnavailable.Code,
		}
	}

	code, message, ok := workflowBillingErrorCodeAndMessage(err)
	if !ok {
		message = workflowFallbackErrorMessage(err)
		return map[string]any{"message": message}
	}

	return map[string]any{
		"message": message,
		"code":    code,
		"params":  workflowBillingErrorParams(err),
	}
}

func workflowStreamErrorMessage(payload map[string]any) string {
	if payload == nil {
		return "unknown error"
	}
	if message, ok := payload["message"].(string); ok && message != "" {
		return message
	}
	return "unknown error"
}

func workflowFallbackErrorMessage(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

func workflowBillingErrorCodeAndMessage(err error) (int, string, bool) {
	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) || userErr == nil {
		return 0, "", false
	}

	switch userErr.Kind {
	case gateway.BillingUserErrorKindOrganizationBalanceInsufficient:
		return response.ErrWorkflowOrganizationBalanceInsufficient.Code, response.ErrWorkflowOrganizationBalanceInsufficient.Message, true
	case gateway.BillingUserErrorKindWorkspaceQuotaInsufficient:
		return response.ErrWorkflowWorkspaceQuotaInsufficient.Code, response.ErrWorkflowWorkspaceQuotaInsufficient.Message, true
	case gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient:
		return response.ErrWorkflowPrivateChannelBalanceInsufficient.Code, response.ErrWorkflowPrivateChannelBalanceInsufficient.Message, true
	case gateway.BillingUserErrorKindModelPricingNotConfigured:
		return response.ErrWorkflowModelPricingNotConfigured.Code, response.ErrWorkflowModelPricingNotConfigured.Message, true
	default:
		return 0, "", false
	}
}

func workflowBillingErrorParams(err error) map[string]any {
	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) || userErr == nil || len(userErr.Params) == 0 {
		return map[string]any{}
	}
	params := make(map[string]any, len(userErr.Params))
	for key, value := range userErr.Params {
		params[key] = value
	}
	return params
}
