package gateway

import (
	"errors"
	"fmt"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
)

type BillingUserErrorKind string

const (
	BillingUserErrorKindOrganizationBalanceInsufficient   BillingUserErrorKind = "organization_balance_insufficient"
	BillingUserErrorKindWorkspaceQuotaInsufficient        BillingUserErrorKind = "workspace_quota_insufficient"
	BillingUserErrorKindPrivateChannelBalanceInsufficient BillingUserErrorKind = "private_channel_balance_insufficient"
	BillingUserErrorKindModelPricingNotConfigured         BillingUserErrorKind = "model_pricing_not_configured"
)

type BillingUserError struct {
	Kind   BillingUserErrorKind
	Cause  error
	Params map[string]interface{}
}

func (e *BillingUserError) Error() string {
	if e == nil {
		return "billing user error"
	}
	return string(e.Kind)
}

func (e *BillingUserError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func wrapBillingUserError(kind BillingUserErrorKind, cause error, params ...map[string]interface{}) error {
	if cause == nil {
		cause = ErrBillingFailed
	}
	return fmt.Errorf("%w: %w", llmerrors.DomainErrBillingFailed, &BillingUserError{
		Kind:   kind,
		Cause:  cause,
		Params: firstBillingUserErrorParams(params...),
	})
}

func firstBillingUserErrorParams(params ...map[string]interface{}) map[string]interface{} {
	for _, item := range params {
		if len(item) == 0 {
			continue
		}
		result := make(map[string]interface{}, len(item))
		for key, value := range item {
			result[key] = value
		}
		return result
	}
	return nil
}

func organizationBalanceInsufficientError() error {
	return wrapBillingUserError(BillingUserErrorKindOrganizationBalanceInsufficient, ErrInsufficientBalance)
}

func privateChannelBalanceInsufficientError() error {
	return wrapBillingUserError(BillingUserErrorKindPrivateChannelBalanceInsufficient, ErrInsufficientBalance)
}

func workspaceQuotaInsufficientError(cause error) error {
	if cause == nil || !errors.Is(cause, ErrInsufficientQuota) {
		cause = ErrInsufficientQuota
	}
	return wrapBillingUserError(BillingUserErrorKindWorkspaceQuotaInsufficient, cause)
}

func modelPricingNotConfiguredError(cause error, params ...map[string]interface{}) error {
	if cause == nil || !errors.Is(cause, ErrPricingNotConfigured) {
		cause = ErrPricingNotConfigured
	}
	return wrapBillingUserError(BillingUserErrorKindModelPricingNotConfigured, cause, params...)
}

func wrapPricingNotConfiguredError(err error, params ...map[string]interface{}) error {
	if errors.Is(err, ErrPricingNotConfigured) {
		return modelPricingNotConfiguredError(err, params...)
	}
	return err
}
