package workflow

import (
	"errors"

	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	"github.com/zgiai/ginext/pkg/response"
)

func buildWorkflowStreamErrorPayload(err error) map[string]any {
	code, message, ok := workflowBillingErrorCodeAndMessage(err)
	if !ok {
		message = workflowFallbackErrorMessage(err)
		return map[string]any{"message": message}
	}

	return map[string]any{
		"message": message,
		"code":    code,
		"params":  map[string]any{},
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
	default:
		return 0, "", false
	}
}
