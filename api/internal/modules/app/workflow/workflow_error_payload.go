package workflow

import (
	"errors"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/pkg/response"
)

var errWorkflowEventPersistenceFailed = errors.New("workflow event persistence failed")

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
